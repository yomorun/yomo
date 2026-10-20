//! Pure helpers shared by model files. Each model calls them explicitly, so a
//! model file shows every rule that applies to it.

use serde_json::Value;

use super::{ReasoningEffort, ThinkingMode};
use crate::provider::{UnifiedRequest, UnifiedToolKind};

/// Reasoning effort when it is set to a recognized native value.
pub(super) fn reasoning_effort(request: &UnifiedRequest) -> Option<ReasoningEffort> {
    request
        .options
        .reasoning_effort
        .as_deref()
        .and_then(ReasoningEffort::parse)
}

/// Thinking mode when `thinking.type` is a recognized native value.
pub(super) fn thinking_mode(request: &UnifiedRequest) -> Option<ThinkingMode> {
    request
        .options
        .thinking
        .as_ref()
        .and_then(|value| value.get("type"))
        .and_then(Value::as_str)
        .and_then(ThinkingMode::parse)
}

/// Requested `thinking.budget_tokens`, if any.
pub(super) fn thinking_budget(request: &UnifiedRequest) -> Option<u64> {
    request
        .options
        .thinking
        .as_ref()
        .and_then(|value| value.get("budget_tokens"))
        .and_then(Value::as_u64)
}

/// Whether the request declares function tools in `tools`. Replayed tool calls
/// are history, not a declaration, so they do not count.
pub(super) fn has_function_tools(request: &UnifiedRequest) -> bool {
    request.tools.as_ref().is_some_and(|tools| {
        tools
            .iter()
            .any(|tool| matches!(tool.kind, UnifiedToolKind::Function { .. }))
    })
}
