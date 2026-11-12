//! Gemini 3.8 Flash request behavior.
//!
//! Gemini 3.8 Flash accepts only `thinkingLevel` (`low`, `medium`, `high`);
//! `minimal` is documented as unsupported and `thinkingBudget` is deprecated
//! for Gemini 3 and later.
//!
//! Sources:
//! - <https://cloud.google.com/vertex-ai/generative-ai/docs/thinking>
//! - <https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/3-8-flash>

use log::debug;

use crate::dialects::Dialect;
use crate::dialects::google_vertex::types::VertexGenerateContentRequest;
use crate::models::{ModelProfile, ModelProfileError};
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use super::profile_support::{
    ends_with_assistant_prefill, gemini_clamp_output_tokens, gemini_level_for_effort,
    gemini_thinking_config, gemini_thinking_config_for_level,
};

const GEMINI_38_FLASH_MODEL: &str = "gemini-3.8-flash";
const MAX_OUTPUT_TOKENS: i32 = 65_536;

/// Gemini 3.8 Flash request behavior.
///
/// The registry also applies this profile to the public aliases routed to
/// `gemini-3.8-flash`; the profile only claims a binding whose upstream model is
/// 3.8 Flash.
pub struct Gemini38FlashProfile;

impl ModelProfile for Gemini38FlashProfile {
    fn adapt_gemini_generate_content_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
        dialect_request: &mut VertexGenerateContentRequest,
    ) -> Result<(), ModelProfileError> {
        if !is_gemini_binding(binding) {
            return Ok(());
        }
        if ends_with_assistant_prefill(request) {
            return Err(ModelProfileError::InvalidRequest {
                message: format!(
                    "{GEMINI_38_FLASH_MODEL} does not support prefilled model turns; the conversation must end with a user or tool-result turn"
                ),
                param: Some("messages".to_owned()),
            });
        }

        let Some(generation) = dialect_request.generation_config.as_mut() else {
            return Ok(());
        };
        let existing_include_thoughts = generation
            .thinking_config
            .as_ref()
            .and_then(|config| config.include_thoughts);

        let effort = request.options.reasoning_effort.as_ref();
        let thinking = request.options.thinking.as_ref();
        if effort.is_some() && thinking.is_some() {
            return Err(ModelProfileError::InvalidRequest {
                message: format!(
                    "{GEMINI_38_FLASH_MODEL} accepts only one of reasoning_effort or thinking"
                ),
                param: Some("reasoning_effort".to_owned()),
            });
        }
        if let Some(effort) = effort {
            let level = gemini_level_for_effort(effort)?;
            debug!(
                "gemini profile: model={} effort={:?} -> thinking_level={:?}",
                GEMINI_38_FLASH_MODEL, effort, level
            );
            generation.thinking_config = Some(gemini_thinking_config_for_level(
                level,
                existing_include_thoughts,
            ));
        } else if let Some(thinking) = thinking {
            generation.thinking_config =
                gemini_thinking_config(thinking, existing_include_thoughts)?;
        }

        gemini_clamp_output_tokens(generation, MAX_OUTPUT_TOKENS, GEMINI_38_FLASH_MODEL);
        Ok(())
    }
}

fn is_gemini_binding(binding: &ModelBinding) -> bool {
    binding.upstream == Upstream::GoogleVertex
        && binding.dialect == Some(Dialect::GeminiGenerateContent)
        && binding.upstream_model == GEMINI_38_FLASH_MODEL
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use crate::api;
    use crate::dialects::google_vertex::types::{
        VertexGenerateContentRequest, VertexGenerationConfig, VertexThinkingConfig,
        VertexThinkingLevel,
    };
    use crate::models::{ModelProfile, ModelProfileError};
    use crate::provider::{UnifiedRequest, UnifiedSourceProtocol};
    use crate::serve_config::ModelBinding;
    use crate::upstreams::Upstream;

    use super::Gemini38FlashProfile;

    fn binding(upstream_model: &str) -> ModelBinding {
        ModelBinding {
            public_model: "gemini-3.8-flash".to_owned(),
            model: "gemini-3.8-flash".to_owned(),
            provider: "google.global".to_owned(),
            upstream: Upstream::GoogleVertex,
            upstream_model: upstream_model.to_owned(),
            dialect: Some(crate::dialects::Dialect::GeminiGenerateContent),
            label: format!("google.global.{upstream_model}"),
        }
    }

    fn chat_request(value: serde_json::Value) -> UnifiedRequest {
        api::request_from_value(&value, UnifiedSourceProtocol::ChatCompletions)
            .expect("parse Chat request")
    }

    fn base_body(max_output_tokens: Option<i32>) -> VertexGenerateContentRequest {
        VertexGenerateContentRequest {
            contents: Vec::new(),
            system_instruction: None,
            generation_config: Some(VertexGenerationConfig {
                max_output_tokens,
                ..Default::default()
            }),
            tools: None,
            tool_config: None,
        }
    }

    fn adapt_38(
        request: &UnifiedRequest,
    ) -> Result<VertexGenerateContentRequest, ModelProfileError> {
        let mut body = base_body(None);
        Gemini38FlashProfile.adapt_gemini_generate_content_request(
            request,
            &binding("gemini-3.8-flash"),
            &mut body,
        )?;
        Ok(body)
    }

    fn thinking(body: VertexGenerateContentRequest) -> Option<VertexThinkingConfig> {
        body.generation_config
            .and_then(|config| config.thinking_config)
    }

    #[test]
    fn maps_documented_effort_levels_to_thinking_levels() {
        for (effort, expected) in [
            ("none", VertexThinkingLevel::Low),
            ("minimal", VertexThinkingLevel::Low),
            ("low", VertexThinkingLevel::Low),
            ("medium", VertexThinkingLevel::Medium),
            ("high", VertexThinkingLevel::High),
            ("xhigh", VertexThinkingLevel::High),
            ("max", VertexThinkingLevel::High),
        ] {
            let request = chat_request(json!({
                "model": "gemini-3.8-flash",
                "messages": [{"role": "user", "content": "hello"}],
                "reasoning_effort": effort
            }));
            let config = thinking(adapt_38(&request).expect("convert effort")).expect("level");
            assert_eq!(config.thinking_level, Some(expected), "{effort}");
            assert_eq!(config.thinking_budget, None, "{effort}");
        }
    }

    #[test]
    fn maps_thinking_budget_to_level_and_keeps_include_thoughts() {
        for (budget, expected) in [
            (0, Some(VertexThinkingLevel::Low)),
            (1024, Some(VertexThinkingLevel::Low)),
            (1025, Some(VertexThinkingLevel::Medium)),
            (8192, Some(VertexThinkingLevel::Medium)),
            (8193, Some(VertexThinkingLevel::High)),
        ] {
            let request = chat_request(json!({
                "model": "gemini-3.8-flash",
                "messages": [{"role": "user", "content": "hello"}],
                "thinking": {"type": "enabled", "budget_tokens": budget, "include_thoughts": false}
            }));
            let config = thinking(adapt_38(&request).expect("convert budget")).expect("level");
            assert_eq!(config.thinking_level, expected, "{budget}");
            assert_eq!(config.thinking_budget, None, "{budget}");
            assert_eq!(config.include_thoughts, Some(false), "{budget}");
        }

        let dynamic = chat_request(json!({
            "model": "gemini-3.8-flash",
            "messages": [{"role": "user", "content": "hello"}],
            "thinking": {"type": "enabled", "budget_tokens": -1}
        }));
        assert!(
            thinking(adapt_38(&dynamic).expect("dynamic budget")).is_none(),
            "dynamic thinking must leave the model default"
        );
    }

    #[test]
    fn cannot_disable_thinking_so_low_is_the_floor() {
        let disabled = chat_request(json!({
            "model": "gemini-3.8-flash",
            "messages": [{"role": "user", "content": "hello"}],
            "thinking": {"type": "disabled"}
        }));
        let config = thinking(adapt_38(&disabled).expect("convert disabled")).expect("level");
        assert_eq!(config.thinking_level, Some(VertexThinkingLevel::Low));
    }

    #[test]
    fn clamps_output_tokens_to_the_model_limit() {
        let request = chat_request(json!({
            "model": "gemini-3.8-flash",
            "messages": [{"role": "user", "content": "hello"}],
            "max_tokens": 128000
        }));
        let mut body = base_body(Some(128_000));
        Gemini38FlashProfile
            .adapt_gemini_generate_content_request(
                &request,
                &binding("gemini-3.8-flash"),
                &mut body,
            )
            .expect("clamp output");
        assert_eq!(
            body.generation_config.expect("config").max_output_tokens,
            Some(65_536)
        );
    }

    #[test]
    fn rejects_a_prefilled_assistant_turn() {
        let request = chat_request(json!({
            "model": "gemini-3.8-flash",
            "messages": [
                {"role": "user", "content": "hello"},
                {"role": "assistant", "content": "prefill"}
            ]
        }));
        assert!(matches!(
            adapt_38(&request),
            Err(ModelProfileError::InvalidRequest { .. })
        ));
    }

    #[test]
    fn ignores_bindings_that_are_not_the_claimed_upstream() {
        let request = chat_request(json!({
            "model": "gemini-3.7-flash",
            "messages": [{"role": "user", "content": "hello"}],
            "reasoning_effort": "minimal"
        }));
        let mut body = base_body(Some(128_000));
        Gemini38FlashProfile
            .adapt_gemini_generate_content_request(
                &request,
                &binding("gemini-3.7-flash"),
                &mut body,
            )
            .expect("foreign binding falls back");
        assert_eq!(
            body.generation_config
                .as_ref()
                .expect("config")
                .max_output_tokens,
            Some(128_000)
        );
        assert!(thinking(body).is_none());
    }
}
