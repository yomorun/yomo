use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize, Debug)]
#[serde(tag = "type", content = "content")]
pub(crate) enum Frame {
    Handshake { payload: HandshakePayload },
    HandshakeAck { payload: HandshakeAckPayload },
}

#[derive(Serialize, Deserialize, Default, Debug)]
pub(crate) struct HandshakePayload {
    pub(crate) sfn_name: String,
    pub(crate) credential: Option<String>,
    pub(crate) metadata: Vec<u8>,
}

#[derive(Serialize, Deserialize, Default, Debug)]
pub(crate) struct HandshakeAckPayload {
    pub(crate) ok: bool,
    pub(crate) reason: Option<String>,
}
