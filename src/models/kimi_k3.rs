//! Kimi K3 request behavior.
//!
//! Source: <https://cloud.tencent.com/document/product/1823/132232>

use crate::models::{ModelProfile, ModelProfileError};
use crate::openai_types::ChatCompletionRequest;
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use super::profile_support::{is_chat_binding, to_chat_request, unsupported_capability};

/// Kimi K3 profile for its exact public model name.
pub struct KimiK3Profile;

impl ModelProfile for KimiK3Profile {
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
                "Kimi K3 uses reasoning_effort; it does not accept the thinking parameter",
                "thinking",
            ));
        }
        to_chat_request(request).map(Some)
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

    use super::KimiK3Profile;

    fn binding() -> ModelBinding {
        ModelBinding {
            public_model: "kimi-k3".to_owned(),
            model: "kimi-k3".to_owned(),
            provider: "qcloud.cn".to_owned(),
            upstream: Upstream::Tokenhub,
            upstream_model: "kimi-k3".to_owned(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "qcloud.cn.kimi-k3".to_owned(),
        }
    }

    #[test]
    fn preserves_reasoning_effort_and_sampling_for_upstream_validation() {
        let request = api::request_from_value(
            &json!({
                "model":"kimi-k3",
                "messages":[{"role":"user","content":"hello"}],
                "reasoning_effort":"high",
                "temperature":0.4
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse Kimi request");
        let converted = KimiK3Profile
            .to_openai_chat_request(&request, &binding())
            .expect("convert Kimi request")
            .expect("profile handles Chat");

        assert_eq!(converted.temperature, Some(0.4));
        assert_eq!(
            converted
                .reasoning_effort
                .as_ref()
                .and_then(|effort| effort.as_str()),
            Some("high")
        );
    }

    #[test]
    fn rejects_the_unsupported_thinking_parameter() {
        let request = api::request_from_value(
            &json!({
                "model":"kimi-k3",
                "messages":[{"role":"user","content":"hello"}],
                "thinking":{"type":"enabled"}
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse Kimi request");
        assert!(matches!(
            KimiK3Profile.to_openai_chat_request(&request, &binding()),
            Err(ModelProfileError::UnsupportedCapability { .. })
        ));
    }
}
