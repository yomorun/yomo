//! MiniMax M3: request rules.
//!
//! M3 supports thinking, and tunable thinking depth belongs to M3.1-Flash, so
//! M3 has no effort parameter on any upstream.

use crate::models::Ctx;
use crate::models::compat::require_effort_values;
use crate::provider::{ProviderError, UnifiedRequest};

const EFFORT_UNSUPPORTED: &str = "minimax-m3-effort-unsupported";

/// M3 has no effort parameter.
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
            model: "minimax-m3",
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
                request.options.thinking = Some(json!({"type": "adaptive"}));
                match adapt(&ctx(upstream), &mut request) {
                    Err(ProviderError::Public { error, .. }) => {
                        assert_eq!(error.param.as_deref(), Some("reasoning_effort"));
                        assert!(error.message.contains(EFFORT_UNSUPPORTED));
                    }
                    other => {
                        panic!("expected a public denial for {upstream:?} {effort}, got {other:?}")
                    }
                }
                assert_eq!(request.options.thinking, Some(json!({"type": "adaptive"})));
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
