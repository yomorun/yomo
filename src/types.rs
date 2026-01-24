use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize, Default)]
pub struct HandshakeReq {
    pub sfn_name: String,
    pub credential: String,
}

#[derive(Debug, Serialize, Deserialize, Default)]
pub struct HandshakeRes {
    pub ok: bool,
    pub reason: String,
}

#[derive(Debug, Serialize, Deserialize, Default)]
pub struct RequestHeaders {
    pub sfn_name: String,
    pub stream: bool,
    pub trace_id: String,
    pub request_id: String,
    pub extension: String,
}

#[derive(Debug, Serialize, Deserialize, Default)]
pub struct ResponseHeaders {
    pub status_code: u16,
    pub error_msg: String,
    pub stream: bool,
    pub extension: String,
}
