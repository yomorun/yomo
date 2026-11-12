//! Gemini 4 Argon request behavior.
//!
//! Sources:
//! - <https://cloud.google.com/vertex-ai/generative-ai/docs/thinking>
//! - <https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-argon>
//!
//! Binding: Google Vertex / Gemini generateContent.
//! Reasoning effort: low, medium, high; `none`/`minimal` map to low, `xhigh`/`max` to high.
//! Thinking: only `thinkingLevel`; `enabled`/budget values map to a level.
//! Output: capped at 65,536 tokens streaming and 32,768 tokens non-streaming.
//! Conversation: assistant-message prefill is rejected.

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

const GEMINI_4_ARGON_MODEL: &str = "gemini-4-argon";
const MAX_OUTPUT_TOKENS: i32 = 65_536;
const NON_STREAM_MAX_OUTPUT_TOKENS: i32 = 32_768;

/// Gemini 4 Argon request behavior.
pub struct Gemini4ArgonProfile;

impl ModelProfile for Gemini4ArgonProfile {
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
                    "{GEMINI_4_ARGON_MODEL} does not support prefilled model turns; the conversation must end with a user or tool-result turn"
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
                    "{GEMINI_4_ARGON_MODEL} accepts only one of reasoning_effort or thinking"
                ),
                param: Some("reasoning_effort".to_owned()),
            });
        }
        if let Some(effort) = effort {
            let level = gemini_level_for_effort(effort)?;
            debug!(
                "gemini profile: model={} effort={:?} -> thinking_level={:?}",
                GEMINI_4_ARGON_MODEL, effort, level
            );
            generation.thinking_config = Some(gemini_thinking_config_for_level(
                level,
                existing_include_thoughts,
            ));
        } else if let Some(thinking) = thinking {
            generation.thinking_config =
                gemini_thinking_config(thinking, existing_include_thoughts)?;
        }

        let cap = if request.controls.stream.unwrap_or(false) {
            MAX_OUTPUT_TOKENS
        } else {
            NON_STREAM_MAX_OUTPUT_TOKENS
        };
        gemini_clamp_output_tokens(generation, cap, GEMINI_4_ARGON_MODEL);
        Ok(())
    }
}

fn is_gemini_binding(binding: &ModelBinding) -> bool {
    binding.upstream == Upstream::GoogleVertex
        && binding.dialect == Some(Dialect::GeminiGenerateContent)
        && binding.upstream_model == GEMINI_4_ARGON_MODEL
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use crate::api;
    use crate::dialects::google_vertex::types::{
        VertexGenerateContentRequest, VertexGenerationConfig, VertexThinkingLevel,
    };
    use crate::models::{ModelProfile, ModelProfileError};
    use crate::provider::{UnifiedRequest, UnifiedSourceProtocol};
    use crate::serve_config::ModelBinding;
    use crate::upstreams::Upstream;

    use super::Gemini4ArgonProfile;

    fn binding(upstream_model: &str) -> ModelBinding {
        ModelBinding {
            public_model: "gemini-4-argon".to_owned(),
            model: "gemini-4-argon".to_owned(),
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

    #[test]
    fn maps_documented_effort_levels_to_argon_levels() {
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
                "model": "gemini-4-argon",
                "messages": [{"role": "user", "content": "hello"}],
                "reasoning_effort": effort
            }));
            let mut body = base_body(None);
            Gemini4ArgonProfile
                .adapt_gemini_generate_content_request(
                    &request,
                    &binding("gemini-4-argon"),
                    &mut body,
                )
                .expect("convert effort");
            let config = body
                .generation_config
                .and_then(|config| config.thinking_config)
                .expect("level");
            assert_eq!(config.thinking_level, Some(expected), "{effort}");
            assert_eq!(config.thinking_budget, None, "{effort}");
        }
    }

    #[test]
    fn caps_non_streaming_output_but_allows_streaming() {
        for (stream, expected) in [(false, 32_768), (true, 65_536)] {
            let request = chat_request(json!({
                "model": "gemini-4-argon",
                "messages": [{"role": "user", "content": "hello"}],
                "max_tokens": 100_000,
                "stream": stream
            }));
            let mut body = base_body(Some(100_000));
            Gemini4ArgonProfile
                .adapt_gemini_generate_content_request(
                    &request,
                    &binding("gemini-4-argon"),
                    &mut body,
                )
                .expect("clamp output");
            assert_eq!(
                body.generation_config.expect("config").max_output_tokens,
                Some(expected),
                "stream={stream}"
            );
        }
    }

    #[test]
    fn rejects_a_prefilled_assistant_turn() {
        let request = chat_request(json!({
            "model": "gemini-4-argon",
            "messages": [
                {"role": "user", "content": "hello"},
                {"role": "assistant", "content": "prefill"}
            ]
        }));
        let mut body = base_body(None);
        assert!(matches!(
            Gemini4ArgonProfile.adapt_gemini_generate_content_request(
                &request,
                &binding("gemini-4-argon"),
                &mut body
            ),
            Err(ModelProfileError::InvalidRequest { .. })
        ));
    }

    #[test]
    fn ignores_bindings_that_are_not_argon() {
        let request = chat_request(json!({
            "model": "gemini-4-argon",
            "messages": [{"role": "user", "content": "hello"}],
            "reasoning_effort": "minimal"
        }));
        let mut body = base_body(Some(100_000));
        Gemini4ArgonProfile
            .adapt_gemini_generate_content_request(
                &request,
                &binding("gemini-3.8-flash"),
                &mut body,
            )
            .expect("foreign binding falls back");
        assert_eq!(
            body.generation_config
                .as_ref()
                .expect("config")
                .max_output_tokens,
            Some(100_000)
        );
        assert!(
            body.generation_config
                .as_ref()
                .expect("config")
                .thinking_config
                .is_none()
        );
    }
}
