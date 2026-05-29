use std::sync::Arc;

use async_trait::async_trait;
use futures_util::StreamExt;
use serde_json::Value;

use crate::llm_provider::vertexai::client::VertexAIClient;
use crate::model_api_provider::provider::{
    ModelApiProvider, ProviderBody, ProviderRequest, ProviderResponse, filter_request_headers,
    filter_response_headers, parse_stream_flag,
};
use crate::serve_config::{ConfigError, ProviderConfig};

#[derive(Clone)]
pub struct GenerateContentClient {
    model_id: String,
    upstream_model: String,
    client: VertexAIClient,
}

impl GenerateContentClient {
    pub fn new(model_id: String, upstream_model: String, client: VertexAIClient) -> Self {
        Self {
            model_id,
            upstream_model,
            client,
        }
    }
}

#[async_trait]
impl ModelApiProvider for GenerateContentClient {
    fn model_id(&self) -> &str {
        &self.model_id
    }

    async fn execute(&self, req: ProviderRequest) -> Result<ProviderResponse, anyhow::Error> {
        let stream = parse_stream_flag(&req.body);
        let headers = filter_request_headers(req.headers);
        let response = self
            .client
            .post_json_with_headers(&self.upstream_model, req.body.to_vec(), stream, headers)
            .await?;
        let status = response.status();
        let mut resp_headers = filter_response_headers(response.headers());

        if stream {
            resp_headers.remove(axum::http::header::CONTENT_LENGTH);
            let body_stream = response.bytes_stream().map(|chunk| match chunk {
                Ok(bytes) => Ok(bytes),
                Err(err) => Err(std::io::Error::new(std::io::ErrorKind::Other, err)),
            });

            Ok(ProviderResponse {
                status,
                headers: resp_headers,
                body: ProviderBody::Stream(Box::pin(body_stream)),
            })
        } else {
            let bytes = response.bytes().await?;
            Ok(ProviderResponse {
                status,
                headers: resp_headers,
                body: ProviderBody::Full(bytes),
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
    body_json.get("usageMetadata").cloned().or_else(|| {
        body_json
            .get("response")
            .and_then(|response| response.get("usageMetadata"))
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
    if obj.contains_key("usageMetadata") {
        obj.insert("usageMetadata".to_string(), usage);
        return true;
    }
    if let Some(response) = obj.get_mut("response").and_then(Value::as_object_mut) {
        response.insert("usageMetadata".to_string(), usage);
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
        let payload = json!({"id": "gen_top", "response": {"id": "gen_nested"}});

        let request_id = extract_request_id_from_full_json(&payload);

        assert_eq!(request_id.as_deref(), Some("gen_top"));
    }

    /// Verifies stream-event request id extraction supports nested `response.id` fallback.
    #[test]
    fn extract_request_id_from_stream_event_json_supports_nested_response_id() {
        let payload = json!({"response": {"id": "gen_nested"}});

        let request_id = extract_request_id_from_stream_event_json(&payload);

        assert_eq!(request_id.as_deref(), Some("gen_nested"));
    }

    /// Verifies full payload usage extraction falls back to `response.usageMetadata`.
    #[test]
    fn extract_usage_from_full_json_reads_nested_usage_metadata() {
        let payload = json!({"response": {"usageMetadata": {"totalTokenCount": 42}}});

        let usage = extract_usage_from_full_json(&payload);

        assert_eq!(usage, Some(json!({"totalTokenCount": 42})));
    }

    /// Verifies stream-event usage extraction supports top-level `usageMetadata` payloads.
    #[test]
    fn extract_usage_from_stream_event_json_reads_top_level_usage_metadata() {
        let payload = json!({"usageMetadata": {"promptTokenCount": 8}});

        let usage = extract_usage_from_stream_event_json(&payload);

        assert_eq!(usage, Some(json!({"promptTokenCount": 8})));
    }

    /// Verifies usage injection updates nested `response.usageMetadata` when top-level field is absent.
    #[test]
    fn inject_usage_into_full_json_updates_nested_response_usage_metadata() {
        let mut payload = json!({"response": {"usageMetadata": {"totalTokenCount": 1}}});
        let new_usage = json!({"totalTokenCount": 99});

        let injected = inject_usage_into_full_json(&mut payload, new_usage.clone());

        assert!(injected);
        assert_eq!(payload["response"]["usageMetadata"], new_usage);
    }
}

pub fn build_client(provider: &ProviderConfig) -> Result<Arc<dyn ModelApiProvider>, ConfigError> {
    if provider.provider_type != "generate_content" {
        return Err(ConfigError::UnknownProviderType(
            provider.provider_type.clone(),
        ));
    }
    let project_id = provider
        .params
        .get("project_id")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("project_id is required".to_string()))?;
    let location = provider
        .params
        .get("location")
        .cloned()
        .unwrap_or_else(|| "global".to_string());
    let credentials_file = provider
        .params
        .get("credentials_file")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("credentials_file is required".to_string()))?;
    let upstream_model = provider
        .params
        .get("model")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("model is required".to_string()))?;

    let client = VertexAIClient::new(project_id, location, credentials_file)
        .map_err(|err| ConfigError::InvalidProvider(err.to_string()))?;
    Ok(Arc::new(GenerateContentClient::new(
        provider.model_id.clone(),
        upstream_model,
        client,
    )))
}
