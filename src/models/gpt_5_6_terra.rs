//! GPT-5.6 Terra Responses request behavior.
//!
//! Source: <https://developers.openai.com/api/docs/models/gpt-5.6-terra>
//!
//! Binding: Azure OpenAI / Responses.
//! Reasoning effort: none, low, medium, high, xhigh, max; other named levels map to the nearest supported level.

use crate::provider::ReasoningEffortLevel;
use crate::upstreams::Upstream;

use super::profile_support::EffortProfile;

/// GPT-5.6 Terra profile for its exact public model name.
pub(super) const PROFILE: EffortProfile = EffortProfile::responses(
    Upstream::AzureOpenai,
    &[
        ReasoningEffortLevel::None,
        ReasoningEffortLevel::Low,
        ReasoningEffortLevel::Medium,
        ReasoningEffortLevel::High,
        ReasoningEffortLevel::XHigh,
        ReasoningEffortLevel::Max,
    ],
);
