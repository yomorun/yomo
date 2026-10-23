//! `/chat/completions` API: parses requests and serializes responses.
//!
//! parse: native Chat request bytes -> `UnifiedRequest`; serialize: IR -> response
//! bytes (non-stream JSON / streaming SSE). Stream serialization reuses
//! `llm_stream_mapper` (IR events -> chat.completion.chunk SSE), keeping the
//! pluggable mapper semantics.

use crate::llm_stream_mapper::OpenAiSseStreamMapper;
use crate::openai_http_mapping::{map_openai_response, map_usage_to_openai, openai_error_response};
use crate::openai_types::ChatCompletionRequest;
use crate::provider::{
    UnifiedCandidate, UnifiedContentPart, UnifiedContentPartKind, UnifiedEnding, UnifiedInputItem,
    UnifiedInputItemKind, UnifiedMediaSource, UnifiedMessageBoundary, UnifiedRequestOptions,
    UnifiedResponseStatus, UnifiedRole, UnifiedSourceProtocol, UnifiedTool, UnifiedToolChoice,
    UnifiedToolKind, UnifiedToolOutput, merge_extensions, output_from_input, parse_role,
    response_envelope, role_name, source_extensions, value_string,
};
use crate::provider::{UnifiedRequest, UnifiedResponse};
use crate::usage_handler::EndpointUsage;
use axum::http::StatusCode;
use serde_json::{Value, json};

const PROTOCOL: UnifiedSourceProtocol = UnifiedSourceProtocol::ChatCompletions;

pub fn parse_content(value: &Value) -> Result<Vec<UnifiedContentPart>, String> {
    if let Some(text) = value.as_str() {
        return Ok(vec![UnifiedContentPart::text(text.into())]);
    }
    if value.is_null() {
        return Ok(vec![]);
    }
    value
        .as_array()
        .ok_or("content must be text or a block array")?
        .iter()
        .map(|block| {
            let kind = match block["type"].as_str().unwrap_or("") {
                "text" => UnifiedContentPartKind::Text {
                    text: value_string(block, "text").unwrap_or_default(),
                    annotations: vec![],
                    citations: None,
                    logprobs: None,
                },
                "image_url" => {
                    let url =
                        value_string(&block["image_url"], "url").ok_or("image_url requires url")?;
                    UnifiedContentPartKind::Image {
                        source: if url.starts_with("data:") {
                            UnifiedMediaSource::DataUrl { url }
                        } else {
                            UnifiedMediaSource::Url { url }
                        },
                        detail: value_string(&block["image_url"], "detail"),
                        transformations: None,
                    }
                }
                "file" => {
                    let file = block.get("file").unwrap_or(block);
                    UnifiedContentPartKind::File {
                        source: media_source(file)?,
                        filename: value_string(file, "filename"),
                        detail: None,
                    }
                }
                "input_audio" => UnifiedContentPartKind::Audio {
                    audio_type: "input_audio".into(),
                    payload: block.clone(),
                },
                kind => UnifiedContentPartKind::Other {
                    part_type: kind.into(),
                    payload: block.clone(),
                },
            };
            Ok(UnifiedContentPart {
                kind,
                cache_control: None,
                source_extensions: source_extensions(block, &["type", "text"], PROTOCOL),
            })
        })
        .collect()
}

fn media_source(value: &Value) -> Result<UnifiedMediaSource, String> {
    if let Some(file_id) = value_string(value, "file_id") {
        return Ok(UnifiedMediaSource::FileId { file_id });
    }
    if let Some(url) = value_string(value, "file_url") {
        return Ok(UnifiedMediaSource::Url { url });
    }
    let data =
        value_string(value, "file_data").ok_or("file requires file_id, file_url or file_data")?;
    Ok(if data.starts_with("data:") {
        UnifiedMediaSource::DataUrl { url: data }
    } else {
        UnifiedMediaSource::Base64 {
            data,
            media_type: value_string(value, "mime_type")
                .unwrap_or_else(|| "application/octet-stream".into()),
        }
    })
}

pub fn content_value(part: &UnifiedContentPart) -> Result<Value, String> {
    let mut value = match &part.kind {
        UnifiedContentPartKind::Text { text, .. } => json!({"type":"text","text":text}),
        UnifiedContentPartKind::Image { source, detail, .. } => {
            let url = match source {
                UnifiedMediaSource::Url { url } | UnifiedMediaSource::DataUrl { url } => {
                    url.clone()
                }
                UnifiedMediaSource::Base64 { data, media_type } => {
                    format!("data:{media_type};base64,{data}")
                }
                UnifiedMediaSource::FileId { .. } => {
                    return Err("Chat image inputs do not support file IDs".into());
                }
            };
            let mut value = json!({"type":"image_url","image_url":{"url":url}});
            if let Some(detail) = detail {
                value["image_url"]["detail"] = json!(detail);
            }
            value
        }
        UnifiedContentPartKind::File {
            source, filename, ..
        } => {
            let mut file = match source {
                UnifiedMediaSource::FileId { file_id } => json!({"file_id":file_id}),
                UnifiedMediaSource::Url { url } => json!({"file_url":url}),
                UnifiedMediaSource::DataUrl { url } => json!({"file_data":url}),
                UnifiedMediaSource::Base64 { data, media_type } => {
                    json!({"file_data":data,"mime_type":media_type})
                }
            };
            if let Some(filename) = filename {
                file["filename"] = json!(filename);
            }
            json!({"type":"file","file":file})
        }
        UnifiedContentPartKind::Audio { payload, .. }
        | UnifiedContentPartKind::Other { payload, .. } => payload.clone(),
        _ => return Err("content has no Chat representation".into()),
    };
    merge_extensions(&mut value, part.source_extensions.as_ref(), PROTOCOL);
    Ok(value)
}

pub fn request_from_value(value: &Value) -> Result<UnifiedRequest, String> {
    let mut request = UnifiedRequest {
        model: value_string(value, "model").unwrap_or_default(),
        ..Default::default()
    };
    for (index, message) in value["messages"]
        .as_array()
        .ok_or("messages is required")?
        .iter()
        .enumerate()
    {
        let role = parse_role(message["role"].as_str().ok_or("message requires role")?)?;
        if role == UnifiedRole::Tool {
            request.input.push(UnifiedInputItem::new(
                UnifiedInputItemKind::FunctionCallOutput {
                    call_id: value_string(message, "tool_call_id")
                        .filter(|id| !id.trim().is_empty())
                        .ok_or("tool_call_id is required for tool messages")?,
                    output: if let Some(text) = message["content"].as_str() {
                        UnifiedToolOutput::Text(text.into())
                    } else {
                        UnifiedToolOutput::Content(parse_content(&message["content"])?)
                    },
                    is_error: None,
                    cache_control: None,
                },
            ));
            continue;
        }
        let boundary = UnifiedMessageBoundary {
            message_id: format!("message-{index}"),
            role,
            block_index: None,
        };
        let mut content = parse_content(&message["content"])?;
        if let Some(text) = value_string(message, "reasoning_content")
            .or_else(|| value_string(message, "reasoning"))
        {
            content.insert(
                0,
                UnifiedContentPart {
                    kind: UnifiedContentPartKind::Thinking {
                        text,
                        signature: None,
                    },
                    cache_control: None,
                    source_extensions: None,
                },
            );
        }
        if let Some(text) = value_string(message, "refusal") {
            content.push(UnifiedContentPart {
                kind: UnifiedContentPartKind::Refusal { text },
                cache_control: None,
                source_extensions: None,
            });
        }
        let mut item = UnifiedInputItem::new(UnifiedInputItemKind::Message {
            role,
            content,
            phase: None,
        });
        item.message = Some(boundary.clone());
        item.source_extensions = source_extensions(
            message,
            &[
                "role",
                "content",
                "tool_calls",
                "reasoning_content",
                "reasoning",
                "refusal",
            ],
            PROTOCOL,
        );
        request.input.push(item);
        for call in message["tool_calls"].as_array().into_iter().flatten() {
            let mut item = UnifiedInputItem::new(UnifiedInputItemKind::FunctionCall {
                call_id: value_string(call, "id").ok_or("tool call requires id")?,
                name: value_string(&call["function"], "name").ok_or("tool name")?,
                arguments: value_string(&call["function"], "arguments").unwrap_or_default(),
                caller: None,
                cache_control: None,
            });
            item.message = Some(boundary.clone());
            request.input.push(item);
        }
    }
    request.tools = value["tools"].as_array().map(|tools| {
        tools
            .iter()
            .map(|tool| {
                let function = &tool["function"];
                UnifiedTool {
                    cache_control: None,
                    source_extensions: source_extensions(
                        function,
                        &["name", "description", "parameters", "strict"],
                        PROTOCOL,
                    ),
                    kind: UnifiedToolKind::Function {
                        name: value_string(function, "name").unwrap_or_default(),
                        description: value_string(function, "description"),
                        parameters: function
                            .get("parameters")
                            .cloned()
                            .unwrap_or_else(|| json!({})),
                        strict: function["strict"].as_bool(),
                        allowed_callers: None,
                    },
                }
            })
            .collect()
    });
    request.tool_choice = value
        .get("tool_choice")
        .map(|choice| match choice.as_str() {
            Some("auto") => UnifiedToolChoice::Auto,
            Some("none") => UnifiedToolChoice::None,
            Some("required") => UnifiedToolChoice::Required,
            _ => value_string(&choice["function"], "name")
                .map(UnifiedToolChoice::Function)
                .unwrap_or_else(|| UnifiedToolChoice::Other(choice.clone())),
        });
    let mut options = value.clone();
    options["max_output_tokens"] = value
        .get("max_completion_tokens")
        .or_else(|| value.get("max_tokens"))
        .cloned()
        .unwrap_or(Value::Null);
    options["stop_sequences"] = value
        .get("stop")
        .map(|stop| {
            if stop.is_string() {
                json!([stop])
            } else {
                stop.clone()
            }
        })
        .unwrap_or(Value::Null);
    request.options =
        serde_json::from_value::<UnifiedRequestOptions>(options).map_err(|err| err.to_string())?;
    request.options.normalize_reasoning_controls();
    request.controls.stream = value["stream"].as_bool();
    request.controls.stream_options = value.get("stream_options").cloned();
    request.controls.store = value["store"].as_bool();
    request.execution_context = value.get("agent_context").cloned();
    request.source_extensions = source_extensions(
        value,
        &[
            "model",
            "messages",
            "tools",
            "tool_choice",
            "max_completion_tokens",
            "max_tokens",
            "stop",
            "temperature",
            "top_p",
            "presence_penalty",
            "frequency_penalty",
            "n",
            "logprobs",
            "top_logprobs",
            "seed",
            "modalities",
            "audio",
            "prediction",
            "response_format",
            "reasoning_effort",
            "thinking",
            "chat_template_kwargs",
            "verbosity",
            "metadata",
            "service_tier",
            "stream",
            "stream_options",
            "store",
            "agent_context",
        ],
        PROTOCOL,
    );
    Ok(request)
}

pub fn request_to_value(request: &UnifiedRequest) -> Result<Value, String> {
    if request.context.previous_response_id.is_some() || request.context.conversation_id.is_some() {
        return Err("server-managed history references require Responses upstream".into());
    }
    let mut messages = Vec::<Value>::new();
    let mut owners = Vec::<Option<String>>::new();
    for item in request.instructions.iter().flatten().chain(&request.input) {
        let owner = item
            .message
            .as_ref()
            .map(|message| message.message_id.clone());
        match &item.kind {
            UnifiedInputItemKind::Message { role, content, .. } => {
                let mut parts = vec![];
                let mut reasoning = String::new();
                let mut refusal = None;
                for part in content {
                    match &part.kind {
                        UnifiedContentPartKind::Other { part_type, payload }
                            if part_type == "pending" && payload.is_null() => {}
                        UnifiedContentPartKind::Thinking { text, signature } => {
                            match signature.as_deref() {
                                // An empty signature carries no verification
                                // material and is treated as unsigned.
                                None | Some("") => reasoning.push_str(text),
                                Some(_) => {
                                    return Err(
                                        "signed thinking blocks have no Chat representation".into(),
                                    );
                                }
                            }
                        }
                        UnifiedContentPartKind::Refusal { text } => refusal = Some(text.clone()),
                        _ => parts.push(content_value(part)?),
                    }
                }
                let content = if parts.is_empty() {
                    Value::Null
                } else {
                    json!(parts)
                };
                // Chat upstreams have no developer role; developer
                // instructions encode as system messages, which these
                // endpoints accept in multiples.
                let encoded_role = match role {
                    UnifiedRole::Developer => "system",
                    other => role_name(*other),
                };
                let mut message = json!({"role":encoded_role,"content":content});
                if !reasoning.is_empty() {
                    message["reasoning_content"] = json!(reasoning);
                }
                if let Some(refusal) = refusal {
                    message["refusal"] = json!(refusal);
                }
                merge_extensions(&mut message, item.source_extensions.as_ref(), PROTOCOL);
                messages.push(message);
                owners.push(owner);
            }
            UnifiedInputItemKind::FunctionCall {
                call_id,
                name,
                arguments,
                ..
            } => {
                let call = json!({"id":call_id,"type":"function","function":{"name":name,"arguments":arguments}});
                if owner.is_some() && owners.last() == Some(&owner) {
                    let last = messages.last_mut().ok_or("tool call owner")?;
                    if !last["tool_calls"].is_array() {
                        last["tool_calls"] = json!([]);
                    }
                    last["tool_calls"]
                        .as_array_mut()
                        .ok_or("tool calls")?
                        .push(call);
                } else {
                    messages.push(json!({"role":"assistant","content":null,"tool_calls":[call]}));
                    owners.push(owner);
                }
            }
            UnifiedInputItemKind::FunctionCallOutput {
                call_id, output, ..
            } => {
                let output = match output {
                    UnifiedToolOutput::Missing => Value::Null,
                    UnifiedToolOutput::Text(text) => json!(text),
                    UnifiedToolOutput::Json(value) => json!(value.to_string()),
                    UnifiedToolOutput::Content(parts) => json!(
                        parts
                            .iter()
                            .map(content_value)
                            .collect::<Result<Vec<_>, _>>()?
                    ),
                };
                messages.push(json!({"role":"tool","tool_call_id":call_id,"content":output}));
                owners.push(None);
            }
            UnifiedInputItemKind::Reasoning {
                summary,
                content,
                encrypted_content,
            } => {
                if encrypted_content.is_some() {
                    return Err("encrypted reasoning has no Chat representation".into());
                }
                let mut reasoning = String::new();
                for part in content.iter().flatten() {
                    if let UnifiedContentPartKind::Thinking { text, .. } = &part.kind {
                        reasoning.push_str(text);
                    }
                }
                for entry in summary {
                    if let Some(text) = entry.get("text").and_then(Value::as_str) {
                        reasoning.push_str(text);
                    }
                }
                let mut message = json!({"role":"assistant","content":null});
                if !reasoning.is_empty() {
                    message["reasoning_content"] = json!(reasoning);
                }
                messages.push(message);
                owners.push(owner);
            }
            _ => return Err("input item has no Chat representation".into()),
        }
    }
    let mut value = serde_json::to_value(&request.options).map_err(|err| err.to_string())?;
    value
        .as_object_mut()
        .ok_or("options")?
        .retain(|_, value| !value.is_null());
    for key in [
        "max_output_tokens",
        "stop_sequences",
        "reasoning_summary",
        "reasoning_extensions",
        "top_k",
        "truncation",
    ] {
        value.as_object_mut().ok_or("options")?.remove(key);
    }
    value["model"] = json!(request.model);
    value["messages"] = json!(messages);
    if let Some(tokens) = request.options.max_output_tokens {
        value["max_completion_tokens"] = json!(tokens);
    }
    if let Some(stop) = &request.options.stop_sequences {
        value["stop"] = json!(stop);
    }
    if let Some(stream) = request.controls.stream {
        value["stream"] = json!(stream);
    }
    if let Some(options) = &request.controls.stream_options {
        value["stream_options"] = options.clone();
    }
    if let Some(store) = request.controls.store {
        value["store"] = json!(store);
    }
    if let Some(tools) = &request.tools {
        let mut native = vec![];
        for tool in tools {
            let UnifiedToolKind::Function {
                name,
                description,
                parameters,
                strict,
                ..
            } = &tool.kind
            else {
                return Err("server tool has no Chat representation".into());
            };
            let mut function = json!({"name":name,"parameters":parameters});
            if let Some(description) = description {
                function["description"] = json!(description);
            }
            if let Some(strict) = strict {
                function["strict"] = json!(strict);
            }
            merge_extensions(&mut function, tool.source_extensions.as_ref(), PROTOCOL);
            native.push(json!({"type":"function","function":function}));
        }
        value["tools"] = json!(native);
    }
    if let Some(choice) = &request.tool_choice {
        value["tool_choice"] = match choice {
            UnifiedToolChoice::Auto => json!("auto"),
            UnifiedToolChoice::None => json!("none"),
            UnifiedToolChoice::Required => json!("required"),
            UnifiedToolChoice::Function(name) => {
                json!({"type":"function","function":{"name":name}})
            }
            UnifiedToolChoice::Other(value) => value.clone(),
            UnifiedToolChoice::Allowed { mode, tools } => {
                json!({"type":"allowed_tools","mode":mode,"tools":tools})
            }
        };
    }
    merge_extensions(&mut value, request.source_extensions.as_ref(), PROTOCOL);
    Ok(value)
}

pub fn response_from_value(value: &Value) -> Result<UnifiedResponse, String> {
    let mut response = response_envelope(
        value,
        PROTOCOL,
        "/chat/completions",
        &["id", "model", "created", "choices", "usage"],
    )?;
    response.created_at = value["created"].as_i64();
    response.status = UnifiedResponseStatus::Completed;
    for choice in value["choices"].as_array().ok_or("missing choices")? {
        let index = choice["index"].as_u64().unwrap_or(0) as u32;
        let request =
            request_from_value(&json!({"model":response.model,"messages":[choice["message"]]}))?;
        let output = request
            .input
            .into_iter()
            .enumerate()
            .map(|(position, input)| {
                let mut item = output_from_input(
                    input,
                    format!("{}-choice-{index}-item-{position}", response.id),
                );
                if let Some(message) = &mut item.message {
                    message.message_id = format!("{}-choice-{index}", response.id);
                }
                item
            })
            .collect();
        response.candidates.push(UnifiedCandidate {
            index,
            output,
            ending: choice["finish_reason"]
                .as_str()
                .map(UnifiedEnding::from_source),
            logprobs: choice
                .get("logprobs")
                .filter(|value| !value.is_null())
                .cloned(),
            source_extensions: source_extensions(
                choice,
                &["index", "message", "finish_reason", "logprobs"],
                PROTOCOL,
            ),
        });
    }
    Ok(response)
}

/// API request parse failure (HTTP 400).
#[derive(Debug)]
pub struct OpenAiParseError {
    pub message: String,
}

impl OpenAiParseError {
    pub fn status(&self) -> StatusCode {
        StatusCode::BAD_REQUEST
    }
}

/// parse: request bytes -> IR.
pub fn parse_request(body: &[u8]) -> Result<UnifiedRequest, OpenAiParseError> {
    let mut request: ChatCompletionRequest =
        serde_json::from_slice(body).map_err(|err| OpenAiParseError {
            message: format!("invalid request body: {err}"),
        })?;
    request.normalize_reasoning_compat_fields();
    crate::dialects::openai_chat_completions::mapper::request_from_chat(&request).map_err(|err| {
        OpenAiParseError {
            message: err.to_string(),
        }
    })
}

/// serialize: non-stream IR response -> chat.completion JSON bytes.
pub fn serialize_non_stream(response: UnifiedResponse) -> Result<Vec<u8>, serde_json::Error> {
    let mapped = map_openai_response(response);
    serde_json::to_vec(&mapped)
}

/// usage -> OpenAI chat usage shape (billing/log view).
pub fn usage_to_openai(usage: &EndpointUsage) -> crate::openai_types::Usage {
    map_usage_to_openai(usage)
}

/// Render an API error response (re-exports openai_error_response).
pub fn error_response(
    status: StatusCode,
    message: &str,
    code: Option<&str>,
) -> axum::response::Response {
    openai_error_response(status, message, code)
}

/// Stream serialization: `OpenAiSseStreamMapper` (IR events -> SSE bytes).
pub fn stream_mapper() -> OpenAiSseStreamMapper {
    OpenAiSseStreamMapper
}

#[cfg(test)]
mod golden_tests {
    use super::*;
    use crate::openai_types::ErrorDetail;
    use crate::provider::{FinishReason, ToolCall, UnifiedEvent};
    use crate::provider::{
        UnifiedCandidate, UnifiedContentDelta, UnifiedEnding, UnifiedEventKind, UnifiedInputItem,
        UnifiedInputItemKind, UnifiedRole, UnifiedUsageUpdate,
    };
    use futures_util::StreamExt;

    #[test]
    fn developer_role_encodes_as_system_for_chat_upstreams() {
        let message = |role: UnifiedRole, text: &str| UnifiedInputItem {
            kind: UnifiedInputItemKind::Message {
                role,
                content: vec![UnifiedContentPart {
                    kind: UnifiedContentPartKind::Text {
                        text: text.into(),
                        annotations: vec![],
                        citations: None,
                        logprobs: None,
                    },
                    cache_control: None,
                    source_extensions: None,
                }],
                phase: None,
            },
            message: None,
            identity: None,
            status: None,
            source_extensions: None,
        };
        let mut request = UnifiedRequest::default();
        request.input = vec![
            message(UnifiedRole::Developer, "env: read-only"),
            message(UnifiedRole::Developer, "be concise"),
            message(UnifiedRole::User, "hi"),
        ];
        let value = request_to_value(&request).expect("encode");
        let messages = value["messages"].as_array().expect("messages");
        assert_eq!(messages[0]["role"], json!("system"));
        assert_eq!(messages[1]["role"], json!("system"));
        assert_eq!(messages[2]["role"], json!("user"));
        assert_eq!(messages[0]["content"][0]["text"], json!("env: read-only"));
    }

    #[test]
    fn assistant_thinking_with_empty_signature_round_trips_as_reasoning_content() {
        // The Messages protocol serializes unsigned thinking with an empty
        // signature string; the Chat encoder must treat it as unsigned.
        let request = UnifiedRequest::default();
        let thinking = |signature: Option<&str>| UnifiedContentPart {
            kind: UnifiedContentPartKind::Thinking {
                text: "plan".into(),
                signature: signature.map(str::to_owned),
            },
            cache_control: None,
            source_extensions: None,
        };
        let message = |parts: Vec<UnifiedContentPart>| UnifiedInputItem {
            kind: UnifiedInputItemKind::Message {
                role: UnifiedRole::Assistant,
                content: parts,
                phase: None,
            },
            message: None,
            identity: None,
            status: None,
            source_extensions: None,
        };
        let mut unsigned = request.clone();
        unsigned.input = vec![message(vec![thinking(None)])];
        let value = request_to_value(&unsigned).expect("unsigned thinking encodes");
        assert_eq!(value["messages"][0]["reasoning_content"], json!("plan"));
        let mut empty_signature = request;
        empty_signature.input = vec![message(vec![thinking(Some(""))])];
        let value = request_to_value(&empty_signature).expect("empty signature encodes");
        assert_eq!(value["messages"][0]["reasoning_content"], json!("plan"));
        let mut signed = empty_signature;
        signed.input = vec![message(vec![thinking(Some("opaque"))])];
        let error = request_to_value(&signed).expect_err("real signatures stay rejected");
        assert!(error.contains("signed thinking"));
    }

    #[test]
    fn reasoning_items_encode_as_assistant_reasoning_content_and_merge_tool_calls() {
        let reasoning = |encrypted: Option<&str>| UnifiedInputItem {
            kind: UnifiedInputItemKind::Reasoning {
                summary: vec![json!({"text": "plan step"})],
                content: None,
                encrypted_content: encrypted.map(str::to_owned),
            },
            message: Some(crate::provider::UnifiedMessageBoundary {
                message_id: "msg-1".into(),
                role: UnifiedRole::Assistant,
                block_index: None,
            }),
            identity: None,
            status: None,
            source_extensions: None,
        };
        let call = UnifiedInputItem {
            kind: UnifiedInputItemKind::FunctionCall {
                call_id: "call-1".into(),
                name: "read".into(),
                arguments: "{}".into(),
                caller: None,
                cache_control: None,
            },
            message: Some(crate::provider::UnifiedMessageBoundary {
                message_id: "msg-1".into(),
                role: UnifiedRole::Assistant,
                block_index: None,
            }),
            identity: None,
            status: None,
            source_extensions: None,
        };
        let mut request = UnifiedRequest::default();
        request.input = vec![reasoning(None), call];
        let value = request_to_value(&request).expect("reasoning item encodes");
        let messages = value["messages"].as_array().expect("messages");
        assert_eq!(
            messages.len(),
            1,
            "reasoning and tool call share one message"
        );
        assert_eq!(messages[0]["reasoning_content"], json!("plan step"));
        assert_eq!(messages[0]["tool_calls"][0]["id"], json!("call-1"));
        let mut encrypted = request;
        encrypted.input = vec![reasoning(Some("cipher"))];
        let error = request_to_value(&encrypted).expect_err("encrypted reasoning stays rejected");
        assert!(error.contains("encrypted reasoning"));
    }

    #[test]
    fn function_choice_and_reasoning_history_survive_conversion() {
        let raw = json!({"model":"m","messages":[{"role":"assistant","content":"text","reasoning_content":"reason","tool_calls":[{"id":"call","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call","content":"ok"}],"tool_choice":{"type":"function","function":{"name":"lookup"}},"n":2});
        let request = request_from_value(&raw).expect("request");
        let native = request_to_value(&request).expect("native");
        assert_eq!(native["messages"][0]["reasoning_content"], "reason");
        assert_eq!(
            native["messages"][0]["tool_calls"],
            raw["messages"][0]["tool_calls"]
        );
        assert_eq!(native["tool_choice"], raw["tool_choice"]);
        assert_eq!(native["n"], 2);
    }

    #[test]
    fn empty_assistant_content_serializes_as_null() {
        let request = UnifiedRequest {
            model: "m".into(),
            input: vec![UnifiedInputItem::new(UnifiedInputItemKind::Message {
                role: UnifiedRole::Assistant,
                content: vec![
                    UnifiedContentPart {
                        cache_control: None,
                        source_extensions: None,
                        kind: UnifiedContentPartKind::Other {
                            part_type: "pending".into(),
                            payload: Value::Null,
                        },
                    },
                    UnifiedContentPart {
                        cache_control: None,
                        source_extensions: None,
                        kind: UnifiedContentPartKind::Thinking {
                            text: "thought".into(),
                            signature: None,
                        },
                    },
                ],
                phase: None,
            })],
            ..Default::default()
        };

        let native = request_to_value(&request).expect("serialize request");
        assert_eq!(native["messages"][0]["content"], Value::Null);
        assert_eq!(native["messages"][0]["reasoning_content"], "thought");
        serde_json::from_value::<ChatCompletionRequest>(native).expect("deserialize chat request");
    }

    /// golden: request parse -> serialize -> deep Value comparison (zero known-field loss).
    #[test]
    fn request_parse_serialize_roundtrip() {
        let sample = r#"{
            "model": "gpt-5.5",
            "messages": [
                {"role": "system", "content": "be terse"},
                {"role": "user", "content": [
                    {"type": "text", "text": "hello"},
                    {"type": "image_url", "image_url": {"url": "data:image/png;base64,aGk="}}
                ]},
                {"role": "assistant", "content": "hi", "reasoning_content": "thinking"},
                {"role": "tool", "content": "ok", "tool_call_id": "call_1"}
            ],
            "temperature": 0.5,
            "top_p": 1.0,
            "max_completion_tokens": 512,
            "stop": ["END"],
            "tools": [{"type": "function", "function": {"name": "shell", "parameters": {"type": "object"}}}],
            "tool_choice": "auto",
            "stream": true,
            "stream_options": {"include_usage": true}
        }"#;

        let original: serde_json::Value = serde_json::from_str(sample).expect("sample");
        let request = parse_request(sample.as_bytes()).expect("parse");
        let mut roundtrip = super::super::request_to_value(
            &request,
            crate::provider::UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("serialize");
        // String content and a one-text-part array are equivalent native forms.
        for message in roundtrip["messages"].as_array_mut().expect("messages") {
            if let Some(parts) = message["content"].as_array() {
                if parts.len() == 1 && parts[0]["type"] == "text" {
                    message["content"] = parts[0]["text"].clone();
                }
            }
        }
        assert_eq!(original, roundtrip);
    }

    /// golden: non-stream IR -> chat.completion JSON snapshot.
    #[test]
    fn non_stream_response_golden() {
        let response = UnifiedResponse::from_completion("req-1".into(),"2026-01-01T00:00:00Z".into(),"upstream-model".into(),"hello".into(),Some("thinking".into()),Some(vec![ToolCall {
                id: Some("call_1".to_string()),
                name: "shell".to_string(),
                description: String::new(),
                arguments: r#"{"cmd":"ls"}"#.to_string(),
            }]),FinishReason::ToolCalls,EndpointUsage::from_endpoint_payload(
                "/chat/completions",
                serde_json::json!({"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}),
            )
            .expect("chat usage"));

        let bytes = serialize_non_stream(response).expect("serialize");
        let value: serde_json::Value = serde_json::from_slice(&bytes).expect("json");
        assert_eq!(
            value,
            serde_json::json!({
                "id": "req-1",
                "created": 1767225600,
                "model": "upstream-model",
                "object": "chat.completion",
                "system_fingerprint": null,
                "choices": [{
                    "message": {
                        "role": "assistant",
                        "content": "hello",
                        "reasoning_content": "thinking",
                        "annotations": [],
                        "refusal": null,
                        "tool_calls": [{
                            "id": "call_1",
                            "type": "function",
                            "function": {"name": "shell", "arguments": "{\"cmd\":\"ls\"}", "description": ""}
                        }]
                    },
                    "finish_reason": "tool_calls",
                    "index": 0,
                    "logprobs": null
                }],
                "usage": {
                    "prompt_tokens": 10,
                    "completion_tokens": 5,
                    "total_tokens": 15
                }
            })
        );
    }

    /// golden: IR event stream -> SSE bytes snapshot (via OpenAiSseStreamMapper).
    #[tokio::test]
    async fn stream_events_golden() {
        use futures_util::stream;

        let events: Vec<Result<UnifiedEvent, crate::provider::ProviderError>> = vec![
            Ok(UnifiedEvent::new(UnifiedEventKind::ResponseCreated { response:UnifiedResponse::new("req-1".into(),"m".into()) })),
            Ok(UnifiedEvent::new(UnifiedEventKind::ContentPartDelta { delta:UnifiedContentDelta::Text("he".into()) })),
            Ok(UnifiedEvent::new(UnifiedEventKind::ContentPartDelta { delta:UnifiedContentDelta::Text("llo".into()) })),
            Ok(UnifiedEvent::new(UnifiedEventKind::Usage {
                usage: EndpointUsage::from_endpoint_payload(
                    "/chat/completions",
                    serde_json::json!({"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}),
                )
                .expect("chat usage"),
                update:UnifiedUsageUpdate::Snapshot,source_usage:None,
            })),
            Ok({let mut response=UnifiedResponse::new("req-1".into(),"m".into());response.candidates.push(UnifiedCandidate { index:0,output:vec![],ending:Some(UnifiedEnding::from_source("stop")),logprobs:None,source_extensions:None });UnifiedEvent::new(UnifiedEventKind::Completed { response:Some(response) })}),
        ];
        let event_stream =
            Box::pin(stream::iter(events)) as crate::llm_stream_mapper::UnifiedEventStream;

        use crate::llm_stream_mapper::StreamChunkMapper;
        let bytes_stream = stream_mapper().map_stream(
            event_stream,
            "trace-1".to_string(),
            "m".to_string(),
            crate::serve_config::ResponseModelMode::Upstream,
            tracing::Span::none(),
        );
        let mut bytes_stream = bytes_stream;
        let mut sse = String::new();
        while let Some(chunk) = bytes_stream.next().await {
            let chunk = chunk.expect("chunk");
            sse.push_str(&String::from_utf8_lossy(&chunk));
        }

        // Stable snapshot: first chunk (role), content deltas, usage chunk, terminal [DONE].
        assert!(sse.starts_with("data: "));
        assert!(sse.contains("\"role\":\"assistant\""));
        assert!(sse.contains("\"content\":\"he\""));
        assert!(sse.contains("\"content\":\"llo\""));
        assert!(sse.contains(
            "\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}"
        ));
        assert!(sse.contains("\"finish_reason\":\"stop\""));
        assert!(sse.ends_with("data: [DONE]\n\n"));
    }

    #[test]
    fn parse_error_carries_400_and_message() {
        let err = parse_request(b"{invalid").expect_err("invalid json");
        assert_eq!(err.status(), StatusCode::BAD_REQUEST);
        assert!(err.message.contains("invalid request body"));

        // Skin error rendering: ErrorResponse shape
        let response = error_response(
            StatusCode::BAD_REQUEST,
            &err.message,
            Some("invalid_request_error"),
        );
        assert_eq!(response.status(), StatusCode::BAD_REQUEST);
    }

    #[test]
    fn unknown_field_error_still_carries_type_details() {
        // The API and the error policy render Public 400 identically.
        let detail = ErrorDetail {
            message: "boom".to_string(),
            r#type: "invalid_request_error".to_string(),
            code: None,
            param: None,
        };
        let payload = serde_json::to_vec(&crate::openai_types::ErrorResponse { error: detail })
            .expect("json");
        let value: serde_json::Value = serde_json::from_slice(&payload).expect("value");
        assert_eq!(value["error"]["type"], "invalid_request_error");
        assert_eq!(value["error"]["message"], "boom");
    }
}
