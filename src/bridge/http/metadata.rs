use anyhow::Result;
use axum::http::HeaderMap;
use nanoid::nanoid;

use crate::metadata::Metadata;

pub(crate) struct HttpMetadata {
    trace_id: String,
    req_id: String,
}

impl HttpMetadata {
    pub fn new(headers: &HeaderMap) -> Result<Self> {
        Ok(Self {
            trace_id: match headers.get("x-trace-id") {
                Some(v) => v.to_str()?.to_owned(),
                None => nanoid!(16),
            },
            req_id: match headers.get("x-req-id") {
                Some(v) => v.to_str()?.to_owned(),
                None => nanoid!(8),
            },
        })
    }
}

impl Metadata for HttpMetadata {
    fn trace_id(&self) -> &str {
        &self.trace_id
    }

    fn req_id(&self) -> &str {
        &self.req_id
    }
}
