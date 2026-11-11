//! GPT-5.6 Luna Responses request behavior.
//!
//! Source: <https://developers.openai.com/api/docs/models/gpt-5.6-luna>

use crate::models::{ModelProfile, ModelProfileError};
use crate::openai_responses_types::ResponsesRequest;
use crate::provider::{ReasoningEffortLevel, UnifiedRequest};
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use super::profile_support::{is_responses_binding, map_effort_to_supported, to_responses_request};

const SUPPORTED_EFFORTS: &[ReasoningEffortLevel] = &[
    ReasoningEffortLevel::None,
    ReasoningEffortLevel::Low,
    ReasoningEffortLevel::Medium,
    ReasoningEffortLevel::High,
    ReasoningEffortLevel::XHigh,
    ReasoningEffortLevel::Max,
];

/// GPT-5.6 Luna profile for its exact public model name.
pub struct Gpt56LunaProfile;

impl ModelProfile for Gpt56LunaProfile {
    fn to_openai_responses_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ResponsesRequest>, ModelProfileError> {
        if !is_responses_binding(binding, Upstream::AzureOpenai) {
            return Ok(None);
        }
        let normalized = map_effort_to_supported(request, SUPPORTED_EFFORTS);
        to_responses_request(&normalized, binding).map(Some)
    }
}
