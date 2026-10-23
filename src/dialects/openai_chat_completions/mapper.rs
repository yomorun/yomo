//! OpenAI Chat Completions response and stream mapping into the shared IR.

use std::collections::HashMap;

use axum::http::StatusCode;
use serde_json;
use serde_json::Value;

use crate::openai_types::{
    ChatCompletionChunk, ChatCompletionChunkToolCall, ChatCompletionChunkToolCallFunction,
    ChatCompletionRequest, ChatCompletionResponse, Content as OpenAIContent, ContentPart,
    ErrorDetail, ToolCall as OpenAIToolCall,
};
use crate::provider::{FinishReason, ProviderError, ToolCall, UnifiedEvent, UnifiedResponse};
use crate::provider::{
    UnifiedContentDelta, UnifiedEnding, UnifiedEventKind, UnifiedEventTarget, UnifiedItemDelta,
    UnifiedOutputItem, UnifiedOutputItemKind, UnifiedPartLocation, UnifiedRequest,
    UnifiedSourceProtocol, UnifiedUsageUpdate,
};
use crate::usage_handler::EndpointUsage;
use crate::utils::ensure_tool_call_id;

pub fn request_to_chat(request: &UnifiedRequest) -> Result<ChatCompletionRequest, ProviderError> {
    let value = crate::api::request_to_value(request, UnifiedSourceProtocol::ChatCompletions)
        .map_err(|err| ProviderError::Public {
            status: StatusCode::BAD_REQUEST,
            error: ErrorDetail {
                message: err,
                r#type: "invalid_request_error".into(),
                code: Some("unsupported_conversion".into()),
                param: None,
            },
        })?;
    serde_json::from_value(value).map_err(|err| ProviderError::internal(err.to_string()))
}
pub fn request_from_chat(request: &ChatCompletionRequest) -> Result<UnifiedRequest, ProviderError> {
    let mut request = request.clone();
    request.normalize_reasoning_compat_fields();
    let value =
        serde_json::to_value(&request).map_err(|err| ProviderError::internal(err.to_string()))?;
    crate::api::request_from_value(&value, UnifiedSourceProtocol::ChatCompletions)
        .map_err(ProviderError::internal)
}

#[derive(Default)]
pub struct ToolCallState {
    id: Option<String>,
    name: String,
    description: String,
    arguments: String,
}

pub struct ToolCallDeltaEvent {
    pub id: Option<String>,
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

    Ok(UnifiedResponse::from_completion(
        response.id,
        response
            .created
            .and_then(|ts| chrono::DateTime::from_timestamp(ts, 0).map(|dt| dt.to_rfc3339()))
            .unwrap_or_else(|| chrono::Utc::now().to_rfc3339()),
        response.model,
        output_text,
        reasoning_content,
        tool_calls,
        finish_reason,
        EndpointUsage::from_endpoint_payload("/chat/completions", usage)
            .expect("openai_compatible mapper expected chat/completions usage payload"),
    ))
}

pub fn map_stream_chunk(
    chunk: ChatCompletionChunk,
    state: &mut StreamMapState,
) -> Vec<UnifiedEvent> {
    let mut events = Vec::new();
    let (chunk_request_id, chunk_model, chunk_created) = chunk_metadata(&chunk);
    if state.request_id.is_empty() && !chunk_request_id.trim().is_empty() {
        state.request_id = chunk_request_id;
        state.model = chunk_model;
        state.created_at = chunk_created;
    }

    if state.request_id.is_empty() {
        state.pending_chunks.push(chunk);
        return events;
    }

    if !state.started && !state.request_id.is_empty() {
        state.started = true;
        let mut response = UnifiedResponse::new(state.request_id.clone(), state.model.clone());
        response.created_at = chrono::DateTime::parse_from_rfc3339(&state.created_at)
            .ok()
            .map(|time| time.timestamp());
        events.push(UnifiedEvent::new(UnifiedEventKind::ResponseCreated {
            response: response.clone(),
        }));
        events.push(UnifiedEvent::new(UnifiedEventKind::ResponseInProgress {
            response: Some(response),
        }));
        let id = state.request_id.clone();
        events.push(
            UnifiedEvent::new(UnifiedEventKind::OutputItemAdded {
                item: UnifiedOutputItem::new(
                    id.clone(),
                    UnifiedOutputItemKind::Message {
                        role: crate::provider::UnifiedRole::Assistant,
                        content: vec![],
                        phase: None,
                    },
                ),
            })
            .located(0, id, 0, None),
        );
    }

    let pending_chunks = std::mem::take(&mut state.pending_chunks);
    for pending_chunk in pending_chunks {
        append_stream_chunk_events(pending_chunk, state, &mut events);
    }

    append_stream_chunk_events(chunk, state, &mut events);

    events
}

fn append_stream_chunk_events(
    chunk: ChatCompletionChunk,
    state: &mut StreamMapState,
    events: &mut Vec<UnifiedEvent>,
) {
    if let Some(delta) = chunk.choices.first().map(|choice| &choice.delta) {
        if let Some(content) = &delta.content {
            events.push(
                UnifiedEvent::new(UnifiedEventKind::ContentPartDelta {
                    delta: UnifiedContentDelta::Text(content.clone()),
                })
                .located(
                    0,
                    state.request_id.clone(),
                    0,
                    Some(UnifiedPartLocation::Content(0)),
                ),
            );
        }
        if let Some(reasoning_content) = &delta.reasoning_content {
            events.push(
                UnifiedEvent::new(UnifiedEventKind::ContentPartDelta {
                    delta: UnifiedContentDelta::Thinking(reasoning_content.clone()),
                })
                .located(
                    0,
                    state.request_id.clone(),
                    0,
                    Some(UnifiedPartLocation::Content(1)),
                ),
            );
        }
        if let Some(tool_calls) = &delta.tool_calls {
            for call in tool_calls {
                if let Some(delta_event) = map_tool_call_delta(call, &mut state.tool_call_state) {
                    let call_id = delta_event.id.unwrap_or_else(|| {
                        ensure_tool_call_id(&state.request_id, delta_event.index as usize)
                    });
                    if state.announced_tools.insert(call_id.clone()) {
                        let item = UnifiedOutputItem::new(
                            call_id.clone(),
                            UnifiedOutputItemKind::FunctionCall {
                                call_id: call_id.clone(),
                                name: delta_event.name,
                                arguments: String::new(),
                                caller: None,
                            },
                        );
                        events.push(
                            UnifiedEvent::new(UnifiedEventKind::OutputItemAdded { item }).located(
                                0,
                                call_id.clone(),
                                delta_event.index as u32 + 1,
                                None,
                            ),
                        );
                    }
                    events.push(
                        UnifiedEvent::new(UnifiedEventKind::ItemDelta {
                            delta: UnifiedItemDelta::FunctionArguments(delta_event.arguments_delta),
                        })
                        .located(
                            0,
                            call_id,
                            delta_event.index as u32 + 1,
                            None,
                        ),
                    );
                }
            }
        }
    }

    if let Some(chunk_usage) = &chunk.usage {
        events.push(UnifiedEvent::new(UnifiedEventKind::Usage {
            usage: EndpointUsage::ChatCompletions(chunk_usage.clone()),
            update: UnifiedUsageUpdate::Snapshot,
            source_usage: None,
        }));
    }

    if let Some(reason) = chunk
        .choices
        .first()
        .and_then(|choice| choice.finish_reason.as_deref())
    {
        let finish_reason_value = reason.to_string();

        if finish_reason_value == "tool_calls" {
            for (index, call_state) in state.tool_call_state.drain() {
                let call_id = call_state
                    .id
                    .unwrap_or_else(|| ensure_tool_call_id(&state.request_id, index as usize));
                let item = UnifiedOutputItem::new(
                    call_id.clone(),
                    UnifiedOutputItemKind::FunctionCall {
                        call_id: call_id.clone(),
                        name: call_state.name,
                        arguments: call_state.arguments,
                        caller: None,
                    },
                );
                events.push(
                    UnifiedEvent::new(UnifiedEventKind::OutputItemDone { item }).located(
                        0,
                        call_id,
                        index as u32 + 1,
                        None,
                    ),
                );
            }
        }

        let mut event = UnifiedEvent::new(UnifiedEventKind::CandidateDone {
            ending: UnifiedEnding::from_source(&finish_reason_value),
        });
        event.target = Some(UnifiedEventTarget {
            candidate_index: 0,
            item_id: None,
            output_index: None,
            part: None,
        });
        events.push(event);
        events.push(UnifiedEvent::new(UnifiedEventKind::Completed {
            response: None,
        }));
    }
}

pub struct StreamMapState {
    pub started: bool,
    pub request_id: String,
    pub model: String,
    pub created_at: String,
    pub tool_call_state: HashMap<i32, ToolCallState>,
    pub pending_chunks: Vec<ChatCompletionChunk>,
    announced_tools: std::collections::HashSet<String>,
}

impl Default for StreamMapState {
    fn default() -> Self {
        Self {
            started: false,
            request_id: String::new(),
            model: String::new(),
            created_at: String::new(),
            tool_call_state: HashMap::new(),
            pending_chunks: Vec::new(),
            announced_tools: Default::default(),
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

fn map_tool_call_delta(
    call: &ChatCompletionChunkToolCall,
    state: &mut HashMap<i32, ToolCallState>,
) -> Option<ToolCallDeltaEvent> {
    let index = call.index;
    let entry = state.entry(index).or_default();
    let mut arguments_delta = String::new();

    if let Some(id) = &call.id {
        entry.id = Some(id.clone());
    }

    if let Some(ChatCompletionChunkToolCallFunction { name, arguments }) = &call.function {
        if let Some(name) = name {
            if !name.trim().is_empty() {
                entry.name = name.clone();
            }
        }
        if let Some(arguments) = arguments {
            arguments_delta = arguments.clone();
            entry.arguments.push_str(arguments);
        }
    }

    Some(ToolCallDeltaEvent {
        id: entry.id.clone(),
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
    use super::{StreamMapState, map_response, map_stream_chunk, request_from_chat};
    use crate::openai_types::{
        ChatCompletionChoice, ChatCompletionChunk, ChatCompletionChunkChoice,
        ChatCompletionChunkDelta, ChatCompletionChunkToolCall, ChatCompletionChunkToolCallFunction,
        ChatCompletionMessage, ChatCompletionRequest, ChatCompletionResponse, Usage,
    };
    use crate::provider::{UnifiedContentDelta, UnifiedEventKind, UnifiedOutputItemKind};

    #[test]
    fn azure_effort_takes_precedence_and_template_controls_are_fallbacks() {
        let request: ChatCompletionRequest = serde_json::from_value(serde_json::json!({
            "model":"m",
            "messages":[],
            "reasoning_effort":"high",
            "thinking":{"type":"disabled","budget_tokens":2048},
            "chat_template_kwargs":{"thinking":false,"reasoning_effort":60}
        }))
        .expect("chat request");

        let unified = request_from_chat(&request).expect("unified request");
        assert_eq!(
            unified.options.thinking,
            Some(serde_json::json!({"type":"enabled","budget_tokens":2048}))
        );
        assert_eq!(
            unified.options.reasoning_effort,
            Some(crate::provider::ReasoningEffort::from("high"))
        );
        assert!(unified.options.chat_template_kwargs.is_none());

        let request: ChatCompletionRequest = serde_json::from_value(serde_json::json!({
            "model":"m",
            "messages":[],
            "reasoning_effort":"none",
            "thinking":{"type":"enabled"}
        }))
        .expect("conflicting controls are accepted");
        let unified = request_from_chat(&request).expect("normalized request");
        assert_eq!(
            unified.options.reasoning_effort,
            Some(crate::provider::ReasoningEffort::from("none"))
        );
        assert_eq!(
            unified.options.thinking,
            Some(serde_json::json!({"type":"disabled"}))
        );

        let request: ChatCompletionRequest = serde_json::from_value(serde_json::json!({
            "model":"m",
            "messages":[],
            "chat_template_kwargs":{"thinking":true,"reasoning_effort":60}
        }))
        .expect("template-style chat request");
        let unified = request_from_chat(&request).expect("unified request");
        assert_eq!(
            unified.options.reasoning_effort,
            Some(crate::provider::ReasoningEffort::Numeric(
                serde_json::Number::from(60)
            ))
        );
        assert_eq!(
            unified.options.thinking,
            Some(serde_json::json!({"type":"enabled"}))
        );
    }

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
        assert_eq!(
            crate::openai_http_mapping::map_openai_response(mapped).choices[0]
                .message
                .reasoning_content
                .as_deref(),
            Some("reasoning")
        );
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
                &event.kind,
                UnifiedEventKind::ContentPartDelta { delta: UnifiedContentDelta::Thinking(delta) } if delta == "think step"
            )
        }));
    }

    #[test]
    fn map_stream_chunk_waits_for_first_non_empty_response_id() {
        let mut state = StreamMapState::default();

        let first_chunk = ChatCompletionChunk {
            id: String::new(),
            created: Some(1),
            model: "m".to_string(),
            object: "chat.completion.chunk".to_string(),
            system_fingerprint: None,
            obfuscation: None,
            choices: Vec::new(),
            usage: None,
        };
        let first_events = map_stream_chunk(first_chunk, &mut state);
        assert!(first_events.is_empty());
        assert!(!state.started);
        assert!(state.request_id.is_empty());
        assert_eq!(state.pending_chunks.len(), 1);

        let second_chunk = ChatCompletionChunk {
            id: "req-1".to_string(),
            created: Some(1),
            model: "m".to_string(),
            object: "chat.completion.chunk".to_string(),
            system_fingerprint: None,
            obfuscation: None,
            choices: vec![ChatCompletionChunkChoice {
                delta: ChatCompletionChunkDelta {
                    role: Some(crate::openai_types::Role::Assistant),
                    content: Some("hello".to_string()),
                    reasoning_content: None,
                    refusal: None,
                    tool_calls: None,
                },
                finish_reason: None,
                index: 0,
                logprobs: None,
            }],
            usage: None,
        };
        let second_events = map_stream_chunk(second_chunk, &mut state);

        assert!(state.started);
        assert_eq!(state.request_id, "req-1");
        assert!(matches!(
            second_events.first().map(|event| &event.kind),
            Some(UnifiedEventKind::ResponseCreated { response }) if response.id == "req-1"
        ));
        assert!(second_events.iter().any(|event| matches!(
            &event.kind,
            UnifiedEventKind::ContentPartDelta { delta: UnifiedContentDelta::Text(delta) } if event.target.as_ref().and_then(|target|target.item_id.as_deref()) == Some("req-1") && delta == "hello"
        )));
    }

    #[test]
    fn map_stream_chunk_replays_pending_content_after_response_id_arrives() {
        let mut state = StreamMapState::default();

        let pending_chunk = ChatCompletionChunk {
            id: String::new(),
            created: Some(1),
            model: "m".to_string(),
            object: "chat.completion.chunk".to_string(),
            system_fingerprint: None,
            obfuscation: None,
            choices: vec![ChatCompletionChunkChoice {
                delta: ChatCompletionChunkDelta {
                    role: Some(crate::openai_types::Role::Assistant),
                    content: Some("hello".to_string()),
                    reasoning_content: None,
                    refusal: None,
                    tool_calls: None,
                },
                finish_reason: None,
                index: 0,
                logprobs: None,
            }],
            usage: None,
        };
        assert!(map_stream_chunk(pending_chunk, &mut state).is_empty());

        let id_chunk = ChatCompletionChunk {
            id: "req-1".to_string(),
            created: Some(1),
            model: "m".to_string(),
            object: "chat.completion.chunk".to_string(),
            system_fingerprint: None,
            obfuscation: None,
            choices: vec![ChatCompletionChunkChoice {
                delta: ChatCompletionChunkDelta {
                    role: None,
                    content: Some(" world".to_string()),
                    reasoning_content: None,
                    refusal: None,
                    tool_calls: None,
                },
                finish_reason: None,
                index: 0,
                logprobs: None,
            }],
            usage: None,
        };
        let events = map_stream_chunk(id_chunk, &mut state);

        let deltas = events
            .iter()
            .filter_map(|event| match &event.kind {
                UnifiedEventKind::ContentPartDelta {
                    delta: UnifiedContentDelta::Text(delta),
                } => event
                    .target
                    .as_ref()
                    .and_then(|target| target.item_id.as_deref())
                    .map(|id| (id, delta.as_str())),
                _ => None,
            })
            .collect::<Vec<_>>();
        assert_eq!(deltas, vec![("req-1", "hello"), ("req-1", " world")]);
        assert!(state.pending_chunks.is_empty());
    }

    #[test]
    fn map_stream_chunk_prefers_provider_tool_call_id() {
        let mut state = StreamMapState::default();
        let mut events = Vec::new();

        let first_chunk = ChatCompletionChunk {
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
                    reasoning_content: None,
                    refusal: None,
                    tool_calls: Some(vec![ChatCompletionChunkToolCall {
                        index: 0,
                        id: Some("call_abc123".to_string()),
                        r#type: Some("function".to_string()),
                        function: Some(ChatCompletionChunkToolCallFunction {
                            name: Some("client_ping".to_string()),
                            arguments: Some("{\"".to_string()),
                        }),
                    }]),
                },
                finish_reason: None,
                index: 0,
                logprobs: None,
            }],
            usage: None,
        };
        events.extend(map_stream_chunk(first_chunk, &mut state));

        let second_chunk = ChatCompletionChunk {
            id: "req-1".to_string(),
            created: Some(1),
            model: "m".to_string(),
            object: "chat.completion.chunk".to_string(),
            system_fingerprint: None,
            obfuscation: None,
            choices: vec![ChatCompletionChunkChoice {
                delta: ChatCompletionChunkDelta {
                    role: None,
                    content: None,
                    reasoning_content: None,
                    refusal: None,
                    tool_calls: Some(vec![ChatCompletionChunkToolCall {
                        index: 0,
                        id: None,
                        r#type: None,
                        function: Some(ChatCompletionChunkToolCallFunction {
                            name: None,
                            arguments: Some("message\":\"ping\"}".to_string()),
                        }),
                    }]),
                },
                finish_reason: None,
                index: 0,
                logprobs: None,
            }],
            usage: None,
        };
        events.extend(map_stream_chunk(second_chunk, &mut state));

        let finish_chunk = ChatCompletionChunk {
            id: "req-1".to_string(),
            created: Some(1),
            model: "m".to_string(),
            object: "chat.completion.chunk".to_string(),
            system_fingerprint: None,
            obfuscation: None,
            choices: vec![ChatCompletionChunkChoice {
                delta: ChatCompletionChunkDelta {
                    role: None,
                    content: None,
                    reasoning_content: None,
                    refusal: None,
                    tool_calls: None,
                },
                finish_reason: Some("tool_calls".to_string()),
                index: 0,
                logprobs: None,
            }],
            usage: None,
        };
        events.extend(map_stream_chunk(finish_chunk, &mut state));

        let delta_ids = events
            .iter()
            .filter_map(|event| match &event.kind {
                UnifiedEventKind::ItemDelta { .. } => event
                    .target
                    .as_ref()
                    .and_then(|target| target.item_id.as_deref()),
                _ => None,
            })
            .collect::<Vec<_>>();
        assert_eq!(delta_ids, vec!["call_abc123", "call_abc123"]);

        let done_ids = events
            .iter()
            .filter_map(|event| match &event.kind {
                UnifiedEventKind::OutputItemDone { item } => {
                    Some(item.identity.internal_id.as_str())
                }
                _ => None,
            })
            .collect::<Vec<_>>();
        assert_eq!(done_ids, vec!["call_abc123"]);
    }

    #[test]
    fn map_stream_chunk_falls_back_to_synthesized_tool_call_id() {
        let mut state = StreamMapState::default();

        let first_chunk = ChatCompletionChunk {
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
                    reasoning_content: None,
                    refusal: None,
                    tool_calls: Some(vec![ChatCompletionChunkToolCall {
                        index: 0,
                        id: None,
                        r#type: Some("function".to_string()),
                        function: Some(ChatCompletionChunkToolCallFunction {
                            name: Some("client_ping".to_string()),
                            arguments: Some("{\"message\":\"ping\"}".to_string()),
                        }),
                    }]),
                },
                finish_reason: None,
                index: 0,
                logprobs: None,
            }],
            usage: None,
        };
        let events = map_stream_chunk(first_chunk, &mut state);
        let delta_id = events.iter().find_map(|event| match &event.kind {
            UnifiedEventKind::ItemDelta { .. } => event
                .target
                .as_ref()
                .and_then(|target| target.item_id.as_deref()),
            _ => None,
        });
        assert_eq!(delta_id, Some("req-1-tool-0"));
    }

    #[test]
    fn map_stream_chunk_keeps_previous_name_when_later_chunk_name_is_empty() {
        let mut state = StreamMapState::default();
        let mut events = Vec::new();

        let first_chunk = ChatCompletionChunk {
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
                    reasoning_content: None,
                    refusal: None,
                    tool_calls: Some(vec![ChatCompletionChunkToolCall {
                        index: 0,
                        id: Some("call_weather".to_string()),
                        r#type: Some("function".to_string()),
                        function: Some(ChatCompletionChunkToolCallFunction {
                            name: Some("get_weather".to_string()),
                            arguments: Some("{\"location\"".to_string()),
                        }),
                    }]),
                },
                finish_reason: None,
                index: 0,
                logprobs: None,
            }],
            usage: None,
        };
        events.extend(map_stream_chunk(first_chunk, &mut state));

        let second_chunk = ChatCompletionChunk {
            id: "req-1".to_string(),
            created: Some(1),
            model: "m".to_string(),
            object: "chat.completion.chunk".to_string(),
            system_fingerprint: None,
            obfuscation: None,
            choices: vec![ChatCompletionChunkChoice {
                delta: ChatCompletionChunkDelta {
                    role: None,
                    content: None,
                    reasoning_content: None,
                    refusal: None,
                    tool_calls: Some(vec![ChatCompletionChunkToolCall {
                        index: 0,
                        id: None,
                        r#type: None,
                        function: Some(ChatCompletionChunkToolCallFunction {
                            name: Some("".to_string()),
                            arguments: Some(":\"beijing\"}".to_string()),
                        }),
                    }]),
                },
                finish_reason: None,
                index: 0,
                logprobs: None,
            }],
            usage: None,
        };
        events.extend(map_stream_chunk(second_chunk, &mut state));

        let finish_chunk = ChatCompletionChunk {
            id: "req-1".to_string(),
            created: Some(1),
            model: "m".to_string(),
            object: "chat.completion.chunk".to_string(),
            system_fingerprint: None,
            obfuscation: None,
            choices: vec![ChatCompletionChunkChoice {
                delta: ChatCompletionChunkDelta {
                    role: None,
                    content: None,
                    reasoning_content: None,
                    refusal: None,
                    tool_calls: None,
                },
                finish_reason: Some("tool_calls".to_string()),
                index: 0,
                logprobs: None,
            }],
            usage: None,
        };
        events.extend(map_stream_chunk(finish_chunk, &mut state));

        let done = events.iter().find_map(|event| match &event.kind {
            UnifiedEventKind::OutputItemDone { item } => match &item.kind {
                UnifiedOutputItemKind::FunctionCall {
                    name, arguments, ..
                } => Some((name.as_str(), arguments.as_str())),
                _ => None,
            },
            _ => None,
        });
        assert_eq!(done, Some(("get_weather", "{\"location\":\"beijing\"}")));
    }
}
