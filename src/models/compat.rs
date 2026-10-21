//! Pure helpers shared by model files. Each model calls them explicitly, so a
//! model file shows every rule that applies to it.

use serde_json::Value;

use super::{ReasoningEffort, ThinkingMode, unsupported};
use crate::provider::{ProviderError, UnifiedRequest};

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

/// Reject a recognized effort value outside `efforts`.
///
/// An empty set means the model has no effort parameter. Unrecognized strings
/// are left to the upstream, so a new native tier is never guessed.
pub(super) fn require_effort_values(
    rule: &str,
    request: &UnifiedRequest,
    efforts: &[ReasoningEffort],
) -> Result<(), ProviderError> {
    let Some(effort) = reasoning_effort(request) else {
        return Ok(());
    };
    if efforts.is_empty() {
        return Err(unsupported(
            rule,
            "reasoning_effort",
            "this model does not support the effort parameter",
        ));
    }
    if !efforts.contains(&effort) {
        let supported = efforts
            .iter()
            .map(|value| effort_name(*value))
            .collect::<Vec<_>>()
            .join("/");
        return Err(unsupported(
            rule,
            "reasoning_effort",
            &format!("this model accepts effort {supported}"),
        ));
    }
    Ok(())
}

fn effort_name(effort: ReasoningEffort) -> &'static str {
    match effort {
        ReasoningEffort::None => "none",
        ReasoningEffort::Minimal => "minimal",
        ReasoningEffort::Low => "low",
        ReasoningEffort::Medium => "medium",
        ReasoningEffort::High => "high",
        ReasoningEffort::XHigh => "xhigh",
        ReasoningEffort::Max => "max",
    }
}
