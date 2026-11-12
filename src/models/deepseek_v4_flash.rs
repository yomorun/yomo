//! DeepSeek V4 Flash request behavior.
//!
//! Source: <https://ark.volcengine.com/region:cn-beijing/docs/deep-thinking>

use crate::models::{ModelProfile, ModelProfileError};
use crate::openai_types::ChatCompletionRequest;
use crate::provider::{ReasoningEffortLevel, UnifiedRequest};
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use super::profile_support::{
    is_chat_binding, preserve_raw_thinking, to_chat_request_with_supported_effort,
    validate_thinking_modes,
};

/// DeepSeek V4 documents `none`, `low`, `high`, and `max` reasoning effort.
const SUPPORTED_EFFORTS: &[ReasoningEffortLevel] = &[
    ReasoningEffortLevel::None,
    ReasoningEffortLevel::Low,
    ReasoningEffortLevel::High,
    ReasoningEffortLevel::Max,
];

/// DeepSeek V4 Flash profile for its exact public model name.
pub struct DeepseekV4FlashProfile;

impl ModelProfile for DeepseekV4FlashProfile {
    fn to_openai_chat_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        if !is_chat_binding(binding, Upstream::VolcengineArk) {
            return Ok(None);
        }

        validate_thinking_modes(
            request.options.thinking.as_ref(),
            &["enabled", "disabled"],
            "DeepSeek V4 Flash",
        )?;
        let mut chat_request = to_chat_request_with_supported_effort(request, SUPPORTED_EFFORTS)?;
        preserve_raw_thinking(request, &mut chat_request);
        Ok(Some(chat_request))
    }
}

#[cfg(test)]
mod tests {
    use crate::api;
    use crate::dialects::Dialect;
    use crate::models::ModelProfile;
    use crate::provider::UnifiedSourceProtocol;
    use crate::serve_config::ModelBinding;
    use crate::upstreams::Upstream;
    use serde_json::json;

    use super::DeepseekV4FlashProfile;

    #[test]
    fn maps_documented_v4_effort_levels_and_preserves_unlisted_values() {
        let binding = ModelBinding {
            public_model: "deepseek-v4-flash".to_owned(),
            model: "deepseek-v4-flash".to_owned(),
            provider: "volcengine.cn".to_owned(),
            upstream: Upstream::VolcengineArk,
            upstream_model: "deepseek-v4-flash-ga-260731".to_owned(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "volcengine.cn.deepseek-v4-flash".to_owned(),
        };
        for (input, expected) in [
            ("none", "none"),
            ("minimal", "low"),
            ("low", "low"),
            ("medium", "high"),
            ("high", "high"),
            ("xhigh", "max"),
            ("max", "max"),
        ] {
            let request = api::request_from_value(
                &json!({
                    "model":"deepseek-v4-flash",
                    "messages":[{"role":"user","content":"hello"}],
                    "reasoning_effort":input
                }),
                UnifiedSourceProtocol::ChatCompletions,
            )
            .expect("parse Chat request");
            let converted = DeepseekV4FlashProfile
                .to_openai_chat_request(&request, &binding)
                .expect("convert Ark request")
                .expect("profile handles Chat");
            assert_eq!(
                converted
                    .reasoning_effort
                    .as_ref()
                    .and_then(|effort| effort.as_str()),
                Some(expected)
            );
        }
    }
}
