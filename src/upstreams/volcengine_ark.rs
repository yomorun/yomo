//! Ark error interpretation.

use super::{UpstreamError, fallback_error};
use axum::http::{HeaderMap, StatusCode};
use serde::Deserialize;
use serde_json::{Value, json};

#[derive(Deserialize)]
struct ErrorEnvelope {
    error: ErrorDetail,
}
#[derive(Deserialize)]
struct ErrorDetail {
    message: String,
    code: Option<String>,
    #[serde(rename = "type")]
    error_type: Option<String>,
}

/// Parse Ark errors with a lossless malformed-response fallback.
pub fn parse_error(status: StatusCode, headers: &HeaderMap, body: &[u8]) -> UpstreamError {
    let mut error = fallback_error(status, headers, body);
    if let Ok(payload) = serde_json::from_slice::<ErrorEnvelope>(body) {
        error.message = payload.error.message;
        error.code = payload.error.code;
        error.error_type = payload.error.error_type;
    }
    error
}

/// Encode Ark Chat's thinking mode and effort controls without model inference.
pub(crate) fn encode_chat_reasoning(body: &mut Value) {
    if body["reasoning_effort"].as_str() == Some("none") && body.get("thinking").is_none() {
        body["thinking"] = json!({"type":"disabled"});
    }
    let mode = thinking_mode(body.get("thinking"));
    if (mode == Some(false) && body["reasoning_effort"].as_str() != Some("none"))
        || (mode == Some(true) && body["reasoning_effort"].as_str() == Some("none"))
    {
        if let Some(object) = body.as_object_mut() {
            object.remove("reasoning_effort");
        }
    }
    if mode == Some(false) {
        if let Some(thinking) = body.get_mut("thinking").and_then(Value::as_object_mut) {
            thinking.remove("budget_tokens");
        }
    }
    if let Some(object) = body.as_object_mut() {
        object.remove("chat_template_kwargs");
    }
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
