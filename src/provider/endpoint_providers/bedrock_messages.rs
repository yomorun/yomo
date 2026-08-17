use std::sync::Arc;

use async_trait::async_trait;
use aws_sdk_bedrockruntime::config::{BehaviorVersion, Token};
use aws_sdk_bedrockruntime::primitives::Blob;
use aws_types::region::Region;
use axum::body::Bytes;
use axum::http::{HeaderMap, StatusCode, header};
use serde::Deserialize;
use serde_json::Value;

use crate::provider::{
    BedrockBadRequestPayload, HttpProviderRequest, HttpProviderResponse, Provider, ProviderBody,
    ProviderError, extract_bedrock_bad_request, extract_messages_request_id_json,
    extract_messages_usage_json, inject_messages_usage_json, parse_stream_flag,
    rewrite_messages_body,
};
use crate::serve_config::{ConfigError, ProviderConfig};

#[derive(Clone)]
pub struct BedrockMessagesClient {
    bedrock_model: String,
    anthropic_version: String,
    default_max_tokens: i32,
    bedrock_client: aws_sdk_bedrockruntime::Client,
}

impl BedrockMessagesClient {
    pub fn new(
        bedrock_model: String,
        anthropic_version: String,
        default_max_tokens: i32,
        bedrock_client: aws_sdk_bedrockruntime::Client,
    ) -> Self {
        Self {
            bedrock_model,
            anthropic_version,
            default_max_tokens,
            bedrock_client,
        }
    }

    fn client(&self) -> &aws_sdk_bedrockruntime::Client {
        &self.bedrock_client
    }
}

#[async_trait]
impl<M: Sync> Provider<M> for BedrockMessagesClient {
    async fn http(
        &self,
        req: HttpProviderRequest,
        _metadata: &M,
    ) -> Result<HttpProviderResponse, ProviderError> {
        let stream = parse_stream_flag(&req.body);
        let body =
            rewrite_messages_body(&req.body, &self.anthropic_version, self.default_max_tokens)?;
        let client = self.client();

        if stream {
            let response = client
                .invoke_model_with_response_stream()
                .model_id(&self.bedrock_model)
                .content_type("application/json")
                .body(Blob::new(body.to_vec()))
                .send()
                .await;
            let response = match response {
                Ok(response) => response,
                Err(err) => {
                    if let Some(response) = map_bedrock_bad_request_response(&err) {
                        return Ok(response);
                    }
                    return Err(ProviderError::internal(format!(
                        "bedrock invoke_model_with_response_stream failed: {err:?}"
                    )));
                }
            };

            let mut stream = response.body;
            let mapped = async_stream::stream! {
                loop {
                    match stream.recv().await {
                        Ok(Some(event)) => {
                            if let Ok(chunk) = event.as_chunk() {
                                if let Some(payload) = chunk.bytes.as_ref() {
                                    let frame = build_sse_frame(payload.as_ref());
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

            Ok(HttpProviderResponse {
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
                .await;
            let response = match response {
                Ok(response) => response,
                Err(err) => {
                    if let Some(response) = map_bedrock_bad_request_response(&err) {
                        return Ok(response);
                    }
                    return Err(ProviderError::internal(format!(
                        "bedrock invoke_model failed: {err:?}"
                    )));
                }
            };

            let mut headers = HeaderMap::new();
            headers.insert(
                axum::http::header::CONTENT_TYPE,
                "application/json"
                    .parse()
                    .expect("static header value must be valid"),
            );
            Ok(HttpProviderResponse {
                status: StatusCode::OK,
                headers,
                body: ProviderBody::Full(Bytes::from(response.body.into_inner())),
            })
        }
    }

    fn extract_request_id(&self, payload_json: &Value) -> Option<String> {
        extract_messages_request_id_json(payload_json)
    }

    fn extract_usage(&self, payload_json: &Value) -> Option<Value> {
        extract_messages_usage_json(payload_json)
    }

    fn inject_usage(&self, payload_json: &mut Value, usage: Value) -> bool {
        inject_messages_usage_json(payload_json, usage)
    }
}

#[derive(Deserialize)]
struct BedrockEventEnvelope {
    #[serde(default)]
    r#type: String,
}

fn anthropic_sse_event_name(payload: &[u8]) -> Option<String> {
    let envelope: BedrockEventEnvelope = serde_json::from_slice(payload).ok()?;
    let event_name = envelope.r#type.trim();
    if event_name.is_empty() {
        return None;
    }
    Some(event_name.to_string())
}

fn build_sse_frame(payload: &[u8]) -> String {
    let payload_text = String::from_utf8_lossy(payload);
    if let Some(event_name) = anthropic_sse_event_name(payload) {
        return format!("event: {event_name}\ndata: {payload_text}\n\n");
    }
    format!("data: {payload_text}\n\n")
}

fn map_bedrock_bad_request_response<E>(
    err: &aws_sdk_bedrockruntime::error::SdkError<E>,
) -> Option<HttpProviderResponse>
where
    E: std::error::Error + Send + Sync + 'static,
{
    let mapped = extract_bedrock_bad_request(err)?;
    Some(bedrock_bad_request_response(mapped))
}

fn bedrock_bad_request_response(mapped: BedrockBadRequestPayload) -> HttpProviderResponse {
    let mut headers = HeaderMap::new();
    let content_type = mapped.content_type.as_deref().unwrap_or("application/json");
    if let Ok(content_type_value) = content_type.parse() {
        headers.insert(header::CONTENT_TYPE, content_type_value);
    }

    HttpProviderResponse {
        status: mapped.status,
        headers,
        body: ProviderBody::Full(mapped.payload),
    }
}

#[cfg(test)]
mod tests {
    use super::{
        anthropic_sse_event_name, bedrock_bad_request_response, build_client, build_sse_frame,
    };
    use crate::provider::{
        BedrockBadRequestPayload, extract_bedrock_bad_request_parts,
        extract_messages_request_id_json, extract_messages_usage_json, inject_messages_usage_json,
    };
    use crate::serve_config::ProviderConfig;
    use axum::http::{StatusCode, header};
    use serde_json::json;
    use std::collections::HashMap;

    /// Verifies full payload request id extraction prefers top-level `id`.
    #[test]
    fn extract_request_id_json_prefers_top_level_id() {
        let payload = json!({"id": "msg_top", "message": {"id": "msg_nested"}});

        let request_id = extract_messages_request_id_json(&payload);

        assert_eq!(request_id.as_deref(), Some("msg_top"));
    }

    /// Verifies request id extraction supports nested `message.id` fallback.
    #[test]
    fn extract_request_id_json_supports_nested_message_id() {
        let payload = json!({"message": {"id": "msg_nested"}});

        let request_id = extract_messages_request_id_json(&payload);

        assert_eq!(request_id.as_deref(), Some("msg_nested"));
    }

    /// Verifies full payload usage extraction falls back to `message.usage`.
    #[test]
    fn extract_usage_json_reads_nested_message_usage() {
        let payload = json!({"message": {"usage": {"input_tokens": 3}}});

        let usage = extract_messages_usage_json(&payload);

        assert_eq!(usage, Some(json!({"input_tokens": 3})));
    }

    /// Verifies usage extraction ignores null usage payloads.
    #[test]
    fn extract_usage_json_ignores_null_usage() {
        let payload = json!({"usage": null});

        let usage = extract_messages_usage_json(&payload);

        assert_eq!(usage, None);
    }

    /// Verifies usage injection writes to nested `message.usage` when top-level field is absent.
    #[test]
    fn inject_usage_json_updates_nested_message_usage() {
        let mut payload = json!({"message": {"usage": {"input_tokens": 1}}});
        let new_usage = json!({"input_tokens": 55});

        let injected = inject_messages_usage_json(&mut payload, new_usage.clone());

        assert!(injected);
        assert_eq!(payload["message"]["usage"], new_usage);
    }

    /// Verifies bedrock client creation accepts explicit bearer token config.
    #[test]
    fn build_client_accepts_aws_bearer_token_param() {
        let mut params = HashMap::new();
        params.insert(
            "model".to_string(),
            "global.anthropic.claude-sonnet-4-6".to_string(),
        );
        params.insert("aws_region".to_string(), "ap-northeast-1".to_string());
        params.insert("aws_bearer_token".to_string(), "test-token".to_string());

        let provider = ProviderConfig {
            provider_type: "bedrock-messages".to_string(),
            model_id: "claude-sonnet-4-6".to_string(),
            label: None,
            params,
        };

        assert!(build_client::<()>(&provider).is_ok());
    }

    /// Verifies bedrock client creation rejects missing bearer token config.
    #[test]
    fn build_client_rejects_missing_aws_bearer_token_param() {
        let mut params = HashMap::new();
        params.insert(
            "model".to_string(),
            "global.anthropic.claude-sonnet-4-6".to_string(),
        );
        params.insert("aws_region".to_string(), "ap-northeast-1".to_string());

        let provider = ProviderConfig {
            provider_type: "bedrock-messages".to_string(),
            model_id: "claude-sonnet-4-6".to_string(),
            label: None,
            params,
        };

        let err = build_client::<()>(&provider)
            .err()
            .expect("missing token must be rejected");

        assert_eq!(
            err.to_string(),
            "invalid provider: aws_bearer_token is required"
        );
    }

    /// Verifies Bedrock client errors with HTTP 400 are passed through to frontend status/body.
    #[test]
    fn passthrough_bad_request_maps_status_and_body() {
        let mapped = extract_bedrock_bad_request_parts(
            StatusCode::BAD_REQUEST,
            Some("application/json"),
            Some(br#"{"message":"bad input"}"#),
        )
        .expect("400 should be mapped");
        let response = bedrock_bad_request_response(mapped);

        assert_eq!(response.status, StatusCode::BAD_REQUEST);
        let content_type = response
            .headers
            .get(header::CONTENT_TYPE)
            .and_then(|value| value.to_str().ok());
        assert_eq!(content_type, Some("application/json"));
        match response.body {
            crate::provider::ProviderBody::Full(payload) => {
                assert_eq!(payload.as_ref(), b"{\"message\":\"bad input\"}");
            }
            crate::provider::ProviderBody::Stream(_) => {
                panic!("expected full payload response")
            }
        }
    }

    /// Verifies non-400 Bedrock client errors are not treated as passthrough responses.
    #[test]
    fn passthrough_bad_request_ignores_non_400() {
        let response = extract_bedrock_bad_request_parts(
            StatusCode::TOO_MANY_REQUESTS,
            Some("application/json"),
            Some(br#"{"message":"throttled"}"#),
        );

        assert!(response.is_none());
    }

    #[test]
    fn bedrock_bad_request_response_defaults_content_type() {
        let mapped = BedrockBadRequestPayload {
            status: StatusCode::BAD_REQUEST,
            content_type: None,
            payload: axum::body::Bytes::from_static(br#"{"message":"bad input"}"#),
        };

        let response = bedrock_bad_request_response(mapped);
        let content_type = response
            .headers
            .get(header::CONTENT_TYPE)
            .and_then(|value| value.to_str().ok());

        assert_eq!(content_type, Some("application/json"));
    }

    #[test]
    fn anthropic_sse_event_name_extracts_type() {
        let payload = br#"{"type":"content_block_delta","index":0}"#;

        let event_name = anthropic_sse_event_name(payload);

        assert_eq!(event_name.as_deref(), Some("content_block_delta"));
    }

    #[test]
    fn anthropic_sse_event_name_returns_none_for_invalid_payload() {
        let event_name = anthropic_sse_event_name(b"not-json");

        assert_eq!(event_name, None);
    }

    #[test]
    fn build_sse_frame_includes_event_when_type_present() {
        let payload = br#"{"type":"message_start","message":{"id":"m1"}}"#;

        let frame = build_sse_frame(payload);

        assert!(frame.starts_with("event: message_start\ndata: {\"type\":\"message_start\","));
        assert!(frame.ends_with("\n\n"));
    }

    #[test]
    fn build_sse_frame_falls_back_to_data_only_when_type_missing() {
        let payload = br#"{"message":{"id":"m1"}}"#;

        let frame = build_sse_frame(payload);

        assert_eq!(frame, "data: {\"message\":{\"id\":\"m1\"}}\n\n");
    }
}

pub fn build_client<M: Sync>(
    provider: &ProviderConfig,
) -> Result<Arc<dyn Provider<M>>, ConfigError> {
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
        .and_then(|raw| raw.parse::<i32>().ok())
        .unwrap_or(4096);
    let aws_bearer_token = provider
        .params
        .get("aws_bearer_token")
        .map(|token| token.trim().to_string())
        .filter(|token| !token.is_empty())
        .ok_or_else(|| ConfigError::InvalidProvider("aws_bearer_token is required".to_string()))?;

    let bedrock_client = aws_sdk_bedrockruntime::Config::builder()
        .behavior_version(BehaviorVersion::latest())
        .region(Region::new(aws_region))
        .bearer_token(Token::new(&aws_bearer_token, None))
        .build();
    let bedrock_client = aws_sdk_bedrockruntime::Client::from_conf(bedrock_client);

    Ok(Arc::new(BedrockMessagesClient::new(
        bedrock_model,
        anthropic_version,
        default_max_tokens,
        bedrock_client,
    )))
}
