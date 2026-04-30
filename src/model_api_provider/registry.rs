use std::collections::HashMap;
use std::sync::Arc;

use axum::http::HeaderMap;

use crate::model_api_provider::provider::ProxyClient;
use crate::model_api_provider::selection::{SelectionError, SelectionResult, SelectionStrategy};
use crate::model_api_provider::ModelApiProvider;
use crate::serve_config::{ConfigError, ModelApiConfig, ModelApiEndpointConfig, ProviderConfig};

#[derive(Clone)]
pub struct ProviderEntry {
    pub model_id: String,
    pub provider: Arc<dyn ModelApiProvider>,
}

pub struct ProviderRegistry<M> {
    providers: HashMap<String, HashMap<String, ProviderEntry>>,
    endpoints: HashMap<String, ModelApiEndpointConfig>,
    strategy: Arc<dyn SelectionStrategy<M>>,
}

impl<M> ProviderRegistry<M> {
    pub fn from_config(
        config: &ModelApiConfig,
        strategy: Arc<dyn SelectionStrategy<M>>,
    ) -> Result<Self, ConfigError> {
        let providers = build_providers(&config.providers, &config.endpoints)?;
        let endpoints = config
            .endpoints
            .iter()
            .map(|endpoint| (endpoint.path.clone(), endpoint.clone()))
            .collect();
        Ok(Self {
            providers,
            endpoints,
            strategy,
        })
    }

    pub fn select(
        &self,
        endpoint: &str,
        model_id: Option<&str>,
        metadata: &M,
    ) -> Result<(SelectionResult, ProviderEntry), SelectionError> {
        let selected = self
            .strategy
            .select(endpoint, model_id, metadata)
            .map_err(|err| err)?;
        let provider = self
            .providers
            .get(endpoint)
            .and_then(|endpoint_models| {
                endpoint_models
                    .values()
                    .find(|provider| {
                        provider.model_id.to_ascii_lowercase()
                            == selected.model_id.to_ascii_lowercase()
                    })
                    .cloned()
            })
            .ok_or(SelectionError::ModelNotSupported)?;
        Ok((selected, provider))
    }

    pub fn endpoint(&self, path: &str) -> Option<&ModelApiEndpointConfig> {
        self.endpoints.get(path)
    }
}

pub struct ByEndpointModel {
    endpoints: HashMap<String, ModelApiEndpointConfig>,
}

impl ByEndpointModel {
    pub fn new(endpoints: HashMap<String, ModelApiEndpointConfig>) -> Self {
        Self { endpoints }
    }
}

impl<M> SelectionStrategy<M> for ByEndpointModel {
    fn select(
        &self,
        endpoint: &str,
        model_id: Option<&str>,
        _metadata: &M,
    ) -> Result<SelectionResult, SelectionError> {
        if let Some(model) = model_id.filter(|value| !value.trim().is_empty()) {
            return Ok(SelectionResult {
                model_id: model.to_string(),
            });
        }
        let endpoint = self.endpoints.get(endpoint);
        if let Some(endpoint) = endpoint {
            if let Some(default_model) = &endpoint.default_model {
                if !default_model.trim().is_empty() {
                    return Ok(SelectionResult {
                        model_id: default_model.clone(),
                    });
                }
            }
        }
        Err(SelectionError::ModelNotSupported)
    }
}

fn build_providers(
    providers: &[ProviderConfig],
    endpoints: &[ModelApiEndpointConfig],
) -> Result<HashMap<String, HashMap<String, ProviderEntry>>, ConfigError> {
    let mut provider_map: HashMap<String, &ProviderConfig> = HashMap::new();
    for item in providers {
        provider_map.insert(item.model_id.clone(), item);
    }

    let mut registry: HashMap<String, HashMap<String, ProviderEntry>> = HashMap::new();
    for endpoint in endpoints {
        let mut endpoint_models: HashMap<String, ProviderEntry> = HashMap::new();
        let mut model_ids = endpoint.models.clone();
        if let Some(default_model) = &endpoint.default_model {
            if !model_ids.iter().any(|model| model == default_model) {
                model_ids.push(default_model.clone());
            }
        }
        for model_id in model_ids.iter() {
            let provider_config = provider_map.get(model_id).ok_or_else(|| {
                ConfigError::InvalidProvider(format!(
                    "model_api endpoint model not found: {}",
                    model_id
                ))
            })?;
            let provider = build_provider(provider_config, endpoint.path.as_str())?;
            let entry = ProviderEntry {
                model_id: provider_config.model_id.clone(),
                provider,
            };
            endpoint_models.insert(model_id.clone(), entry);
        }
        registry.insert(endpoint.path.clone(), endpoint_models);
    }

    Ok(registry)
}

fn build_provider(
    provider: &ProviderConfig,
    endpoint_path: &str,
) -> Result<Arc<dyn ModelApiProvider>, ConfigError> {
    match endpoint_path {
        "/messages" => build_anthropic_client(provider),
        "/responses" => build_openai_client(provider),
        "/embeddings" => build_openai_client(provider),
        "/rerank" => build_openai_client(provider),
        "/audio/speech" => build_openai_client(provider),
        "/audio/transcriptions" => build_openai_client(provider),
        "/images/generations" => build_openai_client(provider),
        "/images/edits" => build_openai_client(provider),
        other => Err(ConfigError::InvalidProvider(format!(
            "unknown model_api endpoint: {}",
            other
        ))),
    }
}

fn build_openai_client(provider: &ProviderConfig) -> Result<Arc<dyn ModelApiProvider>, ConfigError> {
    if provider.provider_type != "openai" {
        return Err(ConfigError::UnknownProviderType(provider.provider_type.clone()));
    }
    let api_key = provider
        .params
        .get("api_key")
        .ok_or_else(|| ConfigError::InvalidProvider("api_key is required".to_string()))?;
    let base_url = provider
        .params
        .get("base_url")
        .cloned()
        .unwrap_or_else(|| "https://api.openai.com/v1".to_string());
    let mut headers = HeaderMap::new();
    let auth_value = format!("Bearer {}", api_key);
    headers.insert(
        axum::http::header::AUTHORIZATION,
        auth_value
            .parse::<axum::http::HeaderValue>()
            .map_err(|err| ConfigError::InvalidProvider(err.to_string()))?,
    );
    Ok(Arc::new(ProxyClient::new(
        reqwest::Client::new(),
        base_url,
        headers,
        provider.model_id.clone(),
    )))
}

fn build_anthropic_client(
    provider: &ProviderConfig,
) -> Result<Arc<dyn ModelApiProvider>, ConfigError> {
    if provider.provider_type != "anthropic" {
        return Err(ConfigError::UnknownProviderType(provider.provider_type.clone()));
    }
    let api_key = provider
        .params
        .get("api_key")
        .ok_or_else(|| ConfigError::InvalidProvider("api_key is required".to_string()))?;
    let base_url = provider
        .params
        .get("base_url")
        .cloned()
        .unwrap_or_else(|| "https://api.anthropic.com/v1".to_string());
    let version = provider
        .params
        .get("anthropic_version")
        .cloned()
        .unwrap_or_else(|| "2023-06-01".to_string());
    let mut headers = HeaderMap::new();
    headers.insert(
        "x-api-key",
        api_key
            .parse::<axum::http::HeaderValue>()
            .map_err(|err| ConfigError::InvalidProvider(err.to_string()))?,
    );
    headers.insert(
        "anthropic-version",
        version
            .parse::<axum::http::HeaderValue>()
            .map_err(|err| ConfigError::InvalidProvider(err.to_string()))?,
    );
    Ok(Arc::new(ProxyClient::new(
        reqwest::Client::new(),
        base_url,
        headers,
        provider.model_id.clone(),
    )))
}
