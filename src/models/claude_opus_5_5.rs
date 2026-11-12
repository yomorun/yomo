//! Claude Opus 5.5 request behavior.
//!
//! Source: <https://platform.claude.com/docs/en/build-with-claude/thinking-troubleshooting>

use crate::models::{ModelProfile, ModelProfileError};
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;

use super::profile_support::{ClaudeEffort, ClaudeRules, ClaudeThinking, apply_claude_rules};

const RULES: ClaudeRules = ClaudeRules {
    model: "claude-opus-5-5",
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

/// Claude Opus 5.5 request behavior.
pub struct ClaudeOpus55Profile;

impl ModelProfile for ClaudeOpus55Profile {
    fn adapt_bedrock_request(
        &self,
        request: &mut UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<(), ModelProfileError> {
        apply_claude_rules(request, binding, &RULES)
    }
}
