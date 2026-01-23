use std::collections::HashMap;

use anyhow::Result;
use log::{info, warn};

use crate::types::{HandshakeReq, RequestHeaders};

pub trait Router: Sync + Send {
    fn handshake(&mut self, conn_id: u64, req: &HandshakeReq) -> (bool, String, Option<u64>);

    fn route(&self, headers: &RequestHeaders) -> Result<Option<u64>>;

    fn remove_sfn(&mut self, conn_id: u64) -> Result<()>;
}

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
    fn handshake(&mut self, conn_id: u64, req: &HandshakeReq) -> (bool, String, Option<u64>) {
        if req.sfn_name.is_empty() {
            return (false, "sfn name is empty".to_string(), None);
        }

        if let Some(token) = &self.auth_token {
            if &req.credential != token {
                return (false, "invalid credential".to_string(), None);
            }
        }

        let v = self.route_map.insert(req.sfn_name.to_owned(), conn_id);

        (true, String::new(), v)
    }

    fn remove_sfn(&mut self, conn_id: u64) -> Result<()> {
        self.route_map.retain(|_name, id| *id != conn_id);
        Ok(())
    }

    fn route(&self, headers: &RequestHeaders) -> Result<Option<u64>> {
        Ok(match self.route_map.get(&headers.sfn_name) {
            Some(conn_id) => {
                info!("route for [{}] to: {}", headers.sfn_name, conn_id);
                Some(conn_id.to_owned())
            }
            None => {
                warn!("route for [{}] not found", headers.sfn_name);
                None
            }
        })
    }
}
