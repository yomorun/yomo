//! Claude Opus 4.7: adaptive and disabled thinking;
//! budget-style `enabled` and `between_tools` are rejected.
//! `output_config.effort` accepts low/medium/high/xhigh/max; `none` and
//! `minimal` are rejected. Temperature must be 1 while thinking is
//! adaptive.

use crate::models::{Ctx, Profile, ReasoningEffort, ThinkingMode};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::EffortField;

const RULE: &str = "claude-opus-4-7-reasoning-controls";

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
        &[ThinkingMode::Adaptive, ThinkingMode::Disabled],
    )?;
    super::require_effort(
        RULE,
        request,
        &[
            ReasoningEffort::Low,
            ReasoningEffort::Medium,
            ReasoningEffort::High,
            ReasoningEffort::XHigh,
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
            model: "claude-opus-4-7",
            upstream: Upstream::AwsBedrock,
            dialect: Dialect::BedrockConverse,
        }
    }

    #[test]
    fn accepts_supported_thinking_and_effort() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "adaptive"}));
        request.options.reasoning_effort = Some("xhigh".into());
        request.options.temperature = Some(1.0);
        assert!(adapt_request(&ctx(), &mut request).is_ok());

        request.options.thinking = Some(json!({"type": "disabled"}));
        request.options.reasoning_effort = None;
        request.options.temperature = None;
        assert!(adapt_request(&ctx(), &mut request).is_ok());
    }

    #[test]
    fn rejects_unsupported_controls_with_the_rule() {
        for thinking in [
            json!({"type": "enabled", "budget_tokens": 1024}),
            json!({"type": "between_tools"}),
        ] {
            let mut request = UnifiedRequest::default();
            request.options.max_output_tokens = Some(4096);
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
