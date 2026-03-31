use std::collections::HashMap;

use anyhow::Result;
use tokio::sync::RwLock;

use crate::metadata::Metadata;

/// Tool Manager: persist tool function definitions
#[async_trait::async_trait]
pub trait ToolMgr: Send + Sync {
    async fn upsert_tool(
        &self,
        tool_name: String,
        schema: String,
        metadata: &Metadata,
    ) -> Result<()>;
    async fn list_tools(&self, metadata: &Metadata) -> Result<HashMap<String, String>>;
}

#[derive(Default)]
pub struct ToolMgrImpl {
    tools: RwLock<HashMap<String, String>>,
}

impl ToolMgrImpl {
    pub fn new() -> Self {
        Self::default()
    }
}

#[async_trait::async_trait]
impl ToolMgr for ToolMgrImpl {
    async fn upsert_tool(
        &self,
        tool_name: String,
        schema: String,
        _metadata: &Metadata,
    ) -> Result<()> {
        self.tools.write().await.insert(tool_name, schema);
        Ok(())
    }

    async fn list_tools(&self, _metadata: &Metadata) -> Result<HashMap<String, String>> {
        Ok(self.tools.read().await.to_owned())
    }
}
