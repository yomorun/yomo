//! DeepSeek V4 Pro request behavior.
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

/// DeepSeek V4 Pro profile shared by its public aliases.
pub struct DeepseekV4ProProfile;

impl ModelProfile for DeepseekV4ProProfile {
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
            "DeepSeek V4 Pro",
        )?;
        let mut chat_request = to_chat_request_with_supported_effort(request, SUPPORTED_EFFORTS)?;
        preserve_raw_thinking(request, &mut chat_request);
        Ok(Some(chat_request))
    }
}

#[cfg(test)]
mod tests {
    use crate::dialects::Dialect;
    use crate::models::ModelProfile;
    use crate::provider::{
        UnifiedContentPart, UnifiedInputItem, UnifiedInputItemKind, UnifiedRequest, UnifiedRole,
    };
    use crate::serve_config::ModelBinding;
    use crate::upstreams::Upstream;

    use super::DeepseekV4ProProfile;

    fn binding(public_model: &str) -> ModelBinding {
        ModelBinding {
            public_model: public_model.to_owned(),
            model: public_model.to_owned(),
            provider: "volcengine.cn".to_owned(),
            upstream: Upstream::VolcengineArk,
            upstream_model: "deepseek-v4-pro-ga-260813".to_owned(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "volcengine.cn.deepseek-v4-pro-0813".to_owned(),
        }
    }

    fn request(effort: &str) -> UnifiedRequest {
        let mut request = UnifiedRequest {
            input: vec![UnifiedInputItem::new(UnifiedInputItemKind::Message {
                role: UnifiedRole::User,
                content: vec![UnifiedContentPart::text("hello".into())],
                phase: None,
            })],
            ..Default::default()
        };
        request.options.reasoning_effort = Some(effort.into());
        request
    }

    #[test]
    fn pro_aliases_share_reasoning_conversion() {
        for public_model in ["deepseek-v4-pro", "deepseek-v4-pro-0813"] {
            let converted = DeepseekV4ProProfile
                .to_openai_chat_request(&request("medium"), &binding(public_model))
                .expect("convert Ark DeepSeek V4 Pro")
                .expect("profile handles its Chat binding");
            assert_eq!(
                converted
                    .reasoning_effort
                    .as_ref()
                    .and_then(|effort| effort.as_str()),
                Some("high")
            );
        }
    }
}
