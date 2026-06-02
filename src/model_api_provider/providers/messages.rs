use std::sync::Arc;

use anyhow::anyhow;
use async_trait::async_trait;
use aws_config::BehaviorVersion;
use aws_credential_types::Credentials;
use aws_sdk_bedrockruntime::primitives::Blob;
use aws_types::region::Region;
use axum::body::Bytes;
use axum::http::{HeaderMap, StatusCode};
use serde_json::Value;
use tokio::sync::OnceCell;

use crate::model_api_provider::provider::{
    ModelApiProvider, ProviderBody, ProviderRequest, ProviderResponse, parse_stream_flag,
    rewrite_messages_body,
};
use crate::serve_config::{ConfigError, ProviderConfig};

#[derive(Clone)]
pub struct MessagesClient {
    model_id: String,
    bedrock_model: String,
    aws_region: String,
    anthropic_version: String,
    default_max_tokens: u64,
    aws_access_key_id: Option<String>,
    aws_secret_access_key: Option<String>,
    aws_session_token: Option<String>,
    bedrock_client: Arc<OnceCell<aws_sdk_bedrockruntime::Client>>,
}

impl MessagesClient {
    pub fn new(
        model_id: String,
        bedrock_model: String,
        aws_region: String,
        anthropic_version: String,
        default_max_tokens: u64,
        aws_access_key_id: Option<String>,
        aws_secret_access_key: Option<String>,
        aws_session_token: Option<String>,
    ) -> Self {
        Self {
            model_id,
            bedrock_model,
            aws_region,
            anthropic_version,
            default_max_tokens,
            aws_access_key_id,
            aws_secret_access_key,
            aws_session_token,
            bedrock_client: Arc::new(OnceCell::new()),
        }
    }

    async fn client(&self) -> Result<&aws_sdk_bedrockruntime::Client, anyhow::Error> {
        self.bedrock_client
            .get_or_try_init(|| async {
                let mut loader = aws_config::defaults(BehaviorVersion::latest())
                    .region(Region::new(self.aws_region.clone()));

                if let (Some(access_key_id), Some(secret_access_key)) = (
                    self.aws_access_key_id.as_ref(),
                    self.aws_secret_access_key.as_ref(),
                ) {
                    let credentials = Credentials::new(
                        access_key_id,
                        secret_access_key,
                        self.aws_session_token.clone(),
                        None,
                        "model-api-messages",
                    );
                    loader = loader.credentials_provider(credentials);
                }

                let config = loader.load().await;
                Ok::<aws_sdk_bedrockruntime::Client, anyhow::Error>(
                    aws_sdk_bedrockruntime::Client::new(&config),
                )
            })
            .await
    }
}

#[async_trait]
impl ModelApiProvider for MessagesClient {
    fn model_id(&self) -> &str {
        &self.model_id
    }

    async fn execute(&self, req: ProviderRequest) -> Result<ProviderResponse, anyhow::Error> {
        let stream = parse_stream_flag(&req.body);
        let body =
            rewrite_messages_body(&req.body, &self.anthropic_version, self.default_max_tokens)?;
        let client = self.client().await?;

        if stream {
            let response = client
                .invoke_model_with_response_stream()
                .model_id(&self.bedrock_model)
                .content_type("application/json")
                .body(Blob::new(body.to_vec()))
                .send()
                .await
                .map_err(|err| {
                    anyhow!("bedrock invoke_model_with_response_stream failed: {err:?}")
                })?;

            let mut stream = response.body;
            let mapped = async_stream::stream! {
                loop {
                    match stream.recv().await {
                        Ok(Some(event)) => {
                            if let Ok(chunk) = event.as_chunk() {
                                if let Some(payload) = chunk.bytes.as_ref() {
                                    let frame = format!(
                                        "data: {}\n\n",
                                        String::from_utf8_lossy(payload.as_ref())
                                    );
                                    yield Ok::<Bytes, std::io::Error>(Bytes::from(frame));
                                }
                            }
                        }
                        Ok(None) => {
                            break;
                        }
                        Err(err) => {
                            yield Err(std::io::Error::new(std::io::ErrorKind::Other, err.to_string()));
                            break;
                        }
                    }
                }
                yield Ok(Bytes::from_static(b"data: [DONE]\n\n"));
            };

            let mut headers = HeaderMap::new();
            headers.insert(
                axum::http::header::CONTENT_TYPE,
                "text/event-stream"
                    .parse()
                    .expect("static header value must be valid"),
            );
            headers.insert(
                axum::http::header::CACHE_CONTROL,
                "no-cache"
                    .parse()
                    .expect("static header value must be valid"),
            );
            headers.insert(
                axum::http::header::CONNECTION,
                "keep-alive"
                    .parse()
                    .expect("static header value must be valid"),
            );

            Ok(ProviderResponse {
                status: StatusCode::OK,
                headers,
                body: ProviderBody::Stream(Box::pin(mapped)),
            })
        } else {
            let response = client
                .invoke_model()
                .model_id(&self.bedrock_model)
                .content_type("application/json")
                .accept("application/json")
                .body(Blob::new(body.to_vec()))
                .send()
                .await
                .map_err(|err| anyhow!("bedrock invoke_model failed: {err:?}"))?;

            let mut headers = HeaderMap::new();
            headers.insert(
                axum::http::header::CONTENT_TYPE,
                "application/json"
                    .parse()
                    .expect("static header value must be valid"),
            );
            Ok(ProviderResponse {
                status: StatusCode::OK,
                headers,
                body: ProviderBody::Full(Bytes::from(response.body.into_inner())),
            })
        }
    }

    fn extract_request_id_from_full(&self, body_json: &Value) -> Option<String> {
        extract_request_id_from_full_json(body_json)
    }

    fn extract_request_id_from_stream_event(&self, event_json: &Value) -> Option<String> {
        extract_request_id_from_stream_event_json(event_json)
    }

    fn extract_usage_from_full(&self, body_json: &Value) -> Option<Value> {
        extract_usage_from_full_json(body_json)
    }

    fn extract_usage_from_stream_event(&self, event_json: &Value) -> Option<Value> {
        extract_usage_from_stream_event_json(event_json)
    }

    fn inject_usage_into_full(&self, body_json: &mut Value, usage: Value) -> bool {
        inject_usage_into_full_json(body_json, usage)
    }

    fn inject_usage_into_stream_event(&self, event_json: &mut Value, usage: Value) -> bool {
        self.inject_usage_into_full(event_json, usage)
    }
}

fn extract_request_id_from_full_json(body_json: &Value) -> Option<String> {
    body_json
        .get("id")
        .and_then(Value::as_str)
        .map(str::to_string)
        .or_else(|| {
            body_json
                .get("message")
                .and_then(|message| message.get("id"))
                .and_then(Value::as_str)
                .map(str::to_string)
        })
}

fn extract_request_id_from_stream_event_json(event_json: &Value) -> Option<String> {
    extract_request_id_from_full_json(event_json)
}

fn extract_usage_from_full_json(body_json: &Value) -> Option<Value> {
    non_null_usage(body_json.get("usage")).or_else(|| {
        non_null_usage(
            body_json
                .get("message")
                .and_then(|message| message.get("usage")),
        )
    })
}

fn extract_usage_from_stream_event_json(event_json: &Value) -> Option<Value> {
    extract_usage_from_full_json(event_json)
}

fn inject_usage_into_full_json(body_json: &mut Value, usage: Value) -> bool {
    let Some(obj) = body_json.as_object_mut() else {
        return false;
    };
    if obj.contains_key("usage") {
        obj.insert("usage".to_string(), usage);
        return true;
    }
    if let Some(message) = obj.get_mut("message").and_then(Value::as_object_mut) {
        message.insert("usage".to_string(), usage);
        return true;
    }
    false
}

fn non_null_usage(value: Option<&Value>) -> Option<Value> {
    value.filter(|usage| !usage.is_null()).cloned()
}

#[cfg(test)]
mod tests {
    use super::{
        extract_request_id_from_full_json, extract_request_id_from_stream_event_json,
        extract_usage_from_full_json, extract_usage_from_stream_event_json,
        inject_usage_into_full_json,
    };
    use serde_json::json;

    /// Verifies full payload request id extraction prefers top-level `id`.
    #[test]
    fn extract_request_id_from_full_json_prefers_top_level_id() {
        let payload = json!({"id": "msg_top", "message": {"id": "msg_nested"}});

        let request_id = extract_request_id_from_full_json(&payload);

        assert_eq!(request_id.as_deref(), Some("msg_top"));
    }

    /// Verifies stream-event request id extraction supports nested `message.id` fallback.
    #[test]
    fn extract_request_id_from_stream_event_json_supports_nested_message_id() {
        let payload = json!({"message": {"id": "msg_nested"}});

        let request_id = extract_request_id_from_stream_event_json(&payload);

        assert_eq!(request_id.as_deref(), Some("msg_nested"));
    }

    /// Verifies full payload usage extraction falls back to `message.usage`.
    #[test]
    fn extract_usage_from_full_json_reads_nested_message_usage() {
        let payload = json!({"message": {"usage": {"input_tokens": 3}}});

        let usage = extract_usage_from_full_json(&payload);

        assert_eq!(usage, Some(json!({"input_tokens": 3})));
    }

    /// Verifies stream-event usage extraction ignores null usage payloads.
    #[test]
    fn extract_usage_from_stream_event_json_ignores_null_usage() {
        let payload = json!({"usage": null});

        let usage = extract_usage_from_stream_event_json(&payload);

        assert_eq!(usage, None);
    }

    /// Verifies usage injection writes to nested `message.usage` when top-level field is absent.
    #[test]
    fn inject_usage_into_full_json_updates_nested_message_usage() {
        let mut payload = json!({"message": {"usage": {"input_tokens": 1}}});
        let new_usage = json!({"input_tokens": 55});

        let injected = inject_usage_into_full_json(&mut payload, new_usage.clone());

        assert!(injected);
        assert_eq!(payload["message"]["usage"], new_usage);
    }
}

pub fn build_client(provider: &ProviderConfig) -> Result<Arc<dyn ModelApiProvider>, ConfigError> {
    if provider.provider_type != "bedrock-messages" {
        return Err(ConfigError::UnknownProviderType(
            provider.provider_type.clone(),
        ));
    }
    let bedrock_model = provider
        .params
        .get("model")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("model is required".to_string()))?;
    let aws_region = provider
        .params
        .get("aws_region")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("aws_region is required".to_string()))?;
    let anthropic_version = provider
        .params
        .get("anthropic_version")
        .cloned()
        .unwrap_or_else(|| "bedrock-2023-05-31".to_string());
    let default_max_tokens = provider
        .params
        .get("max_tokens")
        .and_then(|raw| raw.parse::<u64>().ok())
        .unwrap_or(4096);
    let aws_access_key_id = provider.params.get("aws_access_key_id").cloned();
    let aws_secret_access_key = provider.params.get("aws_secret_access_key").cloned();
    let aws_session_token = provider.params.get("aws_session_token").cloned();

    Ok(Arc::new(MessagesClient::new(
        provider.model_id.clone(),
        bedrock_model,
        aws_region,
        anthropic_version,
        default_max_tokens,
        aws_access_key_id,
        aws_secret_access_key,
        aws_session_token,
    )))
}
