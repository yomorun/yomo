use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
pub(crate) struct HandshakeReq {
    pub(crate) sfn_name: String,
    pub(crate) credential: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub(crate) struct HandshakeRes {
    pub(crate) ok: bool,
    pub(crate) reason: Option<String>,
}
