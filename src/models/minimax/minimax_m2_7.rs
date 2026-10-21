//! MiniMax M2.7: request rules.
//!
//! M2.7 controls reasoning with `thinking`; it has no `reasoning_effort`
//! parameter on any upstream.

use crate::models::Ctx;
use crate::models::compat::require_effort_values;
use crate::provider::{ProviderError, UnifiedRequest};

const EFFORT_UNSUPPORTED: &str = "minimax-m2-7-effort-unsupported";

/// M2.7 has no effort parameter.
pub(super) fn adapt_request(_ctx: &Ctx, request: &mut UnifiedRequest) -> Result<(), ProviderError> {
    require_effort_values(EFFORT_UNSUPPORTED, request, &[])
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::models::adapt_request as adapt;
    use crate::upstreams::{Dialect, Upstream};

    fn ctx(upstream: Upstream) -> Ctx<'static> {
        Ctx {
            model: "minimax-m2.7",
            upstream,
            dialect: Dialect::OpenAiChatCompletions,
        }
    }

    #[test]
    fn rejects_effort_on_every_upstream_and_keeps_thinking() {
        for upstream in [Upstream::Tokenhub, Upstream::VolcengineArk, Upstream::Vllm] {
            for effort in ["none", "minimal", "low", "medium", "high", "xhigh", "max"] {
                let mut request = UnifiedRequest::default();
                request.options.reasoning_effort = Some(effort.into());
                request.options.thinking = Some(json!({"type": "disabled"}));
                match adapt(&ctx(upstream), &mut request) {
                    Err(ProviderError::Public { error, .. }) => {
                        assert_eq!(error.param.as_deref(), Some("reasoning_effort"));
                        assert!(error.message.contains(EFFORT_UNSUPPORTED));
                    }
                    other => {
                        panic!("expected a public denial for {upstream:?} {effort}, got {other:?}")
                    }
                }
                assert_eq!(request.options.thinking, Some(json!({"type": "disabled"})));
            }
        }
    }

    #[test]
    fn unrecognized_effort_is_left_to_the_upstream() {
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("turbo".into());
        assert!(adapt(&ctx(Upstream::Tokenhub), &mut request).is_ok());
        assert!(adapt(&ctx(Upstream::Vllm), &mut request).is_ok());
    }
}
