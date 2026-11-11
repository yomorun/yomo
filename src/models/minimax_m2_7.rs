//! MiniMax M2.7 request behavior.
//!
//! Source: <https://cloud.tencent.com/document/product/1823/132246>

use crate::models::{ModelProfile, ModelProfileError};
use crate::openai_types::ChatCompletionRequest;
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use serde_json::Value;

use super::profile_support::{is_chat_binding, to_chat_request, unsupported_capability};

/// MiniMax M2.7 profile for its exact public model name.
pub struct MinimaxM27Profile;

impl ModelProfile for MinimaxM27Profile {
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
            .thinking
            .as_ref()
            .and_then(|thinking| thinking.get("type"))
            .and_then(Value::as_str)
            == Some("disabled")
        {
            return Err(unsupported_capability(
                "MiniMax M2.7 cannot disable thinking",
                "thinking.type",
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

    use super::MinimaxM27Profile;

    #[test]
    fn rejects_a_request_to_disable_always_on_m2_7_thinking() {
        let binding = ModelBinding {
            public_model: "minimax-m2.7".to_owned(),
            model: "minimax-m2.7".to_owned(),
            provider: "qcloud.cn".to_owned(),
            upstream: Upstream::Tokenhub,
            upstream_model: "minimax-m2.7".to_owned(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "qcloud.cn.minimax-m2.7".to_owned(),
        };
        let request = api::request_from_value(
            &json!({
                "model":"minimax-m2.7",
                "messages":[{"role":"user","content":"hello"}],
                "thinking":{"type":"disabled"}
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse MiniMax request");
        assert!(matches!(
            MinimaxM27Profile.to_openai_chat_request(&request, &binding),
            Err(ModelProfileError::UnsupportedCapability { .. })
        ));
    }
}
