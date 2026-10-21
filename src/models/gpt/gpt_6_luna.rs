//! GPT-6-Luna on Azure: thinking modes are unsupported, but an adaptive request
//! can be carried by explicit reasoning effort over Responses.

use serde_json::Value;

use crate::models::compat::{reasoning_effort, thinking_mode};
use crate::models::{Ctx, ReasoningEffort, unsupported};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::Dialect;

/// Whether `thinking` is exactly `{"type":"adaptive"}` and a non-none effort
/// over Responses already carries the request.
fn adaptive_carried_by_effort(dialect: Dialect, request: &UnifiedRequest) -> bool {
    dialect == Dialect::OpenAiResponses
        && reasoning_effort(request).is_some_and(|effort| effort != ReasoningEffort::None)
        && request
            .options
            .thinking
            .as_ref()
            .and_then(Value::as_object)
            .is_some_and(|thinking| {
                thinking.len() == 1
                    && thinking.get("type").and_then(Value::as_str) == Some("adaptive")
            })
}

const THINKING_UNSUPPORTED: &str = "gpt-6-luna-thinking-unsupported";

/// Drop redundant adaptive thinking; reject every other thinking mode.
pub(super) fn adapt_azure_request(
    ctx: &Ctx,
    request: &mut UnifiedRequest,
) -> Result<(), ProviderError> {
    if adaptive_carried_by_effort(ctx.dialect, request) {
        request.options.thinking = None;
    } else if thinking_mode(request).is_some() {
        return Err(unsupported(
            THINKING_UNSUPPORTED,
            "thinking.type",
            "the thinking parameter is not supported; only adaptive carried by an explicit effort is mapped",
        ));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::upstreams::Upstream;

    fn ctx(dialect: Dialect) -> Ctx<'static> {
        Ctx {
            model: "gpt-6-luna",
            upstream: Upstream::AzureOpenai,
            dialect,
        }
    }

    fn request(effort: Option<&str>, thinking: Value) -> UnifiedRequest {
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = effort.map(Into::into);
        request.options.thinking = Some(thinking);
        request
    }

    #[test]
    fn adaptive_thinking_is_dropped_only_when_responses_effort_carries_it() {
        let mut req = request(Some("high"), json!({"type": "adaptive"}));
        assert!(adapt_azure_request(&ctx(Dialect::OpenAiResponses), &mut req).is_ok());
        assert_eq!(req.options.thinking, None);
        assert_eq!(req.options.reasoning_effort.as_deref(), Some("high"));
    }

    #[test]
    fn other_thinking_requests_are_rejected_without_rewriting() {
        for (dialect, effort, thinking) in [
            (Dialect::OpenAiResponses, None, json!({"type": "adaptive"})),
            (
                Dialect::OpenAiResponses,
                Some("none"),
                json!({"type": "adaptive"}),
            ),
            (
                Dialect::OpenAiResponses,
                Some("high"),
                json!({"type": "adaptive", "budget_tokens": 4096}),
            ),
            (
                Dialect::OpenAiChatCompletions,
                Some("high"),
                json!({"type": "adaptive"}),
            ),
            (
                Dialect::OpenAiResponses,
                Some("high"),
                json!({"type": "enabled"}),
            ),
        ] {
            let mut req = request(effort, thinking.clone());
            match adapt_azure_request(&ctx(dialect), &mut req) {
                Err(ProviderError::Public { error, .. }) => {
                    assert_eq!(error.param.as_deref(), Some("thinking.type"));
                }
                other => panic!("expected a public denial, got {other:?}"),
            }
            assert_eq!(req.options.thinking, Some(thinking));
        }
    }

    #[test]
    fn requests_without_thinking_pass() {
        let mut req = UnifiedRequest::default();
        assert!(adapt_azure_request(&ctx(Dialect::OpenAiChatCompletions), &mut req).is_ok());
    }
}
