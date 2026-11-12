//! Claude Haiku 4.5 request behavior.
//!
//! Source: <https://platform.claude.com/docs/en/build-with-claude/thinking-troubleshooting>
//!
//! Binding: AWS Bedrock / Bedrock Converse, exact public model `claude-haiku-4-5`.
//! Thinking: extended only; `adaptive` is rejected; `enabled` keeps its budget.
//! Reasoning effort: not supported.
//! Sampling: temperature is forced to 1 and top-p is removed when thinking is active.
//! Tool choice: forced `tool_choice` is accepted.

use crate::models::{ModelProfile, ModelProfileError};
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;

use super::profile_support::{ClaudeEffort, ClaudeRules, ClaudeThinking, apply_claude_rules};

const RULES: ClaudeRules = ClaudeRules {
    model: "claude-haiku-4-5",
    thinking: ClaudeThinking::ExtendedOnly,
    effort: ClaudeEffort::None,
    allow_disabled: true,
    disabled_effort_high_only: false,
    allow_between_tools: false,
    between_tools_replaces_disabled: false,
    allow_forced_tool_choice: true,
    reject_prefill: false,
    drop_sampling: false,
};

/// Claude Haiku 4.5 request behavior.
pub struct ClaudeHaiku45Profile;

impl ModelProfile for ClaudeHaiku45Profile {
    fn adapt_bedrock_request(
        &self,
        request: &mut UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<(), ModelProfileError> {
        apply_claude_rules(request, binding, &RULES)
    }
}
