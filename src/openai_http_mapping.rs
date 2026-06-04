use std::collections::{HashMap, HashSet};
use std::pin::Pin;
use std::sync::Arc;
use std::sync::Mutex;

use async_stream::try_stream;
use axum::body::{Body, Bytes};
use axum::http::{StatusCode, header};
use axum::response::Response;
use futures_core::Stream;
use futures_util::StreamExt;
use log::{error, info};
use serde_json;
use serde_json::Value;
use tracing::{Span, field};

use crate::llm_provider::{
    FinishReason, ProviderError, ToOpenAIUsage, ToolCall, UnifiedEvent, UnifiedResponse,
};
use crate::openai_types::{
    ChatCompletionChoice, ChatCompletionChunk, ChatCompletionChunkChoice, ChatCompletionChunkDelta,
    ChatCompletionChunkToolCall, ChatCompletionChunkToolCallFunction, ChatCompletionMessage,
    ChatCompletionRequest, ChatCompletionResponse, Content as OpenAIContent, ContentPart,
    ErrorDetail, ErrorResponse, Role, ToolCall as OpenAIToolCall, ToolCallFunction, ToolChoice,
    Usage,
};
use crate::trace::{record_flattened_json_attributes, set_http_span_status};
use crate::usage_handler::parse_endpoint_usage_as_input_output;

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
        created: parse_created_at(&response.created_at),
        model: response.model,
        object: "chat.completion".to_string(),
        system_fingerprint: None,
        choices: vec![ChatCompletionChoice {
            message: ChatCompletionMessage {
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
        let mut sent_preamble = false;
        let mut response_id = String::new();
        let mut model = default_model;
        let mut created_at = String::new();
        let mut latest_usage_for_root: Option<Value> = None;
        let mut finalizer = StreamSpanFinalizer::new(root_span.clone(), trace_id.clone(), model.clone());
        let mut tool_call_index: HashMap<String, i32> = HashMap::new();
        let mut tool_call_delta_emitted: HashSet<String> = HashSet::new();
        let mut next_tool_index: i32 = 0;

        while let Some(item) = stream.next().await {
            let event = match item {
                Ok(event) => event,
                Err(err) => {
                    error!("chat stream item error: {err} trace_id={trace_id}");
                    finalizer.set_failure(model.clone(), err.to_string());
                    break;
                }
            };
            match event {
                UnifiedEvent::ResponseCreated { id, model: resp_model, created_at: resp_created } => {
                    response_id = id;
                    if !resp_model.trim().is_empty() {
                        model = resp_model;
                        finalizer.set_model(model.clone());
                    }
                    created_at = resp_created;
                    if !sent_preamble {
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
                    tool_call_delta_emitted.insert(id.clone());
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
                    if tool_call_delta_emitted.contains(&id) {
                        continue;
                    }
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
                    latest_usage_for_root = serde_json::to_value(map_usage(&usage)).ok();
                    finalizer.set_latest_usage(usage.clone());
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
                    root_span.record(
                        "finish_reason",
                        field::display(finish_reason.as_deref().unwrap_or("")),
                    );
                    if let Some(usage) = usage.as_ref() {
                        latest_usage_for_root = serde_json::to_value(map_usage(usage)).ok();
                        finalizer.set_latest_usage(usage.clone());
                    }
                    finalizer.set_finish_reason(finish_reason.clone());
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
                }
                UnifiedEvent::Failed { code, message } => {
                    error!(
                        "chat stream failed: model={}, code={}, message={} trace_id={trace_id}",
                        model, code, message
                    );
                    finalizer.set_failure(model.clone(), format!("code={code} message={message}"));
                    break;
                }
                UnifiedEvent::Cancelled { reason } => {
                    error!(
                        "chat stream cancelled: model={}, reason={} trace_id={trace_id}",
                        model, reason
                    );
                    finalizer.set_failure(model.clone(), format!("cancelled: {reason}"));
                    break;
                }
                UnifiedEvent::OutputItemAdded { .. }
                | UnifiedEvent::OutputItemDone { .. }
                | UnifiedEvent::ContentPartAdded { .. }
                | UnifiedEvent::ContentPartDelta { .. }
                | UnifiedEvent::ContentPartDone { .. }
                | UnifiedEvent::ThinkingDelta { .. }
                | UnifiedEvent::ThinkingDone { .. }
                | UnifiedEvent::ServerToolCall { .. }
                | UnifiedEvent::ServerToolCallResult { .. } => {}
            }
        }

        if let Some(usage_value) = latest_usage_for_root {
            record_flattened_json_attributes(&root_span, "usage", &usage_value);
        }
        finalizer.set_success_if_unset();

        yield Bytes::from_static(b"data: [DONE]\n\n");
    }
}

#[derive(Clone)]
struct StreamSpanFinalizer {
    root_span: Span,
    trace_id: String,
    state: Arc<Mutex<StreamFinalState>>,
}

struct StreamFinalState {
    status: StatusCode,
    model: String,
    error: Option<String>,
    finish_reason: Option<String>,
    usage: Option<Value>,
}

impl StreamSpanFinalizer {
    fn new(root_span: Span, trace_id: String, model: String) -> Self {
        Self {
            root_span,
            trace_id,
            state: Arc::new(Mutex::new(StreamFinalState {
                status: StatusCode::OK,
                model,
                error: None,
                finish_reason: None,
                usage: None,
            })),
        }
    }

    fn set_model(&mut self, model: String) {
        if let Ok(mut state) = self.state.lock() {
            state.model = model;
        }
    }

    fn set_latest_usage(&mut self, usage: Value) {
        if let Ok(mut state) = self.state.lock() {
            state.usage = Some(usage);
        }
    }

    fn set_finish_reason(&mut self, finish_reason: Option<String>) {
        if let Ok(mut state) = self.state.lock() {
            state.finish_reason = finish_reason;
        }
    }

    fn set_failure(&mut self, model: String, error: String) {
        if let Ok(mut state) = self.state.lock() {
            state.status = StatusCode::INTERNAL_SERVER_ERROR;
            state.model = model;
            state.error = Some(error);
        }
    }

    fn set_success_if_unset(&mut self) {
        if let Ok(mut state) = self.state.lock() {
            if state.status == StatusCode::OK {
                state.error = None;
            }
        }
    }
}

impl Drop for StreamSpanFinalizer {
    fn drop(&mut self) {
        let Ok(state) = self.state.lock() else {
            return;
        };
        set_http_span_status(&self.root_span, state.status, state.error.as_deref());
        if state.status == StatusCode::OK {
            let usage = state.usage.as_ref();
            info!(
                "http.request.end; status_code=200 model_id={} finish_reason={} prompt_tokens={} completion_tokens={} trace_id={}",
                state.model,
                state.finish_reason.as_deref().unwrap_or(""),
                usage
                    .and_then(|value| {
                        parse_endpoint_usage_as_input_output(
                            "/chat/completions",
                            value,
                            Some(&state.model),
                            Some(&self.trace_id),
                        )
                    })
                    .map(|value| value.input_tokens)
                    .unwrap_or(0),
                usage
                    .and_then(|value| {
                        parse_endpoint_usage_as_input_output(
                            "/chat/completions",
                            value,
                            Some(&state.model),
                            Some(&self.trace_id),
                        )
                    })
                    .map(|value| value.output_tokens)
                    .unwrap_or(0),
                self.trace_id
            );
        } else {
            error!(
                "http.request.end; status_code={} model_id={} error={} trace_id={}",
                state.status.as_u16(),
                state.model,
                state.error.as_deref().unwrap_or("stream failed"),
                self.trace_id
            );
        }
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

pub fn map_usage_to_openai(usage: &serde_json::Value) -> Usage {
    map_usage(usage)
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

fn map_usage(usage: &serde_json::Value) -> Usage {
    if let Ok(openai_usage) = serde_json::from_value::<Usage>(usage.clone()) {
        return openai_usage;
    }
    parse_endpoint_usage_as_input_output("/chat/completions", usage, None, None)
        .map(|value| value.to_openai_usage())
        .unwrap_or(Usage {
            prompt_tokens: 0,
            completion_tokens: 0,
            total_tokens: 0,
            prompt_tokens_details: Some(crate::openai_types::PromptTokensDetails {
                audio_tokens: 0,
                cached_tokens: 0,
            }),
            completion_tokens_details: Some(crate::openai_types::CompletionTokensDetails {
                accepted_prediction_tokens: 0,
                audio_tokens: 0,
                reasoning_tokens: 0,
                rejected_prediction_tokens: 0,
            }),
        })
}

#[cfg(test)]
mod tests {
    use super::{map_openai_response, stream_openai_chunks};
    use crate::llm_provider::{FinishReason, UnifiedEvent, UnifiedResponse};
    use futures_util::StreamExt;
    use serde_json::Value;
    use tracing::Span;

    #[test]
    fn maps_non_streaming_created_timestamp() {
        let response = UnifiedResponse {
            request_id: "resp-1".to_string(),
            created_at: "2026-01-01T00:00:00Z".to_string(),
            model: "gpt-4.1".to_string(),
            output_text: "hello".to_string(),
            tool_calls: None,
            finish_reason: FinishReason::Stop,
            usage: serde_json::json!({
                "prompt_tokens": 1,
                "completion_tokens": 1,
                "total_tokens": 2
            }),
        };

        let mapped = map_openai_response(response);

        assert_eq!(mapped.created, Some(1767225600));
    }

    #[tokio::test]
    async fn suppresses_tool_call_done_after_delta_for_same_id() {
        let events = vec![
            Ok(UnifiedEvent::ResponseCreated {
                id: "req-1".to_string(),
                model: "m".to_string(),
                created_at: "2026-01-01T00:00:00Z".to_string(),
            }),
            Ok(UnifiedEvent::ToolCallDelta {
                id: "req-1-tool-0".to_string(),
                name: "client_ping".to_string(),
                arguments_delta: "{\"message\":\"hello\"}".to_string(),
            }),
            Ok(UnifiedEvent::ToolCallDone {
                id: "req-1-tool-0".to_string(),
                name: "client_ping".to_string(),
                arguments: "{\"message\":\"hello\"}".to_string(),
            }),
            Ok(UnifiedEvent::Completed {
                finish_reason: Some("tool_calls".to_string()),
                usage: Some(serde_json::json!({
                    "input_tokens": 1,
                    "output_tokens": 1,
                    "total_tokens": 2
                })),
            }),
        ];

        let stream = stream_openai_chunks(
            Box::pin(futures_util::stream::iter(events)),
            "trace-1".to_string(),
            "m".to_string(),
            Span::none(),
        );

        let mut payloads = Vec::new();
        futures_util::pin_mut!(stream);
        while let Some(item) = stream.next().await {
            payloads.push(item.expect("stream chunk"));
        }

        let mut tool_call_chunk_count = 0;
        for payload in payloads {
            let text = String::from_utf8(payload.to_vec()).expect("utf8");
            let Some(json) = text.strip_prefix("data: ") else {
                continue;
            };
            let json = json.trim();
            if json == "[DONE]" {
                continue;
            }
            let value: Value = serde_json::from_str(json).expect("valid json chunk");
            let has_tool_calls = value
                .get("choices")
                .and_then(Value::as_array)
                .and_then(|choices| choices.first())
                .and_then(|choice| choice.get("delta"))
                .and_then(|delta| delta.get("tool_calls"))
                .is_some();
            if has_tool_calls {
                tool_call_chunk_count += 1;
            }
        }

        assert_eq!(tool_call_chunk_count, 1);
    }

    #[tokio::test]
    async fn emits_tool_call_done_when_no_delta_seen() {
        let events = vec![
            Ok(UnifiedEvent::ResponseCreated {
                id: "req-1".to_string(),
                model: "m".to_string(),
                created_at: "2026-01-01T00:00:00Z".to_string(),
            }),
            Ok(UnifiedEvent::ToolCallDone {
                id: "req-1-tool-0".to_string(),
                name: "client_ping".to_string(),
                arguments: "{\"message\":\"hello\"}".to_string(),
            }),
            Ok(UnifiedEvent::Completed {
                finish_reason: Some("tool_calls".to_string()),
                usage: Some(serde_json::json!({
                    "input_tokens": 1,
                    "output_tokens": 1,
                    "total_tokens": 2
                })),
            }),
        ];

        let stream = stream_openai_chunks(
            Box::pin(futures_util::stream::iter(events)),
            "trace-1".to_string(),
            "m".to_string(),
            Span::none(),
        );

        let mut payloads = Vec::new();
        futures_util::pin_mut!(stream);
        while let Some(item) = stream.next().await {
            payloads.push(item.expect("stream chunk"));
        }

        let tool_call_chunk_count = payloads
            .into_iter()
            .filter_map(|payload| String::from_utf8(payload.to_vec()).ok())
            .filter_map(|text| text.strip_prefix("data: ").map(str::to_string))
            .filter(|json| json.trim() != "[DONE]")
            .filter_map(|json| serde_json::from_str::<Value>(json.trim()).ok())
            .filter(|value| {
                value
                    .get("choices")
                    .and_then(Value::as_array)
                    .and_then(|choices| choices.first())
                    .and_then(|choice| choice.get("delta"))
                    .and_then(|delta| delta.get("tool_calls"))
                    .is_some()
            })
            .count();

        assert_eq!(tool_call_chunk_count, 1);
    }
}

pub fn openai_error_response(
    status: StatusCode,
    message: &str,
    error_type: Option<&str>,
) -> Response {
    let response = ErrorResponse {
        error: ErrorDetail {
            message: message.to_string(),
            r#type: error_type.unwrap_or("internal_error").to_string(),
            code: None,
            param: None,
        },
    };
    let payload = serde_json::to_vec(&response).unwrap_or_else(|_| b"{}".to_vec());
    Response::builder()
        .status(status)
        .header(header::CONTENT_TYPE, "application/json")
        .body(Body::from(payload))
        .expect("build error response")
}

pub fn map_chat_error(err: ProviderError) -> Response {
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
            StatusCode::INTERNAL_SERVER_ERROR,
            "internal error",
            Some("internal_error"),
        ),
    }
}
