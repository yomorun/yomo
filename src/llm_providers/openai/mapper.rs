use std::collections::HashMap;

use crate::openai_types::{
    ChatCompletionChunk, ChatCompletionChunkToolCall, ChatCompletionChunkToolCallFunction,
    Content as OpenAIContent, ContentPart, Usage as OpenAIUsage,
};
use crate::llm_providers::{
    FinishReason, ProviderError, ToolCall, UnifiedEvent, UnifiedResponse, Usage,
};

#[derive(Default)]
pub struct ToolCallState {
    name: String,
    description: String,
    arguments: String,
}

pub struct ToolCallDeltaEvent {
    pub name: String,
    pub description: String,
    pub arguments_delta: String,
    pub index: i32,
}

pub fn map_response(
    response: crate::openai_types::ChatCompletionResponse,
) -> Result<UnifiedResponse, ProviderError> {
    let choice = response
        .choices
        .into_iter()
        .next()
        .ok_or_else(|| ProviderError::Internal("missing choices".to_string()))?;
    let finish_reason = map_finish_reason_to_provider(choice.finish_reason.as_deref());
    let output_text = extract_text(choice.message.content);
    let tool_calls = choice.message.tool_calls.map(|calls| {
        calls
            .into_iter()
            .map(map_tool_call_from_openai)
            .collect::<Vec<_>>()
    });
    let usage = response
        .usage
        .as_ref()
        .map(map_usage_to_provider)
        .unwrap_or(Usage {
            input_tokens: 0,
            output_tokens: 0,
            total_tokens: 0,
            cached_tokens: None,
            reasoning_tokens: None,
        });

    Ok(UnifiedResponse {
        request_id: response.id,
        model: response.model,
        output_text,
        tool_calls,
        finish_reason,
        usage,
    })
}

pub fn map_stream_chunk(
    chunk: ChatCompletionChunk,
    state: &mut StreamMapState,
) -> Vec<UnifiedEvent> {
    let mut events = Vec::new();
    let (chunk_request_id, chunk_model, chunk_created) = chunk_metadata(&chunk);
    if state.request_id.is_empty() {
        state.request_id = chunk_request_id;
        state.model = chunk_model;
        state.created_at = chunk_created;
    }

    if !state.started {
        state.started = true;
        events.push(UnifiedEvent::ResponseCreated {
            id: state.request_id.clone(),
            model: state.model.clone(),
            created_at: state.created_at.clone(),
        });
        events.push(UnifiedEvent::ResponseInProgress {
            id: state.request_id.clone(),
            model: state.model.clone(),
            created_at: state.created_at.clone(),
        });
        events.push(UnifiedEvent::MessageStart {
            id: state.request_id.clone(),
            role: "assistant".to_string(),
        });
    }

    if let Some(delta) = chunk.choices.first().map(|choice| &choice.delta) {
        if let Some(content) = &delta.content {
            events.push(UnifiedEvent::MessageDelta {
                id: state.request_id.clone(),
                delta: content.clone(),
            });
        }
        if let Some(tool_calls) = &delta.tool_calls {
            for call in tool_calls {
                if let Some(delta_event) = map_tool_call_delta(call, &mut state.tool_call_state) {
                    let call_id =
                        ensure_tool_call_id(&state.request_id, delta_event.index as usize);
                    events.push(UnifiedEvent::ToolCallDelta {
                        id: call_id,
                        name: delta_event.name,
                        arguments_delta: delta_event.arguments_delta,
                    });
                }
            }
        }
    }

    if let Some(chunk_usage) = &chunk.usage {
        state.usage = Some(map_usage_to_provider(chunk_usage));
        events.push(UnifiedEvent::Usage {
            usage: map_usage_to_provider(chunk_usage),
        });
    }

    if let Some(reason) = chunk
        .choices
        .first()
        .and_then(|choice| choice.finish_reason.as_deref())
    {
        let finish_reason_value = reason.to_string();
        let usage = state.usage.clone().unwrap_or(Usage {
            input_tokens: 0,
            output_tokens: 0,
            total_tokens: 0,
            cached_tokens: None,
            reasoning_tokens: None,
        });

        if finish_reason_value == "tool_calls" {
            for (index, call_state) in state.tool_call_state.drain() {
                let call_id = ensure_tool_call_id(&state.request_id, index as usize);
                events.push(UnifiedEvent::ToolCallDone {
                    id: call_id,
                    name: call_state.name,
                    arguments: call_state.arguments,
                });
            }
        }

        events.push(UnifiedEvent::MessageStop {
            id: state.request_id.clone(),
            stop_reason: Some(finish_reason_value.clone()),
        });
        events.push(UnifiedEvent::Completed {
            finish_reason: Some(finish_reason_value),
            usage: Some(usage),
        });
    }

    events
}

pub struct StreamMapState {
    pub started: bool,
    pub request_id: String,
    pub model: String,
    pub created_at: String,
    pub usage: Option<Usage>,
    pub tool_call_state: HashMap<i32, ToolCallState>,
}

impl Default for StreamMapState {
    fn default() -> Self {
        Self {
            started: false,
            request_id: String::new(),
            model: String::new(),
            created_at: String::new(),
            usage: None,
            tool_call_state: HashMap::new(),
        }
    }
}

pub fn map_usage_to_provider(usage: &OpenAIUsage) -> Usage {
    Usage {
        input_tokens: usage.prompt_tokens,
        output_tokens: usage.completion_tokens,
        total_tokens: usage.total_tokens,
        cached_tokens: usage.cached_tokens,
        reasoning_tokens: usage.reasoning_tokens,
    }
}

pub fn map_tool_call_from_openai(call: crate::openai_types::ToolCall) -> ToolCall {
    ToolCall {
        id: call.id,
        name: call.function.name,
        description: call.function.description.unwrap_or_default(),
        arguments: call.function.arguments,
    }
}

pub fn map_finish_reason_to_provider(reason: Option<&str>) -> FinishReason {
    match reason.unwrap_or("other") {
        "stop" => FinishReason::Stop,
        "length" => FinishReason::Length,
        "tool_calls" => FinishReason::ToolCalls,
        "content_filter" => FinishReason::ContentFilter,
        _ => FinishReason::Other,
    }
}

pub fn ensure_tool_call_id(prefix: &str, index: usize) -> String {
    format!("{prefix}-tool-{index}")
}

fn map_tool_call_delta(
    call: &ChatCompletionChunkToolCall,
    state: &mut HashMap<i32, ToolCallState>,
) -> Option<ToolCallDeltaEvent> {
    let index = call.index;
    let entry = state.entry(index).or_default();
    let mut arguments_delta = String::new();

    if let Some(ChatCompletionChunkToolCallFunction { name, arguments }) = &call.function {
        if let Some(name) = name {
            entry.name = name.clone();
        }
        if let Some(arguments) = arguments {
            arguments_delta = arguments.clone();
            entry.arguments.push_str(arguments);
        }
    }

    Some(ToolCallDeltaEvent {
        name: entry.name.clone(),
        description: entry.description.clone(),
        arguments_delta,
        index,
    })
}

fn chunk_metadata(chunk: &ChatCompletionChunk) -> (String, String, String) {
    let created_at = if let Some(ts) = chunk.created.filter(|ts| *ts > 0) {
        chrono::DateTime::from_timestamp(ts, 0)
            .unwrap_or_else(|| chrono::Utc::now())
            .to_rfc3339()
    } else {
        chrono::Utc::now().to_rfc3339()
    };
    (chunk.id.clone(), chunk.model.clone(), created_at)
}

fn extract_text(content: Option<OpenAIContent>) -> String {
    match content {
        Some(OpenAIContent::Text(text)) => text,
        Some(OpenAIContent::Parts(parts)) => parts
            .into_iter()
            .filter_map(|part| match part {
                ContentPart::Text { text } => Some(text),
                _ => None,
            })
            .collect::<Vec<_>>()
            .join(""),
        None => String::new(),
    }
}
