//! GPT family: Azure dialect routes and per-model request rules.

use crate::models::compat::{has_function_tools, reasoning_effort};
use crate::models::{Ctx, Profile, ReasoningEffort as Effort, generic_profile, unsupported};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::{Dialect, EffortField, Upstream};

mod gpt_6_luna;

const MODELS: &[&str] = &[
    "gpt-5.1-codex",
    "gpt-5.1-codex-max",
    "gpt-5.2-codex",
    "gpt-5.3-codex",
    "gpt-5.4",
    "gpt-5.4-mini",
    "gpt-5.4-nano",
    "gpt-5.5",
    "gpt-5.6-sol",
    "gpt-5.6-luna",
    "gpt-5.6-terra",
    "gpt-6-astra",
    "gpt-6-sol",
    "gpt-6-luna",
    "gpt-6.1-sol",
];

/// Codex models are Responses-only on Azure.
const AZURE_RESPONSES_ONLY: &[&str] = &[
    "gpt-5.1-codex",
    "gpt-5.1-codex-max",
    "gpt-5.2-codex",
    "gpt-5.3-codex",
];

/// Azure routes that can be reached over Chat and Responses; the dialect must
/// be chosen explicitly.
const AZURE_CHAT_AND_RESPONSES: &[&str] = &[
    "gpt-5.6-sol",
    "gpt-5.6-luna",
    "gpt-5.6-terra",
    "gpt-6-astra",
    "gpt-6-sol",
    "gpt-6-luna",
    "gpt-6.1-sol",
];

/// Azure Chat Completions function tools on GPT-5.6 require effort `none`.
const AZURE_GPT_56: &[&str] = &["gpt-5.6-sol", "gpt-5.6-luna", "gpt-5.6-terra"];
const AZURE_GPT_56_CHAT_TOOLS: &str = "gpt56-chat-tools-require-effort-none";

fn azure_profile(id: &str) -> Profile {
    if AZURE_RESPONSES_ONLY.contains(&id) {
        Profile {
            default_dialect: Dialect::OpenAiResponses,
            dialects: &[Dialect::OpenAiResponses],
            max_tokens_field: None,
            effort_field: EffortField::Reasoning,
            ..Profile::chat()
        }
    } else if AZURE_CHAT_AND_RESPONSES.contains(&id) {
        Profile {
            dialects: &[Dialect::OpenAiChatCompletions, Dialect::OpenAiResponses],
            requires_explicit_dialect: true,
            ..Profile::chat()
        }
    } else {
        Profile::chat()
    }
}

pub(super) fn profile(id: &str, upstream: Upstream) -> Option<Profile> {
    if !MODELS.contains(&id) {
        return None;
    }
    match upstream {
        Upstream::AzureOpenai => Some(azure_profile(id)),
        other => generic_profile(other),
    }
}

/// Function tools over Azure Chat require an explicit effort of `none`; an
/// unset effort defaults to medium, which is rejected too. Only tools declared
/// in `tools` count: replaying earlier calls is history, not a declaration.
fn azure_gpt_56_chat_tools(ctx: &Ctx, request: &UnifiedRequest) -> Result<(), ProviderError> {
    if ctx.dialect != Dialect::OpenAiChatCompletions || !has_function_tools(request) {
        return Ok(());
    }
    match reasoning_effort(request) {
        Some(Effort::None) => Ok(()),
        Some(_) => Err(unsupported(
            AZURE_GPT_56_CHAT_TOOLS,
            "reasoning_effort",
            "function tools in this scope require effort none",
        )),
        None => Err(unsupported(
            AZURE_GPT_56_CHAT_TOOLS,
            "reasoning_effort",
            "effort is unset; the documented default for this combination is medium, not the required none",
        )),
    }
}

pub(super) fn adapt_request(
    id: &str,
    ctx: &Ctx,
    request: &mut UnifiedRequest,
) -> Result<(), ProviderError> {
    if ctx.upstream != Upstream::AzureOpenai {
        return Ok(());
    }
    if AZURE_GPT_56.contains(&id) {
        azure_gpt_56_chat_tools(ctx, request)?;
    }
    if id == "gpt-6-luna" {
        gpt_6_luna::adapt_azure_request(ctx, request)?;
    }
    Ok(())
}

// Hosted Azure restrictions are not promoted to model-wide facts.

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::models::{adapt_request as adapt, profile};
    use crate::provider::{UnifiedInputItem, UnifiedInputItemKind, UnifiedTool, UnifiedToolKind};

    fn ctx(model: &'static str, dialect: Dialect) -> Ctx<'static> {
        Ctx {
            model,
            upstream: Upstream::AzureOpenai,
            dialect,
        }
    }

    fn with_function_tool(effort: Option<&str>) -> UnifiedRequest {
        let mut request = UnifiedRequest::default();
        request.tools = Some(vec![UnifiedTool {
            cache_control: None,
            source_extensions: None,
            kind: UnifiedToolKind::Function {
                name: "read".into(),
                description: None,
                parameters: json!({"type": "object"}),
                strict: None,
                allowed_callers: None,
            },
        }]);
        request.options.reasoning_effort = effort.map(Into::into);
        request
    }

    fn with_replayed_call(effort: Option<&str>) -> UnifiedRequest {
        let mut request = UnifiedRequest::default();
        request.input = vec![UnifiedInputItem::new(UnifiedInputItemKind::FunctionCall {
            call_id: "call_1".into(),
            name: "read".into(),
            arguments: "{}".into(),
            caller: None,
            cache_control: None,
        })];
        request.options.reasoning_effort = effort.map(Into::into);
        request
    }

    #[test]
    fn azure_chat_tools_require_effort_none_and_name_the_rule() {
        let chat = ctx("gpt-5.6-sol", Dialect::OpenAiChatCompletions);
        assert!(adapt(&chat, &mut with_function_tool(Some("none"))).is_ok());
        for (effort, expected) in [(Some("high"), "require effort none"), (None, "unset")] {
            match adapt(&chat, &mut with_function_tool(effort)) {
                Err(ProviderError::Public { error, .. }) => {
                    assert!(error.message.contains(AZURE_GPT_56_CHAT_TOOLS));
                    assert!(error.message.contains(expected), "{}", error.message);
                    assert_eq!(error.param.as_deref(), Some("reasoning_effort"));
                }
                other => panic!("expected a public denial, got {other:?}"),
            }
        }
    }

    #[test]
    fn azure_tool_rule_is_limited_to_gpt_56_chat() {
        let responses = ctx("gpt-5.6-sol", Dialect::OpenAiResponses);
        assert!(adapt(&responses, &mut with_function_tool(Some("high"))).is_ok());
        for model in ["gpt-5.3-codex", "gpt-6.1-sol", "gpt-5.5"] {
            let chat = ctx(model, Dialect::OpenAiChatCompletions);
            assert!(adapt(&chat, &mut with_function_tool(Some("high"))).is_ok());
        }
        let no_tools = ctx("gpt-5.6-luna", Dialect::OpenAiChatCompletions);
        assert!(adapt(&no_tools, &mut UnifiedRequest::default()).is_ok());
        // The rule is Azure-specific.
        let vivgrid = Ctx {
            upstream: Upstream::Vivgrid,
            ..ctx("gpt-5.6-sol", Dialect::OpenAiChatCompletions)
        };
        assert!(adapt(&vivgrid, &mut with_function_tool(Some("high"))).is_ok());
    }

    #[test]
    fn azure_replayed_tool_calls_do_not_require_effort_none() {
        let chat = ctx("gpt-5.6-sol", Dialect::OpenAiChatCompletions);
        assert!(adapt(&chat, &mut with_replayed_call(Some("high"))).is_ok());
        assert!(adapt(&chat, &mut with_replayed_call(None)).is_ok());
        // Declaring the same tool still triggers the rule.
        match adapt(&chat, &mut with_function_tool(Some("high"))) {
            Err(ProviderError::Public { error, .. }) => {
                assert!(error.message.contains(AZURE_GPT_56_CHAT_TOOLS));
                assert_eq!(error.param.as_deref(), Some("reasoning_effort"));
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
    }

    #[test]
    fn azure_dialects_follow_model_routes() {
        for model in AZURE_CHAT_AND_RESPONSES {
            let found = profile(model, Upstream::AzureOpenai).expect("configured route");
            assert!(found.dialects.contains(&Dialect::OpenAiResponses));
            assert!(found.dialects.contains(&Dialect::OpenAiChatCompletions));
            assert!(found.requires_explicit_dialect);
        }
        let older = profile("gpt-5.5", Upstream::AzureOpenai).expect("older chat model");
        assert_eq!(older.dialects, &[Dialect::OpenAiChatCompletions]);
        assert!(!older.requires_explicit_dialect);
        for model in AZURE_RESPONSES_ONLY {
            let codex = profile(model, Upstream::AzureOpenai).expect("codex");
            assert_eq!(codex.default_dialect, Dialect::OpenAiResponses);
            assert_eq!(codex.dialects, &[Dialect::OpenAiResponses]);
            assert_eq!(codex.effort_field, EffortField::Reasoning);
            assert_eq!(codex.max_tokens_field, None);
        }
    }

    #[test]
    fn gpt_models_are_only_served_by_known_upstreams() {
        assert_eq!(profile("gpt-5.5", Upstream::Tokenhub), None);
        assert_eq!(profile("gpt-5.5", Upstream::AwsBedrock), None);
        assert_eq!(profile("gpt-unreleased", Upstream::AzureOpenai), None);
        assert_eq!(profile("GPT-5.5", Upstream::Vivgrid), Some(Profile::chat()));
    }
}
