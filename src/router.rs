use std::collections::HashMap;

use anyhow::{Result, bail};
use log::debug;

use crate::types::{HandshakeRequest, RequestHeaders};

/// Router trait for managing routing
pub trait Router: Sync + Send {
    /// Handle client registration handshake
    fn handshake(&mut self, conn_id: u64, req: &HandshakeRequest) -> Result<Option<u64>>;

    /// Route request to appropriate client
    fn route(&self, headers: &RequestHeaders) -> Result<Option<u64>>;

    /// Remove disconnected client
    fn remove(&mut self, conn_id: u64);
}

/// Router implementation
pub struct RouterImpl {
    auth_token: Option<String>,

    route_map: HashMap<String, u64>,
}

impl RouterImpl {
    pub fn new(auth_token: Option<String>) -> Self {
        Self {
            auth_token,
            route_map: HashMap::new(),
        }
    }
}

impl Router for RouterImpl {
    fn handshake(&mut self, conn_id: u64, req: &HandshakeRequest) -> Result<Option<u64>> {
        if req.name.is_empty() {
            bail!("name cannot be empty");
        }

        if let Some(token) = &self.auth_token {
            if &req.credential != token {
                bail!("invalid credential");
            }
        }

        Ok(self.route_map.insert(req.name.to_owned(), conn_id))
    }

    fn remove(&mut self, conn_id: u64) {
        self.route_map.retain(|_name, id| *id != conn_id);
    }

    fn route(&self, headers: &RequestHeaders) -> Result<Option<u64>> {
        if !headers.name.is_empty() {
            if let Some(conn_id) = self.route_map.get(&headers.name) {
                debug!(
                    "[{}|{}] route [{}] --> conn_id: {}",
                    headers.trace_id, headers.span_id, headers.name, conn_id
                );
                return Ok(Some(conn_id.to_owned()));
            }
        }
        Ok(None)
    }
}
