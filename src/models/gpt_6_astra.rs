//! GPT-6 Astra Responses request behavior.
//!
//! Source: <https://developers.openai.com/api/docs/models/gpt-6-astra>
//!
//! Binding: Azure OpenAI / Responses.
//! Reasoning effort: low, medium, high, xhigh, max; other named levels map to the nearest supported level.

use crate::provider::ReasoningEffortLevel;
use crate::upstreams::Upstream;

use super::profile_support::EffortProfile;

/// GPT-6 Astra profile for its exact public model name.
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
