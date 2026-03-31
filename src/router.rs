use std::collections::HashMap;

use anyhow::{Result, bail};
use log::debug;
use tokio::sync::RwLock;

use crate::metadata::Metadata;

/// Router trait for managing routing
#[async_trait::async_trait]
pub trait Router: Sync + Send {
    /// Register a tool connection.
    ///
    /// Returns the previously registered connection id when the same name already exists.
    async fn register(&self, conn_id: u64, name: &str, metadata: &Metadata) -> Result<Option<u64>>;

    /// Route request to appropriate client.
    async fn route(&self, name: &str, metadata: &Metadata) -> Result<Option<u64>>;

    /// Remove disconnected client
    async fn remove(&self, conn_id: u64);
}

/// Router implementation
pub struct RouterImpl {
    route_map: RwLock<HashMap<String, u64>>,
}

impl RouterImpl {
    /// Create a new router instance.
    pub fn new() -> Self {
        Self {
            route_map: RwLock::default(),
        }
    }
}

#[async_trait::async_trait]
impl Router for RouterImpl {
    async fn register(
        &self,
        conn_id: u64,
        name: &str,
        _metadata: &Metadata,
    ) -> Result<Option<u64>> {
        if name.is_empty() {
            bail!("name cannot be empty");
        }

        Ok(self
            .route_map
            .write()
            .await
            .insert(name.to_owned(), conn_id))
    }

    async fn route(&self, name: &str, _metadata: &Metadata) -> Result<Option<u64>> {
        if name.is_empty() {
            return Ok(None);
        }

        if let Some(conn_id) = self.route_map.read().await.get(name) {
            debug!("route [{}] --> conn_id: {}", name, conn_id);
            return Ok(Some(*conn_id));
        }

        Ok(None)
    }

    async fn remove(&self, conn_id: u64) {
        self.route_map
            .write()
            .await
            .retain(|_key, id| *id != conn_id);
    }
}
