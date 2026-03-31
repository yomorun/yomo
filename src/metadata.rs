use std::collections::HashMap;

use anyhow::Result;

/// Metadata: key-value pairs
pub type Metadata = HashMap<String, String>;

/// MetadataMgr trait for managing metadata
pub trait MetadataMgr: Send + Sync {
    fn new_from_auth_info(&self, auth_info: &str) -> Result<Metadata>;
    fn new_from_extension(&self, extension: &str) -> Result<Metadata>;
}

pub struct MetadataMgrImpl {}

impl MetadataMgrImpl {
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
