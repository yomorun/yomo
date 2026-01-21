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
