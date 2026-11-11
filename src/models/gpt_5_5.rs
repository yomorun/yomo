//! GPT-5.5 Chat Completions request behavior.
//!
//! Source: <https://developers.openai.com/api/docs/models/gpt-5.5>

use crate::models::{ModelProfile, ModelProfileError};
use crate::openai_types::ChatCompletionRequest;
use crate::provider::{ReasoningEffortLevel, UnifiedRequest};
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use super::profile_support::{is_chat_binding, map_effort_to_supported, to_chat_request};

const SUPPORTED_EFFORTS: &[ReasoningEffortLevel] = &[
    ReasoningEffortLevel::None,
    ReasoningEffortLevel::Low,
    ReasoningEffortLevel::Medium,
    ReasoningEffortLevel::High,
    ReasoningEffortLevel::XHigh,
];

/// GPT-5.5 profile for its exact public model name.
pub struct Gpt55Profile;

impl ModelProfile for Gpt55Profile {
    fn to_openai_chat_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        if !is_chat_binding(binding, Upstream::AzureOpenai) {
            return Ok(None);
        }
        let normalized = map_effort_to_supported(request, SUPPORTED_EFFORTS);
        to_chat_request(&normalized).map(Some)
    }
}
