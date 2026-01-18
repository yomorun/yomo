use anyhow::Result;
use axum::http::HeaderMap;

use crate::{bridge::http::metadata::HttpMetadata, metadata::Metadata};

pub trait HttpMiddleware: Sync + Send {
    fn new_metadata(&self, headers: &HeaderMap) -> Result<Box<dyn Metadata>>;
}

#[derive(Default)]
pub struct HttpMiddlewareImpl;

impl HttpMiddleware for HttpMiddlewareImpl {
    fn new_metadata(&self, headers: &HeaderMap) -> Result<Box<dyn Metadata>> {
        Ok(Box::new(HttpMetadata::new(headers)?))
    }
}
