use async_trait::async_trait;
use axum::http::{HeaderMap, HeaderValue, header};
use std::sync::Arc;

use crate::provider::{
    HttpProviderRequest, HttpProviderResponse, Provider, ProviderError, proxy_request,
};
use crate::serve_config::{ConfigError, ProviderConfig};

#[derive(Clone)]
pub struct ProxyClient {
    client: reqwest::Client,
    base_url: String,
    auth_headers: HeaderMap,
    upstream_model: String,
}

impl ProxyClient {
    pub fn new(
        client: reqwest::Client,
        base_url: String,
        auth_headers: HeaderMap,
        upstream_model: String,
    ) -> Self {
        Self {
            client,
            base_url,
            auth_headers,
            upstream_model,
        }
    }
}

#[async_trait]
impl<M: Sync> Provider<M> for ProxyClient {
    async fn http(
        &self,
        req: HttpProviderRequest,
        _metadata: &M,
    ) -> Result<HttpProviderResponse, ProviderError> {
        proxy_request(
            &self.client,
            &self.base_url,
            self.auth_headers.clone(),
            Some(self.upstream_model.as_str()),
            req,
        )
        .await
    }
}

pub fn build_client<M: Sync>(
    provider: &ProviderConfig,
) -> Result<Arc<dyn Provider<M>>, ConfigError> {
    let api_key = provider
        .params
        .get("api_key")
        .ok_or_else(|| ConfigError::InvalidProvider("api_key is required".to_string()))?;
    let base_url = provider
        .params
        .get("base_url")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("base_url is required".to_string()))?;
    let upstream_model = provider
        .params
        .get("model")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("model is required".to_string()))?;
    let mut headers = HeaderMap::new();
    let auth_value = format!("Bearer {}", api_key);
    headers.insert(
        header::AUTHORIZATION,
        auth_value
            .parse::<HeaderValue>()
            .map_err(|err| ConfigError::InvalidProvider(err.to_string()))?,
    );
    Ok(Arc::new(ProxyClient::new(
        reqwest::Client::new(),
        base_url,
        headers,
        upstream_model,
    )))
}
