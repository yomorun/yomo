//! Claude Sonnet 5.5 request behavior.
//!
//! Source: <https://platform.claude.com/docs/en/build-with-claude/thinking-troubleshooting>
//!
//! Binding: AWS Bedrock / Bedrock Converse, exact public model `claude-sonnet-5-5`.
//! Thinking: adaptive or `between_tools`; `enabled` is converted to `adaptive`,
//! `disabled` is converted to `between_tools`.
//! Reasoning effort: low, medium, high, xhigh, max; `none`/`minimal` map to low.
//! Sampling: temperature, top-p, and top-k are removed.
//! Tool choice: forced `tool_choice` is rejected.

use crate::models::{ModelProfile, ModelProfileError};
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;

use super::profile_support::{ClaudeEffort, ClaudeRules, ClaudeThinking, apply_claude_rules};

const RULES: ClaudeRules = ClaudeRules {
    model: "claude-sonnet-5-5",
    thinking: ClaudeThinking::AdaptiveOnly,
    effort: ClaudeEffort::Full,
    allow_disabled: false,
    disabled_effort_high_only: false,
    allow_between_tools: true,
    between_tools_replaces_disabled: true,
    allow_forced_tool_choice: false,
    reject_prefill: false,
    drop_sampling: true,
};

/// Claude Sonnet 5.5 request behavior.
pub struct ClaudeSonnet55Profile;

impl ModelProfile for ClaudeSonnet55Profile {
    fn adapt_bedrock_request(
        &self,
        request: &mut UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<(), ModelProfileError> {
        apply_claude_rules(request, binding, &RULES)
    }
}
