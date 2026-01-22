use std::collections::HashMap;

use anyhow::Result;
use serde::Deserialize;

use crate::{metadata::Metadata, types::HandshakeReq};

#[derive(Debug, Clone, Deserialize, Default)]
pub struct ZipperMiddlewareImplConfig {
    #[serde(default)]
    auth_token: Option<String>,
}

pub trait ZipperMiddleware: Sync + Send {
    fn handshake(&mut self, conn_id: u64, req: &HandshakeReq) -> (bool, String, Option<u64>);

    fn route(&self, name: &str, metadata: &Box<dyn Metadata>) -> Result<Option<u64>>;

    fn remove_sfn(&mut self, conn_id: u64) -> Result<()>;
}

pub struct ZipperMiddlewareImpl {
    auth_token: Option<String>,

    route_map: HashMap<String, u64>,
}

impl ZipperMiddlewareImpl {
    pub fn new(config: ZipperMiddlewareImplConfig) -> Self {
        Self {
            auth_token: config.auth_token,
            route_map: HashMap::new(),
        }
    }
}

impl ZipperMiddleware for ZipperMiddlewareImpl {
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

    fn route(&self, name: &str, _metadata: &Box<dyn Metadata>) -> Result<Option<u64>> {
        Ok(match self.route_map.get(name) {
            Some(conn_id) => Some(conn_id.to_owned()),
            None => None,
        })
    }
}
