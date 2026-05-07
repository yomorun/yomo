use anyhow::Result;
use axum::http::HeaderMap;

/// Build metadata from handshake auth info and request extension.
pub trait MetadataMgr<A, M>: Send + Sync {
    /// Build metadata from request extension payload and authenticator output.
    fn new_from_extension(&self, auth_info: &A, extension: &str) -> Result<M>;

    /// Build metadata from llm-api http headers.
    fn new_from_http_headers(&self, headers: &HeaderMap) -> Result<M>;
}

/// Default metadata manager.
pub struct MetadataMgrImpl {}

impl MetadataMgrImpl {
    /// Creates a default no-op metadata manager.
    pub fn new() -> Self {
        Self {}
    }
}

impl<A> MetadataMgr<A, ()> for MetadataMgrImpl {
    fn new_from_extension(&self, _auth_info: &A, _extension: &str) -> Result<()> {
        Ok(())
    }

    fn new_from_http_headers(&self, _headers: &HeaderMap) -> Result<()> {
        Ok(())
    }
}
