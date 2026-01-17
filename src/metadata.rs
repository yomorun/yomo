use anyhow::Result;
use axum::http::HeaderMap;
use nanoid::nanoid;

pub trait Metadata: Sync + Send {
    fn trace_id(&self) -> &str;

    fn req_id(&self) -> &str;
}

pub(crate) struct DefaultMetadata {
    trace_id: String,
    req_id: String,
}

impl DefaultMetadata {
    pub fn new(headers: &HeaderMap) -> Result<Self> {
        Ok(Self {
            trace_id: match headers.get("x-trace-id") {
                Some(v) => v.to_str()?.to_owned(),
                None => nanoid!(12),
            },
            req_id: match headers.get("x-req-id") {
                Some(v) => v.to_str()?.to_owned(),
                None => nanoid!(8),
            },
        })
    }
}

impl Metadata for DefaultMetadata {
    fn trace_id(&self) -> &str {
        &self.trace_id
    }

    fn req_id(&self) -> &str {
        &self.req_id
    }
}
