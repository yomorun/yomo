use std::collections::HashMap;

use anyhow::Result;

/// Metadata key/value pairs used for route and tool filtering.
pub type Metadata = HashMap<String, String>;

/// Builds metadata from handshake auth info and request extension.
pub trait MetadataMgr: Send + Sync {
    /// Builds metadata from authenticator output.
    fn new_from_auth_info(&self, auth_info: &str) -> Result<Metadata>;
    /// Builds metadata from request extension payload.
    fn new_from_extension(&self, extension: &str) -> Result<Metadata>;
}

/// Default metadata manager.
pub struct MetadataMgrImpl {}

impl MetadataMgrImpl {
    /// Creates a default no-op metadata manager.
    pub fn new() -> Self {
        Self {}
    }
}

impl MetadataMgr for MetadataMgrImpl {
    fn new_from_auth_info(&self, _auth_info: &str) -> Result<Metadata> {
        Ok(Metadata::new())
    }

    fn new_from_extension(&self, _extension: &str) -> Result<Metadata> {
        Ok(Metadata::new())
    }
}
