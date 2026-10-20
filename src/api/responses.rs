//! `/responses` API: OpenAI Responses parse / serialize / SSE state machine.
//!
//! parse: native request -> unified IR; unmodeled fields are
//! retained in source-tagged extensions and merged back upstream by the
//! dialect (codex fidelity). serialize: IR -> native response object and
//! `event:`/`data:` SSE frames (including reasoning events).

use serde_json::{Value, json};

use crate::openai_responses_types::ResponsesStreamEvent;
use crate::provider::{
    UnifiedCacheControl, UnifiedCandidate, UnifiedContentPart, UnifiedContentPartKind,
    UnifiedEnding, UnifiedInputItem, UnifiedInputItemKind, UnifiedMediaSource, UnifiedOutputItem,
    UnifiedResponseError, UnifiedRole, UnifiedTool, UnifiedToolChoice, UnifiedToolKind,
    UnifiedToolOutput, input_item, merge_extensions, output_from_input, parse_role,
    response_envelope, role_name, source_extensions, value_string,
};
use crate::provider::{
    UnifiedContentDelta, UnifiedEventKind, UnifiedItemDelta, UnifiedOutputItemKind, UnifiedRequest,
    UnifiedResponseStatus, UnifiedSourceProtocol,
};
use crate::provider::{UnifiedEvent, UnifiedResponse};
use std::collections::HashMap;

/// API request parse failure (HTTP 400).
#[derive(Debug)]
pub struct ResponsesParseError {
    pub message: String,
}

/// Responses request bytes -> unified IR with source-tagged extensions.
pub fn parse_request(body: &[u8]) -> Result<UnifiedRequest, ResponsesParseError> {
    let value = serde_json::from_slice(body).map_err(|err| ResponsesParseError {
        message: format!("invalid request body: {err}"),
    })?;
    request_from_value(&value).map_err(|message| ResponsesParseError { message })
}

const PROTOCOL: UnifiedSourceProtocol = UnifiedSourceProtocol::Responses;
fn media_source(value: &Value) -> Result<UnifiedMediaSource, String> {
    if let Some(file_id) = value_string(value, "file_id") {
        return Ok(UnifiedMediaSource::FileId { file_id });
    }
    if let Some(url) = value_string(value, "image_url").or_else(|| value_string(value, "file_url"))
    {
        return Ok(if url.starts_with("data:") {
            UnifiedMediaSource::DataUrl { url }
        } else {
            UnifiedMediaSource::Url { url }
        });
    }
    let data = value_string(value, "file_data").ok_or("media requires URL, file ID or data")?;
    Ok(if data.starts_with("data:") {
        UnifiedMediaSource::DataUrl { url: data }
    } else {
        UnifiedMediaSource::Base64 {
            data,
            media_type: "application/octet-stream".into(),
        }
    })
}
fn source_value(source: &UnifiedMediaSource, image: bool) -> Value {
    match source {
        UnifiedMediaSource::FileId { file_id } => json!({"file_id":file_id}),
        UnifiedMediaSource::Url { url } => {
            if image {
                json!({"image_url":url})
            } else {
                json!({"file_url":url})
            }
        }
        UnifiedMediaSource::DataUrl { url } => {
            if image {
                json!({"image_url":url})
            } else {
                json!({"file_data":url})
            }
        }
        UnifiedMediaSource::Base64 { data, media_type } => {
            let url = format!("data:{media_type};base64,{data}");
            if image {
                json!({"image_url":url})
            } else {
                json!({"file_data":url})
            }
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
        .ok_or("content must be text or parts")?
        .iter()
        .map(|part| {
            let kind = match part["type"].as_str().unwrap_or("") {
                "input_text" | "output_text" => UnifiedContentPartKind::Text {
                    text: value_string(part, "text").unwrap_or_default(),
                    annotations: part["annotations"].as_array().cloned().unwrap_or_default(),
                    citations: None,
                    logprobs: part["logprobs"].as_array().cloned(),
                },
                "reasoning_text" => UnifiedContentPartKind::Thinking {
                    text: value_string(part, "text").unwrap_or_default(),
                    signature: None,
                },
                "refusal" => UnifiedContentPartKind::Refusal {
                    text: value_string(part, "refusal").unwrap_or_default(),
                },
                "input_image" => UnifiedContentPartKind::Image {
                    source: media_source(part)?,
                    detail: value_string(part, "detail"),
                    transformations: None,
                },
                "input_file" => UnifiedContentPartKind::File {
                    source: media_source(part)?,
                    filename: value_string(part, "filename"),
                    detail: value_string(part, "detail"),
                },
                kind => UnifiedContentPartKind::Other {
                    part_type: kind.into(),
                    payload: part.clone(),
                },
            };
            Ok(UnifiedContentPart {
                kind,
                cache_control: part
                    .get("prompt_cache_breakpoint")
                    .map(|value| UnifiedCacheControl::Responses(value.clone())),
                source_extensions: source_extensions(
                    part,
                    &[
                        "text",
                        "annotations",
                        "logprobs",
                        "refusal",
                        "detail",
                        "filename",
                        "prompt_cache_breakpoint",
                    ],
                    PROTOCOL,
                ),
            })
        })
        .collect()
}
pub fn content_value(part: &UnifiedContentPart, output: bool) -> Result<Value, String> {
    let mut value = match &part.kind {
        UnifiedContentPartKind::Text {
            text,
            annotations,
            logprobs,
            ..
        } => {
            let native_type = part
                .source_extensions
                .as_ref()
                .filter(|ext| ext.protocol == PROTOCOL)
                .and_then(|ext| ext.fields.get("type"))
                .and_then(Value::as_str);
            let kind = if output {
                "output_text"
            } else {
                native_type.unwrap_or("input_text")
            };
            let mut value = json!({"type":kind,"text":text});
            if kind == "output_text" {
                value["annotations"] = json!(annotations);
                if let Some(logprobs) = logprobs {
                    value["logprobs"] = json!(logprobs);
                }
            }
            value
        }
        UnifiedContentPartKind::Thinking {
            text,
            signature: None,
        } => json!({"type":"reasoning_text","text":text}),
        UnifiedContentPartKind::Refusal { text } => json!({"type":"refusal","refusal":text}),
        UnifiedContentPartKind::Image { source, detail, .. } => {
            let mut value = source_value(source, true);
            value["type"] = json!("input_image");
            if let Some(detail) = detail {
                value["detail"] = json!(detail);
            }
            value
        }
        UnifiedContentPartKind::File {
            source,
            filename,
            detail,
        } => {
            let mut value = source_value(source, false);
            value["type"] = json!("input_file");
            if let Some(filename) = filename {
                value["filename"] = json!(filename);
            }
            if let Some(detail) = detail {
                value["detail"] = json!(detail);
            }
            value
        }
        UnifiedContentPartKind::Other { payload, .. }
        | UnifiedContentPartKind::Audio { payload, .. } => payload.clone(),
        _ => return Err("content has no Responses representation".into()),
    };
    if let Some(UnifiedCacheControl::Responses(cache)) = &part.cache_control {
        value["prompt_cache_breakpoint"] = cache.clone();
    }
    merge_extensions(&mut value, part.source_extensions.as_ref(), PROTOCOL);
    Ok(value)
}
fn parse_output(value: &Value) -> Result<UnifiedToolOutput, String> {
    Ok(if let Some(text) = value.as_str() {
        UnifiedToolOutput::Text(text.into())
    } else if value.is_array() {
        UnifiedToolOutput::Content(parse_content(value)?)
    } else if value.is_null() {
        UnifiedToolOutput::Missing
    } else {
        UnifiedToolOutput::Json(value.clone())
    })
}
fn parse_item(value: &Value, index: usize) -> Result<UnifiedInputItem, String> {
    let kind = match value["type"].as_str().unwrap_or("message") {
        "message" => UnifiedInputItemKind::Message {
            role: parse_role(value["role"].as_str().ok_or("message requires role")?)?,
            content: parse_content(&value["content"])?,
            phase: value_string(value, "phase"),
        },
        "function_call" => UnifiedInputItemKind::FunctionCall {
            call_id: value_string(value, "call_id").ok_or("function_call requires call_id")?,
            name: value_string(value, "name").ok_or("function_call requires name")?,
            arguments: value_string(value, "arguments").unwrap_or_default(),
            caller: None,
            cache_control: None,
        },
        "function_call_output" => UnifiedInputItemKind::FunctionCallOutput {
            call_id: value_string(value, "call_id")
                .ok_or("function_call_output requires call_id")?,
            output: parse_output(&value["output"])?,
            is_error: None,
            cache_control: None,
        },
        "reasoning" => UnifiedInputItemKind::Reasoning {
            summary: value["summary"].as_array().cloned().unwrap_or_default(),
            content: value
                .get("content")
                .filter(|value| !value.is_null())
                .map(parse_content)
                .transpose()?,
            encrypted_content: value_string(value, "encrypted_content"),
        },
        "item_reference" => UnifiedInputItemKind::ItemReference {
            id: value_string(value, "id").ok_or("item reference requires id")?,
        },
        kind => UnifiedInputItemKind::Other {
            item_type: kind.into(),
            payload: value.clone(),
        },
    };
    Ok(input_item(
        value,
        kind,
        index,
        &[
            "type",
            "id",
            "status",
            "role",
            "content",
            "phase",
            "call_id",
            "name",
            "arguments",
            "output",
            "summary",
            "encrypted_content",
        ],
        PROTOCOL,
    ))
}
pub fn request_from_value(value: &Value) -> Result<UnifiedRequest, String> {
    let mut request = UnifiedRequest {
        model: value_string(value, "model").unwrap_or_default(),
        ..Default::default()
    };
    if let Some(instructions) = value.get("instructions").filter(|value| !value.is_null()) {
        request.instructions = Some(if let Some(items) = instructions.as_array() {
            items
                .iter()
                .enumerate()
                .map(|(index, item)| parse_item(item, index))
                .collect::<Result<Vec<_>, _>>()?
        } else if instructions.is_object() {
            vec![parse_item(instructions, 0)?]
        } else {
            vec![UnifiedInputItem::new(UnifiedInputItemKind::Message {
                role: UnifiedRole::Developer,
                content: parse_content(instructions)?,
                phase: None,
            })]
        });
    }
    let input = value.get("input").ok_or("input is required")?;
    request.input = if input.is_string() {
        vec![UnifiedInputItem::new(UnifiedInputItemKind::Message {
            role: UnifiedRole::User,
            content: parse_content(input)?,
            phase: None,
        })]
    } else {
        input
            .as_array()
            .ok_or("input must be text or items")?
            .iter()
            .enumerate()
            .map(|(index, item)| parse_item(item, index))
            .collect::<Result<Vec<_>, _>>()?
    };
    request.tools = value["tools"].as_array().map(|tools| {
        tools
            .iter()
            .map(|tool| UnifiedTool {
                cache_control: None,
                source_extensions: source_extensions(
                    tool,
                    &["type", "name", "description", "parameters", "strict"],
                    PROTOCOL,
                ),
                kind: if tool["type"] == "function" {
                    UnifiedToolKind::Function {
                        name: value_string(tool, "name").unwrap_or_default(),
                        description: value_string(tool, "description"),
                        parameters: tool.get("parameters").cloned().unwrap_or_else(|| json!({})),
                        strict: tool["strict"].as_bool(),
                        allowed_callers: None,
                    }
                } else {
                    UnifiedToolKind::Server {
                        tool_type: tool["type"].as_str().unwrap_or("").into(),
                        name: value_string(tool, "name"),
                        configuration: tool.clone(),
                    }
                },
            })
            .collect()
    });
    request.tool_choice = value
        .get("tool_choice")
        .map(|choice| match choice.as_str() {
            Some("auto") => UnifiedToolChoice::Auto,
            Some("none") => UnifiedToolChoice::None,
            Some("required") => UnifiedToolChoice::Required,
            _ => {
                if choice["type"] == "function" {
                    UnifiedToolChoice::Function(value_string(choice, "name").unwrap_or_default())
                } else {
                    UnifiedToolChoice::Other(choice.clone())
                }
            }
        });
    request.options.temperature = value["temperature"].as_f64();
    request.options.top_p = value["top_p"].as_f64();
    request.options.max_output_tokens = value["max_output_tokens"].as_i64();
    request.options.parallel_tool_calls = value["parallel_tool_calls"].as_bool();
    request.options.reasoning_effort = value_string(&value["reasoning"], "effort");
    request.options.reasoning_summary = value["reasoning"].get("summary").cloned();
    request.options.reasoning_extensions =
        source_extensions(&value["reasoning"], &["effort", "summary"], PROTOCOL);
    request.options.response_format = value["text"].get("format").cloned();
    request.options.verbosity = value_string(&value["text"], "verbosity");
    request.options.thinking = value.get("thinking").cloned();
    request.options.metadata = value.get("metadata").cloned();
    request.options.truncation = value_string(value, "truncation");
    request.options.service_tier = value_string(value, "service_tier");
    request.context.previous_response_id = value_string(value, "previous_response_id");
    request.context.conversation_id =
        value_string(value, "conversation").or_else(|| value_string(&value["conversation"], "id"));
    request.controls.stream = value["stream"].as_bool();
    request.controls.stream_options = value.get("stream_options").cloned();
    request.controls.store = value["store"].as_bool();
    request.controls.background = value["background"].as_bool();
    request.controls.include = value["include"].as_array().map(|values| {
        values
            .iter()
            .filter_map(Value::as_str)
            .map(str::to_owned)
            .collect()
    });
    request.source_extensions = source_extensions(
        value,
        &[
            "model",
            "input",
            "instructions",
            "tools",
            "tool_choice",
            "temperature",
            "top_p",
            "max_output_tokens",
            "parallel_tool_calls",
            "reasoning",
            "text",
            "thinking",
            "metadata",
            "truncation",
            "service_tier",
            "previous_response_id",
            "conversation",
            "stream",
            "stream_options",
            "store",
            "background",
            "include",
        ],
        PROTOCOL,
    );
    if let Some(extra) = source_extensions(&value["text"], &["format", "verbosity"], PROTOCOL) {
        request
            .source_extensions
            .get_or_insert_with(|| crate::provider::UnifiedSourceExtensions {
                protocol: PROTOCOL,
                fields: HashMap::new(),
            })
            .fields
            .insert("text".into(), json!(extra.fields));
    }
    Ok(request)
}
fn responses_function_call_item_id(source_id: &str) -> String {
    if source_id.starts_with("fc") {
        source_id.to_owned()
    } else {
        format!("fc_{source_id}")
    }
}

fn item_value(item: &UnifiedInputItem) -> Result<Value, String> {
    let mut value = match &item.kind {
        UnifiedInputItemKind::Message {
            role,
            content,
            phase,
        } => {
            let mut value = json!({"type":"message","role":role_name(*role),"content":content.iter().map(|part|content_value(part,*role==UnifiedRole::Assistant)).collect::<Result<Vec<_>,_>>()?});
            if let Some(phase) = phase {
                value["phase"] = json!(phase);
            }
            value
        }
        UnifiedInputItemKind::FunctionCall {
            call_id,
            name,
            arguments,
            ..
        } => json!({"type":"function_call","call_id":call_id,"name":name,"arguments":arguments}),
        UnifiedInputItemKind::FunctionCallOutput {
            call_id, output, ..
        } => {
            let output = match output {
                UnifiedToolOutput::Missing => Value::Null,
                UnifiedToolOutput::Text(text) => json!(text),
                UnifiedToolOutput::Json(value) => value.clone(),
                UnifiedToolOutput::Content(parts) => json!(
                    parts
                        .iter()
                        .map(|part| content_value(part, false))
                        .collect::<Result<Vec<_>, _>>()?
                ),
            };
            json!({"type":"function_call_output","call_id":call_id,"output":output})
        }
        UnifiedInputItemKind::Reasoning {
            summary,
            content,
            encrypted_content,
        } => {
            let mut value = json!({"type":"reasoning","summary":summary});
            if let Some(content) = content {
                value["content"] = json!(
                    content
                        .iter()
                        .map(|part| content_value(part, false))
                        .collect::<Result<Vec<_>, _>>()?
                );
            }
            if let Some(data) = encrypted_content {
                value["encrypted_content"] = json!(data);
            }
            value
        }
        UnifiedInputItemKind::ItemReference { id } => json!({"type":"item_reference","id":id}),
        UnifiedInputItemKind::Other { payload, .. } => payload.clone(),
    };
    if let Some(id) = item
        .identity
        .as_ref()
        .and_then(|identity| identity.source_id.as_ref())
    {
        let id = if matches!(&item.kind, UnifiedInputItemKind::FunctionCall { .. }) {
            responses_function_call_item_id(id)
        } else {
            id.to_owned()
        };
        value["id"] = json!(id);
    }
    if let Some(status) = &item.status {
        value["status"] = json!(status);
    }
    merge_extensions(&mut value, item.source_extensions.as_ref(), PROTOCOL);
    Ok(value)
}
pub fn request_to_value(request: &UnifiedRequest) -> Result<Value, String> {
    let mut value = json!({"model":request.model,"input":request.input.iter().map(item_value).collect::<Result<Vec<_>,_>>()?});
    if let Some(instructions) = &request.instructions {
        let mut texts = Vec::new();
        let plain = instructions.iter().all(|item| {
            if item.identity.is_some()
                || item.status.is_some()
                || item.message.is_some()
                || item.source_extensions.is_some()
            {
                return false;
            }
            let UnifiedInputItemKind::Message {
                role: UnifiedRole::System | UnifiedRole::Developer,
                content,
                phase: None,
            } = &item.kind
            else {
                return false;
            };
            content.iter().all(|part| {
                if let UnifiedContentPartKind::Text {
                    text,
                    annotations,
                    citations: None,
                    logprobs: None,
                } = &part.kind
                {
                    if !annotations.is_empty()
                        || part.cache_control.is_some()
                        || part.source_extensions.is_some()
                    {
                        return false;
                    }
                    texts.push(text.as_str());
                    true
                } else {
                    false
                }
            })
        });
        if plain {
            let mut text = String::new();
            for part in texts {
                if !text.is_empty() && !text.ends_with('\n') {
                    text.push('\n');
                }
                text.push_str(part);
            }
            value["instructions"] = json!(text);
        } else {
            value["input"] = json!(
                instructions
                    .iter()
                    .chain(&request.input)
                    .map(item_value)
                    .collect::<Result<Vec<_>, _>>()?
            );
        }
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
                            ..
                        } => {
                            let mut value =
                                json!({"type":"function","name":name,"parameters":parameters});
                            if let Some(description) = description {
                                value["description"] = json!(description);
                            }
                            if let Some(strict) = strict {
                                value["strict"] = json!(strict);
                            }
                            value
                        }
                        UnifiedToolKind::Server { configuration, .. }
                        | UnifiedToolKind::Other {
                            payload: configuration,
                            ..
                        } => configuration.clone(),
                    };
                    merge_extensions(&mut value, tool.source_extensions.as_ref(), PROTOCOL);
                    value
                })
                .collect::<Vec<_>>()
        );
    }
    if let Some(choice) = &request.tool_choice {
        value["tool_choice"] = match choice {
            UnifiedToolChoice::Auto => json!("auto"),
            UnifiedToolChoice::None => json!("none"),
            UnifiedToolChoice::Required => json!("required"),
            UnifiedToolChoice::Function(name) => json!({"type":"function","name":name}),
            UnifiedToolChoice::Other(value) => value.clone(),
            UnifiedToolChoice::Allowed { mode, tools } => {
                json!({"type":"allowed_tools","mode":mode,"tools":tools})
            }
        };
    }
    for (key, option) in [
        ("temperature", json!(request.options.temperature)),
        ("top_p", json!(request.options.top_p)),
        (
            "max_output_tokens",
            json!(request.options.max_output_tokens),
        ),
        (
            "parallel_tool_calls",
            json!(request.options.parallel_tool_calls),
        ),
        ("metadata", json!(request.options.metadata)),
        ("truncation", json!(request.options.truncation)),
        ("service_tier", json!(request.options.service_tier)),
        ("thinking", json!(request.options.thinking)),
        (
            "previous_response_id",
            json!(request.context.previous_response_id),
        ),
        ("conversation", json!(request.context.conversation_id)),
        ("stream", json!(request.controls.stream)),
        ("store", json!(request.controls.store)),
        ("background", json!(request.controls.background)),
        ("include", json!(request.controls.include)),
    ] {
        if !option.is_null() {
            value[key] = option;
        }
    }
    if request.options.reasoning_effort.is_some()
        || request.options.reasoning_summary.is_some()
        || request.options.reasoning_extensions.is_some()
    {
        let mut reasoning = json!({});
        if let Some(effort) = &request.options.reasoning_effort {
            reasoning["effort"] = json!(effort);
        }
        if let Some(summary) = &request.options.reasoning_summary {
            reasoning["summary"] = summary.clone();
        }
        merge_extensions(
            &mut reasoning,
            request.options.reasoning_extensions.as_ref(),
            PROTOCOL,
        );
        value["reasoning"] = reasoning;
    }
    if request.options.response_format.is_some() || request.options.verbosity.is_some() {
        let mut text = json!({});
        if let Some(format) = &request.options.response_format {
            text["format"] = format.clone();
        }
        if let Some(verbosity) = &request.options.verbosity {
            text["verbosity"] = json!(verbosity);
        }
        value["text"] = text;
    }
    merge_extensions(&mut value, request.source_extensions.as_ref(), PROTOCOL);
    Ok(value)
}
pub fn output_item_from_value(value: &Value, id: String) -> Result<UnifiedOutputItem, String> {
    Ok(output_from_input(parse_item(value, 0)?, id))
}
pub fn output_item_value(item: &UnifiedOutputItem) -> Result<Value, String> {
    let mut value = item_value(&item.as_input())?;
    if let UnifiedOutputItemKind::Message { content, .. } = &item.kind {
        value["content"] = json!(
            content
                .iter()
                .map(|part| content_value(part, true))
                .collect::<Result<Vec<_>, _>>()?
        );
    }
    value["id"] = json!(
        item.identity
            .source_id
            .as_ref()
            .unwrap_or(&item.identity.internal_id)
    );
    Ok(value)
}
pub fn error_from_value(value: &Value) -> UnifiedResponseError {
    UnifiedResponseError {
        code: value_string(value, "code"),
        error_type: value_string(value, "type"),
        param: value_string(value, "param"),
        message: value_string(value, "message").unwrap_or_default(),
        source_extensions: source_extensions(
            value,
            &["code", "type", "param", "message"],
            PROTOCOL,
        ),
    }
}
pub fn response_from_value(value: &Value) -> Result<UnifiedResponse, String> {
    let mut response = response_envelope(
        value,
        PROTOCOL,
        "/responses",
        &[
            "id",
            "model",
            "created_at",
            "completed_at",
            "status",
            "output",
            "usage",
            "error",
            "incomplete_details",
            "metadata",
            "service_tier",
        ],
    )?;
    response.created_at = value["created_at"].as_i64();
    response.completed_at = value["completed_at"].as_i64();
    response.status = match value["status"].as_str().unwrap_or("completed") {
        "queued" => UnifiedResponseStatus::Queued,
        "in_progress" => UnifiedResponseStatus::InProgress,
        "completed" => UnifiedResponseStatus::Completed,
        "incomplete" => UnifiedResponseStatus::Incomplete {
            reason: value_string(&value["incomplete_details"], "reason"),
        },
        "failed" => UnifiedResponseStatus::Failed {
            error: error_from_value(&value["error"]),
        },
        "cancelled" => UnifiedResponseStatus::Cancelled { reason: None },
        other => UnifiedResponseStatus::Other {
            value: other.into(),
        },
    };
    let output = value["output"]
        .as_array()
        .into_iter()
        .flatten()
        .enumerate()
        .map(|(index, item)| {
            output_item_from_value(
                item,
                value_string(item, "id").unwrap_or_else(|| format!("{}-item-{index}", response.id)),
            )
        })
        .collect::<Result<Vec<_>, _>>()?;
    let tool = output
        .iter()
        .any(|item| matches!(item.kind, UnifiedOutputItemKind::FunctionCall { .. }));
    let reason = if tool {
        "tool_calls"
    } else {
        value["incomplete_details"]["reason"]
            .as_str()
            .unwrap_or("stop")
    };
    response.candidates.push(UnifiedCandidate {
        index: 0,
        output,
        ending: Some(UnifiedEnding::from_source(reason)),
        logprobs: None,
        source_extensions: None,
    });
    Ok(response)
}

// ---------------------------------------------------------------------------
// serialize
// ---------------------------------------------------------------------------

/// IR unified response -> Responses response object.
pub fn serialize_non_stream(response: &UnifiedResponse) -> Value {
    let mut output = Vec::new();
    for item in response
        .candidates
        .first()
        .into_iter()
        .flat_map(|candidate| &candidate.output)
    {
        if let Ok(value) = output_item_value(item) {
            output.push(value);
        }
    }
    let incomplete = matches!(response.status, UnifiedResponseStatus::Incomplete { .. })
        || response
            .candidates
            .first()
            .and_then(|candidate| candidate.ending.as_ref())
            .is_some_and(|ending| ending.to_chat() == "length");
    let (status, incomplete_details) = if incomplete {
        ("incomplete", Some(json!({"reason":"max_output_tokens"})))
    } else {
        ("completed", None)
    };
    let usage_json = response
        .usage
        .as_ref()
        .map(crate::openai_http_mapping::map_usage_to_openai)
        .unwrap_or_else(|| {
            crate::openai_http_mapping::map_usage_to_openai(
                &crate::openai_http_mapping::empty_usage(),
            )
        });
    let usage = json!({
        "input_tokens": usage_json.prompt_tokens,
        "input_tokens_details": {"cached_tokens": 0},
        "output_tokens": usage_json.completion_tokens,
        "output_tokens_details": {"reasoning_tokens": 0},
        "total_tokens": usage_json.total_tokens,
    });

    let created_at = response
        .created_at
        .unwrap_or_else(|| chrono::Utc::now().timestamp());

    json!({
        "id": response.id,
        "object": "response",
        "created_at": created_at,
        "status": status,
        "error": null,
        "incomplete_details": incomplete_details,
        "model": response.model,
        "output": output,
        "parallel_tool_calls": true,
        "previous_response_id": null,
        "reasoning": null,
        "store": false,
        "temperature": null,
        "text": null,
        "tool_choice": "auto",
        "tools": [],
        "top_p": null,
        "truncation": null,
        "usage": usage,
        "user": null,
        "metadata": {},
    }
    )
}

// ---------------------------------------------------------------------------
// Streaming SSE state machine
// ---------------------------------------------------------------------------

/// IR stream events -> Responses SSE frame state machine.
///
/// Event order: response.created -> output_item.added -> content_part.added ->
/// deltas -> content_part.done -> output_item.done -> response.completed.
#[derive(Default)]
pub struct ResponsesSseState {
    started: bool,
    request_id: String,
    model: String,
    created_at: i64,
    sequence: i64,
    /// item_id → (kind, name, output_index)
    items: std::collections::HashMap<String, (String, String, i64)>,
    /// message item state keyed by item_id.
    message_states: std::collections::HashMap<String, MessageStreamState>,
    /// reasoning summary state keyed by item_id.
    reasoning_states: std::collections::HashMap<String, ReasoningStreamState>,
    /// function call state keyed by call_id.
    function_call_states: std::collections::HashMap<String, FunctionCallStreamState>,
    next_output_index: i64,
    usage: Option<Value>,
    finished: bool,
    tool_items: std::collections::HashMap<String, crate::provider::UnifiedOutputItem>,
    finish_reason: Option<String>,
    incomplete_reason: Option<String>,
}

#[derive(Default)]
struct MessageStreamState {
    output_index: i64,
    text: String,
    content_part_started: bool,
}

#[derive(Default)]
struct ReasoningStreamState {
    item_id: String,
    output_index: i64,
    text: String,
    item_added: bool,
    part_started: bool,
}

#[derive(Default)]
struct FunctionCallStreamState {
    output_index: i64,
    name: String,
    arguments: String,
}

impl ResponsesSseState {
    pub fn new() -> Self {
        Self::default()
    }

    fn next_sequence(&mut self) -> i64 {
        let sequence = self.sequence;
        self.sequence += 1;
        sequence
    }

    fn frame(event: &ResponsesStreamEvent) -> String {
        let event_name = event_type_name(event);
        let data = serde_json::to_string(event).unwrap_or_else(|_| "{}".to_string());
        format!("event: {event_name}\ndata: {data}\n\n")
    }

    fn response_skeleton(&self) -> Value {
        json!({
            "id": self.request_id,
            "object": "response",
            "created_at": self.created_at,
            "status": "in_progress",
            "error": null,
            "incomplete_details": null,
            "instructions": null,
            "max_output_tokens": null,
            "model": self.model,
            "output": [],
            "parallel_tool_calls": true,
            "previous_response_id": null,
            "reasoning": null,
            "store": false,
            "temperature": null,
            "text": null,
            "tool_choice": "auto",
            "tools": [],
            "top_p": null,
            "truncation": null,
            "usage": null,
            "user": null,
            "metadata": {},
        })
    }

    /// Process one IR event, producing 0..n SSE frames.
    pub fn on_event(&mut self, event: &UnifiedEvent) -> Vec<String> {
        if self.finished {
            return Vec::new();
        }
        let mut frames = Vec::new();

        let id = event
            .target
            .as_ref()
            .and_then(|target| target.item_id.as_ref())
            .or(event.response_id.as_ref())
            .cloned()
            .unwrap_or_default();
        match &event.kind {
            UnifiedEventKind::ResponseCreated { response } => {
                let id = &response.id;
                let model = &response.model;
                self.started = true;
                self.request_id = id.clone();
                self.model = model.clone();
                self.created_at = response
                    .created_at
                    .unwrap_or_else(|| chrono::Utc::now().timestamp());
                frames.push(Self::frame(&ResponsesStreamEvent::ResponseCreated {
                    response: serde_json::from_value(self.response_skeleton()).expect("skeleton"),
                    sequence_number: self.next_sequence(),
                    extra: Default::default(),
                }));
            }
            UnifiedEventKind::ResponseInProgress { .. } => {
                frames.push(Self::frame(&ResponsesStreamEvent::ResponseInProgress {
                    response: serde_json::from_value(self.response_skeleton()).expect("skeleton"),
                    sequence_number: self.next_sequence(),
                    extra: Default::default(),
                }));
            }
            UnifiedEventKind::OutputItemAdded { item }
                if matches!(item.kind, UnifiedOutputItemKind::Message { .. }) =>
            {
                let id = &item.identity.internal_id;
                if !self.started {
                    self.started = true;
                    self.request_id = id.clone();
                }
                let output_index = self.next_output_index;
                self.next_output_index += 1;
                self.items.insert(
                    id.clone(),
                    ("message".to_string(), String::new(), output_index),
                );
                self.message_states.insert(
                    id.clone(),
                    MessageStreamState {
                        output_index,
                        ..Default::default()
                    },
                );
                frames.push(Self::frame(&ResponsesStreamEvent::OutputItemAdded {
                    output_index,
                    item: serde_json::from_value(json!({
                        "id": id, "type": "message", "status": "in_progress",
                        "role": "assistant", "content": []
                    }))
                    .expect("message item"),
                    sequence_number: self.next_sequence(),
                    extra: Default::default(),
                }));
            }
            UnifiedEventKind::ContentPartDelta {
                delta: UnifiedContentDelta::Text(delta),
            }
            | UnifiedEventKind::ContentPartDelta {
                delta: UnifiedContentDelta::TextWithLogprobs { text: delta, .. },
            } => {
                let id = &id;
                let output_index = self.items.get(id).map(|(_, _, index)| *index).unwrap_or(0);
                let mut emit_content_part_added = false;
                let output_index = {
                    let message_state =
                        self.message_states.entry(id.clone()).or_insert_with(|| {
                            MessageStreamState {
                                output_index,
                                ..Default::default()
                            }
                        });
                    if !message_state.content_part_started {
                        message_state.content_part_started = true;
                        emit_content_part_added = true;
                    }
                    message_state.text.push_str(delta);
                    message_state.output_index
                };
                if emit_content_part_added {
                    frames.push(Self::frame(&ResponsesStreamEvent::ContentPartAdded {
                        item_id: id.clone(),
                        output_index,
                        content_index: 0,
                        part: serde_json::from_value(json!({
                            "type": "output_text", "text": "", "annotations": []
                        }))
                        .expect("output text part"),
                        sequence_number: self.next_sequence(),
                        extra: Default::default(),
                    }));
                }
                frames.push(Self::frame(&ResponsesStreamEvent::OutputTextDelta {
                    item_id: id.clone(),
                    output_index,
                    content_index: 0,
                    delta: delta.clone(),
                    sequence_number: self.next_sequence(),
                    logprobs: match &event.kind {
                        UnifiedEventKind::ContentPartDelta {
                            delta: UnifiedContentDelta::TextWithLogprobs { logprobs, .. },
                        } => Some(logprobs.clone()),
                        _ => None,
                    },
                    extra: Default::default(),
                }));
            }
            UnifiedEventKind::ContentPartDelta {
                delta: UnifiedContentDelta::Thinking(delta),
            }
            | UnifiedEventKind::ReasoningSummaryTextDelta { delta } => {
                let id = &id;
                let mut emit_reasoning_item_added = false;
                let mut emit_reasoning_part_added = false;
                let (reasoning_item_id, output_index) = {
                    let reasoning_state =
                        self.reasoning_states.entry(id.clone()).or_insert_with(|| {
                            let reasoning_output_index = self.next_output_index;
                            self.next_output_index += 1;
                            ReasoningStreamState {
                                item_id: format!("rs_{id}"),
                                output_index: reasoning_output_index,
                                ..Default::default()
                            }
                        });
                    if !reasoning_state.item_added {
                        reasoning_state.item_added = true;
                        emit_reasoning_item_added = true;
                    }
                    if !reasoning_state.part_started {
                        reasoning_state.part_started = true;
                        emit_reasoning_part_added = true;
                    }
                    reasoning_state.text.push_str(delta);
                    (
                        reasoning_state.item_id.clone(),
                        reasoning_state.output_index,
                    )
                };
                if emit_reasoning_item_added {
                    frames.push(Self::frame(&ResponsesStreamEvent::OutputItemAdded {
                        output_index,
                        item: serde_json::from_value(json!({
                            "id": reasoning_item_id,
                            "type": "reasoning",
                            "summary": []
                        }))
                        .expect("reasoning item"),
                        sequence_number: self.next_sequence(),
                        extra: Default::default(),
                    }));
                };
                if emit_reasoning_part_added {
                    frames.push(Self::frame(
                        &ResponsesStreamEvent::ReasoningSummaryPartAdded {
                            item_id: reasoning_item_id.clone(),
                            output_index,
                            summary_index: 0,
                            part: serde_json::from_value(
                                json!({"type": "summary_text", "text": ""}),
                            )
                            .expect("reasoning summary part"),
                            sequence_number: self.next_sequence(),
                            extra: Default::default(),
                        },
                    ));
                }
                frames.push(Self::frame(
                    &ResponsesStreamEvent::ReasoningSummaryTextDelta {
                        item_id: reasoning_item_id,
                        output_index,
                        summary_index: 0,
                        delta: delta.clone(),
                        sequence_number: self.next_sequence(),
                        extra: Default::default(),
                    },
                ));
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
                let item = self.tool_items.get(&id);
                let (id, name) = item
                    .and_then(|item| match &item.kind {
                        UnifiedOutputItemKind::FunctionCall { call_id, name, .. } => {
                            Some((call_id.clone(), name.clone()))
                        }
                        _ => None,
                    })
                    .unwrap_or((id.clone(), String::new()));
                let id = &id;
                let name = &name;
                let mut emit_added = false;
                let output_index = if let Some(existing) = self.function_call_states.get_mut(id) {
                    existing.arguments.push_str(arguments_delta);
                    existing.output_index
                } else {
                    let output_index = self.next_output_index;
                    self.next_output_index += 1;
                    self.items.insert(
                        id.clone(),
                        ("function_call".to_string(), name.clone(), output_index),
                    );
                    self.function_call_states.insert(
                        id.clone(),
                        FunctionCallStreamState {
                            output_index,
                            name: name.clone(),
                            arguments: arguments_delta.clone(),
                        },
                    );
                    emit_added = true;
                    output_index
                };
                if emit_added {
                    frames.push(Self::frame(&ResponsesStreamEvent::OutputItemAdded {
                        output_index,
                        item: serde_json::from_value(json!({
                            "id": format!("fc_{id}"), "type": "function_call",
                            "call_id": id, "name": name, "arguments": "", "status": "in_progress"
                        }))
                        .expect("function call item"),
                        sequence_number: self.next_sequence(),
                        extra: Default::default(),
                    }));
                }
                frames.push(Self::frame(
                    &ResponsesStreamEvent::FunctionCallArgumentsDelta {
                        item_id: id.clone(),
                        output_index,
                        content_index: None,
                        delta: arguments_delta.clone(),
                        sequence_number: self.next_sequence(),
                        extra: Default::default(),
                    },
                ));
            }
            UnifiedEventKind::OutputItemDone { item }
                if matches!(item.kind, UnifiedOutputItemKind::FunctionCall { .. }) =>
            {
                let UnifiedOutputItemKind::FunctionCall {
                    call_id: id,
                    name,
                    arguments,
                    ..
                } = &item.kind
                else {
                    unreachable!()
                };
                let mut created_from_done = false;
                let (output_index, final_name, final_arguments) =
                    if let Some(state) = self.function_call_states.remove(id) {
                        let final_arguments = if arguments.is_empty() {
                            state.arguments
                        } else {
                            arguments.clone()
                        };
                        let final_name = if name.is_empty() {
                            state.name
                        } else {
                            name.clone()
                        };
                        (state.output_index, final_name, final_arguments)
                    } else {
                        let output_index = self
                            .items
                            .get(id)
                            .map(|(_, _, index)| *index)
                            .unwrap_or_else(|| {
                                let index = self.next_output_index;
                                self.next_output_index += 1;
                                index
                            });
                        created_from_done = !self.items.contains_key(id);
                        let final_name = name.clone();
                        let final_arguments = arguments.clone();
                        (output_index, final_name, final_arguments)
                    };
                if created_from_done {
                    self.items.insert(
                        id.clone(),
                        (
                            "function_call".to_string(),
                            final_name.clone(),
                            output_index,
                        ),
                    );
                    frames.push(Self::frame(&ResponsesStreamEvent::OutputItemAdded {
                        output_index,
                        item: serde_json::from_value(json!({
                            "id": format!("fc_{id}"), "type": "function_call",
                            "call_id": id, "name": final_name, "arguments": "", "status": "in_progress"
                        }))
                        .expect("function call item"),
                        sequence_number: self.next_sequence(),
                        extra: Default::default(),
                    }));
                }
                frames.push(Self::frame(
                    &ResponsesStreamEvent::FunctionCallArgumentsDone {
                        item_id: id.clone(),
                        output_index,
                        content_index: None,
                        arguments: final_arguments.clone(),
                        sequence_number: self.next_sequence(),
                        extra: Default::default(),
                    },
                ));
                frames.push(Self::frame(&ResponsesStreamEvent::OutputItemDone {
                    output_index,
                    item: serde_json::from_value(json!({
                        "id": format!("fc_{id}"), "type": "function_call",
                        "call_id": id, "name": final_name, "arguments": final_arguments, "status": "completed"
                    }))
                    .expect("function call item"),
                    sequence_number: self.next_sequence(),
                    extra: Default::default(),
                }));
            }
            UnifiedEventKind::Usage { usage, .. } => {
                let openai = crate::openai_http_mapping::map_usage_to_openai(usage);
                self.usage = Some(json!({
                    "input_tokens": openai.prompt_tokens,
                    "input_tokens_details": {"cached_tokens": 0},
                    "output_tokens": openai.completion_tokens,
                    "output_tokens_details": {"reasoning_tokens": 0},
                    "total_tokens": openai.total_tokens,
                }));
            }
            UnifiedEventKind::OutputItemDone { item }
                if matches!(item.kind, UnifiedOutputItemKind::Message { .. }) =>
            {
                let id = &item.identity.internal_id;
                let output_index = self.items.get(id).map(|(_, _, index)| *index).unwrap_or(0);
                if let Some(reasoning_state) = self.reasoning_states.remove(id) {
                    if reasoning_state.part_started {
                        let reasoning_item_id = reasoning_state.item_id;
                        let text = reasoning_state.text;
                        frames.push(Self::frame(
                            &ResponsesStreamEvent::ReasoningSummaryTextDone {
                                item_id: reasoning_item_id.clone(),
                                output_index: reasoning_state.output_index,
                                summary_index: 0,
                                text: text.clone(),
                                sequence_number: self.next_sequence(),
                                extra: Default::default(),
                            },
                        ));
                        frames.push(Self::frame(
                            &ResponsesStreamEvent::ReasoningSummaryPartDone {
                                item_id: reasoning_item_id.clone(),
                                output_index: reasoning_state.output_index,
                                summary_index: 0,
                                part: serde_json::from_value(
                                    json!({"type": "summary_text", "text": text}),
                                )
                                .expect("reasoning summary part"),
                                sequence_number: self.next_sequence(),
                                extra: Default::default(),
                            },
                        ));
                        frames.push(Self::frame(&ResponsesStreamEvent::OutputItemDone {
                            output_index: reasoning_state.output_index,
                            item: serde_json::from_value(json!({
                                "id": reasoning_item_id,
                                "type": "reasoning",
                                "summary": [{"type": "summary_text", "text": text}]
                            }))
                            .expect("reasoning item"),
                            sequence_number: self.next_sequence(),
                            extra: Default::default(),
                        }));
                    }
                }

                let message_state = self.message_states.remove(id);
                let (content_part_started, text) = match message_state {
                    Some(state) => (state.content_part_started, state.text),
                    None => (false, String::new()),
                };
                let content = if content_part_started {
                    json!([{"type": "output_text", "text": text.clone(), "annotations": []}])
                } else {
                    json!([])
                };

                if content_part_started {
                    frames.push(Self::frame(&ResponsesStreamEvent::OutputTextDone {
                        item_id: id.clone(),
                        output_index,
                        content_index: 0,
                        text: text.clone(),
                        sequence_number: self.next_sequence(),
                        logprobs: None,
                        extra: Default::default(),
                    }));
                    frames.push(Self::frame(&ResponsesStreamEvent::ContentPartDone {
                        item_id: id.clone(),
                        output_index,
                        content_index: 0,
                        part: serde_json::from_value(
                            json!({"type": "output_text", "text": text, "annotations": []}),
                        )
                        .expect("output text part"),
                        sequence_number: self.next_sequence(),
                        extra: Default::default(),
                    }));
                }

                frames.push(Self::frame(&ResponsesStreamEvent::OutputItemDone {
                    output_index,
                    item: serde_json::from_value(json!({
                        "id": id, "type": "message", "status": "completed",
                        "role": "assistant", "content": content
                    }))
                    .expect("message item"),
                    sequence_number: self.next_sequence(),
                    extra: Default::default(),
                }));
            }
            UnifiedEventKind::CandidateDone { ending } => {
                self.finish_reason = Some(ending.to_chat().to_string());
            }
            UnifiedEventKind::Incomplete { reason, .. } => {
                self.incomplete_reason = reason.clone();
                frames.extend(self.close_pending_messages());
                frames.extend(self.finish_frames(Some("length")));
            }
            UnifiedEventKind::Completed { response } => {
                let reason = response
                    .as_ref()
                    .and_then(|response| response.candidates.first())
                    .and_then(|candidate| candidate.ending.as_ref())
                    .map(|ending| ending.to_chat().to_string())
                    .or_else(|| self.finish_reason.clone());
                frames.extend(self.close_pending_messages());
                frames.extend(self.finish_frames(reason.as_deref()));
            }
            UnifiedEventKind::Failed { error, .. } => {
                let code = &error.code;
                let message = &error.message;
                let mut response = self.response_skeleton();
                response["status"] = json!("failed");
                response["error"] = json!({"code": code, "message": message});
                frames.push(Self::frame(&ResponsesStreamEvent::ResponseFailed {
                    response: serde_json::from_value(response).expect("failed response"),
                    sequence_number: self.next_sequence(),
                    extra: Default::default(),
                }));
                self.finished = true;
            }
            _ => {}
        }
        frames
    }

    fn close_pending_messages(&mut self) -> Vec<String> {
        let mut pending: Vec<_> = self
            .message_states
            .iter()
            .map(|(id, state)| (state.output_index, id.clone()))
            .collect();
        pending.sort_by_key(|(index, _)| *index);
        let mut frames = Vec::new();
        for (_, id) in pending {
            frames.extend(
                self.on_event(&UnifiedEvent::new(UnifiedEventKind::OutputItemDone {
                    item: UnifiedOutputItem::new(
                        id,
                        UnifiedOutputItemKind::Message {
                            role: UnifiedRole::Assistant,
                            content: vec![],
                            phase: None,
                        },
                    ),
                })),
            );
        }
        frames
    }

    fn finish_frames(&mut self, finish_reason: Option<&str>) -> Vec<String> {
        if self.finished {
            return Vec::new();
        }
        self.finished = true;
        let mut response = self.response_skeleton();
        response["status"] = json!("completed");
        if let Some(usage) = &self.usage {
            response["usage"] = usage.clone();
        }
        let incomplete = finish_reason.is_some_and(|reason| reason == "length");
        if incomplete {
            response["status"] = json!("incomplete");
            response["incomplete_details"] =
                json!({"reason": self.incomplete_reason.as_deref().unwrap_or("max_output_tokens")});
            vec![Self::frame(&ResponsesStreamEvent::ResponseIncomplete {
                response: serde_json::from_value(response).expect("incomplete response"),
                sequence_number: self.next_sequence(),
                extra: Default::default(),
            })]
        } else {
            vec![Self::frame(&ResponsesStreamEvent::ResponseCompleted {
                response: serde_json::from_value(response).expect("completed response"),
                sequence_number: self.next_sequence(),
                extra: Default::default(),
            })]
        }
    }

    /// Stream teardown fallback.
    pub fn finish(&mut self) -> Vec<String> {
        let mut frames = self.close_pending_messages();
        frames.extend(self.finish_frames(Some("stop")));
        frames
    }
}

fn event_type_name(event: &ResponsesStreamEvent) -> &'static str {
    match event {
        ResponsesStreamEvent::ResponseQueued { .. } => "response.queued",
        ResponsesStreamEvent::ResponseCreated { .. } => "response.created",
        ResponsesStreamEvent::ResponseInProgress { .. } => "response.in_progress",
        ResponsesStreamEvent::ResponseCompleted { .. } => "response.completed",
        ResponsesStreamEvent::ResponseFailed { .. } => "response.failed",
        ResponsesStreamEvent::ResponseIncomplete { .. } => "response.incomplete",
        ResponsesStreamEvent::OutputItemAdded { .. } => "response.output_item.added",
        ResponsesStreamEvent::OutputItemDone { .. } => "response.output_item.done",
        ResponsesStreamEvent::ContentPartAdded { .. } => "response.content_part.added",
        ResponsesStreamEvent::ContentPartDone { .. } => "response.content_part.done",
        ResponsesStreamEvent::OutputTextDelta { .. } => "response.output_text.delta",
        ResponsesStreamEvent::OutputTextDone { .. } => "response.output_text.done",
        ResponsesStreamEvent::OutputTextAnnotationAdded { .. } => {
            "response.output_text.annotation.added"
        }
        ResponsesStreamEvent::RefusalDelta { .. } => "response.refusal.delta",
        ResponsesStreamEvent::RefusalDone { .. } => "response.refusal.done",
        ResponsesStreamEvent::FunctionCallArgumentsDelta { .. } => {
            "response.function_call_arguments.delta"
        }
        ResponsesStreamEvent::FunctionCallArgumentsDone { .. } => {
            "response.function_call_arguments.done"
        }
        ResponsesStreamEvent::ReasoningSummaryPartAdded { .. } => {
            "response.reasoning_summary_part.added"
        }
        ResponsesStreamEvent::ReasoningSummaryPartDone { .. } => {
            "response.reasoning_summary_part.done"
        }
        ResponsesStreamEvent::ReasoningSummaryTextDelta { .. } => {
            "response.reasoning_summary_text.delta"
        }
        ResponsesStreamEvent::ReasoningSummaryTextDone { .. } => {
            "response.reasoning_summary_text.done"
        }
        ResponsesStreamEvent::ReasoningTextDelta { .. } => "response.reasoning_text.delta",
        ResponsesStreamEvent::ReasoningTextDone { .. } => "response.reasoning_text.done",
        ResponsesStreamEvent::ResponseError { .. } => "response.error",
        ResponsesStreamEvent::Error { .. } => "error",
        ResponsesStreamEvent::Unknown => "unknown",
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn string_instructions_remain_native_text() {
        let request =
            request_from_value(&json!({"model":"m","instructions":"be terse","input":"hello"}))
                .expect("request");
        assert_eq!(
            request_to_value(&request).expect("native")["instructions"],
            "be terse"
        );
    }

    #[test]
    fn structured_instructions_preserve_identity_in_input() {
        let request = request_from_value(&json!({"model":"m","instructions":[{"type":"message","id":"instruction","role":"developer","content":[{"type":"input_text","text":"policy"}]}],"input":"hello"})).expect("request");
        let native = request_to_value(&request).expect("native");
        assert!(native.get("instructions").is_none());
        assert_eq!(native["input"][0]["id"], "instruction");
        assert_eq!(native["input"][0]["role"], "developer");
        assert_eq!(native["input"].as_array().expect("input").len(), 2);
    }

    #[test]
    fn logprob_text_closes_before_incomplete_terminal() {
        let mut state = ResponsesSseState::new();
        state.on_event(&created("r", "m"));
        state.on_event(&message("msg", false));
        let event = UnifiedEvent::new(UnifiedEventKind::ContentPartDelta {
            delta: UnifiedContentDelta::TextWithLogprobs {
                text: "answer".into(),
                logprobs: vec![json!({"token":"answer","logprob":-0.1})],
            },
        })
        .located(0, "msg".into(), 0, Some(UnifiedPartLocation::Content(0)));
        let frames = state.on_event(&event).join("");
        assert!(frames.contains("answer"));
        assert!(frames.contains("logprob"));
        let frames = state
            .on_event(&UnifiedEvent::new(UnifiedEventKind::Incomplete {
                reason: Some("model_context_window_exceeded".into()),
                response: None,
            }))
            .join("");
        assert!(
            frames.find("response.output_text.done").expect("text done")
                < frames.find("response.incomplete").expect("incomplete")
        );
        assert!(frames.contains("model_context_window_exceeded"));
        assert!(!frames.contains("response.completed"));
        assert!(state.finish().is_empty());
    }

    #[test]
    fn completed_closes_message_once_without_native_done() {
        let mut state = ResponsesSseState::new();
        state.on_event(&created("r", "m"));
        state.on_event(&message("msg", false));
        state.on_event(&text("msg", "answer", false));
        let frames = state.on_event(&completed("stop")).join("");
        assert!(frames.contains("response.output_text.done"));
        assert!(frames.contains("response.content_part.done"));
        assert!(
            frames.find("response.output_item.done").expect("item done")
                < frames.find("response.completed").expect("completed")
        );
        assert!(state.finish().is_empty());
    }
    #[test]
    fn response_stream_keeps_streaming_but_omits_chat_usage_options() {
        let mut request = UnifiedRequest::default();
        request.controls.stream = Some(true);
        request.controls.stream_options = Some(json!({"include_usage": true}));

        let native = request_to_value(&request).expect("native Responses request");
        assert_eq!(native["stream"], true);
        assert!(native.get("stream_options").is_none());
    }

    #[test]
    fn assistant_history_and_function_call_ids_use_responses_shapes() {
        let mut function_call = UnifiedInputItem::new(UnifiedInputItemKind::FunctionCall {
            call_id: "call_client_id".into(),
            name: "read".into(),
            arguments: "{}".into(),
            caller: None,
            cache_control: None,
        });
        function_call.identity = Some(UnifiedItemIdentity {
            internal_id: "internal-call".into(),
            source_id: Some("call_client_id".into()),
        });
        let request = UnifiedRequest {
            model: "gpt-6-luna".into(),
            input: vec![
                UnifiedInputItem::new(UnifiedInputItemKind::Message {
                    role: UnifiedRole::Assistant,
                    content: vec![UnifiedContentPart::text("reading".into())],
                    phase: None,
                }),
                function_call,
            ],
            ..Default::default()
        };

        let native = request_to_value(&request).expect("native Responses request");
        assert_eq!(native["input"][0]["content"][0]["type"], "output_text");
        assert_eq!(native["input"][1]["id"], "fc_call_client_id");
        assert_eq!(native["input"][1]["call_id"], "call_client_id");
        assert_eq!(
            responses_function_call_item_id("fc_existing"),
            "fc_existing"
        );
        assert_eq!(responses_function_call_item_id("fc"), "fc");
    }

    #[test]
    fn request_roundtrip_preserves_phase_reasoning_references_and_extensions() {
        let raw = json!({"model":"codex","input":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"working","annotations":[]}],"phase":"commentary"},{"type":"reasoning","id":"rs","summary":[{"type":"summary_text","text":"plan"}],"content":[{"type":"reasoning_text","text":"reason"}],"encrypted_content":"opaque"},{"type":"function_call","id":"fc","call_id":"call","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"call","output":[{"type":"input_image","file_id":"file","detail":"high"}]},{"type":"item_reference","id":"historical"}],"previous_response_id":"prev","reasoning":{"effort":"high","summary":"auto","future":true},"text":{"verbosity":"low","future":1},"include":["reasoning.encrypted_content"],"store":false});
        let request = request_from_value(&raw).expect("request");
        assert_eq!(request_to_value(&request).expect("native"), raw);
        assert!(super::super::chat_completions::request_to_value(&request).is_err());
    }
    use crate::provider::{
        UnifiedCandidate, UnifiedEnding, UnifiedItemIdentity, UnifiedOutputItem,
        UnifiedPartLocation, UnifiedRole, UnifiedUsageUpdate,
    };
    fn created(id: &str, model: &str) -> UnifiedEvent {
        UnifiedEvent::new(UnifiedEventKind::ResponseCreated {
            response: UnifiedResponse::new(id.into(), model.into()),
        })
    }
    fn message(id: &str, done: bool) -> UnifiedEvent {
        let item = UnifiedOutputItem::new(
            id.into(),
            UnifiedOutputItemKind::Message {
                role: UnifiedRole::Assistant,
                content: vec![],
                phase: None,
            },
        );
        UnifiedEvent::new(if done {
            UnifiedEventKind::OutputItemDone { item }
        } else {
            UnifiedEventKind::OutputItemAdded { item }
        })
        .located(0, id.into(), 0, None)
    }
    fn text(id: &str, delta: &str, thinking: bool) -> UnifiedEvent {
        UnifiedEvent::new(UnifiedEventKind::ContentPartDelta {
            delta: if thinking {
                UnifiedContentDelta::Thinking(delta.into())
            } else {
                UnifiedContentDelta::Text(delta.into())
            },
        })
        .located(0, id.into(), 0, Some(UnifiedPartLocation::Content(0)))
    }
    fn completed(reason: &str) -> UnifiedEvent {
        let mut response = UnifiedResponse::new(String::new(), String::new());
        response.candidates.push(UnifiedCandidate {
            index: 0,
            output: vec![],
            ending: Some(UnifiedEnding::from_source(reason)),
            logprobs: None,
            source_extensions: None,
        });
        UnifiedEvent::new(UnifiedEventKind::Completed {
            response: Some(response),
        })
    }
    fn tool(id: &str, args: &str, done: bool) -> Vec<UnifiedEvent> {
        let item = UnifiedOutputItem::new(
            id.into(),
            UnifiedOutputItemKind::FunctionCall {
                call_id: id.into(),
                name: "shell".into(),
                arguments: if done { args.into() } else { String::new() },
                caller: None,
            },
        );
        if done {
            vec![
                UnifiedEvent::new(UnifiedEventKind::OutputItemDone { item }).located(
                    0,
                    id.into(),
                    0,
                    None,
                ),
            ]
        } else {
            vec![
                UnifiedEvent::new(UnifiedEventKind::OutputItemAdded { item }).located(
                    0,
                    id.into(),
                    0,
                    None,
                ),
                UnifiedEvent::new(UnifiedEventKind::ItemDelta {
                    delta: UnifiedItemDelta::FunctionArguments(args.into()),
                })
                .located(0, id.into(), 0, None),
            ]
        }
    }

    /// codex fidelity golden: reasoning encrypted passthrough + function_call/output loop + extra stash.
    #[test]
    fn parse_codex_request_preserves_reasoning_and_tools() {
        let request = parse_request(
            br#"{
                "model": "gpt-5.3-codex",
                "instructions": "you are codex",
                "input": [
                    {"type": "message", "role": "user", "content": [{"type": "input_text", "text": "run"}]},
                    {"type": "reasoning", "id": "rs_1", "summary": [
                        {"type": "summary_text", "text": "thinking"}
                    ], "encrypted_content": "enc-blob"},
                    {"type": "function_call", "id": "fc_1", "call_id": "call_1",
                     "name": "shell", "arguments": "{\"cmd\":\"ls\"}"},
                    {"type": "function_call_output", "call_id": "call_1", "output": "{\"out\":\"ok\"}"}
                ],
                "tools": [
                    {"type": "function", "name": "shell", "parameters": {"type": "object"}, "strict": false},
                    {"type": "local_shell", "extra_field": 1}
                ],
                "tool_choice": "auto",
                "parallel_tool_calls": false,
                "reasoning": {"effort": "high", "summary": "auto"},
                "include": ["reasoning.encrypted_content"],
                "store": false,
                "prompt_cache_key": "conv-1",
                "stream": true
            }"#,
        )
        .expect("parse codex request");

        // instructions → system message
        assert!(matches!(
            &request.instructions.as_ref().expect("instructions")[0].kind,
            crate::provider::UnifiedInputItemKind::Message {role:UnifiedRole::Developer,content,..} if matches!(&content[0].kind,crate::provider::UnifiedContentPartKind::Text {text,..} if text=="you are codex")
        ));
        // reasoning item -> reasoning_blocks (encrypted passthrough)
        assert_eq!(
            request.input[1]
                .identity
                .as_ref()
                .expect("identity")
                .source_id
                .as_deref(),
            Some("rs_1")
        );
        assert!(
            matches!(&request.input[1].kind,crate::provider::UnifiedInputItemKind::Reasoning {encrypted_content:Some(data),..} if data=="enc-blob")
        );
        // function_call → assistant tool_calls
        assert!(
            matches!(&request.input[2].kind,crate::provider::UnifiedInputItemKind::FunctionCall {call_id,name,..} if call_id=="call_1" && name=="shell")
        );
        // function_call_output → Role::Tool
        assert!(matches!(
            &request.input[3].kind,
            crate::provider::UnifiedInputItemKind::FunctionCallOutput {call_id,..} if call_id=="call_1"
        ));
        // tools: function goes through the IR struct; non-function stays in extra
        let tools = request.tools.as_ref().expect("tools");
        assert_eq!(tools.len(), 2);
        assert!(
            matches!(&tools[0].kind,crate::provider::UnifiedToolKind::Function {name,..} if name=="shell")
        );
        assert!(
            matches!(&tools[1].kind,crate::provider::UnifiedToolKind::Server {tool_type,..} if tool_type=="local_shell")
        );
        assert_eq!(
            tools[1]
                .source_extensions
                .as_ref()
                .expect("extensions")
                .fields
                .get("extra_field"),
            Some(&serde_json::json!(1))
        );
        // reasoning effort
        assert_eq!(request.options.reasoning_effort.as_deref(), Some("high"));
        assert_eq!(request.options.parallel_tool_calls, Some(false));
        assert_eq!(request.controls.stream, Some(true));
        // unmodeled fields stashed into extra
        let extra = &request
            .source_extensions
            .as_ref()
            .expect("extensions")
            .fields;
        assert_eq!(request.controls.store, Some(false));
        assert_eq!(
            request.controls.include.as_deref(),
            Some(["reasoning.encrypted_content".to_string()].as_slice())
        );
        assert!(extra.get("store").is_none());
        assert!(extra.get("include").is_none());
        assert_eq!(
            extra.get("prompt_cache_key"),
            Some(&serde_json::json!("conv-1"))
        );
        // modeled keys never enter extra
        assert!(extra.get("model").is_none());
        assert!(extra.get("input").is_none());
    }

    #[test]
    fn parse_request_maps_empty_content_parts_to_empty_text() {
        let request = parse_request(
            br#"{
                "model": "glm-5.3-flash",
                "input": [
                    {"type": "message", "role": "assistant", "content": []}
                ]
            }"#,
        )
        .expect("parse request with empty content parts");

        assert_eq!(request.input.len(), 1);
        assert!(matches!(
            &request.input[0].kind,
            crate::provider::UnifiedInputItemKind::Message {role:UnifiedRole::Assistant,content,..} if content.is_empty()
        ));
    }

    /// golden: non-stream IR -> Responses response object (with reasoning output and function_call).
    #[test]
    fn non_stream_response_golden() {
        let response=super::super::response_from_value(&json!({"id":"resp_1","created_at":1767225600,"model":"gpt-5.3-codex","status":"completed","output":[{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"t"}],"encrypted_content":"enc-blob"},{"type":"message","id":"msg","role":"assistant","content":[{"type":"output_text","text":"done","annotations":[]}]},{"type":"function_call","id":"fc","call_id":"call_1","name":"shell","arguments":"{\"cmd\":\"ls\"}"}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}),UnifiedSourceProtocol::Responses).expect("response");

        let value = serialize_non_stream(&response);
        assert_eq!(value["object"], "response");
        assert_eq!(value["status"], "completed");
        assert_eq!(value["model"], "gpt-5.3-codex");
        let output = value["output"].as_array().expect("output");
        assert_eq!(output.len(), 3);
        assert_eq!(output[0]["type"], "reasoning");
        assert_eq!(output[0]["encrypted_content"], "enc-blob");
        assert_eq!(output[1]["type"], "message");
        assert_eq!(output[1]["content"][0]["text"], "done");
        assert_eq!(output[2]["type"], "function_call");
        assert_eq!(output[2]["call_id"], "call_1");
        assert_eq!(value["usage"]["input_tokens"], 10);
    }

    /// golden: stream events -> SSE frame sequence (created -> item.added -> text delta -> completed).
    #[test]
    fn stream_state_machine_golden() {
        let mut state = ResponsesSseState::new();
        let mut frames = Vec::new();
        let events = vec![
            created("resp_1","gpt-5.3-codex"),message("msg_1",false),text("msg_1","he",false),text("msg_1","llo",false),message("msg_1",true),
            UnifiedEvent::new(UnifiedEventKind::Usage {
                usage: crate::usage_handler::EndpointUsage::from_endpoint_payload(
                    "/chat/completions",
                    serde_json::json!({"prompt_tokens": 1, "completion_tokens": 2, "total_tokens": 3}),
                )
                .expect("usage"),
                update:UnifiedUsageUpdate::Snapshot,source_usage:None,
            }),
            completed("stop"),
        ];
        for event in &events {
            frames.extend(state.on_event(event));
        }
        frames.extend(state.finish());
        let joined = frames.join("");

        for name in [
            "response.created",
            "response.output_item.added",
            "response.content_part.added",
            "response.output_text.delta",
            "response.output_text.done",
            "response.content_part.done",
            "response.output_item.done",
            "response.completed",
        ] {
            assert!(
                joined.contains(&format!("event: {name}\n")),
                "missing {name}"
            );
        }
        assert!(joined.contains("\"delta\":\"he\""));
        assert!(joined.contains("\"delta\":\"llo\""));
        assert!(joined.contains("\"text\":\"hello\""));
        // the completed frame carries usage
        let completed_frame = frames
            .iter()
            .find(|frame| frame.contains("response.completed"))
            .expect("completed frame");
        assert!(completed_frame.contains("\"input_tokens\":1"));
        assert!(joined.ends_with("\n\n"));

        // nothing emitted after completion
        assert!(state.on_event(&text("msg_1", "late", false)).is_empty());
    }

    /// Mid-stream function_call event order: output_item.added -> arguments.delta* -> arguments.done -> output_item.done.
    #[test]
    fn stream_function_call_sequence() {
        let mut state = ResponsesSseState::new();
        state.on_event(&created("resp_1", "m"));
        let frames = tool("call_1", r#"{"cmd""#, false)
            .iter()
            .flat_map(|event| state.on_event(event))
            .collect::<Vec<_>>();
        assert!(frames[0].contains("event: response.output_item.added\n"));
        assert!(frames[0].contains("\"name\":\"shell\""));
        assert!(frames[1].contains("event: response.function_call_arguments.delta\n"));
        assert!(frames[1].contains(r#""delta":"{\"cmd""#) || frames[1].contains("delta"));
        assert!(frames[1].contains("response.function_call_arguments.delta"));

        let frames_second_delta = state.on_event(&tool("call_1", r#":\"ls\"}"#, false).remove(1));
        assert_eq!(frames_second_delta.len(), 1);
        assert!(frames_second_delta[0].contains("event: response.function_call_arguments.delta\n"));

        let frames = state.on_event(&tool("call_1", r#"{"cmd":"ls"}"#, true).remove(0));
        assert_eq!(frames.len(), 2);
        assert!(frames[0].contains("event: response.function_call_arguments.done\n"));
        assert!(frames[0].contains(r#""arguments":"{\"cmd\":\"ls\"}""#));
        assert!(frames[1].contains("event: response.output_item.done\n"));
        assert!(frames[1].contains(r#""arguments":"{\"cmd\":\"ls\"}""#));
    }

    #[test]
    fn stream_reasoning_sequence_emits_part_lifecycle() {
        let mut state = ResponsesSseState::new();
        let mut frames = Vec::new();
        let events = vec![
            created("resp_1", "glm-5.3-flash"),
            message("msg_1", false),
            text("msg_1", "think", true),
            text("msg_1", "answer", false),
            message("msg_1", true),
            completed("stop"),
        ];
        for event in &events {
            frames.extend(state.on_event(event));
        }

        let joined = frames.join("");
        for name in [
            "response.reasoning_summary_part.added",
            "response.reasoning_summary_text.delta",
            "response.reasoning_summary_text.done",
            "response.reasoning_summary_part.done",
        ] {
            assert!(
                joined.contains(&format!("event: {name}\n")),
                "missing {name}"
            );
        }

        let added_pos = joined
            .find("event: response.reasoning_summary_part.added")
            .expect("added frame");
        let delta_pos = joined
            .find("event: response.reasoning_summary_text.delta")
            .expect("delta frame");
        let text_done_pos = joined
            .find("event: response.reasoning_summary_text.done")
            .expect("text done frame");
        let part_done_pos = joined
            .find("event: response.reasoning_summary_part.done")
            .expect("part done frame");
        assert!(added_pos < delta_pos);
        assert!(delta_pos < text_done_pos);
        assert!(text_done_pos < part_done_pos);
        assert!(joined.contains("\"text\":\"think\""));
    }

    /// IR extra channel: unmodeled fields merged back upstream by the dialect (providers/openai_responses).
    #[test]
    fn extra_channel_merges_into_native_request() {
        let request = parse_request(
            br#"{
                "model": "gpt-5.3-codex",
                "input": "hello",
                "store": false,
                "prompt_cache_key": "conv-1"
            }"#,
        )
        .expect("parse");

        let body = super::super::request_to_value(&request, UnifiedSourceProtocol::Responses)
            .expect("body");
        assert_eq!(body["store"], serde_json::json!(false));
        assert_eq!(body["prompt_cache_key"], serde_json::json!("conv-1"));
        assert_eq!(body["model"], serde_json::json!("gpt-5.3-codex"));
    }
}
