use std::sync::Arc;
use std::time::Duration;

use async_stream::try_stream;
use futures_core::Stream;
use futures_util::StreamExt;
use reqwest::StatusCode;
use serde_json::Value;
use tokio::time::timeout;

use crate::dialects::Dialect;
use crate::dialects::credential::sanitized_transport_headers;
use crate::openai_types::{
    ChatCompletionChunk, ChatCompletionRequest, ChatCompletionResponse, ErrorDetail,
};
use crate::provider::UpstreamLog;
use crate::serve_config::{AuthScheme, ModelBinding};
use crate::upstreams::{Upstream, UpstreamError, parse_error, request_path};
use crate::utils::SseDataReader;
use axum::http::HeaderMap;
use reqwest::RequestBuilder;

#[derive(Debug)]
pub enum ClientError {
    Http(reqwest::Error),
    InvalidRequest(String),
    InvalidResponse(String),
    Timeout(String),
    Api(ApiError),
}

impl std::fmt::Display for ClientError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ClientError::Http(err) => write!(f, "http error: {err}"),
            ClientError::InvalidRequest(message) => write!(f, "invalid request: {message}"),
            ClientError::InvalidResponse(message) => write!(f, "invalid response: {message}"),
            ClientError::Timeout(message) => write!(f, "timeout: {message}"),
            ClientError::Api(err) => write!(f, "api error: {err}"),
        }
    }
}

impl std::error::Error for ClientError {}

impl From<reqwest::Error> for ClientError {
    fn from(err: reqwest::Error) -> Self {
        ClientError::Http(err)
    }
}

#[derive(Debug)]
pub enum ApiError {
    /// Explicit vendor error interpretation with original diagnostic context.
    Upstream(Box<UpstreamError>),
    OpenAI {
        status: StatusCode,
        error: ErrorDetail,
    },
    Custom {
        status: Option<StatusCode>,
        value: Value,
    },
    Unknown {
        status: StatusCode,
        body: String,
    },
}

impl std::fmt::Display for ApiError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ApiError::Upstream(error) => write!(f, "status {}, {}", error.status, error.message),
            ApiError::OpenAI { status, error } => {
                write!(f, "status {status}, {}", error.message)
            }
            ApiError::Custom { value, .. } => write!(f, "custom error: {value}"),
            ApiError::Unknown { status, body } => write!(f, "status {status}, {body}"),
        }
    }
}

#[derive(Clone)]
pub struct Config {
    pub api_key: String,
    pub base_url: String,
    pub request_timeout: Duration,
    pub stream_first_byte_timeout: Duration,
    pub stream_idle_timeout: Duration,
    pub error_parser: Option<Arc<dyn Fn(&[u8]) -> Option<Value> + Send + Sync>>,
    /// Fixed vendor integration; no URL or model-prefix inference.
    pub binding: Option<ModelBinding>,
    /// Authentication scheme for both streaming and complete calls.
    pub auth_scheme: AuthScheme,
}

pub const DEFAULT_REQUEST_TIMEOUT_SECS: u64 = 600;
pub const DEFAULT_STREAM_FIRST_BYTE_TIMEOUT_SECS: u64 = 180;
pub const DEFAULT_STREAM_IDLE_TIMEOUT_SECS: u64 = 180;

impl Config {
    pub fn new(api_key: impl Into<String>) -> Self {
        Self {
            api_key: api_key.into(),
            base_url: "https://api.openai.com/v1".to_string(),
            request_timeout: Duration::from_secs(DEFAULT_REQUEST_TIMEOUT_SECS),
            stream_first_byte_timeout: Duration::from_secs(DEFAULT_STREAM_FIRST_BYTE_TIMEOUT_SECS),
            stream_idle_timeout: Duration::from_secs(DEFAULT_STREAM_IDLE_TIMEOUT_SECS),
            error_parser: None,
            binding: None,
            auth_scheme: AuthScheme::Bearer,
        }
    }

    pub fn base_url(mut self, base_url: impl Into<String>) -> Self {
        self.base_url = base_url.into();
        self
    }

    pub fn error_parser(
        mut self,
        parser: impl Fn(&[u8]) -> Option<Value> + Send + Sync + 'static,
    ) -> Self {
        self.error_parser = Some(Arc::new(parser));
        self
    }

    pub fn request_timeout_secs(mut self, timeout_secs: u64) -> Self {
        self.request_timeout = Duration::from_secs(timeout_secs);
        self
    }

    pub fn stream_first_byte_timeout_secs(mut self, timeout_secs: u64) -> Self {
        self.stream_first_byte_timeout = Duration::from_secs(timeout_secs);
        self
    }

    pub fn stream_idle_timeout_secs(mut self, timeout_secs: u64) -> Self {
        self.stream_idle_timeout = Duration::from_secs(timeout_secs);
        self
    }
}

#[derive(Clone)]
pub struct Client {
    pub(crate) client: reqwest::Client,
    pub(crate) config: Config,
}

impl Client {
    fn apply_auth(&self, builder: RequestBuilder) -> RequestBuilder {
        if self.config.api_key.trim().is_empty() {
            return builder;
        }
        match self.config.auth_scheme {
            AuthScheme::Bearer => apply_optional_bearer_auth(builder, &self.config.api_key),
            AuthScheme::ApiKey => builder.header("api-key", &self.config.api_key),
        }
    }

    fn request_body(&self, request: &ChatCompletionRequest) -> Result<Value, ClientError> {
        let mut request = request.clone();
        request.normalize_reasoning_compat_fields();
        let mut body = serde_json::to_value(&request)
            .map_err(|error| ClientError::InvalidRequest(error.to_string()))?;
        if let Some(upstream) = self.config.binding.as_ref().map(|binding| binding.upstream) {
            match upstream {
                Upstream::AzureOpenai => {
                    crate::upstreams::azure_openai::encode_chat_reasoning(&mut body)
                }
                Upstream::Tokenhub => crate::upstreams::tokenhub::encode_chat_reasoning(&mut body),
                Upstream::VolcengineArk => {
                    crate::upstreams::volcengine_ark::encode_chat_reasoning(&mut body)
                }
                Upstream::Vllm => crate::upstreams::vllm::encode_chat_reasoning(&mut body),
                _ => {}
            }
        }
        Ok(body)
    }
    pub fn new(config: Config) -> Result<Self, ClientError> {
        let client = reqwest::Client::builder()
            .timeout(config.request_timeout)
            .build()?;
        Ok(Self { client, config })
    }

    /// Build a client around a shared HTTP client. Explicit upstream bindings
    /// select reasoning-control encoding for each chat backend.
    pub fn with_client(config: Config, client: reqwest::Client) -> Self {
        Self { client, config }
    }

    pub async fn chat_completions(
        &self,
        mut request: ChatCompletionRequest,
        transport_headers: &HeaderMap,
    ) -> Result<ChatCompletionResponse, ClientError> {
        clear_agent_context(&mut request);
        let url = format!(
            "{}{}",
            trimmed_base_url(&self.config.base_url),
            request_path(Dialect::OpenAiChatCompletions)
        );
        let diagnostics = UpstreamLog::new("openai-chat-completions", &request.model);
        let body = self.request_body(&request)?;
        diagnostics.json("upstream.request", Some(&url), &body);
        let request_builder = apply_transport_headers(self.client.post(url), transport_headers);
        let response = self.apply_auth(request_builder).json(&body).send().await?;

        let status = response.status();
        diagnostics.response_start(status.as_u16(), response.headers());
        let headers = response.headers().clone();
        let bytes = response.bytes().await?;
        diagnostics.bytes("upstream.response_body", &bytes);

        if !status.is_success() {
            if let Some(binding) = &self.config.binding {
                return Err(ClientError::Api(ApiError::Upstream(Box::new(parse_error(
                    binding.upstream,
                    status,
                    &headers,
                    &bytes,
                )))));
            }
            return Err(self.parse_error(status, &bytes));
        }

        let parsed: ChatCompletionResponse = serde_json::from_slice(&bytes)
            .map_err(|err| ClientError::InvalidResponse(err.to_string()))?;
        Ok(parsed)
    }

    pub async fn chat_completions_stream(
        &self,
        mut request: ChatCompletionRequest,
        transport_headers: &HeaderMap,
    ) -> Result<impl Stream<Item = Result<ChatCompletionChunk, ClientError>> + use<'_>, ClientError>
    {
        clear_agent_context(&mut request);
        request.stream = Some(true);
        let url = format!(
            "{}{}",
            trimmed_base_url(&self.config.base_url),
            request_path(Dialect::OpenAiChatCompletions)
        );
        let diagnostics = UpstreamLog::new("openai-chat-completions", &request.model);
        let body = self.request_body(&request)?;
        diagnostics.json("upstream.request", Some(&url), &body);
        let response = timeout(
            self.config.stream_first_byte_timeout,
            self.apply_auth(apply_transport_headers(
                self.client.post(url),
                transport_headers,
            ))
            .json(&body)
            .send(),
        )
        .await
        .map_err(|_| ClientError::Timeout("stream first byte timeout".to_string()))??;

        let status = response.status();
        diagnostics.response_start(status.as_u16(), response.headers());
        if !status.is_success() {
            let headers = response.headers().clone();
            let bytes = response.bytes().await?;
            diagnostics.bytes("upstream.response_body", &bytes);
            if let Some(binding) = &self.config.binding {
                return Err(ClientError::Api(ApiError::Upstream(Box::new(parse_error(
                    binding.upstream,
                    status,
                    &headers,
                    &bytes,
                )))));
            }
            return Err(self.parse_error(status, &bytes));
        }

        let mut stream = response.bytes_stream();
        let custom_error_parser = self.config.error_parser.clone();
        let binding = self.config.binding.clone();

        Ok(try_stream! {
            let mut reader = SseDataReader::default();
            loop {
                let chunk = timeout(self.config.stream_idle_timeout, stream.next())
                    .await
                    .map_err(|_| ClientError::Timeout("stream idle timeout".to_string()))?;
                let Some(chunk) = chunk else {
                    break;
                };
                let chunk = chunk.map_err(ClientError::Http)?;

                for data in reader.push(&chunk) {
                    diagnostics.bytes("upstream.stream_event", data.as_bytes());
                    if data == "[DONE]" {
                        return;
                    }

                    match serde_json::from_str::<ChatCompletionChunk>(&data) {
                        Ok(chunk) => yield chunk,
                        Err(err) => {
                            let raw = data.as_bytes();
                            if let Some(binding) = &binding {
                                Err(ClientError::Api(ApiError::Upstream(Box::new(parse_error(
                                    binding.upstream, StatusCode::BAD_GATEWAY, &HeaderMap::new(), raw,
                                )))))?;
                            }
                            if let Some(parser) = &custom_error_parser {
                                if let Some(custom) = (parser)(raw) {
                                    Err(ClientError::Api(ApiError::Custom {
                                        status: None,
                                        value: custom,
                                    }))?;
                                }
                            }
                            Err(ClientError::InvalidResponse(err.to_string()))?;
                        }
                    }
                }
            }
        })
    }

    pub(crate) fn parse_error(&self, status: StatusCode, body: &[u8]) -> ClientError {
        if let Some(parser) = &self.config.error_parser {
            if let Some(custom) = (parser)(body) {
                return ClientError::Api(ApiError::Custom {
                    status: Some(status),
                    value: custom,
                });
            }
        }

        if let Some(error) = crate::upstreams::azure_openai::parse_compatible_error(status, body) {
            return ClientError::Api(ApiError::OpenAI { status, error });
        }

        let text = String::from_utf8_lossy(body).to_string();
        ClientError::Api(ApiError::Unknown { status, body: text })
    }
}

pub(super) fn apply_optional_bearer_auth(
    builder: reqwest::RequestBuilder,
    api_key: &str,
) -> reqwest::RequestBuilder {
    if api_key.trim().is_empty() {
        builder
    } else {
        builder.bearer_auth(api_key)
    }
}

pub(crate) fn trimmed_base_url(base_url: &str) -> &str {
    base_url.trim_end_matches('/')
}

fn clear_agent_context(request: &mut ChatCompletionRequest) {
    request.agent_context = None;
}

/// Apply application-supplied transport headers before authentication so that
/// provider credentials always win over client-supplied values.
fn apply_transport_headers(builder: RequestBuilder, headers: &HeaderMap) -> RequestBuilder {
    let headers = sanitized_transport_headers(headers);
    if headers.is_empty() {
        return builder;
    }
    builder.headers(headers)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::upstreams::Upstream;
    use reqwest::header;

    #[test]
    fn clear_agent_context_removes_agent_context() {
        let mut request = ChatCompletionRequest {
            model: "gpt-test".to_string(),
            messages: Vec::new(),
            n: None,
            temperature: None,
            top_p: None,
            presence_penalty: None,
            frequency_penalty: None,
            logprobs: None,
            top_logprobs: None,
            modalities: None,
            audio: None,
            max_completion_tokens: None,
            stop: None,
            response_format: None,
            thinking: None,
            reasoning_effort: None,
            chat_template_kwargs: None,
            prediction: None,
            verbosity: None,
            tools: None,
            tool_choice: None,
            allowed_tools: None,
            parallel_tool_calls: None,
            service_tier: None,
            seed: None,
            stream: None,
            stream_options: None,
            metadata: None,
            agent_context: Some(serde_json::json!({"key": "value"})),

            extra: Default::default(),
        };

        clear_agent_context(&mut request);

        assert!(request.agent_context.is_none());
    }

    #[test]
    fn parse_error_accepts_upstream_null_type_for_bad_request() {
        let client = Client::new(Config::new("test-key")).expect("build client");
        let body = br#"{
            "error": {
                "message": "filtered",
                "type": null,
                "param": "prompt",
                "code": "content_filter",
                "status": 400,
                "innererror": {
                    "code": "ResponsibleAIPolicyViolation"
                }
            }
        }"#;

        let parsed = client.parse_error(StatusCode::BAD_REQUEST, body);

        let ClientError::Api(ApiError::OpenAI { status, error }) = parsed else {
            panic!("expected openai api error");
        };
        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert_eq!(error.message, "filtered");
        assert_eq!(error.r#type, "invalid_request_error");
        assert_eq!(error.code.as_deref(), Some("content_filter"));
        assert_eq!(error.param.as_deref(), Some("prompt"));
    }

    #[test]
    fn bound_chat_encoding_keeps_unknown_extensions_and_exact_effort() {
        let mut config = Config::new("test-key");
        config.binding = Some(ModelBinding {
            public_model: "fast".into(),
            model: "glm-5.3-flash".into(),
            provider: "qcloud.cn".into(),
            upstream: Upstream::Tokenhub,
            upstream_model: "vendor-id".into(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "qcloud.cn.vendor-id".into(),
        });
        let client = Client::new(config).expect("client");
        let mut request = ChatCompletionRequest {
            reasoning_effort: Some("max".into()),
            ..Default::default()
        };
        request
            .extra
            .insert("tool_stream".into(), Value::Bool(true));
        let body = client.request_body(&request).expect("encoding");
        assert_eq!(body["reasoning_effort"], "max");
        assert_eq!(body["tool_stream"], true);
    }

    #[test]
    fn api_key_auth_does_not_add_bearer_authentication() {
        let mut config = Config::new("test-key");
        config.auth_scheme = AuthScheme::ApiKey;
        let client = Client::new(config).expect("client");
        let request = client
            .apply_auth(
                client
                    .client
                    .post("https://example.test/v1/chat/completions"),
            )
            .build()
            .expect("request");
        assert_eq!(
            request.headers().get("api-key").expect("api key"),
            "test-key"
        );
        assert!(request.headers().get("authorization").is_none());
    }

    #[test]
    fn transport_headers_are_applied_before_authentication() {
        let client = Client::new(Config::new("server-key")).expect("build client");
        let mut transport = HeaderMap::new();
        transport.insert("x-client-header", "client-value".parse().unwrap());
        transport.insert(header::AUTHORIZATION, "Bearer client-key".parse().unwrap());

        let request = client
            .apply_auth(apply_transport_headers(
                client
                    .client
                    .post("https://example.test/v1/chat/completions"),
                &transport,
            ))
            .build()
            .expect("request");

        assert_eq!(request.headers()["x-client-header"], "client-value");
        assert_eq!(
            request.headers()[header::AUTHORIZATION],
            "Bearer server-key"
        );
    }

    #[test]
    fn transport_headers_are_optional() {
        let client = Client::new(Config::new("server-key")).expect("build client");
        let request = apply_transport_headers(
            client
                .client
                .post("https://example.test/v1/chat/completions"),
            &HeaderMap::new(),
        )
        .build()
        .expect("request");
        assert!(request.headers().is_empty());
    }

    #[test]
    fn chat_body_is_not_rewritten_based_on_upstream_or_model() {
        let mut config = Config::new("");
        config.binding = Some(ModelBinding {
            public_model: "deepseek-v4-flash".into(),
            model: "deepseek-v4-flash".into(),
            provider: "allegro.cn.local".into(),
            upstream: Upstream::OpenaiCompatible,
            upstream_model: "DeepSeek-V4-Flash".into(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "allegro.cn.DeepSeek-V4-Flash".into(),
        });
        let client = Client::new(config).expect("client");
        let request = ChatCompletionRequest {
            reasoning_effort: Some("high".into()),
            chat_template_kwargs: Some(
                serde_json::from_value(serde_json::json!({"existing":true}))
                    .expect("template arguments"),
            ),
            ..Default::default()
        };
        let body = client.request_body(&request).expect("encoded");
        assert_eq!(body["reasoning_effort"], "high");
        assert_eq!(body["chat_template_kwargs"]["existing"], true);
    }

    fn bound_reasoning_client(upstream: Upstream) -> Client {
        let mut config = Config::new("");
        config.binding = Some(ModelBinding {
            public_model: "public-model".into(),
            model: "model".into(),
            provider: "connection".into(),
            upstream,
            upstream_model: "deployment".into(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "connection.deployment".into(),
        });
        Client::new(config).expect("client")
    }

    #[test]
    fn reasoning_controls_encode_for_all_chat_upstreams() {
        let request = ChatCompletionRequest {
            thinking: Some(crate::openai_types::ThinkingConfig {
                kind: crate::openai_types::ThinkingType::Enabled,
                budget_tokens: Some(2048),
            }),
            reasoning_effort: Some("high".into()),
            ..Default::default()
        };

        let azure = bound_reasoning_client(Upstream::AzureOpenai)
            .request_body(&request)
            .expect("Azure body");
        assert_eq!(azure["reasoning_effort"], "high");
        assert!(azure.get("thinking").is_none());
        assert!(azure.get("chat_template_kwargs").is_none());

        for upstream in [Upstream::Tokenhub, Upstream::VolcengineArk] {
            let body = bound_reasoning_client(upstream)
                .request_body(&request)
                .expect("vendor body");
            assert_eq!(body["reasoning_effort"], "high");
            assert_eq!(body["thinking"]["type"], "enabled");
            assert_eq!(body["thinking"]["budget_tokens"], 2048);
            assert!(body.get("chat_template_kwargs").is_none());
        }

        let vllm = bound_reasoning_client(Upstream::Vllm)
            .request_body(&request)
            .expect("vLLM/SGLang body");
        assert!(vllm.get("thinking").is_none());
        assert!(vllm.get("reasoning_effort").is_none());
        assert_eq!(vllm["chat_template_kwargs"]["thinking"], true);
        assert_eq!(vllm["chat_template_kwargs"]["reasoning_effort"], "high");
        assert_eq!(vllm["thinking_token_budget"], 2048);
    }

    #[test]
    fn reasoning_effort_none_disables_thinking_per_upstream() {
        let request = ChatCompletionRequest {
            reasoning_effort: Some("none".into()),
            ..Default::default()
        };

        let azure = bound_reasoning_client(Upstream::AzureOpenai)
            .request_body(&request)
            .expect("Azure body");
        assert_eq!(azure["reasoning_effort"], "none");

        let tokenhub = bound_reasoning_client(Upstream::Tokenhub)
            .request_body(&request)
            .expect("TokenHub body");
        assert_eq!(tokenhub["thinking"]["type"], "disabled");

        let vllm = bound_reasoning_client(Upstream::Vllm)
            .request_body(&request)
            .expect("vLLM body");
        assert_eq!(vllm["chat_template_kwargs"]["thinking"], false);
        assert!(
            vllm["chat_template_kwargs"]
                .get("reasoning_effort")
                .is_none()
        );
    }

    #[test]
    fn numeric_effort_is_nested_for_self_hosted_backends() {
        let request = ChatCompletionRequest {
            reasoning_effort: Some(crate::provider::ReasoningEffort::Numeric(
                serde_json::Number::from(60),
            )),
            ..Default::default()
        };
        let body = bound_reasoning_client(Upstream::Vllm)
            .request_body(&request)
            .expect("vLLM body");
        assert_eq!(body["chat_template_kwargs"]["reasoning_effort"], 60);
    }

    #[test]
    fn parse_error_defaults_null_type_to_internal_error_for_non_bad_request() {
        let client = Client::new(Config::new("test-key")).expect("build client");
        let body = br#"{
            "error": {
                "message": "upstream failed",
                "type": null,
                "code": "server_error"
            }
        }"#;

        let parsed = client.parse_error(StatusCode::INTERNAL_SERVER_ERROR, body);

        let ClientError::Api(ApiError::OpenAI { status, error }) = parsed else {
            panic!("expected openai api error");
        };
        assert_eq!(status, StatusCode::INTERNAL_SERVER_ERROR);
        assert_eq!(error.r#type, "internal_error");
    }

    #[test]
    fn optional_bearer_auth_skips_header_for_empty_key() {
        let request =
            apply_optional_bearer_auth(reqwest::Client::new().post("https://example.com"), " ")
                .build()
                .expect("build request");
        assert!(request.headers().get(header::AUTHORIZATION).is_none());
    }

    #[test]
    fn optional_bearer_auth_sets_header_for_non_empty_key() {
        let request =
            apply_optional_bearer_auth(reqwest::Client::new().post("https://example.com"), "key")
                .build()
                .expect("build request");
        assert_eq!(
            request.headers().get(header::AUTHORIZATION).unwrap(),
            "Bearer key"
        );
    }
}
