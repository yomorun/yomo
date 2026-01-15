use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize, Default)]
pub struct SfnRequest {
    pub args: String,
    pub context: String,
}

#[derive(Serialize, Deserialize, Default)]
pub struct SfnResponse {
    pub result: String,
}
