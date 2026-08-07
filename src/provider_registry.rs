use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use anyhow::Error;
use async_trait::async_trait;
use futures_core::Stream;

use crate::llm_provider::anthropic_messages::{
    build_anthropic_messages_provider, build_bedrock_messages_provider,
};
use crate::llm_provider::openai::build_openai_provider;
use crate::llm_provider::openai_compatible::build_openai_compatible_provider;
use crate::llm_provider::tokenhub::build_tokenhub_provider;
use crate::llm_provider::vertexai::build_vertexai_provider;
use crate::llm_provider::vllm_deepseek::build_vllm_deepseek_provider;
use crate::llm_provider::{Provider, ProviderError, UnifiedEvent, UnifiedResponse};
use crate::model_api_provider::providers;
use crate::model_api_provider::{ModelApiProvider, ProviderRequest, ProviderResponse};
use crate::openai_types::ChatCompletionRequest;
use crate::provider_error_notifier::{ProviderErrorEvent, ProviderErrorNotifier};
use crate::serve_config::{ConfigError, EndpointConfig, EndpointKind, ProviderConfig};

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

/// Selects a request model for an endpoint.
pub trait SelectionStrategy<M>: Send + Sync {
    fn select(
        &self,
        endpoint: EndpointKind,
        model_id: Option<&str>,
        metadata: &M,
    ) -> Result<SelectionResult, SelectionError>;
}

/// Builds a custom chat provider from configuration.
pub type ChatProviderBuilder<M> =
    Arc<dyn Fn(&ProviderConfig) -> Result<Option<Arc<dyn Provider<M>>>, ConfigError> + Send + Sync>;

/// Builds a custom model-api endpoint provider from configuration.
pub type EndpointProviderBuilder<M> = Arc<
    dyn Fn(
            &ProviderConfig,
            EndpointKind,
        ) -> Result<Option<Arc<dyn ModelApiProvider<M>>>, ConfigError>
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
    chat_provider: Option<Arc<dyn Provider<M>>>,
    endpoint_providers: HashMap<EndpointKind, Arc<dyn ModelApiProvider<M>>>,
}

#[derive(Clone)]
struct Catalog<M> {
    providers: HashMap<String, ProviderCatalogEntry<M>>,
    endpoints: HashMap<EndpointKind, EndpointConfig>,
}

#[derive(Clone)]
struct ResolvedChatProvider<M> {
    model_id: String,
    label: Option<String>,
    provider: Arc<dyn Provider<M>>,
}

#[derive(Clone)]
struct ResolvedEndpointProvider<M> {
    model_id: String,
    label: Option<String>,
    provider: Arc<dyn ModelApiProvider<M>>,
}

#[derive(Clone)]
pub struct ChatProviderEntry<M> {
    pub model_id: String,
    pub label: Option<String>,
    pub provider: Arc<dyn Provider<M>>,
}

#[derive(Clone)]
pub struct EndpointProviderEntry<M> {
    pub model_id: String,
    pub label: Option<String>,
    pub provider: Arc<dyn ModelApiProvider<M>>,
}

#[derive(Clone)]
pub struct ProviderRegistry<M> {
    catalog: Catalog<M>,
    strategy: Arc<dyn SelectionStrategy<M>>,
    error_notifier: Option<Arc<dyn ProviderErrorNotifier<M>>>,
}

impl<M> Catalog<M> {
    fn from_config(
        providers: &[ProviderConfig],
        endpoints: &[EndpointConfig],
        chat_builder: Option<&ChatProviderBuilder<M>>,
        endpoint_builder: Option<&EndpointProviderBuilder<M>>,
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
            let chat_provider = build_chat_provider_with_custom(provider, chat_builder)?;
            provider_map.insert(
                provider.model_id.to_ascii_lowercase(),
                ProviderCatalogEntry {
                    model_id: provider.model_id.clone(),
                    label: provider.label.clone(),
                    chat_provider,
                    endpoint_providers: HashMap::new(),
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
                if endpoint_kind == EndpointKind::ChatCompletions {
                    if provider_entry.chat_provider.is_none() {
                        return Err(ConfigError::InvalidProvider(format!(
                            "provider {} does not support /chat/completions",
                            provider_entry.model_id
                        )));
                    }
                    continue;
                }

                let provider_config = provider_config_by_model
                    .get(provider_entry.model_id.to_ascii_lowercase().as_str())
                    .copied()
                    .ok_or_else(|| {
                        ConfigError::InvalidProvider(format!(
                            "provider config not found for {}",
                            provider_entry.model_id
                        ))
                    })?;

                let endpoint_provider = build_endpoint_provider_with_custom(
                    provider_config,
                    endpoint_kind,
                    endpoint_builder,
                )?;
                provider_entry
                    .endpoint_providers
                    .insert(endpoint_kind, endpoint_provider);
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

    fn resolve_chat(&self, model_id: &str) -> Result<ResolvedChatProvider<M>, SelectionError> {
        let catalog = self
            .providers
            .get(&model_id.to_ascii_lowercase())
            .ok_or_else(|| SelectionError::ModelNotSupported {
                model: model_id.to_string(),
            })?;
        let provider =
            catalog
                .chat_provider
                .clone()
                .ok_or_else(|| SelectionError::ModelNotSupported {
                    model: model_id.to_string(),
                })?;
        Ok(ResolvedChatProvider {
            model_id: catalog.model_id.clone(),
            label: catalog.label.clone(),
            provider,
        })
    }

    fn resolve_endpoint(
        &self,
        endpoint: EndpointKind,
        model_id: &str,
    ) -> Result<ResolvedEndpointProvider<M>, SelectionError> {
        let catalog = self
            .providers
            .get(&model_id.to_ascii_lowercase())
            .ok_or_else(|| SelectionError::ModelNotSupported {
                model: model_id.to_string(),
            })?;
        let provider = catalog
            .endpoint_providers
            .get(&endpoint)
            .cloned()
            .ok_or_else(|| SelectionError::ModelNotSupported {
                model: model_id.to_string(),
            })?;
        Ok(ResolvedEndpointProvider {
            model_id: catalog.model_id.clone(),
            label: catalog.label.clone(),
            provider,
        })
    }

    fn model_list(&self) -> Vec<String> {
        let mut models = Vec::new();
        let mut seen = HashSet::new();
        for endpoint in self.endpoints.values() {
            for model in endpoint.models.iter().chain(endpoint.default_model.iter()) {
                let normalized = model.to_ascii_lowercase();
                if !seen.insert(normalized.clone()) {
                    continue;
                }

                let model_id = self
                    .providers
                    .get(&normalized)
                    .map(|provider| provider.model_id.clone())
                    .unwrap_or_else(|| model.clone());
                models.push(model_id);
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

impl<M> ProviderRegistry<M> {
    pub fn from_config(
        providers: &[ProviderConfig],
        endpoints: &[EndpointConfig],
        strategy: Arc<dyn SelectionStrategy<M>>,
    ) -> Result<Self, ConfigError> {
        Self::from_config_with_builders(providers, endpoints, strategy, None, None)
    }

    pub fn from_config_with_builders(
        providers: &[ProviderConfig],
        endpoints: &[EndpointConfig],
        strategy: Arc<dyn SelectionStrategy<M>>,
        chat_builder: Option<ChatProviderBuilder<M>>,
        endpoint_builder: Option<EndpointProviderBuilder<M>>,
    ) -> Result<Self, ConfigError> {
        let catalog = Catalog::from_config(
            providers,
            endpoints,
            chat_builder.as_ref(),
            endpoint_builder.as_ref(),
        )?;

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

    pub fn select_chat(
        &self,
        endpoint: EndpointKind,
        model_id: Option<&str>,
        metadata: &M,
    ) -> Result<ChatProviderEntry<M>, SelectionError>
    where
        M: Clone + Send + Sync + 'static,
    {
        let selected_model = self.select_model(endpoint, model_id, metadata)?;
        let resolved = self.catalog.resolve_chat(selected_model.as_str())?;
        let provider =
            self.wrap_chat_provider(endpoint, resolved.provider.clone(), &resolved.model_id);
        Ok(ChatProviderEntry {
            model_id: resolved.model_id,
            label: resolved.label,
            provider,
        })
    }

    pub fn select_endpoint(
        &self,
        endpoint: EndpointKind,
        model_id: Option<&str>,
        metadata: &M,
    ) -> Result<EndpointProviderEntry<M>, SelectionError>
    where
        M: Clone + Send + Sync + 'static,
    {
        let selected_model = self.select_model(endpoint, model_id, metadata)?;
        let resolved = self
            .catalog
            .resolve_endpoint(endpoint, selected_model.as_str())?;
        let provider = self.wrap_endpoint_provider(resolved.provider.clone(), &resolved.model_id);
        Ok(EndpointProviderEntry {
            model_id: resolved.model_id,
            label: resolved.label,
            provider,
        })
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

    fn wrap_chat_provider(
        &self,
        endpoint: EndpointKind,
        provider: Arc<dyn Provider<M>>,
        model_id: &str,
    ) -> Arc<dyn Provider<M>>
    where
        M: Clone + Send + Sync + 'static,
    {
        if let Some(notifier) = &self.error_notifier {
            return Arc::new(HookedChatProvider {
                inner: provider,
                error_notifier: Arc::clone(notifier),
                model_id: model_id.to_string(),
                endpoint: format!("/v1{}", endpoint.as_path()),
            });
        }
        provider
    }

    fn wrap_endpoint_provider(
        &self,
        provider: Arc<dyn ModelApiProvider<M>>,
        model_id: &str,
    ) -> Arc<dyn ModelApiProvider<M>>
    where
        M: Clone + Send + Sync + 'static,
    {
        if let Some(notifier) = &self.error_notifier {
            return Arc::new(HookedEndpointProvider {
                inner: provider,
                error_notifier: Arc::clone(notifier),
                model_id: model_id.to_string(),
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

    pub fn model_list(&self) -> Vec<String> {
        self.catalog.model_list()
    }
}

#[derive(Clone)]
struct HookedChatProvider<M> {
    inner: Arc<dyn Provider<M>>,
    error_notifier: Arc<dyn ProviderErrorNotifier<M>>,
    model_id: String,
    endpoint: String,
}

#[async_trait]
impl<M> Provider<M> for HookedChatProvider<M>
where
    M: Clone + Send + Sync + 'static,
{
    fn model_id(&self) -> &str {
        self.inner.model_id()
    }

    async fn complete(
        &self,
        request: ChatCompletionRequest,
        metadata: &M,
    ) -> Result<UnifiedResponse, ProviderError> {
        self.inner
            .complete(request, metadata)
            .await
            .map_err(|err| self.notify_error(metadata, err))
    }

    async fn stream<'a>(
        &'a self,
        request: ChatCompletionRequest,
        metadata: &M,
    ) -> Result<
        std::pin::Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    > {
        self.inner
            .stream(request, metadata)
            .await
            .map_err(|err| self.notify_error(metadata, err))
    }
}

impl<M> HookedChatProvider<M>
where
    M: Clone + Send + Sync + 'static,
{
    fn notify_error(&self, metadata: &M, err: ProviderError) -> ProviderError {
        let http_status = match &err {
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
                endpoint: Some(self.endpoint.clone()),
            });
        err
    }
}

#[derive(Clone)]
struct HookedEndpointProvider<M> {
    inner: Arc<dyn ModelApiProvider<M>>,
    error_notifier: Arc<dyn ProviderErrorNotifier<M>>,
    model_id: String,
}

#[async_trait]
impl<M> ModelApiProvider<M> for HookedEndpointProvider<M>
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

impl<M> HookedEndpointProvider<M>
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

fn build_chat_provider_with_custom<M>(
    provider: &ProviderConfig,
    custom_builder: Option<&ChatProviderBuilder<M>>,
) -> Result<Option<Arc<dyn Provider<M>>>, ConfigError> {
    if let Some(builder) = custom_builder {
        if let Some(provider) = builder(provider)? {
            return Ok(Some(provider));
        }
    }

    match provider.provider_type.as_str() {
        "openai-compatible" => Ok(Some(Arc::new(build_openai_compatible_provider(
            &provider.params,
        )?))),
        "openai" => Ok(Some(Arc::new(build_openai_provider(&provider.params)?))),
        "anthropic" => Ok(Some(Arc::new(build_anthropic_messages_provider(
            &provider.params,
        )?))),
        "bedrock" => Ok(Some(Arc::new(build_bedrock_messages_provider(
            &provider.params,
        )?))),
        "tokenhub" => Ok(Some(Arc::new(build_tokenhub_provider(&provider.params)?))),
        "vertexai" => Ok(Some(Arc::new(build_vertexai_provider(&provider.params)?))),
        "vllm-deepseek" => Ok(Some(Arc::new(build_vllm_deepseek_provider(
            &provider.params,
        )?))),
        _ => Ok(None),
    }
}

fn build_endpoint_provider_with_custom<M>(
    provider: &ProviderConfig,
    endpoint: EndpointKind,
    custom_builder: Option<&EndpointProviderBuilder<M>>,
) -> Result<Arc<dyn ModelApiProvider<M>>, ConfigError> {
    if let Some(builder) = custom_builder {
        if let Some(provider) = builder(provider, endpoint)? {
            return Ok(provider);
        }
    }

    match endpoint {
        EndpointKind::Messages => match provider.provider_type.as_str() {
            "anthropic" => providers::messages::build_client(provider),
            "bedrock" => providers::bedrock_messages::build_client(provider),
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
        EndpointKind::ModelsGenerateContent => match provider.provider_type.as_str() {
            "vertexai" => providers::generate_content::build_client(provider),
            other => Err(ConfigError::InvalidProvider(format!(
                "provider type {other} is not supported for /models/:generateContent"
            ))),
        },
        EndpointKind::ChatCompletions => Err(ConfigError::InvalidProvider(
            "chat/completions uses chat provider directly".to_string(),
        )),
    }
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use std::pin::Pin;
    use std::sync::Arc;

    use anyhow::Error;
    use async_trait::async_trait;

    use super::{
        ByEndpointModel, Catalog, ChatProviderBuilder, EndpointProviderBuilder,
        ProviderCatalogEntry, ProviderRegistry, SelectionError, SelectionResult, SelectionStrategy,
    };
    use crate::llm_provider::{Provider, ProviderError, UnifiedEvent, UnifiedResponse};
    use crate::model_api_provider::{ModelApiProvider, ProviderRequest, ProviderResponse};
    use crate::openai_types::ChatCompletionRequest;
    use crate::serve_config::{EndpointConfig, EndpointKind, ProviderConfig};

    struct DenyStrategy;

    impl SelectionStrategy<()> for DenyStrategy {
        fn select(
            &self,
            _endpoint: EndpointKind,
            _model_id: Option<&str>,
            _metadata: &(),
        ) -> Result<SelectionResult, SelectionError> {
            Err(SelectionError::AccessDenied)
        }
    }

    struct DummyChatProvider;

    struct DummyEndpointProvider;

    #[async_trait]
    impl Provider<()> for DummyChatProvider {
        fn model_id(&self) -> &str {
            "chat-a"
        }

        async fn complete(
            &self,
            _request: ChatCompletionRequest,
            _metadata: &(),
        ) -> Result<UnifiedResponse, ProviderError> {
            Err(ProviderError::internal("not used in test"))
        }

        async fn stream<'a>(
            &'a self,
            _request: ChatCompletionRequest,
            _metadata: &(),
        ) -> Result<
            Pin<
                Box<
                    dyn futures_core::Stream<Item = Result<UnifiedEvent, ProviderError>>
                        + Send
                        + 'a,
                >,
            >,
            ProviderError,
        > {
            Err(ProviderError::internal("not used in test"))
        }
    }

    #[async_trait]
    impl ModelApiProvider<()> for DummyEndpointProvider {
        fn model_id(&self) -> &str {
            "chat-a"
        }

        async fn execute(
            &self,
            _req: ProviderRequest,
            _metadata: &(),
        ) -> Result<ProviderResponse, Error> {
            Err(Error::msg("not used in test"))
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
    fn from_config_rejects_unsupported_messages_provider_type() {
        let providers = vec![ProviderConfig {
            provider_type: "openai-compatible".to_string(),
            model_id: "oai-a".to_string(),
            label: None,
            params: HashMap::from([
                ("api_key".to_string(), "sk-test".to_string()),
                (
                    "base_url".to_string(),
                    "https://api.example.com/v1".to_string(),
                ),
                ("model".to_string(), "gpt-4.1".to_string()),
            ]),
        }];
        let endpoints = vec![EndpointConfig {
            path: "/messages".to_string(),
            models: vec!["oai-a".to_string()],
            default_model: Some("oai-a".to_string()),
        }];

        let strategy = Arc::new(ByEndpointModel::new(HashMap::from([(
            EndpointKind::Messages,
            endpoints[0].clone(),
        )])));
        let err = match ProviderRegistry::<()>::from_config(&providers, &endpoints, strategy) {
            Ok(_) => panic!("/messages should reject openai-compatible provider type"),
            Err(err) => err,
        };

        assert!(
            err.to_string()
                .contains("provider type openai-compatible is not supported for /messages")
        );
    }

    #[test]
    fn from_config_with_builders_accepts_custom_chat_provider() {
        let providers = vec![ProviderConfig {
            provider_type: "custom-chat".to_string(),
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
        let chat_builder: ChatProviderBuilder<()> = Arc::new(|provider| {
            if provider.provider_type == "custom-chat" {
                return Ok(Some(Arc::new(DummyChatProvider)));
            }
            Ok(None)
        });

        let registry = ProviderRegistry::<()>::from_config_with_builders(
            &providers,
            &endpoints,
            strategy,
            Some(chat_builder),
            None,
        )
        .expect("custom chat provider should be accepted for chat completions");

        let selected = registry
            .select_chat(EndpointKind::ChatCompletions, None, &())
            .expect("chat provider should be selectable");
        assert_eq!(selected.model_id, "gpt-5.5");
    }

    #[test]
    fn from_config_with_builders_accepts_custom_endpoint_provider() {
        let providers = vec![ProviderConfig {
            provider_type: "custom-endpoint".to_string(),
            model_id: "gpt-5.5".to_string(),
            label: None,
            params: HashMap::new(),
        }];
        let endpoints = vec![EndpointConfig {
            path: "/responses".to_string(),
            models: vec!["gpt-5.5".to_string()],
            default_model: Some("gpt-5.5".to_string()),
        }];
        let strategy = Arc::new(ByEndpointModel::new(HashMap::from([(
            EndpointKind::Responses,
            endpoints[0].clone(),
        )])));
        let endpoint_builder: EndpointProviderBuilder<()> = Arc::new(|provider, endpoint| {
            if provider.provider_type == "custom-endpoint" && endpoint == EndpointKind::Responses {
                return Ok(Some(Arc::new(DummyEndpointProvider)));
            }
            Ok(None)
        });

        let registry = ProviderRegistry::<()>::from_config_with_builders(
            &providers,
            &endpoints,
            strategy,
            None,
            Some(endpoint_builder),
        )
        .expect("custom endpoint provider should be accepted for responses endpoint");

        let selected = registry
            .select_endpoint(EndpointKind::Responses, None, &())
            .expect("responses provider should be selectable");
        assert_eq!(selected.model_id, "gpt-5.5");
    }

    #[test]
    fn select_chat_propagates_access_denied_from_strategy() {
        let registry = ProviderRegistry {
            catalog: Catalog {
                providers: HashMap::new(),
                endpoints: HashMap::new(),
            },
            strategy: Arc::new(DenyStrategy),
            error_notifier: None,
        };

        let err = match registry.select_chat(EndpointKind::ChatCompletions, Some("chat-a"), &()) {
            Ok(_) => panic!("selection should be denied by strategy"),
            Err(err) => err,
        };

        assert!(matches!(err, SelectionError::AccessDenied));
    }

    #[test]
    fn select_endpoint_requires_endpoint_capability() {
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
                        chat_provider: Some(Arc::new(DummyChatProvider)),
                        endpoint_providers: HashMap::new(),
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

        let err = match registry.select_endpoint(EndpointKind::Responses, None, &()) {
            Ok(_) => panic!("endpoint provider capability is required"),
            Err(err) => err,
        };
        assert!(matches!(err, SelectionError::ModelNotSupported { .. }));
    }

    #[test]
    fn from_config_rejects_duplicate_endpoint_paths() {
        let providers = vec![ProviderConfig {
            provider_type: "openai-compatible".to_string(),
            model_id: "oai-a".to_string(),
            label: None,
            params: HashMap::from([
                ("api_key".to_string(), "sk-test".to_string()),
                (
                    "base_url".to_string(),
                    "https://api.example.com/v1".to_string(),
                ),
                ("model".to_string(), "gpt-4.1".to_string()),
            ]),
        }];
        let endpoints = vec![
            EndpointConfig {
                path: "/responses".to_string(),
                models: vec!["oai-a".to_string()],
                default_model: Some("oai-a".to_string()),
            },
            EndpointConfig {
                path: "/responses".to_string(),
                models: vec!["oai-a".to_string()],
                default_model: Some("oai-a".to_string()),
            },
        ];

        let strategy = Arc::new(ByEndpointModel::new(HashMap::from([(
            EndpointKind::Responses,
            endpoints[0].clone(),
        )])));
        let err = match ProviderRegistry::<()>::from_config(&providers, &endpoints, strategy) {
            Ok(_) => panic!("duplicate endpoint path must be rejected"),
            Err(err) => err,
        };

        assert!(
            err.to_string()
                .contains("duplicate endpoint path: /responses")
        );
    }

    #[test]
    fn model_list_returns_only_models_enabled_by_endpoints() {
        let providers = vec![
            ProviderConfig {
                provider_type: "openai-compatible".to_string(),
                model_id: "enabled-model".to_string(),
                label: None,
                params: HashMap::from([
                    ("api_key".to_string(), "sk-test".to_string()),
                    (
                        "base_url".to_string(),
                        "https://api.example.com/v1".to_string(),
                    ),
                    ("model".to_string(), "enabled-model".to_string()),
                ]),
            },
            ProviderConfig {
                provider_type: "openai-compatible".to_string(),
                model_id: "unused-model".to_string(),
                label: None,
                params: HashMap::from([
                    ("api_key".to_string(), "sk-test".to_string()),
                    (
                        "base_url".to_string(),
                        "https://api.example.com/v1".to_string(),
                    ),
                    ("model".to_string(), "unused-model".to_string()),
                ]),
            },
        ];
        let endpoints = vec![EndpointConfig {
            path: "/chat/completions".to_string(),
            models: vec!["enabled-model".to_string()],
            default_model: Some("enabled-model".to_string()),
        }];
        let strategy = Arc::new(ByEndpointModel::new(HashMap::from([(
            EndpointKind::ChatCompletions,
            endpoints[0].clone(),
        )])));

        let registry = ProviderRegistry::<()>::from_config(&providers, &endpoints, strategy)
            .expect("registry should be created");

        assert_eq!(registry.model_list(), vec!["enabled-model".to_string()]);
    }
}
