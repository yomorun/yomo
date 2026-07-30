use std::pin::Pin;

use async_stream::try_stream;
use aws_sdk_bedrockruntime::config::{BehaviorVersion, Token};
use aws_sdk_bedrockruntime::primitives::Blob;
use aws_types::region::Region;
use axum::http::StatusCode;
use futures_core::Stream;
use futures_util::StreamExt;
use serde_json::Value;

use crate::llm_provider::ProviderError;
use crate::openai_types::ErrorDetail;
use crate::serve_config::ConfigError;

use super::types::{
    AnthropicErrorEnvelope, AnthropicRequest, AnthropicResponse, BedrockRequest, StreamEvent,
};

#[derive(Clone)]
pub(super) enum Backend {
    Direct(DirectClient),
    Bedrock(BedrockClient),
}

#[derive(Clone)]
pub(super) struct DirectClient {
    pub client: reqwest::Client,
    pub base_url: String,
    pub auth_style: AuthStyle,
    pub api_key: String,
}

#[derive(Clone)]
pub(super) struct BedrockClient {
    pub model_id: String,
    pub aws_region: String,
    pub aws_bearer_token: String,
}

#[derive(Clone, Copy)]
pub(super) enum AuthStyle {
    XApiKey,
    Bearer,
}

pub(super) fn parse_auth_style(value: Option<&String>) -> Result<AuthStyle, ConfigError> {
    let style = value.cloned().unwrap_or_else(|| "x-api-key".to_string());
    match style.as_str() {
        "x-api-key" => Ok(AuthStyle::XApiKey),
        "bearer" => Ok(AuthStyle::Bearer),
        other => Err(ConfigError::InvalidProvider(format!(
            "unknown auth_style: {}",
            other
        ))),
    }
}

#[derive(Debug)]
pub(super) enum ClientError {
    Http(reqwest::Error),
    Parse(String),
    Api {
        status: StatusCode,
        error: ErrorDetail,
    },
    Internal(String),
}

impl std::fmt::Display for ClientError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ClientError::Http(err) => write!(f, "http error: {err}"),
            ClientError::Parse(err) => write!(f, "parse error: {err}"),
            ClientError::Api { status, error } => {
                write!(f, "api error {status}: {}", error.message)
            }
            ClientError::Internal(err) => write!(f, "internal error: {err}"),
        }
    }
}

impl From<reqwest::Error> for ClientError {
    fn from(value: reqwest::Error) -> Self {
        Self::Http(value)
    }
}

pub(super) fn map_client_error(err: ClientError) -> ProviderError {
    match err {
        ClientError::Api { status, error } if status == StatusCode::BAD_REQUEST => {
            ProviderError::Public { status, error }
        }
        ClientError::Api { status, error } => {
            ProviderError::internal_with_upstream_status(status, error.message)
        }
        other => ProviderError::internal(other.to_string()),
    }
}

impl DirectClient {
    pub(super) async fn send_complete(
        &self,
        request: AnthropicRequest,
        anthropic_version: &str,
    ) -> Result<AnthropicResponse, ClientError> {
        let mut builder = self
            .client
            .post(format!("{}/messages", self.base_url.trim_end_matches('/')))
            .header("anthropic-version", anthropic_version)
            .header(reqwest::header::CONTENT_TYPE, "application/json");
        builder = match self.auth_style {
            AuthStyle::XApiKey => builder.header("x-api-key", self.api_key.as_str()),
            AuthStyle::Bearer => builder.bearer_auth(&self.api_key),
        };

        let response = builder.json(&request).send().await?;
        let status = response.status();
        let bytes = response.bytes().await?;
        if !status.is_success() {
            return Err(parse_api_error(status, &bytes));
        }
        serde_json::from_slice::<AnthropicResponse>(&bytes)
            .map_err(|err| ClientError::Parse(err.to_string()))
    }

    pub(super) async fn send_stream(
        &self,
        request: AnthropicRequest,
        anthropic_version: &str,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<StreamEvent, ProviderError>> + Send>>, ClientError>
    {
        let mut builder = self
            .client
            .post(format!("{}/messages", self.base_url.trim_end_matches('/')))
            .header("anthropic-version", anthropic_version)
            .header(reqwest::header::CONTENT_TYPE, "application/json");
        builder = match self.auth_style {
            AuthStyle::XApiKey => builder.header("x-api-key", self.api_key.as_str()),
            AuthStyle::Bearer => builder.bearer_auth(&self.api_key),
        };

        let response = builder.json(&request).send().await?;
        let status = response.status();
        if !status.is_success() {
            let bytes = response.bytes().await?;
            return Err(parse_api_error(status, &bytes));
        }

        let stream = response.bytes_stream();
        Ok(Box::pin(try_stream! {
            futures_util::pin_mut!(stream);
            let mut buffer = String::new();
            while let Some(item) = stream.next().await {
                let chunk = item.map_err(ClientError::from).map_err(map_client_error)?;
                let text = String::from_utf8_lossy(&chunk);
                buffer.push_str(&text);

                while let Some(pos) = buffer.find('\n') {
                    let line = buffer[..pos].trim().to_string();
                    buffer.drain(..=pos);

                    if line.is_empty() || line.starts_with("event:") {
                        continue;
                    }
                    if let Some(data) = line.strip_prefix("data:") {
                        let payload = data.trim();
                        if payload.is_empty() || payload == "[DONE]" {
                            continue;
                        }
                        let event: StreamEvent = serde_json::from_str(payload).map_err(|err| {
                            ProviderError::internal(format!("parse anthropic stream event: {err}"))
                        })?;
                        yield event;
                    }
                }
            }
        }))
    }
}

impl BedrockClient {
    pub(super) async fn send_complete(
        &self,
        request: BedrockRequest,
    ) -> Result<AnthropicResponse, ClientError> {
        let client = self.client().await?;
        let body =
            serde_json::to_vec(&request).map_err(|err| ClientError::Parse(err.to_string()))?;
        let response = client
            .invoke_model()
            .model_id(&self.model_id)
            .content_type("application/json")
            .accept("application/json")
            .body(Blob::new(body))
            .send()
            .await
            .map_err(|err| {
                ClientError::Internal(format!("bedrock invoke_model failed: {err:?}"))
            })?;

        serde_json::from_slice::<AnthropicResponse>(&response.body.into_inner())
            .map_err(|err| ClientError::Parse(err.to_string()))
    }

    pub(super) async fn send_stream(
        &self,
        request: BedrockRequest,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<StreamEvent, ProviderError>> + Send>>, ClientError>
    {
        let client = self.client().await?;
        let body =
            serde_json::to_vec(&request).map_err(|err| ClientError::Parse(err.to_string()))?;
        let response = client
            .invoke_model_with_response_stream()
            .model_id(&self.model_id)
            .content_type("application/json")
            .body(Blob::new(body))
            .send()
            .await
            .map_err(|err| {
                ClientError::Internal(format!("bedrock stream invoke failed: {err:?}"))
            })?;

        let mut stream = response.body;
        Ok(Box::pin(try_stream! {
            loop {
                match stream.recv().await {
                    Ok(Some(event)) => {
                        if let Ok(chunk) = event.as_chunk() {
                            if let Some(payload) = chunk.bytes.as_ref() {
                                let parsed = serde_json::from_slice::<StreamEvent>(payload.as_ref())
                                    .map_err(|err| ProviderError::internal(format!("parse bedrock stream event: {err}")))?;
                                yield parsed;
                            }
                        }
                    }
                    Ok(None) => break,
                    Err(err) => {
                        Err(ProviderError::internal(format!("bedrock stream recv failed: {err}")))?
                    }
                }
            }
        }))
    }

    async fn client(&self) -> Result<aws_sdk_bedrockruntime::Client, ClientError> {
        let config = aws_sdk_bedrockruntime::Config::builder()
            .behavior_version(BehaviorVersion::latest())
            .region(Region::new(self.aws_region.clone()))
            .bearer_token(Token::new(&self.aws_bearer_token, None))
            .build();
        Ok(aws_sdk_bedrockruntime::Client::from_conf(config))
    }
}

fn parse_api_error(status: reqwest::StatusCode, bytes: &[u8]) -> ClientError {
    if let Ok(envelope) = serde_json::from_slice::<AnthropicErrorEnvelope>(bytes) {
        return ClientError::Api {
            status,
            error: ErrorDetail {
                message: envelope.error.message,
                r#type: envelope
                    .error
                    .r#type
                    .unwrap_or_else(|| default_error_type(status).to_string()),
                code: None,
                param: None,
            },
        };
    }

    if let Ok(value) = serde_json::from_slice::<Value>(bytes) {
        let message = value
            .get("error")
            .and_then(|error| error.get("message"))
            .and_then(Value::as_str)
            .or_else(|| value.get("message").and_then(Value::as_str))
            .unwrap_or("unknown anthropic error")
            .to_string();
        return ClientError::Api {
            status,
            error: ErrorDetail {
                message,
                r#type: default_error_type(status).to_string(),
                code: None,
                param: None,
            },
        };
    }

    ClientError::Api {
        status,
        error: ErrorDetail {
            message: String::from_utf8_lossy(bytes).to_string(),
            r#type: default_error_type(status).to_string(),
            code: None,
            param: None,
        },
    }
}

fn default_error_type(status: StatusCode) -> &'static str {
    if status == StatusCode::BAD_REQUEST {
        return "invalid_request_error";
    }
    "internal_error"
}
