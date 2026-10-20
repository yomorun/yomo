//! Azure OpenAI error envelope.

use super::{UpstreamError, fallback_error};
use crate::openai_types::ErrorDetail as PublicErrorDetail;
use axum::http::{HeaderMap, StatusCode};
use serde::Deserialize;
use serde_json::Value;

#[derive(Deserialize)]
struct ErrorEnvelope {
    error: ErrorDetail,
}
#[derive(Deserialize)]
struct ErrorDetail {
    message: String,
    #[serde(rename = "type")]
    error_type: Option<String>,
    code: Option<Value>,
    param: Option<String>,
}

/// Decode a shared OpenAI-style envelope for generic Chat clients. Vendor
/// payload structures remain owned by upstream integrations, not dialects.
pub(crate) fn parse_compatible_error(status: StatusCode, body: &[u8]) -> Option<PublicErrorDetail> {
    let payload: ErrorEnvelope = serde_json::from_slice(body).ok()?;
    Some(PublicErrorDetail {
        message: payload.error.message,
        r#type: payload
            .error
            .error_type
            .filter(|value| !value.trim().is_empty())
            .unwrap_or_else(|| {
                if status == StatusCode::BAD_REQUEST {
                    "invalid_request_error".into()
                } else {
                    "internal_error".into()
                }
            }),
        code: payload.error.code.map(|value| match value {
            Value::String(value) => value,
            value => value.to_string(),
        }),
        param: payload.error.param,
    })
}

/// Parse Azure's nullable error fields without trusting a nested status value.
pub fn parse_error(status: StatusCode, headers: &HeaderMap, body: &[u8]) -> UpstreamError {
    let mut error = fallback_error(status, headers, body);
    if let Ok(payload) = serde_json::from_slice::<ErrorEnvelope>(body) {
        error.message = payload.error.message;
        error.error_type = payload.error.error_type;
        error.code = payload.error.code.map(|value| match value {
            Value::String(value) => value,
            value => value.to_string(),
        });
        error.param = payload.error.param;
    }
    error
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn nullable_type_and_embedded_status_do_not_replace_http_status() {
        let error = parse_error(
            StatusCode::BAD_REQUEST,
            &HeaderMap::new(),
            br#"{"error":{"message":"invalid","type":null,"code":"InvalidArgument","status":500}}"#,
        );
        assert_eq!(error.status, StatusCode::BAD_REQUEST);
        assert_eq!(error.code.as_deref(), Some("InvalidArgument"));
        assert_eq!(error.error_type, None);
    }
}
