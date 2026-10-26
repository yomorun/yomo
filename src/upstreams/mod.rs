//! Explicit upstream vendor selection, wire vocabulary and error interpretation.
//!
//! This layer knows how to talk to a vendor (authentication, paths, error
//! envelopes). Model-specific capabilities are not inferred by this layer.

use axum::http::{HeaderMap, StatusCode};
use serde::Deserialize;

use crate::dialects::Dialect;

pub mod aws_bedrock;
pub mod azure_openai;
pub mod google_vertex;
pub mod tokenhub;
pub mod vllm;
pub mod volcengine_ark;

/// Service integration selected by a configured connection.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum Upstream {
    /// Tencent TokenHub.
    Tokenhub,
    /// Azure OpenAI.
    AzureOpenai,
    /// Volcengine Ark.
    VolcengineArk,
    /// AWS Bedrock.
    AwsBedrock,
    /// Google Vertex AI.
    GoogleVertex,
    /// Explicit OpenAI-compatible integration for custom services.
    OpenaiCompatible,
    /// Self-hosted vLLM or SGLang using OpenAI-compatible chat completions.
    Vllm,
    /// Opaque forwarding without a conversation dialect.
    Custom,
}

/// Endpoint path owned by the upstream integration; dialect clients encode the body.
pub fn request_path(dialect: Dialect) -> &'static str {
    match dialect {
        Dialect::OpenAiChatCompletions => "/chat/completions",
        Dialect::OpenAiResponses => "/responses",
        Dialect::AnthropicMessages => "/messages",
        Dialect::BedrockConverse => "/converse",
        Dialect::GeminiGenerateContent => ":generateContent",
    }
}

/// Failure class used independently of the client's API error format.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum UpstreamErrorClass {
    /// Invalid parameters or input.
    InvalidRequest,
    /// Missing or invalid credentials.
    Authentication,
    /// Access denied.
    Permission,
    /// Rate limit, unless the vendor establishes a quota error.
    RateLimit,
    /// Account quota exhausted.
    Quota,
    /// Upstream service failure.
    Service,
    /// No established error category.
    Unknown,
}

/// Normalized error with original response retained for internal diagnostics.
#[derive(Debug, Clone)]
pub struct UpstreamError {
    /// HTTP status is authoritative even when a payload embeds another status.
    pub status: StatusCode,
    /// Vendor code, preserved rather than mapped away.
    pub code: Option<String>,
    /// Vendor type/category.
    pub error_type: Option<String>,
    /// Human-readable description.
    pub message: String,
    /// Related input parameter.
    pub param: Option<String>,
    /// Upstream response correlation identifier.
    pub request_id: Option<String>,
    /// Error classification.
    pub class: UpstreamErrorClass,
    /// Original bytes, including non-JSON gateway responses.
    pub raw_body: Vec<u8>,
}

pub(super) fn fallback_error(
    status: StatusCode,
    headers: &HeaderMap,
    body: &[u8],
) -> UpstreamError {
    let class = match status.as_u16() {
        400 | 422 => UpstreamErrorClass::InvalidRequest,
        401 => UpstreamErrorClass::Authentication,
        403 => UpstreamErrorClass::Permission,
        429 => UpstreamErrorClass::RateLimit,
        500..=599 => UpstreamErrorClass::Service,
        _ => UpstreamErrorClass::Unknown,
    };
    let request_id = [
        "x-request-id",
        "request-id",
        "x-ms-request-id",
        "x-amzn-requestid",
    ]
    .iter()
    .find_map(|name| {
        headers
            .get(*name)
            .and_then(|value| value.to_str().ok())
            .map(str::to_owned)
    });
    UpstreamError {
        status,
        code: None,
        error_type: None,
        message: String::from_utf8_lossy(body).into_owned(),
        param: None,
        request_id,
        class,
        raw_body: body.to_vec(),
    }
}

/// Interpret a vendor response without assuming a client's error format.
pub fn parse_error(
    upstream: Upstream,
    status: StatusCode,
    headers: &HeaderMap,
    body: &[u8],
) -> UpstreamError {
    match upstream {
        Upstream::Tokenhub => tokenhub::parse_error(status, headers, body),
        // OpenAI-compatible services use the standard OpenAI error envelope.
        Upstream::OpenaiCompatible => tokenhub::parse_error(status, headers, body),
        Upstream::AzureOpenai => azure_openai::parse_error(status, headers, body),
        Upstream::VolcengineArk => volcengine_ark::parse_error(status, headers, body),
        Upstream::Vllm => vllm::parse_error(status, headers, body),
        Upstream::AwsBedrock => aws_bedrock::parse_error(status, headers, body),
        Upstream::GoogleVertex => google_vertex::parse_error(status, headers, body),
        _ => fallback_error(status, headers, body),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn openai_compatible_errors_use_the_openai_envelope() {
        let body = br#"{
            "error": {
                "message": "bad model",
                "type": "invalid_request_error",
                "code": "bad_request",
                "param": "model"
            }
        }"#;

        let error = parse_error(
            Upstream::OpenaiCompatible,
            StatusCode::BAD_REQUEST,
            &HeaderMap::new(),
            body,
        );

        assert_eq!(error.message, "bad model");
        assert_eq!(error.error_type.as_deref(), Some("invalid_request_error"));
        assert_eq!(error.code.as_deref(), Some("bad_request"));
        assert_eq!(error.param.as_deref(), Some("model"));
    }
}
