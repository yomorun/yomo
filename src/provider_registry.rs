use std::collections::{HashMap, HashSet};
use std::pin::Pin;
use std::sync::Arc;

use async_trait::async_trait;
use futures_core::Stream;

use crate::endpoint_api::EndpointKind;
use crate::openai_types::ChatCompletionRequest;
use crate::provider::anthropic_messages::{
    build_anthropic_messages_provider, build_bedrock_messages_provider,
};
use crate::provider::openai::build_openai_provider;
use crate::provider::openai_compatible::build_openai_compatible_provider;
use crate::provider::providers;
use crate::provider::tokenhub::build_tokenhub_provider;
use crate::provider::vertexai::build_vertexai_provider;
use crate::provider::vllm_deepseek::build_vllm_deepseek_provider;
use crate::provider::{
    HttpProviderRequest, HttpProviderResponse, Provider, ProviderError, UnifiedEvent,
    UnifiedResponse,
};
use crate::provider_error_notifier::{ProviderErrorEvent, ProviderErrorNotifier};
use crate::serve_config::{ConfigError, EndpointConfig, ProviderConfig};

#[derive(Clone, Debug)]
pub struct SelectionResult {
    pub model_id: String,
}

#[derive(Debug)]
pub enum SelectionError {
    ModelRequired,
    ModelNotSupported { model: String },
    OutstandingBalance,
    AccessDenied,
}

/// Selects request models and visible model lists for endpoints.
pub trait SelectionStrategy<M>: Send + Sync {
    fn select(
        &self,
        endpoint: EndpointKind,
        model_id: Option<&str>,
        metadata: &M,
    ) -> Result<SelectionResult, SelectionError>;

    fn list_models(&self, models: Vec<String>, metadata: &M) -> Vec<String> {
        let _ = metadata;
        models
    }
}

/// Builds a custom provider from configuration for one endpoint.
pub type ProviderBuilder<M> = Arc<
    dyn Fn(&ProviderConfig, EndpointKind) -> Result<Option<Arc<dyn Provider<M>>>, ConfigError>
        + Send
        + Sync,
>;

#[derive(Clone)]
pub struct ByEndpointModel {
    endpoints: HashMap<EndpointKind, EndpointConfig>,
}

impl ByEndpointModel {
    pub fn new(endpoints: HashMap<EndpointKind, EndpointConfig>) -> Self {
        Self { endpoints }
    }
}

impl<M> SelectionStrategy<M> for ByEndpointModel {
    fn select(
        &self,
        endpoint: EndpointKind,
        model_id: Option<&str>,
        _metadata: &M,
    ) -> Result<SelectionResult, SelectionError> {
        if let Some(model) = model_id.filter(|value| !value.trim().is_empty()) {
            return Ok(SelectionResult {
                model_id: model.to_string(),
            });
        }
        let endpoint = self.endpoints.get(&endpoint);
        if let Some(endpoint) = endpoint {
            if let Some(default_model) = &endpoint.default_model {
                if !default_model.trim().is_empty() {
                    return Ok(SelectionResult {
                        model_id: default_model.clone(),
                    });
                }
            }
        }
        Err(SelectionError::ModelRequired)
    }
}

#[derive(Clone)]
struct ProviderCatalogEntry<M> {
    model_id: String,
    label: Option<String>,
    providers: HashMap<EndpointKind, Arc<dyn Provider<M>>>,
}

#[derive(Clone)]
struct Catalog<M> {
    providers: HashMap<String, ProviderCatalogEntry<M>>,
    endpoints: HashMap<EndpointKind, EndpointConfig>,
}

#[derive(Clone)]
pub struct ProviderEntry<M> {
    pub model_id: String,
    pub label: Option<String>,
    pub endpoint: EndpointKind,
    pub provider: Arc<dyn Provider<M>>,
}

#[derive(Clone)]
pub struct ProviderRegistry<M> {
    catalog: Catalog<M>,
    strategy: Arc<dyn SelectionStrategy<M>>,
    error_notifier: Option<Arc<dyn ProviderErrorNotifier<M>>>,
}

impl<M: Sync> Catalog<M> {
    fn from_config(
        providers: &[ProviderConfig],
        endpoints: &[EndpointConfig],
        provider_builder: Option<&ProviderBuilder<M>>,
    ) -> Result<Self, ConfigError> {
        let mut endpoint_map = HashMap::new();
        for endpoint in endpoints {
            let endpoint_kind = endpoint.endpoint()?;
            if endpoint_map
                .insert(endpoint_kind, endpoint.clone())
                .is_some()
            {
                return Err(ConfigError::InvalidProvider(format!(
                    "duplicate endpoint path: {}",
                    endpoint_kind.as_path()
                )));
            }
        }

        let mut provider_config_by_model = HashMap::new();
        for provider in providers {
            provider_config_by_model.insert(provider.model_id.to_ascii_lowercase(), provider);
        }

        let mut provider_map = HashMap::new();
        for provider in providers {
            provider_map.insert(
                provider.model_id.to_ascii_lowercase(),
                ProviderCatalogEntry {
                    model_id: provider.model_id.clone(),
                    label: provider.label.clone(),
                    providers: HashMap::new(),
                },
            );
        }

        for endpoint in endpoints {
            let endpoint_kind = endpoint.endpoint()?;
            let model_set = endpoint_models(endpoint);
            for model in model_set {
                let provider_entry = provider_map.get_mut(&model).ok_or_else(|| {
                    ConfigError::InvalidProvider(format!("model not found: {model}"))
                })?;
                let provider_config = provider_config_by_model
                    .get(provider_entry.model_id.to_ascii_lowercase().as_str())
                    .copied()
                    .ok_or_else(|| {
                        ConfigError::InvalidProvider(format!(
                            "provider config not found for {}",
                            provider_entry.model_id
                        ))
                    })?;
                let provider =
                    build_provider_with_custom(provider_config, endpoint_kind, provider_builder)?;
                provider_entry.providers.insert(endpoint_kind, provider);
            }
        }

        Ok(Self {
            providers: provider_map,
            endpoints: endpoint_map,
        })
    }

    fn supports_model(&self, endpoint: EndpointKind, model_id: &str) -> bool {
        let Some(endpoint_config) = self.endpoints.get(&endpoint) else {
            return false;
        };
        endpoint_config
            .models
            .iter()
            .any(|model| model.eq_ignore_ascii_case(model_id))
            || endpoint_config
                .default_model
                .as_ref()
                .is_some_and(|model| model.eq_ignore_ascii_case(model_id))
    }

    fn resolve_provider(
        &self,
        endpoint: EndpointKind,
        model_id: &str,
    ) -> Result<ProviderEntry<M>, SelectionError> {
        let catalog = self
            .providers
            .get(&model_id.to_ascii_lowercase())
            .ok_or_else(|| SelectionError::ModelNotSupported {
                model: model_id.to_string(),
            })?;
        let provider = catalog.providers.get(&endpoint).cloned().ok_or_else(|| {
            SelectionError::ModelNotSupported {
                model: model_id.to_string(),
            }
        })?;
        Ok(ProviderEntry {
            model_id: catalog.model_id.clone(),
            label: catalog.label.clone(),
            endpoint,
            provider,
        })
    }

    fn list_models(&self) -> Vec<String> {
        let mut seen = HashSet::new();
        let mut models = Vec::new();
        for endpoint in self.endpoints.values() {
            for model in endpoint.models.iter().chain(endpoint.default_model.iter()) {
                let normalized = model.to_ascii_lowercase();
                if seen.insert(normalized.clone()) {
                    let model_id = self
                        .providers
                        .get(&normalized)
                        .map(|provider| provider.model_id.clone())
                        .unwrap_or_else(|| model.clone());
                    models.push(model_id);
                }
            }
        }
        models
    }
}

fn endpoint_models(endpoint: &EndpointConfig) -> HashSet<String> {
    let mut models = HashSet::new();
    for model in &endpoint.models {
        models.insert(model.to_ascii_lowercase());
    }
    if let Some(default_model) = &endpoint.default_model {
        models.insert(default_model.to_ascii_lowercase());
    }
    models
}

impl<M: Sync> ProviderRegistry<M> {
    pub fn from_config(
        providers: &[ProviderConfig],
        endpoints: &[EndpointConfig],
        strategy: Arc<dyn SelectionStrategy<M>>,
    ) -> Result<Self, ConfigError> {
        Self::from_config_with_builder(providers, endpoints, strategy, None)
    }

    pub fn from_config_with_builder(
        providers: &[ProviderConfig],
        endpoints: &[EndpointConfig],
        strategy: Arc<dyn SelectionStrategy<M>>,
        provider_builder: Option<ProviderBuilder<M>>,
    ) -> Result<Self, ConfigError> {
        let catalog = Catalog::from_config(providers, endpoints, provider_builder.as_ref())?;
        Ok(Self {
            catalog,
            strategy,
            error_notifier: None,
        })
    }

    pub fn with_error_notifier(mut self, notifier: Arc<dyn ProviderErrorNotifier<M>>) -> Self {
        self.error_notifier = Some(notifier);
        self
    }

    pub fn list_models(&self, metadata: &M) -> Vec<String> {
        self.strategy
            .list_models(self.catalog.list_models(), metadata)
    }

    pub fn select_provider(
        &self,
        endpoint: EndpointKind,
        model_id: Option<&str>,
        metadata: &M,
    ) -> Result<ProviderEntry<M>, SelectionError>
    where
        M: Clone + Send + Sync + 'static,
    {
        let selected_model = self.select_model(endpoint, model_id, metadata)?;
        let mut resolved = self
            .catalog
            .resolve_provider(endpoint, selected_model.as_str())?;
        resolved.provider = self.wrap_provider(resolved.provider, &resolved.model_id, endpoint);
        Ok(resolved)
    }

    fn select_model(
        &self,
        endpoint: EndpointKind,
        model_id: Option<&str>,
        metadata: &M,
    ) -> Result<String, SelectionError> {
        let selected = self.strategy.select(endpoint, model_id, metadata)?;
        if !self
            .catalog
            .supports_model(endpoint, selected.model_id.as_str())
        {
            return Err(SelectionError::ModelNotSupported {
                model: selected.model_id,
            });
        }
        Ok(selected.model_id)
    }

    fn wrap_provider(
        &self,
        provider: Arc<dyn Provider<M>>,
        model_id: &str,
        endpoint: EndpointKind,
    ) -> Arc<dyn Provider<M>>
    where
        M: Clone + Send + Sync + 'static,
    {
        if let Some(notifier) = &self.error_notifier {
            return Arc::new(HookedProvider {
                inner: provider,
                error_notifier: Arc::clone(notifier),
                model_id: model_id.to_string(),
                endpoint,
            });
        }
        provider
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
}

#[derive(Clone)]
struct HookedProvider<M> {
    inner: Arc<dyn Provider<M>>,
    error_notifier: Arc<dyn ProviderErrorNotifier<M>>,
    model_id: String,
    endpoint: EndpointKind,
}

#[async_trait]
impl<M> Provider<M> for HookedProvider<M>
where
    M: Clone + Send + Sync + 'static,
{
    async fn complete(
        &self,
        request: ChatCompletionRequest,
        metadata: &M,
    ) -> Result<UnifiedResponse, ProviderError> {
        match self.inner.complete(request, metadata).await {
            Ok(response) => Ok(response),
            Err(err) => {
                self.notify_error(metadata, &err);
                Err(err)
            }
        }
    }

    async fn stream<'a>(
        &'a self,
        request: ChatCompletionRequest,
        metadata: &'a M,
    ) -> Result<
        Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    > {
        match self.inner.stream(request, metadata).await {
            Ok(response) => Ok(response),
            Err(err) => {
                self.notify_error(metadata, &err);
                Err(err)
            }
        }
    }

    async fn http(
        &self,
        request: HttpProviderRequest,
        metadata: &M,
    ) -> Result<HttpProviderResponse, ProviderError> {
        match self.inner.http(request, metadata).await {
            Ok(response) => Ok(response),
            Err(err) => {
                self.notify_error(metadata, &err);
                Err(err)
            }
        }
    }

    fn extract_request_id(&self, payload_json: &serde_json::Value) -> Option<String> {
        self.inner.extract_request_id(payload_json)
    }

    fn extract_usage(&self, payload_json: &serde_json::Value) -> Option<serde_json::Value> {
        self.inner.extract_usage(payload_json)
    }

    fn inject_usage(&self, payload_json: &mut serde_json::Value, usage: serde_json::Value) -> bool {
        self.inner.inject_usage(payload_json, usage)
    }
}

impl<M> HookedProvider<M>
where
    M: Clone + Send + Sync + 'static,
{
    fn notify_error(&self, metadata: &M, err: &ProviderError) {
        let http_status = match err {
            ProviderError::Public { status, .. } => Some(status.as_u16()),
            ProviderError::Internal {
                upstream_http_status,
                ..
            } => Some(upstream_http_status.as_u16()),
        };
        self.error_notifier
            .notify_provider_error(ProviderErrorEvent {
                model: Some(self.model_id.clone()),
                metadata: metadata.clone(),
                http_status,
                error: err.to_string(),
                endpoint: Some(format!("/v1{}", self.endpoint.as_path())),
            });
    }
}

fn build_provider_with_custom<M: Sync>(
    provider: &ProviderConfig,
    endpoint: EndpointKind,
    custom_builder: Option<&ProviderBuilder<M>>,
) -> Result<Arc<dyn Provider<M>>, ConfigError> {
    if let Some(builder) = custom_builder {
        if let Some(provider) = builder(provider, endpoint)? {
            return Ok(provider);
        }
    }

    match endpoint {
        EndpointKind::ChatCompletions => match provider.provider_type.as_str() {
            "openai-compatible" => Ok(Arc::new(build_openai_compatible_provider(
                &provider.params,
            )?)),
            "openai" => Ok(Arc::new(build_openai_provider(&provider.params)?)),
            "anthropic-messages" => Ok(Arc::new(build_anthropic_messages_provider(
                &provider.params,
            )?)),
            "bedrock-messages" => Ok(Arc::new(build_bedrock_messages_provider(&provider.params)?)),
            "tokenhub" => Ok(Arc::new(build_tokenhub_provider(&provider.params)?)),
            "vertexai" => Ok(Arc::new(build_vertexai_provider(&provider.params)?)),
            "vllm-deepseek" => Ok(Arc::new(build_vllm_deepseek_provider(&provider.params)?)),
            other => Err(ConfigError::InvalidProvider(format!(
                "provider type {other} is not supported for /chat/completions"
            ))),
        },
        EndpointKind::Messages => match provider.provider_type.as_str() {
            "anthropic-messages" => providers::messages::build_client(provider),
            "bedrock-messages" => providers::bedrock_messages::build_client(provider),
            other => Err(ConfigError::InvalidProvider(format!(
                "provider type {other} is not supported for /messages"
            ))),
        },
        EndpointKind::Responses => match provider.provider_type.as_str() {
            "openai-compatible" | "openai" | "tokenhub" => {
                providers::responses::build_client(provider)
            }
            other => Err(ConfigError::InvalidProvider(format!(
                "provider type {other} is not supported for /responses"
            ))),
        },
        EndpointKind::Embeddings
        | EndpointKind::Rerank
        | EndpointKind::AudioSpeech
        | EndpointKind::AudioTranscriptions
        | EndpointKind::ImagesGenerations
        | EndpointKind::ImagesEdits => match provider.provider_type.as_str() {
            "openai-compatible" | "openai" | "tokenhub" => {
                providers::passthrough::build_client(provider)
            }
            other => Err(ConfigError::InvalidProvider(format!(
                "provider type {other} is not supported for {}",
                endpoint.as_path()
            ))),
        },
        EndpointKind::ModelsGenerateContent | EndpointKind::ModelsStreamGenerateContent => {
            match provider.provider_type.as_str() {
                "vertexai" => providers::generate_content::build_client(provider),
                other => Err(ConfigError::InvalidProvider(format!(
                    "provider type {other} is not supported for {}",
                    endpoint.as_path()
                ))),
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use std::sync::Arc;

    use async_trait::async_trait;

    use super::*;
    use crate::serve_config::{EndpointConfig, ProviderConfig};

    struct DummyProvider;

    #[async_trait]
    impl Provider<()> for DummyProvider {
        async fn complete(
            &self,
            _request: ChatCompletionRequest,
            _metadata: &(),
        ) -> Result<UnifiedResponse, ProviderError> {
            Err(ProviderError::internal("dummy"))
        }
    }

    #[test]
    fn by_endpoint_model_uses_endpoint_default_model() {
        let mut endpoints = HashMap::new();
        endpoints.insert(
            EndpointKind::ChatCompletions,
            EndpointConfig {
                path: "/chat/completions".to_string(),
                models: vec!["chat-a".to_string()],
                default_model: Some("chat-a".to_string()),
            },
        );
        let strategy = ByEndpointModel::new(endpoints);

        let selected = strategy
            .select(EndpointKind::ChatCompletions, None, &())
            .expect("default model should be selected");

        assert_eq!(selected.model_id, "chat-a");
    }

    #[test]
    fn from_config_with_builder_accepts_custom_provider() {
        let providers = vec![ProviderConfig {
            provider_type: "custom-provider".to_string(),
            model_id: "gpt-5.5".to_string(),
            label: None,
            params: HashMap::new(),
        }];
        let endpoints = vec![EndpointConfig {
            path: "/chat/completions".to_string(),
            models: vec!["gpt-5.5".to_string()],
            default_model: Some("gpt-5.5".to_string()),
        }];
        let strategy = Arc::new(ByEndpointModel::new(HashMap::from([(
            EndpointKind::ChatCompletions,
            endpoints[0].clone(),
        )])));
        let builder: ProviderBuilder<()> = Arc::new(|provider, endpoint| {
            if provider.provider_type == "custom-provider"
                && endpoint == EndpointKind::ChatCompletions
            {
                return Ok(Some(Arc::new(DummyProvider)));
            }
            Ok(None)
        });

        let registry = ProviderRegistry::<()>::from_config_with_builder(
            &providers,
            &endpoints,
            strategy,
            Some(builder),
        )
        .expect("custom provider should be accepted");

        let selected = registry
            .select_provider(EndpointKind::ChatCompletions, None, &())
            .expect("provider should be selectable");
        assert_eq!(selected.model_id, "gpt-5.5");
    }

    #[test]
    fn select_provider_requires_endpoint_capability() {
        let endpoint = EndpointConfig {
            path: "/responses".to_string(),
            models: vec!["chat-a".to_string()],
            default_model: Some("chat-a".to_string()),
        };
        let registry = ProviderRegistry {
            catalog: Catalog {
                providers: HashMap::from([(
                    "chat-a".to_string(),
                    ProviderCatalogEntry {
                        model_id: "chat-a".to_string(),
                        label: None,
                        providers: HashMap::new(),
                    },
                )]),
                endpoints: HashMap::from([(EndpointKind::Responses, endpoint)]),
            },
            strategy: Arc::new(ByEndpointModel::new(HashMap::from([(
                EndpointKind::Responses,
                EndpointConfig {
                    path: "/responses".to_string(),
                    models: vec!["chat-a".to_string()],
                    default_model: Some("chat-a".to_string()),
                },
            )]))),
            error_notifier: None,
        };

        let err = match registry.select_provider(EndpointKind::Responses, None, &()) {
            Ok(_) => panic!("provider capability is required"),
            Err(err) => err,
        };
        assert!(matches!(err, SelectionError::ModelNotSupported { .. }));
    }
}
