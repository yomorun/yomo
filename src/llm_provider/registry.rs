use std::collections::HashMap;
use std::sync::Arc;

use async_trait::async_trait;
use futures_core::Stream;

use crate::llm_provider::Provider;
use crate::llm_provider::ProviderError;
use crate::llm_provider::UnifiedEvent;
use crate::llm_provider::UnifiedResponse;
use crate::llm_provider::openai_compatible::build_openai_compatible_provider;
use crate::llm_provider::selection::SelectionError;
use crate::llm_provider::selection::SelectionStrategy;
use crate::llm_provider::tokenhub::build_tokenhub_provider;
use crate::llm_provider::vertexai::build_vertexai_provider;
use crate::llm_provider::vllm_deepseek::build_vllm_deepseek_provider;
use crate::openai_types::ChatCompletionRequest;
use crate::provider_error_notifier::{ProviderErrorEvent, ProviderErrorNotifier};
use crate::serve_config::ConfigError;
use crate::serve_config::ProviderConfig;

#[derive(Clone)]
pub struct ProviderEntry {
    pub provider_type: String,
    pub model_id: String,
    pub label: Option<String>,
    pub provider: Arc<dyn Provider>,
}

#[derive(Clone)]
pub struct ProviderRegistry<M> {
    providers: HashMap<String, ProviderEntry>,
    strategy: Arc<dyn SelectionStrategy<M>>,
    error_notifier: Option<Arc<dyn ProviderErrorNotifier<M>>>,
}

impl<M> ProviderRegistry<M> {
    pub fn from_providers(
        providers: &[ProviderConfig],
        strategy: Arc<dyn SelectionStrategy<M>>,
    ) -> Result<Self, ConfigError> {
        let mut registry: HashMap<String, ProviderEntry> = HashMap::new();
        let mut model_ids = std::collections::HashSet::new();
        for item in providers {
            if item.provider_type.trim().is_empty() {
                return Err(ConfigError::InvalidProvider(format!(
                    "provider type is required for {}",
                    item.model_id
                )));
            }
            if item.model_id.trim().is_empty() {
                return Err(ConfigError::InvalidProvider(
                    "model_id is required for provider".to_string(),
                ));
            }
            let normalized_model_id = item.model_id.to_ascii_lowercase();
            if !model_ids.insert(normalized_model_id) {
                return Err(ConfigError::InvalidProvider(format!(
                    "duplicate model_id: {}",
                    item.model_id
                )));
            }
        }

        for item in providers {
            let provider: Arc<dyn Provider> = match item.provider_type.as_str() {
                "openai-compatible" => Arc::new(build_openai_compatible_provider(&item.params)?),
                "tokenhub" => Arc::new(build_tokenhub_provider(&item.params)?),
                "vllm_deepseek" => Arc::new(build_vllm_deepseek_provider(&item.params)?),
                "vertexai" => Arc::new(build_vertexai_provider(&item.params)?),
                other => return Err(ConfigError::UnknownProviderType(other.to_string())),
            };

            let entry = ProviderEntry {
                provider_type: item.provider_type.clone(),
                model_id: item.model_id.clone(),
                label: item.label.clone(),
                provider,
            };
            registry.insert(item.model_id.clone(), entry);
        }

        Ok(Self::new(registry, strategy))
    }

    pub fn new(
        providers: HashMap<String, ProviderEntry>,
        strategy: Arc<dyn SelectionStrategy<M>>,
    ) -> Self {
        Self {
            providers,
            strategy,
            error_notifier: None,
        }
    }

    pub fn with_error_notifier(mut self, notifier: Arc<dyn ProviderErrorNotifier<M>>) -> Self {
        self.error_notifier = Some(notifier);
        self
    }

    pub fn select(
        &self,
        model_id: Option<&str>,
        metadata: &M,
    ) -> Result<ProviderEntry, SelectionError>
    where
        M: Clone + Send + Sync + 'static,
    {
        let selected = self
            .strategy
            .select(model_id, metadata)
            .map_err(|err| err)?;
        let mut provider = self
            .providers
            .values()
            .find(|provider| {
                provider.model_id.to_ascii_lowercase() == selected.model_id.to_ascii_lowercase()
            })
            .cloned()
            .ok_or(SelectionError::ModelNotSupported)?;
        if let Some(notifier) = &self.error_notifier {
            provider.provider = Arc::new(HookedProvider {
                inner: Arc::clone(&provider.provider),
                error_notifier: Arc::clone(notifier),
                metadata: metadata.clone(),
                model_id: provider.model_id.clone(),
                endpoint: "/v1/chat/completions".to_string(),
            });
        }
        Ok(provider)
    }

    pub fn providers(&self) -> &HashMap<String, ProviderEntry> {
        &self.providers
    }

    pub fn model_list(&self) -> Vec<String> {
        self.providers
            .values()
            .map(|provider| provider.model_id.clone())
            .collect()
    }
}

#[derive(Clone)]
struct HookedProvider<M> {
    inner: Arc<dyn Provider>,
    error_notifier: Arc<dyn ProviderErrorNotifier<M>>,
    metadata: M,
    model_id: String,
    endpoint: String,
}

#[async_trait]
impl<M> Provider for HookedProvider<M>
where
    M: Clone + Send + Sync + 'static,
{
    fn model_id(&self) -> &str {
        self.inner.model_id()
    }

    async fn complete(
        &self,
        request: ChatCompletionRequest,
    ) -> Result<UnifiedResponse, ProviderError> {
        self.inner
            .complete(request)
            .await
            .map_err(|err| self.notify_error(err))
    }

    async fn stream<'a>(
        &'a self,
        request: ChatCompletionRequest,
    ) -> Result<
        std::pin::Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    > {
        self.inner
            .stream(request)
            .await
            .map_err(|err| self.notify_error(err))
    }
}

impl<M> HookedProvider<M>
where
    M: Clone + Send + Sync + 'static,
{
    fn notify_error(&self, err: ProviderError) -> ProviderError {
        let http_status = match &err {
            ProviderError::Public { status, .. } => Some(status.as_u16()),
            ProviderError::Internal(_) => None,
        };
        self.error_notifier
            .notify_provider_error(ProviderErrorEvent {
                model: Some(self.model_id.clone()),
                metadata: self.metadata.clone(),
                http_status,
                error: err.to_string(),
                endpoint: Some(self.endpoint.clone()),
            });
        err
    }
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use std::pin::Pin;
    use std::sync::Arc;

    use async_trait::async_trait;
    use axum::http::StatusCode;
    use futures_core::Stream;

    use super::{ProviderEntry, ProviderRegistry};
    use crate::llm_provider::selection::{ByModel, SelectionStrategy};
    use crate::llm_provider::{Provider, ProviderError, UnifiedEvent, UnifiedResponse};
    use crate::openai_types::{ChatCompletionRequest, ErrorDetail};
    use std::sync::Mutex;

    use crate::provider_error_notifier::{ProviderErrorEvent, ProviderErrorNotifier};

    struct FailingProvider;

    #[async_trait]
    impl Provider for FailingProvider {
        fn model_id(&self) -> &str {
            "demo-model"
        }

        async fn complete(
            &self,
            _request: ChatCompletionRequest,
        ) -> Result<UnifiedResponse, ProviderError> {
            Err(ProviderError::Public {
                status: StatusCode::PAYMENT_REQUIRED,
                error: ErrorDetail {
                    message: "upstream_402".to_string(),
                    r#type: "invalid_request_error".to_string(),
                    code: None,
                    param: None,
                },
            })
        }

        async fn stream<'a>(
            &'a self,
            _request: ChatCompletionRequest,
        ) -> Result<
            Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
            ProviderError,
        > {
            Err(ProviderError::Internal("stream_fail".to_string()))
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
    async fn provider_registry_error_notifier_receives_complete_error() {
        let mut providers = HashMap::new();
        providers.insert(
            "demo-model".to_string(),
            ProviderEntry {
                provider_type: "openai-compatible".to_string(),
                model_id: "demo-model".to_string(),
                label: Some("demo".to_string()),
                provider: Arc::new(FailingProvider),
            },
        );
        let notifier = Arc::new(CollectNotifier {
            calls: Mutex::new(Vec::new()),
        });
        let strategy: Arc<dyn SelectionStrategy<()>> = Arc::new(ByModel);
        let registry = ProviderRegistry::new(providers, strategy)
            .with_error_notifier(Arc::clone(&notifier) as Arc<dyn ProviderErrorNotifier<()>>);

        let entry = registry
            .select(Some("demo-model"), &())
            .expect("select provider");

        let err = entry
            .provider
            .complete(empty_request())
            .await
            .expect_err("provider should fail");
        match err {
            ProviderError::Public { status, .. } => {
                assert_eq!(status, StatusCode::PAYMENT_REQUIRED)
            }
            other => panic!("unexpected error: {other}"),
        }
        let calls = notifier.calls.lock().expect("lock calls");
        assert_eq!(calls.len(), 1);
        assert_eq!(calls[0].model.as_deref(), Some("demo-model"));
        assert_eq!(
            calls[0].http_status,
            Some(StatusCode::PAYMENT_REQUIRED.as_u16())
        );
        assert_eq!(calls[0].endpoint.as_deref(), Some("/v1/chat/completions"));
    }

    fn empty_request() -> ChatCompletionRequest {
        ChatCompletionRequest {
            model: "demo-model".to_string(),
            messages: Vec::new(),
            n: None,
            temperature: None,
            top_p: None,
            presence_penalty: None,
            frequency_penalty: None,
            logprobs: None,
            top_logprobs: None,
            modalities: None,
            audio: None,
            max_completion_tokens: None,
            stop: None,
            response_format: None,
            reasoning_effort: None,
            chat_template_kwargs: None,
            prediction: None,
            verbosity: None,
            tools: None,
            tool_choice: None,
            allowed_tools: None,
            parallel_tool_calls: None,
            service_tier: None,
            seed: None,
            stream: None,
            stream_options: None,
            metadata: None,
            agent_context: None,
            thinking: None,
        }
    }
}
