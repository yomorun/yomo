//! TokenHub platform error interpretation.

use axum::http::{HeaderMap, StatusCode};
use serde::Deserialize;
use serde_json::Value;

use super::{UpstreamError, UpstreamErrorClass, fallback_error};

#[derive(Deserialize)]
struct ErrorEnvelope {
    error: ErrorDetail,
}
#[derive(Deserialize)]
struct ErrorDetail {
    message: Option<String>,
    #[serde(rename = "type")]
    error_type: Option<String>,
    code: Option<Value>,
    param: Option<String>,
    source: Option<String>,
    upstream_code: Option<String>,
    upstream_status: Option<u16>,
}

/// Parse Chat-style platform errors while retaining gateway response fallbacks.
pub fn parse_error(status: StatusCode, headers: &HeaderMap, body: &[u8]) -> UpstreamError {
    let mut error = fallback_error(status, headers, body);
    if let Ok(payload) = serde_json::from_slice::<ErrorEnvelope>(body) {
        if let Some(message) = payload.error.message {
            error.message = message;
        }
        error.error_type = payload.error.error_type;
        error.code = payload.error.code.map(|code| match code {
            Value::String(value) => value,
            value => value.to_string(),
        });
        error.param = payload.error.param;
        if payload.error.source.is_some()
            || payload.error.upstream_code.is_some()
            || payload.error.upstream_status.is_some()
        {
            error.message = format!(
                "{} [source={}, upstream_code={}, upstream_status={}]",
                error.message,
                payload.error.source.as_deref().unwrap_or_default(),
                payload.error.upstream_code.as_deref().unwrap_or_default(),
                payload
                    .error
                    .upstream_status
                    .map(|status| status.to_string())
                    .unwrap_or_default()
            );
        }
        if error.code.as_deref() == Some("insufficient_quota") {
            error.class = UpstreamErrorClass::Quota;
        }
    }
    error
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn preserves_status_request_id_and_non_json_body() {
        let mut headers = HeaderMap::new();
        headers.insert("x-request-id", "upstream-123".parse().expect("header"));
        let body = b"<html>gateway unavailable</html>";
        let error = parse_error(StatusCode::BAD_GATEWAY, &headers, body);
        assert_eq!(error.status, StatusCode::BAD_GATEWAY);
        assert_eq!(error.request_id.as_deref(), Some("upstream-123"));
        assert_eq!(error.raw_body, body);
    }
    #[test]
    fn distinguishes_quota_from_throttling_without_losing_code() {
        let error = parse_error(StatusCode::TOO_MANY_REQUESTS, &HeaderMap::new(), br#"{"error":{"message":"quota","code":"insufficient_quota","type":null,"param":null}}"#);
        assert_eq!(error.class, UpstreamErrorClass::Quota);
        assert_eq!(error.code.as_deref(), Some("insufficient_quota"));
    }

    #[test]
    fn aggregation_error_metadata_is_not_discarded() {
        let error = parse_error(StatusCode::BAD_GATEWAY, &HeaderMap::new(), br#"{"error":{"message":"upstream rejected","code":"upstream_error","source":"provider","upstream_code":"limit","upstream_status":429}}"#);
        assert_eq!(error.code.as_deref(), Some("upstream_error"));
        assert!(error.message.contains("upstream_status=429"));
        assert!(error.message.contains("upstream_code=limit"));
    }
}
