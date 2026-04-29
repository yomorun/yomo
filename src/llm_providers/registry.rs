use std::collections::HashMap;
use std::sync::Arc;

use crate::serve_config::ConfigError;
use crate::serve_config::ProviderConfig;
use crate::llm_providers::Provider;
use crate::llm_providers::openai::build_openai_provider;
use crate::llm_providers::selection::SelectionStrategy;

#[derive(Clone)]
pub struct ProviderEntry {
    pub provider_type: String,
    pub model_id: String,
    pub provider: Arc<dyn Provider>,
}

pub struct ProviderRegistry<M> {
    providers: HashMap<String, ProviderEntry>,
    strategy: Arc<dyn SelectionStrategy<M>>,
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
                "openai" => Arc::new(build_openai_provider(&item.params)?),
                other => return Err(ConfigError::UnknownProviderType(other.to_string())),
            };

            let entry = ProviderEntry {
                provider_type: item.provider_type.clone(),
                model_id: item.model_id.clone(),
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
        }
    }

    pub fn select(
        &self,
        model_id: Option<&str>,
        metadata: &M,
    ) -> Result<
        (crate::llm_providers::selection::SelectionResult, ProviderEntry),
        crate::llm_providers::selection::SelectionError,
    > {
        let selected = self
            .strategy
            .select(model_id, metadata)
            .map_err(|err| err)?;
        let provider = self
            .providers
            .values()
            .find(|provider| {
                provider.model_id.to_ascii_lowercase()
                    == selected.model_id.to_ascii_lowercase()
            })
            .cloned()
            .ok_or(crate::llm_providers::selection::SelectionError::ModelNotSupported)?;
        Ok((selected, provider))
    }

    pub fn providers(&self) -> &HashMap<String, ProviderEntry> {
        &self.providers
    }
}
