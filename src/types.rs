use serde::{Deserialize, Serialize};

/// Handshake request from SFN to Zipper
#[derive(Debug, Serialize, Deserialize, Default)]
pub struct HandshakeRequest {
    pub sfn_name: String,
    pub credential: String,
}

/// Handshake response from Zipper to SFN
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
    pub sfn_name: String,
    pub trace_id: String,
    pub request_id: String,
    pub body_format: BodyFormat,
    pub extension: String,
}

/// Response headers for responses from SFN
#[derive(Debug, Serialize, Deserialize, Default)]
pub struct ResponseHeaders {
    pub status_code: u16,
    pub error_msg: String,
    pub body_format: BodyFormat,
    pub extension: String,
}
