use serde::{Deserialize, Serialize};

/// Handshake request from Tool to Zipper
#[derive(Debug, Serialize, Deserialize, Default)]
pub struct HandshakeRequest {
    pub name: String,
    pub credential: String,
    #[serde(default)]
    pub json_schema: Option<String>,
}

/// Handshake response from Zipper to Tool
#[derive(Debug, Serialize, Deserialize, Default)]
pub struct HandshakeResponse {
    pub status_code: u16,
    pub error_msg: String,
}

#[derive(Debug, Serialize, Deserialize, Default, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum BodyFormat {
    #[default]
    Null,
    Bytes,
    Chunk,
}

/// Request headers for proxying requests through the system
#[derive(Debug, Serialize, Deserialize, Default)]
pub struct RequestHeaders {
    pub name: String,
    pub trace_id: String,
    pub span_id: String,
    pub body_format: BodyFormat,
    pub extension: String,
}

/// Response headers for responses from Tool
#[derive(Debug, Serialize, Deserialize, Default)]
pub struct ResponseHeaders {
    pub status_code: u16,
    pub error_msg: String,
    pub body_format: BodyFormat,
    pub extension: String,
}

/// Tool request body
#[derive(Debug, Serialize, Deserialize, Default)]
pub struct ToolRequest {
    pub args: String,
    pub agent_context: Option<String>,
}

/// Tool response body
#[derive(Debug, Serialize, Deserialize, Default)]
pub struct ToolResponse {
    pub result: Option<String>,
    pub error_msg: Option<String>,
}
