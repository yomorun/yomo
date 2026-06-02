use std::sync::Arc;
use std::time::Duration;

use async_stream::try_stream;
use futures_core::Stream;
use futures_util::StreamExt;
use log::debug;
use reqwest::StatusCode;
use serde_json::Value;
use tokio::time::timeout;

use crate::openai_types::{
    ChatCompletionChunk, ChatCompletionRequest, ChatCompletionResponse, ErrorDetail, ErrorResponse,
};

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
    OpenAI {
        status: StatusCode,
        error: ErrorDetail,
    },
    Custom(Value),
    Unknown {
        status: StatusCode,
        body: String,
    },
}

impl std::fmt::Display for ApiError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ApiError::OpenAI { status, error } => {
                write!(f, "status {status}, {}", error.message)
            }
            ApiError::Custom(value) => write!(f, "custom error: {value}"),
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
}

pub const DEFAULT_REQUEST_TIMEOUT_SECS: u64 = 300;
pub const DEFAULT_STREAM_FIRST_BYTE_TIMEOUT_SECS: u64 = 60;
pub const DEFAULT_STREAM_IDLE_TIMEOUT_SECS: u64 = 30;

impl Config {
    pub fn new(api_key: impl Into<String>) -> Self {
        Self {
            api_key: api_key.into(),
            base_url: "https://api.openai.com/v1".to_string(),
            request_timeout: Duration::from_secs(DEFAULT_REQUEST_TIMEOUT_SECS),
            stream_first_byte_timeout: Duration::from_secs(DEFAULT_STREAM_FIRST_BYTE_TIMEOUT_SECS),
            stream_idle_timeout: Duration::from_secs(DEFAULT_STREAM_IDLE_TIMEOUT_SECS),
            error_parser: None,
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
    pub fn new(config: Config) -> Result<Self, ClientError> {
        let client = reqwest::Client::builder()
            .timeout(config.request_timeout)
            .build()?;
        Ok(Self { client, config })
    }

    pub fn with_client(config: Config, client: reqwest::Client) -> Self {
        Self { client, config }
    }

    pub async fn chat_completions(
        &self,
        request: ChatCompletionRequest,
    ) -> Result<ChatCompletionResponse, ClientError> {
        let url = format!(
            "{}/chat/completions",
            trimmed_base_url(&self.config.base_url)
        );
        let response = self
            .client
            .post(url)
            .bearer_auth(&self.config.api_key)
            .json(&request)
            .send()
            .await?;

        let status = response.status();
        let bytes = response.bytes().await?;
        debug_response_json("non_stream", Some(status), &bytes);

        if !status.is_success() {
            return Err(self.parse_error(status, &bytes));
        }

        let parsed: ChatCompletionResponse = serde_json::from_slice(&bytes)
            .map_err(|err| ClientError::InvalidResponse(err.to_string()))?;
        Ok(parsed)
    }

    pub async fn chat_completions_stream(
        &self,
        mut request: ChatCompletionRequest,
    ) -> Result<impl Stream<Item = Result<ChatCompletionChunk, ClientError>>, ClientError> {
        request.stream = Some(true);
        let url = format!(
            "{}/chat/completions",
            trimmed_base_url(&self.config.base_url)
        );
        let response = timeout(
            self.config.stream_first_byte_timeout,
            self.client
                .post(url)
                .bearer_auth(&self.config.api_key)
                .json(&request)
                .send(),
        )
        .await
        .map_err(|_| ClientError::Timeout("stream first byte timeout".to_string()))??;

        let status = response.status();
        if !status.is_success() {
            let bytes = response.bytes().await?;
            debug_response_json("stream", Some(status), &bytes);
            return Err(self.parse_error(status, &bytes));
        }

        let mut stream = response.bytes_stream();
        let parse_error = self.config.error_parser.clone();

        Ok(try_stream! {
            let mut buffer = String::new();
            loop {
                let chunk = timeout(self.config.stream_idle_timeout, stream.next())
                    .await
                    .map_err(|_| ClientError::Timeout("stream idle timeout".to_string()))?;
                let Some(chunk) = chunk else {
                    break;
                };
                let chunk = chunk.map_err(ClientError::Http)?;
                debug_response_json("stream_chunk", None, &chunk);
                let text = String::from_utf8_lossy(&chunk);
                buffer.push_str(&text);

                while let Some(pos) = buffer.find('\n') {
                    let line = buffer[..pos].trim().to_string();
                    buffer.drain(..=pos);

                    if line.is_empty() {
                        continue;
                    }

                    if let Some(data) = line.strip_prefix("data: ") {
                        debug_stream_event_json(data);
                        if data == "[DONE]" {
                            return;
                        }

                        match serde_json::from_str::<ChatCompletionChunk>(data) {
                            Ok(chunk) => yield chunk,
                            Err(err) => {
                                let raw = data.as_bytes();
                                if let Some(parser) = &parse_error {
                                    if let Some(custom) = (parser)(raw) {
                                        Err(ClientError::Api(ApiError::Custom(custom)))?;
                                    }
                                }
                                Err(ClientError::InvalidResponse(err.to_string()))?;
                            }
                        }
                    }
                }
            }
        })
    }

    pub(crate) fn parse_error(&self, status: StatusCode, body: &[u8]) -> ClientError {
        if let Some(parser) = &self.config.error_parser {
            if let Some(custom) = (parser)(body) {
                return ClientError::Api(ApiError::Custom(custom));
            }
        }

        if let Ok(parsed) = serde_json::from_slice::<ErrorResponse>(body) {
            return ClientError::Api(ApiError::OpenAI {
                status,
                error: parsed.error,
            });
        }

        let text = String::from_utf8_lossy(body).to_string();
        ClientError::Api(ApiError::Unknown { status, body: text })
    }
}

const MAX_DEBUG_BODY_BYTES: usize = 8 * 1024;

fn debug_body(bytes: &[u8]) -> String {
    let body = String::from_utf8_lossy(bytes);
    let compact = compact_json_string(&body);
    truncate_for_debug(&compact)
}

fn debug_body_value(bytes: &[u8]) -> Value {
    let body = String::from_utf8_lossy(bytes);
    serde_json::from_str::<Value>(&body).unwrap_or_else(|_| Value::String(debug_body(bytes)))
}

fn truncate_for_debug(value: &str) -> String {
    if value.len() <= MAX_DEBUG_BODY_BYTES {
        return value.to_string();
    }
    let mut end = MAX_DEBUG_BODY_BYTES;
    while !value.is_char_boundary(end) {
        end -= 1;
    }
    format!(
        "{}...[truncated {} bytes]",
        &value[..end],
        value.len() - MAX_DEBUG_BODY_BYTES
    )
}

fn debug_response_json(event: &str, status: Option<StatusCode>, body: &[u8]) {
    let truncated = body.len() > MAX_DEBUG_BODY_BYTES;
    let payload = serde_json::json!({
        "target": "openai_compatible.client.response",
        "event": event,
        "status": status.map(|value| value.as_u16()),
        "body": debug_body_value(body),
        "truncated": truncated,
    });
    debug!("{}", payload);
}

fn debug_stream_event_json(data: &str) {
    let compact = compact_json_string(data);
    let data_value = serde_json::from_str::<Value>(&compact)
        .unwrap_or_else(|_| Value::String(truncate_for_debug(&compact)));
    let payload = serde_json::json!({
        "target": "openai_compatible.client.response",
        "event": "stream_event",
        "data": data_value,
        "truncated": compact.len() > MAX_DEBUG_BODY_BYTES,
    });
    debug!("{}", payload);
}

fn compact_json_string(value: &str) -> String {
    serde_json::from_str::<Value>(value)
        .map(|json| json.to_string())
        .unwrap_or_else(|_| value.to_string())
}

pub(crate) fn trimmed_base_url(base_url: &str) -> &str {
    base_url.trim_end_matches('/')
}
