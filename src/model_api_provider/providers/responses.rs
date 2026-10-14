use async_trait::async_trait;
use axum::body::Bytes;
use axum::http::{HeaderMap, HeaderValue, header};
use serde_json::Value;
use std::sync::Arc;

use crate::model_api_provider::provider::{
    ModelApiProvider, ProviderRequest, ProviderResponse, proxy_request,
};
use crate::serve_config::{ConfigError, ProviderConfig};

/// Hosted tool types OpenAI bills per call or per container on top of tokens
/// (web search, file search, code interpreter, image generation). Those charges
/// are absent from `usage`, so they cannot be attributed to a caller. Matched by
/// prefix to cover dated variants such as `web_search_preview_2025_03_11`.
const BILLED_TOOL_TYPE_PREFIXES: [&str; 4] = [
    "web_search",
    "file_search",
    "code_interpreter",
    "image_generation",
];

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
impl<M> ModelApiProvider<M> for ResponsesClient {
    fn model_id(&self) -> &str {
        &self.model_id
    }

    fn prepare_request_body(&self, body: Bytes) -> Result<Bytes, String> {
        strip_billed_tools(&body)
    }

    async fn execute(
        &self,
        mut req: ProviderRequest,
        _metadata: &M,
    ) -> Result<ProviderResponse, anyhow::Error> {
        req.endpoint_path = "/responses".to_string();
        if req
            .content_type
            .as_deref()
            .is_some_and(|content_type| content_type.starts_with("application/json"))
        {
            if let Some(sanitized_body) = sanitize_responses_request_body(&req.body) {
                req.body = sanitized_body;
            }
        }
        proxy_request(
            &self.client,
            &self.base_url,
            self.auth_headers.clone(),
            Some(self.upstream_model.as_str()),
            req,
        )
        .await
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
    non_null_usage(payload_json.get("usage")).or_else(|| {
        non_null_usage(
            payload_json
                .get("response")
                .and_then(|response| response.get("usage")),
        )
    })
}

fn inject_usage_json(payload_json: &mut Value, usage: Value) -> bool {
    let Some(obj) = payload_json.as_object_mut() else {
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

fn non_null_usage(value: Option<&Value>) -> Option<Value> {
    value.filter(|usage| !usage.is_null()).cloned()
}

/// Drops separately billed hosted tools and returns the re-serialized body, so
/// upstream parses exactly the JSON checked here (e.g. a duplicated `tools` key
/// cannot slip past the check). Tools are dropped rather than rejected because
/// agent clients send them by default (Codex declares `web_search` unless it is
/// disabled), and rejecting would fail every request from them.
fn strip_billed_tools(body: &Bytes) -> Result<Bytes, String> {
    let mut payload_json: Value =
        serde_json::from_slice(body).map_err(|err| format!("invalid json body: {err}"))?;
    match payload_json.get_mut("tools") {
        None | Some(Value::Null) => {}
        Some(Value::Array(tools)) => {
            tools.retain(|tool| !is_billed_tool(tool));
        }
        Some(_) => return Err("`tools` must be an array".to_string()),
    }
    serde_json::to_vec(&payload_json)
        .map(Bytes::from)
        .map_err(|err| format!("invalid json body: {err}"))
}

fn is_billed_tool(tool: &Value) -> bool {
    let Some(tool_type) = tool.get("type").and_then(Value::as_str) else {
        return false;
    };
    if tool_type == "shell" {
        // Hosted shell runs in a billed OpenAI container; only shell executed by
        // the client (no environment, or `environment.type: local`) is kept.
        return match tool.get("environment") {
            None | Some(Value::Null) => false,
            Some(environment) => environment.get("type").and_then(Value::as_str) != Some("local"),
        };
    }
    BILLED_TOOL_TYPE_PREFIXES
        .iter()
        .any(|prefix| tool_type.starts_with(prefix))
}

fn sanitize_responses_request_body(body: &Bytes) -> Option<Bytes> {
    let mut payload_json: Value = serde_json::from_slice(body).ok()?;
    let payload_obj = payload_json.as_object_mut()?;
    let allow_summary_passthrough = payload_obj
        .get("model")
        .and_then(Value::as_str)
        .is_some_and(|model| model.starts_with("gpt-"));

    let mut changed = false;

    if !allow_summary_passthrough {
        if payload_obj.remove("summary").is_some() {
            changed = true;
        }
        if let Some(reasoning_obj) = payload_obj
            .get_mut("reasoning")
            .and_then(Value::as_object_mut)
        {
            if reasoning_obj.remove("summary").is_some() {
                changed = true;
            }
        }
    }

    let Some(input_items) = payload_obj.get_mut("input").and_then(Value::as_array_mut) else {
        if !changed {
            return None;
        }
        return serde_json::to_vec(&payload_json).ok().map(Bytes::from);
    };

    for item in input_items {
        let Some(item_obj) = item.as_object_mut() else {
            continue;
        };

        let missing_or_empty_type = match item_obj.get("type") {
            None => true,
            Some(Value::String(value)) => value.trim().is_empty(),
            Some(Value::Null) => true,
            _ => false,
        };
        if !missing_or_empty_type {
            continue;
        }

        let has_role = item_obj.get("role").and_then(Value::as_str).is_some();
        let has_content = item_obj.contains_key("content");
        if has_role && has_content {
            item_obj.insert("type".to_string(), Value::String("message".to_string()));
            changed = true;
        }
    }

    if !changed {
        return None;
    }

    serde_json::to_vec(&payload_json).ok().map(Bytes::from)
}

#[cfg(test)]
mod tests {
    use super::{
        extract_request_id_json, extract_usage_json, inject_usage_json,
        sanitize_responses_request_body, strip_billed_tools,
    };
    use axum::body::Bytes;
    use serde_json::{Value, json};

    fn strip(request: Value) -> Result<Value, String> {
        let body =
            Bytes::from(serde_json::to_vec(&request).expect("request json should serialize"));
        strip_billed_tools(&body)
            .map(|body| serde_json::from_slice(&body).expect("stripped json should parse"))
    }

    /// Verifies per-call and per-container billed hosted tools are dropped while client tools stay.
    #[test]
    fn strip_billed_tools_drops_hosted_billed_tools() {
        let request = json!({
            "model": "gpt-5",
            "input": "hi",
            "tools": [
                {"type": "function", "name": "lookup", "parameters": {"type": "object"}},
                {"type": "web_search", "external_web_access": false},
                {"type": "web_search_preview_2025_03_11"},
                {"type": "file_search", "vector_store_ids": ["vs_1"]},
                {"type": "code_interpreter", "container": {"type": "auto"}},
                {"type": "image_generation"},
                {"type": "shell", "environment": {"type": "container_auto"}},
                {"type": "shell", "environment": {"type": "container_reference", "container_id": "cntr_1"}},
                {"type": "custom", "name": "apply_patch"},
                {"type": "local_shell"},
                {"type": "shell", "environment": {"type": "local"}},
                {"type": "shell"},
                {"type": "mcp", "server_label": "docs", "server_url": "https://example.com/mcp"}
            ]
        });

        let stripped = strip(request).expect("request should be accepted");

        assert_eq!(
            stripped["tools"],
            json!([
                {"type": "function", "name": "lookup", "parameters": {"type": "object"}},
                {"type": "custom", "name": "apply_patch"},
                {"type": "local_shell"},
                {"type": "shell", "environment": {"type": "local"}},
                {"type": "shell"},
                {"type": "mcp", "server_label": "docs", "server_url": "https://example.com/mcp"}
            ])
        );
        assert_eq!(stripped["model"], "gpt-5");
    }

    /// Verifies requests without tools pass through unchanged.
    #[test]
    fn strip_billed_tools_keeps_requests_without_tools() {
        let request = json!({"model": "gpt-5", "input": "hi"});
        assert_eq!(strip(request.clone()), Ok(request));

        let request = json!({"model": "gpt-5", "input": "hi", "tools": null});
        assert_eq!(strip(request.clone()), Ok(request));
    }

    /// Verifies malformed bodies are rejected instead of forwarded unchecked.
    #[test]
    fn strip_billed_tools_rejects_malformed_tools() {
        assert_eq!(
            strip(json!({"tools": {"type": "web_search"}})),
            Err("`tools` must be an array".to_string())
        );
        assert!(strip_billed_tools(&Bytes::from_static(b"not json")).is_err());
    }

    /// Verifies a duplicated `tools` key forwards only the value that was checked.
    #[test]
    fn strip_billed_tools_forwards_checked_duplicate_key_value() {
        let body = Bytes::from_static(br#"{"tools":[],"tools":[{"type":"web_search"}]}"#);

        let stripped = strip_billed_tools(&body).expect("request should be accepted");

        let parsed: Value = serde_json::from_slice(&stripped).expect("stripped json should parse");
        assert_eq!(parsed, json!({"tools": []}));
    }

    /// Verifies full payload request id extraction prefers top-level `id`.
    #[test]
    fn extract_request_id_json_prefers_top_level_id() {
        let payload = json!({"id": "resp_top", "response": {"id": "resp_nested"}});

        let request_id = extract_request_id_json(&payload);

        assert_eq!(request_id.as_deref(), Some("resp_top"));
    }

    /// Verifies request id extraction supports nested `response.id` fallback.
    #[test]
    fn extract_request_id_json_supports_nested_response_id() {
        let payload = json!({"response": {"id": "resp_nested"}});

        let request_id = extract_request_id_json(&payload);

        assert_eq!(request_id.as_deref(), Some("resp_nested"));
    }

    /// Verifies full payload usage extraction falls back to `response.usage`.
    #[test]
    fn extract_usage_json_reads_nested_usage() {
        let payload = json!({"response": {"usage": {"total_tokens": 42}}});

        let usage = extract_usage_json(&payload);

        assert_eq!(usage, Some(json!({"total_tokens": 42})));
    }

    /// Verifies usage extraction ignores null usage payloads.
    #[test]
    fn extract_usage_json_ignores_null_usage() {
        let payload = json!({"usage": null});

        let usage = extract_usage_json(&payload);

        assert_eq!(usage, None);
    }

    /// Verifies usage extraction supports nested completed-response usage payloads.
    #[test]
    fn extract_usage_json_reads_nested_response_usage() {
        let payload =
            json!({"type": "response.completed", "response": {"usage": {"input_tokens": 8}}});

        let usage = extract_usage_json(&payload);

        assert_eq!(usage, Some(json!({"input_tokens": 8})));
    }

    /// Verifies usage injection writes to the top-level usage field when present.
    #[test]
    fn inject_usage_json_updates_top_level_usage() {
        let mut payload = json!({"usage": {"total_tokens": 1}});
        let new_usage = json!({"total_tokens": 99});

        let injected = inject_usage_json(&mut payload, new_usage.clone());

        assert!(injected);
        assert_eq!(payload.get("usage"), Some(&new_usage));
    }

    /// Verifies message-like input items get `type: message` for alias models on /responses.
    #[test]
    fn sanitize_responses_request_body_adds_message_type_for_alias_model() {
        let body = Bytes::from(
            serde_json::to_vec(&json!({
                "model": "alias-fast",
                "input": [
                    {
                        "role": "user",
                        "content": [{"type": "input_text", "text": "hi"}]
                    }
                ]
            }))
            .expect("serialize test body"),
        );

        let sanitized =
            sanitize_responses_request_body(&body).expect("expected request body to be sanitized");
        let sanitized_json: serde_json::Value =
            serde_json::from_slice(&sanitized).expect("parse sanitized request body");

        assert_eq!(sanitized_json["input"][0]["type"], "message");
        assert_eq!(sanitized_json["input"][0]["content"][0]["text"], "hi");
    }

    /// Verifies string-form `input` payloads are left untouched.
    #[test]
    fn sanitize_responses_request_body_skips_string_input() {
        let body = Bytes::from(
            serde_json::to_vec(&json!({
                "model": "alias-fast",
                "input": "hello"
            }))
            .expect("serialize test body"),
        );

        let sanitized = sanitize_responses_request_body(&body);

        assert!(sanitized.is_none());
    }

    /// Verifies non-message items are preserved while compatible message items are repaired.
    #[test]
    fn sanitize_responses_request_body_preserves_non_message_items() {
        let body = Bytes::from(
            serde_json::to_vec(&json!({
                "model": "alias-fast",
                "input": [
                    {
                        "type": "",
                        "foo": "bar"
                    },
                    {
                        "type": "",
                        "role": "user",
                        "content": [{"type": "input_text", "text": "hello"}]
                    },
                    {
                        "type": "function_call",
                        "name": "calc",
                        "arguments": "{\"x\":1}"
                    },
                    {
                        "type": "function_call_output",
                        "call_id": "call_123",
                        "output": "{\"ok\":true}"
                    }
                ]
            }))
            .expect("serialize test body"),
        );

        let sanitized =
            sanitize_responses_request_body(&body).expect("expected request body to be sanitized");
        let sanitized_json: serde_json::Value =
            serde_json::from_slice(&sanitized).expect("parse sanitized request body");

        assert_eq!(sanitized_json["input"].as_array().map(Vec::len), Some(4));
        assert_eq!(sanitized_json["input"][0]["type"], "");
        assert_eq!(sanitized_json["input"][0]["foo"], "bar");
        assert_eq!(sanitized_json["input"][1]["type"], "message");
        assert_eq!(sanitized_json["input"][1]["content"][0]["text"], "hello");
        assert_eq!(sanitized_json["input"][2]["type"], "function_call");
        assert_eq!(sanitized_json["input"][2]["name"], "calc");
        assert_eq!(sanitized_json["input"][3]["type"], "function_call_output");
        assert_eq!(sanitized_json["input"][3]["call_id"], "call_123");
    }

    /// Verifies all message-shaped roles are repaired without dropping other inputs.
    #[test]
    fn sanitize_responses_request_body_repairs_all_message_roles() {
        let body = Bytes::from(
            serde_json::to_vec(&json!({
                "model": "alias-fast",
                "input": [
                    {
                        "role": "developer",
                        "content": "system rules"
                    },
                    {
                        "type": null,
                        "role": "assistant",
                        "id": "msg_1",
                        "phase": "loop",
                        "content": [{"type": "output_text", "text": "ok"}]
                    },
                    {
                        "type": "  ",
                        "role": "user",
                        "content": [{"type": "input_text", "text": "hi"}]
                    },
                    {
                        "type": "reasoning",
                        "id": "rs_1",
                        "encrypted_content": "abc"
                    }
                ]
            }))
            .expect("serialize test body"),
        );

        let sanitized =
            sanitize_responses_request_body(&body).expect("expected request body to be sanitized");
        let sanitized_json: serde_json::Value =
            serde_json::from_slice(&sanitized).expect("parse sanitized request body");

        assert_eq!(sanitized_json["input"][0]["type"], "message");
        assert_eq!(sanitized_json["input"][1]["type"], "message");
        assert_eq!(sanitized_json["input"][2]["type"], "message");
        assert_eq!(sanitized_json["input"][1]["id"], "msg_1");
        assert_eq!(sanitized_json["input"][1]["phase"], "loop");
        assert_eq!(sanitized_json["input"][3]["type"], "reasoning");
    }

    /// Verifies stream responses requests receive the same message type repair behavior.
    #[test]
    fn sanitize_responses_request_body_adds_message_type_for_stream_requests() {
        let body = Bytes::from(
            serde_json::to_vec(&json!({
                "model": "alias-fast",
                "stream": true,
                "input": [
                    {
                        "role": "user",
                        "content": [{"type": "input_text", "text": "hello"}]
                    }
                ]
            }))
            .expect("serialize test body"),
        );

        let sanitized =
            sanitize_responses_request_body(&body).expect("expected request body to be sanitized");
        let sanitized_json: serde_json::Value =
            serde_json::from_slice(&sanitized).expect("parse sanitized request body");

        assert_eq!(sanitized_json["input"][0]["type"], "message");
        assert_eq!(sanitized_json["input"][0]["content"][0]["text"], "hello");
        assert_eq!(sanitized_json["stream"], true);
    }

    /// Verifies non-gpt models drop top-level and reasoning summary fields.
    #[test]
    fn sanitize_responses_request_body_strips_summary_for_non_gpt_models() {
        let body = Bytes::from(
            serde_json::to_vec(&json!({
                "model": "alias-fast",
                "summary": "auto",
                "reasoning": {
                    "effort": "medium",
                    "summary": "concise"
                },
                "input": [
                    {
                        "role": "user",
                        "content": [{"type": "input_text", "text": "hello"}]
                    }
                ]
            }))
            .expect("serialize test body"),
        );

        let sanitized =
            sanitize_responses_request_body(&body).expect("expected request body to be sanitized");
        let sanitized_json: serde_json::Value =
            serde_json::from_slice(&sanitized).expect("parse sanitized request body");

        assert_eq!(sanitized_json.get("summary"), None);
        assert_eq!(sanitized_json["reasoning"].get("summary"), None);
        assert_eq!(sanitized_json["reasoning"]["effort"], "medium");
        assert_eq!(sanitized_json["input"][0]["type"], "message");
    }

    /// Verifies gpt models keep summary fields untouched.
    #[test]
    fn sanitize_responses_request_body_keeps_summary_for_gpt_models() {
        let body = Bytes::from(
            serde_json::to_vec(&json!({
                "model": "gpt-6-sol",
                "summary": "auto",
                "reasoning": {
                    "effort": "high",
                    "summary": "detailed"
                },
                "input": [
                    {
                        "type": "message",
                        "role": "user",
                        "content": [{"type": "input_text", "text": "hello"}]
                    }
                ]
            }))
            .expect("serialize test body"),
        );

        let sanitized = sanitize_responses_request_body(&body);

        assert!(sanitized.is_none());
    }

    /// Verifies non-gpt models still strip summary even when `input` is not an array.
    #[test]
    fn sanitize_responses_request_body_strips_summary_without_array_input() {
        let body = Bytes::from(
            serde_json::to_vec(&json!({
                "model": "alias-fast",
                "summary": "auto",
                "reasoning": {
                    "summary": "concise"
                },
                "input": "hello"
            }))
            .expect("serialize test body"),
        );

        let sanitized =
            sanitize_responses_request_body(&body).expect("expected request body to be sanitized");
        let sanitized_json: serde_json::Value =
            serde_json::from_slice(&sanitized).expect("parse sanitized request body");

        assert_eq!(sanitized_json.get("summary"), None);
        assert_eq!(sanitized_json["reasoning"].get("summary"), None);
        assert_eq!(sanitized_json["input"], "hello");
    }
}

pub fn build_client<M>(
    provider: &ProviderConfig,
) -> Result<Arc<dyn ModelApiProvider<M>>, ConfigError> {
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
        header::AUTHORIZATION,
        auth_value
            .parse::<HeaderValue>()
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
