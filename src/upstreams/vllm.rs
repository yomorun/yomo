//! Reasoning controls and OpenAI-compatible errors for self-hosted vLLM/SGLang.

use axum::http::{HeaderMap, StatusCode};
use serde::Deserialize;
use serde_json::{Value, json};

use super::{UpstreamError, fallback_error};

/// vLLM's OpenAI server emits a flat error object; some versions also use the
/// standard nested OpenAI error envelope.
#[derive(Deserialize)]
#[serde(untagged)]
enum ErrorEnvelope {
    Nested { error: ErrorResponse },
    Flat(ErrorResponse),
}

#[derive(Deserialize)]
struct ErrorResponse {
    message: Option<String>,
    #[serde(rename = "type")]
    error_type: Option<String>,
    param: Option<Value>,
    code: Option<Value>,
}

/// Encode vLLM/SGLang's chat-template reasoning controls.
///
/// Effort values are forwarded verbatim because the two runtimes' preset maps
/// differ; numeric values remain numeric for DeepSeek-V4.1 deployments.
pub(crate) fn encode_chat_reasoning(body: &mut Value) {
    let mut kwargs = body
        .get("chat_template_kwargs")
        .and_then(Value::as_object)
        .cloned()
        .unwrap_or_default();
    let mode = thinking_mode(body.get("thinking"));
    let effort = body.get("reasoning_effort").cloned();
    let thinking_budget = body
        .get("thinking")
        .and_then(|thinking| thinking.get("budget_tokens"))
        .filter(|budget| budget.as_u64().is_some())
        .cloned();

    if !kwargs.contains_key("thinking") {
        if let Some(enabled) = mode {
            kwargs.insert("thinking".into(), json!(enabled));
        } else if let Some(effort) = kwargs.get("reasoning_effort").or(effort.as_ref()) {
            kwargs.insert("thinking".into(), json!(!effort_is_none(Some(effort))));
        } else if thinking_budget.is_some() {
            kwargs.insert("thinking".into(), json!(true));
        }
    }
    if let Some(effort) = effort {
        if !effort_is_none(Some(&effort)) && !kwargs.contains_key("reasoning_effort") {
            kwargs.insert("reasoning_effort".into(), effort);
        }
    }

    let effective_mode = kwargs.get("thinking").and_then(Value::as_bool).or(mode);
    match effective_mode {
        Some(false) => {
            kwargs.remove("reasoning_effort");
        }
        Some(true)
            if kwargs
                .get("reasoning_effort")
                .is_some_and(effort_is_none_value) =>
        {
            kwargs.remove("reasoning_effort");
        }
        _ => {}
    }

    if effective_mode != Some(false) {
        if let Some(budget) = thinking_budget {
            body["thinking_token_budget"] = budget;
        }
    }
    if let Some(object) = body.as_object_mut() {
        object.remove("thinking");
        object.remove("reasoning_effort");
        object.remove("chat_template_kwargs");
        if !kwargs.is_empty() {
            object.insert("chat_template_kwargs".into(), Value::Object(kwargs));
        }
    }
}

/// Parse flat vLLM or nested OpenAI error payloads, preserving HTTP status as
/// authoritative and retaining malformed bodies through the shared fallback.
pub fn parse_error(status: StatusCode, headers: &HeaderMap, body: &[u8]) -> UpstreamError {
    let mut error = fallback_error(status, headers, body);
    let response = match serde_json::from_slice::<ErrorEnvelope>(body) {
        Ok(ErrorEnvelope::Nested { error }) | Ok(ErrorEnvelope::Flat(error)) => error,
        Err(_) => return error,
    };
    if let Some(message) = response.message {
        error.message = message;
    }
    error.error_type = response.error_type;
    error.code = response.code.map(value_to_string);
    error.param = response.param.and_then(|value| match value {
        Value::Null => None,
        Value::String(value) => Some(value),
        value => Some(value.to_string()),
    });
    error
}

fn value_to_string(value: Value) -> String {
    match value {
        Value::String(value) => value,
        value => value.to_string(),
    }
}

fn effort_is_none(value: Option<&Value>) -> bool {
    value.and_then(Value::as_str) == Some("none")
}

fn effort_is_none_value(value: &Value) -> bool {
    effort_is_none(Some(value))
}

fn thinking_mode(value: Option<&Value>) -> Option<bool> {
    match value? {
        Value::Object(object) => match object.get("type").and_then(Value::as_str) {
            Some("enabled" | "adaptive") => Some(true),
            Some("disabled") => Some(false),
            _ => None,
        },
        Value::Bool(enabled) => Some(*enabled),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::upstreams::UpstreamErrorClass;

    #[test]
    fn parses_vllm_flat_error_response() {
        let error = parse_error(
            StatusCode::BAD_REQUEST,
            &HeaderMap::new(),
            br#"{"object":"error","message":"invalid effort","type":"BadRequestError","param":"reasoning_effort","code":400}"#,
        );
        assert_eq!(error.status, StatusCode::BAD_REQUEST);
        assert_eq!(error.message, "invalid effort");
        assert_eq!(error.error_type.as_deref(), Some("BadRequestError"));
        assert_eq!(error.param.as_deref(), Some("reasoning_effort"));
        assert_eq!(error.code.as_deref(), Some("400"));
        assert_eq!(error.class, UpstreamErrorClass::InvalidRequest);
    }

    #[test]
    fn parses_nested_openai_error_and_keeps_http_status() {
        let error = parse_error(
            StatusCode::TOO_MANY_REQUESTS,
            &HeaderMap::new(),
            br#"{"error":{"message":"busy","type":"rate_limit_error","code":"429","param":null,"status":400}}"#,
        );
        assert_eq!(error.status, StatusCode::TOO_MANY_REQUESTS);
        assert_eq!(error.message, "busy");
        assert_eq!(error.code.as_deref(), Some("429"));
        assert_eq!(error.class, UpstreamErrorClass::RateLimit);
    }

    #[test]
    fn malformed_error_payload_uses_lossless_fallback() {
        let body = b"not json";
        let error = parse_error(StatusCode::BAD_GATEWAY, &HeaderMap::new(), body);
        assert_eq!(error.message, "not json");
        assert_eq!(error.raw_body, body);
    }
}
