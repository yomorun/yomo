use std::collections::HashMap;

use anyhow::{Ok, Result, bail};

use crate::{metadata::Metadata, zipper::config::ZipperMiddlewareImplConfig};

pub trait ZipperMiddleware: Sync + Send {
    fn handshake(
        &mut self,
        conn_id: u64,
        sfn_name: &str,
        credential: Option<String>,
    ) -> Result<Option<u64>>;

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
    fn handshake(
        &mut self,
        conn_id: u64,
        sfn_name: &str,
        credential: Option<String>,
    ) -> Result<Option<u64>> {
        if sfn_name.is_empty() {
            bail!("sfn name is empty");
        }

        if let Some(token) = &self.auth_token {
            if let Some(c) = &credential {
                if c != token {
                    bail!("credential mismatch");
                }
            } else {
                bail!("credential is empty");
            }
        }

        let existed_conn_id = match self.route_map.get(sfn_name) {
            Some(conn_id) => Some(conn_id.to_owned()),
            None => None,
        };

        self.route_map.insert(sfn_name.to_owned(), conn_id);

        Ok(existed_conn_id)
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
