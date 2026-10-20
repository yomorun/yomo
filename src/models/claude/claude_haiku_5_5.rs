//! Claude Haiku 5.5 on Bedrock Converse: adaptive thinking and
//! `output_config.effort = high` through additionalModelRequestFields; no
//! thinking budget.

use crate::models::compat::{reasoning_effort, thinking_budget, thinking_mode};
use crate::models::{Ctx, Profile, ReasoningEffort, ThinkingMode, unsupported};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::EffortField;

const RULE: &str = "bedrock-haiku-5-5-reasoning-controls-only";

pub(super) fn bedrock_profile() -> Profile {
    Profile {
        effort_field: EffortField::OutputConfig,
        ..super::bedrock_profile()
    }
}

/// Reject controls outside this model's rule set; nothing is silently dropped
/// or mapped.
pub(super) fn adapt_bedrock_request(
    _ctx: &Ctx,
    request: &UnifiedRequest,
) -> Result<(), ProviderError> {
    if request.options.thinking.is_some() && thinking_mode(request) != Some(ThinkingMode::Adaptive)
    {
        return Err(unsupported(
            RULE,
            "thinking mapping",
            "only adaptive thinking is supported",
        ));
    }
    if request.options.reasoning_effort.is_some()
        && reasoning_effort(request) != Some(ReasoningEffort::High)
    {
        return Err(unsupported(
            RULE,
            "reasoning_effort",
            "only effort high is supported",
        ));
    }
    if thinking_budget(request).is_some() {
        return Err(unsupported(
            RULE,
            "thinking.budget_tokens",
            "no thinking budget is accepted",
        ));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::models::{adapt_request, profile};
    use crate::upstreams::{Dialect, Upstream};

    fn ctx() -> Ctx<'static> {
        Ctx {
            model: "claude-haiku-5-5",
            upstream: Upstream::AwsBedrock,
            dialect: Dialect::BedrockConverse,
        }
    }

    #[test]
    fn allows_only_supported_adaptive_effort_controls() {
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("high".into());
        request.options.thinking = Some(json!({"type": "adaptive"}));
        assert!(adapt_request(&ctx(), &mut request).is_ok());

        request.options.thinking = Some(json!({"type": "enabled"}));
        assert!(adapt_request(&ctx(), &mut request).is_err());
        request.options.thinking = Some(json!({"type": "adaptive", "budget_tokens": 4096}));
        match adapt_request(&ctx(), &mut request) {
            Err(ProviderError::Public { error, .. }) => {
                assert_eq!(error.param.as_deref(), Some("thinking.budget_tokens"));
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
        request.options.thinking = None;
        request.options.reasoning_effort = Some("low".into());
        assert!(adapt_request(&ctx(), &mut request).is_err());
        request.options.reasoning_effort = Some("turbo".into());
        assert!(adapt_request(&ctx(), &mut request).is_err());
    }

    #[test]
    fn effort_is_carried_by_output_config() {
        let haiku = profile("claude-haiku-5-5", Upstream::AwsBedrock).expect("profile");
        assert_eq!(haiku.effort_field, EffortField::OutputConfig);
        assert_eq!(haiku.dialects, &[Dialect::BedrockConverse]);
    }
}
