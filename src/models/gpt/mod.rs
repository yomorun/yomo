//! GPT family: Azure dialect routes and per-model request rules.

use crate::models::{Ctx, Profile, generic_profile};
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

/// GPT-5.6 and later only speak OpenAI Responses, including Codex routes.
const RESPONSES_ONLY: &[&str] = &[
    "gpt-5.1-codex",
    "gpt-5.1-codex-max",
    "gpt-5.2-codex",
    "gpt-5.3-codex",
    "gpt-5.6-sol",
    "gpt-5.6-luna",
    "gpt-5.6-terra",
    "gpt-6-astra",
    "gpt-6-sol",
    "gpt-6-luna",
    "gpt-6.1-sol",
];

fn responses_profile() -> Profile {
    Profile {
        default_dialect: Dialect::OpenAiResponses,
        dialects: &[Dialect::OpenAiResponses],
        max_tokens_field: None,
        effort_field: EffortField::Reasoning,
        ..Profile::chat()
    }
}

pub(super) fn profile(id: &str, upstream: Upstream) -> Option<Profile> {
    if !MODELS.contains(&id) {
        return None;
    }
    if RESPONSES_ONLY.contains(&id) {
        return match upstream {
            Upstream::AzureOpenai | Upstream::Vivgrid => Some(responses_profile()),
            other => generic_profile(other),
        };
    }
    match upstream {
        Upstream::AzureOpenai => Some(Profile::chat()),
        other => generic_profile(other),
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
    if id == "gpt-6-luna" {
        gpt_6_luna::adapt_azure_request(ctx, request)?;
    }
    Ok(())
}

// Hosted Azure restrictions are not promoted to model-wide facts.

#[cfg(test)]
mod tests {
    use super::*;
    use crate::models::profile;

    #[test]
    fn gpt_56_and_later_are_responses_only() {
        for model in RESPONSES_ONLY {
            let azure = profile(model, Upstream::AzureOpenai).expect("configured Azure route");
            assert_eq!(azure.default_dialect, Dialect::OpenAiResponses);
            assert_eq!(azure.dialects, &[Dialect::OpenAiResponses]);
            assert!(!azure.requires_explicit_dialect);
            assert_eq!(azure.effort_field, EffortField::Reasoning);
            assert_eq!(azure.max_tokens_field, None);

            let vivgrid = profile(model, Upstream::Vivgrid).expect("Vivgrid route");
            assert_eq!(vivgrid.default_dialect, Dialect::OpenAiResponses);
            assert_eq!(vivgrid.dialects, &[Dialect::OpenAiResponses]);
        }
    }

    #[test]
    fn older_gpt_models_stay_chat() {
        for upstream in [Upstream::AzureOpenai, Upstream::Vivgrid] {
            let older = profile("gpt-5.5", upstream).expect("older chat model");
            assert_eq!(older.default_dialect, Dialect::OpenAiChatCompletions);
            assert_eq!(older.dialects, &[Dialect::OpenAiChatCompletions]);
            assert!(!older.requires_explicit_dialect);
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
