//! Kimi K3 and K2.8 Preview: request rules.
//!
//! Both models always think. They have no `thinking` parameter and are steered
//! with `reasoning_effort` only, so a bare `thinking: enabled` is equivalent to
//! omitting the field. TokenHub accepts that bare form and removes it; every
//! other upstream receives the model-level effort rule only.

use serde_json::Value;

use crate::models::compat::require_effort_values;
use crate::models::{Ctx, ReasoningEffort, unsupported};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::Upstream;

const EFFORT: &str = "kimi-k3-effort";
const THINKING: &str = "kimi-k3-thinking";

/// K3/K2.8 accept low/high/max effort everywhere; TokenHub also normalizes a
/// bare `thinking: enabled`.
pub(super) fn adapt_request(ctx: &Ctx, request: &mut UnifiedRequest) -> Result<(), ProviderError> {
    require_effort_values(
        EFFORT,
        request,
        &[
            ReasoningEffort::Low,
            ReasoningEffort::High,
            ReasoningEffort::Max,
        ],
    )?;
    if ctx.upstream != Upstream::Tokenhub {
        return Ok(());
    }
    remove_always_on_thinking(request)
}

/// Drop a bare `thinking: enabled`; reject anything the model cannot represent.
fn remove_always_on_thinking(request: &mut UnifiedRequest) -> Result<(), ProviderError> {
    let Some(thinking) = request.options.thinking.as_ref() else {
        return Ok(());
    };
    let Some(object) = thinking.as_object() else {
        return Err(unsupported(
            THINKING,
            "thinking",
            "thinking must be an object with a type field",
        ));
    };
    let kind = object
        .get("type")
        .and_then(Value::as_str)
        .map(str::to_owned);
    let has_extra_keys = object.len() != 1;
    match kind.as_deref() {
        Some("enabled") if !has_extra_keys => {
            request.options.thinking = None;
            Ok(())
        }
        Some("enabled") => Err(unsupported(
            THINKING,
            "thinking",
            "K3 has no thinking budget; only a bare enabled type is accepted",
        )),
        Some("disabled") => Err(unsupported(
            THINKING,
            "thinking",
            "K3 always thinks and cannot be disabled",
        )),
        Some("adaptive") | Some("between_tools") => Err(unsupported(
            THINKING,
            "thinking",
            "K3 does not accept this thinking type",
        )),
        _ => Err(unsupported(
            THINKING,
            "thinking.type",
            "unknown thinking type",
        )),
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::models::adapt_request as adapt;
    use crate::upstreams::Dialect;

    fn ctx(upstream: Upstream, model: &'static str) -> Ctx<'static> {
        Ctx {
            model,
            upstream,
            dialect: Dialect::OpenAiChatCompletions,
        }
    }

    fn with_effort(effort: &str) -> UnifiedRequest {
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some(effort.into());
        request
    }

    #[test]
    fn accepts_low_high_and_max_effort_on_every_upstream() {
        for model in ["kimi-k3", "kimi-k2.8-preview"] {
            for upstream in [Upstream::Tokenhub, Upstream::VolcengineArk, Upstream::Vllm] {
                for effort in ["low", "high", "max"] {
                    let mut request = with_effort(effort);
                    assert!(
                        adapt(&ctx(upstream, model), &mut request).is_ok(),
                        "{model} {upstream:?} {effort}"
                    );
                }
            }
        }
    }

    #[test]
    fn rejects_other_recognized_effort_values_on_every_upstream() {
        for model in ["kimi-k3", "kimi-k2.8-preview"] {
            for upstream in [Upstream::Tokenhub, Upstream::VolcengineArk, Upstream::Vllm] {
                for effort in ["none", "minimal", "medium", "xhigh"] {
                    let mut request = with_effort(effort);
                    match adapt(&ctx(upstream, model), &mut request) {
                        Err(ProviderError::Public { error, .. }) => {
                            assert_eq!(error.param.as_deref(), Some("reasoning_effort"));
                            assert!(error.message.contains(EFFORT));
                        }
                        other => panic!(
                            "expected a public denial for {model} {upstream:?} {effort}, got {other:?}"
                        ),
                    }
                    assert_eq!(request.options.reasoning_effort.as_deref(), Some(effort));
                }
            }
        }
    }

    #[test]
    fn tokenhub_bare_enabled_thinking_is_removed_idempotently() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "enabled"}));
        assert!(adapt(&ctx(Upstream::Tokenhub, "kimi-k3"), &mut request).is_ok());
        assert_eq!(request.options.thinking, None);
        assert!(adapt(&ctx(Upstream::Tokenhub, "kimi-k3"), &mut request).is_ok());
        assert_eq!(request.options.thinking, None);
    }

    #[test]
    fn tokenhub_thinking_that_cannot_be_honored_is_rejected_without_rewrites() {
        for thinking in [
            json!({"type": "disabled"}),
            json!({"type": "adaptive"}),
            json!({"type": "between_tools"}),
            json!({"type": "enabled", "budget_tokens": 1024}),
            json!({"type": "auto"}),
            json!("enabled"),
        ] {
            let mut request = UnifiedRequest::default();
            request.options.thinking = Some(thinking.clone());
            match adapt(&ctx(Upstream::Tokenhub, "kimi-k3"), &mut request) {
                Err(ProviderError::Public { error, .. }) => {
                    assert!(error.message.contains(THINKING));
                }
                other => panic!("expected a public denial for {thinking}, got {other:?}"),
            }
            assert_eq!(request.options.thinking, Some(thinking));
        }
    }

    #[test]
    fn thinking_is_left_alone_outside_tokenhub() {
        let mut request = with_effort("low");
        request.options.thinking = Some(json!({"type": "disabled"}));
        assert!(adapt(&ctx(Upstream::VolcengineArk, "kimi-k3"), &mut request).is_ok());
        assert_eq!(request.options.reasoning_effort.as_deref(), Some("low"));
        assert_eq!(request.options.thinking, Some(json!({"type": "disabled"})));
    }
}
