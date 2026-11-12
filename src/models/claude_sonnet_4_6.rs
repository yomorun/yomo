//! Claude Sonnet 4.6 request behavior.
//!
//! Source: <https://platform.claude.com/docs/en/build-with-claude/thinking-troubleshooting>
//!
//! Binding: AWS Bedrock / Bedrock Converse, exact public model `claude-sonnet-4-6`.
//! Thinking: adaptive or extended (deprecated); `enabled` keeps its budget.
//! Reasoning effort: low, medium, high, max; `xhigh` maps to max.
//! Sampling: temperature is forced to 1 and top-p is removed when thinking is active.
//! Tool choice: forced `tool_choice` is accepted.

use crate::models::{ModelProfile, ModelProfileError};
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;

use super::profile_support::{ClaudeEffort, ClaudeRules, ClaudeThinking, apply_claude_rules};

const RULES: ClaudeRules = ClaudeRules {
    model: "claude-sonnet-4-6",
    thinking: ClaudeThinking::AdaptiveExtended,
    effort: ClaudeEffort::UpToMax,
    allow_disabled: true,
    disabled_effort_high_only: false,
    allow_between_tools: false,
    between_tools_replaces_disabled: false,
    allow_forced_tool_choice: true,
    reject_prefill: false,
    drop_sampling: false,
};

/// Claude Sonnet 4.6 request behavior.
pub struct ClaudeSonnet46Profile;

impl ModelProfile for ClaudeSonnet46Profile {
    fn adapt_bedrock_request(
        &self,
        request: &mut UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<(), ModelProfileError> {
        apply_claude_rules(request, binding, &RULES)
    }
}
