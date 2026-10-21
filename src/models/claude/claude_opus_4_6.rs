//! Claude Opus 4.6: budget-style `enabled` (budget floor
//! 1024, below `max_tokens`), `adaptive` and `disabled` thinking;
//! `between_tools` is rejected. `output_config.effort` accepts
//! low/medium/high/max and also works next to budget thinking; `xhigh`, `none`
//! and `minimal` are rejected. Temperature must be 1 while thinking is
//! enabled or adaptive.

use crate::models::{Ctx, Profile, ReasoningEffort, ThinkingMode};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::EffortField;

const RULE: &str = "claude-opus-4-6-reasoning-controls";

pub(super) fn bedrock_profile() -> Profile {
    Profile {
        effort_field: EffortField::OutputConfig,
        ..super::bedrock_profile()
    }
}

/// Reject controls outside this model's rule set; nothing is silently dropped
/// or mapped.
pub(super) fn adapt_request(_ctx: &Ctx, request: &UnifiedRequest) -> Result<(), ProviderError> {
    super::require_thinking(
        RULE,
        request,
        &[
            ThinkingMode::Adaptive,
            ThinkingMode::Enabled,
            ThinkingMode::Disabled,
        ],
    )?;
    super::require_effort(
        RULE,
        request,
        &[
            ReasoningEffort::Low,
            ReasoningEffort::Medium,
            ReasoningEffort::High,
            ReasoningEffort::Max,
        ],
    )?;
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
    use crate::models::adapt_request;
    use crate::upstreams::{Dialect, Upstream};

    fn ctx() -> Ctx<'static> {
        Ctx {
            model: "claude-opus-4-6",
            upstream: Upstream::AwsBedrock,
            dialect: Dialect::BedrockConverse,
        }
    }

    #[test]
    fn accepts_supported_thinking_and_effort() {
        let mut request = UnifiedRequest::default();
        request.options.max_output_tokens = Some(2048);
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 1024}));
        request.options.reasoning_effort = Some("max".into());
        assert!(adapt_request(&ctx(), &mut request).is_ok());

        request.options.thinking = Some(json!({"type": "adaptive"}));
        request.options.reasoning_effort = Some("high".into());
        assert!(adapt_request(&ctx(), &mut request).is_ok());

        request.options.thinking = Some(json!({"type": "disabled"}));
        request.options.reasoning_effort = None;
        assert!(adapt_request(&ctx(), &mut request).is_ok());
    }

    #[test]
    fn rejects_unsupported_controls_with_the_rule() {
        for thinking in [
            json!({"type": "between_tools"}),
            json!({"type": "enabled", "budget_tokens": 512}),
            json!({"type": "enabled", "budget_tokens": 1024}),
        ] {
            let mut request = UnifiedRequest::default();
            request.options.max_output_tokens = Some(1024);
            request.options.thinking = Some(thinking);
            assert_rule(&mut request);
        }

        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "adaptive"}));
        request.options.temperature = Some(0.7);
        assert!(matches!(
            adapt_request(&ctx(), &mut request),
            Err(ProviderError::Public { .. })
        ));

        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "adaptive"}));
        request.options.reasoning_effort = Some("minimal".into());
        assert_rule(&mut request);
    }

    fn assert_rule(request: &mut UnifiedRequest) {
        match adapt_request(&ctx(), request) {
            Err(ProviderError::Public { error, .. }) => {
                assert!(error.message.contains(RULE), "{}", error.message);
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
    }
}
