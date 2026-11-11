//! GLM-5.2 request behavior.
//!
//! Source: <https://cloud.tencent.com/document/product/1823/132061>

use crate::models::{ModelProfile, ModelProfileError};
use crate::openai_types::ChatCompletionRequest;
use crate::provider::{
    UnifiedContentPart, UnifiedContentPartKind, UnifiedInputItemKind, UnifiedRequest,
    UnifiedToolOutput,
};
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use super::profile_support::{
    is_chat_binding, map_reasoning_effort, preserve_raw_thinking, to_chat_request,
    unsupported_capability, validate_thinking_modes,
};

/// GLM-5.2 profile for its exact public model name.
pub struct Glm52Profile;

impl ModelProfile for Glm52Profile {
    fn to_openai_chat_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        if !is_chat_binding(binding, Upstream::Tokenhub) {
            return Ok(None);
        }

        let normalized = map_reasoning_effort(request, map_glm52_effort);
        validate_thinking_modes(
            normalized.options.thinking.as_ref(),
            &["enabled", "disabled"],
            "GLM-5.2",
        )?;
        if has_non_text_content(&normalized) {
            return Err(unsupported_capability(
                "GLM-5.2 accepts text-only input",
                "messages",
            ));
        }
        if normalized.options.audio.is_some()
            || normalized
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
                "GLM-5.2 only supports text output",
                "modalities",
            ));
        }

        let mut chat_request = to_chat_request(&normalized)?;
        preserve_raw_thinking(&normalized, &mut chat_request);
        Ok(Some(chat_request))
    }
}

fn map_glm52_effort(value: &str) -> Option<&'static str> {
    match value {
        "minimal" => Some("none"),
        "low" | "medium" => Some("high"),
        "xhigh" => Some("max"),
        _ => None,
    }
}

fn has_non_text_content(request: &UnifiedRequest) -> bool {
    let has_non_text_part = |parts: &[UnifiedContentPart]| {
        parts.iter().any(|part| {
            !matches!(
                &part.kind,
                UnifiedContentPartKind::Text { .. }
                    | UnifiedContentPartKind::Refusal { .. }
                    | UnifiedContentPartKind::Thinking { .. }
                    | UnifiedContentPartKind::RedactedThinking { .. }
            )
        })
    };

    request
        .instructions
        .iter()
        .flatten()
        .chain(&request.input)
        .any(|item| match &item.kind {
            UnifiedInputItemKind::Message { content, .. } => has_non_text_part(content),
            UnifiedInputItemKind::FunctionCallOutput {
                output: UnifiedToolOutput::Content(content),
                ..
            } => has_non_text_part(content),
            UnifiedInputItemKind::Reasoning {
                content: Some(content),
                ..
            } => has_non_text_part(content),
            UnifiedInputItemKind::Other { .. } => true,
            _ => false,
        })
}

#[cfg(test)]
mod tests {
    use crate::api;
    use crate::dialects::Dialect;
    use crate::models::{ModelProfile, ModelProfileError};
    use crate::openai_types::ChatCompletionRequest;
    use crate::provider::UnifiedSourceProtocol;
    use crate::serve_config::ModelBinding;
    use crate::upstreams::Upstream;
    use serde_json::{Value, json};

    use super::Glm52Profile;

    fn convert(value: &Value) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        let request = api::request_from_value(value, UnifiedSourceProtocol::ChatCompletions)
            .expect("parse Chat request");
        Glm52Profile.to_openai_chat_request(&request, &binding())
    }

    fn binding() -> ModelBinding {
        ModelBinding {
            public_model: "glm-5.2".to_owned(),
            model: "glm-5.2".to_owned(),
            provider: "qcloud.cn".to_owned(),
            upstream: Upstream::Tokenhub,
            upstream_model: "glm-5.2".to_owned(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "qcloud.cn.glm-5.2".to_owned(),
        }
    }

    #[test]
    fn maps_glm52_effort_values_without_changing_unlisted_values() {
        for (input, expected) in [
            ("low", "high"),
            ("medium", "high"),
            ("xhigh", "max"),
            ("none", "none"),
        ] {
            let converted = convert(&json!({
                "model":"glm-5.2",
                "messages":[{"role":"user","content":"hello"}],
                "reasoning_effort":input
            }))
            .expect("convert TokenHub GLM request")
            .expect("profile handles Chat");
            assert_eq!(
                converted
                    .reasoning_effort
                    .as_ref()
                    .and_then(|effort| effort.as_str()),
                Some(expected)
            );
        }

        let converted = convert(&json!({
            "model":"glm-5.2",
            "messages":[{"role":"user","content":"hello"}],
            "reasoning_effort":"vendor-specific"
        }))
        .expect("preserve an unlisted effort")
        .expect("profile handles Chat");
        assert_eq!(
            converted
                .reasoning_effort
                .as_ref()
                .and_then(|effort| effort.as_str()),
            Some("vendor-specific")
        );
    }

    #[test]
    fn rejects_documented_unsupported_multimodal_input() {
        let result = convert(&json!({
            "model":"glm-5.2",
            "messages":[{
                "role":"user",
                "content":[
                    {"type":"text","text":"describe this"},
                    {"type":"image_url","image_url":{"url":"https://example.com/image.png"}}
                ]
            }]
        }));
        assert!(matches!(
            result,
            Err(ModelProfileError::UnsupportedCapability { .. })
        ));
    }
}
