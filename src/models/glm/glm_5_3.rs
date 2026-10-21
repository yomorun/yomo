//! GLM-5.3 family: profile and request rules.
//!
//! GLM-5.3 and GLM-5.3-Flash always think; the model cannot disable thinking.

use serde_json::Value;

use crate::models::{Ctx, Profile, ThinkingMode, unsupported};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::Dialect;

const THINKING_CANNOT_BE_DISABLED: &str = "glm-5-3-thinking-cannot-be-disabled";

/// TokenHub serves GLM-5.3-Flash over Chat, Responses and Messages; Chat is the default.
pub(super) fn flash_tokenhub_profile() -> Profile {
    Profile {
        dialects: &[
            Dialect::OpenAiChatCompletions,
            Dialect::OpenAiResponses,
            Dialect::AnthropicMessages,
        ],
        ..Profile::chat()
    }
}

/// GLM-5.3 and GLM-5.3-Flash always think, regardless of the serving upstream.
///
/// Forced tool choice together with thinking is not established either way, so
/// it is passed through untouched. Effort is never rewritten: the evidence for
/// an exhaustive effort list or a max->high mapping conflicts.
pub(super) fn adapt_request(_ctx: &Ctx, request: &mut UnifiedRequest) -> Result<(), ProviderError> {
    let mode = request
        .options
        .thinking
        .as_ref()
        .and_then(|value| value.get("type"))
        .and_then(Value::as_str)
        .and_then(ThinkingMode::parse);
    if mode == Some(ThinkingMode::Disabled) {
        return Err(unsupported(
            THINKING_CANNOT_BE_DISABLED,
            "thinking",
            "this model always thinks and cannot be disabled",
        ));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::models::{adapt_request as adapt, profile};
    use crate::upstreams::Upstream;

    fn ctx(upstream: Upstream, model: &'static str) -> Ctx<'static> {
        Ctx {
            model,
            upstream,
            dialect: Dialect::OpenAiChatCompletions,
        }
    }

    fn with_thinking(kind: &str) -> UnifiedRequest {
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("max".into());
        request.options.thinking = Some(json!({ "type": kind }));
        request
    }

    #[test]
    fn rejects_disabled_thinking_on_every_upstream_and_keeps_effort() {
        for model in ["GLM-5.3", "GLM-5.3-Flash"] {
            for upstream in [Upstream::Tokenhub, Upstream::VolcengineArk, Upstream::Vllm] {
                let mut request = with_thinking("disabled");
                match adapt(&ctx(upstream, model), &mut request) {
                    Err(ProviderError::Public { error, .. }) => {
                        assert_eq!(error.code.as_deref(), Some("unsupported_capability"));
                        assert_eq!(error.param.as_deref(), Some("thinking"));
                        assert!(error.message.contains(THINKING_CANNOT_BE_DISABLED));
                    }
                    other => panic!("expected a public denial, got {other:?}"),
                }
            }
        }
        for (upstream, model) in [
            (Upstream::Tokenhub, "GLM-5.3"),
            (Upstream::VolcengineArk, "GLM-5.3-Flash"),
            (Upstream::Vllm, "GLM-5.3"),
        ] {
            let mut request = with_thinking("enabled");
            assert!(adapt(&ctx(upstream, model), &mut request).is_ok());
            assert_eq!(request.options.reasoning_effort.as_deref(), Some("max"));
            assert_eq!(request.options.thinking, Some(json!({ "type": "enabled" })));
        }
    }

    #[test]
    fn profile_depends_on_upstream() {
        let tokenhub = profile("glm-5.3-flash", Upstream::Tokenhub).expect("tokenhub");
        assert_eq!(tokenhub.default_dialect, Dialect::OpenAiChatCompletions);
        assert!(tokenhub.dialects.contains(&Dialect::OpenAiResponses));
        assert!(tokenhub.dialects.contains(&Dialect::AnthropicMessages));
        assert_eq!(
            profile("glm-5.3", Upstream::Tokenhub),
            Some(Profile::chat())
        );
        assert_eq!(
            profile("glm-5.3-flash", Upstream::Vllm),
            Some(Profile::vllm_chat())
        );
        assert_eq!(profile("glm-5.3-flash", Upstream::AzureOpenai), None);
        assert_eq!(profile("glm-unreleased", Upstream::Tokenhub), None);
    }
}
