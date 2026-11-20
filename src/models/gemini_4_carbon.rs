//! Gemini 4 Carbon request behavior.
//!
//! Source: <https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-carbon>
//!
//! Binding: Google Vertex / Gemini generateContent, exact public model `gemini-4-carbon`.
//! Reasoning effort: low, medium, high; `none`/`minimal` map to low, `xhigh`/`max` to high.
//! Thinking: only `thinkingLevel` (LOW, MEDIUM, HIGH); the model rejects
//! `thinking_budget` and `thinking_level="MINIMAL"` with HTTP 400.
//! Output: 65,536 tokens streaming and 32,768 tokens non-streaming.
//! Sampling: custom temperature, top-p, and top-k values are ignored.

use crate::dialects::Dialect;
use crate::dialects::google_vertex::types::VertexGenerateContentRequest;
use crate::models::{ModelProfile, ModelProfileError};
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use super::profile_support::{apply_gemini_reasoning_controls, gemini_clamp_output_tokens};

const GEMINI_4_CARBON_MODEL: &str = "gemini-4-carbon";
const MAX_OUTPUT_TOKENS: i32 = 65_536;
const NON_STREAM_MAX_OUTPUT_TOKENS: i32 = 32_768;

/// Gemini 4 Carbon request behavior.
pub struct Gemini4CarbonProfile;

impl ModelProfile for Gemini4CarbonProfile {
    fn adapt_gemini_generate_content_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
        dialect_request: &mut VertexGenerateContentRequest,
    ) -> Result<(), ModelProfileError> {
        if !is_gemini_4_carbon_binding(binding) {
            return Ok(());
        }
        let Some(generation) = dialect_request.generation_config.as_mut() else {
            return Ok(());
        };
        apply_gemini_reasoning_controls(request, generation, GEMINI_4_CARBON_MODEL, false)?;

        let cap = if request.controls.stream.unwrap_or(false) {
            MAX_OUTPUT_TOKENS
        } else {
            NON_STREAM_MAX_OUTPUT_TOKENS
        };
        gemini_clamp_output_tokens(generation, cap, GEMINI_4_CARBON_MODEL);
        Ok(())
    }
}

fn is_gemini_4_carbon_binding(binding: &ModelBinding) -> bool {
    binding.upstream == Upstream::GoogleVertex
        && binding.dialect == Some(Dialect::GeminiGenerateContent)
        && binding.upstream_model == GEMINI_4_CARBON_MODEL
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use crate::api;
    use crate::dialects::google_vertex::types::{
        VertexGenerateContentRequest, VertexGenerationConfig, VertexThinkingLevel,
    };
    use crate::models::ModelProfile;
    use crate::provider::{UnifiedRequest, UnifiedSourceProtocol};
    use crate::serve_config::ModelBinding;
    use crate::upstreams::Upstream;

    use super::Gemini4CarbonProfile;

    fn binding(upstream_model: &str) -> ModelBinding {
        ModelBinding {
            public_model: "gemini-4-carbon".to_owned(),
            model: "gemini-4-carbon".to_owned(),
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

    fn thinking_level(body: VertexGenerateContentRequest) -> Option<VertexThinkingLevel> {
        body.generation_config
            .and_then(|config| config.thinking_config)
            .and_then(|config| config.thinking_level)
    }

    #[test]
    fn maps_documented_effort_levels_and_clamps_xhigh() {
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
                "model": "gemini-4-carbon",
                "messages": [{"role": "user", "content": "hello"}],
                "reasoning_effort": effort
            }));
            let mut body = base_body(None);
            Gemini4CarbonProfile
                .adapt_gemini_generate_content_request(
                    &request,
                    &binding("gemini-4-carbon"),
                    &mut body,
                )
                .expect("convert effort");
            assert_eq!(thinking_level(body), Some(expected), "{effort}");
        }
    }

    #[test]
    fn maps_an_enabled_thinking_budget_to_the_closest_level() {
        let request = chat_request(json!({
            "model": "gemini-4-carbon",
            "messages": [{"role": "user", "content": "hello"}],
            "thinking": {"type": "enabled", "budget_tokens": 2048}
        }));
        let mut body = base_body(None);
        Gemini4CarbonProfile
            .adapt_gemini_generate_content_request(&request, &binding("gemini-4-carbon"), &mut body)
            .expect("convert budget");
        let config = body
            .generation_config
            .and_then(|config| config.thinking_config)
            .expect("thinking config");
        assert_eq!(config.thinking_level, Some(VertexThinkingLevel::Medium));
        assert_eq!(config.thinking_budget, None);
    }

    #[test]
    fn caps_non_streaming_output_but_allows_streaming() {
        for (stream, expected) in [(false, 32_768), (true, 65_536)] {
            let request = chat_request(json!({
                "model": "gemini-4-carbon",
                "messages": [{"role": "user", "content": "hello"}],
                "max_tokens": 100_000,
                "stream": stream
            }));
            let mut body = base_body(Some(100_000));
            Gemini4CarbonProfile
                .adapt_gemini_generate_content_request(
                    &request,
                    &binding("gemini-4-carbon"),
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
    fn ignores_bindings_that_are_not_carbon() {
        let request = chat_request(json!({
            "model": "gemini-4-carbon",
            "messages": [{"role": "user", "content": "hello"}],
            "reasoning_effort": "minimal"
        }));
        let mut body = base_body(Some(100_000));
        Gemini4CarbonProfile
            .adapt_gemini_generate_content_request(&request, &binding("gemini-4-argon"), &mut body)
            .expect("foreign binding falls back");
        assert_eq!(
            body.generation_config
                .as_ref()
                .expect("config")
                .max_output_tokens,
            Some(100_000)
        );
        assert!(thinking_level(body).is_none());
    }
}
