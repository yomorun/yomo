use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use async_trait::async_trait;
use futures_core::Stream;

use crate::dialects::build_bound_client;
use crate::provider::UnifiedRequest;
use crate::provider::{BoundProvider, Provider, ProviderError, UnifiedEvent, UnifiedResponse};
use crate::provider_error_notifier::{ProviderErrorEvent, ProviderErrorNotifier};
use crate::serve_config::{
    ConfigError, EndpointConfig, EndpointKind, ProviderConfig, ProviderKind, ProviderKindParams,
    ResolvedBinding, ResponseModelMode, parse_response_model_mode,
};

#[derive(Clone, Debug)]
pub struct SelectionResult {
    pub model_id: String,
}

#[derive(Debug)]
pub enum SelectionError {
    ModelRequired,
    ModelNotSupported {
        model: String,
    },
    ModelEndpointNotSupported {
        model: String,
        endpoint: EndpointKind,
    },
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
struct RuntimeChatProvider<M> {
    model_id: String,
    label: Option<String>,
    response_model_mode: ResponseModelMode,
    provider: Arc<dyn Provider<M>>,
}

#[derive(Clone)]
pub struct ChatProviderEntry<M> {
    pub model_id: String,
    pub label: Option<String>,
    pub response_model_mode: ResponseModelMode,
    pub provider: Arc<dyn Provider<M>>,
}

/// Chat model registry abstraction used by HTTP handlers.
pub trait ProviderRegistry<M>: Send + Sync {
    fn select_chat(
        &self,
        endpoint: EndpointKind,
        model_id: Option<&str>,
        metadata: &M,
    ) -> Result<ChatProviderEntry<M>, SelectionError>
    where
        M: Clone + Send + Sync + 'static;

    fn model_list(&self) -> Vec<String>;
}

#[derive(Clone)]
pub struct ConfigProviderRegistry<M> {
    registry: Registry,
    strategy: Arc<dyn SelectionStrategy<M>>,
    chats: HashMap<String, RuntimeChatProvider<M>>,
    error_notifier: Option<Arc<dyn ProviderErrorNotifier<M>>>,
}

impl<M> ConfigProviderRegistry<M> {
    pub fn from_entries(
        entries: &[ProviderConfig],
        strategy: Arc<dyn SelectionStrategy<M>>,
    ) -> Result<Self, ConfigError>
    where
        M: Send + Sync + 'static,
    {
        let registry = build_registry(entries)?;
        let mut chats = HashMap::new();

        for typed in entries {
            if matches!(typed.params, ProviderKindParams::Passthrough(_)) {
                continue;
            }

            let provider = build_dialect_chat_provider(typed)?;
            chats.insert(
                typed.model_id.to_ascii_lowercase(),
                RuntimeChatProvider {
                    model_id: typed.model_id.clone(),
                    label: typed.label.clone(),
                    response_model_mode: typed_response_model_mode(typed)?,
                    provider,
                },
            );
        }

        Ok(Self {
            registry,
            strategy,
            chats,
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
        let SelectionResult { model_id } = self.strategy.select(endpoint, model_id, metadata)?;
        let Some(entry) = self.registry.get(&model_id) else {
            return Err(SelectionError::ModelNotSupported { model: model_id });
        };
        if !entry.serves_endpoint(endpoint) {
            return Err(SelectionError::ModelEndpointNotSupported {
                model: entry.model_id.clone(),
                endpoint,
            });
        }
        let chat = self
            .chats
            .get(&entry.model_id.to_ascii_lowercase())
            .ok_or_else(|| SelectionError::ModelEndpointNotSupported {
                model: entry.model_id.clone(),
                endpoint,
            })?;
        let provider =
            self.wrap_chat_provider(endpoint, Arc::clone(&chat.provider), &chat.model_id);
        Ok(ChatProviderEntry {
            model_id: chat.model_id.clone(),
            label: chat.label.clone(),
            response_model_mode: chat.response_model_mode.clone(),
            provider,
        })
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

    pub fn model_list(&self) -> Vec<String> {
        self.registry
            .model_ids()
            .into_iter()
            .map(str::to_string)
            .collect()
    }
}

impl<M> ProviderRegistry<M> for ConfigProviderRegistry<M> {
    fn select_chat(
        &self,
        endpoint: EndpointKind,
        model_id: Option<&str>,
        metadata: &M,
    ) -> Result<ChatProviderEntry<M>, SelectionError>
    where
        M: Clone + Send + Sync + 'static,
    {
        ConfigProviderRegistry::select_chat(self, endpoint, model_id, metadata)
    }

    fn model_list(&self) -> Vec<String> {
        ConfigProviderRegistry::model_list(self)
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

    fn binding(&self) -> Option<&ResolvedBinding> {
        self.inner.binding()
    }

    async fn complete(
        &self,
        request: UnifiedRequest,
        metadata: &M,
    ) -> Result<UnifiedResponse, ProviderError> {
        self.inner
            .complete(request, metadata)
            .await
            .map_err(|err| self.notify_error(metadata, err))
    }

    async fn stream<'a>(
        &'a self,
        request: UnifiedRequest,
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

fn build_dialect_chat_provider<M>(
    provider: &ProviderConfig,
) -> Result<Arc<dyn Provider<M>>, ConfigError>
where
    M: Send + Sync + 'static,
{
    let client = build_bound_client::<M>(provider).map_err(ConfigError::InvalidProvider)?;
    match &provider.binding {
        Some(binding) => Ok(Arc::new(BoundProvider::new(client, binding.clone()))),
        None => Ok(client),
    }
}

fn typed_response_model_mode(provider: &ProviderConfig) -> Result<ResponseModelMode, ConfigError> {
    let raw = match &provider.params {
        ProviderKindParams::OpenAiChatCompletions(params) => params.response_model.as_deref(),
        ProviderKindParams::OpenAiResponses(params) => params.response_model.as_deref(),
        _ => None,
    };
    parse_response_model_mode(raw).map_err(|err| {
        ConfigError::InvalidProvider(format!(
            "invalid response_model for {}: {}",
            provider.model_id, err
        ))
    })
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use std::sync::Arc;

    use super::{
        ByEndpointModel, ConfigProviderRegistry, Registry, SelectionError, SelectionResult,
        SelectionStrategy,
    };
    use crate::serve_config::{EndpointConfig, EndpointKind, ProviderConfig, ResponseModelMode};

    fn typed_entry(json: &str) -> ProviderConfig {
        serde_json::from_str(json).expect("typed entry")
    }

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
    fn from_entries_builds_selectable_dialect_provider() {
        let providers = vec![typed_entry(
            r#"{"type": "openai-chat-completions", "model_id": "gpt-5.5",
                "params": {"base_url": "https://api.example.com/v1",
                "auth": {"scheme": "bearer", "api_key": "sk-test"},
                "model": "gpt-5.5"}}"#,
        )];
        let strategy = Arc::new(ByEndpointModel::new(HashMap::from([(
            EndpointKind::ChatCompletions,
            EndpointConfig {
                path: "/chat/completions".to_string(),
                models: vec!["gpt-5.5".to_string()],
                default_model: Some("gpt-5.5".to_string()),
            },
        )])));
        let registry = ConfigProviderRegistry::<()>::from_entries(&providers, strategy)
            .expect("dialect provider should build");

        let selected = registry
            .select_chat(EndpointKind::ChatCompletions, None, &())
            .expect("chat provider should be selectable");
        assert_eq!(selected.model_id, "gpt-5.5");
    }

    #[test]
    fn select_chat_propagates_access_denied_from_strategy() {
        let registry = ConfigProviderRegistry {
            registry: Registry::default(),
            strategy: Arc::new(DenyStrategy),
            chats: HashMap::new(),
            error_notifier: None,
        };

        let err = match registry.select_chat(EndpointKind::ChatCompletions, Some("chat-a"), &()) {
            Ok(_) => panic!("selection should be denied by strategy"),
            Err(err) => err,
        };

        assert!(matches!(err, SelectionError::AccessDenied));
    }

    #[test]
    fn model_list_returns_registry_models() {
        let providers = vec![
            typed_entry(
                r#"{"type": "openai-chat-completions", "model_id": "enabled-model",
                    "params": {"base_url": "https://api.example.com/v1",
                    "auth": {"scheme": "bearer", "api_key": "sk-test"},
                    "model": "enabled-model"}}"#,
            ),
            typed_entry(
                r#"{"type": "passthrough", "model_id": "embed-model", "serve": ["/embeddings"],
                    "params": {"base_url": "https://api.example.com/v1", "model": "embed-model"}}"#,
            ),
        ];
        let strategy = Arc::new(ByEndpointModel::new(HashMap::new()));

        let mut models = ConfigProviderRegistry::<()>::from_entries(&providers, strategy)
            .expect("registry should be created")
            .model_list();
        models.sort();
        assert_eq!(
            models,
            vec!["embed-model".to_string(), "enabled-model".to_string()]
        );
    }

    #[test]
    fn select_chat_propagates_provider_response_model_mode() {
        let providers = vec![typed_entry(
            r#"{"type": "openai-chat-completions", "model_id": "enabled-model",
                "params": {"base_url": "https://api.example.com/v1",
                "auth": {"scheme": "bearer", "api_key": "sk-test"},
                "model": "enabled-model", "response_model": "masked-model"}}"#,
        )];
        let strategy = Arc::new(ByEndpointModel::new(HashMap::from([(
            EndpointKind::ChatCompletions,
            EndpointConfig {
                path: "/chat/completions".to_string(),
                models: vec!["enabled-model".to_string()],
                default_model: Some("enabled-model".to_string()),
            },
        )])));

        let registry = ConfigProviderRegistry::<()>::from_entries(&providers, strategy)
            .expect("registry should be created");
        let selected = registry
            .select_chat(EndpointKind::ChatCompletions, None, &())
            .expect("chat provider should be selectable");

        assert_eq!(
            selected.response_model_mode,
            ResponseModelMode::Fixed("masked-model".to_string())
        );
    }
}

// ---------------------------------------------------------------------------
// Registry and execution mode
// ---------------------------------------------------------------------------

/// Execution mode, derived in code rather than configured.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ExecutionMode {
    /// Dialect kind x chat endpoint: goes through the single run_agent_loop engine.
    AgentLoop,
    /// passthrough / non-chat endpoint: forwarded as-is.
    Passthrough,
}

/// Chat endpoints (the three a dialect kind may bind to).
fn is_chat_endpoint(endpoint: EndpointKind) -> bool {
    matches!(
        endpoint,
        EndpointKind::ChatCompletions | EndpointKind::Responses | EndpointKind::Messages
    )
}

/// Determine the execution mode.
pub fn execution_mode(endpoint_kind: EndpointKind, kind: ProviderKind) -> ExecutionMode {
    match kind {
        ProviderKind::Passthrough => ExecutionMode::Passthrough,
        _ => {
            if is_chat_endpoint(endpoint_kind) {
                ExecutionMode::AgentLoop
            } else {
                ExecutionMode::Passthrough
            }
        }
    }
}

/// One entry in the registry, available for lookup at request time.
#[derive(Clone)]
pub struct RegistryEntry {
    pub model_id: String,
    pub kind: ProviderKind,
    /// Display label (o11y / agent loop).
    pub label: Option<String>,
    /// passthrough serve list (already resolved to EndpointKind); always empty for dialect kinds.
    pub serves: Vec<EndpointKind>,
    /// passthrough only: forwarder built at startup (zero construction at request time).
    pub forwarder: Option<crate::dialects::passthrough::PassthroughForwarder>,
}

impl RegistryEntry {
    /// Whether this model serves an endpoint (part of the existence check).
    pub fn serves_endpoint(&self, endpoint: EndpointKind) -> bool {
        match self.kind {
            ProviderKind::Passthrough => self.serves.iter().any(|served| *served == endpoint),
            _ => is_chat_endpoint(endpoint),
        }
    }
}

/// Registry of configured providers, with entries registered after deduplication.
#[derive(Clone, Default)]
pub struct Registry {
    entries: HashMap<String, RegistryEntry>,
}

impl Registry {
    pub fn get(&self, model_id: &str) -> Option<&RegistryEntry> {
        self.entries.get(&model_id.to_ascii_lowercase())
    }

    pub fn entry_count(&self) -> usize {
        self.entries.len()
    }

    pub fn model_ids(&self) -> Vec<&str> {
        self.entries
            .values()
            .map(|entry| entry.model_id.as_str())
            .collect()
    }
}

/// Build the registry, validating provider entries and preparing their forwarders.
pub fn build_registry(entries: &[ProviderConfig]) -> Result<Registry, ConfigError> {
    let mut model_ids = HashSet::new();
    let mut registry_entries = HashMap::new();

    for typed in entries {
        // Reject duplicate model IDs, ignoring case.
        let normalized = typed.model_id.to_ascii_lowercase();
        if !model_ids.insert(normalized.clone()) {
            return Err(ConfigError::InvalidProvider(format!(
                "duplicate model_id: {}",
                typed.model_id
            )));
        }

        // serve is a passthrough-only endpoint binding declaration.
        // Dialect kinds appear on the three chat endpoints automatically; declaring serve is rejected.
        // passthrough must declare a non-empty serve explicitly and must not hijack chat endpoints.
        // GoogleVertex entries additionally bind /models/:generateContent
        // using the native protocol with service-account authentication.
        let (serves, forwarder) = match &typed.params {
            ProviderKindParams::Passthrough(p) => {
                let serves = match &typed.serve {
                    Some(serve) if !serve.is_empty() => {
                        let mut endpoint_kinds = Vec::new();
                        for path in serve {
                            // Reject serve paths that do not map to a known endpoint.
                            let endpoint_kind = EndpointKind::from_config_path(path)?;
                            if is_chat_endpoint(endpoint_kind) {
                                return Err(ConfigError::InvalidProvider(format!(
                                    "passthrough cannot serve chat endpoint: {path}"
                                )));
                            }
                            endpoint_kinds.push(endpoint_kind);
                        }
                        endpoint_kinds
                    }
                    Some(_) => {
                        return Err(ConfigError::InvalidProvider(format!(
                            "passthrough provider {} has empty serve list",
                            typed.model_id
                        )));
                    }
                    None => {
                        return Err(ConfigError::InvalidProvider(format!(
                            "passthrough provider {} requires a serve list",
                            typed.model_id
                        )));
                    }
                };
                let forwarder = Some(
                    crate::dialects::passthrough::PassthroughForwarder::new(
                        &crate::serve_config::PassthroughParams {
                            base_url: p.base_url.clone(),
                            auth: p.auth.clone(),
                            google_vertex: p.google_vertex.clone(),
                            model: p.model.clone(),
                        },
                    )
                    .map_err(ConfigError::InvalidProvider)?,
                );
                (serves, forwarder)
            }
            ProviderKindParams::GoogleVertex(google) => {
                let forwarder = Some(
                    crate::dialects::passthrough::PassthroughForwarder::new(
                        &crate::serve_config::PassthroughParams {
                            base_url: None,
                            auth: None,
                            google_vertex: Some(crate::serve_config::GoogleVertexPassthroughAuth {
                                project_id: google.project_id.clone(),
                                location: google.location.clone(),
                                credentials_file: google.credentials_file.clone(),
                            }),
                            model: google.model.clone(),
                        },
                    )
                    .map_err(ConfigError::InvalidProvider)?,
                );
                (vec![EndpointKind::ModelsGenerateContent], forwarder)
            }
            _ => {
                if typed.serve.is_some() {
                    return Err(ConfigError::InvalidProvider(format!(
                        "dialect provider {} cannot declare serve (chat endpoints are automatic)",
                        typed.model_id
                    )));
                }
                (Vec::new(), None)
            }
        };

        registry_entries.insert(
            normalized,
            RegistryEntry {
                model_id: typed.model_id.clone(),
                kind: typed.kind,
                label: typed.label.clone(),
                serves,
                forwarder,
            },
        );
    }

    Ok(Registry {
        entries: registry_entries,
    })
}

#[cfg(test)]
mod registry_tests {
    use super::{ExecutionMode, Registry, build_registry, execution_mode};
    use crate::serve_config::{EndpointKind, ProviderConfig, ProviderKind};

    fn typed_entry(json: &str) -> ProviderConfig {
        serde_json::from_str(json).expect("typed entry json")
    }

    fn chat_kind_entry(model_id: &str, kind: &str) -> ProviderConfig {
        let base = format!(
            r#"{{"type": "{kind}", "model_id": "{model_id}", "params": {{"base_url": "https://x.example.com/v1", "auth": {{"scheme": "bearer", "api_key": "k"}}, "model": "{model_id}"}}}}"#
        );
        typed_entry(&base)
    }

    #[test]
    fn build_registry_accepts_chat_and_passthrough_entries() {
        let entries = vec![
            chat_kind_entry("glm-5.3-flash", "openai-chat-completions"),
            chat_kind_entry("gpt-5.3-codex", "openai-responses"),
            typed_entry(
                r#"{"type": "passthrough", "model_id": "BAAI/bge-m3", "serve": ["/embeddings"],
                    "params": {"base_url": "http://127.0.0.1:8001/v1", "model": "BAAI/bge-m3"}}"#,
            ),
        ];
        let registry = build_registry(&entries).expect("registry");
        assert_eq!(registry.entry_count(), 3);
    }

    #[test]
    fn build_registry_rejects_duplicate_model_id_case_insensitive() {
        let entries = vec![
            chat_kind_entry("glm-5.3-flash", "openai-chat-completions"),
            chat_kind_entry("GLM-5.3-FLASH", "openai-responses"),
        ];
        let err = build_registry(&entries)
            .err()
            .expect("duplicate model_id must be rejected");
        assert!(err.to_string().contains("duplicate model_id"));
    }

    #[test]
    fn build_registry_rejects_unknown_serve_path() {
        let entries = vec![typed_entry(
            r#"{"type": "passthrough", "model_id": "x", "serve": ["/nope"],
                "params": {"base_url": "http://127.0.0.1:8001/v1", "model": "x"}}"#,
        )];
        let err = build_registry(&entries)
            .err()
            .expect("unknown serve path must be rejected");
        assert!(err.to_string().contains("unknown endpoint path"));
    }

    #[test]
    fn build_registry_rejects_passthrough_serving_chat_endpoint() {
        let entries = vec![typed_entry(
            r#"{"type": "passthrough", "model_id": "x", "serve": ["/messages"],
                "params": {"base_url": "http://127.0.0.1:8001/v1", "model": "x"}}"#,
        )];
        let err = build_registry(&entries)
            .err()
            .expect("chat endpoint hijack must be rejected");
        assert!(
            err.to_string()
                .contains("passthrough cannot serve chat endpoint")
        );
    }

    #[test]
    fn build_registry_rejects_passthrough_without_serve() {
        let entries = vec![typed_entry(
            r#"{"type": "passthrough", "model_id": "x",
                "params": {"base_url": "http://127.0.0.1:8001/v1", "model": "x"}}"#,
        )];
        let err = build_registry(&entries)
            .err()
            .expect("passthrough without serve must be rejected");
        assert!(err.to_string().contains("requires a serve list"));
    }

    #[test]
    fn build_registry_rejects_dialect_kind_with_serve() {
        let entries = vec![typed_entry(
            r#"{"type": "openai-chat-completions", "model_id": "glm", "serve": ["/embeddings"],
                "params": {"base_url": "https://x.example.com/v1",
                           "auth": {"scheme": "bearer", "api_key": "k"}, "model": "glm"}}"#,
        )];
        let err = build_registry(&entries)
            .err()
            .expect("dialect kind with serve must be rejected");
        assert!(err.to_string().contains("cannot declare serve"));
    }

    #[test]
    fn registry_entry_serves_chat_endpoints_for_dialects() {
        let entries = vec![chat_kind_entry("glm", "openai-chat-completions")];
        let registry = build_registry(&entries).expect("registry");
        let entry = registry.get("glm").expect("entry");
        assert!(entry.serves_endpoint(EndpointKind::ChatCompletions));
        assert!(entry.serves_endpoint(EndpointKind::Responses));
        assert!(entry.serves_endpoint(EndpointKind::Messages));
        assert!(!entry.serves_endpoint(EndpointKind::Embeddings));
        assert!(!entry.serves_endpoint(EndpointKind::SystemOne));
    }

    #[test]
    fn registry_entry_serves_only_listed_paths_for_passthrough() {
        let entries = vec![typed_entry(
            r#"{"type": "passthrough", "model_id": "bge", "serve": ["/embeddings", "/rerank"],
                "params": {"base_url": "http://127.0.0.1:8001/v1", "model": "bge"}}"#,
        )];
        let registry = build_registry(&entries).expect("registry");
        let entry = registry.get("bge").expect("entry");
        assert!(entry.serves_endpoint(EndpointKind::Embeddings));
        assert!(entry.serves_endpoint(EndpointKind::Rerank));
        assert!(!entry.serves_endpoint(EndpointKind::ChatCompletions));
        assert!(!entry.serves_endpoint(EndpointKind::ImagesEdits));
        assert_eq!(entry.kind, ProviderKind::Passthrough);
    }

    #[test]
    fn execution_mode_routes_by_kind_and_endpoint() {
        // dialect x chat endpoint -> AgentLoop
        assert_eq!(
            execution_mode(
                EndpointKind::ChatCompletions,
                ProviderKind::OpenAiChatCompletions
            ),
            ExecutionMode::AgentLoop
        );
        assert_eq!(
            execution_mode(EndpointKind::Responses, ProviderKind::OpenAiResponses),
            ExecutionMode::AgentLoop
        );
        assert_eq!(
            execution_mode(EndpointKind::Messages, ProviderKind::BedrockConverse),
            ExecutionMode::AgentLoop
        );
        // passthrough → Passthrough
        assert_eq!(
            execution_mode(EndpointKind::Embeddings, ProviderKind::Passthrough),
            ExecutionMode::Passthrough
        );
        // dialect x non-chat endpoint -> Passthrough
        assert_eq!(
            execution_mode(
                EndpointKind::Embeddings,
                ProviderKind::OpenAiChatCompletions
            ),
            ExecutionMode::Passthrough
        );
        assert_eq!(
            execution_mode(
                EndpointKind::ModelsGenerateContent,
                ProviderKind::GoogleVertex
            ),
            ExecutionMode::Passthrough
        );
    }

    #[test]
    fn registry_lookup_is_case_insensitive() {
        let entries = vec![chat_kind_entry("GLM-5.3-Flash", "openai-chat-completions")];
        let registry = build_registry(&entries).expect("registry");
        assert!(registry.get("glm-5.3-flash").is_some());
        assert!(registry.get("UNKNOWN").is_none());
        assert_eq!(registry.model_ids(), vec!["GLM-5.3-Flash"]);
    }

    #[test]
    fn empty_registry_is_valid() {
        let registry = build_registry(&[]).expect("empty registry");
        assert_eq!(registry.entry_count(), 0);
        assert_eq!(Registry::default().entry_count(), 0);
    }
}
