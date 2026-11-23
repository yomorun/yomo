//! Google error interpretation.

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
    status: Option<String>,
}

/// Preserve authoritative HTTP status and Google's symbolic error status.
pub fn parse_error(status: StatusCode, headers: &HeaderMap, body: &[u8]) -> UpstreamError {
    let mut error = fallback_error(status, headers, body);
    if let Ok(payload) = serde_json::from_slice::<ErrorEnvelope>(body) {
        error.message = payload.error.message;
        error.code = payload.error.status;
        error.error_type = Some(
            match error.code.as_deref() {
                Some("UNAUTHENTICATED") => "authentication_error",
                Some("PERMISSION_DENIED") => "permission_denied_error",
                Some("RESOURCE_EXHAUSTED") => "rate_limit_error",
                Some("FAILED_PRECONDITION") => "failed_precondition_error",
                Some("INVALID_ARGUMENT") => "invalid_argument_error",
                _ => "upstream_error",
            }
            .to_owned(),
        );
        if error
            .message
            .to_ascii_lowercase()
            .contains("missing a thought_signature")
        {
            error.error_type = Some("missing_thought_signature_error".into());
            error.code = Some("missing_thought_signature".into());
        }
    }
    error
}
