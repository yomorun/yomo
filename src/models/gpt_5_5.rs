//! GPT-5.5 Chat Completions request behavior.
//!
//! Source: <https://developers.openai.com/api/docs/models/gpt-5.5>

use crate::provider::ReasoningEffortLevel;
use crate::upstreams::Upstream;

use super::profile_support::EffortProfile;

/// GPT-5.5 profile for its exact public model name.
pub(super) const PROFILE: EffortProfile = EffortProfile::chat(
    Upstream::AzureOpenai,
    &[
        ReasoningEffortLevel::None,
        ReasoningEffortLevel::Low,
        ReasoningEffortLevel::Medium,
        ReasoningEffortLevel::High,
        ReasoningEffortLevel::XHigh,
    ],
);
