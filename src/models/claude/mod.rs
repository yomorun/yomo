//! Claude family: Bedrock Converse profiles, model-level request rules, and
//! Bedrock-only client-key checks.

use crate::models::compat::{require_effort_values, thinking_budget, thinking_mode};
use crate::models::{Ctx, Profile, ReasoningEffort, ThinkingMode, generic_profile, unsupported};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::{Dialect, EffortField, Upstream};

mod claude_fable_5;
mod claude_fable_5_1;
mod claude_haiku_4_5;
mod claude_haiku_5_5;
mod claude_opus_4_6;
mod claude_opus_4_7;
mod claude_opus_4_8;
mod claude_opus_5;
mod claude_opus_5_5;
mod claude_sonnet_4_6;
mod claude_sonnet_5;
mod claude_sonnet_5_5;

const MODELS: &[&str] = &[
    "claude-sonnet-4-6",
    "claude-sonnet-5",
    "claude-sonnet-5-5",
    "claude-haiku-4-5",
    "claude-haiku-5-5",
    "claude-opus-4-6",
    "claude-opus-4-7",
    "claude-opus-4-8",
    "claude-opus-5",
    "claude-opus-5-5",
    "claude-fable-5",
    "claude-fable-5-1",
];

/// Bedrock Converse only. Messages fields are not Converse fields; reasoning
/// effort is carried in `output_config.effort`, so a model without its own
/// file can pass the control through.
fn bedrock_profile() -> Profile {
    Profile {
        default_dialect: Dialect::BedrockConverse,
        dialects: &[Dialect::BedrockConverse],
        requires_explicit_dialect: false,
        max_tokens_field: None,
        effort_field: EffortField::OutputConfig,
    }
}

pub(super) fn profile(id: &str, upstream: Upstream) -> Option<Profile> {
    if !MODELS.contains(&id) {
        return None;
    }
    match (id, upstream) {
        ("claude-haiku-4-5", Upstream::AwsBedrock) => Some(claude_haiku_4_5::bedrock_profile()),
        ("claude-haiku-5-5", Upstream::AwsBedrock) => Some(claude_haiku_5_5::bedrock_profile()),
        ("claude-sonnet-4-6", Upstream::AwsBedrock) => Some(claude_sonnet_4_6::bedrock_profile()),
        ("claude-sonnet-5", Upstream::AwsBedrock) => Some(claude_sonnet_5::bedrock_profile()),
        ("claude-sonnet-5-5", Upstream::AwsBedrock) => Some(claude_sonnet_5_5::bedrock_profile()),
        ("claude-opus-4-6", Upstream::AwsBedrock) => Some(claude_opus_4_6::bedrock_profile()),
        ("claude-opus-4-7", Upstream::AwsBedrock) => Some(claude_opus_4_7::bedrock_profile()),
        ("claude-opus-4-8", Upstream::AwsBedrock) => Some(claude_opus_4_8::bedrock_profile()),
        ("claude-opus-5", Upstream::AwsBedrock) => Some(claude_opus_5::bedrock_profile()),
        ("claude-opus-5-5", Upstream::AwsBedrock) => Some(claude_opus_5_5::bedrock_profile()),
        ("claude-fable-5", Upstream::AwsBedrock) => Some(claude_fable_5::bedrock_profile()),
        ("claude-fable-5-1", Upstream::AwsBedrock) => Some(claude_fable_5_1::bedrock_profile()),
        (_, Upstream::AwsBedrock) => Some(bedrock_profile()),
        (_, other) => generic_profile(other),
    }
}

/// Each configured model has its own model-level rules. A Claude model without
/// a dedicated file is passed through untouched: the mapper forwards its
/// reasoning controls and the upstream decides what it supports.
///
/// Bedrock Converse additionally rejects client-only thinking keys. Those
/// checks stay platform-specific and run after the model-level rules.
pub(super) fn adapt_request(
    id: &str,
    ctx: &Ctx,
    request: &mut UnifiedRequest,
) -> Result<(), ProviderError> {
    match id {
        "claude-haiku-4-5" => claude_haiku_4_5::adapt_request(ctx, request)?,
        "claude-haiku-5-5" => claude_haiku_5_5::adapt_request(ctx, request)?,
        "claude-sonnet-4-6" => claude_sonnet_4_6::adapt_request(ctx, request)?,
        "claude-sonnet-5" => claude_sonnet_5::adapt_request(ctx, request)?,
        "claude-sonnet-5-5" => claude_sonnet_5_5::adapt_request(ctx, request)?,
        "claude-opus-4-6" => claude_opus_4_6::adapt_request(ctx, request)?,
        "claude-opus-4-7" => claude_opus_4_7::adapt_request(ctx, request)?,
        "claude-opus-4-8" => claude_opus_4_8::adapt_request(ctx, request)?,
        "claude-opus-5" => claude_opus_5::adapt_request(ctx, request)?,
        "claude-opus-5-5" => claude_opus_5_5::adapt_request(ctx, request)?,
        "claude-fable-5" => claude_fable_5::adapt_request(ctx, request)?,
        "claude-fable-5-1" => claude_fable_5_1::adapt_request(ctx, request)?,
        _ => return Ok(()),
    }
    if ctx.upstream != Upstream::AwsBedrock {
        return Ok(());
    }
    match id {
        "claude-haiku-4-5" => claude_haiku_4_5::adapt_bedrock_request(ctx, request),
        "claude-haiku-5-5" => claude_haiku_5_5::adapt_bedrock_request(ctx, request),
        "claude-sonnet-4-6" => claude_sonnet_4_6::adapt_bedrock_request(ctx, request),
        "claude-sonnet-5" => claude_sonnet_5::adapt_bedrock_request(ctx, request),
        "claude-sonnet-5-5" => claude_sonnet_5_5::adapt_bedrock_request(ctx, request),
        "claude-opus-4-6" => claude_opus_4_6::adapt_bedrock_request(ctx, request),
        "claude-opus-4-7" => claude_opus_4_7::adapt_bedrock_request(ctx, request),
        "claude-opus-4-8" => claude_opus_4_8::adapt_bedrock_request(ctx, request),
        "claude-opus-5" => claude_opus_5::adapt_bedrock_request(ctx, request),
        "claude-opus-5-5" => claude_opus_5_5::adapt_bedrock_request(ctx, request),
        "claude-fable-5" => claude_fable_5::adapt_bedrock_request(ctx, request),
        "claude-fable-5-1" => claude_fable_5_1::adapt_bedrock_request(ctx, request),
        _ => Ok(()),
    }
}

/// Reject thinking keys that only the client understands and Bedrock Converse
/// cannot represent: `block_binding` is not accepted and `display` only
/// accepts summarized/omitted.
pub(super) fn reject_client_only_thinking_keys(
    request: &UnifiedRequest,
) -> Result<(), ProviderError> {
    let Some(thinking) = request
        .options
        .thinking
        .as_ref()
        .and_then(|value| value.as_object())
    else {
        return Ok(());
    };
    if thinking.contains_key("block_binding") {
        return Err(unsupported(
            "claude-client-only-thinking-keys",
            "thinking.block_binding",
            "block_binding inputs are not supported",
        ));
    }
    if let Some(display) = thinking.get("display").and_then(serde_json::Value::as_str)
        && display != "summarized"
        && display != "omitted"
    {
        return Err(unsupported(
            "claude-client-only-thinking-keys",
            "thinking.display",
            "only 'summarized' or 'omitted' are supported",
        ));
    }
    Ok(())
}

/// Reject a temperature other than 1 whenever thinking is requested. Claude
/// models do not accept modified sampling while thinking is configured.
pub(super) fn reject_modified_sampling_with_thinking(
    request: &UnifiedRequest,
) -> Result<(), ProviderError> {
    if request.options.thinking.is_some()
        && request
            .options
            .temperature
            .is_some_and(|value| value != 1.0)
    {
        return Err(unsupported(
            "claude-thinking-requires-temperature-1",
            "temperature",
            "temperature may only be 1 while thinking is enabled or adaptive",
        ));
    }
    Ok(())
}

/// Reject sampling changes only while thinking actually runs. A `disabled`
/// block does not reason, so it keeps its temperature.
pub(super) fn reject_modified_sampling_with_active_thinking(
    request: &UnifiedRequest,
) -> Result<(), ProviderError> {
    let active = matches!(
        thinking_mode(request),
        Some(ThinkingMode::Enabled | ThinkingMode::Adaptive | ThinkingMode::BetweenTools)
    );
    if active
        && request
            .options
            .temperature
            .is_some_and(|value| value != 1.0)
    {
        return Err(unsupported(
            "claude-thinking-requires-temperature-1",
            "temperature",
            "temperature may only be 1 while thinking is enabled or adaptive",
        ));
    }
    Ok(())
}

/// Validate the thinking block of one Claude model against its mode set.
///
/// A model that accepts `enabled` uses budget-style thinking, with a 1024 floor
/// and a bound below `max_output_tokens`.
pub(super) fn require_thinking(
    rule: &str,
    request: &UnifiedRequest,
    modes: &[ThinkingMode],
) -> Result<(), ProviderError> {
    if request.options.thinking.is_none() {
        return Ok(());
    }
    let mode = thinking_mode(request)
        .ok_or_else(|| unsupported(rule, "thinking.type", "unknown thinking type"))?;
    if !modes.contains(&mode) {
        return Err(unsupported(
            rule,
            "thinking",
            &format!(
                "thinking type {} is not supported for this model",
                thinking_name(mode)
            ),
        ));
    }
    if mode == ThinkingMode::Enabled {
        let budget = thinking_budget(request).ok_or_else(|| {
            unsupported(
                rule,
                "thinking.budget_tokens",
                "budget_tokens is required for enabled thinking",
            )
        })?;
        if budget < 1024 {
            return Err(unsupported(
                rule,
                "thinking.budget_tokens",
                "the budget floor is 1024",
            ));
        }
        if let Some(max) = request
            .options
            .max_output_tokens
            .filter(|max| budget >= *max as u64)
        {
            return Err(unsupported(
                rule,
                "thinking.budget_tokens",
                &format!("the budget must stay below max_output_tokens ({max})"),
            ));
        }
    } else if thinking_budget(request).is_some() {
        return Err(unsupported(
            rule,
            "thinking.budget_tokens",
            "no thinking budget is accepted",
        ));
    }
    Ok(())
}

/// Reject effort values outside the set accepted by one Claude model.
pub(super) fn require_effort(
    rule: &str,
    request: &UnifiedRequest,
    efforts: &[ReasoningEffort],
) -> Result<(), ProviderError> {
    require_effort_values(rule, request, efforts)
}

fn thinking_name(mode: ThinkingMode) -> &'static str {
    match mode {
        ThinkingMode::Disabled => "disabled",
        ThinkingMode::Enabled => "enabled",
        ThinkingMode::Adaptive => "adaptive",
        ThinkingMode::BetweenTools => "between_tools",
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::models::{adapt_request as adapt, profile};

    fn bedrock(model: &'static str) -> Ctx<'static> {
        Ctx {
            model,
            upstream: Upstream::AwsBedrock,
            dialect: Dialect::BedrockConverse,
        }
    }

    #[test]
    fn bedrock_models_report_their_effort_location() {
        let found = profile("claude-opus-5", Upstream::AwsBedrock).expect("opus");
        assert_eq!(found.default_dialect, Dialect::BedrockConverse);
        assert_eq!(found.dialects, &[Dialect::BedrockConverse]);
        assert_eq!(found.effort_field, EffortField::OutputConfig);
        // Haiku 4.5 has no effort parameter at all.
        let haiku = profile("claude-haiku-4-5", Upstream::AwsBedrock).expect("haiku");
        assert_eq!(haiku.effort_field, EffortField::Unknown);
    }

    #[test]
    fn client_only_thinking_rejection_is_not_platform_specific() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "adaptive", "block_binding": {}}));

        match reject_client_only_thinking_keys(&request) {
            Err(ProviderError::Public { error, .. }) => {
                assert!(error.message.contains("claude-client-only-thinking-keys"));
                assert!(!error.message.contains("Bedrock"));
                assert!(!error.message.contains("Converse"));
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
    }

    #[test]
    fn bedrock_models_without_rules_pass_controls_through() {
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("high".into());
        request.options.thinking = Some(json!({"type": "adaptive"}));
        request.options.temperature = Some(0.7);
        assert!(adapt(&bedrock("claude-unreleased"), &mut request).is_ok());
        // Nothing is rewritten or rejected on the model layer.
        assert_eq!(request.options.reasoning_effort.as_deref(), Some("high"));
        assert_eq!(request.options.thinking, Some(json!({"type": "adaptive"})));
        assert_eq!(request.options.temperature, Some(0.7));
    }

    #[test]
    fn model_rules_apply_outside_bedrock() {
        let chat = |model: &'static str| Ctx {
            model,
            upstream: Upstream::Vllm,
            dialect: Dialect::OpenAiChatCompletions,
        };
        for (model, thinking, effort) in [
            ("claude-sonnet-5", json!({"type": "enabled"}), None),
            ("claude-opus-5-5", json!({"type": "disabled"}), None),
            ("claude-haiku-4-5", json!({"type": "adaptive"}), None),
            (
                "claude-sonnet-4-6",
                json!({"type": "adaptive"}),
                Some("xhigh"),
            ),
            ("claude-sonnet-5-5", json!({"type": "disabled"}), None),
        ] {
            let mut request = UnifiedRequest::default();
            request.options.thinking = Some(thinking);
            request.options.reasoning_effort = effort.map(Into::into);
            assert!(adapt(&chat(model), &mut request).is_err(), "{model}");
        }

        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "disabled"}));
        request.options.reasoning_effort = Some("high".into());
        assert!(adapt(&chat("claude-opus-4-6"), &mut request).is_ok());
    }

    #[test]
    fn claude_is_not_served_by_openai_style_vendors_except_generic_hosts() {
        assert_eq!(profile("claude-opus-5", Upstream::Tokenhub), None);
        assert_eq!(profile("claude-opus-5", Upstream::AzureOpenai), None);
        assert_eq!(
            profile("claude-opus-5", Upstream::Vivgrid),
            Some(Profile::chat())
        );
        assert_eq!(profile("claude-unreleased", Upstream::AwsBedrock), None);
    }
}
