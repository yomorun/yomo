use std::collections::HashMap;

use anyhow::Result;
use tokio::sync::RwLock;

/// Stores and queries tool function definitions.
#[async_trait::async_trait]
pub trait ToolMgr<A, M>: Send + Sync {
    /// Inserts or updates a tool definition.
    ///
    /// `schema` is expected to be a JSON object string that contains at least
    /// `description` and `parameters` fields for LLM tool registration.
    async fn upsert_tool(&self, tool_name: String, schema: String, auth_info: &A) -> Result<()>;

    /// Lists available tools for the provided metadata scope.
    ///
    /// Returns a map keyed by tool name with raw JSON schema string values.
    async fn list_tools(&self, metadata: &M) -> Result<HashMap<String, String>>;
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
impl<A, M> ToolMgr<A, M> for ToolMgrImpl {
    async fn upsert_tool(&self, tool_name: String, schema: String, _auth_info: &A) -> Result<()> {
        self.tools.write().await.insert(tool_name, schema);
        Ok(())
    }

    async fn list_tools(&self, _metadata: &M) -> Result<HashMap<String, String>> {
        Ok(self.tools.read().await.to_owned())
    }
}
