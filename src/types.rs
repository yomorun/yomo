use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
pub struct HandshakeReq {
    pub sfn_name: String,
    pub credential: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct HandshakeRes {
    pub ok: bool,
    pub reason: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct RequestHeaders {
    pub stream: bool,
    pub sfn_name: String,
    pub trace_id: String,
    pub req_id: String,
}
