//! DeepSeek V4.1 Flash request behavior.
//!
//! Source: <https://ark.volcengine.com/region:cn-beijing/docs/deep-thinking>

use crate::models::{ModelProfile, ModelProfileError};
use crate::openai_types::ChatCompletionRequest;
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use super::profile_support::{
    is_chat_binding, map_reasoning_effort, preserve_raw_thinking, to_chat_request,
    validate_thinking_modes,
};

/// DeepSeek V4.1 Flash profile for its exact public model name.
pub struct DeepseekV41FlashProfile;

impl ModelProfile for DeepseekV41FlashProfile {
    fn to_openai_chat_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        if !is_chat_binding(binding, Upstream::VolcengineArk) {
            return Ok(None);
        }

        let normalized = map_reasoning_effort(request, map_v41_effort);
        validate_thinking_modes(
            normalized.options.thinking.as_ref(),
            &["enabled", "disabled"],
            "DeepSeek V4.1 Flash",
        )?;
        let mut chat_request = to_chat_request(&normalized)?;
        preserve_raw_thinking(&normalized, &mut chat_request);
        Ok(Some(chat_request))
    }
}

fn map_v41_effort(value: &str) -> Option<&'static str> {
    match value {
        "minimal" => Some("low"),
        "medium" | "xhigh" => Some("high"),
        "ultra" => Some("max"),
        _ => None,
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

    use super::DeepseekV41FlashProfile;

    #[test]
    fn maps_documented_effort_aliases_for_ark_v41_flash() {
        let binding = ModelBinding {
            public_model: "deepseek-v4.1-flash".to_owned(),
            model: "deepseek-v4.1-flash".to_owned(),
            provider: "volcengine.cn".to_owned(),
            upstream: Upstream::VolcengineArk,
            upstream_model: "deepseek-v4-1-flash-261017".to_owned(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "volcengine.cn.deepseek-v4.1-flash".to_owned(),
        };
        for (input, expected) in [
            ("minimal", "low"),
            ("medium", "high"),
            ("xhigh", "high"),
            ("ultra", "max"),
        ] {
            let request = api::request_from_value(
                &json!({
                    "model":"deepseek-v4.1-flash",
                    "messages":[{"role":"user","content":"hello"}],
                    "reasoning_effort":input
                }),
                UnifiedSourceProtocol::ChatCompletions,
            )
            .expect("parse Chat request");
            let converted = DeepseekV41FlashProfile
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
