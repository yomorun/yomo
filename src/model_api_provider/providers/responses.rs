use async_trait::async_trait;
use axum::http::HeaderMap;
use serde_json::Value;
use std::sync::Arc;

use crate::model_api_provider::provider::{
    ModelApiProvider, ProviderRequest, ProviderResponse, proxy_request,
};
use crate::serve_config::{ConfigError, ProviderConfig};

#[derive(Clone)]
pub struct ResponsesClient {
    client: reqwest::Client,
    base_url: String,
    auth_headers: HeaderMap,
    model_id: String,
    upstream_model: String,
}

impl ResponsesClient {
    pub fn new(
        client: reqwest::Client,
        base_url: String,
        auth_headers: HeaderMap,
        model_id: String,
        upstream_model: String,
    ) -> Self {
        Self {
            client,
            base_url,
            auth_headers,
            model_id,
            upstream_model,
        }
    }
}

#[async_trait]
impl ModelApiProvider for ResponsesClient {
    fn model_id(&self) -> &str {
        &self.model_id
    }

    async fn execute(&self, mut req: ProviderRequest) -> Result<ProviderResponse, anyhow::Error> {
        req.endpoint_path = "/responses".to_string();
        proxy_request(
            &self.client,
            &self.base_url,
            self.auth_headers.clone(),
            Some(self.upstream_model.as_str()),
            req,
        )
        .await
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
                .get("response")
                .and_then(|response| response.get("id"))
                .and_then(Value::as_str)
                .map(str::to_string)
        })
}

fn extract_request_id_from_stream_event_json(event_json: &Value) -> Option<String> {
    extract_request_id_from_full_json(event_json)
}

fn extract_usage_from_full_json(body_json: &Value) -> Option<Value> {
    body_json.get("usage").cloned().or_else(|| {
        body_json
            .get("response")
            .and_then(|response| response.get("usage"))
            .cloned()
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
    if let Some(response) = obj.get_mut("response").and_then(Value::as_object_mut) {
        response.insert("usage".to_string(), usage);
        return true;
    }
    false
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
        let payload = json!({"id": "resp_top", "response": {"id": "resp_nested"}});

        let request_id = extract_request_id_from_full_json(&payload);

        assert_eq!(request_id.as_deref(), Some("resp_top"));
    }

    /// Verifies stream-event request id extraction supports nested `response.id` fallback.
    #[test]
    fn extract_request_id_from_stream_event_json_supports_nested_response_id() {
        let payload = json!({"response": {"id": "resp_nested"}});

        let request_id = extract_request_id_from_stream_event_json(&payload);

        assert_eq!(request_id.as_deref(), Some("resp_nested"));
    }

    /// Verifies full payload usage extraction falls back to `response.usage`.
    #[test]
    fn extract_usage_from_full_json_reads_nested_usage() {
        let payload = json!({"response": {"usage": {"total_tokens": 42}}});

        let usage = extract_usage_from_full_json(&payload);

        assert_eq!(usage, Some(json!({"total_tokens": 42})));
    }

    /// Verifies stream-event usage extraction supports top-level usage payloads.
    #[test]
    fn extract_usage_from_stream_event_json_reads_top_level_usage() {
        let payload = json!({"usage": {"prompt_tokens": 8}});

        let usage = extract_usage_from_stream_event_json(&payload);

        assert_eq!(usage, Some(json!({"prompt_tokens": 8})));
    }

    /// Verifies usage injection writes to the top-level usage field when present.
    #[test]
    fn inject_usage_into_full_json_updates_top_level_usage() {
        let mut payload = json!({"usage": {"total_tokens": 1}});
        let new_usage = json!({"total_tokens": 99});

        let injected = inject_usage_into_full_json(&mut payload, new_usage.clone());

        assert!(injected);
        assert_eq!(payload.get("usage"), Some(&new_usage));
    }
}

pub fn build_client(provider: &ProviderConfig) -> Result<Arc<dyn ModelApiProvider>, ConfigError> {
    if provider.provider_type != "responses" {
        return Err(ConfigError::UnknownProviderType(
            provider.provider_type.clone(),
        ));
    }
    let api_key = provider
        .params
        .get("api_key")
        .ok_or_else(|| ConfigError::InvalidProvider("api_key is required".to_string()))?;
    let base_url = provider
        .params
        .get("base_url")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("base_url is required".to_string()))?;
    let upstream_model = provider
        .params
        .get("model")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("model is required".to_string()))?;

    let mut headers = HeaderMap::new();
    let auth_value = format!("Bearer {}", api_key);
    headers.insert(
        axum::http::header::AUTHORIZATION,
        auth_value
            .parse::<axum::http::HeaderValue>()
            .map_err(|err| ConfigError::InvalidProvider(err.to_string()))?,
    );

    Ok(Arc::new(ResponsesClient::new(
        reqwest::Client::new(),
        base_url,
        headers,
        provider.model_id.clone(),
        upstream_model,
    )))
}
