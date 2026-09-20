use async_trait::async_trait;
use axum::http::{HeaderMap, HeaderValue, header};
use std::sync::Arc;

use crate::model_api_provider::provider::{
    ModelApiProvider, ProviderRequest, ProviderResponse, proxy_request,
};
use crate::serve_config::{ConfigError, ProviderConfig};

#[derive(Clone)]
pub struct TypeSafeSystemOneClient {
    client: reqwest::Client,
    base_url: String,
    auth_headers: HeaderMap,
    model_id: String,
    upstream_model: String,
}

impl TypeSafeSystemOneClient {
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
impl<M> ModelApiProvider<M> for TypeSafeSystemOneClient {
    fn model_id(&self) -> &str {
        &self.model_id
    }

    async fn execute(
        &self,
        mut req: ProviderRequest,
        _metadata: &M,
    ) -> Result<ProviderResponse, anyhow::Error> {
        req.endpoint_path = "/systemone".to_string();
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

pub fn build_client<M>(
    provider: &ProviderConfig,
) -> Result<Arc<dyn ModelApiProvider<M>>, ConfigError> {
    let api_key = provider
        .params
        .get("api_key")
        .ok_or_else(|| ConfigError::InvalidProvider("api_key is required".to_string()))?;
    let base_url = provider
        .params
        .get("base_url")
        .cloned()
        .unwrap_or_else(|| "https://api.typesafe.ai/v1".to_string());
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

    Ok(Arc::new(TypeSafeSystemOneClient::new(
        reqwest::Client::new(),
        base_url,
        headers,
        provider.model_id.clone(),
        upstream_model,
    )))
}

#[cfg(test)]
mod tests {
    use super::build_client;
    use crate::serve_config::ProviderConfig;
    use std::collections::HashMap;

    #[test]
    fn build_client_accepts_defaults_and_required_fields() {
        let mut params = HashMap::new();
        params.insert("api_key".to_string(), "ts-test".to_string());
        params.insert("model".to_string(), "jev-latest".to_string());
        let provider = ProviderConfig {
            provider_type: "typesafe-systemone".to_string(),
            model_id: "jev-latest".to_string(),
            label: None,
            params,
        };

        let client = build_client::<()>(&provider).expect("typesafe systemone client should build");

        assert_eq!(client.model_id(), "jev-latest");
    }

    #[test]
    fn build_client_rejects_missing_api_key() {
        let mut params = HashMap::new();
        params.insert("model".to_string(), "jev-latest".to_string());
        let provider = ProviderConfig {
            provider_type: "typesafe-systemone".to_string(),
            model_id: "jev-latest".to_string(),
            label: None,
            params,
        };

        let err = build_client::<()>(&provider)
            .err()
            .expect("missing api_key should fail");

        assert_eq!(err.to_string(), "invalid provider: api_key is required");
    }

    #[test]
    fn build_client_rejects_missing_model() {
        let mut params = HashMap::new();
        params.insert("api_key".to_string(), "ts-test".to_string());
        let provider = ProviderConfig {
            provider_type: "typesafe-systemone".to_string(),
            model_id: "jev-latest".to_string(),
            label: None,
            params,
        };

        let err = build_client::<()>(&provider)
            .err()
            .expect("missing model should fail");

        assert_eq!(err.to_string(), "invalid provider: model is required");
    }
}
