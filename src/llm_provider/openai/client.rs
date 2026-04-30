use std::sync::Arc;
use std::time::Duration;

use async_stream::try_stream;
use futures_core::Stream;
use futures_util::StreamExt;
use reqwest::StatusCode;
use serde_json::Value;
use tokio::time::timeout;

use crate::openai_types::{
    ApiError, ChatCompletionChunk, ChatCompletionRequest, ChatCompletionResponse, ClientError,
    ErrorResponse,
};

#[derive(Clone)]
pub struct Config {
    pub api_key: String,
    pub base_url: String,
    pub error_parser: Option<Arc<dyn Fn(&[u8]) -> Option<Value> + Send + Sync>>,
}

impl Config {
    pub fn new(api_key: impl Into<String>) -> Self {
        Self {
            api_key: api_key.into(),
            base_url: "https://api.openai.com".to_string(),
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
}

#[derive(Clone)]
pub struct Client {
    pub(crate) client: reqwest::Client,
    pub(crate) config: Config,
}

impl Client {
    pub fn new(config: Config) -> Result<Self, ClientError> {
        let client = reqwest::Client::builder()
            .timeout(Duration::from_secs(300))
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
            Duration::from_secs(60),
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
            return Err(self.parse_error(status, &bytes));
        }

        let mut stream = response.bytes_stream();
        let parse_error = self.config.error_parser.clone();

        Ok(try_stream! {
            let mut buffer = String::new();
            loop {
                let chunk = timeout(Duration::from_secs(30), stream.next())
                    .await
                    .map_err(|_| ClientError::Timeout("stream idle timeout".to_string()))?;
                let Some(chunk) = chunk else {
                    break;
                };
                let chunk = chunk.map_err(ClientError::Http)?;
                let text = String::from_utf8_lossy(&chunk);
                buffer.push_str(&text);

                while let Some(pos) = buffer.find('\n') {
                    let line = buffer[..pos].trim().to_string();
                    buffer.drain(..=pos);

                    if line.is_empty() {
                        continue;
                    }

                    if let Some(data) = line.strip_prefix("data: ") {
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

pub(crate) fn trimmed_base_url(base_url: &str) -> &str {
    base_url.trim_end_matches('/')
}
