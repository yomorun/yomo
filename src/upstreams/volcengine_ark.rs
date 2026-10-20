//! Ark error interpretation.

use super::{UpstreamError, fallback_error};
use axum::http::{HeaderMap, StatusCode};
use serde::Deserialize;

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
