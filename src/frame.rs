use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
#[serde(tag = "t", content = "v")]
pub enum Frame<T> {
    Packet(T),
    Chunk(usize, T),
    ChunkDone(usize),
}

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
pub struct HandlerDelta {
    pub delta: String,
}
