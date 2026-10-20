//! AWS error interpretation.

use super::{UpstreamError, fallback_error};
use crate::openai_types::ErrorDetail;
use crate::provider::ProviderError;
use aws_sdk_bedrockruntime::error::ProvideErrorMetadata;
use axum::http::{HeaderMap, StatusCode};
use serde::Deserialize;

#[derive(Deserialize)]
struct ErrorBody {
    message: String,
    #[serde(rename = "__type")]
    error_type: Option<String>,
}

/// Parse HTTP AWS errors while preserving SDK/transport errors in their own layer.
pub fn parse_error(status: StatusCode, headers: &HeaderMap, body: &[u8]) -> UpstreamError {
    let mut error = fallback_error(status, headers, body);
    if let Ok(payload) = serde_json::from_slice::<ErrorBody>(body) {
        error.message = payload.message;
        error.error_type = payload.error_type;
    }
    if let Some(value) = headers
        .get("x-amzn-errortype")
        .and_then(|value| value.to_str().ok())
    {
        error.code = Some(value.split(':').next().unwrap_or(value).to_owned());
    }
    error
}

/// Interpret SDK service metadata independently of the dialect transport.
pub(crate) fn sdk_service_error(error: &impl ProvideErrorMetadata) -> ProviderError {
    let code = error.code().unwrap_or("UnknownBedrockError");
    let status = match code {
        "ValidationException" => StatusCode::BAD_REQUEST,
        "AccessDeniedException" => StatusCode::FORBIDDEN,
        "ThrottlingException" => StatusCode::TOO_MANY_REQUESTS,
        "ServiceUnavailableException" => StatusCode::SERVICE_UNAVAILABLE,
        _ => StatusCode::BAD_GATEWAY,
    };
    ProviderError::Public {
        status,
        error: ErrorDetail {
            message: error.message().unwrap_or(code).to_owned(),
            r#type: "upstream_error".into(),
            code: Some(code.to_owned()),
            param: None,
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use aws_sdk_bedrockruntime::error::ErrorMetadata;

    #[test]
    fn sdk_throttling_retains_service_code_and_appropriate_status() {
        let metadata = ErrorMetadata::builder()
            .code("ThrottlingException")
            .message("rate exceeded")
            .build();
        let ProviderError::Public { status, error } = sdk_service_error(&metadata) else {
            panic!("public upstream error");
        };
        assert_eq!(status, StatusCode::TOO_MANY_REQUESTS);
        assert_eq!(error.code.as_deref(), Some("ThrottlingException"));
        assert_eq!(error.message, "rate exceeded");
    }
}
