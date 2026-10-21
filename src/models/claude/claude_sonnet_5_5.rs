//! Claude Sonnet 5.5: adaptive and `between_tools`
//! thinking; budget-style `enabled` and `disabled` are rejected.
//! `output_config.effort` accepts low/medium/high/xhigh/max; `none` and
//! `minimal` are rejected. Temperature must be 1 while thinking is
//! enabled or adaptive.

use crate::models::compat::{reasoning_effort, thinking_budget, thinking_mode};
use crate::models::{Ctx, Profile, ReasoningEffort, ThinkingMode, unsupported};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::EffortField;

const RULE: &str = "claude-sonnet-5-5-reasoning-controls";

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
            Some(ThinkingMode::Adaptive | ThinkingMode::BetweenTools) => {}
            Some(ThinkingMode::Disabled) => {
                return Err(unsupported(
                    RULE,
                    "thinking",
                    "this model cannot disable thinking; use thinking.type between_tools",
                ));
            }
            Some(ThinkingMode::Enabled) => {
                return Err(unsupported(
                    RULE,
                    "thinking",
                    "budget-style enabled thinking is not supported for this model; use adaptive thinking with output_config.effort",
                ));
            }
            None => {
                return Err(unsupported(RULE, "thinking.type", "unknown thinking type"));
            }
        }
        if thinking_budget(request).is_some() {
            return Err(unsupported(
                RULE,
                "thinking.budget_tokens",
                "no thinking budget is accepted",
            ));
        }
    }
    if let Some(effort) = reasoning_effort(request) {
        match effort {
            ReasoningEffort::Low
            | ReasoningEffort::Medium
            | ReasoningEffort::High
            | ReasoningEffort::XHigh
            | ReasoningEffort::Max => {}
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
            model: "claude-sonnet-5-5",
            upstream: Upstream::AwsBedrock,
            dialect: Dialect::BedrockConverse,
        }
    }

    #[test]
    fn allows_only_adaptive_and_effort_controls() {
        let mut request = UnifiedRequest::default();
        for effort in ["low", "medium", "high", "xhigh", "max"] {
            request.options.reasoning_effort = Some(effort.into());
            assert!(adapt_request(&ctx(), &mut request).is_ok(), "{effort}");
        }
        request.options.thinking = Some(json!({"type": "adaptive"}));
        assert!(adapt_request(&ctx(), &mut request).is_ok());
        request.options.thinking = Some(json!({"type": "between_tools"}));
        assert!(adapt_request(&ctx(), &mut request).is_ok());
        request.options.thinking = Some(json!({"type": "adaptive", "display": "summarized"}));
        assert!(adapt_request(&ctx(), &mut request).is_ok());
    }

    #[test]
    fn budget_style_and_disabled_thinking_are_rejected() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 2048}));
        match adapt_request(&ctx(), &mut request) {
            Err(ProviderError::Public { error, .. }) => {
                assert!(error.message.contains("not supported"), "{}", error.message);
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
        request.options.thinking = Some(json!({"type": "disabled"}));
        assert!(adapt_request(&ctx(), &mut request).is_err());
        request.options.thinking = Some(json!({"type": "adaptive", "budget_tokens": 2048}));
        assert!(adapt_request(&ctx(), &mut request).is_err());
        request.options.thinking = Some(json!({"type": "auto"}));
        assert!(adapt_request(&ctx(), &mut request).is_err());
    }

    #[test]
    fn effort_none_minimal_and_sampling_are_rejected() {
        let mut request = UnifiedRequest::default();
        for effort in ["none", "minimal"] {
            request.options.reasoning_effort = Some(effort.into());
            assert!(adapt_request(&ctx(), &mut request).is_err(), "{effort}");
        }
        request.options.reasoning_effort = Some("high".into());
        request.options.thinking = Some(json!({"type": "adaptive"}));
        request.options.temperature = Some(0.7);
        match adapt_request(&ctx(), &mut request) {
            Err(ProviderError::Public { error, .. }) => {
                assert_eq!(error.param.as_deref(), Some("temperature"));
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
        request.options.temperature = Some(1.0);
        assert!(adapt_request(&ctx(), &mut request).is_ok());
    }

    #[test]
    fn client_only_thinking_keys_are_rejected_and_effort_is_carried_by_output_config() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(
            json!({"type": "adaptive", "block_binding": {"prefix_mismatch_behavior": "error"}}),
        );
        assert!(adapt_request(&ctx(), &mut request).is_err());
        request.options.thinking = Some(json!({"type": "adaptive", "display": "updates"}));
        assert!(adapt_request(&ctx(), &mut request).is_err());

        let sonnet = profile("claude-sonnet-5-5", Upstream::AwsBedrock).expect("profile");
        assert_eq!(sonnet.effort_field, EffortField::OutputConfig);
        assert_eq!(sonnet.dialects, &[Dialect::BedrockConverse]);
    }
}
