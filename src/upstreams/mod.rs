//! Explicit upstream vendor selection, wire vocabulary and error interpretation.
//!
//! This layer knows how to talk to a vendor (authentication, paths, error
//! envelopes). What a particular model supports lives in `crate::models`.

use axum::http::{HeaderMap, StatusCode};
use serde::Deserialize;

pub mod aws_bedrock;
pub mod azure_openai;
pub mod google_vertex;
pub mod tokenhub;
pub mod vivgrid;
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
    /// Vivgrid API.
    Vivgrid,
    /// Explicit OpenAI-compatible integration for custom services.
    OpenaiCompatible,
    /// Explicit self-hosted vLLM integration.
    Vllm,
    /// Opaque forwarding without a conversation dialect.
    Custom,
}

/// Format used to communicate with the selected upstream.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Deserialize)]
pub enum Dialect {
    /// OpenAI Chat Completions.
    #[serde(rename = "openai-chat-completions")]
    OpenAiChatCompletions,
    /// OpenAI Responses.
    #[serde(rename = "openai-responses")]
    OpenAiResponses,
    /// Anthropic Messages.
    #[serde(rename = "anthropic-messages")]
    AnthropicMessages,
    /// Bedrock Converse.
    #[serde(rename = "bedrock-converse")]
    BedrockConverse,
    /// Gemini generateContent.
    #[serde(rename = "gemini-generate-content")]
    GeminiGenerateContent,
}

/// Target field used for a Chat output-token budget.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum MaxTokensField {
    /// Legacy max_tokens field.
    MaxTokens,
    /// max_completion_tokens field.
    MaxCompletionTokens,
}

/// Native reasoning-effort field binding, without value conversion.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EffortField {
    /// reasoning_effort at the request root.
    ReasoningEffort,
    /// reasoning.effort.
    Reasoning,
    /// output_config.effort.
    OutputConfig,
    /// generationConfig.thinkingConfig.thinkingLevel; whether a concrete
    /// model accepts a given level is decided by the model file.
    ThinkingLevel,
    /// vLLM chat_template_kwargs.reasoning_effort.
    ChatTemplate,
    /// Model-specific fields require further evidence.
    Unknown,
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
        Upstream::AzureOpenai => azure_openai::parse_error(status, headers, body),
        Upstream::VolcengineArk => volcengine_ark::parse_error(status, headers, body),
        Upstream::AwsBedrock => aws_bedrock::parse_error(status, headers, body),
        Upstream::GoogleVertex => google_vertex::parse_error(status, headers, body),
        Upstream::Vivgrid => vivgrid::parse_error(status, headers, body),
        _ => fallback_error(status, headers, body),
    }
}
