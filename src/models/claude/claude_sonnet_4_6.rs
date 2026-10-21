//! Claude Sonnet 4.6: budget-style `enabled` (budget floor
//! 1024, below `max_output_tokens`), `adaptive` and `disabled` thinking;
//! `between_tools` is rejected. `output_config.effort` accepts
//! low/medium/high/max; `xhigh`, `none` and `minimal` are rejected.
//! Temperature must be 1 while thinking is enabled or adaptive.

use crate::models::compat::{reasoning_effort, thinking_budget, thinking_mode};
use crate::models::{Ctx, Profile, ReasoningEffort, ThinkingMode, unsupported};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::EffortField;

const RULE: &str = "claude-sonnet-4-6-reasoning-controls";
/// Upstream floor: `budget_tokens >= 1024`.
const BUDGET_FLOOR: u64 = 1024;

pub(super) fn bedrock_profile() -> Profile {
    Profile {
        effort_field: EffortField::OutputConfig,
        ..super::bedrock_profile()
    }
}

/// Reject controls outside this model's rule set; nothing is silently dropped
/// or mapped.
pub(super) fn adapt_request(_ctx: &Ctx, request: &UnifiedRequest) -> Result<(), ProviderError> {
    if request.options.thinking.is_some() {
        match thinking_mode(request) {
            Some(ThinkingMode::Enabled) => {
                let budget = thinking_budget(request).ok_or_else(|| {
                    unsupported(
                        RULE,
                        "thinking.budget_tokens",
                        "budget_tokens is required for enabled thinking",
                    )
                })?;
                if budget < BUDGET_FLOOR {
                    return Err(unsupported(
                        RULE,
                        "thinking.budget_tokens",
                        "the budget floor is 1024",
                    ));
                }
                if let Some(max) = request
                    .options
                    .max_output_tokens
                    .filter(|max| budget >= *max as u64)
                {
                    return Err(unsupported(
                        RULE,
                        "thinking.budget_tokens",
                        &format!("the budget must stay below max_output_tokens ({max})"),
                    ));
                }
            }
            Some(ThinkingMode::Disabled | ThinkingMode::Adaptive) => {}
            Some(ThinkingMode::BetweenTools) => {
                return Err(unsupported(
                    RULE,
                    "thinking",
                    "between_tools thinking is not supported for this model",
                ));
            }
            None => {
                return Err(unsupported(RULE, "thinking.type", "unknown thinking type"));
            }
        }
    }
    if let Some(effort) = reasoning_effort(request) {
        match effort {
            ReasoningEffort::Low
            | ReasoningEffort::Medium
            | ReasoningEffort::High
            | ReasoningEffort::Max => {}
            ReasoningEffort::XHigh => {
                return Err(unsupported(
                    RULE,
                    "reasoning_effort",
                    "xhigh is not supported for this model; supported levels are low/medium/high/max",
                ));
            }
            ReasoningEffort::None | ReasoningEffort::Minimal => {
                return Err(unsupported(
                    RULE,
                    "reasoning_effort",
                    "supported effort levels are low/medium/high/xhigh/max",
                ));
            }
        }
    }
    super::reject_modified_sampling_with_thinking(request)
}

/// Bedrock Converse rejects client-only thinking keys.
pub(super) fn adapt_bedrock_request(
    _ctx: &Ctx,
    request: &UnifiedRequest,
) -> Result<(), ProviderError> {
    super::reject_client_only_thinking_keys(request)
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::models::{adapt_request, profile};
    use crate::upstreams::{Dialect, Upstream};

    fn ctx() -> Ctx<'static> {
        Ctx {
            model: "claude-sonnet-4-6",
            upstream: Upstream::AwsBedrock,
            dialect: Dialect::BedrockConverse,
        }
    }

    #[test]
    fn allows_budget_thinking_within_documented_bounds() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 2048}));
        request.options.max_output_tokens = Some(4000);
        assert!(adapt_request(&ctx(), &mut request).is_ok());
        request.options.reasoning_effort = Some("high".into());
        assert!(adapt_request(&ctx(), &mut request).is_ok());
        request.options.reasoning_effort = Some("max".into());
        assert!(adapt_request(&ctx(), &mut request).is_ok());

        request.options.thinking = Some(json!({"type": "disabled"}));
        assert!(adapt_request(&ctx(), &mut request).is_ok());
        request.options.thinking = Some(json!({"type": "adaptive"}));
        assert!(adapt_request(&ctx(), &mut request).is_ok());
    }

    #[test]
    fn budget_bounds_and_required_budget_are_enforced() {
        let mut request = UnifiedRequest::default();
        request.options.max_output_tokens = Some(4000);
        request.options.thinking = Some(json!({"type": "enabled"}));
        match adapt_request(&ctx(), &mut request) {
            Err(ProviderError::Public { error, .. }) => {
                assert_eq!(error.param.as_deref(), Some("thinking.budget_tokens"));
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 1023}));
        assert!(adapt_request(&ctx(), &mut request).is_err());
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 4000}));
        assert!(adapt_request(&ctx(), &mut request).is_err());
        // Without a known max_output_tokens the relation is left to upstream.
        request.options.max_output_tokens = None;
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 8192}));
        assert!(adapt_request(&ctx(), &mut request).is_ok());
    }

    #[test]
    fn between_tools_xhigh_and_sampling_are_rejected() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "between_tools"}));
        assert!(adapt_request(&ctx(), &mut request).is_err());
        request.options.reasoning_effort = Some("xhigh".into());
        request.options.thinking = None;
        match adapt_request(&ctx(), &mut request) {
            Err(ProviderError::Public { error, .. }) => {
                assert!(error.message.contains("xhigh"), "{}", error.message);
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
        request.options.reasoning_effort = Some("none".into());
        assert!(adapt_request(&ctx(), &mut request).is_err());
        request.options.reasoning_effort = Some("minimal".into());
        assert!(adapt_request(&ctx(), &mut request).is_err());
    }

    #[test]
    fn sampling_client_only_keys_and_profile_facts() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 2048}));
        request.options.temperature = Some(0.7);
        match adapt_request(&ctx(), &mut request) {
            Err(ProviderError::Public { error, .. }) => {
                assert_eq!(error.param.as_deref(), Some("temperature"));
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
        request.options.temperature = Some(1.0);
        assert!(adapt_request(&ctx(), &mut request).is_ok());
        request.options.thinking = Some(
            json!({"type": "adaptive", "block_binding": {"prefix_mismatch_behavior": "error"}}),
        );
        assert!(adapt_request(&ctx(), &mut request).is_err());
        request.options.thinking = Some(json!({"type": "adaptive", "display": "updates"}));
        assert!(adapt_request(&ctx(), &mut request).is_err());

        let sonnet = profile("claude-sonnet-4-6", Upstream::AwsBedrock).expect("profile");
        assert_eq!(sonnet.effort_field, EffortField::OutputConfig);
        assert_eq!(sonnet.dialects, &[Dialect::BedrockConverse]);
    }
}
