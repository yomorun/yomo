//! MiniMax M3 request behavior.
//!
//! Source: <https://cloud.tencent.com/document/product/1823/132246>

use crate::models::{ModelProfile, ModelProfileError};
use crate::openai_types::ChatCompletionRequest;
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use super::profile_support::{is_chat_binding, to_chat_request, validate_thinking_modes};

/// MiniMax M3 profile for its exact public model name.
pub struct MinimaxM3Profile;

impl ModelProfile for MinimaxM3Profile {
    fn to_openai_chat_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        if !is_chat_binding(binding, Upstream::Tokenhub) {
            return Ok(None);
        }
        validate_thinking_modes(
            request.options.thinking.as_ref(),
            &["adaptive", "disabled"],
            "MiniMax M3",
        )?;
        to_chat_request(request).map(Some)
    }
}

#[cfg(test)]
mod tests {
    use crate::api;
    use crate::dialects::Dialect;
    use crate::models::ModelProfile;
    use crate::openai_types::ThinkingType;
    use crate::provider::UnifiedSourceProtocol;
    use crate::serve_config::ModelBinding;
    use crate::upstreams::Upstream;
    use serde_json::json;

    use super::MinimaxM3Profile;

    #[test]
    fn preserves_documented_adaptive_thinking_and_reasoning_split() {
        let binding = ModelBinding {
            public_model: "minimax-m3".to_owned(),
            model: "minimax-m3".to_owned(),
            provider: "qcloud.cn".to_owned(),
            upstream: Upstream::Tokenhub,
            upstream_model: "minimax-m3".to_owned(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "qcloud.cn.minimax-m3".to_owned(),
        };
        let request = api::request_from_value(
            &json!({
                "model":"minimax-m3",
                "messages":[{"role":"user","content":"hello"}],
                "thinking":{"type":"adaptive"},
                "reasoning_split":true
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse MiniMax request");
        let converted = MinimaxM3Profile
            .to_openai_chat_request(&request, &binding)
            .expect("convert MiniMax request")
            .expect("profile handles Chat");
        assert_eq!(converted.extra.get("reasoning_split"), Some(&json!(true)));
        assert_eq!(
            converted.thinking.as_ref().map(|thinking| &thinking.kind),
            Some(&ThinkingType::Adaptive)
        );
    }
}
