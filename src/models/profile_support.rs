//! Shared helpers for model-specific profiles.
//!
//! General normalization policy lives in `src/models/README.md`; each model's
//! supported set and constraints live in its own `src/models/<model>.rs` header.

use std::collections::HashMap;

use log::debug;
use serde_json::{Map, Value, json};

use crate::dialects::Dialect;
use crate::dialects::google_vertex::types::{
    VertexGenerationConfig, VertexThinkingConfig, VertexThinkingLevel,
};
use crate::dialects::openai_chat_completions::mapper::request_to_chat;
use crate::dialects::openai_responses::mapper::ir_to_responses_request;
use crate::models::{ModelProfile, ModelProfileError, from_dialect_conversion_error};
use crate::openai_responses_types::{ResponsesReasoningConfig, ResponsesRequest};
use crate::openai_types::ChatCompletionRequest;
use crate::provider::{
    ReasoningEffort, ReasoningEffortLevel, UnifiedContentPart, UnifiedContentPartKind,
    UnifiedInputItemKind, UnifiedRequest, UnifiedRole, UnifiedToolChoice, UnifiedToolOutput,
};
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

pub(super) fn is_chat_binding(binding: &ModelBinding, upstream: Upstream) -> bool {
    binding.upstream == upstream && binding.dialect == Some(Dialect::OpenAiChatCompletions)
}

pub(super) fn is_responses_binding(binding: &ModelBinding, upstream: Upstream) -> bool {
    binding.upstream == upstream && binding.dialect == Some(Dialect::OpenAiResponses)
}

pub(super) fn to_chat_request(
    request: &UnifiedRequest,
) -> Result<ChatCompletionRequest, ModelProfileError> {
    request_to_chat(request).map_err(from_dialect_conversion_error)
}

/// Convert to Chat and apply an already resolved effort override.
pub(super) fn to_chat_request_with_effort(
    request: &UnifiedRequest,
    effort: Option<ReasoningEffort>,
) -> Result<ChatCompletionRequest, ModelProfileError> {
    let mut chat_request = to_chat_request(request)?;
    if let Some(effort) = effort {
        chat_request.reasoning_effort = Some(effort);
    }
    Ok(chat_request)
}

/// Convert to Chat and normalize the effort onto the model's supported levels.
pub(super) fn to_chat_request_with_supported_effort(
    request: &UnifiedRequest,
    supported: &[ReasoningEffortLevel],
) -> Result<ChatCompletionRequest, ModelProfileError> {
    to_chat_request_with_effort(request, supported_effort(request, supported))
}

pub(super) fn to_responses_request(
    request: &UnifiedRequest,
    binding: &ModelBinding,
) -> Result<ResponsesRequest, ModelProfileError> {
    ir_to_responses_request(request, &binding.upstream_model).map_err(from_dialect_conversion_error)
}

/// Convert to Responses and map the effort onto the model's supported levels.
pub(super) fn to_responses_request_with_supported_effort(
    request: &UnifiedRequest,
    binding: &ModelBinding,
    supported: &[ReasoningEffortLevel],
) -> Result<ResponsesRequest, ModelProfileError> {
    let mut responses_request = to_responses_request(request, binding)?;
    if let Some(effort) = supported_effort(request, supported)
        && let Some(value) = effort.as_str()
    {
        responses_request
            .reasoning
            .get_or_insert_with(|| ResponsesReasoningConfig {
                effort: None,
                summary: None,
                extra: HashMap::new(),
            })
            .effort = Some(value.to_owned());
    }
    Ok(responses_request)
}

/// A profile whose only request behavior is effort normalization.
#[derive(Clone, Copy)]
pub(super) struct EffortProfile {
    endpoint: EffortEndpoint,
    upstream: Upstream,
    supported: &'static [ReasoningEffortLevel],
}

#[derive(Clone, Copy, PartialEq, Eq)]
enum EffortEndpoint {
    Chat,
    Responses,
}

impl EffortProfile {
    /// Profile for a model served through Chat Completions.
    pub(super) const fn chat(
        upstream: Upstream,
        supported: &'static [ReasoningEffortLevel],
    ) -> Self {
        Self {
            endpoint: EffortEndpoint::Chat,
            upstream,
            supported,
        }
    }

    /// Profile for a model served through the Responses API.
    pub(super) const fn responses(
        upstream: Upstream,
        supported: &'static [ReasoningEffortLevel],
    ) -> Self {
        Self {
            endpoint: EffortEndpoint::Responses,
            upstream,
            supported,
        }
    }
}

impl ModelProfile for EffortProfile {
    fn to_openai_chat_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        if self.endpoint != EffortEndpoint::Chat || !is_chat_binding(binding, self.upstream) {
            return Ok(None);
        }
        to_chat_request_with_supported_effort(request, self.supported).map(Some)
    }

    fn to_openai_responses_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ResponsesRequest>, ModelProfileError> {
        if self.endpoint != EffortEndpoint::Responses
            || !is_responses_binding(binding, self.upstream)
        {
            return Ok(None);
        }
        to_responses_request_with_supported_effort(request, binding, self.supported).map(Some)
    }
}

/// Resolve the effort a request should send for a model with `supported` levels.
///
/// The general policy is documented in `src/models/README.md`; the supported
/// set is declared by the model's profile module:
/// - supported named levels pass through unchanged;
/// - other named levels are rewritten to the nearest supported level, with ties
///   going to the higher level so normalization never silently weakens quality;
/// - unknown strings and numeric values pass through for the upstream to validate;
/// - omitted efforts stay omitted.
pub(super) fn supported_effort(
    request: &UnifiedRequest,
    supported: &[ReasoningEffortLevel],
) -> Option<ReasoningEffort> {
    let effort = request.options.reasoning_effort.as_ref()?;
    let ReasoningEffort::Named(requested) = effort else {
        return Some(effort.clone());
    };
    if supported.contains(requested) {
        return Some(ReasoningEffort::Named(*requested));
    }
    nearest_effort(*requested, supported).map(|nearest| {
        let forwarded = ReasoningEffort::from(effort_name(nearest));
        log_rewritten_effort(request, effort, &forwarded);
        forwarded
    })
}

/// Resolve the effort after applying a model's documented spellings.
///
/// `aliases` map provider-specific spellings onto a supported level before the
/// [`supported_effort`] policy is applied.
pub(super) fn aliased_supported_effort(
    request: &UnifiedRequest,
    supported: &[ReasoningEffortLevel],
    aliases: &[(&str, ReasoningEffortLevel)],
) -> Option<ReasoningEffort> {
    if let Some(value) = request
        .options
        .reasoning_effort
        .as_ref()
        .and_then(ReasoningEffort::as_str)
        && let Some((_, level)) = aliases.iter().find(|(alias, _)| *alias == value)
    {
        let forwarded = ReasoningEffort::Named(*level);
        if let Some(requested) = request.options.reasoning_effort.as_ref() {
            log_rewritten_effort(request, requested, &forwarded);
        }
        return Some(forwarded);
    }
    supported_effort(request, supported)
}

fn log_rewritten_effort(
    request: &UnifiedRequest,
    requested: &ReasoningEffort,
    forwarded: &ReasoningEffort,
) {
    if requested != forwarded {
        debug!(
            "reasoning_effort normalized for model `{}`: {requested:?} -> {forwarded:?}",
            request.model
        );
    }
}

fn nearest_effort(
    requested: ReasoningEffortLevel,
    supported: &[ReasoningEffortLevel],
) -> Option<ReasoningEffortLevel> {
    let mut nearest = None;
    for candidate in supported {
        let distance = (effort_rank(*candidate) - effort_rank(requested)).abs();
        match nearest {
            None => nearest = Some((distance, *candidate)),
            Some((best_distance, best_candidate))
                if distance < best_distance
                    || (distance == best_distance
                        && effort_rank(*candidate) > effort_rank(best_candidate)) =>
            {
                nearest = Some((distance, *candidate));
            }
            _ => {}
        }
    }
    nearest.map(|(_, candidate)| candidate)
}

fn effort_rank(effort: ReasoningEffortLevel) -> i8 {
    match effort {
        ReasoningEffortLevel::None => 0,
        ReasoningEffortLevel::Minimal => 1,
        ReasoningEffortLevel::Low => 2,
        ReasoningEffortLevel::Medium => 3,
        ReasoningEffortLevel::High => 4,
        ReasoningEffortLevel::XHigh => 5,
        ReasoningEffortLevel::Max => 6,
    }
}

fn effort_name(effort: ReasoningEffortLevel) -> &'static str {
    match effort {
        ReasoningEffortLevel::None => "none",
        ReasoningEffortLevel::Minimal => "minimal",
        ReasoningEffortLevel::Low => "low",
        ReasoningEffortLevel::Medium => "medium",
        ReasoningEffortLevel::High => "high",
        ReasoningEffortLevel::XHigh => "xhigh",
        ReasoningEffortLevel::Max => "max",
    }
}

pub(super) fn preserve_raw_thinking(
    request: &UnifiedRequest,
    chat_request: &mut ChatCompletionRequest,
) {
    if let Some(thinking) = &request.options.thinking {
        chat_request.thinking = None;
        chat_request
            .extra
            .insert("thinking".to_owned(), thinking.clone());
    }
}

pub(super) fn set_max_tokens_field(chat_request: &mut ChatCompletionRequest) {
    if let Some(max_tokens) = chat_request.max_completion_tokens.take() {
        chat_request
            .extra
            .insert("max_tokens".to_owned(), json!(max_tokens));
    }
}

pub(super) fn set_top_k_field(chat_request: &mut ChatCompletionRequest, top_k: Option<i64>) {
    if let Some(top_k) = top_k {
        chat_request.extra.insert("top_k".to_owned(), json!(top_k));
    }
}

pub(super) fn unsupported_capability(
    message: impl Into<String>,
    parameter: &str,
) -> ModelProfileError {
    ModelProfileError::UnsupportedCapability {
        message: message.into(),
        param: Some(parameter.to_owned()),
    }
}

pub(super) fn validate_thinking_modes(
    thinking: Option<&Value>,
    supported: &[&str],
    model_name: &str,
) -> Result<(), ModelProfileError> {
    let Some(thinking) = thinking else {
        return Ok(());
    };
    let Some(fields) = thinking.as_object() else {
        return Err(ModelProfileError::InvalidRequest {
            message: "thinking must be an object".to_owned(),
            param: Some("thinking".to_owned()),
        });
    };
    let Some(mode) = fields.get("type").and_then(Value::as_str) else {
        return Err(ModelProfileError::InvalidRequest {
            message: "thinking.type is required when thinking is specified".to_owned(),
            param: Some("thinking.type".to_owned()),
        });
    };
    if supported.contains(&mode) {
        return Ok(());
    }
    Err(unsupported_capability(
        format!("{model_name} does not support thinking.type `{mode}`"),
        "thinking.type",
    ))
}

/// Whether any request item carries a non-text content part.
pub(super) fn has_non_text_content(request: &UnifiedRequest) -> bool {
    let has_non_text_part = |parts: &[UnifiedContentPart]| {
        parts.iter().any(|part| {
            !matches!(
                &part.kind,
                UnifiedContentPartKind::Text { .. }
                    | UnifiedContentPartKind::Refusal { .. }
                    | UnifiedContentPartKind::Thinking { .. }
                    | UnifiedContentPartKind::RedactedThinking { .. }
            )
        })
    };

    request
        .instructions
        .iter()
        .flatten()
        .chain(&request.input)
        .any(|item| match &item.kind {
            UnifiedInputItemKind::Message { content, .. } => has_non_text_part(content),
            UnifiedInputItemKind::FunctionCallOutput {
                output: UnifiedToolOutput::Content(content),
                ..
            } => has_non_text_part(content),
            UnifiedInputItemKind::Reasoning {
                content: Some(content),
                ..
            } => has_non_text_part(content),
            UnifiedInputItemKind::Other { .. } => true,
            _ => false,
        })
}

/// Map a unified reasoning effort onto the Gemini level supported by 3.8/4.
pub(super) fn gemini_level_for_effort(
    effort: &ReasoningEffort,
) -> Result<VertexThinkingLevel, ModelProfileError> {
    match effort.as_str() {
        Some("none" | "minimal" | "low") => Ok(VertexThinkingLevel::Low),
        Some("medium") => Ok(VertexThinkingLevel::Medium),
        Some("high" | "xhigh" | "max") => Ok(VertexThinkingLevel::High),
        Some(other) => Err(ModelProfileError::InvalidRequest {
            message: format!("unsupported reasoning_effort {other} for Gemini"),
            param: Some("reasoning_effort".to_owned()),
        }),
        None => Err(ModelProfileError::InvalidRequest {
            message: "numeric reasoning_effort is not supported by Google Vertex".to_owned(),
            param: Some("reasoning_effort".to_owned()),
        }),
    }
}

/// Build a Gemini thinking config for a resolved level.
pub(super) fn gemini_thinking_config_for_level(
    level: VertexThinkingLevel,
    include_thoughts: Option<bool>,
) -> VertexThinkingConfig {
    VertexThinkingConfig {
        thinking_budget: None,
        thinking_level: Some(level),
        include_thoughts: include_thoughts.or(Some(true)),
    }
}

/// Translate a unified `thinking` object into a Gemini 3/4 thinking config.
pub(super) fn gemini_thinking_config(
    thinking: &Value,
    existing_include_thoughts: Option<bool>,
) -> Result<Option<VertexThinkingConfig>, ModelProfileError> {
    let Some(fields) = thinking.as_object() else {
        return Err(ModelProfileError::InvalidRequest {
            message: "thinking must be an object".to_owned(),
            param: Some("thinking".to_owned()),
        });
    };
    let type_name = fields.get("type").and_then(Value::as_str).ok_or_else(|| {
        ModelProfileError::InvalidRequest {
            message: "thinking.type is required".to_owned(),
            param: Some("thinking.type".to_owned()),
        }
    })?;
    let include_thoughts = fields
        .get("include_thoughts")
        .or_else(|| fields.get("includeThoughts"))
        .and_then(Value::as_bool)
        .or(existing_include_thoughts);

    let level = match type_name {
        "enabled" => match fields.get("budget_tokens") {
            Some(value) => {
                let budget = value
                    .as_i64()
                    .ok_or_else(|| ModelProfileError::InvalidRequest {
                        message: "thinking.budget_tokens must be an integer".to_owned(),
                        param: Some("thinking.budget_tokens".to_owned()),
                    })?;
                gemini_level_for_budget(budget)?
            }
            None => None,
        },
        // Gemini has no adaptive mode; leave the model default untouched.
        "adaptive" => None,
        // Gemini 3+ cannot turn thinking off; low is the closest supported level.
        "disabled" => Some(VertexThinkingLevel::Low),
        other => {
            return Err(ModelProfileError::InvalidRequest {
                message: format!("unsupported thinking.type {other} for Gemini"),
                param: Some("thinking.type".to_owned()),
            });
        }
    };

    if level.is_none() && include_thoughts.is_none() {
        return Ok(None);
    }
    Ok(Some(VertexThinkingConfig {
        thinking_budget: None,
        thinking_level: level,
        include_thoughts,
    }))
}

fn gemini_level_for_budget(budget: i64) -> Result<Option<VertexThinkingLevel>, ModelProfileError> {
    match budget {
        // Dynamic thinking is the model default; do not emit a level.
        -1 => Ok(None),
        // Gemini 3+ cannot disable thinking, so zero maps to the lowest level.
        0..=1024 => Ok(Some(VertexThinkingLevel::Low)),
        1025..=8192 => Ok(Some(VertexThinkingLevel::Medium)),
        budget if budget > 8192 => Ok(Some(VertexThinkingLevel::High)),
        other => Err(ModelProfileError::InvalidRequest {
            message: format!("thinking.budget_tokens {other} is not valid for Gemini 3 or later"),
            param: Some("thinking.budget_tokens".to_owned()),
        }),
    }
}

/// Clamp a Gemini output budget to the request-specific model cap.
pub(super) fn gemini_clamp_output_tokens(
    generation: &mut VertexGenerationConfig,
    cap: i32,
    model: &str,
) {
    if let Some(tokens) = generation.max_output_tokens {
        let clamped = tokens.min(cap);
        if clamped != tokens {
            debug!(
                "gemini profile: model={} max_output_tokens {} -> {}",
                model, tokens, clamped
            );
        }
        generation.max_output_tokens = Some(clamped);
    }
}

/// Whether the conversation ends with a prefilled assistant message.
pub(super) fn ends_with_assistant_prefill(request: &UnifiedRequest) -> bool {
    let Some(last) = request.input.last() else {
        return false;
    };
    let UnifiedInputItemKind::Message {
        role: UnifiedRole::Assistant,
        content,
        ..
    } = &last.kind
    else {
        return false;
    };
    content.iter().any(|part| match &part.kind {
        UnifiedContentPartKind::Text { text, .. } => !text.trim().is_empty(),
        _ => false,
    })
}

/// Thinking modes supported by a Claude Bedrock route.
#[derive(Clone, Copy, PartialEq, Eq)]
pub(super) enum ClaudeThinking {
    /// Budget-based extended thinking only (Haiku 4.5).
    ExtendedOnly,
    /// Extended thinking (deprecated) or adaptive thinking (Claude 4.6).
    AdaptiveExtended,
    /// Adaptive thinking only (Claude 4.7 and later).
    AdaptiveOnly,
}

/// Effort levels accepted by a Claude Bedrock route.
#[derive(Clone, Copy, PartialEq, Eq)]
pub(super) enum ClaudeEffort {
    /// The model does not support `output_config.effort` (Haiku 4.5).
    None,
    /// low / medium / high / max (Claude 4.6).
    UpToMax,
    /// low / medium / high / xhigh / max (Claude 4.7 and later).
    Full,
}

/// Model-specific Claude rules applied before Bedrock Converse encoding.
pub(super) struct ClaudeRules {
    /// Exact public model name used for registry selection and diagnostics.
    pub model: &'static str,
    /// Thinking modes the model accepts.
    pub thinking: ClaudeThinking,
    /// Effort levels the model accepts.
    pub effort: ClaudeEffort,
    /// Whether `thinking.type=disabled` is accepted.
    pub allow_disabled: bool,
    /// Whether disabled thinking is accepted only at effort high or below.
    pub disabled_effort_high_only: bool,
    /// Whether `thinking.type=between_tools` is accepted.
    pub allow_between_tools: bool,
    /// Whether `disabled` is represented as `between_tools` (Sonnet 5.5).
    pub between_tools_replaces_disabled: bool,
    /// Whether forced tool choice is accepted while thinking is active.
    pub allow_forced_tool_choice: bool,
    /// Whether assistant-message prefill is rejected.
    pub reject_prefill: bool,
    /// Whether temperature and top-p must be removed from the request.
    pub drop_sampling: bool,
}

/// Apply model-specific Claude rules to a unified request before encoding.
pub(super) fn apply_claude_rules(
    request: &mut UnifiedRequest,
    binding: &ModelBinding,
    rules: &ClaudeRules,
) -> Result<(), ModelProfileError> {
    if binding.upstream != Upstream::AwsBedrock
        || binding.dialect != Some(Dialect::BedrockConverse)
        || binding.public_model != rules.model
    {
        return Ok(());
    }

    if rules.reject_prefill && ends_with_assistant_prefill(request) {
        return Err(claude_unsupported(
            format!(
                "{} does not support assistant message prefill; the conversation must end with a user turn",
                rules.model
            ),
            "messages",
        ));
    }

    normalize_claude_effort(request, rules)?;
    normalize_claude_thinking(request, rules)?;
    normalize_claude_sampling(request, rules);
    normalize_claude_tool_choice(request, rules)?;
    Ok(())
}

fn normalize_claude_effort(
    request: &mut UnifiedRequest,
    rules: &ClaudeRules,
) -> Result<(), ModelProfileError> {
    let mapped: &'static str = {
        let Some(effort) = request.options.reasoning_effort.as_ref() else {
            return Ok(());
        };
        if rules.effort == ClaudeEffort::None {
            return Err(claude_unsupported(
                format!("{} does not support reasoning_effort", rules.model),
                "reasoning_effort",
            ));
        }
        let Some(effort) = effort.as_str() else {
            return Err(claude_invalid(
                "numeric reasoning_effort is not supported by Bedrock Converse",
                "reasoning_effort",
            ));
        };
        match effort {
            "none" | "minimal" | "low" => "low",
            "medium" => "medium",
            "high" => "high",
            "xhigh" if rules.effort == ClaudeEffort::Full => "xhigh",
            "xhigh" => "max",
            "max" => "max",
            other => {
                return Err(claude_invalid(
                    format!("unsupported reasoning_effort {other} for {}", rules.model),
                    "reasoning_effort",
                ));
            }
        }
    };

    debug!(
        "claude profile: model={} reasoning_effort -> {mapped}",
        rules.model
    );
    request.options.reasoning_effort = Some(mapped.into());
    Ok(())
}

fn normalize_claude_thinking(
    request: &mut UnifiedRequest,
    rules: &ClaudeRules,
) -> Result<(), ModelProfileError> {
    let Some(thinking) = request.options.thinking.clone() else {
        return Ok(());
    };
    let Some(fields) = thinking.as_object() else {
        return Err(claude_invalid("thinking must be an object", "thinking"));
    };
    if fields.get("block_binding").is_some() {
        return Err(claude_unsupported(
            format!("{} rejects thinking.block_binding", rules.model),
            "thinking.block_binding",
        ));
    }
    let Some(type_name) = fields.get("type").and_then(Value::as_str) else {
        return Err(claude_invalid("thinking.type is required", "thinking.type"));
    };
    let display = claude_display(fields)?;
    let mut normalized = json!({"type": type_name});

    match type_name {
        "enabled" => {
            if rules.thinking == ClaudeThinking::AdaptiveOnly {
                debug!(
                    "claude profile: model={} thinking.type enabled -> adaptive",
                    rules.model
                );
                normalized = json!({"type": "adaptive"});
            } else {
                let budget = claude_budget(fields, request.options.max_output_tokens)?;
                normalized["budget_tokens"] = json!(budget);
            }
        }
        "adaptive" => {
            if rules.thinking == ClaudeThinking::ExtendedOnly {
                return Err(claude_unsupported(
                    format!("{} does not support adaptive thinking", rules.model),
                    "thinking.type",
                ));
            }
            reject_claude_budget(fields, rules)?;
        }
        "disabled" => {
            if rules.between_tools_replaces_disabled {
                reject_claude_budget(fields, rules)?;
                normalized = json!({"type": "between_tools"});
            } else if !rules.allow_disabled {
                return Err(claude_unsupported(
                    format!("{} cannot disable thinking", rules.model),
                    "thinking.type",
                ));
            } else {
                reject_claude_budget(fields, rules)?;
                if rules.disabled_effort_high_only && claude_effort_is_high_or_above(request) {
                    return Err(claude_unsupported(
                        format!(
                            "{} accepts thinking.type=disabled only at effort high or below",
                            rules.model
                        ),
                        "thinking.type",
                    ));
                }
            }
        }
        "between_tools" => {
            if !rules.allow_between_tools {
                return Err(claude_unsupported(
                    format!(
                        "{} does not support thinking.type=between_tools",
                        rules.model
                    ),
                    "thinking.type",
                ));
            }
            reject_claude_budget(fields, rules)?;
        }
        other => {
            return Err(claude_invalid(
                format!("unsupported thinking.type {other} for {}", rules.model),
                "thinking.type",
            ));
        }
    }

    if matches!(type_name, "enabled" | "adaptive")
        && let Some(display) = display
    {
        normalized["display"] = json!(display);
    }
    request.options.thinking = Some(normalized);
    Ok(())
}

fn normalize_claude_sampling(request: &mut UnifiedRequest, rules: &ClaudeRules) {
    let thinking_active = request
        .options
        .thinking
        .as_ref()
        .and_then(|thinking| thinking.get("type"))
        .and_then(Value::as_str)
        .is_some_and(|kind| matches!(kind, "enabled" | "adaptive" | "between_tools"));

    if rules.drop_sampling {
        request.options.temperature = None;
        request.options.top_p = None;
        request.options.top_k = None;
    } else if thinking_active {
        request.options.temperature = Some(1.0);
        request.options.top_p = None;
        request.options.top_k = None;
    }
}

fn normalize_claude_tool_choice(
    request: &UnifiedRequest,
    rules: &ClaudeRules,
) -> Result<(), ModelProfileError> {
    if rules.allow_forced_tool_choice {
        return Ok(());
    }
    if matches!(
        request.tool_choice.as_ref(),
        Some(UnifiedToolChoice::Required | UnifiedToolChoice::Function(_))
    ) {
        return Err(claude_unsupported(
            format!("{} rejects forced tool_choice", rules.model),
            "tool_choice",
        ));
    }
    Ok(())
}

fn claude_display(fields: &Map<String, Value>) -> Result<Option<String>, ModelProfileError> {
    match fields.get("display").and_then(Value::as_str) {
        None => Ok(None),
        Some(display @ ("summarized" | "omitted")) => Ok(Some(display.to_owned())),
        Some("updates") => Ok(None),
        Some(other) => Err(claude_invalid(
            format!("thinking.display {other} is not accepted; only summarized/omitted"),
            "thinking.display",
        )),
    }
}

fn claude_budget(
    fields: &Map<String, Value>,
    max_output_tokens: Option<i64>,
) -> Result<u64, ModelProfileError> {
    let budget = fields
        .get("budget_tokens")
        .and_then(Value::as_u64)
        .ok_or_else(|| {
            claude_invalid(
                "thinking.budget_tokens is required for enabled thinking",
                "thinking.budget_tokens",
            )
        })?;
    if budget < 1024 {
        return Err(claude_invalid(
            "thinking.budget_tokens must be at least 1024",
            "thinking.budget_tokens",
        ));
    }
    if let Some(max) = max_output_tokens.filter(|max| budget >= *max as u64) {
        return Err(claude_invalid(
            format!("thinking.budget_tokens must be below max_tokens ({max})"),
            "thinking.budget_tokens",
        ));
    }
    Ok(budget)
}

fn reject_claude_budget(
    fields: &Map<String, Value>,
    rules: &ClaudeRules,
) -> Result<(), ModelProfileError> {
    if fields.get("budget_tokens").is_some() {
        return Err(claude_invalid(
            format!(
                "{} accepts no thinking budget for this thinking type",
                rules.model
            ),
            "thinking.budget_tokens",
        ));
    }
    Ok(())
}

fn claude_effort_is_high_or_above(request: &UnifiedRequest) -> bool {
    request
        .options
        .reasoning_effort
        .as_ref()
        .and_then(|effort| effort.as_str())
        .is_some_and(|effort| matches!(effort, "xhigh" | "max"))
}

fn claude_invalid(message: impl Into<String>, param: &str) -> ModelProfileError {
    ModelProfileError::InvalidRequest {
        message: message.into(),
        param: Some(param.to_owned()),
    }
}

fn claude_unsupported(message: impl Into<String>, param: &str) -> ModelProfileError {
    ModelProfileError::UnsupportedCapability {
        message: message.into(),
        param: Some(param.to_owned()),
    }
}

#[cfg(test)]
mod tests {
    use crate::provider::{ReasoningEffort, ReasoningEffortLevel, UnifiedRequest};

    use super::{aliased_supported_effort, supported_effort};

    #[test]
    fn supported_effort_maps_unsupported_names_to_nearest_and_breaks_ties_upward() {
        let supported = [
            ReasoningEffortLevel::None,
            ReasoningEffortLevel::Low,
            ReasoningEffortLevel::Medium,
            ReasoningEffortLevel::High,
            ReasoningEffortLevel::XHigh,
            ReasoningEffortLevel::Max,
        ];
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("minimal".into());

        let effort = supported_effort(&request, &supported).expect("effort present");
        assert_eq!(effort.as_str(), Some("low"));
    }

    #[test]
    fn preserves_numeric_and_unknown_effort_values() {
        let supported = [ReasoningEffortLevel::Low, ReasoningEffortLevel::High];
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("provider-specific".into());
        let effort = supported_effort(&request, &supported).expect("effort present");
        assert_eq!(effort.as_str(), Some("provider-specific"));

        request.options.reasoning_effort = Some(ReasoningEffort::Numeric(7.into()));
        assert_eq!(
            supported_effort(&request, &supported),
            Some(ReasoningEffort::Numeric(7.into()))
        );
    }

    #[test]
    fn aliases_resolve_before_nearest_effort_mapping() {
        let supported = [
            ReasoningEffortLevel::Low,
            ReasoningEffortLevel::High,
            ReasoningEffortLevel::Max,
        ];
        let aliases = [("ultra", ReasoningEffortLevel::Max)];
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("ultra".into());
        let effort =
            aliased_supported_effort(&request, &supported, &aliases).expect("effort present");
        assert_eq!(effort.as_str(), Some("max"));

        request.options.reasoning_effort = Some("minimal".into());
        let effort =
            aliased_supported_effort(&request, &supported, &aliases).expect("effort present");
        assert_eq!(effort.as_str(), Some("low"));
    }

    #[test]
    fn omitted_effort_stays_omitted() {
        let supported = [ReasoningEffortLevel::Low, ReasoningEffortLevel::High];
        assert_eq!(
            supported_effort(&UnifiedRequest::default(), &supported),
            None
        );
        assert_eq!(
            aliased_supported_effort(&UnifiedRequest::default(), &supported, &[]),
            None
        );
    }
}
