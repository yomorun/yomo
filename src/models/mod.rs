//! Model-specific request normalization for the client conversation APIs.
//!
//! Keep the normalizer map and shared interfaces here. Add model-specific
//! implementations in their own files (one model per file, or a related group
//! of models per file) and register them through [`ModelRequestNormalizerMap`].

use std::collections::HashMap;
use std::sync::Arc;

use serde_json::Value;

use crate::provider::UnifiedRequest;

mod default;

use default::DefaultModelRequestNormalizer;

/// Client API whose request body is being normalized.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum ClientApi {
    /// OpenAI Chat Completions request.
    ChatCompletions,
    /// OpenAI Responses request.
    Responses,
    /// Anthropic Messages request.
    Messages,
}

/// Converts a model-specific client request body into the unified request IR.
pub trait ModelRequestNormalizer: Send + Sync {
    /// Normalize a Chat Completions request body.
    fn chat_completions(&self, model_id: &str, body: &Value) -> Result<UnifiedRequest, String>;

    /// Normalize a Responses request body.
    fn responses(&self, model_id: &str, body: &Value) -> Result<UnifiedRequest, String>;

    /// Normalize a Messages request body.
    fn messages(&self, model_id: &str, body: &Value) -> Result<UnifiedRequest, String>;
}

/// Model-id keyed collection of request normalizers with a default fallback.
#[derive(Clone)]
pub struct ModelRequestNormalizerMap {
    models: HashMap<String, Arc<dyn ModelRequestNormalizer>>,
    default: Arc<dyn ModelRequestNormalizer>,
}

impl ModelRequestNormalizerMap {
    /// Create a map that falls back to the built-in API request conversions.
    pub fn new() -> Self {
        Self {
            models: HashMap::new(),
            default: Arc::new(DefaultModelRequestNormalizer),
        }
    }

    /// Register a normalizer for an exact model id.
    pub fn register(
        &mut self,
        model_id: impl Into<String>,
        normalizer: Arc<dyn ModelRequestNormalizer>,
    ) -> Option<Arc<dyn ModelRequestNormalizer>> {
        self.models.insert(model_id.into(), normalizer)
    }

    /// Replace the normalizer used when a model has no explicit registration.
    pub fn set_default(&mut self, normalizer: Arc<dyn ModelRequestNormalizer>) {
        self.default = normalizer;
    }

    /// Normalize a request using the exact model-id match or the default.
    pub fn normalize(
        &self,
        api: ClientApi,
        model_id: &str,
        body: &Value,
    ) -> Result<UnifiedRequest, String> {
        let normalizer = self.models.get(model_id).unwrap_or(&self.default);
        match api {
            ClientApi::ChatCompletions => normalizer.chat_completions(model_id, body),
            ClientApi::Responses => normalizer.responses(model_id, body),
            ClientApi::Messages => normalizer.messages(model_id, body),
        }
    }
}

impl Default for ModelRequestNormalizerMap {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use serde_json::json;

    use super::{ClientApi, ModelRequestNormalizer, ModelRequestNormalizerMap};
    use crate::provider::UnifiedRequest;

    struct TestNormalizer(&'static str);

    impl ModelRequestNormalizer for TestNormalizer {
        fn chat_completions(
            &self,
            _model_id: &str,
            _body: &serde_json::Value,
        ) -> Result<UnifiedRequest, String> {
            Ok(UnifiedRequest {
                model: self.0.into(),
                ..Default::default()
            })
        }

        fn responses(
            &self,
            model_id: &str,
            body: &serde_json::Value,
        ) -> Result<UnifiedRequest, String> {
            self.chat_completions(model_id, body)
        }

        fn messages(
            &self,
            model_id: &str,
            body: &serde_json::Value,
        ) -> Result<UnifiedRequest, String> {
            self.chat_completions(model_id, body)
        }
    }

    #[test]
    fn model_normalizer_map_uses_exact_model_and_default_fallback() {
        let mut normalizers = ModelRequestNormalizerMap::default();
        normalizers.register("custom-model", Arc::new(TestNormalizer("custom")));

        let custom = normalizers
            .normalize(ClientApi::ChatCompletions, "custom-model", &json!({}))
            .unwrap();
        assert_eq!(custom.model, "custom");

        let fallback = normalizers
            .normalize(
                ClientApi::ChatCompletions,
                "other-model",
                &json!({"model":"other-model","messages":[]}),
            )
            .unwrap();
        assert_eq!(fallback.model, "other-model");
    }
}
