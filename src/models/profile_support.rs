use serde_json::{Value, json};

use crate::dialects::Dialect;
use crate::dialects::openai_chat_completions::mapper::request_to_chat;
use crate::dialects::openai_responses::mapper::ir_to_responses_request;
use crate::models::{ModelProfileError, from_dialect_conversion_error};
use crate::openai_responses_types::ResponsesRequest;
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

pub(super) fn to_responses_request(
    request: &UnifiedRequest,
    binding: &ModelBinding,
) -> Result<ResponsesRequest, ModelProfileError> {
    ir_to_responses_request(request, &binding.upstream_model).map_err(from_dialect_conversion_error)
}

pub(super) fn map_reasoning_effort<F>(request: &UnifiedRequest, map: F) -> UnifiedRequest
where
    F: Fn(&str) -> Option<&'static str>,
{
    let mut normalized = request.clone();
    if let Some(effort) = request.options.reasoning_effort.as_ref()
        && let Some(value) = effort.as_str()
        && let Some(mapped) = map(value)
    {
        normalized.options.reasoning_effort = Some(ReasoningEffort::from(mapped));
    }
    normalized
}

pub(super) fn map_effort_to_supported(
    request: &UnifiedRequest,
    supported: &[ReasoningEffortLevel],
) -> UnifiedRequest {
    let mut normalized = request.clone();
    let Some(ReasoningEffort::Named(requested)) = request.options.reasoning_effort.as_ref() else {
        return normalized;
    };
    if supported.contains(requested) {
        return normalized;
    }
    if let Some(nearest) = nearest_effort(*requested, supported) {
        normalized.options.reasoning_effort = Some(ReasoningEffort::from(effort_name(nearest)));
    }
    normalized
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

    use super::map_effort_to_supported;

    #[test]
    fn maps_unsupported_named_effort_to_nearest_and_breaks_ties_upward() {
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

        let normalized = map_effort_to_supported(&request, &supported);
        assert_eq!(
            normalized
                .options
                .reasoning_effort
                .as_ref()
                .and_then(ReasoningEffort::as_str),
            Some("low")
        );
    }

    #[test]
    fn preserves_numeric_and_unknown_effort_values() {
        let supported = [ReasoningEffortLevel::Low, ReasoningEffortLevel::High];
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("provider-specific".into());
        let normalized = map_effort_to_supported(&request, &supported);
        assert_eq!(
            normalized
                .options
                .reasoning_effort
                .as_ref()
                .and_then(ReasoningEffort::as_str),
            Some("provider-specific")
        );

        request.options.reasoning_effort = Some(ReasoningEffort::Numeric(7.into()));
        let normalized = map_effort_to_supported(&request, &supported);
        assert_eq!(
            normalized.options.reasoning_effort,
            Some(ReasoningEffort::Numeric(7.into()))
        );
    }
}
