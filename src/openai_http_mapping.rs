use std::pin::Pin;

use async_stream::try_stream;
use axum::body::Bytes;
use axum::http::{StatusCode, header};
use futures_core::Stream;
use futures_util::StreamExt;
use log::{debug, error};
use tracing::{Span, field, debug_span};

use crate::openai_types::{
    ChatCompletionChunk, ChatCompletionChunkChoice, ChatCompletionChunkDelta,
    ChatCompletionChunkToolCall, ChatCompletionChunkToolCallFunction, ChatCompletionRequest,
    ChatCompletionResponse, Content as OpenAIContent, ContentPart, ErrorResponse, Role,
    ToolCall as OpenAIToolCall, ToolCallFunction, ToolChoice, Usage,
};
use crate::llm_provider::{FinishReason, ProviderError, ToolCall, UnifiedEvent, UnifiedResponse};

pub fn map_openai_response(response: UnifiedResponse) -> ChatCompletionResponse {
    let content = if response.output_text.is_empty() {
        None
    } else {
        Some(OpenAIContent::Text(response.output_text))
    };
    let tool_calls = response.tool_calls.map(|calls| {
        calls
            .into_iter()
            .map(map_tool_call_to_openai)
            .collect::<Vec<_>>()
    });

    ChatCompletionResponse {
        id: response.request_id,
        created: None,
        model: response.model,
        object: "chat.completion".to_string(),
        system_fingerprint: None,
        choices: vec![crate::openai_types::ChatCompletionChoice {
            message: crate::openai_types::ChatCompletionMessage {
                role: Role::Assistant,
                content,
                annotations: Vec::new(),
                refusal: None,
                tool_calls,
            },
            finish_reason: Some(map_finish_reason_string(&response.finish_reason)),
            index: 0,
            logprobs: None,
        }],
        usage: Some(map_usage_to_openai(&response.usage)),
    }
}

pub fn validate_openai_request(request: &ChatCompletionRequest) -> Result<(), String> {
    if request.model.trim().is_empty() {
        return Err("model is required".to_string());
    }
    if request.messages.is_empty() {
        return Err("messages is required".to_string());
    }

    for message in &request.messages {
        if !matches!(
            message.role,
            Role::System | Role::Developer | Role::User | Role::Assistant | Role::Tool
        ) {
            return Err(format!("invalid role: {}", message.role.as_str()));
        }
        match &message.content {
            OpenAIContent::Text(text) => {
                if text.trim().is_empty()
                    && !(message.role == Role::Assistant && message.tool_calls.is_some())
                {
                    return Err("content is empty".to_string());
                }
            }
            OpenAIContent::Parts(parts) => {
                if parts.is_empty() {
                    return Err("content parts is empty".to_string());
                }
                for part in parts {
                    match part {
                        ContentPart::Text { text } => {
                            if text.trim().is_empty() {
                                return Err("text content is empty".to_string());
                            }
                        }
                        ContentPart::Image { image_url } => {
                            if image_url.url.trim().is_empty() {
                                return Err("image_url is empty".to_string());
                            }
                        }
                        ContentPart::InputAudio { input_audio } => {
                            if input_audio.data.trim().is_empty() {
                                return Err("input_audio data is empty".to_string());
                            }
                            if input_audio.format.trim().is_empty() {
                                return Err("input_audio format is empty".to_string());
                            }
                        }
                        ContentPart::File {
                            file_id, file_data, ..
                        } => {
                            if file_id.as_deref().unwrap_or("").trim().is_empty()
                                && file_data.as_deref().unwrap_or("").trim().is_empty()
                            {
                                return Err("file content is empty".to_string());
                            }
                        }
                    }
                }
            }
        }
        if message.tool_calls.is_some() && message.role != Role::Assistant {
            return Err("message tool_calls is not supported".to_string());
        }
        if message.role == Role::Tool {
            if message
                .tool_call_id
                .as_deref()
                .unwrap_or("")
                .trim()
                .is_empty()
            {
                return Err("tool_call_id is required for tool messages".to_string());
            }
            match &message.content {
                OpenAIContent::Text(_) => {}
                OpenAIContent::Parts(_) => {
                    return Err("tool messages must use text content".to_string());
                }
            }
        }
    }

    if let Some(tools) = &request.tools {
        for tool in tools {
            if tool.r#type != "function" {
                return Err("only function tool is supported".to_string());
            }
            if tool.function.name.trim().is_empty() {
                return Err("tool name is required".to_string());
            }
        }
    }

    if let Some(tool_choice) = &request.tool_choice {
        match tool_choice {
            ToolChoice::Name(name) => {
                if !matches!(name.as_str(), "none" | "auto" | "required") {
                    return Err("tool_choice value is invalid".to_string());
                }
            }
            ToolChoice::Object { r#type, function } => {
                if r#type != "function" || function.name.trim().is_empty() {
                    return Err("tool_choice function is invalid".to_string());
                }
                return Err("tool_choice function is not supported".to_string());
            }
        }
    }

    Ok(())
}

pub fn stream_openai_chunks(
    stream: Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send>>,
    trace_id: String,
    default_model: String,
    root_span: Span,
) -> impl Stream<Item = Result<Bytes, std::io::Error>> {
    try_stream! {
        futures_util::pin_mut!(stream);
        let mut role_sent = false;
        let mut finish_sent = false;
        let mut response_span: Option<Span> = None;
        let mut sent_preamble = false;
        let mut response_id = String::new();
        let mut model = default_model;
        let mut created_at = String::new();
        let mut tool_call_index: std::collections::HashMap<String, i32> = std::collections::HashMap::new();
        let mut next_tool_index: i32 = 0;

        let ensure_response_span = |response_span: &mut Option<Span>| {
            if response_span.is_none() {
                let span = debug_span!(
                    parent: &root_span,
                    "response.stream",
                    http.status_code = StatusCode::OK.as_u16() as i64,
                    finish_reason = field::Empty,
                );
                *response_span = Some(span);
            }
        };

        while let Some(item) = stream.next().await {
            let event = match item {
                Ok(event) => event,
                Err(err) => {
                    error!("chat stream item error: {err} trace_id={trace_id}");
                    break;
                }
            };
            match event {
                UnifiedEvent::ResponseCreated { id, model: resp_model, created_at: resp_created } => {
                    response_id = id;
                    if !resp_model.trim().is_empty() {
                        model = resp_model;
                    }
                    created_at = resp_created;
                    if !sent_preamble {
                        ensure_response_span(&mut response_span);
                        sent_preamble = true;
                        yield sse_chunk(ChatCompletionChunk {
                            id: String::new(),
                            created: Some(0),
                            model: String::new(),
                            object: "chat.completion.chunk".to_string(),
                            system_fingerprint: None,
                            obfuscation: None,
                            choices: Vec::new(),
                            usage: None,
                        });
                    }
                }
                UnifiedEvent::ResponseInProgress { .. } => {}
                UnifiedEvent::MessageStart { .. } => {}
                UnifiedEvent::MessageDelta { id, delta } => {
                    if response_id.is_empty() {
                        response_id = id.clone();
                    }
                    let delta = ChatCompletionChunkDelta {
                        role: if role_sent { None } else { Some(Role::Assistant) },
                        content: Some(delta),
                        refusal: None,
                        tool_calls: None,
                    };
                    role_sent = true;
                    ensure_response_span(&mut response_span);
                    yield sse_chunk(ChatCompletionChunk {
                        id: response_id.clone(),
                        created: parse_created_at(&created_at),
                        model: model.clone(),
                        object: "chat.completion.chunk".to_string(),
                        system_fingerprint: None,
                        obfuscation: None,
                        choices: vec![ChatCompletionChunkChoice {
                            delta,
                            finish_reason: None,
                            index: 0,
                            logprobs: None,
                        }],
                        usage: None,
                    });
                }
                UnifiedEvent::ToolCallDelta { id, name, arguments_delta } => {
                    let index = *tool_call_index.entry(id.clone()).or_insert_with(|| {
                        let current = next_tool_index;
                        next_tool_index += 1;
                        current
                    });
                    let delta = ChatCompletionChunkDelta {
                        role: if role_sent { None } else { Some(Role::Assistant) },
                        content: None,
                        refusal: None,
                        tool_calls: Some(vec![ChatCompletionChunkToolCall {
                            index,
                            id: Some(id),
                            r#type: Some("function".to_string()),
                            function: Some(ChatCompletionChunkToolCallFunction {
                                name: Some(name),
                                arguments: Some(arguments_delta),
                            }),
                        }]),
                    };
                    role_sent = true;
                    ensure_response_span(&mut response_span);
                    yield sse_chunk(ChatCompletionChunk {
                        id: response_id.clone(),
                        created: parse_created_at(&created_at),
                        model: model.clone(),
                        object: "chat.completion.chunk".to_string(),
                        system_fingerprint: None,
                        obfuscation: None,
                        choices: vec![ChatCompletionChunkChoice {
                            delta,
                            finish_reason: None,
                            index,
                            logprobs: None,
                        }],
                        usage: None,
                    });
                }
                UnifiedEvent::ToolCallDone { id, name, arguments } => {
                    let index = *tool_call_index.entry(id.clone()).or_insert_with(|| {
                        let current = next_tool_index;
                        next_tool_index += 1;
                        current
                    });
                    let delta = ChatCompletionChunkDelta {
                        role: if role_sent { None } else { Some(Role::Assistant) },
                        content: None,
                        refusal: None,
                        tool_calls: Some(vec![ChatCompletionChunkToolCall {
                            index,
                            id: Some(id),
                            r#type: Some("function".to_string()),
                            function: Some(ChatCompletionChunkToolCallFunction {
                                name: Some(name),
                                arguments: Some(arguments),
                            }),
                        }]),
                    };
                    role_sent = true;
                    ensure_response_span(&mut response_span);
                    yield sse_chunk(ChatCompletionChunk {
                        id: response_id.clone(),
                        created: parse_created_at(&created_at),
                        model: model.clone(),
                        object: "chat.completion.chunk".to_string(),
                        system_fingerprint: None,
                        obfuscation: None,
                        choices: vec![ChatCompletionChunkChoice {
                            delta,
                            finish_reason: None,
                            index,
                            logprobs: None,
                        }],
                        usage: None,
                    });
                }
                UnifiedEvent::Usage { usage } => {
                    ensure_response_span(&mut response_span);
                    yield sse_chunk(ChatCompletionChunk {
                        id: response_id.clone(),
                        created: parse_created_at(&created_at),
                        model: model.clone(),
                        object: "chat.completion.chunk".to_string(),
                        system_fingerprint: None,
                        obfuscation: None,
                        choices: Vec::new(),
                        usage: Some(map_usage(&usage)),
                    });
                }
                UnifiedEvent::MessageStop { .. } => {}
                UnifiedEvent::Completed { finish_reason, usage } => {
                    let model_for_log = model.clone();
                    if let (Some(reason), Some(span)) = (finish_reason.as_deref(), response_span.as_ref()) {
                        span.record("finish_reason", field::display(reason));
                    }
                    if !finish_sent {
                        let request_id = response_id.clone();
                        let model = model.clone();
                        let delta = ChatCompletionChunkDelta {
                            role: if role_sent { None } else { Some(Role::Assistant) },
                            content: None,
                            refusal: None,
                            tool_calls: None,
                        };
                        role_sent = true;
                        finish_sent = true;
                        let usage = usage.as_ref().map(map_usage);
                        ensure_response_span(&mut response_span);
                        yield sse_chunk(ChatCompletionChunk {
                            id: request_id,
                            created: parse_created_at(&created_at),
                            model,
                            object: "chat.completion.chunk".to_string(),
                            system_fingerprint: None,
                            obfuscation: None,
                            choices: vec![ChatCompletionChunkChoice {
                                delta,
                                finish_reason: finish_reason.clone().map(|value| map_finish_reason(&value)),
                                index: 0,
                                logprobs: None,
                            }],
                            usage,
                        });
                    }
                    let finish_reason_log = finish_reason.as_deref().unwrap_or("");
                    let usage_for_log = usage.as_ref();
                    debug!(
                        "http.request.stream.completed; model_id={} finish_reason={} prompt_tokens={} completion_tokens={} trace_id={}",
                        model_for_log,
                        finish_reason_log,
                        usage_for_log.map(|value| value.input_tokens).unwrap_or(0),
                        usage_for_log.map(|value| value.output_tokens).unwrap_or(0),
                        trace_id
                    );
                }
                UnifiedEvent::Failed { code, message } => {
                    error!(
                        "chat stream failed: model={}, code={}, message={} trace_id={trace_id}",
                        model, code, message
                    );
                    break;
                }
                UnifiedEvent::Cancelled { reason } => {
                    error!(
                        "chat stream cancelled: model={}, reason={} trace_id={trace_id}",
                        model, reason
                    );
                    break;
                }
                UnifiedEvent::OutputItemAdded { .. }
                | UnifiedEvent::OutputItemDone { .. }
                | UnifiedEvent::ContentPartAdded { .. }
                | UnifiedEvent::ContentPartDelta { .. }
                | UnifiedEvent::ContentPartDone { .. }
                | UnifiedEvent::ThinkingDelta { .. }
                | UnifiedEvent::ThinkingDone { .. } => {}
            }
        }

        yield Bytes::from_static(b"data: [DONE]\n\n");
    }
}

fn map_tool_call_to_openai(call: ToolCall) -> OpenAIToolCall {
    OpenAIToolCall {
        id: call.id,
        r#type: Some("function".to_string()),
        function: ToolCallFunction {
            name: call.name,
            arguments: call.arguments,
            description: Some(call.description),
        },
    }
}

fn map_finish_reason_string(reason: &FinishReason) -> String {
    match reason {
        FinishReason::Stop => "stop",
        FinishReason::Length => "length",
        FinishReason::ToolCalls => "tool_calls",
        FinishReason::ContentFilter => "content_filter",
        FinishReason::Other => "other",
    }
    .to_string()
}

pub fn map_usage_to_openai(usage: &crate::llm_provider::Usage) -> Usage {
    Usage {
        prompt_tokens: usage.input_tokens,
        completion_tokens: usage.output_tokens,
        total_tokens: usage.total_tokens,
        cached_tokens: usage.cached_tokens,
        reasoning_tokens: usage.reasoning_tokens,
        prompt_tokens_details: Some(crate::openai_types::PromptTokensDetails {
            audio_tokens: 0,
            cached_tokens: usage.cached_tokens.unwrap_or(0),
        }),
        completion_tokens_details: Some(crate::openai_types::CompletionTokensDetails {
            accepted_prediction_tokens: 0,
            audio_tokens: 0,
            reasoning_tokens: usage.reasoning_tokens.unwrap_or(0),
            rejected_prediction_tokens: 0,
        }),
    }
}

fn sse_chunk(chunk: ChatCompletionChunk) -> Bytes {
    let payload = serde_json::to_string(&chunk).unwrap_or_else(|_| "{}".to_string());
    Bytes::from(format!("data: {payload}\n\n"))
}

fn parse_created_at(value: &str) -> Option<i64> {
    if value.trim().is_empty() {
        return None;
    }
    chrono::DateTime::parse_from_rfc3339(value)
        .ok()
        .map(|dt| dt.timestamp())
}

fn map_finish_reason(reason: &str) -> String {
    match reason {
        "stop" => "stop",
        "length" => "length",
        "tool_calls" => "tool_calls",
        "content_filter" => "content_filter",
        _ => "other",
    }
    .to_string()
}

fn map_usage(usage: &crate::llm_provider::Usage) -> Usage {
    Usage {
        prompt_tokens: usage.input_tokens,
        completion_tokens: usage.output_tokens,
        total_tokens: usage.total_tokens,
        cached_tokens: usage.cached_tokens,
        reasoning_tokens: usage.reasoning_tokens,
        prompt_tokens_details: Some(crate::openai_types::PromptTokensDetails {
            audio_tokens: 0,
            cached_tokens: usage.cached_tokens.unwrap_or(0),
        }),
        completion_tokens_details: Some(crate::openai_types::CompletionTokensDetails {
            accepted_prediction_tokens: 0,
            audio_tokens: 0,
            reasoning_tokens: usage.reasoning_tokens.unwrap_or(0),
            rejected_prediction_tokens: 0,
        }),
    }
}

pub fn openai_error_response(
    status: axum::http::StatusCode,
    message: &str,
    error_type: Option<&str>,
) -> axum::response::Response {
    let response = ErrorResponse {
        error: crate::openai_types::ErrorDetail {
            message: message.to_string(),
            r#type: error_type.unwrap_or("internal_error").to_string(),
            code: None,
            param: None,
        },
    };
    let payload = serde_json::to_vec(&response).unwrap_or_else(|_| b"{}".to_vec());
    axum::response::Response::builder()
        .status(status)
        .header(header::CONTENT_TYPE, "application/json")
        .body(axum::body::Body::from(payload))
        .expect("build error response")
}

pub fn map_chat_error(err: ProviderError) -> axum::response::Response {
    match err {
        ProviderError::Public { status, error } => {
            let response = ErrorResponse { error };
            let payload = serde_json::to_vec(&response).unwrap_or_else(|_| b"{}".to_vec());
            axum::response::Response::builder()
                .status(status)
                .header(header::CONTENT_TYPE, "application/json")
                .body(axum::body::Body::from(payload))
                .expect("build error response")
        }
        ProviderError::Internal(_) => openai_error_response(
            axum::http::StatusCode::INTERNAL_SERVER_ERROR,
            "internal error",
            Some("internal_error"),
        ),
    }
}
