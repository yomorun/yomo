//! Gemini 2.5: reasoning maps to `thinkingConfig.thinkingBudget` with
//! documented per-model bounds.

use crate::models::{ReasoningEffort, ThinkingMode, unsupported};
use crate::provider::{ProviderError, UnifiedRequest};
use serde_json::json;

use super::{
    RULE_2_5_EFFORT, RULE_BUDGET_RANGE, RULE_CONFLICT, RULE_PRO_CANNOT_DISABLE, RULE_TYPE,
    budget_to_level, effort_tier, level_to_budget, thinking_budget_i64,
};

/// Thinking budget bounds for a 2.5 model: `(min, max, can disable with 0)`.
pub(super) type BudgetFacts = (i64, i64, bool);

/// 2.5-flash budget facts; 0 turns thinking off.
const FLASH_2_5_BUDGET: BudgetFacts = (1, 24_576, true);
/// 2.5-pro budget facts; the model cannot turn thinking off.
const PRO_2_5_BUDGET: BudgetFacts = (128, 32_768, false);

/// Budget facts for a 2.5 model, `None` for every other generation.
pub(super) fn budget_facts(id: &str) -> Option<BudgetFacts> {
    match id {
        "gemini-2.5-flash" => Some(FLASH_2_5_BUDGET),
        "gemini-2.5-pro" => Some(PRO_2_5_BUDGET),
        _ => None,
    }
}

/// Rewrite the thinking object to carry `budget`, preserving the mode.
fn set_thinking_budget(request: &mut UnifiedRequest, mode: ThinkingMode, budget: i64) {
    let type_name = match mode {
        ThinkingMode::Enabled => "enabled",
        ThinkingMode::Adaptive => "adaptive",
        // Unreachable for Gemini: the type whitelist rejects between_tools
        // before any rewrite, but the match must stay exhaustive.
        ThinkingMode::BetweenTools | ThinkingMode::Disabled => "disabled",
    };
    request.options.thinking = Some(json!({"type": type_name, "budget_tokens": budget}));
}

/// 2.5 models: thinking maps to thinkingBudget; effort maps lossily onto the
/// same bands (user-licensed local policy) and merges with an explicit budget
/// exactly like the Gemini 3 side merges with a level.
pub(super) fn adapt(
    request: &mut UnifiedRequest,
    (min, max, can_disable): BudgetFacts,
    mode: Option<ThinkingMode>,
) -> Result<(), ProviderError> {
    let effort = request.options.reasoning_effort.as_deref();
    let tier = effort_tier(effort);
    let Some(mode) = mode else {
        let Some(tier) = tier else {
            return Err(unsupported(
                RULE_2_5_EFFORT,
                "reasoning_effort",
                "2.5 models express reasoning only through thinking.budget_tokens; only minimal/low/medium/high map onto it",
            ));
        };
        // User-licensed lossy rewrite: the effort tier becomes a budget.
        request.options.thinking = Some(json!({
            "type": "enabled",
            "budget_tokens": level_to_budget(tier),
        }));
        request.options.reasoning_effort = None;
        return Ok(());
    };
    let budget = thinking_budget_i64(request);
    match mode {
        ThinkingMode::BetweenTools => {
            return Err(unsupported(
                RULE_TYPE,
                "thinking.type",
                "between_tools is not a Vertex thinking mode",
            ));
        }
        ThinkingMode::Disabled => {
            if matches!(budget, Some(value) if value > 0) {
                return Err(unsupported(
                    RULE_CONFLICT,
                    "thinking.budget_tokens",
                    "a positive budget contradicts thinking.type disabled",
                ));
            }
            if !can_disable {
                return Err(unsupported(
                    RULE_PRO_CANNOT_DISABLE,
                    "thinking",
                    "this model cannot turn thinking off",
                ));
            }
            if let Some(effort) = effort {
                if tier.is_some() {
                    return Err(unsupported(
                        RULE_CONFLICT,
                        "reasoning_effort",
                        "a reasoning effort contradicts thinking.type disabled",
                    ));
                }
                if ReasoningEffort::parse(effort) != Some(ReasoningEffort::None) {
                    return Err(unsupported(
                        RULE_2_5_EFFORT,
                        "reasoning_effort",
                        "only minimal/low/medium/high map onto thinking.budget_tokens",
                    ));
                }
            }
            // effort=none contributes nothing: plain disabled carries the merge.
            request.options.reasoning_effort = None;
        }
        ThinkingMode::Enabled | ThinkingMode::Adaptive => {
            // Budget shape is validated before any merge so conflicts carry exact facts.
            match budget {
                Some(-1) | None => {}
                Some(0) if can_disable => {}
                Some(0) => {
                    return Err(unsupported(
                        RULE_PRO_CANNOT_DISABLE,
                        "thinking",
                        "this model cannot turn thinking off",
                    ));
                }
                Some(value) if (min..=max).contains(&value) => {}
                Some(value) => {
                    return Err(unsupported(
                        RULE_BUDGET_RANGE,
                        "thinking.budget_tokens",
                        &format!(
                            "budget {value} must be {min}..={max}, 0 to disable, or -1 for dynamic"
                        ),
                    ));
                }
            }
            match tier {
                Some(tier) => {
                    let target = level_to_budget(tier);
                    match budget {
                        // The tier is the more specific control; it becomes the budget.
                        None | Some(-1) => {
                            set_thinking_budget(request, mode, target);
                            request.options.reasoning_effort = None;
                        }
                        Some(0) => {
                            return Err(unsupported(
                                RULE_CONFLICT,
                                "reasoning_effort",
                                "a zero budget contradicts an explicit reasoning effort",
                            ));
                        }
                        Some(value) if budget_to_level(value) != tier => {
                            return Err(unsupported(
                                RULE_CONFLICT,
                                "reasoning_effort",
                                &format!(
                                    "effort maps to {tier} but thinking.budget_tokens maps to {}",
                                    budget_to_level(value)
                                ),
                            ));
                        }
                        Some(_) => request.options.reasoning_effort = None,
                    }
                }
                None => {
                    let Some(effort) = effort else {
                        return Ok(());
                    };
                    if ReasoningEffort::parse(effort) == Some(ReasoningEffort::None) {
                        return Err(unsupported(
                            RULE_CONFLICT,
                            "reasoning_effort",
                            "effort none contradicts an explicitly enabled thinking request",
                        ));
                    }
                    return Err(unsupported(
                        RULE_2_5_EFFORT,
                        "reasoning_effort",
                        "only minimal/low/medium/high map onto thinking.budget_tokens",
                    ));
                }
            }
        }
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::super::vertex;
    use super::*;
    use crate::models::adapt_request as adapt;

    #[test]
    fn budget_requests_pass_through_within_documented_bounds() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 1024}));
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_ok());
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 24576}));
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_ok());
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": -1}));
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_ok());
        request.options.thinking = Some(json!({"type": "adaptive", "budget_tokens": 128}));
        assert!(adapt(&vertex("gemini-2.5-pro"), &mut request).is_ok());
    }

    #[test]
    fn out_of_range_budgets_are_rejected_with_documented_bounds() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 24577}));
        match adapt(&vertex("gemini-2.5-flash"), &mut request) {
            Err(ProviderError::Public { error, .. }) => {
                assert!(error.message.contains("24576"), "{}", error.message);
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 127}));
        assert!(adapt(&vertex("gemini-2.5-pro"), &mut request).is_err());
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": -2}));
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_err());
    }

    #[test]
    fn disabling_thinking_follows_per_model_bounds() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "disabled"}));
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_ok());
        match adapt(&vertex("gemini-2.5-pro"), &mut request) {
            Err(ProviderError::Public { error, .. }) => {
                assert_eq!(error.param.as_deref(), Some("thinking"));
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 0}));
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_ok());
    }

    #[test]
    fn effort_bands_onto_budgets_for_2_5_models() {
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("high".into());
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_ok());
        assert_eq!(request.options.reasoning_effort, None);
        assert_eq!(
            request.options.thinking,
            Some(json!({"type": "enabled", "budget_tokens": 24576}))
        );

        request.options.reasoning_effort = Some("medium".into());
        request.options.thinking = Some(json!({"type": "adaptive"}));
        assert!(adapt(&vertex("gemini-2.5-pro"), &mut request).is_ok());
        assert_eq!(request.options.reasoning_effort, None);
        assert_eq!(
            request.options.thinking,
            Some(json!({"type": "adaptive", "budget_tokens": 8192}))
        );

        // Agreeing controls merge; the budget survives, the effort is dropped.
        request.options.reasoning_effort = Some("medium".into());
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_ok());
        assert_eq!(request.options.reasoning_effort, None);
        assert_eq!(
            request.options.thinking,
            Some(json!({"type": "adaptive", "budget_tokens": 8192}))
        );

        // minimal lands at the top of its band.
        request.options.reasoning_effort = Some("minimal".into());
        request.options.thinking = None;
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_ok());
        assert_eq!(
            request.options.thinking,
            Some(json!({"type": "enabled", "budget_tokens": 1024}))
        );
    }

    #[test]
    fn disagreeing_2_5_controls_conflict_without_rewrites() {
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("low".into());
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 1024}));
        match adapt(&vertex("gemini-2.5-flash"), &mut request) {
            Err(ProviderError::Public { error, .. }) => {
                assert!(error.message.contains("maps to"), "{}", error.message);
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
        assert_eq!(request.options.reasoning_effort.as_deref(), Some("low"));

        request.options.reasoning_effort = Some("high".into());
        request.options.thinking = Some(json!({"type": "disabled"}));
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_err());
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 0}));
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_err());
    }

    #[test]
    fn effort_without_a_budget_tier_stays_rejected_for_2_5() {
        let mut request = UnifiedRequest::default();
        for value in ["none", "xhigh", "max"] {
            request.options.reasoning_effort = Some(value.into());
            assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_err());
        }
        // none contradicts an enabled request; it only merges with disabled.
        request.options.reasoning_effort = Some("none".into());
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 1024}));
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_err());
        request.options.thinking = Some(json!({"type": "disabled"}));
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_ok());
        assert_eq!(request.options.reasoning_effort, None);
        // xhigh/max have no tier even combined with disabled.
        request.options.reasoning_effort = Some("max".into());
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_err());
    }
}
