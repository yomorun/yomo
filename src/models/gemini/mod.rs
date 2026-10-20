//! Gemini family: Vertex generateContent profile and request rules.
//!
//! Gemini 3+ models take `thinkingConfig.thinkingLevel` and reject
//! `thinkingBudget`; 2.5 models take `thinkingConfig.thinkingBudget` and
//! reject `thinkingLevel`. 2.5 budgets are soft limits: 2.5-flash 1–24,576
//! (0 turns thinking off), 2.5-pro 128–32,768 (cannot be turned off), `-1`
//! means an explicit dynamic budget.

use crate::models::{Ctx, Profile, ReasoningEffort, ThinkingMode, generic_profile, unsupported};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::{Dialect, EffortField, Upstream};
use serde_json::{Value, json};

const MODELS: &[&str] = &[
    "gemini-2.5-flash",
    "gemini-2.5-pro",
    "gemini-3-flash-preview",
    "gemini-3.1-pro-preview",
    "gemini-3.5-flash",
    "gemini-3.6-flash",
    "gemini-3.7-flash",
    "gemini-3.8-flash",
    "gemini-4-argon",
];

/// Stable rule names cited in errors and tests.
const RULE_TYPE: &str = "vertex-gemini-thinking-type";
const RULE_FIELDS: &str = "vertex-gemini-thinking-fields";
const RULE_2_5_EFFORT: &str = "vertex-gemini-2-5-effort-unmapped";
const RULE_3_CANNOT_DISABLE: &str = "vertex-gemini-thinking-cannot-be-disabled";
const RULE_PRO_CANNOT_DISABLE: &str = "vertex-gemini-2-5-pro-thinking-cannot-be-disabled";
const RULE_BUDGET_RANGE: &str = "vertex-gemini-thinking-budget-range";
const RULE_LEVEL_UNSUPPORTED: &str = "vertex-gemini-effort-level-unsupported";
const RULE_CONFLICT: &str = "vertex-gemini-thinking-effort-conflict";

/// Thinking budget bounds for a 2.5 model: `(min, max, can disable with 0)`.
type BudgetFacts = (i64, i64, bool);

/// 2.5-flash budget facts; 0 turns thinking off.
const FLASH_2_5_BUDGET: BudgetFacts = (1, 24_576, true);
/// 2.5-pro budget facts; the model cannot turn thinking off.
const PRO_2_5_BUDGET: BudgetFacts = (128, 32_768, false);

/// Thinking levels supported by gemini-3-flash-preview, gemini-3.5-flash and
/// gemini-3.6-flash.
const LEVELS_MINIMAL_UP: &[&str] = &["minimal", "low", "medium", "high"];
/// Thinking levels supported by gemini-3.1-pro-preview, gemini-3.7-flash and
/// gemini-3.8-flash; they do not accept MINIMAL.
const LEVELS_LOW_UP: &'static [&'static str] = &["low", "medium", "high"];

pub(super) fn profile(id: &str, upstream: Upstream) -> Option<Profile> {
    if !MODELS.contains(&id) {
        return None;
    }
    match upstream {
        // Gemini 3+ carries effort through thinkingLevel; whether a concrete
        // model accepts a value is decided by adapt_request below. The dialect
        // mapper encodes whatever single control survives adaptation.
        Upstream::GoogleVertex => Some(Profile {
            default_dialect: Dialect::GeminiGenerateContent,
            dialects: &[Dialect::GeminiGenerateContent],
            requires_explicit_dialect: false,
            max_tokens_field: None,
            effort_field: EffortField::ThinkingLevel,
        }),
        other => generic_profile(other),
    }
}

/// Which generation a Vertex Gemini model belongs to.
enum Generation {
    /// thinkingBudget with documented bounds.
    Budget(BudgetFacts),
    /// thinkingLevel with the model's supported tiers, ascending.
    Level(&'static [&'static str]),
}

fn generation(id: &str) -> Generation {
    match id {
        "gemini-2.5-flash" => Generation::Budget(FLASH_2_5_BUDGET),
        "gemini-2.5-pro" => Generation::Budget(PRO_2_5_BUDGET),
        "gemini-3-flash-preview" | "gemini-3.5-flash" | "gemini-3.6-flash" => {
            Generation::Level(LEVELS_MINIMAL_UP)
        }
        // The remaining MODELS entries are 3+ models without MINIMAL.
        _ => Generation::Level(LEVELS_LOW_UP),
    }
}

/// Requested `thinking.budget_tokens` as a signed value; `-1` (explicit
/// dynamic) is meaningful for Vertex, so `u64` parsing is not enough.
fn thinking_budget_i64(request: &UnifiedRequest) -> Option<i64> {
    request
        .options
        .thinking
        .as_ref()
        .and_then(|value| value.get("budget_tokens"))
        .and_then(Value::as_i64)
}

/// Local policy, not a vendor fact: band a requested budget onto Gemini 3
/// thinking levels. TokenHub's default budget of 8192 and the 2.5 dynamic
/// default of up to 8192 both land on MEDIUM. Callers must snap the result up
/// to the model's supported set.
fn budget_to_level(budget: i64) -> &'static str {
    match budget {
        i64::MIN..=1024 => "minimal",
        1025..=4096 => "low",
        4097..=16384 => "medium",
        16385..=i64::MAX => "high",
    }
}

/// Local policy, the exact inverse of [`budget_to_level`]: representative
/// budgets for each tier, chosen so banding inverts losslessly (1024→minimal,
/// 4096→low, 8192→medium, 24576→high) and every value stays inside both 2.5
/// models' documented ranges.
fn level_to_budget(level: &str) -> i64 {
    match level {
        "minimal" => 1024,
        "low" => 4096,
        "medium" => 8192,
        _ => 24_576,
    }
}

/// Effort values that have a tier on the shared lossy axis. `none`,
/// `xhigh`, `max` and unknown values have no counterpart.
fn effort_tier(effort: Option<&str>) -> Option<&'static str> {
    match effort.and_then(ReasoningEffort::parse) {
        Some(ReasoningEffort::Minimal) => Some("minimal"),
        Some(ReasoningEffort::Low) => Some("low"),
        Some(ReasoningEffort::Medium) => Some("medium"),
        Some(ReasoningEffort::High) => Some("high"),
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

/// Validate the thinking object shape: an object with a known `type` and only
/// keys this mapping understands.
fn validate_thinking_shape(request: &UnifiedRequest) -> Result<ThinkingMode, ProviderError> {
    let Some(thinking) = request.options.thinking.as_ref() else {
        return Err(unsupported(
            RULE_TYPE,
            "thinking",
            "missing thinking object",
        ));
    };
    let Some(thinking) = thinking.as_object() else {
        return Err(unsupported(
            RULE_TYPE,
            "thinking",
            "thinking must be an object with a type field",
        ));
    };
    if thinking
        .keys()
        .any(|key| key != "type" && key != "budget_tokens")
    {
        return Err(unsupported(
            RULE_FIELDS,
            "thinking",
            "only type and budget_tokens are mapped for Vertex Gemini",
        ));
    }
    match thinking.get("type").and_then(Value::as_str) {
        Some("disabled") => Ok(ThinkingMode::Disabled),
        Some("enabled") => Ok(ThinkingMode::Enabled),
        Some("adaptive") => Ok(ThinkingMode::Adaptive),
        _ => Err(unsupported(
            RULE_TYPE,
            "thinking.type",
            "unknown thinking type",
        )),
    }
}

/// Adapt a request for Vertex: validate per-model facts, merge thinking and
/// effort into one control, and apply the user-licensed lossy budget→level
/// rewrite for Gemini 3 models. Rejections never modify the request; merges
/// are idempotent so agent-loop rechecks converge.
pub(super) fn adapt_request(
    id: &str,
    ctx: &Ctx,
    request: &mut UnifiedRequest,
) -> Result<(), ProviderError> {
    if ctx.upstream != Upstream::GoogleVertex {
        return Ok(());
    }
    let mode = request
        .options
        .thinking
        .as_ref()
        .map(|_| validate_thinking_shape(request))
        .transpose()?;
    match generation(id) {
        Generation::Budget(facts) => adapt_budget_generation(request, facts, mode),
        Generation::Level(supported) => adapt_level_generation(request, supported, mode),
    }
}

/// 2.5 models: thinking maps to thinkingBudget; effort maps lossily onto the
/// same bands (user-licensed local policy) and merges with an explicit budget
/// exactly like the Gemini 3 side merges with a level.
fn adapt_budget_generation(
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

/// Gemini 3+ models: effort and budget both resolve to a thinking level.
fn adapt_level_generation(
    request: &mut UnifiedRequest,
    supported: &'static [&'static str],
    mode: Option<ThinkingMode>,
) -> Result<(), ProviderError> {
    let effort = request.options.reasoning_effort.as_deref();
    let budget = thinking_budget_i64(request);
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

    use super::*;
    use crate::models::{adapt_request as adapt, profile};

    fn vertex(model: &'static str) -> Ctx<'static> {
        Ctx {
            model,
            upstream: Upstream::GoogleVertex,
            dialect: Dialect::GeminiGenerateContent,
        }
    }

    #[test]
    fn vertex_uses_generate_content_with_a_thinking_level_effort_field() {
        let found = profile("gemini-2.5-pro", Upstream::GoogleVertex).expect("vertex");
        assert_eq!(found.default_dialect, Dialect::GeminiGenerateContent);
        assert_eq!(found.dialects, &[Dialect::GeminiGenerateContent]);
        assert_eq!(found.effort_field, EffortField::ThinkingLevel);
        assert_eq!(profile("gemini-2.5-pro", Upstream::Tokenhub), None);
        assert_eq!(profile("gemini-unreleased", Upstream::GoogleVertex), None);
    }

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
    fn disabling_thinking_follows_per_model_facts() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "disabled"}));
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_ok());
        match adapt(&vertex("gemini-2.5-pro"), &mut request) {
            Err(ProviderError::Public { error, .. }) => {
                assert_eq!(error.param.as_deref(), Some("thinking"));
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
        assert!(adapt(&vertex("gemini-3.5-flash"), &mut request).is_err());
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 0}));
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_ok());
        assert!(adapt(&vertex("gemini-3.1-pro-preview"), &mut request).is_err());
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

    #[test]
    fn adaptive_requests_keep_the_model_default_or_the_effort_level() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "adaptive"}));
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_ok());
        assert!(adapt(&vertex("gemini-3.5-flash"), &mut request).is_ok());
        request.options.reasoning_effort = Some("low".into());
        assert!(adapt(&vertex("gemini-3.5-flash"), &mut request).is_ok());
        assert_eq!(request.options.thinking, None);
    }

    #[test]
    fn malformed_thinking_objects_are_rejected_without_rewrites() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "auto"}));
        assert!(adapt(&vertex("gemini-3.5-flash"), &mut request).is_err());
        request.options.thinking = Some(json!({"type": "enabled", "display": "updates"}));
        assert!(adapt(&vertex("gemini-3.5-flash"), &mut request).is_err());
        request.options.thinking = Some(json!("enabled"));
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_err());
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 1024}));
        let vivgrid = Ctx {
            upstream: Upstream::Vivgrid,
            ..vertex("gemini-2.5-flash")
        };
        assert!(adapt(&vivgrid, &mut request).is_ok());
    }

    #[test]
    fn rewrites_are_idempotent_across_agent_loop_rounds() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 8192}));
        assert!(adapt(&vertex("gemini-3.5-flash"), &mut request).is_ok());
        let first = request.options.reasoning_effort.clone();
        assert!(adapt(&vertex("gemini-3.5-flash"), &mut request).is_ok());
        assert_eq!(request.options.reasoning_effort, first);
        assert_eq!(request.options.thinking, None);

        // The 2.5 effort→budget rewrite converges the same way.
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("medium".into());
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_ok());
        let budgeted = request.options.thinking.clone();
        assert!(adapt(&vertex("gemini-2.5-flash"), &mut request).is_ok());
        assert_eq!(request.options.thinking, budgeted);
        assert_eq!(request.options.reasoning_effort, None);
    }
}
