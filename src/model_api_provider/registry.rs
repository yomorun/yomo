use std::collections::HashMap;
use std::sync::Arc;

use anyhow::Error;
use async_trait::async_trait;

use crate::model_api_provider::ModelApiProvider;
use crate::model_api_provider::ProviderRequest;
use crate::model_api_provider::ProviderResponse;
use crate::model_api_provider::providers;
use crate::model_api_provider::selection::{SelectionError, SelectionResult, SelectionStrategy};
use crate::provider_error_notifier::{ProviderErrorEvent, ProviderErrorNotifier};
use crate::serve_config::{ConfigError, ModelApiConfig, ModelApiEndpointConfig, ProviderConfig};

#[derive(Clone)]
pub struct ProviderEntry<M> {
    pub model_id: String,
    pub label: Option<String>,
    pub provider: Arc<dyn ModelApiProvider<M>>,
}

#[derive(Clone)]
pub struct ProviderRegistry<M> {
    providers: HashMap<String, HashMap<String, ProviderEntry<M>>>,
    endpoints: HashMap<String, ModelApiEndpointConfig>,
    strategy: Arc<dyn SelectionStrategy<M>>,
    error_notifier: Option<Arc<dyn ProviderErrorNotifier<M>>>,
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
            error_notifier: None,
        })
    }

    pub fn with_error_notifier(mut self, notifier: Arc<dyn ProviderErrorNotifier<M>>) -> Self {
        self.error_notifier = Some(notifier);
        self
    }

    pub fn select(
        &self,
        endpoint: &str,
        model_id: Option<&str>,
        metadata: &M,
    ) -> Result<ProviderEntry<M>, SelectionError>
    where
        M: Clone + Send + Sync + 'static,
    {
        let selected = self
            .strategy
            .select(endpoint, model_id, metadata)
            .map_err(|err| err)?;
        let mut provider = self
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
        if let Some(notifier) = &self.error_notifier {
            provider.provider = Arc::new(HookedProvider {
                inner: Arc::clone(&provider.provider),
                error_notifier: Arc::clone(notifier),
                model_id: provider.model_id.clone(),
            });
        }
        Ok(provider)
    }

    pub fn notify_http_error(
        &self,
        endpoint: &str,
        model: &str,
        metadata: &M,
        status: u16,
        error: String,
    ) where
        M: Clone,
    {
        let Some(notifier) = &self.error_notifier else {
            return;
        };
        notifier.notify_provider_error(ProviderErrorEvent {
            model: Some(model.to_string()),
            metadata: metadata.clone(),
            http_status: Some(status),
            error,
            endpoint: Some(endpoint.to_string()),
        });
    }

    pub fn endpoint(&self, path: &str) -> Option<&ModelApiEndpointConfig> {
        self.endpoints.get(path)
    }

    pub fn model_list(&self) -> Vec<String> {
        let mut models = std::collections::HashSet::new();
        for endpoint_models in self.providers.values() {
            for provider in endpoint_models.values() {
                models.insert(provider.model_id.clone());
            }
        }
        models.into_iter().collect()
    }
}

#[derive(Clone)]
struct HookedProvider<M> {
    inner: Arc<dyn ModelApiProvider<M>>,
    error_notifier: Arc<dyn ProviderErrorNotifier<M>>,
    model_id: String,
}

#[async_trait]
impl<M> ModelApiProvider<M> for HookedProvider<M>
where
    M: Clone + Send + Sync + 'static,
{
    fn model_id(&self) -> &str {
        self.inner.model_id()
    }

    async fn execute(&self, req: ProviderRequest, metadata: &M) -> Result<ProviderResponse, Error> {
        let endpoint = req.endpoint_path.clone();
        self.inner
            .execute(req, metadata)
            .await
            .map_err(|err| self.notify_error(endpoint.as_str(), metadata, err))
    }
}

impl<M> HookedProvider<M>
where
    M: Clone + Send + Sync + 'static,
{
    fn notify_error(&self, endpoint: &str, metadata: &M, err: Error) -> Error {
        self.error_notifier
            .notify_provider_error(ProviderErrorEvent {
                model: Some(self.model_id.clone()),
                metadata: metadata.clone(),
                http_status: None,
                error: err.to_string(),
                endpoint: Some(endpoint.to_string()),
            });
        err
    }
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use std::sync::Arc;

    use anyhow::anyhow;
    use async_trait::async_trait;
    use axum::http::{HeaderMap, Method, StatusCode};

    use std::sync::Mutex;

    use super::{ProviderEntry, ProviderRegistry};
    use crate::model_api_provider::selection::{SelectionResult, SelectionStrategy};
    use crate::model_api_provider::{
        ModelApiProvider, ProviderBody, ProviderRequest, ProviderResponse,
    };
    use crate::provider_error_notifier::{ProviderErrorEvent, ProviderErrorNotifier};

    struct SelectModel;

    impl SelectionStrategy<()> for SelectModel {
        fn select(
            &self,
            _endpoint: &str,
            _model_id: Option<&str>,
            _metadata: &(),
        ) -> Result<SelectionResult, crate::model_api_provider::SelectionError> {
            Ok(SelectionResult {
                model_id: "embed-1".to_string(),
            })
        }
    }

    struct FailingProvider;

    #[async_trait]
    impl ModelApiProvider<()> for FailingProvider {
        fn model_id(&self) -> &str {
            "embed-1"
        }

        async fn execute(
            &self,
            _req: ProviderRequest,
            _metadata: &(),
        ) -> Result<ProviderResponse, anyhow::Error> {
            Err(anyhow!("upstream timeout"))
        }
    }

    struct CollectNotifier {
        calls: Mutex<Vec<ProviderErrorEvent<()>>>,
    }

    impl ProviderErrorNotifier<()> for CollectNotifier {
        fn notify_provider_error(&self, event: ProviderErrorEvent<()>) {
            self.calls.lock().expect("lock notifier calls").push(event);
        }
    }

    #[tokio::test]
    async fn provider_registry_error_notifier_receives_model_api_error() {
        let mut endpoint_models = HashMap::new();
        endpoint_models.insert(
            "embed-1".to_string(),
            ProviderEntry {
                model_id: "embed-1".to_string(),
                label: Some("embed".to_string()),
                provider: Arc::new(FailingProvider),
            },
        );

        let mut providers = HashMap::new();
        providers.insert("/embeddings".to_string(), endpoint_models);

        let mut endpoints = HashMap::new();
        endpoints.insert(
            "/embeddings".to_string(),
            crate::serve_config::ModelApiEndpointConfig {
                path: "/embeddings".to_string(),
                default_model: Some("embed-1".to_string()),
                models: vec!["embed-1".to_string()],
            },
        );

        let notifier = Arc::new(CollectNotifier {
            calls: Mutex::new(Vec::new()),
        });
        let registry = ProviderRegistry {
            providers,
            endpoints,
            strategy: Arc::new(SelectModel),
            error_notifier: Some(Arc::clone(&notifier) as Arc<dyn ProviderErrorNotifier<()>>),
        };

        let entry = registry
            .select("/embeddings", None, &())
            .expect("select provider");
        let err = match entry
            .provider
            .execute(
                ProviderRequest {
                    method: Method::POST,
                    endpoint_path: "/embeddings".to_string(),
                    headers: HeaderMap::new(),
                    body: axum::body::Bytes::new(),
                    is_stream: false,
                    content_type: None,
                },
                &(),
            )
            .await
        {
            Ok(_) => panic!("provider should fail"),
            Err(err) => err,
        };
        assert_eq!(err.to_string(), "upstream timeout");
        let calls = notifier.calls.lock().expect("lock calls");
        assert_eq!(calls.len(), 1);
        assert_eq!(calls[0].model.as_deref(), Some("embed-1"));
        assert_eq!(calls[0].http_status, None);
        assert_eq!(calls[0].endpoint.as_deref(), Some("/embeddings"));
    }

    struct SuccessProvider;

    #[async_trait]
    impl ModelApiProvider<()> for SuccessProvider {
        fn model_id(&self) -> &str {
            "embed-1"
        }

        async fn execute(
            &self,
            _req: ProviderRequest,
            _metadata: &(),
        ) -> Result<ProviderResponse, anyhow::Error> {
            Ok(ProviderResponse {
                status: StatusCode::OK,
                headers: HeaderMap::new(),
                body: ProviderBody::Full(axum::body::Bytes::new()),
            })
        }
    }

    #[tokio::test]
    async fn provider_registry_error_notifier_does_not_change_success_response() {
        let mut endpoint_models = HashMap::new();
        endpoint_models.insert(
            "embed-1".to_string(),
            ProviderEntry {
                model_id: "embed-1".to_string(),
                label: None,
                provider: Arc::new(SuccessProvider),
            },
        );
        let mut providers = HashMap::new();
        providers.insert("/embeddings".to_string(), endpoint_models);
        let mut endpoints = HashMap::new();
        endpoints.insert(
            "/embeddings".to_string(),
            crate::serve_config::ModelApiEndpointConfig {
                path: "/embeddings".to_string(),
                default_model: Some("embed-1".to_string()),
                models: vec!["embed-1".to_string()],
            },
        );

        let notifier = Arc::new(CollectNotifier {
            calls: Mutex::new(Vec::new()),
        });
        let registry = ProviderRegistry {
            providers,
            endpoints,
            strategy: Arc::new(SelectModel),
            error_notifier: Some(Arc::clone(&notifier) as Arc<dyn ProviderErrorNotifier<()>>),
        };

        let entry = registry
            .select("/embeddings", None, &())
            .expect("select provider");
        let response = entry
            .provider
            .execute(
                ProviderRequest {
                    method: Method::POST,
                    endpoint_path: "/embeddings".to_string(),
                    headers: HeaderMap::new(),
                    body: axum::body::Bytes::new(),
                    is_stream: false,
                    content_type: None,
                },
                &(),
            )
            .await
            .expect("provider should succeed");
        assert_eq!(response.status, StatusCode::OK);
        let calls = notifier.calls.lock().expect("lock calls");
        assert!(calls.is_empty());
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

fn build_providers<M>(
    providers: &[ProviderConfig],
    endpoints: &[ModelApiEndpointConfig],
) -> Result<HashMap<String, HashMap<String, ProviderEntry<M>>>, ConfigError> {
    let mut provider_map: HashMap<String, &ProviderConfig> = HashMap::new();
    for item in providers {
        provider_map.insert(item.model_id.clone(), item);
    }

    let mut registry: HashMap<String, HashMap<String, ProviderEntry<M>>> = HashMap::new();
    for endpoint in endpoints {
        let mut endpoint_models: HashMap<String, ProviderEntry<M>> = HashMap::new();
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
                label: provider_config.label.clone(),
                provider,
            };
            endpoint_models.insert(model_id.clone(), entry);
        }
        registry.insert(endpoint.path.clone(), endpoint_models);
    }

    Ok(registry)
}

fn build_provider<M>(
    provider: &ProviderConfig,
    endpoint_path: &str,
) -> Result<Arc<dyn ModelApiProvider<M>>, ConfigError> {
    match endpoint_path {
        "/messages" => match provider.provider_type.as_str() {
            "bedrock-messages" => providers::bedrock_messages::build_client(provider),
            "messages" => providers::messages::build_client(provider),
            other => Err(ConfigError::UnknownProviderType(other.to_string())),
        },
        "/responses" => providers::responses::build_client(provider),
        "/embeddings" => providers::passthrough::build_client(provider),
        "/rerank" => providers::passthrough::build_client(provider),
        "/audio/speech" => providers::passthrough::build_client(provider),
        "/audio/transcriptions" => providers::passthrough::build_client(provider),
        "/images/generations" => providers::passthrough::build_client(provider),
        "/images/edits" => providers::passthrough::build_client(provider),
        "/models/:generateContent" => providers::generate_content::build_client(provider),
        other => Err(ConfigError::InvalidProvider(format!(
            "unknown model_api endpoint: {}",
            other
        ))),
    }
}
