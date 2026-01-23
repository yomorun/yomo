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

#[derive(Debug, Serialize, Deserialize, Default)]
pub struct RequestHeaders {
    pub trace_id: String,
    pub req_id: String,
    pub sfn_name: String,
    pub stream: bool,
    pub extension: String,
}

#[derive(Debug, Serialize, Deserialize, Default)]
pub struct RequestBody {
    pub args: String,
    #[serde(default)]
    pub context: String,
}

#[derive(Debug, Serialize, Deserialize, Default)]
pub struct ResponseBody {
    pub data: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub error: String,
}
