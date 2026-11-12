//! GPT-5.4 Nano Chat Completions request behavior.
//!
//! Source: <https://developers.openai.com/api/docs/models/gpt-5.4-nano>

use crate::models::{ModelProfile, ModelProfileError};
use crate::openai_types::ChatCompletionRequest;
use crate::provider::{ReasoningEffortLevel, UnifiedRequest};
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use super::profile_support::{is_chat_binding, to_chat_request_with_supported_effort};

const SUPPORTED_EFFORTS: &[ReasoningEffortLevel] = &[
    ReasoningEffortLevel::None,
    ReasoningEffortLevel::Low,
    ReasoningEffortLevel::Medium,
    ReasoningEffortLevel::High,
    ReasoningEffortLevel::XHigh,
];

/// GPT-5.4 Nano profile for its exact public model name.
pub struct Gpt54NanoProfile;

impl ModelProfile for Gpt54NanoProfile {
    fn to_openai_chat_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        if !is_chat_binding(binding, Upstream::AzureOpenai) {
            return Ok(None);
        }
        to_chat_request_with_supported_effort(request, SUPPORTED_EFFORTS).map(Some)
    }
}
