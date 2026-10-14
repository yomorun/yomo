use std::sync::Arc;

use async_trait::async_trait;
use axum::body::Bytes;
use futures_util::StreamExt;
use serde_json::Value;

use crate::llm_provider::vertexai::client::VertexAIClient;
use crate::model_api_provider::provider::{
    ModelApiProvider, ProviderBody, ProviderRequest, ProviderResponse, filter_request_headers,
    filter_response_headers, parse_stream_flag, should_stream_response,
};
use crate::serve_config::{ConfigError, ProviderConfig};

/// `Tool` fields forwarded to Vertex AI, normalized by [`normalize_field_name`].
/// These are billed through tokens reported in `usageMetadata`. Everything else
/// is rejected: Google Search / Web Grounding for Enterprise / Google Maps
/// grounding are billed per query and "grounding with your data" (`retrieval`)
/// per prompt, none of which shows up in `usageMetadata`. Computer Use is also
/// rejected, as is any tool type Google adds later until it is reviewed here.
const ALLOWED_TOOL_FIELDS: [&str; 3] = ["functiondeclarations", "codeexecution", "urlcontext"];

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
impl<M> ModelApiProvider<M> for GenerateContentClient {
    fn model_id(&self) -> &str {
        &self.model_id
    }

    fn prepare_request_body(&self, body: Bytes) -> Result<Bytes, String> {
        sanitize_request_body(&body)
    }

    async fn execute(
        &self,
        req: ProviderRequest,
        _metadata: &M,
    ) -> Result<ProviderResponse, anyhow::Error> {
        let stream_requested = parse_stream_flag(&req.body);
        let headers = filter_request_headers(req.headers);
        let response = self
            .client
            .post_json_with_headers(
                &self.upstream_model,
                req.body.to_vec(),
                stream_requested,
                headers,
            )
            .await?;
        let status = response.status();
        let mut resp_headers = filter_response_headers(response.headers());
        let is_stream_response = should_stream_response(stream_requested, status);

        if is_stream_response {
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

    fn extract_request_id(&self, payload_json: &Value) -> Option<String> {
        extract_request_id_json(payload_json)
    }

    fn extract_usage(&self, payload_json: &Value) -> Option<Value> {
        extract_usage_json(payload_json)
    }

    fn inject_usage(&self, payload_json: &mut Value, usage: Value) -> bool {
        inject_usage_json(payload_json, usage)
    }
}

/// Rejects requests carrying tools outside [`ALLOWED_TOOL_FIELDS`] and returns
/// the re-serialized body, so upstream parses exactly the JSON checked here
/// (e.g. a duplicated `tools` key cannot slip past the check).
fn sanitize_request_body(body: &Bytes) -> Result<Bytes, String> {
    let value: Value =
        serde_json::from_slice(body).map_err(|err| format!("invalid json body: {err}"))?;
    match value.get("tools") {
        None | Some(Value::Null) => {}
        Some(Value::Array(tools)) => {
            for tool in tools {
                let Some(tool) = tool.as_object() else {
                    return Err("each entry in `tools` must be an object".to_string());
                };
                if let Some(field) = tool.keys().find(|field| {
                    !ALLOWED_TOOL_FIELDS.contains(&normalize_field_name(field).as_str())
                }) {
                    return Err(format!("tool `{field}` is not supported"));
                }
            }
        }
        Some(_) => return Err("`tools` must be an array".to_string()),
    }
    serde_json::to_vec(&value)
        .map(Bytes::from)
        .map_err(|err| format!("invalid json body: {err}"))
}

/// Vertex accepts both the camelCase and snake_case spelling of proto fields.
fn normalize_field_name(field: &str) -> String {
    field
        .chars()
        .filter(|ch| *ch != '_')
        .map(|ch| ch.to_ascii_lowercase())
        .collect()
}

fn extract_request_id_json(payload_json: &Value) -> Option<String> {
    payload_json
        .get("id")
        .and_then(Value::as_str)
        .map(str::to_string)
        .or_else(|| {
            payload_json
                .get("response")
                .and_then(|response| response.get("id"))
                .and_then(Value::as_str)
                .map(str::to_string)
        })
}

fn extract_usage_json(payload_json: &Value) -> Option<Value> {
    non_null_usage(payload_json.get("usageMetadata")).or_else(|| {
        non_null_usage(
            payload_json
                .get("response")
                .and_then(|response| response.get("usageMetadata")),
        )
    })
}

fn inject_usage_json(payload_json: &mut Value, usage: Value) -> bool {
    let Some(obj) = payload_json.as_object_mut() else {
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

fn non_null_usage(value: Option<&Value>) -> Option<Value> {
    value.filter(|usage| !usage.is_null()).cloned()
}

#[cfg(test)]
mod tests {
    use super::{
        extract_request_id_json, extract_usage_json, inject_usage_json, sanitize_request_body,
    };
    use axum::body::Bytes;
    use serde_json::{Value, json};

    fn sanitize(request: Value) -> Result<Value, String> {
        let body =
            Bytes::from(serde_json::to_vec(&request).expect("request json should serialize"));
        sanitize_request_body(&body)
            .map(|body| serde_json::from_slice(&body).expect("sanitized json should parse"))
    }

    /// Verifies function declarations, code execution and URL context are forwarded unchanged.
    #[test]
    fn sanitize_request_body_allows_token_billed_tools() {
        let request = json!({
            "contents": [{"role": "user", "parts": [{"text": "hi"}]}],
            "tools": [
                {"functionDeclarations": [{"name": "lookup", "parameters": {"type": "object"}}]},
                {"codeExecution": {}},
                {"url_context": {}}
            ]
        });

        assert_eq!(sanitize(request.clone()), Ok(request));
    }

    /// Verifies requests without tools pass through.
    #[test]
    fn sanitize_request_body_allows_missing_or_null_tools() {
        let request = json!({"contents": [{"role": "user", "parts": [{"text": "hi"}]}]});
        assert_eq!(sanitize(request.clone()), Ok(request));

        let request = json!({"contents": [], "tools": null});
        assert_eq!(sanitize(request.clone()), Ok(request));
    }

    /// Verifies grounding and computer use tools are rejected in either field spelling.
    #[test]
    fn sanitize_request_body_rejects_separately_billed_tools() {
        for field in [
            "googleSearch",
            "google_search",
            "googleSearchRetrieval",
            "enterpriseWebSearch",
            "googleMaps",
            "google_maps",
            "retrieval",
            "computerUse",
            "computer_use",
            "someFutureTool",
        ] {
            let request = json!({
                "contents": [],
                "tools": [
                    {"functionDeclarations": []},
                    {field: {}}
                ]
            });

            assert_eq!(
                sanitize(request),
                Err(format!("tool `{field}` is not supported")),
                "{field} must be rejected"
            );
        }
    }

    /// Verifies a blocked tool sharing an object with function declarations is rejected.
    #[test]
    fn sanitize_request_body_rejects_blocked_tool_in_mixed_entry() {
        let request = json!({
            "contents": [],
            "tools": [{"functionDeclarations": [], "googleSearch": {}}]
        });

        assert_eq!(
            sanitize(request),
            Err("tool `googleSearch` is not supported".to_string())
        );
    }

    /// Verifies malformed `tools` values are rejected instead of forwarded.
    #[test]
    fn sanitize_request_body_rejects_malformed_tools() {
        assert_eq!(
            sanitize(json!({"tools": {"googleSearch": {}}})),
            Err("`tools` must be an array".to_string())
        );
        assert_eq!(
            sanitize(json!({"tools": ["googleSearch"]})),
            Err("each entry in `tools` must be an object".to_string())
        );
        assert!(sanitize_request_body(&Bytes::from_static(b"not json")).is_err());
    }

    /// Verifies a duplicated `tools` key forwards only the value that was checked.
    #[test]
    fn sanitize_request_body_forwards_checked_duplicate_key_value() {
        let body = Bytes::from_static(br#"{"tools":[{"googleSearch":{}}],"tools":[]}"#);

        let sanitized = sanitize_request_body(&body).expect("last tools value is allowed");

        let parsed: Value =
            serde_json::from_slice(&sanitized).expect("sanitized json should parse");
        assert_eq!(parsed, json!({"tools": []}));
    }

    /// Verifies full payload request id extraction prefers top-level `id`.
    #[test]
    fn extract_request_id_json_prefers_top_level_id() {
        let payload = json!({"id": "gen_top", "response": {"id": "gen_nested"}});

        let request_id = extract_request_id_json(&payload);

        assert_eq!(request_id.as_deref(), Some("gen_top"));
    }

    /// Verifies request id extraction supports nested `response.id` fallback.
    #[test]
    fn extract_request_id_json_supports_nested_response_id() {
        let payload = json!({"response": {"id": "gen_nested"}});

        let request_id = extract_request_id_json(&payload);

        assert_eq!(request_id.as_deref(), Some("gen_nested"));
    }

    /// Verifies full payload usage extraction falls back to `response.usageMetadata`.
    #[test]
    fn extract_usage_json_reads_nested_usage_metadata() {
        let payload = json!({"response": {"usageMetadata": {"totalTokenCount": 42}}});

        let usage = extract_usage_json(&payload);

        assert_eq!(usage, Some(json!({"totalTokenCount": 42})));
    }

    /// Verifies usage extraction ignores null usage metadata payloads.
    #[test]
    fn extract_usage_json_ignores_null_usage_metadata() {
        let payload = json!({"usageMetadata": null});

        let usage = extract_usage_json(&payload);

        assert_eq!(usage, None);
    }

    /// Verifies usage injection updates nested `response.usageMetadata` when top-level field is absent.
    #[test]
    fn inject_usage_json_updates_nested_response_usage_metadata() {
        let mut payload = json!({"response": {"usageMetadata": {"totalTokenCount": 1}}});
        let new_usage = json!({"totalTokenCount": 99});

        let injected = inject_usage_json(&mut payload, new_usage.clone());

        assert!(injected);
        assert_eq!(payload["response"]["usageMetadata"], new_usage);
    }
}

pub fn build_client<M>(
    provider: &ProviderConfig,
) -> Result<Arc<dyn ModelApiProvider<M>>, ConfigError> {
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
