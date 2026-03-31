use std::collections::HashMap;

use anyhow::Result;
use tokio::sync::RwLock;

use crate::metadata::Metadata;

/// Stores and queries tool function definitions.
#[async_trait::async_trait]
pub trait ToolMgr: Send + Sync {
    /// Inserts or updates a tool definition.
    ///
    /// `schema` is expected to be a JSON object string that contains at least
    /// `description` and `parameters` fields for LLM tool registration.
    async fn upsert_tool(
        &self,
        tool_name: String,
        schema: String,
        metadata: &Metadata,
    ) -> Result<()>;

    /// Lists available tools for the provided metadata scope.
    ///
    /// Returns a map keyed by tool name with raw JSON schema string values.
    async fn list_tools(&self, metadata: &Metadata) -> Result<HashMap<String, String>>;
}

/// In-memory tool manager.
pub struct ToolMgrImpl {
    tools: RwLock<HashMap<String, String>>,
}

impl ToolMgrImpl {
    pub fn new() -> Self {
        Self {
            tools: RwLock::default(),
        }
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
