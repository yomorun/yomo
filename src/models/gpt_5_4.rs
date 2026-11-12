//! GPT-5.4 Chat Completions request behavior.
//!
//! Source: <https://developers.openai.com/api/docs/models/gpt-5.4>
//!
//! Binding: Azure OpenAI / Chat Completions.
//! Reasoning effort: none, low, medium, high, xhigh; other named levels map to the nearest supported level.

use crate::provider::ReasoningEffortLevel;
use crate::upstreams::Upstream;

use super::profile_support::EffortProfile;

/// GPT-5.4 profile for its exact public model name.
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
