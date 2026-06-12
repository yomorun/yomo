use std::collections::HashMap;

use serde_json;
use serde_json::Value;

use crate::llm_provider::{FinishReason, ProviderError, ToolCall, UnifiedEvent, UnifiedResponse};
use crate::openai_types::{
    ChatCompletionChunk, ChatCompletionChunkToolCall, ChatCompletionChunkToolCallFunction,
    ChatCompletionResponse, Content as OpenAIContent, ContentPart, ToolCall as OpenAIToolCall,
};
use crate::usage_handler::EndpointUsage;

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

pub fn map_response(response: ChatCompletionResponse) -> Result<UnifiedResponse, ProviderError> {
    let choice = response
        .choices
        .into_iter()
        .next()
        .ok_or_else(|| ProviderError::internal("missing choices".to_string()))?;
    let finish_reason = map_finish_reason_to_provider(choice.finish_reason.as_deref());
    let output_text = extract_text(choice.message.content);
    let reasoning_content = choice.message.reasoning_content;
    let tool_calls = choice.message.tool_calls.map(|calls| {
        calls
            .into_iter()
            .map(map_tool_call_from_openai)
            .collect::<Vec<_>>()
    });
    let usage = response
        .usage
        .as_ref()
        .and_then(|usage| serde_json::to_value(usage).ok())
        .unwrap_or(Value::Null);

    Ok(UnifiedResponse {
        request_id: response.id,
        created_at: response
            .created
            .and_then(|ts| chrono::DateTime::from_timestamp(ts, 0).map(|dt| dt.to_rfc3339()))
            .unwrap_or_else(|| chrono::Utc::now().to_rfc3339()),
        model: response.model,
        output_text,
        reasoning_content,
        tool_calls,
        finish_reason,
        usage: EndpointUsage::from_endpoint_payload("/chat/completions", usage)
            .expect("openai_compatible mapper expected chat/completions usage payload"),
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
        if let Some(reasoning_content) = &delta.reasoning_content {
            events.push(UnifiedEvent::ThinkingDelta {
                id: state.request_id.clone(),
                delta: reasoning_content.clone(),
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
        events.push(UnifiedEvent::Usage {
            usage: EndpointUsage::ChatCompletions(chunk_usage.clone()),
        });
    }

    if let Some(reason) = chunk
        .choices
        .first()
        .and_then(|choice| choice.finish_reason.as_deref())
    {
        let finish_reason_value = reason.to_string();

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
        });
    }

    events
}

pub struct StreamMapState {
    pub started: bool,
    pub request_id: String,
    pub model: String,
    pub created_at: String,
    pub tool_call_state: HashMap<i32, ToolCallState>,
}

impl Default for StreamMapState {
    fn default() -> Self {
        Self {
            started: false,
            request_id: String::new(),
            model: String::new(),
            created_at: String::new(),
            tool_call_state: HashMap::new(),
        }
    }
}

pub fn map_tool_call_from_openai(call: OpenAIToolCall) -> ToolCall {
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

#[cfg(test)]
mod tests {
    use super::{StreamMapState, map_response, map_stream_chunk};
    use crate::llm_provider::UnifiedEvent;
    use crate::openai_types::{
        ChatCompletionChoice, ChatCompletionChunk, ChatCompletionChunkChoice,
        ChatCompletionChunkDelta, ChatCompletionMessage, ChatCompletionResponse, Usage,
    };

    #[test]
    fn map_response_maps_reasoning_content() {
        let response = ChatCompletionResponse {
            id: "resp-1".to_string(),
            created: Some(1),
            model: "m".to_string(),
            object: "chat.completion".to_string(),
            system_fingerprint: None,
            choices: vec![ChatCompletionChoice {
                message: ChatCompletionMessage {
                    role: crate::openai_types::Role::Assistant,
                    content: Some(crate::openai_types::Content::Text("answer".to_string())),
                    reasoning_content: Some("reasoning".to_string()),
                    annotations: Vec::new(),
                    refusal: None,
                    tool_calls: None,
                },
                finish_reason: Some("stop".to_string()),
                index: 0,
                logprobs: None,
            }],
            usage: Some(Usage {
                prompt_tokens: 1,
                completion_tokens: 1,
                total_tokens: 2,
                prompt_tokens_details: None,
                completion_tokens_details: None,
            }),
        };

        let mapped = map_response(response).expect("map response");
        assert_eq!(mapped.reasoning_content.as_deref(), Some("reasoning"));
    }

    #[test]
    fn map_stream_chunk_emits_thinking_delta_from_reasoning_content() {
        let chunk = ChatCompletionChunk {
            id: "req-1".to_string(),
            created: Some(1),
            model: "m".to_string(),
            object: "chat.completion.chunk".to_string(),
            system_fingerprint: None,
            obfuscation: None,
            choices: vec![ChatCompletionChunkChoice {
                delta: ChatCompletionChunkDelta {
                    role: Some(crate::openai_types::Role::Assistant),
                    content: None,
                    reasoning_content: Some("think step".to_string()),
                    refusal: None,
                    tool_calls: None,
                },
                finish_reason: None,
                index: 0,
                logprobs: None,
            }],
            usage: None,
        };

        let events = map_stream_chunk(chunk, &mut StreamMapState::default());
        assert!(events.iter().any(|event| {
            matches!(
                event,
                UnifiedEvent::ThinkingDelta { delta, .. } if delta == "think step"
            )
        }));
    }
}
