use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
pub struct Request {
    pub data: Vec<u8>,
    pub stream: bool,
}

#[derive(Debug, Serialize, Deserialize)]
#[serde(tag = "t", content = "v")]
pub enum Response {
    Data(Vec<u8>),
    Error(String),
    End,
}
