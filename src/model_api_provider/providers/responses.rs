use async_trait::async_trait;
use axum::http::HeaderMap;
use std::sync::Arc;

use crate::model_api_provider::provider::{
    ModelApiProvider, ProviderRequest, ProviderResponse, proxy_request,
};
use crate::serve_config::{ConfigError, ProviderConfig};

#[derive(Clone)]
pub struct ResponsesClient {
    client: reqwest::Client,
    base_url: String,
    auth_headers: HeaderMap,
    model_id: String,
    upstream_model: String,
}

impl ResponsesClient {
    pub fn new(
        client: reqwest::Client,
        base_url: String,
        auth_headers: HeaderMap,
        model_id: String,
        upstream_model: String,
    ) -> Self {
        Self {
            client,
            base_url,
            auth_headers,
            model_id,
            upstream_model,
        }
    }
}

#[async_trait]
impl ModelApiProvider for ResponsesClient {
    fn model_id(&self) -> &str {
        &self.model_id
    }

    async fn execute(&self, mut req: ProviderRequest) -> Result<ProviderResponse, anyhow::Error> {
        req.endpoint_path = "/responses".to_string();
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

pub fn build_client(provider: &ProviderConfig) -> Result<Arc<dyn ModelApiProvider>, ConfigError> {
    if provider.provider_type != "responses" {
        return Err(ConfigError::UnknownProviderType(
            provider.provider_type.clone(),
        ));
    }
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
        axum::http::header::AUTHORIZATION,
        auth_value
            .parse::<axum::http::HeaderValue>()
            .map_err(|err| ConfigError::InvalidProvider(err.to_string()))?,
    );

    Ok(Arc::new(ResponsesClient::new(
        reqwest::Client::new(),
        base_url,
        headers,
        provider.model_id.clone(),
        upstream_model,
    )))
}
