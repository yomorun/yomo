//! `/messages` API: Anthropic Messages parse / serialize / SSE state machine.
//!
//! parse: Anthropic native request -> IR; serialize: IR -> native
//! response and `event:`/`data:` SSE frames. tool_use <-> IR tool messages close
//! the loop in both directions.

use serde_json::Value;

use crate::anthropic_messages_types::{
    AnthropicMessagesResponse, AnthropicResponseContentBlock, AnthropicStreamEvent, AnthropicUsage,
};
use crate::provider::StopReason;
use crate::provider::{FinishReason, UnifiedEvent, UnifiedResponse};
use crate::provider::{
    UnifiedCacheControl, UnifiedCandidate, UnifiedContentPart, UnifiedDocumentSource,
    UnifiedInputItem, UnifiedInputItemKind, UnifiedMediaSource, UnifiedMessageBoundary,
    UnifiedOutputItem, UnifiedResponseStatus, UnifiedRole, UnifiedSourceProtocol, UnifiedTool,
    UnifiedToolChoice, UnifiedToolKind, UnifiedToolOutput, input_item, merge_extensions,
    output_from_input, parse_role, response_envelope, source_extensions, value_string,
};
use crate::provider::{
    UnifiedContentDelta, UnifiedContentPartKind, UnifiedEnding, UnifiedEventKind, UnifiedItemDelta,
    UnifiedOutputItemKind, UnifiedRequest,
};
use serde_json::json;

/// API request parse failure (HTTP 400).
#[derive(Debug)]
pub struct AnthropicParseError {
    pub message: String,
}

/// Error rendering: Anthropic error envelope (`{"type":"error","error":{...}}`).
pub fn error_response(
    status: axum::http::StatusCode,
    error_type: &str,
    message: &str,
) -> axum::response::Response {
    let payload = serde_json::json!({
        "type": "error",
        "error": {"type": error_type, "message": message}
    });
    axum::response::Response::builder()
        .status(status)
        .header("content-type", "application/json")
        .body(axum::body::Body::from(payload.to_string()))
        .expect("build error response")
}

/// parse: Anthropic Messages request bytes -> IR.
pub fn parse_request(body: &[u8]) -> Result<UnifiedRequest, AnthropicParseError> {
    let native: crate::anthropic_messages_types::AnthropicMessagesRequest =
        serde_json::from_slice(body).map_err(|err| AnthropicParseError {
            message: format!("invalid request body: {err}"),
        })?;
    let _ = native;
    let value = serde_json::from_slice(body).map_err(|err| AnthropicParseError {
        message: err.to_string(),
    })?;
    request_from_value(&value).map_err(|message| AnthropicParseError { message })
}

const PROTOCOL: UnifiedSourceProtocol = UnifiedSourceProtocol::Messages;

fn media_source(value: &Value) -> Result<UnifiedMediaSource, String> {
    match value["type"].as_str() {
        Some("file") => Ok(UnifiedMediaSource::FileId {
            file_id: value_string(value, "file_id").ok_or("file source requires file_id")?,
        }),
        Some("url") => Ok(UnifiedMediaSource::Url {
            url: value_string(value, "url").ok_or("URL source requires url")?,
        }),
        Some("base64") => Ok(UnifiedMediaSource::Base64 {
            data: value_string(value, "data").ok_or("base64 source requires data")?,
            media_type: value_string(value, "media_type")
                .ok_or("base64 source requires media_type")?,
        }),
        _ => Err("unsupported Messages media source".into()),
    }
}
fn source_value(source: &UnifiedMediaSource) -> Value {
    match source {
        UnifiedMediaSource::FileId { file_id } => json!({"type":"file","file_id":file_id}),
        UnifiedMediaSource::Url { url } => json!({"type":"url","url":url}),
        UnifiedMediaSource::Base64 { data, media_type } => {
            json!({"type":"base64","data":data,"media_type":media_type})
        }
        UnifiedMediaSource::DataUrl { url } => {
            let (metadata, data) = url.split_once(',').unwrap_or(("", ""));
            json!({"type":"base64","media_type":metadata.trim_start_matches("data:").trim_end_matches(";base64"),"data":data})
        }
    }
}

pub fn parse_content(value: &Value) -> Result<Vec<UnifiedContentPart>, String> {
    if let Some(text) = value.as_str() {
        return Ok(vec![UnifiedContentPart::text(text.into())]);
    }
    if value.is_null() {
        return Ok(vec![]);
    }
    value
        .as_array()
        .ok_or("content must be text or blocks")?
        .iter()
        .map(|block| {
            let kind = match block["type"].as_str().unwrap_or("") {
                "text" => UnifiedContentPartKind::Text {
                    text: value_string(block, "text").unwrap_or_default(),
                    annotations: vec![],
                    citations: block.get("citations").cloned(),
                    logprobs: None,
                },
                "thinking" => UnifiedContentPartKind::Thinking {
                    text: value_string(block, "thinking").unwrap_or_default(),
                    signature: value_string(block, "signature"),
                },
                "redacted_thinking" => UnifiedContentPartKind::RedactedThinking {
                    data: value_string(block, "data").unwrap_or_default(),
                },
                "image" => UnifiedContentPartKind::Image {
                    source: media_source(&block["source"])?,
                    detail: None,
                    transformations: block.get("transformations").cloned(),
                },
                "document" => {
                    let source = &block["source"];
                    let source = match source["type"].as_str() {
                        Some("text") => UnifiedDocumentSource::Text {
                            text: value_string(source, "data").unwrap_or_default(),
                            media_type: value_string(source, "media_type")
                                .unwrap_or_else(|| "text/plain".into()),
                        },
                        Some("content") => UnifiedDocumentSource::Content {
                            content: parse_content(&source["content"])?,
                        },
                        _ => UnifiedDocumentSource::Media {
                            source: media_source(source)?,
                        },
                    };
                    UnifiedContentPartKind::Document {
                        source,
                        title: value_string(block, "title"),
                        context: value_string(block, "context"),
                        citations: block.get("citations").cloned(),
                    }
                }
                kind => UnifiedContentPartKind::Other {
                    part_type: kind.into(),
                    payload: block.clone(),
                },
            };
            Ok(UnifiedContentPart {
                kind,
                cache_control: block
                    .get("cache_control")
                    .map(|value| UnifiedCacheControl::Messages(value.clone())),
                source_extensions: source_extensions(
                    block,
                    &[
                        "type",
                        "text",
                        "citations",
                        "thinking",
                        "signature",
                        "data",
                        "cache_control",
                        "transformations",
                        "title",
                        "context",
                    ],
                    PROTOCOL,
                ),
            })
        })
        .collect()
}

pub fn content_value(part: &UnifiedContentPart) -> Result<Value, String> {
    let mut value = match &part.kind {
        UnifiedContentPartKind::Text {
            text, citations, ..
        } => {
            let mut value = json!({"type":"text","text":text});
            if let Some(citations) = citations {
                value["citations"] = citations.clone();
            }
            value
        }
        UnifiedContentPartKind::Thinking { text, signature } => {
            json!({"type":"thinking","thinking":text,"signature":signature.as_deref().unwrap_or("")})
        }
        UnifiedContentPartKind::RedactedThinking { data } => {
            json!({"type":"redacted_thinking","data":data})
        }
        UnifiedContentPartKind::Image {
            source,
            transformations,
            ..
        } => {
            let mut value = json!({"type":"image","source":source_value(source)});
            if let Some(transformations) = transformations {
                value["transformations"] = transformations.clone();
            }
            value
        }
        UnifiedContentPartKind::File { source, .. } => {
            json!({"type":"document","source":source_value(source)})
        }
        UnifiedContentPartKind::Document {
            source,
            title,
            context,
            citations,
        } => {
            let source = match source {
                UnifiedDocumentSource::Media { source } => source_value(source),
                UnifiedDocumentSource::Text { text, media_type } => {
                    json!({"type":"text","data":text,"media_type":media_type})
                }
                UnifiedDocumentSource::Content { content } => {
                    json!({"type":"content","content":content.iter().map(content_value).collect::<Result<Vec<_>,_>>()?})
                }
            };
            let mut value = json!({"type":"document","source":source});
            if let Some(title) = title {
                value["title"] = json!(title);
            }
            if let Some(context) = context {
                value["context"] = json!(context);
            }
            if let Some(citations) = citations {
                value["citations"] = citations.clone();
            }
            value
        }
        UnifiedContentPartKind::Other { payload, .. } => payload.clone(),
        _ => return Err("content has no Messages representation".into()),
    };
    if let Some(UnifiedCacheControl::Messages(cache)) = &part.cache_control {
        value["cache_control"] = cache.clone();
    }
    merge_extensions(&mut value, part.source_extensions.as_ref(), PROTOCOL);
    Ok(value)
}

fn parse_output(value: &Value) -> Result<UnifiedToolOutput, String> {
    Ok(if value.is_null() {
        UnifiedToolOutput::Missing
    } else if let Some(text) = value.as_str() {
        UnifiedToolOutput::Text(text.into())
    } else if value.is_array() {
        UnifiedToolOutput::Content(parse_content(value)?)
    } else {
        UnifiedToolOutput::Json(value.clone())
    })
}
fn parse_block(block: &Value, role: UnifiedRole, index: usize) -> Result<UnifiedInputItem, String> {
    let cache = block
        .get("cache_control")
        .map(|value| UnifiedCacheControl::Messages(value.clone()));
    let kind = match block["type"].as_str() {
        Some("tool_use") => UnifiedInputItemKind::FunctionCall {
            call_id: value_string(block, "id").ok_or("tool_use requires id")?,
            name: value_string(block, "name").ok_or("tool_use requires name")?,
            arguments: block["input"].to_string(),
            caller: block.get("caller").cloned(),
            cache_control: cache,
        },
        Some("tool_result") => UnifiedInputItemKind::FunctionCallOutput {
            call_id: value_string(block, "tool_use_id")
                .ok_or("tool_result requires tool_use_id")?,
            output: parse_output(block.get("content").unwrap_or(&Value::Null))?,
            is_error: block["is_error"].as_bool(),
            cache_control: cache,
        },
        _ => {
            return Ok(UnifiedInputItem::new(UnifiedInputItemKind::Message {
                role,
                content: parse_content(&json!([block]))?,
                phase: None,
            }));
        }
    };
    Ok(input_item(
        block,
        kind,
        index,
        &[
            "type",
            "id",
            "name",
            "input",
            "caller",
            "cache_control",
            "tool_use_id",
            "content",
            "is_error",
        ],
        PROTOCOL,
    ))
}

pub fn request_from_value(value: &Value) -> Result<UnifiedRequest, String> {
    let mut request = UnifiedRequest {
        model: value_string(value, "model").unwrap_or_default(),
        ..Default::default()
    };
    if let Some(system) = value.get("system") {
        request.instructions = Some(vec![UnifiedInputItem::new(UnifiedInputItemKind::Message {
            role: UnifiedRole::System,
            content: parse_content(system)?,
            phase: None,
        })]);
    }
    for (index, message) in value["messages"]
        .as_array()
        .ok_or("messages is required")?
        .iter()
        .enumerate()
    {
        let role = parse_role(message["role"].as_str().ok_or("message requires role")?)?;
        if role == UnifiedRole::System {
            request
                .instructions
                .get_or_insert_with(Vec::new)
                .push(UnifiedInputItem::new(UnifiedInputItemKind::Message {
                    role: UnifiedRole::System,
                    content: parse_content(&message["content"])?,
                    phase: None,
                }));
            continue;
        }
        if !matches!(role, UnifiedRole::User | UnifiedRole::Assistant) {
            return Err("Messages roles must be user or assistant".into());
        }
        if let Some(blocks) = message["content"].as_array() {
            for (position, block) in blocks.iter().enumerate() {
                let mut item = parse_block(block, role, request.input.len())?;
                item.message = Some(UnifiedMessageBoundary {
                    message_id: format!("message-{index}"),
                    role,
                    block_index: Some(position as u32),
                });
                request.input.push(item);
            }
        } else {
            let mut item = UnifiedInputItem::new(UnifiedInputItemKind::Message {
                role,
                content: parse_content(&message["content"])?,
                phase: None,
            });
            item.message = Some(UnifiedMessageBoundary {
                message_id: format!("message-{index}"),
                role,
                block_index: None,
            });
            request.input.push(item);
        }
    }
    request.tools = value["tools"].as_array().map(|tools| {
        tools
            .iter()
            .map(|tool| UnifiedTool {
                cache_control: tool
                    .get("cache_control")
                    .map(|value| UnifiedCacheControl::Messages(value.clone())),
                source_extensions: source_extensions(
                    tool,
                    &[
                        "type",
                        "name",
                        "description",
                        "input_schema",
                        "strict",
                        "allowed_callers",
                        "cache_control",
                    ],
                    PROTOCOL,
                ),
                kind: if tool.get("type").is_some() {
                    UnifiedToolKind::Server {
                        tool_type: tool["type"].as_str().unwrap_or("").into(),
                        name: value_string(tool, "name"),
                        configuration: tool.clone(),
                    }
                } else {
                    UnifiedToolKind::Function {
                        name: value_string(tool, "name").unwrap_or_default(),
                        description: value_string(tool, "description"),
                        parameters: tool
                            .get("input_schema")
                            .cloned()
                            .unwrap_or_else(|| json!({})),
                        strict: tool["strict"].as_bool(),
                        allowed_callers: tool["allowed_callers"].as_array().map(|values| {
                            values
                                .iter()
                                .filter_map(Value::as_str)
                                .map(str::to_owned)
                                .collect()
                        }),
                    }
                },
            })
            .collect()
    });
    request.tool_choice = value
        .get("tool_choice")
        .map(|choice| match choice["type"].as_str() {
            Some("auto") => UnifiedToolChoice::Auto,
            Some("none") => UnifiedToolChoice::None,
            Some("any") => UnifiedToolChoice::Required,
            Some("tool") => {
                UnifiedToolChoice::Function(value_string(choice, "name").unwrap_or_default())
            }
            _ => UnifiedToolChoice::Other(choice.clone()),
        });
    request.options.temperature = value["temperature"].as_f64();
    request.options.top_p = value["top_p"].as_f64();
    request.options.top_k = value["top_k"].as_i64();
    request.options.max_output_tokens = value["max_tokens"].as_i64();
    request.options.stop_sequences = value["stop_sequences"].as_array().map(|values| {
        values
            .iter()
            .filter_map(Value::as_str)
            .map(str::to_owned)
            .collect()
    });
    request.options.parallel_tool_calls = value["tool_choice"]["disable_parallel_tool_use"]
        .as_bool()
        .map(|disable| !disable);
    request.options.thinking = value.get("thinking").cloned();
    request.options.reasoning_effort = value["output_config"]["effort"].as_str().map(str::to_owned);
    request.options.response_format = value["output_config"].get("format").cloned();
    request.options.metadata = value.get("metadata").cloned();
    request.options.service_tier = value_string(value, "service_tier");
    request.controls.stream = value["stream"].as_bool();
    request.source_extensions = source_extensions(
        value,
        &[
            "model",
            "system",
            "messages",
            "tools",
            "tool_choice",
            "temperature",
            "top_p",
            "top_k",
            "max_tokens",
            "stop_sequences",
            "thinking",
            "metadata",
            "service_tier",
            "stream",
        ],
        PROTOCOL,
    );
    Ok(request)
}

fn item_value(item: &UnifiedInputItem) -> Result<Value, String> {
    let mut value = match &item.kind {
        UnifiedInputItemKind::Message { content, .. } => json!(
            content
                .iter()
                .map(content_value)
                .collect::<Result<Vec<_>, _>>()?
        ),
        UnifiedInputItemKind::FunctionCall {
            call_id,
            name,
            arguments,
            caller,
            cache_control,
        } => {
            let input: Value = serde_json::from_str(arguments)
                .map_err(|err| format!("invalid tool arguments: {err}"))?;
            let mut value = json!({"type":"tool_use","id":call_id,"name":name,"input":input});
            if let Some(caller) = caller {
                value["caller"] = caller.clone();
            }
            if let Some(UnifiedCacheControl::Messages(cache)) = cache_control {
                value["cache_control"] = cache.clone();
            }
            value
        }
        UnifiedInputItemKind::FunctionCallOutput {
            call_id,
            output,
            is_error,
            cache_control,
        } => {
            let mut value = json!({"type":"tool_result","tool_use_id":call_id});
            match output {
                UnifiedToolOutput::Missing => {}
                UnifiedToolOutput::Text(text) => value["content"] = json!(text),
                UnifiedToolOutput::Json(output) => value["content"] = output.clone(),
                UnifiedToolOutput::Content(parts) => {
                    value["content"] = json!(
                        parts
                            .iter()
                            .map(content_value)
                            .collect::<Result<Vec<_>, _>>()?
                    )
                }
            }
            if let Some(is_error) = is_error {
                value["is_error"] = json!(is_error);
            }
            if let Some(UnifiedCacheControl::Messages(cache)) = cache_control {
                value["cache_control"] = cache.clone();
            }
            value
        }
        UnifiedInputItemKind::Other { payload, .. } => payload.clone(),
        _ => return Err("input item has no Messages representation".into()),
    };
    merge_extensions(&mut value, item.source_extensions.as_ref(), PROTOCOL);
    Ok(value)
}

pub fn request_to_value(request: &UnifiedRequest) -> Result<Value, String> {
    if request.context.previous_response_id.is_some() || request.context.conversation_id.is_some() {
        return Err("server-managed history references require Responses upstream".into());
    }
    let mut value = json!({"model":request.model});
    let mut messages = Vec::<Value>::new();
    let mut owners = Vec::<Option<String>>::new();
    let mut system = vec![];
    for item in request.instructions.iter().flatten().chain(&request.input) {
        if let UnifiedInputItemKind::Message {
            role: UnifiedRole::System | UnifiedRole::Developer,
            content,
            ..
        } = &item.kind
        {
            system.extend(
                content
                    .iter()
                    .map(content_value)
                    .collect::<Result<Vec<_>, _>>()?,
            );
            continue;
        }
        let native = item_value(item)?;
        let blocks = if native.is_array() {
            native.as_array().cloned().unwrap_or_default()
        } else {
            vec![native]
        };
        let role = item
            .message
            .as_ref()
            .map(|message| message.role)
            .unwrap_or(match item.kind {
                UnifiedInputItemKind::FunctionCallOutput { .. } => UnifiedRole::User,
                UnifiedInputItemKind::Message { role, .. } => role,
                _ => UnifiedRole::Assistant,
            });
        let owner = item
            .message
            .as_ref()
            .map(|message| message.message_id.clone());
        if owner.is_some() && owners.last() == Some(&owner) {
            messages.last_mut().ok_or("message owner")?["content"]
                .as_array_mut()
                .ok_or("blocks")?
                .extend(blocks);
        } else {
            messages.push(json!({"role":crate::provider::role_name(role),"content":blocks}));
            owners.push(owner);
        }
    }
    value["messages"] = json!(messages);
    if !system.is_empty() {
        value["system"] = json!(system);
    }
    if let Some(tools) = &request.tools {
        value["tools"] = json!(
            tools
                .iter()
                .map(|tool| {
                    let mut value = match &tool.kind {
                        UnifiedToolKind::Function {
                            name,
                            description,
                            parameters,
                            strict,
                            allowed_callers,
                        } => {
                            let mut value = json!({"name":name,"input_schema":parameters});
                            if let Some(description) = description {
                                value["description"] = json!(description);
                            }
                            if let Some(strict) = strict {
                                value["strict"] = json!(strict);
                            }
                            if let Some(callers) = allowed_callers {
                                value["allowed_callers"] = json!(callers);
                            }
                            value
                        }
                        UnifiedToolKind::Server { configuration, .. }
                        | UnifiedToolKind::Other {
                            payload: configuration,
                            ..
                        } => configuration.clone(),
                    };
                    if let Some(UnifiedCacheControl::Messages(cache)) = &tool.cache_control {
                        value["cache_control"] = cache.clone();
                    }
                    merge_extensions(&mut value, tool.source_extensions.as_ref(), PROTOCOL);
                    value
                })
                .collect::<Vec<_>>()
        );
    }
    if let Some(choice) = &request.tool_choice {
        value["tool_choice"] = match choice {
            UnifiedToolChoice::Auto => json!({"type":"auto"}),
            UnifiedToolChoice::None => json!({"type":"none"}),
            UnifiedToolChoice::Required => json!({"type":"any"}),
            UnifiedToolChoice::Function(name) => json!({"type":"tool","name":name}),
            UnifiedToolChoice::Other(raw) => raw.clone(),
            _ => return Err("tool selection has no Messages representation".into()),
        };
        if let Some(parallel) = request.options.parallel_tool_calls {
            value["tool_choice"]["disable_parallel_tool_use"] = json!(!parallel);
        }
    }
    for (key, option) in [
        ("temperature", json!(request.options.temperature)),
        ("top_p", json!(request.options.top_p)),
        ("top_k", json!(request.options.top_k)),
        ("max_tokens", json!(request.options.max_output_tokens)),
        ("stop_sequences", json!(request.options.stop_sequences)),
        ("thinking", json!(request.options.thinking)),
        ("metadata", json!(request.options.metadata)),
        ("service_tier", json!(request.options.service_tier)),
        ("stream", json!(request.controls.stream)),
    ] {
        if !option.is_null() {
            value[key] = option;
        }
    }
    merge_extensions(&mut value, request.source_extensions.as_ref(), PROTOCOL);
    if let Some(effort) = &request.options.reasoning_effort {
        value["output_config"]["effort"] = json!(effort);
    }
    if let Some(format) = &request.options.response_format {
        value["output_config"]["format"] = format.clone();
    }
    Ok(value)
}

pub fn output_item_value(item: &UnifiedOutputItem) -> Result<Value, String> {
    item_value(&item.as_input())
}
pub fn response_from_value(value: &Value) -> Result<UnifiedResponse, String> {
    let mut response = response_envelope(
        value,
        PROTOCOL,
        "/messages",
        &[
            "id",
            "model",
            "type",
            "role",
            "content",
            "stop_reason",
            "stop_sequence",
            "usage",
        ],
    )?;
    response.status = UnifiedResponseStatus::Completed;
    let mut output = vec![];
    for (index, block) in value["content"]
        .as_array()
        .ok_or("response content is required")?
        .iter()
        .enumerate()
    {
        let mut input = parse_block(block, UnifiedRole::Assistant, index)?;
        input.message = Some(UnifiedMessageBoundary {
            message_id: response.id.clone(),
            role: UnifiedRole::Assistant,
            block_index: Some(index as u32),
        });
        output.push(output_from_input(
            input,
            format!("{}-item-{index}", response.id),
        ));
    }
    let ending = value["stop_reason"].as_str().map(|reason| {
        let mut ending = UnifiedEnding::from_source(reason);
        ending.stop_sequence = value_string(value, "stop_sequence");
        ending
    });
    response.candidates.push(UnifiedCandidate {
        index: 0,
        output,
        ending,
        logprobs: None,
        source_extensions: None,
    });
    Ok(response)
}

// ---------------------------------------------------------------------------
// serialize
// ---------------------------------------------------------------------------

/// IR unified response -> Anthropic Messages non-streaming response body.
pub fn serialize_non_stream(response: &UnifiedResponse) -> Value {
    let native = unified_to_anthropic_response(response);
    serde_json::to_value(&native).unwrap_or(Value::Null)
}

pub fn unified_to_anthropic_response(response: &UnifiedResponse) -> AnthropicMessagesResponse {
    let mut content = Vec::new();
    for item in response
        .candidates
        .first()
        .into_iter()
        .flat_map(|candidate| &candidate.output)
    {
        match &item.kind {
            UnifiedOutputItemKind::Message { content: parts, .. } => {
                for part in parts {
                    if let Ok(value) = content_value(part) {
                        if let Ok(block) = serde_json::from_value(value) {
                            content.push(block);
                        }
                    }
                }
            }
            UnifiedOutputItemKind::FunctionCall {
                call_id,
                name,
                arguments,
                caller,
            } => content.push(AnthropicResponseContentBlock::ToolUse {
                id: call_id.clone(),
                name: name.clone(),
                input: serde_json::from_str(arguments).unwrap_or(Value::Object(Default::default())),
                caller: caller.clone(),
                extra: Default::default(),
            }),
            _ => {}
        }
    }
    let ending = response
        .candidates
        .first()
        .and_then(|candidate| candidate.ending.as_ref());
    let stop_reason = ending
        .map(UnifiedEnding::to_messages)
        .map(str::to_owned)
        .unwrap_or_else(|| anthropic_stop_reason(&FinishReason::Stop, None));
    let stop_sequence = ending.and_then(|ending| ending.stop_sequence.clone());
    let usage = response
        .usage
        .as_ref()
        .map(map_usage_to_anthropic)
        .unwrap_or_default();

    AnthropicMessagesResponse {
        id: response.id.clone(),
        r#type: Some("message".to_string()),
        role: "assistant".to_string(),
        model: response.model.clone(),
        content,
        stop_reason: Some(stop_reason),
        stop_sequence,
        usage,
        extra: Default::default(),
    }
}

fn anthropic_stop_reason(finish: &FinishReason, stop_sequence: Option<&str>) -> String {
    let reason = StopReason::from_openai(match finish {
        FinishReason::Stop => "stop",
        FinishReason::Length => "length",
        FinishReason::ToolCalls => "tool_calls",
        FinishReason::ContentFilter => "content_filter",
        FinishReason::Other => "other",
    });
    // Preserve the anthropic stop_sequence semantics when a stop sequence was hit.
    if stop_sequence.is_some() {
        return "stop_sequence".to_string();
    }
    reason.to_anthropic().to_string()
}

fn map_usage_to_anthropic(usage: &crate::usage_handler::EndpointUsage) -> AnthropicUsage {
    let openai = crate::openai_http_mapping::map_usage_to_openai(usage);
    AnthropicUsage {
        input_tokens: Some(openai.prompt_tokens),
        output_tokens: Some(openai.completion_tokens),
        ..Default::default()
    }
}

// ---------------------------------------------------------------------------
// Streaming SSE state machine
// ---------------------------------------------------------------------------

/// IR stream events -> Anthropic SSE frame state machine.
///
/// Tracks content block lifecycle: one each of text / thinking / tool_use;
/// emits `content_block_start` before the first delta and `content_block_stop` on close.
#[derive(Default)]
pub struct AnthropicSseState {
    message_started: bool,
    request_id: String,
    model: String,
    /// (block index, kind, tool_use_id)
    open_blocks: Vec<(i64, BlockKind, String)>,
    next_index: i64,
    usage: Option<AnthropicUsage>,
    finished: bool,
    tool_items: std::collections::HashMap<String, crate::provider::UnifiedOutputItem>,
    stop_reason: Option<String>,
}

#[derive(Debug, Clone, PartialEq)]
enum BlockKind {
    Text,
    Thinking,
    ToolUse,
}

impl AnthropicSseState {
    pub fn new() -> Self {
        Self::default()
    }

    fn frame(event: &AnthropicStreamEvent) -> String {
        let event_name = match event {
            AnthropicStreamEvent::MessageStart { .. } => "message_start",
            AnthropicStreamEvent::MessageDelta { .. } => "message_delta",
            AnthropicStreamEvent::MessageStop => "message_stop",
            AnthropicStreamEvent::ContentBlockStart { .. } => "content_block_start",
            AnthropicStreamEvent::ContentBlockDelta { .. } => "content_block_delta",
            AnthropicStreamEvent::ContentBlockStop { .. } => "content_block_stop",
            AnthropicStreamEvent::Ping => "ping",
            AnthropicStreamEvent::Error { .. } => "error",
            AnthropicStreamEvent::Unknown => "unknown",
        };
        let data = serde_json::to_string(event).unwrap_or_else(|_| "{}".to_string());
        format!("event: {event_name}\ndata: {data}\n\n")
    }

    /// Return the open block of the given kind, opening a new one if absent. Returns (index, newly_opened).
    fn ensure_block(&mut self, kind: BlockKind, tool_use_id: &str) -> (i64, bool) {
        if let Some((index, _, _)) = self
            .open_blocks
            .iter()
            .find(|(_, k, id)| *k == kind && id == tool_use_id)
        {
            return (*index, false);
        }
        let index = self.next_index;
        self.next_index += 1;
        self.open_blocks
            .push((index, kind, tool_use_id.to_string()));
        (index, true)
    }

    fn close_block(&mut self, kind: &BlockKind, tool_use_id: &str) -> Option<i64> {
        let position = self
            .open_blocks
            .iter()
            .position(|(_, k, id)| k == kind && id == tool_use_id)?;
        let (index, _, _) = self.open_blocks.remove(position);
        Some(index)
    }

    fn message_start_frame(&self, id: &str, model: &str) -> String {
        Self::frame(&AnthropicStreamEvent::MessageStart {
            message: AnthropicMessagesResponse {
                id: id.to_string(),
                r#type: Some("message".to_string()),
                role: "assistant".to_string(),
                model: model.to_string(),
                content: Vec::new(),
                stop_reason: None,
                stop_sequence: None,
                usage: AnthropicUsage::default(),
                extra: Default::default(),
            },
        })
    }

    /// Process one IR event, producing 0..n SSE frames.
    pub fn on_event(&mut self, event: &UnifiedEvent) -> Vec<String> {
        if self.finished {
            return Vec::new();
        }
        let mut frames = Vec::new();

        let id = event
            .response_id
            .clone()
            .or_else(|| {
                event
                    .target
                    .as_ref()
                    .and_then(|target| target.item_id.clone())
            })
            .unwrap_or_default();
        match &event.kind {
            UnifiedEventKind::ResponseCreated { response }
            | UnifiedEventKind::ResponseInProgress {
                response: Some(response),
            } => {
                let id = &response.id;
                let model = &response.model;
                if !self.message_started {
                    self.message_started = true;
                    self.request_id = id.clone();
                    self.model = model.clone();
                    frames.push(self.message_start_frame(id, model));
                }
            }
            UnifiedEventKind::OutputItemAdded { item }
                if matches!(item.kind, UnifiedOutputItemKind::Message { .. }) =>
            {
                let id = &item.identity.internal_id;
                if !self.message_started {
                    self.message_started = true;
                    self.request_id = id.clone();
                    frames.push(self.message_start_frame(id, &self.model));
                }
            }
            UnifiedEventKind::ContentPartDelta {
                delta: UnifiedContentDelta::Text(delta),
            }
            | UnifiedEventKind::ContentPartDelta {
                delta: UnifiedContentDelta::TextWithLogprobs { text: delta, .. },
            } => {
                let (index, opened) = self.ensure_block(BlockKind::Text, "");
                if opened {
                    frames.push(Self::frame(&AnthropicStreamEvent::ContentBlockStart {
                        index,
                        content_block: AnthropicResponseContentBlock::Text {
                            text: String::new(),
                            citations: None,
                            extra: Default::default(),
                        },
                        extra: Default::default(),
                    }));
                }
                frames.push(Self::frame(&AnthropicStreamEvent::ContentBlockDelta {
                    index,
                    delta: crate::anthropic_messages_types::AnthropicContentDelta::TextDelta {
                        text: delta.clone(),
                    },
                    extra: Default::default(),
                }));
            }
            UnifiedEventKind::ContentPartDelta {
                delta: UnifiedContentDelta::Thinking(delta),
            }
            | UnifiedEventKind::ReasoningSummaryTextDelta { delta } => {
                let (index, opened) = self.ensure_block(BlockKind::Thinking, "");
                if opened {
                    frames.push(Self::frame(&AnthropicStreamEvent::ContentBlockStart {
                        index,
                        content_block: AnthropicResponseContentBlock::Thinking {
                            thinking: String::new(),
                            signature: String::new(),
                            extra: Default::default(),
                        },
                        extra: Default::default(),
                    }));
                }
                frames.push(Self::frame(&AnthropicStreamEvent::ContentBlockDelta {
                    index,
                    delta: crate::anthropic_messages_types::AnthropicContentDelta::ThinkingDelta {
                        thinking: delta.clone(),
                    },
                    extra: Default::default(),
                }));
            }
            UnifiedEventKind::OutputItemAdded { item }
                if matches!(item.kind, UnifiedOutputItemKind::FunctionCall { .. }) =>
            {
                self.tool_items
                    .insert(item.identity.internal_id.clone(), item.clone());
            }
            UnifiedEventKind::ItemDelta {
                delta: UnifiedItemDelta::FunctionArguments(arguments_delta),
            } => {
                let item_id = event
                    .target
                    .as_ref()
                    .and_then(|target| target.item_id.as_ref());
                let item = item_id.and_then(|id| self.tool_items.get(id));
                let (id, name) = item
                    .and_then(|item| match &item.kind {
                        UnifiedOutputItemKind::FunctionCall { call_id, name, .. } => {
                            Some((call_id.clone(), name.clone()))
                        }
                        _ => None,
                    })
                    .unwrap_or((id.clone(), String::new()));
                let (index, opened) = self.ensure_block(BlockKind::ToolUse, &id);
                if opened {
                    frames.push(Self::frame(&AnthropicStreamEvent::ContentBlockStart {
                        index,
                        content_block: AnthropicResponseContentBlock::ToolUse {
                            id: id.to_string(),
                            name: name.to_string(),
                            input: Value::Object(Default::default()),
                            caller: None,
                            extra: Default::default(),
                        },
                        extra: Default::default(),
                    }));
                }
                frames.push(Self::frame(&AnthropicStreamEvent::ContentBlockDelta {
                    index,
                    delta: crate::anthropic_messages_types::AnthropicContentDelta::InputJsonDelta {
                        partial_json: arguments_delta.clone(),
                    },
                    extra: Default::default(),
                }));
            }
            UnifiedEventKind::OutputItemDone { item }
                if matches!(item.kind, UnifiedOutputItemKind::FunctionCall { .. }) =>
            {
                let UnifiedOutputItemKind::FunctionCall { call_id: id, .. } = &item.kind else {
                    unreachable!()
                };
                if let Some(index) = self.close_block(&BlockKind::ToolUse, id) {
                    frames.push(Self::frame(&AnthropicStreamEvent::ContentBlockStop {
                        index,
                        extra: Default::default(),
                    }));
                }
            }
            UnifiedEventKind::ContentPartDone { part }
                if matches!(part.kind, UnifiedContentPartKind::Thinking { .. }) =>
            {
                if let Some(index) = self.close_block(&BlockKind::Thinking, "") {
                    frames.push(Self::frame(&AnthropicStreamEvent::ContentBlockStop {
                        index,
                        extra: Default::default(),
                    }));
                }
            }
            UnifiedEventKind::Usage { usage, .. } => {
                self.usage = Some(map_usage_to_anthropic(usage));
            }
            UnifiedEventKind::CandidateDone { ending } => {
                self.stop_reason = Some(ending.to_messages().to_string());
            }
            UnifiedEventKind::Completed { response }
            | UnifiedEventKind::Incomplete { response, .. } => {
                frames.extend(self.close_all_blocks());
                let stop_reason = response
                    .as_ref()
                    .and_then(|response| response.candidates.first())
                    .and_then(|candidate| candidate.ending.as_ref())
                    .map(|ending| ending.to_messages().to_string())
                    .or_else(|| self.stop_reason.clone())
                    .unwrap_or_else(|| "end_turn".to_string());
                frames.push(Self::frame(&AnthropicStreamEvent::MessageDelta {
                    delta: crate::anthropic_messages_types::AnthropicMessageDeltaBody {
                        stop_reason: Some(stop_reason),
                        stop_sequence: None,
                    },
                    usage: self.usage.clone(),
                    extra: Default::default(),
                }));
                frames.push(Self::frame(&AnthropicStreamEvent::MessageStop));
                self.finished = true;
            }
            UnifiedEventKind::Failed { error, .. } => {
                frames.push(Self::frame(&AnthropicStreamEvent::Error {
                    error: crate::anthropic_messages_types::AnthropicErrorBody {
                        r#type: error.code.clone(),
                        message: error.message.clone(),
                    },
                }));
                self.finished = true;
            }
            _ => {}
        }
        frames
    }

    fn close_all_blocks(&mut self) -> Vec<String> {
        let open = std::mem::take(&mut self.open_blocks);
        open.iter()
            .map(|(index, _, _)| {
                Self::frame(&AnthropicStreamEvent::ContentBlockStop {
                    index: *index,
                    extra: Default::default(),
                })
            })
            .collect()
    }

    /// Mid-stream error frame (with the `event: error` header).
    pub fn error_frame(error_type: &str, message: &str) -> String {
        Self::frame(&AnthropicStreamEvent::Error {
            error: crate::anthropic_messages_types::AnthropicErrorBody {
                r#type: Some(error_type.to_string()),
                message: message.to_string(),
            },
        })
    }

    /// Stream teardown fallback: emits closing frames when message_stop was not sent.
    pub fn finish(&mut self) -> Vec<String> {
        if self.finished {
            return Vec::new();
        }
        let mut frames = self.close_all_blocks();
        frames.push(Self::frame(&AnthropicStreamEvent::MessageDelta {
            delta: crate::anthropic_messages_types::AnthropicMessageDeltaBody {
                stop_reason: Some("end_turn".to_string()),
                stop_sequence: None,
            },
            usage: self.usage.clone(),
            extra: Default::default(),
        }));
        frames.push(Self::frame(&AnthropicStreamEvent::MessageStop));
        self.finished = true;
        frames
    }
}

/// Tool loop: IR tool_calls -> native tool_use content blocks (reused by serialize).
pub fn tool_calls_to_anthropic_content(
    calls: &[crate::provider::ToolCall],
) -> Vec<AnthropicResponseContentBlock> {
    calls
        .iter()
        .map(|call| AnthropicResponseContentBlock::ToolUse {
            id: call.id.clone().unwrap_or_default(),
            name: call.name.clone(),
            input: serde_json::from_str(&call.arguments)
                .unwrap_or(Value::Object(Default::default())),
            caller: None,
            extra: Default::default(),
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_request_moves_system_role_messages_into_instructions() {
        let request = request_from_value(&json!({
            "model":"claude-haiku-5-5",
            "system":"base policy",
            "messages":[
                {"role":"system","content":[{"type":"text","text":"session context"}]},
                {"role":"user","content":"do the task"}
            ]
        }))
        .expect("system role message should be accepted as instruction");

        let instructions = request.instructions.as_ref().expect("instructions");
        assert_eq!(instructions.len(), 2);
        assert!(instructions.iter().all(|item| matches!(
            &item.kind,
            UnifiedInputItemKind::Message {
                role: UnifiedRole::System,
                ..
            }
        )));
        assert_eq!(request.input.len(), 1);
        assert!(matches!(
            &request.input[0].kind,
            UnifiedInputItemKind::Message {
                role: UnifiedRole::User,
                ..
            }
        ));
    }

    #[test]
    fn output_config_effort_is_visible_to_capabilities_and_native_format_is_preserved() {
        let raw = json!({"model":"claude","max_tokens":8192,
            "messages":[{"role":"user","content":"hello"}],
            "output_config":{"effort":"max","format":{"type":"json_schema","schema":{"type":"object"}}}
        });
        let request = request_from_value(&raw).expect("request");
        assert_eq!(request.options.reasoning_effort.as_deref(), Some("max"));
        assert_eq!(
            request_to_value(&request).expect("native")["output_config"],
            raw["output_config"]
        );
    }

    #[test]
    fn text_with_logprobs_is_emitted_as_messages_text() {
        let mut state = AnthropicSseState::new();
        let frames = state
            .on_event(&UnifiedEvent::new(UnifiedEventKind::ContentPartDelta {
                delta: UnifiedContentDelta::TextWithLogprobs {
                    text: "answer".into(),
                    logprobs: vec![],
                },
            }))
            .join("");
        assert!(frames.contains("text_delta"));
        assert!(frames.contains("answer"));
    }
    use crate::openai_http_mapping::validate_unified_request;
    use crate::provider::{UnifiedPartLocation, UnifiedUsageUpdate};
    use serde_json::json;

    #[test]
    fn request_roundtrip_preserves_interleaved_blocks_and_cache() {
        let raw = json!({"model":"claude","max_tokens":8192,"system":[{"type":"text","text":"system","cache_control":{"type":"ephemeral","ttl":"1h"}}],"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"plan","signature":"opaque"},{"type":"text","text":"before"},{"type":"tool_use","id":"call","name":"lookup","input":{"x":1},"caller":{"type":"direct"}},{"type":"text","text":"after"}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call","content":[{"type":"text","text":"ok"}],"is_error":false},{"type":"image","source":{"type":"file","file_id":"file"},"cache_control":{"type":"ephemeral"}}]}],"tools":[{"name":"lookup","input_schema":{"type":"object"},"defer_loading":true,"cache_control":{"type":"ephemeral"}}],"tool_choice":{"type":"any","disable_parallel_tool_use":true},"thinking":{"type":"enabled","budget_tokens":2048}});
        let request = request_from_value(&raw).expect("request");
        assert_eq!(request_to_value(&request).expect("native"), raw);
    }

    fn text_event(id: &str, delta: &str, thinking: bool) -> UnifiedEvent {
        UnifiedEvent::new(UnifiedEventKind::ContentPartDelta {
            delta: if thinking {
                UnifiedContentDelta::Thinking(delta.into())
            } else {
                UnifiedContentDelta::Text(delta.into())
            },
        })
        .located(
            0,
            id.into(),
            0,
            Some(UnifiedPartLocation::Content(if thinking { 1 } else { 0 })),
        )
    }

    fn parse(sample: &str) -> UnifiedRequest {
        parse_request(sample.as_bytes()).expect("parse unified")
    }

    /// golden: Anthropic request parse -> key IR structure assertions.
    #[test]
    fn parse_request_maps_all_block_types() {
        let request = parse(
            r#"{
                "model": "claude-opus-5",
                "max_tokens": 8192,
                "system": "be terse",
                "messages": [
                    {"role": "user", "content": "hello"},
                    {"role": "assistant", "content": [
                        {"type": "thinking", "thinking": "hmm", "signature": "sig"},
                        {"type": "text", "text": "hi"},
                        {"type": "tool_use", "id": "toolu_1", "name": "shell", "input": {"cmd": "ls"}}
                    ]},
                    {"role": "user", "content": [
                        {"type": "tool_result", "tool_use_id": "toolu_1", "content": "{\"out\":\"ok\"}"},
                        {"type": "text", "text": "continue"}
                    ]}
                ],
                "tools": [{"name": "shell", "description": "run", "input_schema": {"type": "object"}}],
                "tool_choice": {"type": "auto"},
                "stream": true
            }"#,
        );

        assert_eq!(request.model, "claude-opus-5");
        assert_eq!(request.input.len(), 6);
        // system
        assert!(matches!(
            &request.instructions.as_ref().expect("instructions")[0].kind,
            UnifiedInputItemKind::Message {role:UnifiedRole::System,content,..} if matches!(&content[0].kind,UnifiedContentPartKind::Text {text,..}if text=="be terse")
        ));
        // assistant:thinking → reasoning_blocks,text → content,tool_use → tool_calls
        assert!(
            matches!(&request.input[1].kind,UnifiedInputItemKind::Message {content,..}if matches!(&content[0].kind,UnifiedContentPartKind::Thinking {text,signature:Some(signature)}if text=="hmm"&&signature=="sig"))
        );
        assert!(
            matches!(&request.input[2].kind,UnifiedInputItemKind::Message {content,..}if matches!(&content[0].kind,UnifiedContentPartKind::Text {text,..}if text=="hi"))
        );
        assert!(
            matches!(&request.input[3].kind,UnifiedInputItemKind::FunctionCall {call_id,name,arguments,..}if call_id=="toolu_1"&&name=="shell"&&arguments==r#"{"cmd":"ls"}"#)
        );
        assert_eq!(
            request.input[1]
                .message
                .as_ref()
                .expect("boundary")
                .message_id,
            request.input[3]
                .message
                .as_ref()
                .expect("boundary")
                .message_id
        );
        // tool_result → Role::Tool
        assert!(matches!(
            &request.input[4].kind,
            UnifiedInputItemKind::FunctionCallOutput {call_id,..}if call_id=="toolu_1"
        ));
        // the following text stays in a user message
        assert!(matches!(
            &request.input[5].kind,
            UnifiedInputItemKind::Message {role:UnifiedRole::User,content,..}if matches!(&content[0].kind,UnifiedContentPartKind::Text {text,..}if text=="continue")
        ));
        // tools / tool_choice / stream
        assert_eq!(request.tools.as_ref().map(Vec::len), Some(1));
        assert!(matches!(
            &request.tool_choice,
            Some(UnifiedToolChoice::Auto)
        ));
        assert_eq!(request.controls.stream, Some(true));
        assert_eq!(request.options.max_output_tokens, Some(8192));
    }

    #[test]
    fn parse_request_maps_any_tool_choice_to_required() {
        let request = parse(
            r#"{
                "model": "claude-opus-5",
                "max_tokens": 100,
                "messages": [{"role": "user", "content": "run"}],
                "tools": [{"name": "shell", "input_schema": {"type": "object"}}],
                "tool_choice": {"type": "any"}
            }"#,
        );

        assert!(matches!(
            &request.tool_choice,
            Some(UnifiedToolChoice::Required)
        ));
        assert!(validate_unified_request(&request).is_ok());
    }

    #[test]
    fn parse_request_preserves_specific_tool_choice_and_definitions() {
        let request = parse(
            r#"{
                "model": "claude-opus-5",
                "max_tokens": 100,
                "messages": [{"role": "user", "content": "run"}],
                "tools": [
                    {"name": "shell", "input_schema": {"type": "object"}},
                    {"name": "grep", "input_schema": {"type": "object"}}
                ],
                "tool_choice": {"type": "tool", "name": "grep"}
            }"#,
        );

        assert!(
            matches!(&request.tool_choice,Some(UnifiedToolChoice::Function(name))if name=="grep")
        );
        let tools = request.tools.as_ref().expect("tools");
        assert_eq!(tools.len(), 2);
        assert!(matches!(&tools[1].kind,UnifiedToolKind::Function {name,..}if name=="grep"));
        assert!(validate_unified_request(&request).is_ok());
    }

    #[test]
    fn parse_rejects_invalid_json_as_400() {
        let err = parse_request(b"{invalid").expect_err("invalid");
        assert!(err.message.contains("invalid request body"));
    }

    /// golden: non-stream IR -> Anthropic response body snapshot.
    #[test]
    fn non_stream_response_golden() {
        let response = UnifiedResponse::from_completion(
            "msg_1".into(),
            "2026-01-01T00:00:00Z".into(),
            "claude-opus-5".into(),
            "answer".into(),
            Some("hmm".into()),
            Some(vec![crate::provider::ToolCall {
                id: Some("toolu_1".to_string()),
                name: "shell".to_string(),
                description: String::new(),
                arguments: r#"{"cmd":"ls"}"#.to_string(),
            }]),
            FinishReason::ToolCalls,
            crate::usage_handler::EndpointUsage::from_endpoint_payload(
                "/chat/completions",
                json!({"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}),
            )
            .expect("usage"),
        );

        let value = serialize_non_stream(&response);
        assert_eq!(
            value,
            json!({
                "id": "msg_1",
                "type": "message",
                "role": "assistant",
                "model": "claude-opus-5",
                "content": [
                    {"type": "thinking", "thinking": "hmm", "signature": ""},
                    {"type": "text", "text": "answer"},
                    {"type": "tool_use", "id": "toolu_1", "name": "shell", "input": {"cmd": "ls"}}
                ],
                "stop_reason": "tool_use",
                "stop_sequence": null,
                "usage": {"input_tokens": 10, "output_tokens": 5}
            })
        );
    }

    /// golden: stream events -> SSE frame sequence (message_start -> text deltas -> message_delta -> message_stop).
    #[test]
    fn stream_state_machine_golden() {
        let mut state = AnthropicSseState::new();
        let mut frames = Vec::new();
        let events = vec![
            UnifiedEvent::new(UnifiedEventKind::ResponseCreated {
                response: UnifiedResponse::new("msg_1".into(), "claude-opus-5".into()),
            }),
            text_event("msg_1", "hmm", true),
            UnifiedEvent::new(UnifiedEventKind::ContentPartDone {
                part: UnifiedContentPart {
                    kind: UnifiedContentPartKind::Thinking {
                        text: "hmm".into(),
                        signature: None,
                    },
                    cache_control: None,
                    source_extensions: None,
                },
            }),
            text_event("msg_1", "he", false),
            text_event("msg_1", "llo", false),
            UnifiedEvent::new(UnifiedEventKind::OutputItemAdded {
                item: crate::provider::UnifiedOutputItem::new(
                    "toolu_1".into(),
                    UnifiedOutputItemKind::FunctionCall {
                        call_id: "toolu_1".into(),
                        name: "shell".into(),
                        arguments: String::new(),
                        caller: None,
                    },
                ),
            })
            .located(0, "toolu_1".into(), 1, None),
            UnifiedEvent::new(UnifiedEventKind::ItemDelta {
                delta: UnifiedItemDelta::FunctionArguments(r#"{"cmd""#.into()),
            })
            .located(0, "toolu_1".into(), 1, None),
            UnifiedEvent::new(UnifiedEventKind::ItemDelta {
                delta: UnifiedItemDelta::FunctionArguments(r#":"ls"}"#.into()),
            })
            .located(0, "toolu_1".into(), 1, None),
            UnifiedEvent::new(UnifiedEventKind::OutputItemDone {
                item: crate::provider::UnifiedOutputItem::new(
                    "toolu_1".into(),
                    UnifiedOutputItemKind::FunctionCall {
                        call_id: "toolu_1".into(),
                        name: "shell".into(),
                        arguments: r#"{"cmd":"ls"}"#.into(),
                        caller: None,
                    },
                ),
            })
            .located(0, "toolu_1".into(), 1, None),
            UnifiedEvent::new(UnifiedEventKind::Usage {
                usage: crate::usage_handler::EndpointUsage::from_endpoint_payload(
                    "/chat/completions",
                    json!({"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}),
                )
                .expect("usage"),
                update: UnifiedUsageUpdate::Snapshot,
                source_usage: None,
            }),
            {
                let mut response = UnifiedResponse::new("msg_1".into(), "claude-opus-5".into());
                response.candidates.push(UnifiedCandidate {
                    index: 0,
                    output: vec![],
                    ending: Some(UnifiedEnding::from_source("tool_calls")),
                    logprobs: None,
                    source_extensions: None,
                });
                UnifiedEvent::new(UnifiedEventKind::Completed {
                    response: Some(response),
                })
            },
        ];
        for event in &events {
            frames.extend(state.on_event(event));
        }
        frames.extend(state.finish());

        let joined = frames.join("");
        // SSE frame shapes
        for name in [
            "message_start",
            "content_block_start",
            "content_block_delta",
            "content_block_stop",
            "message_delta",
            "message_stop",
        ] {
            assert!(
                joined.contains(&format!("event: {name}\n")),
                "missing {name}"
            );
        }
        // thinking and text blocks each open once
        assert!(joined.matches("event: content_block_start\n").count() == 3);
        // text delta content
        assert!(
            joined.contains("\"text_delta\"")
                && joined.contains("\"he\"")
                && joined.contains("\"llo\"")
        );
        // tool_use argument deltas
        assert!(joined.contains("input_json_delta"));
        // termination: tool_use stop_reason + usage
        assert!(joined.contains("\"stop_reason\":\"tool_use\""));
        assert!(joined.contains("\"input_tokens\":10"));
        assert!(joined.ends_with("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"));

        // nothing emitted after completion
        assert!(
            state
                .on_event(&text_event("msg_1", "late", false))
                .is_empty()
        );
    }

    /// Tool loop: parse(tool_use) -> IR tool_calls; parse(tool_result) -> IR tool message.
    #[test]
    fn tool_loop_roundtrip_through_ir() {
        let request = parse(
            r#"{
                "model": "claude-opus-5",
                "max_tokens": 100,
                "messages": [
                    {"role": "user", "content": "run"},
                    {"role": "assistant", "content": [
                        {"type": "tool_use", "id": "toolu_1", "name": "shell", "input": {"cmd": "ls"}}
                    ]},
                    {"role": "user", "content": [
                        {"type": "tool_result", "tool_use_id": "toolu_1", "content": "ok"}
                    ]}
                ]
            }"#,
        );
        let UnifiedInputItemKind::FunctionCall {
            name, arguments, ..
        } = &request.input[1].kind
        else {
            panic!("function call")
        };
        assert_eq!(name, "shell");
        assert!(matches!(
            &request.input[2].kind,
            UnifiedInputItemKind::FunctionCallOutput {call_id,output:UnifiedToolOutput::Text(text),..}if call_id=="toolu_1"&&text=="ok"
        ));

        // serialize side: IR tool_calls -> tool_use blocks
        let unified_calls = vec![crate::provider::ToolCall {
            id: Some("toolu_1".to_string()),
            name: "shell".to_string(),
            description: String::new(),
            arguments: arguments.clone(),
        }];
        let blocks = tool_calls_to_anthropic_content(&unified_calls);
        assert!(matches!(
            &blocks[0],
            AnthropicResponseContentBlock::ToolUse { name, input, .. }
                if name == "shell" && input == &json!({"cmd": "ls"})
        ));
    }
}
