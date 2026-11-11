//! GPT-6 Astra Responses request behavior.
//!
//! Source: <https://developers.openai.com/api/docs/models/gpt-6-astra>

use crate::models::{ModelProfile, ModelProfileError};
use crate::openai_responses_types::ResponsesRequest;
use crate::provider::{ReasoningEffortLevel, UnifiedRequest};
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

use super::profile_support::{is_responses_binding, map_effort_to_supported, to_responses_request};

const SUPPORTED_EFFORTS: &[ReasoningEffortLevel] = &[
    ReasoningEffortLevel::Low,
    ReasoningEffortLevel::Medium,
    ReasoningEffortLevel::High,
    ReasoningEffortLevel::XHigh,
    ReasoningEffortLevel::Max,
];

/// GPT-6 Astra profile for its exact public model name.
pub struct Gpt6AstraProfile;

impl ModelProfile for Gpt6AstraProfile {
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
