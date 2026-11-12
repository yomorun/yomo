//! GLM-5.3 request behavior.
//!
//! Sources:
//! - <https://docs.bigmodel.cn/cn/guide/models/text/glm-5.3>
//! - <https://ark.volcengine.com/region:cn-beijing/docs/chat-api>
//!
//! Binding: Volcengine Ark / Chat Completions.
//! Reasoning effort: low, high, max.
//! Thinking: always on; only `enabled` is accepted.
//! Sampling: top-k is removed before forwarding; text-only input and output.

use crate::dialects::Dialect;
use crate::models::{ModelProfile, ModelProfileError};
use crate::openai_types::ChatCompletionRequest;
use crate::provider::{ReasoningEffortLevel, UnifiedRequest};
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;
use serde_json::Value;

use super::profile_support::{
    has_non_text_content, to_chat_request_with_supported_effort, unsupported_capability,
};

/// GLM-5.3 documents `low`, `high`, and `max` reasoning effort.
const SUPPORTED_EFFORTS: &[ReasoningEffortLevel] = &[
    ReasoningEffortLevel::Low,
    ReasoningEffortLevel::High,
    ReasoningEffortLevel::Max,
];

/// GLM-5.3 profile for its exact public model name.
pub struct Glm53Profile;

impl ModelProfile for Glm53Profile {
    fn to_openai_chat_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        if binding.upstream != Upstream::VolcengineArk
            || binding.dialect != Some(Dialect::OpenAiChatCompletions)
        {
            return Ok(None);
        }

        validate_thinking(request.options.thinking.as_ref())?;
        if has_non_text_content(request) {
            return Err(unsupported_capability(
                "GLM-5.3 accepts text-only input",
                "messages",
            ));
        }
        if request.options.audio.is_some()
            || request
                .options
                .modalities
                .as_ref()
                .is_some_and(|modalities| {
                    modalities
                        .iter()
                        .any(|modality| !modality.eq_ignore_ascii_case("text"))
                })
        {
            return Err(unsupported_capability(
                "GLM-5.3 only supports text output",
                "modalities",
            ));
        }

        let mut chat_request = to_chat_request_with_supported_effort(request, SUPPORTED_EFFORTS)?;
        if let Some(thinking) = &request.options.thinking {
            chat_request.thinking = None;
            chat_request
                .extra
                .insert("thinking".to_owned(), thinking.clone());
        }
        // Chat sources keep `top_k` as a source extension and the conversion
        // merges it back, so remove it explicitly for this model.
        chat_request.extra.remove("top_k");
        Ok(Some(chat_request))
    }
}

fn validate_thinking(thinking: Option<&Value>) -> Result<(), ModelProfileError> {
    let Some(thinking) = thinking else {
        return Ok(());
    };
    let Some(fields) = thinking.as_object() else {
        return Err(ModelProfileError::InvalidRequest {
            message: "thinking must be an object".to_owned(),
            param: Some("thinking".to_owned()),
        });
    };
    let Some(mode) = fields.get("type").and_then(Value::as_str) else {
        return Err(ModelProfileError::InvalidRequest {
            message: "thinking.type is required when thinking is specified".to_owned(),
            param: Some("thinking.type".to_owned()),
        });
    };
    if mode == "enabled" {
        return Ok(());
    }
    Err(unsupported_capability(
        "GLM-5.3 always enables thinking and does not support disabling it",
        "thinking.type",
    ))
}

#[cfg(test)]
mod tests {
    use crate::api;
    use crate::dialects::Dialect;
    use crate::models::{ModelProfile, ModelProfileError, ModelProfileRegistry};
    use crate::provider::UnifiedSourceProtocol;
    use crate::serve_config::ModelBinding;
    use crate::upstreams::Upstream;
    use serde_json::json;

    use super::Glm53Profile;

    fn binding(upstream: Upstream) -> ModelBinding {
        ModelBinding {
            public_model: "glm-5.3".to_owned(),
            model: "glm-5.3".to_owned(),
            provider: match upstream {
                Upstream::VolcengineArk => "volcengine.cn",
                Upstream::Tokenhub => "qcloud.cn",
                _ => "other",
            }
            .to_owned(),
            upstream,
            upstream_model: "glm-5-3-260814".to_owned(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "glm-5.3".to_owned(),
        }
    }

    #[test]
    fn yomo_registry_selects_glm53_profile_for_its_exact_public_name() {
        let profiles = ModelProfileRegistry::default();
        let request = api::request_from_value(
            &json!({
                "model":"glm-5.3",
                "messages":[{"role":"user","content":"hello"}],
                "thinking":{"type":"enabled"},
                "reasoning_effort":"max"
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse GLM-5.3 request");

        let converted = profiles
            .resolve("glm-5.3")
            .to_openai_chat_request(&request, &binding(Upstream::VolcengineArk))
            .expect("convert GLM-5.3 request")
            .expect("Yomo registers the model-specific profile");
        assert_eq!(
            converted
                .reasoning_effort
                .as_ref()
                .and_then(|effort| effort.as_str()),
            Some("max")
        );
    }

    #[test]
    fn profile_is_scoped_by_upstream_and_rejects_disabled_or_multimodal_semantics() {
        let request = api::request_from_value(
            &json!({
                "model":"glm-5.3",
                "messages":[{"role":"user","content":"hello"}],
                "thinking":{"type":"enabled"}
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse GLM-5.3 request");
        assert!(
            Glm53Profile
                .to_openai_chat_request(&request, &binding(Upstream::Tokenhub))
                .expect("other upstream uses fallback")
                .is_none()
        );

        let disabled = api::request_from_value(
            &json!({
                "model":"glm-5.3",
                "messages":[{"role":"user","content":"hello"}],
                "thinking":{"type":"disabled"}
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse GLM-5.3 request");
        assert!(matches!(
            Glm53Profile.to_openai_chat_request(&disabled, &binding(Upstream::VolcengineArk)),
            Err(ModelProfileError::UnsupportedCapability { .. })
        ));

        let image = api::request_from_value(
            &json!({
                "model":"glm-5.3",
                "messages":[{
                    "role":"user",
                    "content":[
                        {"type":"text","text":"describe"},
                        {"type":"image_url","image_url":{"url":"https://example.com/image.png"}}
                    ]
                }]
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse GLM-5.3 request");
        assert!(matches!(
            Glm53Profile.to_openai_chat_request(&image, &binding(Upstream::VolcengineArk)),
            Err(ModelProfileError::UnsupportedCapability { .. })
        ));
    }

    #[test]
    fn rewrites_reasoning_effort_to_documented_levels() {
        for (input, expected) in [
            ("none", "low"),
            ("minimal", "low"),
            ("low", "low"),
            ("medium", "high"),
            ("high", "high"),
            ("xhigh", "max"),
            ("max", "max"),
            ("vendor-specific", "vendor-specific"),
        ] {
            let request = api::request_from_value(
                &json!({
                    "model":"glm-5.3",
                    "messages":[{"role":"user","content":"hello"}],
                    "thinking":{"type":"enabled"},
                    "reasoning_effort":input
                }),
                UnifiedSourceProtocol::ChatCompletions,
            )
            .expect("parse GLM-5.3 request");
            let converted = Glm53Profile
                .to_openai_chat_request(&request, &binding(Upstream::VolcengineArk))
                .expect("convert GLM-5.3 request")
                .expect("profile handles Chat");
            assert_eq!(
                converted
                    .reasoning_effort
                    .as_ref()
                    .and_then(|effort| effort.as_str()),
                Some(expected),
                "unexpected effort for {input}"
            );
        }
    }

    #[test]
    fn does_not_forward_the_removed_top_k_parameter() {
        for (protocol, value) in [
            (
                UnifiedSourceProtocol::ChatCompletions,
                json!({
                    "model":"glm-5.3",
                    "messages":[{"role":"user","content":"hello"}],
                    "thinking":{"type":"enabled"},
                    "top_k":40
                }),
            ),
            (
                UnifiedSourceProtocol::Messages,
                json!({
                    "model":"glm-5.3",
                    "max_tokens":100,
                    "messages":[{"role":"user","content":"hello"}],
                    "thinking":{"type":"enabled"},
                    "top_k":40
                }),
            ),
        ] {
            let request =
                api::request_from_value(&value, protocol).expect("parse GLM-5.3 client request");
            let converted = Glm53Profile
                .to_openai_chat_request(&request, &binding(Upstream::VolcengineArk))
                .expect("convert GLM-5.3 request")
                .expect("profile handles Chat");
            assert_eq!(
                converted.extra.get("top_k"),
                None,
                "top_k must not reach GLM-5.3"
            );
        }
    }
}
