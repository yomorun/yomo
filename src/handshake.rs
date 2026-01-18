use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize)]
pub(crate) struct HandshakeReq {
    pub(crate) sfn_name: String,
    pub(crate) credential: Option<String>,
}

#[derive(Serialize, Deserialize)]
pub(crate) struct HandshakeRes {
    pub(crate) ok: bool,
}
