use std::collections::HashMap;

use anyhow::{Ok, Result, bail};
use axum::http::HeaderMap;
use log::{debug, error, info};

use crate::metadata::{RequestMetadata, SfnMetadata};

pub trait Middleware: Sync + Send {
    fn create_request_metadata(&self, headers: &HeaderMap) -> Result<RequestMetadata>;

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

#[derive(Default)]
pub(crate) struct MiddlewareImpl {
    route_map: HashMap<String, u64>,
}

impl Middleware for MiddlewareImpl {
    fn create_request_metadata(&self, headers: &HeaderMap) -> Result<RequestMetadata> {
        Ok(RequestMetadata {
            trace_id: match headers.get("x-trace-id") {
                Some(v) => v.to_str()?.to_owned(),
                None => String::new(),
            },
            req_id: match headers.get("x-req-id") {
                Some(v) => v.to_str()?.to_owned(),
                None => String::new(),
            },
            ..Default::default()
        })
    }

    fn handshake(
        &mut self,
        conn_id: u64,
        sfn_name: &str,
        credential: &str,
        _metadata: &SfnMetadata,
    ) -> Result<Option<u64>> {
        if credential != "" {
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
            "route: name={}, metadata={:?}, v={:?}",
            name, metadata, conn_id
        );

        Ok(match conn_id {
            Some(conn_id) => Some(conn_id.to_owned()),
            None => None,
        })
    }
}
