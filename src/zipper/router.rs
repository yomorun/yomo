use std::collections::HashMap;

use anyhow::{Result, bail};

use crate::types::{HandshakeRequest, RequestHeaders};

/// Router trait for managing SFN routing
pub trait Router: Sync + Send {
    /// Handle SFN registration handshake
    fn handshake(&mut self, conn_id: u64, req: &HandshakeRequest) -> Result<Option<u64>>;

    /// Route request to appropriate SFN
    fn route(&self, headers: &RequestHeaders) -> Result<Option<u64>>;

    /// Remove disconnected SFN
    fn remove_sfn(&mut self, conn_id: u64);
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
        if req.sfn_name.is_empty() {
            bail!("sfn_name cannot be empty");
        }

        if let Some(token) = &self.auth_token {
            if &req.credential != token {
                bail!("invalid credential");
            }
        }

        Ok(self.route_map.insert(req.sfn_name.to_owned(), conn_id))
    }

    fn remove_sfn(&mut self, conn_id: u64) {
        self.route_map.retain(|_name, id| *id != conn_id);
    }

    fn route(&self, headers: &RequestHeaders) -> Result<Option<u64>> {
        if !headers.sfn_name.is_empty() {
            if let Some(conn_id) = self.route_map.get(&headers.sfn_name) {
                return Ok(Some(conn_id.to_owned()));
            }
        }
        Ok(None)
    }
}
