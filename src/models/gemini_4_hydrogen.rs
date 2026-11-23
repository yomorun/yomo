//! Gemini 4 Hydrogen request behavior.
//!
//! Source: <https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-hydrogen>
//!
//! Binding: Google Vertex / Gemini generateContent, exact public model `gemini-4-hydrogen`.
//! Reasoning effort: low, medium, high, xhigh; `none`/`minimal` map to low, `max` to high.
//! Thinking: only `thinkingLevel` (LOW, MEDIUM, HIGH, XHIGH); the model rejects
//! `thinking_budget` and `thinking_level="MINIMAL"` with HTTP 400.
//! Output: 65,536 tokens per foreground segment.
//! Sampling: custom temperature, top-p, and top-k values are ignored.

use crate::dialects::Dialect;
use crate::dialects::google_vertex::types::VertexGenerateContentRequest;
use crate::models::{ModelProfile, ModelProfileError};
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use super::profile_support::apply_gemini_reasoning_controls;

const GEMINI_4_HYDROGEN_MODEL: &str = "gemini-4-hydrogen";

/// Gemini 4 Hydrogen request behavior.
pub struct Gemini4HydrogenProfile;

impl ModelProfile for Gemini4HydrogenProfile {
    fn adapt_gemini_generate_content_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
        dialect_request: &mut VertexGenerateContentRequest,
    ) -> Result<(), ModelProfileError> {
        if !is_gemini_4_hydrogen_binding(binding) {
            return Ok(());
        }
        let Some(generation) = dialect_request.generation_config.as_mut() else {
            return Ok(());
        };
        apply_gemini_reasoning_controls(request, generation, GEMINI_4_HYDROGEN_MODEL, true)
    }
}

fn is_gemini_4_hydrogen_binding(binding: &ModelBinding) -> bool {
    binding.upstream == Upstream::GoogleVertex
        && binding.dialect == Some(Dialect::GeminiGenerateContent)
        && binding.upstream_model == GEMINI_4_HYDROGEN_MODEL
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

    use super::Gemini4HydrogenProfile;

    fn binding(upstream_model: &str) -> ModelBinding {
        ModelBinding {
            public_model: "gemini-4-hydrogen".to_owned(),
            model: "gemini-4-hydrogen".to_owned(),
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

    fn base_body() -> VertexGenerateContentRequest {
        VertexGenerateContentRequest {
            contents: Vec::new(),
            system_instruction: None,
            generation_config: Some(VertexGenerationConfig::default()),
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
    fn maps_documented_effort_levels_and_forwards_xhigh() {
        for (effort, expected) in [
            ("none", VertexThinkingLevel::Low),
            ("minimal", VertexThinkingLevel::Low),
            ("low", VertexThinkingLevel::Low),
            ("medium", VertexThinkingLevel::Medium),
            ("high", VertexThinkingLevel::High),
            ("xhigh", VertexThinkingLevel::Xhigh),
            ("max", VertexThinkingLevel::High),
        ] {
            let request = chat_request(json!({
                "model": "gemini-4-hydrogen",
                "messages": [{"role": "user", "content": "hello"}],
                "reasoning_effort": effort
            }));
            let mut body = base_body();
            Gemini4HydrogenProfile
                .adapt_gemini_generate_content_request(
                    &request,
                    &binding("gemini-4-hydrogen"),
                    &mut body,
                )
                .expect("convert effort");
            assert_eq!(thinking_level(body), Some(expected), "{effort}");
        }
    }

    #[test]
    fn maps_an_enabled_thinking_budget_to_the_closest_level() {
        let request = chat_request(json!({
            "model": "gemini-4-hydrogen",
            "messages": [{"role": "user", "content": "hello"}],
            "thinking": {"type": "enabled", "budget_tokens": 2048}
        }));
        let mut body = base_body();
        Gemini4HydrogenProfile
            .adapt_gemini_generate_content_request(
                &request,
                &binding("gemini-4-hydrogen"),
                &mut body,
            )
            .expect("convert budget");
        assert_eq!(thinking_level(body), Some(VertexThinkingLevel::Medium));
    }

    #[test]
    fn ignores_bindings_that_are_not_hydrogen() {
        let request = chat_request(json!({
            "model": "gemini-4-hydrogen",
            "messages": [{"role": "user", "content": "hello"}],
            "reasoning_effort": "minimal"
        }));
        let mut body = base_body();
        Gemini4HydrogenProfile
            .adapt_gemini_generate_content_request(&request, &binding("gemini-4-carbon"), &mut body)
            .expect("foreign binding falls back");
        assert!(thinking_level(body).is_none());
    }
}
