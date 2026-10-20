//! Claude Haiku 4.5 on Bedrock Converse: budget-style `enabled` (budget floor
//! 1024, below `max_tokens`) and `disabled` thinking; `adaptive` and
//! `between_tools` are rejected. The model has no effort parameter, so the
//! profile keeps `EffortField::Unknown` and the binding check rejects any
//! reasoning_effort. `display` accepts only summarized/omitted, block_binding
//! is rejected, and temperature must be 1 only while thinking is enabled.

use crate::models::{Ctx, Profile, ThinkingMode};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::EffortField;

const RULE: &str = "bedrock-haiku-4-5-reasoning-controls-only";

pub(super) fn bedrock_profile() -> Profile {
    Profile {
        // This model has no effort parameter, so any effort is rejected.
        effort_field: EffortField::Unknown,
        ..super::bedrock_profile()
    }
}

/// Reject controls outside this model's rule set; nothing is silently dropped
/// or mapped.
pub(super) fn adapt_bedrock_request(
    _ctx: &Ctx,
    request: &UnifiedRequest,
) -> Result<(), ProviderError> {
    super::require_thinking(
        RULE,
        request,
        &[ThinkingMode::Enabled, ThinkingMode::Disabled],
    )?;
    super::reject_modified_sampling_with_active_thinking(request)
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::models::{adapt_request, profile};
    use crate::upstreams::{Dialect, EffortField, Upstream};

    fn ctx() -> Ctx<'static> {
        Ctx {
            model: "claude-haiku-4-5",
            upstream: Upstream::AwsBedrock,
            dialect: Dialect::BedrockConverse,
        }
    }

    #[test]
    fn accepts_budget_and_disabled_thinking() {
        let mut request = UnifiedRequest::default();
        request.options.max_output_tokens = Some(2048);
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 1024}));
        assert!(adapt_request(&ctx(), &mut request).is_ok());

        request.options.thinking = Some(json!({"type": "disabled"}));
        request.options.temperature = Some(0.7);
        assert!(adapt_request(&ctx(), &mut request).is_ok());
    }

    #[test]
    fn rejects_adaptive_thinking_and_budget_bounds_with_the_rule() {
        for thinking in [
            json!({"type": "adaptive"}),
            json!({"type": "between_tools"}),
            json!({"type": "enabled", "budget_tokens": 512}),
            json!({"type": "enabled", "budget_tokens": 1024}),
        ] {
            let mut request = UnifiedRequest::default();
            request.options.max_output_tokens = Some(1024);
            request.options.thinking = Some(thinking);
            match adapt_request(&ctx(), &mut request) {
                Err(ProviderError::Public { error, .. }) => {
                    assert!(error.message.contains(RULE), "{}", error.message);
                }
                other => panic!("expected a public denial, got {other:?}"),
            }
        }
    }

    #[test]
    fn profile_has_no_effort_field() {
        let found = profile("claude-haiku-4-5", Upstream::AwsBedrock).expect("route");
        assert_eq!(found.effort_field, EffortField::Unknown);
        assert_eq!(found.default_dialect, Dialect::BedrockConverse);
    }
}
