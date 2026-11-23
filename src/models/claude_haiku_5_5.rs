//! Claude Haiku 5.5 request behavior.
//!
//! Source: <https://platform.claude.com/docs/en/build-with-claude/thinking-troubleshooting>
//!
//! Binding: AWS Bedrock / Bedrock Converse, exact public model `claude-haiku-5-5`.
//! Thinking: adaptive only; `enabled` is converted to `adaptive`; `disabled` is
//! accepted only at effort high or below.
//! Reasoning effort: low, medium, high, xhigh, max; `none`/`minimal` map to low.
//! Sampling: temperature, top-p, and top-k are removed.
//! Tool choice: forced `tool_choice` is accepted.
//! Conversation: assistant-message prefill is rejected.

use crate::models::{ModelProfile, ModelProfileError};
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;

use super::profile_support::{ClaudeEffort, ClaudeRules, ClaudeThinking, apply_claude_rules};

const RULES: ClaudeRules = ClaudeRules {
    model: "claude-haiku-5-5",
    thinking: ClaudeThinking::AdaptiveOnly,
    effort: ClaudeEffort::Full,
    allow_disabled: true,
    disabled_effort_high_only: true,
    allow_between_tools: false,
    between_tools_replaces_disabled: false,
    allow_forced_tool_choice: true,
    reject_prefill: true,
    drop_sampling: true,
};

/// Claude Haiku 5.5 request behavior.
pub struct ClaudeHaiku55Profile;

impl ModelProfile for ClaudeHaiku55Profile {
    fn adapt_bedrock_request(
        &self,
        request: &mut UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<(), ModelProfileError> {
        apply_claude_rules(request, binding, &RULES)
    }
}
