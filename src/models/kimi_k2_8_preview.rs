//! Kimi K2.8 Preview request behavior.
//!
//! Source: <https://cloud.tencent.com/document/product/1823/132232>

use crate::models::{ModelProfile, ModelProfileError};
use crate::openai_types::ChatCompletionRequest;
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use super::profile_support::{
    is_chat_binding, set_max_tokens_field, to_chat_request, unsupported_capability,
};

/// Kimi K2.8 Preview profile for its exact public model name.
pub struct KimiK28PreviewProfile;

impl ModelProfile for KimiK28PreviewProfile {
    fn to_openai_chat_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        if !is_chat_binding(binding, Upstream::Tokenhub) {
            return Ok(None);
        }
        if request.options.thinking.is_some() {
            return Err(unsupported_capability(
                "Kimi K2.8 Preview uses reasoning_effort; it does not accept the thinking parameter",
                "thinking",
            ));
        }

        let mut chat_request = to_chat_request(request)?;
        set_max_tokens_field(&mut chat_request);
        Ok(Some(chat_request))
    }
}

#[cfg(test)]
mod tests {
    use crate::api;
    use crate::dialects::Dialect;
    use crate::models::{ModelProfile, ModelProfileError};
    use crate::provider::UnifiedSourceProtocol;
    use crate::serve_config::ModelBinding;
    use crate::upstreams::Upstream;
    use serde_json::json;

    use super::KimiK28PreviewProfile;

    fn binding() -> ModelBinding {
        ModelBinding {
            public_model: "kimi-k2.8-preview".to_owned(),
            model: "kimi-k2.8-preview".to_owned(),
            provider: "qcloud.cn".to_owned(),
            upstream: Upstream::Tokenhub,
            upstream_model: "kimi-k2.8-preview".to_owned(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "qcloud.cn.kimi-k2.8-preview".to_owned(),
        }
    }

    #[test]
    fn encodes_the_model_specific_max_tokens_field() {
        let request = api::request_from_value(
            &json!({
                "model":"kimi-k2.8-preview",
                "messages":[{"role":"user","content":"hello"}],
                "max_tokens":32768,
                "reasoning_effort":"max"
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse Kimi request");
        let converted = KimiK28PreviewProfile
            .to_openai_chat_request(&request, &binding())
            .expect("convert Kimi request")
            .expect("profile handles Chat");

        assert_eq!(converted.max_completion_tokens, None);
        assert_eq!(converted.extra.get("max_tokens"), Some(&json!(32768)));
    }

    #[test]
    fn rejects_thinking_but_leaves_other_controls_for_the_upstream() {
        let thinking_request = api::request_from_value(
            &json!({
                "model":"kimi-k2.8-preview",
                "messages":[{"role":"user","content":"hello"}],
                "thinking":{"type":"enabled"}
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse Kimi request");
        assert!(matches!(
            KimiK28PreviewProfile.to_openai_chat_request(&thinking_request, &binding()),
            Err(ModelProfileError::UnsupportedCapability { .. })
        ));

        let sampling_request = api::request_from_value(
            &json!({
                "model":"kimi-k2.8-preview",
                "messages":[{"role":"user","content":"hello"}],
                "temperature":0.4
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse Kimi request");
        let converted = KimiK28PreviewProfile
            .to_openai_chat_request(&sampling_request, &binding())
            .expect("leave fixed sampling validation upstream")
            .expect("profile handles Chat");
        assert_eq!(converted.temperature, Some(0.4));
    }
}
