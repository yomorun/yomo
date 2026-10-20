//! Claude family: Bedrock Converse profiles and per-model request rules.

use crate::models::{Ctx, Profile, generic_profile, mapping_pending, unsupported};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::{Dialect, EffortField, Upstream};

mod claude_sonnet_4_6;
mod claude_sonnet_5_5;

mod claude_haiku_5_5;

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

/// Bedrock Converse only. Messages fields are not Converse fields, so the
/// effort location stays unmapped until a model's Converse fields are probed.
fn bedrock_profile() -> Profile {
    Profile {
        default_dialect: Dialect::BedrockConverse,
        dialects: &[Dialect::BedrockConverse],
        requires_explicit_dialect: false,
        max_tokens_field: None,
        effort_field: EffortField::Unknown,
    }
}

pub(super) fn profile(id: &str, upstream: Upstream) -> Option<Profile> {
    if !MODELS.contains(&id) {
        return None;
    }
    match (id, upstream) {
        ("claude-haiku-5-5", Upstream::AwsBedrock) => Some(claude_haiku_5_5::bedrock_profile()),
        ("claude-sonnet-5-5", Upstream::AwsBedrock) => Some(claude_sonnet_5_5::bedrock_profile()),
        ("claude-sonnet-4-6", Upstream::AwsBedrock) => Some(claude_sonnet_4_6::bedrock_profile()),
        (_, Upstream::AwsBedrock) => Some(bedrock_profile()),
        (_, other) => generic_profile(other),
    }
}

/// Haiku 5.5, Sonnet 5.5 and Sonnet 4.6 are the only Converse mappings that
/// have been probed. Other Claude models support thinking too, but the Bedrock
/// dialect mapper does not encode it for them yet and rejects both controls.
/// Fail early with the same outcome instead of after a stream has started.
pub(super) fn adapt_request(
    id: &str,
    ctx: &Ctx,
    request: &mut UnifiedRequest,
) -> Result<(), ProviderError> {
    if ctx.upstream != Upstream::AwsBedrock {
        return Ok(());
    }
    match id {
        "claude-haiku-5-5" => claude_haiku_5_5::adapt_bedrock_request(ctx, request),
        "claude-sonnet-5-5" => claude_sonnet_5_5::adapt_bedrock_request(ctx, request),
        "claude-sonnet-4-6" => claude_sonnet_4_6::adapt_bedrock_request(ctx, request),
        _ if request.options.thinking.is_some() => Err(mapping_pending(ctx, "thinking")),
        _ => Ok(()),
    }
}

/// Reject thinking keys that only the client understands and Bedrock Converse
/// cannot represent (probed 2026-10-19: `block_binding` returns "Extra inputs
/// are not permitted"; `display` only accepts summarized/omitted).
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
            "bedrock-claude-client-only-thinking-keys",
            "thinking.block_binding",
            "Bedrock Converse rejects block_binding inputs",
        ));
    }
    if let Some(display) = thinking.get("display").and_then(serde_json::Value::as_str) {
        if display != "summarized" && display != "omitted" {
            return Err(unsupported(
                "bedrock-claude-client-only-thinking-keys",
                "thinking.display",
                "Bedrock only accepts 'summarized' or 'omitted'",
            ));
        }
    }
    Ok(())
}

/// Reject modified sampling alongside thinking (probed 2026-10-19 on both
/// Sonnet models: `temperature 0.7` with adaptive/enabled/between_tools
/// thinking returns 400; the upstream error says temperature may only be 1
/// when thinking is enabled or adaptive, and is deprecated under
/// between_tools). Temperature 1 is accepted per the upstream error text.
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
            "bedrock-claude-thinking-requires-temperature-1",
            "temperature",
            "temperature may only be 1 while thinking is enabled or adaptive",
        ));
    }
    Ok(())
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
    fn bedrock_uses_converse_with_unknown_effort_location_by_default() {
        let found = profile("claude-opus-5", Upstream::AwsBedrock).expect("opus");
        assert_eq!(found.default_dialect, Dialect::BedrockConverse);
        assert_eq!(found.dialects, &[Dialect::BedrockConverse]);
        assert_eq!(found.effort_field, EffortField::Unknown);
    }

    #[test]
    fn bedrock_thinking_is_a_pending_mapping_for_models_without_a_probe() {
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("high".into());
        assert!(adapt(&bedrock("claude-opus-5"), &mut request).is_ok());
        request.options.thinking = Some(json!({"type": "adaptive"}));
        match adapt(&bedrock("claude-opus-5"), &mut request) {
            Err(ProviderError::Public { error, .. }) => {
                assert!(
                    error.message.contains("not yet mapped"),
                    "{}",
                    error.message
                );
            }
            other => panic!("expected a public error, got {other:?}"),
        }
        // Nothing is rewritten on failure.
        assert_eq!(request.options.reasoning_effort.as_deref(), Some("high"));
        let vivgrid = Ctx {
            upstream: Upstream::Vivgrid,
            ..bedrock("claude-opus-5")
        };
        assert!(adapt(&vivgrid, &mut request).is_ok());
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
