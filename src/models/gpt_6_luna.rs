//! GPT-6 Luna Responses request behavior.
//!
//! Source: <https://developers.openai.com/api/docs/models/gpt-6-luna>
//!
//! Binding: Azure OpenAI / Responses.
//! Reasoning effort: none, low, medium, high, xhigh, max; other named levels map to the nearest supported level.
//! Thinking: OpenAI Responses has no adaptive mode, so an adaptive control is
//! treated as the model default and dropped; an explicit effort is kept.

use log::debug;

use crate::models::{ModelProfile, ModelProfileError};
use crate::openai_responses_types::ResponsesRequest;
use crate::provider::{ReasoningEffortLevel, UnifiedRequest};
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use super::profile_support::{
    drop_adaptive_thinking, is_responses_binding, to_responses_request_with_supported_effort,
};

const SUPPORTED_EFFORTS: &[ReasoningEffortLevel] = &[
    ReasoningEffortLevel::None,
    ReasoningEffortLevel::Low,
    ReasoningEffortLevel::Medium,
    ReasoningEffortLevel::High,
    ReasoningEffortLevel::XHigh,
    ReasoningEffortLevel::Max,
];

/// GPT-6 Luna profile for its exact public model name.
pub struct Gpt6LunaProfile;

impl ModelProfile for Gpt6LunaProfile {
    fn to_openai_responses_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ResponsesRequest>, ModelProfileError> {
        if !is_responses_binding(binding, Upstream::AzureOpenai) {
            return Ok(None);
        }
        let mut normalized = request.clone();
        if drop_adaptive_thinking(&mut normalized) {
            debug!("gpt-6-luna profile: dropped adaptive thinking for the Responses target");
        }
        to_responses_request_with_supported_effort(&normalized, binding, SUPPORTED_EFFORTS)
            .map(Some)
    }
}

#[cfg(test)]
mod tests {
    use serde_json::{Value, json};

    use crate::api;
    use crate::dialects::Dialect;
    use crate::models::ModelProfile;
    use crate::provider::UnifiedSourceProtocol;
    use crate::serve_config::ModelBinding;
    use crate::upstreams::Upstream;

    use super::Gpt6LunaProfile;

    fn binding() -> ModelBinding {
        ModelBinding {
            public_model: "gpt-6-luna".to_owned(),
            model: "gpt-6-luna".to_owned(),
            provider: "azure.jpn".to_owned(),
            upstream: Upstream::AzureOpenai,
            upstream_model: "gpt-6-luna".to_owned(),
            dialect: Some(Dialect::OpenAiResponses),
            label: "azure.jpn.gpt-6-luna".to_owned(),
        }
    }

    fn request(thinking: Value) -> crate::provider::UnifiedRequest {
        let mut request = api::request_from_value(
            &json!({"model": "gpt-6-luna", "messages": [{"role": "user", "content": "hi"}]}),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse request");
        request.options.thinking = Some(thinking);
        request.options.reasoning_effort = Some("high".into());
        request
    }

    #[test]
    fn adaptive_thinking_is_dropped_and_effort_kept() {
        let request = Gpt6LunaProfile
            .to_openai_responses_request(
                &request(json!({"type": "adaptive", "display": "updates"})),
                &binding(),
            )
            .expect("profile conversion")
            .expect("gpt-6-luna handles Responses");
        let body = serde_json::to_value(request).expect("serialize");
        assert!(
            body.get("thinking").is_none(),
            "adaptive thinking must not reach Azure"
        );
        assert_eq!(body["reasoning"]["effort"], "high");
    }

    #[test]
    fn concrete_thinking_controls_are_preserved() {
        let request = Gpt6LunaProfile
            .to_openai_responses_request(
                &request(json!({"type": "enabled", "budget_tokens": 2048})),
                &binding(),
            )
            .expect("profile conversion")
            .expect("gpt-6-luna handles Responses");
        let body = serde_json::to_value(request).expect("serialize");
        assert_eq!(body["thinking"]["type"], "enabled");
        assert_eq!(body["reasoning"]["effort"], "high");
    }

    #[test]
    fn foreign_binding_falls_back_to_the_default_conversion() {
        let mut foreign = binding();
        foreign.upstream = Upstream::GoogleVertex;
        assert!(
            Gpt6LunaProfile
                .to_openai_responses_request(&request(json!({"type": "adaptive"})), &foreign,)
                .expect("profile conversion")
                .is_none()
        );
    }
}
