//! Built-in model request normalization, preserving current API behavior.

use serde_json::Value;

use crate::anthropic_messages_types::AnthropicMessagesRequest;
use crate::models::ModelRequestNormalizer;
use crate::openai_types::ChatCompletionRequest;
use crate::provider::UnifiedRequest;

pub(super) struct DefaultModelRequestNormalizer;

impl ModelRequestNormalizer for DefaultModelRequestNormalizer {
    fn chat_completions(&self, _model_id: &str, body: &Value) -> Result<UnifiedRequest, String> {
        let mut native: ChatCompletionRequest = serde_json::from_value(body.clone())
            .map_err(|err| format!("invalid request body: {err}"))?;
        native.normalize_reasoning_compat_fields();
        crate::dialects::openai_chat_completions::mapper::request_from_chat(&native)
            .map_err(|err| err.to_string())
    }

    fn responses(&self, _model_id: &str, body: &Value) -> Result<UnifiedRequest, String> {
        crate::api::responses::request_from_value(body)
    }

    fn messages(&self, _model_id: &str, body: &Value) -> Result<UnifiedRequest, String> {
        let _: AnthropicMessagesRequest = serde_json::from_value(body.clone())
            .map_err(|err| format!("invalid request body: {err}"))?;
        crate::api::messages::request_from_value(body)
    }
}
