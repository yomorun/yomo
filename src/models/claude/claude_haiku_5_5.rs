//! Claude Haiku 5.5: adaptive thinking by default, or disabled thinking at
//! high effort or below. Legacy budget-style `enabled` is rejected by the
//! model. `output_config.effort` accepts low/medium/high/xhigh/max; `none` and
//! `minimal` are rejected. Thinking and a modified temperature are
//! incompatible.

use crate::models::compat::{reasoning_effort, thinking_budget, thinking_mode};
use crate::models::{Ctx, Profile, ReasoningEffort, ThinkingMode, unsupported};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::EffortField;

const RULE: &str = "claude-haiku-5-5-reasoning-controls";

const EFFORTS: &[ReasoningEffort] = &[
    ReasoningEffort::Low,
    ReasoningEffort::Medium,
    ReasoningEffort::High,
    ReasoningEffort::XHigh,
    ReasoningEffort::Max,
];

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
            Some(ThinkingMode::Adaptive | ThinkingMode::Disabled) => {}
            Some(ThinkingMode::Enabled) => {
                return Err(unsupported(
                    RULE,
                    "thinking",
                    "budget-style enabled thinking is not supported for this model; use adaptive thinking with output_config.effort",
                ));
            }
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
        if thinking_budget(request).is_some() {
            return Err(unsupported(
                RULE,
                "thinking.budget_tokens",
                "no thinking budget is accepted",
            ));
        }
    }
    super::require_effort(RULE, request, EFFORTS)?;
    if thinking_mode(request) == Some(ThinkingMode::Disabled)
        && matches!(
            reasoning_effort(request),
            Some(ReasoningEffort::XHigh | ReasoningEffort::Max)
        )
    {
        return Err(unsupported(
            RULE,
            "reasoning_effort",
            "disabled thinking is only supported at high effort or below",
        ));
    }
    super::reject_modified_sampling_with_active_thinking(request)
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

    fn ctx(upstream: Upstream) -> Ctx<'static> {
        Ctx {
            model: "claude-haiku-5-5",
            upstream,
            dialect: if upstream == Upstream::AwsBedrock {
                Dialect::BedrockConverse
            } else {
                Dialect::OpenAiChatCompletions
            },
        }
    }

    #[test]
    fn allows_adaptive_and_disabled_effort_matrix() {
        let mut request = UnifiedRequest::default();
        for effort in ["low", "medium", "high", "xhigh", "max"] {
            request.options.thinking = Some(json!({"type": "adaptive"}));
            request.options.reasoning_effort = Some(effort.into());
            assert!(
                adapt_request(&ctx(Upstream::AwsBedrock), &mut request).is_ok(),
                "adaptive {effort}"
            );
        }
        request.options.thinking = Some(json!({"type": "disabled"}));
        for effort in ["low", "medium", "high"] {
            request.options.reasoning_effort = Some(effort.into());
            assert!(
                adapt_request(&ctx(Upstream::AwsBedrock), &mut request).is_ok(),
                "disabled {effort}"
            );
        }
        for effort in ["xhigh", "max"] {
            request.options.reasoning_effort = Some(effort.into());
            match adapt_request(&ctx(Upstream::AwsBedrock), &mut request) {
                Err(ProviderError::Public { error, .. }) => {
                    assert_eq!(error.param.as_deref(), Some("reasoning_effort"));
                    assert!(error.message.contains("high effort or below"));
                }
                other => panic!("expected a public denial for disabled {effort}, got {other:?}"),
            }
        }
        request.options.thinking = None;
        request.options.reasoning_effort = Some("medium".into());
        assert!(adapt_request(&ctx(Upstream::AwsBedrock), &mut request).is_ok());
    }

    #[test]
    fn rejects_enabled_between_tools_budget_and_unsupported_effort() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "enabled"}));
        assert!(adapt_request(&ctx(Upstream::AwsBedrock), &mut request).is_err());
        request.options.thinking = Some(json!({"type": "between_tools"}));
        assert!(adapt_request(&ctx(Upstream::AwsBedrock), &mut request).is_err());
        request.options.thinking = Some(json!({"type": "adaptive", "budget_tokens": 4096}));
        match adapt_request(&ctx(Upstream::AwsBedrock), &mut request) {
            Err(ProviderError::Public { error, .. }) => {
                assert_eq!(error.param.as_deref(), Some("thinking.budget_tokens"));
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
        request.options.thinking = Some(json!({"type": "disabled", "budget_tokens": 0}));
        assert!(adapt_request(&ctx(Upstream::AwsBedrock), &mut request).is_err());
        request.options.thinking = None;
        for effort in ["none", "minimal"] {
            request.options.reasoning_effort = Some(effort.into());
            assert!(
                adapt_request(&ctx(Upstream::AwsBedrock), &mut request).is_err(),
                "{effort}"
            );
        }
        request.options.reasoning_effort = Some("turbo".into());
        assert!(adapt_request(&ctx(Upstream::AwsBedrock), &mut request).is_ok());
    }

    #[test]
    fn model_rule_applies_outside_bedrock() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "disabled"}));
        request.options.reasoning_effort = Some("high".into());
        assert!(adapt_request(&ctx(Upstream::Vllm), &mut request).is_ok());
        request.options.reasoning_effort = Some("xhigh".into());
        assert!(adapt_request(&ctx(Upstream::Vllm), &mut request).is_err());
    }

    #[test]
    fn bedrock_client_only_keys_stay_platform_specific() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "adaptive", "block_binding": {}}));
        assert!(adapt_request(&ctx(Upstream::AwsBedrock), &mut request).is_err());
        assert!(adapt_request(&ctx(Upstream::Vllm), &mut request).is_ok());
    }

    #[test]
    fn effort_is_carried_by_output_config() {
        let haiku = profile("claude-haiku-5-5", Upstream::AwsBedrock).expect("profile");
        assert_eq!(haiku.effort_field, EffortField::OutputConfig);
        assert_eq!(haiku.dialects, &[Dialect::BedrockConverse]);
    }
}
