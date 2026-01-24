use std::collections::HashMap;

use anyhow::{Result, bail};
use log::{info, warn};

use crate::types::{HandshakeReq, RequestHeaders};

/// Router trait for managing SFN routing
pub trait Router: Sync + Send {
    /// Handle SFN registration handshake
    fn handshake(&mut self, conn_id: u64, req: &HandshakeReq) -> Result<Option<u64>>;

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
    fn handshake(&mut self, conn_id: u64, req: &HandshakeReq) -> Result<Option<u64>> {
        if req.sfn_name.is_empty() {
            bail!("sfn name is empty");
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
        Ok(match self.route_map.get(&headers.sfn_name) {
            Some(conn_id) => {
                info!(
                    "[{}|{}] route for [{}]: conn_id={}",
                    headers.trace_id, headers.request_id, headers.sfn_name, conn_id
                );
                Some(conn_id.to_owned())
            }
            None => {
                warn!(
                    "[{}|{}] route for [{}]: not found",
                    headers.trace_id, headers.request_id, headers.sfn_name
                );
                None
            }
        })
    }
}
