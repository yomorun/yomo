//! GPT-6.1 Sol Responses request behavior.
//!
//! Source: <https://developers.openai.com/api/docs/models/gpt-6.1-sol>

use crate::provider::ReasoningEffortLevel;
use crate::upstreams::Upstream;

use super::profile_support::EffortProfile;

/// GPT-6.1 Sol profile for its exact public model name.
pub(super) const PROFILE: EffortProfile = EffortProfile::responses(
    Upstream::AzureOpenai,
    &[
        ReasoningEffortLevel::Low,
        ReasoningEffortLevel::Medium,
        ReasoningEffortLevel::High,
        ReasoningEffortLevel::XHigh,
        ReasoningEffortLevel::Max,
    ],
);
