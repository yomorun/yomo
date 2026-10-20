//! Model profiles and per-model request adaptation.
//!
//! There is no model-family type and no capability abstraction. Every model
//! owns its quirks in its own file: [`profile`] answers static questions
//! (which dialects, which field names) and [`adapt_request`] validates or
//! rewrites a request right before it is sent. Families are only directories.

use axum::http::StatusCode;

use crate::openai_types::ErrorDetail;
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::{Dialect, EffortField, MaxTokensField, Upstream};

mod compat;

pub mod claude;
pub mod deepseek;
pub mod gemini;
pub mod glm;
pub mod gpt;
pub mod kimi;
pub mod minimax;

/// Reasoning-effort value exposed by model APIs.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ReasoningEffort {
    /// No additional reasoning effort.
    None,
    /// Minimal reasoning effort.
    Minimal,
    /// Low reasoning effort.
    Low,
    /// Medium reasoning effort.
    Medium,
    /// High reasoning effort.
    High,
    /// Extra-high reasoning effort.
    XHigh,
    /// Maximum reasoning effort.
    Max,
}

impl ReasoningEffort {
    /// Parse an exact native value without normalization or nearest-value mapping.
    pub fn parse(value: &str) -> Option<Self> {
        match value {
            "none" => Some(Self::None),
            "minimal" => Some(Self::Minimal),
            "low" => Some(Self::Low),
            "medium" => Some(Self::Medium),
            "high" => Some(Self::High),
            "xhigh" => Some(Self::XHigh),
            "max" => Some(Self::Max),
            _ => None,
        }
    }
}

/// Reasoning or thinking execution mode.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ThinkingMode {
    /// Thinking is disabled.
    Disabled,
    /// Thinking is enabled explicitly.
    Enabled,
    /// The model chooses its thinking behavior adaptively.
    Adaptive,
    /// Claude-native mode: no thinking before responding, but the short
    /// updates the model writes between tool calls still come back as
    /// thinking blocks (probed on Claude Sonnet 5.5 via Bedrock).
    BetweenTools,
}

impl ThinkingMode {
    /// Parse only established modes; auto is not assumed to mean adaptive.
    pub fn parse(value: &str) -> Option<Self> {
        match value {
            "disabled" => Some(Self::Disabled),
            "enabled" => Some(Self::Enabled),
            "adaptive" => Some(Self::Adaptive),
            "between_tools" => Some(Self::BetweenTools),
            _ => None,
        }
    }
}

/// Static integration facts for one model on one upstream.
///
/// A profile is resolved once at startup and never inspects a request.
/// Request-dependent behavior belongs to [`adapt_request`].
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Profile {
    /// Dialect used when the configuration does not select one.
    pub default_dialect: Dialect,
    /// Dialects this model can be reached through on the upstream.
    pub dialects: &'static [Dialect],
    /// Whether configuration must explicitly select one of `dialects`.
    pub requires_explicit_dialect: bool,
    /// Chat output-budget field, if applicable.
    pub max_tokens_field: Option<MaxTokensField>,
    /// Location of the native reasoning-effort field.
    pub effort_field: EffortField,
}

impl Profile {
    /// OpenAI Chat Completions only, with the standard field names.
    pub const fn chat() -> Self {
        Self {
            default_dialect: Dialect::OpenAiChatCompletions,
            dialects: &[Dialect::OpenAiChatCompletions],
            requires_explicit_dialect: false,
            max_tokens_field: Some(MaxTokensField::MaxCompletionTokens),
            effort_field: EffortField::ReasoningEffort,
        }
    }

    /// Self-hosted vLLM Chat: `max_tokens` and `chat_template_kwargs` effort.
    pub const fn vllm_chat() -> Self {
        Self {
            max_tokens_field: Some(MaxTokensField::MaxTokens),
            effort_field: EffortField::ChatTemplate,
            ..Self::chat()
        }
    }
}

/// Read-only context of the call being adapted.
#[derive(Debug, Clone, Copy)]
pub struct Ctx<'a> {
    /// Canonical model identity.
    pub model: &'a str,
    /// Upstream the request is sent to.
    pub upstream: Upstream,
    /// Dialect selected for this model at startup.
    pub dialect: Dialect,
}

/// Resolve the static profile of `model` on `upstream`, or `None` if the
/// upstream does not serve the model.
pub fn profile(model: &str, upstream: Upstream) -> Option<Profile> {
    if upstream == Upstream::OpenaiCompatible {
        return Some(Profile::chat());
    }
    let id = model.to_ascii_lowercase();
    let family: fn(&str, Upstream) -> Option<Profile> = if id.starts_with("glm-") {
        glm::profile
    } else if id.starts_with("gpt-") {
        gpt::profile
    } else if id.starts_with("claude-") {
        claude::profile
    } else if id.starts_with("gemini-") {
        gemini::profile
    } else if id.starts_with("deepseek-") {
        deepseek::profile
    } else if id.starts_with("kimi-") {
        kimi::profile
    } else if id.starts_with("minimax-") {
        minimax::profile
    } else {
        return None;
    };
    family(&id, upstream)
}

/// Validate and rewrite a request immediately before it is sent upstream.
///
/// Each model decides for itself what to reject or rewrite. Rejections use the
/// public `unsupported_capability` error.
pub fn adapt_request(ctx: &Ctx, request: &mut UnifiedRequest) -> Result<(), ProviderError> {
    let id = ctx.model.to_ascii_lowercase();
    if id.starts_with("glm-") {
        glm::adapt_request(&id, ctx, request)
    } else if id.starts_with("gpt-") {
        gpt::adapt_request(&id, ctx, request)
    } else if id.starts_with("claude-") {
        claude::adapt_request(&id, ctx, request)
    } else if id.starts_with("gemini-") {
        gemini::adapt_request(&id, ctx, request)
    } else {
        Ok(())
    }
}

/// Build the public error for a request a model cannot honor.
///
/// `rule` is a stable kebab-case identifier cited in logs and tests; keep the
/// source of the fact (doc or probe) next to the rule in the model file.
pub fn unsupported(rule: &str, param: &str, reason: &str) -> ProviderError {
    ProviderError::Public {
        status: StatusCode::BAD_REQUEST,
        error: ErrorDetail {
            message: format!("{param} rejected by rule {rule}: {reason}"),
            r#type: "invalid_request_error".into(),
            code: Some("unsupported_capability".into()),
            param: Some(param.into()),
        },
    }
}

/// Profile for upstreams that host every registered model through plain Chat.
/// Upstreams with model-specific integrations are handled by the model files.
pub(crate) fn generic_profile(upstream: Upstream) -> Option<Profile> {
    match upstream {
        Upstream::Vivgrid => Some(Profile::chat()),
        Upstream::Vllm => Some(Profile::vllm_chat()),
        _ => None,
    }
}

/// Build the public error for a control that yomo cannot yet encode for this
/// model on the selected upstream.
///
/// This is an implementation gap in the dialect mapper, not a statement about
/// the model: the model may well support the control. Callers mirror the
/// mapper so the request fails before a stream starts, and must be removed
/// once the mapping lands.
pub(crate) fn mapping_pending(ctx: &Ctx, param: &str) -> ProviderError {
    ProviderError::Public {
        status: StatusCode::BAD_REQUEST,
        error: ErrorDetail {
            message: format!(
                "{param} is not yet mapped for {} on {:?}",
                ctx.model, ctx.upstream
            ),
            r#type: "invalid_request_error".into(),
            code: Some("unsupported_capability".into()),
            param: Some(param.into()),
        },
    }
}

#[cfg(test)]
mod tests {
    use super::{Profile, ReasoningEffort, ThinkingMode, Upstream, profile};

    #[test]
    fn native_values_are_not_normalized_to_another_effort_or_thinking_mode() {
        assert_eq!(ReasoningEffort::parse("max"), Some(ReasoningEffort::Max));
        assert_eq!(ReasoningEffort::parse("MAX"), None);
        assert_eq!(ReasoningEffort::parse("turbo"), None);
        assert_eq!(ThinkingMode::parse("auto"), None);
        assert_eq!(
            ThinkingMode::parse("adaptive"),
            Some(ThinkingMode::Adaptive)
        );
    }

    #[test]
    fn openai_compatible_hosts_unknown_models_with_plain_chat() {
        assert_eq!(
            profile("private-custom-model", Upstream::OpenaiCompatible),
            Some(Profile::chat())
        );
        assert_eq!(profile("private-custom-model", Upstream::Tokenhub), None);
    }

    #[test]
    fn chat_vendors_serve_only_their_registered_families() {
        for (model, upstream, served) in [
            ("deepseek-v4-pro", Upstream::Tokenhub, true),
            ("deepseek-v4-pro", Upstream::VolcengineArk, true),
            ("glm-5.3", Upstream::VolcengineArk, true),
            ("kimi-k3", Upstream::Tokenhub, true),
            ("kimi-k3", Upstream::VolcengineArk, false),
            ("minimax-m3", Upstream::Tokenhub, true),
            ("minimax-m3", Upstream::VolcengineArk, false),
            ("minimax-m3", Upstream::Vllm, true),
            ("minimax-m3", Upstream::Custom, false),
            ("deepseek-v9", Upstream::Tokenhub, false),
        ] {
            assert_eq!(
                profile(model, upstream).is_some(),
                served,
                "{model} on {upstream:?}"
            );
        }
        assert_eq!(
            profile("kimi-k3", Upstream::Vllm),
            Some(Profile::vllm_chat())
        );
    }
}
