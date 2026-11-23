//! Claude Fable 5.2 request behavior.
//!
//! Source: <https://platform.claude.com/docs/en/models/fable-5-2/overview>
//!
//! Binding: AWS Bedrock / Bedrock Converse, exact public model `claude-fable-5-2`.
//! Thinking: adaptive only and always on; `enabled` is converted to `adaptive`,
//! `disabled` is rejected. Reasoning effort: low, medium, high, xhigh, max.
//! Sampling: temperature, top-p, and top-k are removed. Tool choice: forced
//! `tool_choice` is rejected.

use crate::models::{ModelProfile, ModelProfileError};
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;

use super::profile_support::{ClaudeEffort, ClaudeRules, ClaudeThinking, apply_claude_rules};

const RULES: ClaudeRules = ClaudeRules {
    model: "claude-fable-5-2",
    thinking: ClaudeThinking::AdaptiveOnly,
    effort: ClaudeEffort::Full,
    allow_disabled: false,
    disabled_effort_high_only: false,
    allow_between_tools: false,
    between_tools_replaces_disabled: false,
    allow_forced_tool_choice: false,
    reject_prefill: false,
    drop_sampling: true,
};

/// Claude Fable 5.2 request behavior.
pub struct ClaudeFable52Profile;

impl ModelProfile for ClaudeFable52Profile {
    fn adapt_bedrock_request(
        &self,
        request: &mut UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<(), ModelProfileError> {
        apply_claude_rules(request, binding, &RULES)
    }
}
