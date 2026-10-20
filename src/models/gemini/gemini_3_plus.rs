//! Gemini 3+: reasoning maps to `thinkingConfig.thinkingLevel`, with the
//! supported levels snap up from the requested budget tier.

use crate::models::{ReasoningEffort, ThinkingMode, unsupported};
use crate::provider::{ProviderError, UnifiedRequest};

use super::{
    RULE_3_CANNOT_DISABLE, RULE_BUDGET_RANGE, RULE_CONFLICT, RULE_LEVEL_UNSUPPORTED,
    budget_to_level,
};

/// Thinking levels supported by gemini-3-flash-preview, gemini-3.5-flash and
/// gemini-3.6-flash.
const LEVELS_MINIMAL_UP: &[&str] = &["minimal", "low", "medium", "high"];
/// Thinking levels supported by gemini-3.1-pro-preview, gemini-3.7-flash and
/// gemini-3.8-flash; they do not accept MINIMAL.
const LEVELS_LOW_UP: &[&str] = &["low", "medium", "high"];

/// Supported thinking levels for a 3+ model, ascending.
pub(super) fn levels(id: &str) -> &'static [&'static str] {
    match id {
        "gemini-3-flash-preview" | "gemini-3.5-flash" | "gemini-3.6-flash" => LEVELS_MINIMAL_UP,
        // The remaining MODELS entries are 3+ models without MINIMAL.
        _ => LEVELS_LOW_UP,
    }
}

/// Snap a level up to the nearest tier the model supports. Level sets are
/// contiguous ascending suffixes of the four-tier axis, so the snapped axis
/// position always indexes into the axis.
fn snap_level(level: &str, supported: &'static [&'static str]) -> &'static str {
    let requested = LEVELS_MINIMAL_UP
        .iter()
        .position(|tier| *tier == level)
        .unwrap_or(LEVELS_MINIMAL_UP.len() - 1);
    let floor = supported
        .iter()
        .filter_map(|tier| LEVELS_MINIMAL_UP.iter().position(|axis| axis == tier))
        .min()
        .unwrap_or(0);
    LEVELS_MINIMAL_UP[requested.max(floor)]
}

/// Reject when the effort value cannot be carried by a Gemini 3 level.
fn effort_level(
    rule: &str,
    request: &UnifiedRequest,
    supported: &'static [&'static str],
) -> Result<&'static str, ProviderError> {
    let level = match request
        .options
        .reasoning_effort
        .as_deref()
        .and_then(ReasoningEffort::parse)
    {
        Some(ReasoningEffort::Minimal) => "minimal",
        Some(ReasoningEffort::Low) => "low",
        Some(ReasoningEffort::Medium) => "medium",
        Some(ReasoningEffort::High) => "high",
        _ => {
            return Err(unsupported(
                rule,
                "reasoning_effort",
                "only minimal/low/medium/high map to a Gemini 3 thinking level",
            ));
        }
    };
    if supported.contains(&level) {
        Ok(level)
    } else {
        Err(unsupported(
            rule,
            "reasoning_effort",
            &format!("{level} is not a supported thinking level for this model"),
        ))
    }
}

/// Gemini 3+ models: effort and budget both resolve to a thinking level.
pub(super) fn adapt(
    request: &mut UnifiedRequest,
    supported: &'static [&'static str],
    mode: Option<ThinkingMode>,
) -> Result<(), ProviderError> {
    let effort = request.options.reasoning_effort.as_deref();
    let budget = super::thinking_budget_i64(request);
    let mode = match mode {
        Some(ThinkingMode::Disabled) => {
            return Err(unsupported(
                RULE_3_CANNOT_DISABLE,
                "thinking",
                "Gemini 3 models have no thinking level that turns thinking off",
            ));
        }
        Some(mode) => Some(mode),
        None => None,
    };
    if budget.is_some_and(|value| value == 0) {
        return Err(unsupported(
            RULE_3_CANNOT_DISABLE,
            "thinking.budget_tokens",
            "a zero budget means off and Gemini 3 models cannot turn thinking off",
        ));
    }
    if budget.is_some_and(|value| value < -1) {
        return Err(unsupported(
            RULE_BUDGET_RANGE,
            "thinking.budget_tokens",
            "only -1 is a valid negative budget (explicit dynamic)",
        ));
    }
    let Some(effort) = effort else {
        // Thinking only: band the budget onto a level (lossy rewrite, user
        // licensed) or leave the request untouched for the model default.
        if let Some(budget) = budget.filter(|value| *value >= 1) {
            let level = snap_level(budget_to_level(budget), supported);
            request.options.reasoning_effort = Some(level.to_string());
            request.options.thinking = None;
        }
        return Ok(());
    };
    if ReasoningEffort::parse(effort) == Some(ReasoningEffort::None) {
        return match mode {
            Some(_) => Err(unsupported(
                RULE_CONFLICT,
                "reasoning_effort",
                "effort none contradicts an explicitly enabled thinking request",
            )),
            None => Err(unsupported(
                RULE_LEVEL_UNSUPPORTED,
                "reasoning_effort",
                "Gemini 3 thinking levels have no none value",
            )),
        };
    }
    let effort_level = effort_level(RULE_LEVEL_UNSUPPORTED, request, supported)?;
    if let Some(budget) = budget.filter(|value| *value >= 1) {
        let budget_level = snap_level(budget_to_level(budget), supported);
        if budget_level != effort_level {
            return Err(unsupported(
                RULE_CONFLICT,
                "reasoning_effort",
                &format!(
                    "effort maps to {effort_level} but thinking.budget_tokens maps to {budget_level}"
                ),
            ));
        }
    }
    // effort carries the merged control; the thinking object adds nothing the
    // encoder can express beyond the level.
    request.options.thinking = None;
    Ok(())
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::super::vertex;
    use super::*;
    use crate::models::adapt_request as adapt;

    #[test]
    fn thinking_cannot_be_disabled() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "disabled"}));
        assert!(adapt(&vertex("gemini-3.5-flash"), &mut request).is_err());
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 0}));
        assert!(adapt(&vertex("gemini-3.1-pro-preview"), &mut request).is_err());
    }

    #[test]
    fn effort_maps_to_thinking_levels_for_gemini_3() {
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("high".into());
        assert!(adapt(&vertex("gemini-3.5-flash"), &mut request).is_ok());
        request.options.reasoning_effort = Some("minimal".into());
        assert!(adapt(&vertex("gemini-3.5-flash"), &mut request).is_ok());
        // MINIMAL is not in this model's supported set.
        assert!(adapt(&vertex("gemini-3.7-flash"), &mut request).is_err());
        // Gemini 4 uses thinkingLevel too and does not accept MINIMAL.
        assert!(adapt(&vertex("gemini-4-argon"), &mut request).is_err());
        request.options.reasoning_effort = Some("high".into());
        assert!(adapt(&vertex("gemini-4-argon"), &mut request).is_ok());
        request.options.reasoning_effort = Some("none".into());
        assert!(adapt(&vertex("gemini-3.5-flash"), &mut request).is_err());
        request.options.reasoning_effort = Some("max".into());
        assert!(adapt(&vertex("gemini-3.5-flash"), &mut request).is_err());
    }

    #[test]
    fn budgets_band_onto_levels_and_snap_up_when_needed() {
        fn banded_level(model: &'static str, budget: i64) -> String {
            let mut request = UnifiedRequest::default();
            request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": budget}));
            adapt(&vertex(model), &mut request).expect("budget bands onto a level");
            request
                .options
                .reasoning_effort
                .expect("banding sets the effort level")
        }
        // TokenHub's default budget of 8192 lands on MEDIUM.
        assert_eq!(banded_level("gemini-3.5-flash", 8192), "medium");
        // Small budgets snap up from MINIMAL to LOW where MINIMAL is unsupported.
        assert_eq!(banded_level("gemini-3.1-pro-preview", 512), "low");
        // MINIMAL stays available on models that list it.
        assert_eq!(banded_level("gemini-3.6-flash", 512), "minimal");
        // Large budgets land on HIGH.
        assert_eq!(banded_level("gemini-3.8-flash", 32768), "high");
    }

    #[test]
    fn merged_controls_agree_or_conflict() {
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("high".into());
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 32768}));
        assert!(adapt(&vertex("gemini-3.5-flash"), &mut request).is_ok());
        assert_eq!(request.options.thinking, None);
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 8192}));
        match adapt(&vertex("gemini-3.5-flash"), &mut request) {
            Err(ProviderError::Public { error, .. }) => {
                assert!(error.message.contains("maps to"), "{}", error.message);
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
        // Rejections never modify the request.
        assert_eq!(
            request.options.thinking,
            Some(json!({"type": "enabled", "budget_tokens": 8192}))
        );
        request.options.reasoning_effort = Some("none".into());
        request.options.thinking = Some(json!({"type": "adaptive"}));
        assert!(adapt(&vertex("gemini-3.5-flash"), &mut request).is_err());
    }
}
