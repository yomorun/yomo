//! Claude Opus 4.8 request behavior.
//!
//! Source: <https://platform.claude.com/docs/en/build-with-claude/thinking-troubleshooting>
//!
//! Binding: AWS Bedrock / Bedrock Converse, exact public model `claude-opus-4-8`.
//! Thinking: adaptive only; `enabled` is converted to `adaptive`; `disabled` is
//! accepted.
//! Reasoning effort: low, medium, high, xhigh, max; `none`/`minimal` map to low.
//! Sampling: temperature, top-p, and top-k are removed.
//! Tool choice: forced `tool_choice` is accepted.

use crate::models::{ModelProfile, ModelProfileError};
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;

use super::profile_support::{ClaudeEffort, ClaudeRules, ClaudeThinking, apply_claude_rules};

const RULES: ClaudeRules = ClaudeRules {
    model: "claude-opus-4-8",
    thinking: ClaudeThinking::AdaptiveOnly,
    effort: ClaudeEffort::Full,
    allow_disabled: true,
    disabled_effort_high_only: false,
    allow_between_tools: false,
    between_tools_replaces_disabled: false,
    allow_forced_tool_choice: true,
    reject_prefill: false,
    drop_sampling: true,
};

/// Claude Opus 4.8 request behavior.
pub struct ClaudeOpus48Profile;

impl ModelProfile for ClaudeOpus48Profile {
    fn adapt_bedrock_request(
        &self,
        request: &mut UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<(), ModelProfileError> {
        apply_claude_rules(request, binding, &RULES)
    }
}
