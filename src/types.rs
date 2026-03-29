use serde::{Deserialize, Serialize};

/// Handshake request from Tool to Zipper
#[derive(Debug, Serialize, Deserialize, Default)]
pub struct HandshakeRequest {
    /// Unique tool name used for routing.
    pub name: String,
    /// Credential token used for authentication.
    pub credential: String,
    /// Optional JSON schema describing the tool input/output contract.
    #[serde(default)]
    pub json_schema: Option<String>,
}

/// Handshake response from Zipper to Tool
#[derive(Debug, Serialize, Deserialize, Default)]
pub struct HandshakeResponse {
    /// HTTP-like status code for handshake result.
    pub status_code: u16,
    /// Error message when handshake is rejected.
    pub error_msg: String,
}

/// Response body transfer mode.
#[derive(Debug, Serialize, Deserialize, Default, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum BodyFormat {
    /// No body payload.
    #[default]
    Null,
    /// A single binary payload.
    Bytes,
    /// A chunked streaming payload.
    Chunk,
}

/// Request headers for proxying requests through the system
#[derive(Debug, Serialize, Deserialize, Default)]
pub struct RequestHeaders {
    /// Target tool name.
    pub name: String,
    /// Distributed trace identifier.
    pub trace_id: String,
    /// Distributed span identifier.
    pub span_id: String,
    /// Request body framing format.
    pub body_format: BodyFormat,
    /// User-defined extension metadata.
    pub extension: String,
}

/// Response headers for responses from Tool
#[derive(Debug, Serialize, Deserialize, Default)]
pub struct ResponseHeaders {
    /// HTTP-like status code.
    pub status_code: u16,
    /// Error message when request handling fails.
    pub error_msg: String,
    /// Response body framing format.
    pub body_format: BodyFormat,
    /// User-defined extension metadata.
    pub extension: String,
}

/// Tool request body
#[derive(Debug, Serialize, Deserialize, Default)]
pub struct ToolRequest {
    /// Serialized tool arguments.
    pub args: String,
    /// Optional context for agent/tool execution.
    pub agent_context: Option<String>,
}

/// Tool response body
#[derive(Debug, Serialize, Deserialize, Default)]
pub struct ToolResponse {
    /// Tool execution result payload.
    pub result: Option<String>,
    /// Error details returned by tool execution.
    pub error_msg: Option<String>,
}
