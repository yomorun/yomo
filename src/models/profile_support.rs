use std::collections::HashMap;

use serde_json::{Value, json};

use crate::dialects::Dialect;
use crate::dialects::openai_chat_completions::mapper::request_to_chat;
use crate::dialects::openai_responses::mapper::ir_to_responses_request;
use crate::models::{ModelProfileError, from_dialect_conversion_error};
use crate::openai_responses_types::{ResponsesReasoningConfig, ResponsesRequest};
use crate::openai_types::ChatCompletionRequest;
use crate::provider::{ReasoningEffort, ReasoningEffortLevel, UnifiedRequest};
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

/// Convert to Chat and apply a model-specific `reasoning_effort` mapping.
pub(super) fn to_chat_request_with_mapped_effort<F>(
    request: &UnifiedRequest,
    map: F,
) -> Result<ChatCompletionRequest, ModelProfileError>
where
    F: Fn(&str) -> Option<&'static str>,
{
    to_chat_request_with_effort(request, mapped_effort(request, map))
}

/// Convert to Chat and map the effort onto the model's supported levels.
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

/// Resolve the effort a request should send after applying `map`.
///
/// Returns `None` when the request has no effort, otherwise the mapped value or
/// the original value when `map` leaves it unchanged, without copying the request.
pub(super) fn mapped_effort<F>(request: &UnifiedRequest, map: F) -> Option<ReasoningEffort>
where
    F: Fn(&str) -> Option<&'static str>,
{
    let effort = request.options.reasoning_effort.as_ref()?;
    Some(match effort.as_str() {
        Some(value) => map(value)
            .map(ReasoningEffort::from)
            .unwrap_or_else(|| effort.clone()),
        None => effort.clone(),
    })
}

/// Resolve the effort a request should send after mapping onto `supported` levels.
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
    nearest_effort(*requested, supported).map(|nearest| ReasoningEffort::from(effort_name(nearest)))
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

#[cfg(test)]
mod tests {
    use crate::provider::{ReasoningEffort, ReasoningEffortLevel, UnifiedRequest};

    use super::{mapped_effort, supported_effort};

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
    fn mapped_effort_leaves_absent_and_unlisted_values_unchanged() {
        let request = UnifiedRequest::default();
        assert_eq!(mapped_effort(&request, |_| Some("high")), None);

        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("vendor".into());
        assert_eq!(
            mapped_effort(&request, |_| None).and_then(|effort| effort.as_str().map(str::to_owned)),
            Some("vendor".to_owned())
        );
        assert_eq!(
            mapped_effort(&request, |value| (value == "vendor").then_some("high"))
                .and_then(|effort| effort.as_str().map(str::to_owned)),
            Some("high".to_owned())
        );
    }
}
