use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
pub struct HandlerRequest {
    pub args: String,
    pub stream: bool,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct HandlerResponse {
    pub result: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct HandlerChunk {
    pub chunk: String,
}
