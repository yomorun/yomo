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
use serde_json::Value;

mod gemini_2_5;
mod gemini_3_plus;

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
    match gemini_2_5::budget_facts(id) {
        Some(facts) => gemini_2_5::adapt(request, facts, mode),
        None => gemini_3_plus::adapt(request, gemini_3_plus::levels(id), mode),
    }
}

#[cfg(test)]
fn vertex(model: &'static str) -> Ctx<'static> {
    Ctx {
        model,
        upstream: Upstream::GoogleVertex,
        dialect: Dialect::GeminiGenerateContent,
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::models::{adapt_request as adapt, profile};

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
