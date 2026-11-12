//! DeepSeek V4 Flash Vision request behavior.
//!
//! Source: <https://cloud.tencent.com/document/product/1823/132248>

use crate::models::{ModelProfile, ModelProfileError};
use crate::openai_types::ChatCompletionRequest;
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use serde_json::json;

use super::profile_support::{
    is_chat_binding, mapped_effort, preserve_raw_thinking, to_chat_request, unsupported_capability,
    validate_thinking_modes,
};

/// DeepSeek V4 Flash Vision profile for its exact public model name.
pub struct DeepseekV4FlashVisionExpProfile;

impl ModelProfile for DeepseekV4FlashVisionExpProfile {
    fn to_openai_chat_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        if !is_chat_binding(binding, Upstream::Tokenhub) {
            return Ok(None);
        }

        let effort = mapped_effort(request, map_deepseek_v4_effort);
        if effort.as_ref().and_then(|effort| effort.as_str()) == Some("minimal") {
            return Err(unsupported_capability(
                "DeepSeek V4 does not document `minimal`; use `low`, `high`, `max`, or thinking.type",
                "reasoning_effort",
            ));
        }
        validate_thinking_modes(
            request.options.thinking.as_ref(),
            &["enabled", "disabled"],
            "DeepSeek V4 Flash Vision",
        )?;
        let mut chat_request = to_chat_request(request)?;
        preserve_raw_thinking(request, &mut chat_request);
        if effort.as_ref().and_then(|effort| effort.as_str()) == Some("none") {
            chat_request.reasoning_effort = None;
            if request.options.thinking.is_none() {
                chat_request.thinking = None;
                chat_request
                    .extra
                    .insert("thinking".to_owned(), json!({"type": "disabled"}));
            }
        } else if let Some(effort) = effort {
            chat_request.reasoning_effort = Some(effort);
        }
        Ok(Some(chat_request))
    }
}

fn map_deepseek_v4_effort(value: &str) -> Option<&'static str> {
    match value {
        "medium" | "xhigh" => Some("high"),
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

    use super::DeepseekV4FlashVisionExpProfile;

    #[test]
    fn maps_effort_and_represents_none_with_thinking_disabled() {
        let binding = ModelBinding {
            public_model: "deepseek-v4-flash-vision-exp".to_owned(),
            model: "deepseek-v4-flash-vision-exp".to_owned(),
            provider: "qcloud.cn".to_owned(),
            upstream: Upstream::Tokenhub,
            upstream_model: "deepseek/deepseek-v4-flash-vision-exp".to_owned(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "qcloud.cn.deepseek-v4-flash-vision-exp".to_owned(),
        };

        let medium = api::request_from_value(
            &json!({
                "model":"deepseek-v4-flash-vision-exp",
                "messages":[{"role":"user","content":"hello"}],
                "reasoning_effort":"medium"
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse medium request");
        let converted = DeepseekV4FlashVisionExpProfile
            .to_openai_chat_request(&medium, &binding)
            .expect("convert medium request")
            .expect("profile handles Chat");
        assert_eq!(
            converted
                .reasoning_effort
                .as_ref()
                .and_then(|effort| effort.as_str()),
            Some("high")
        );

        let none = api::request_from_value(
            &json!({
                "model":"deepseek-v4-flash-vision-exp",
                "messages":[{"role":"user","content":"hello"}],
                "reasoning_effort":"none"
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse no-thinking request");
        let converted = DeepseekV4FlashVisionExpProfile
            .to_openai_chat_request(&none, &binding)
            .expect("convert no-thinking request")
            .expect("profile handles Chat");
        assert_eq!(converted.reasoning_effort, None);
        assert_eq!(
            converted.extra.get("thinking"),
            Some(&json!({"type":"disabled"}))
        );
    }
}
