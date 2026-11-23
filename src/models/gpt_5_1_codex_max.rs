//! GPT-5.1 Codex-Max Responses request behavior.
//!
//! Sources:
//! - <https://developers.openai.com/api/docs/models/gpt-5.1-codex-max>
//! - <https://learn.microsoft.com/en-us/azure/foundry/openai/how-to/reasoning>
//!
//! Binding: Azure OpenAI / Responses.
//! Reasoning effort: none, low, medium, high, xhigh; other named levels map to the nearest supported level.

use crate::provider::ReasoningEffortLevel;
use crate::upstreams::Upstream;

use super::profile_support::EffortProfile;

/// GPT-5.1 Codex-Max profile for its exact public model name.
pub(super) const PROFILE: EffortProfile = EffortProfile::responses(
    Upstream::AzureOpenai,
    &[
        ReasoningEffortLevel::None,
        ReasoningEffortLevel::Low,
        ReasoningEffortLevel::Medium,
        ReasoningEffortLevel::High,
        ReasoningEffortLevel::XHigh,
    ],
);
