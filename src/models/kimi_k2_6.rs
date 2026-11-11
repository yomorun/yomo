//! Kimi K2.6 request behavior.
//!
//! Source: <https://cloud.tencent.com/document/product/1823/132232>

use crate::models::{ModelProfile, ModelProfileError};
use crate::openai_types::ChatCompletionRequest;
use crate::provider::{ReasoningEffort, ReasoningEffortLevel, UnifiedRequest};
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use super::profile_support::{
    is_chat_binding, preserve_raw_thinking, set_max_tokens_field, to_chat_request,
    unsupported_capability, validate_thinking_modes,
};

/// Kimi K2.6 profile for its exact public model name.
pub struct KimiK26Profile;

impl ModelProfile for KimiK26Profile {
    fn to_openai_chat_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        if !is_chat_binding(binding, Upstream::Tokenhub) {
            return Ok(None);
        }
        if request
            .options
            .reasoning_effort
            .as_ref()
            .is_some_and(is_named_effort)
        {
            return Err(unsupported_capability(
                "Kimi K2.6 uses thinking rather than reasoning_effort",
                "reasoning_effort",
            ));
        }
        validate_thinking_modes(
            request.options.thinking.as_ref(),
            &["enabled", "disabled"],
            "Kimi K2.6",
        )?;
        if let Some(thinking) = request.options.thinking.as_ref()
            && let Some(keep) = thinking.get("keep")
            && !keep.is_null()
            && keep.as_str() != Some("all")
        {
            return Err(unsupported_capability(
                "Kimi K2.6 supports only thinking.keep=`all`",
                "thinking.keep",
            ));
        }

        let mut chat_request = to_chat_request(request)?;
        set_max_tokens_field(&mut chat_request);
        preserve_raw_thinking(request, &mut chat_request);
        Ok(Some(chat_request))
    }
}

fn is_named_effort(effort: &ReasoningEffort) -> bool {
    matches!(
        effort,
        ReasoningEffort::Named(
            ReasoningEffortLevel::None
                | ReasoningEffortLevel::Minimal
                | ReasoningEffortLevel::Low
                | ReasoningEffortLevel::Medium
                | ReasoningEffortLevel::High
                | ReasoningEffortLevel::XHigh
                | ReasoningEffortLevel::Max
        )
    )
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

    use super::KimiK26Profile;

    fn binding() -> ModelBinding {
        ModelBinding {
            public_model: "kimi-k2.6".to_owned(),
            model: "kimi-k2.6".to_owned(),
            provider: "qcloud.cn".to_owned(),
            upstream: Upstream::Tokenhub,
            upstream_model: "kimi-k2.6".to_owned(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "qcloud.cn.kimi-k2.6".to_owned(),
        }
    }

    #[test]
    fn maps_thinking_keep_and_max_tokens_without_losing_extensions() {
        let request = api::request_from_value(
            &json!({
                "model":"kimi-k2.6",
                "messages":[{"role":"user","content":"hello"}],
                "max_tokens":16384,
                "thinking":{"type":"enabled","keep":"all"}
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse Kimi request");
        let mut converted = KimiK26Profile
            .to_openai_chat_request(&request, &binding())
            .expect("convert Kimi request")
            .expect("profile handles Chat");

        assert_eq!(converted.max_completion_tokens, None);
        assert_eq!(converted.extra.get("max_tokens"), Some(&json!(16384)));
        converted.normalize_reasoning_compat_fields();
        let wire = serde_json::to_value(converted).expect("serialize Kimi request");
        assert_eq!(wire["thinking"]["keep"], "all");
    }

    #[test]
    fn rejects_reasoning_effort_because_k26_uses_thinking() {
        let request = api::request_from_value(
            &json!({
                "model":"kimi-k2.6",
                "messages":[{"role":"user","content":"hello"}],
                "reasoning_effort":"high"
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse Kimi request");
        assert!(matches!(
            KimiK26Profile.to_openai_chat_request(&request, &binding()),
            Err(ModelProfileError::UnsupportedCapability { .. })
        ));
    }
}
