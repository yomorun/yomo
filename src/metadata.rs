use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize, Default, Debug)]
pub struct SfnMetadata {
    pub extra: String,
}

#[derive(Serialize, Deserialize, Default, Debug)]
pub struct RequestMetadata {
    pub trace_id: String,
    pub req_id: String,
    pub extra: String,
}
