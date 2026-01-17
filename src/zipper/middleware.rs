use std::collections::HashMap;

use anyhow::{Ok, Result, bail};
use axum::http::HeaderMap;
use nanoid::nanoid;

use crate::{
    metadata::{RequestMetadata, SfnMetadata},
    zipper::config::MiddlewareConfig,
};

pub trait Middleware: Sync + Send {
    fn new_request_metadata(&self, headers: &HeaderMap) -> Result<RequestMetadata>;

    fn new_trace_id(&self, _headers: &HeaderMap) -> Result<String> {
        Ok(nanoid!(12))
    }

    fn new_req_id(&self, _headers: &HeaderMap) -> Result<String> {
        Ok(nanoid!(8))
    }

    fn handshake(
        &mut self,
        conn_id: u64,
        sfn_name: String,
        credential: Option<String>,
        metadata: SfnMetadata,
    ) -> Result<Option<u64>>;

    fn route(&self, name: &str, metadata: &RequestMetadata) -> Result<Option<u64>>;

    fn remove_sfn(&mut self, conn_id: u64) -> Result<()>;
}

pub struct DefaultMiddleware {
    auth_token: Option<String>,

    route_map: HashMap<String, u64>,
}

impl DefaultMiddleware {
    pub fn new(config: MiddlewareConfig) -> Self {
        Self {
            auth_token: config.auth_token,
            route_map: HashMap::new(),
        }
    }
}

impl Middleware for DefaultMiddleware {
    fn new_request_metadata(&self, headers: &HeaderMap) -> Result<RequestMetadata> {
        Ok(RequestMetadata {
            trace_id: self.new_trace_id(headers)?,
            req_id: self.new_req_id(headers)?,
            ..Default::default()
        })
    }

    fn handshake(
        &mut self,
        conn_id: u64,
        sfn_name: String,
        credential: Option<String>,
        _metadata: SfnMetadata,
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

        let existed_conn_id = match self.route_map.get(&sfn_name) {
            Some(conn_id) => Some(conn_id.to_owned()),
            None => None,
        };

        self.route_map.insert(sfn_name, conn_id);

        Ok(existed_conn_id)
    }

    fn remove_sfn(&mut self, conn_id: u64) -> Result<()> {
        self.route_map.retain(|_name, id| *id != conn_id);
        Ok(())
    }

    fn route(&self, name: &str, _metadata: &RequestMetadata) -> Result<Option<u64>> {
        Ok(match self.route_map.get(name) {
            Some(conn_id) => Some(conn_id.to_owned()),
            None => None,
        })
    }
}
