use std::{collections::HashMap, sync::Arc};

use anyhow::Result;
use serde_json::{Value, json};
use tokio::sync::RwLock;

#[derive(Debug, Clone)]
pub struct ToolFunction {
    pub tool_name: String,
    pub description: String,
    pub parameters: Value,
}

impl ToolFunction {
    pub fn to_openai_tool(&self) -> Value {
        json!({
            "type": "function",
            "function": {
                "name": self.tool_name,
                "description": self.description,
                "parameters": self.parameters,
            }
        })
    }
}

/// Tool Manager: persist tool function definitions
#[async_trait::async_trait]
pub trait ToolMgr: Send + Sync {
    async fn upsert_tool(&self, tool: ToolFunction) -> Result<()>;
    async fn list_tools(&self) -> Result<Vec<ToolFunction>>;
}

#[derive(Clone, Default)]
pub struct ToolMgrImpl {
    tools: Arc<RwLock<HashMap<String, ToolFunction>>>,
}

impl ToolMgrImpl {
    pub fn new() -> Self {
        Self::default()
    }
}

#[async_trait::async_trait]
impl ToolMgr for ToolMgrImpl {
    async fn upsert_tool(&self, tool: ToolFunction) -> Result<()> {
        self.tools
            .write()
            .await
            .insert(tool.tool_name.to_owned(), tool);
        Ok(())
    }

    async fn list_tools(&self) -> Result<Vec<ToolFunction>> {
        Ok(self.tools.read().await.values().cloned().collect())
    }
}
