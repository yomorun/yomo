use std::collections::HashMap;

use anyhow::{Ok, Result, bail};
use axum::http::HeaderMap;
use log::{debug, error, info};

use crate::metadata::{RequestMetadata, SfnMetadata};

pub trait Middleware: Sync + Send {
    fn new_request_metadata(&self, headers: &HeaderMap) -> Result<RequestMetadata>;

    fn new_trace_id(&self, headers: &HeaderMap) -> Result<String> {
        Ok(match headers.get("x-trace-id") {
            Some(v) => v.to_str()?.to_owned(),
            None => String::new(),
        })
    }

    fn new_req_id(&self, headers: &HeaderMap) -> Result<String> {
        Ok(match headers.get("x-trace-id") {
            Some(v) => v.to_str()?.to_owned(),
            None => String::new(),
        })
    }

    fn handshake(
        &mut self,
        conn_id: u64,
        sfn_name: &str,
        credential: &str,
        metadata: &SfnMetadata,
    ) -> Result<Option<u64>>;

    fn route(&self, name: &str, metadata: &RequestMetadata) -> Result<Option<u64>>;

    fn remove_sfn(&mut self, conn_id: u64) -> Result<()>;
}

pub(crate) struct MiddlewareImpl {
    token: String,

    route_map: HashMap<String, u64>,
}

impl MiddlewareImpl {
    pub(crate) fn new(token: String) -> Self {
        MiddlewareImpl {
            token,
            route_map: HashMap::new(),
        }
    }
}
impl Middleware for MiddlewareImpl {
    fn new_request_metadata(&self, headers: &HeaderMap) -> Result<RequestMetadata> {
        let metadata = RequestMetadata {
            trace_id: self.new_trace_id(headers)?,
            req_id: self.new_req_id(headers)?,
            ..Default::default()
        };

        debug!("metadata: {:?}", metadata);

        Ok(metadata)
    }

    fn handshake(
        &mut self,
        conn_id: u64,
        sfn_name: &str,
        credential: &str,
        _metadata: &SfnMetadata,
    ) -> Result<Option<u64>> {
        if credential != self.token {
            error!("invalid credential");
            bail!("invalid credential");
        }

        let existed_conn_id = match self.route_map.get(sfn_name) {
            Some(conn_id) => Some(conn_id.to_owned()),
            None => None,
        };

        self.route_map.insert(sfn_name.to_owned(), conn_id);

        Ok(existed_conn_id)
    }

    fn remove_sfn(&mut self, conn_id: u64) -> Result<()> {
        info!("sfn unregistered: {}", conn_id);
        self.route_map.retain(|_name, id| *id != conn_id);
        Ok(())
    }

    fn route(&self, name: &str, metadata: &RequestMetadata) -> Result<Option<u64>> {
        let conn_id = self.route_map.get(name);
        debug!(
            "route: name={}, metadata={:?}, conn_id={:?}",
            name, metadata, conn_id
        );

        Ok(match conn_id {
            Some(conn_id) => Some(conn_id.to_owned()),
            None => None,
        })
    }
}
