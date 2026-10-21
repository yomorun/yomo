//! OpenAI Responses API support.
//!
//! This module translates chat completions requests into Responses API
//! requests and maps Responses API responses and streaming events back to
//! the unified provider types ([`UnifiedResponse`] / [`UnifiedEvent`]).
//!
//! Reference: `POST /v1/responses` of the OpenAI Responses API.

use std::collections::{HashMap, HashSet};

use axum::http::StatusCode;
use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::llm_provider::{FinishReason, ProviderError, ToolCall, UnifiedEvent, UnifiedResponse};
use crate::model_api_provider::{
    ResponsesInputTokensDetails, ResponsesOutputTokensDetails, ResponsesUsage,
};
use crate::openai_types::{
    ChatCompletionRequest, Content, ContentPart, ErrorDetail, ResponseFormat, Role, ThinkingType,
    ToolChoice, ToolDefinition,
};
use crate::usage_handler::EndpointUsage;

/// Upstream Responses API request body.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ResponsesRequest {
    pub model: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub input: Vec<ResponseInputItem>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub instructions: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_output_tokens: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub top_p: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reasoning: Option<ResponsesReasoning>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub text: Option<ResponsesTextConfig>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tools: Option<Vec<ResponsesFunctionTool>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_choice: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub parallel_tool_calls: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stream: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub service_tier: Option<String>,
}

/// Reasoning configuration for the Responses API.
#[derive(Debug, Clone, Serialize, Deserialize, Default, PartialEq)]
pub struct ResponsesReasoning {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub effort: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub summary: Option<String>,
}

/// Text configuration for the Responses API.
#[derive(Debug, Clone, Serialize, Deserialize, Default, PartialEq)]
pub struct ResponsesTextConfig {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub format: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub verbosity: Option<String>,
}

/// An input item for the Responses API.
///
/// Serialized with an explicit `type` tag; unknown item shapes from a
/// previous turn are preserved round-trip when pass-through is used.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum ResponseInputItem {
    Message {
        role: Role,
        /// `string` or `input_text` / `input_image` parts.
        #[serde(default)]
        content: Value,
    },
    FunctionCall {
        call_id: String,
        name: String,
        #[serde(default)]
        arguments: String,
    },
    FunctionCallOutput {
        call_id: String,
        /// Tool output as plain text or as a list of input content parts.
        output: Value,
    },
}

impl ResponseInputItem {
    fn message(role: Role, content: Value) -> Self {
        Self::Message { role, content }
    }
}

/// A function tool definition for the Responses API.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct ResponsesFunctionTool {
    /// Always `function` for the Responses API.
    pub r#type: String,
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub parameters: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strict: Option<bool>,
}

/// A non-streaming Responses API response body.
#[derive(Debug, Clone, Deserialize, Default)]
#[serde(default)]
pub struct ResponsesResponse {
    pub id: Option<String>,
    pub model: Option<String>,
    pub created_at: Option<f64>,
    pub status: Option<String>,
    pub incomplete_details: Option<Value>,
    pub error: Option<ResponsesError>,
    pub output: Vec<Value>,
    pub usage: Option<Value>,
}

/// An error attached to a Responses API response.
#[derive(Debug, Clone, Deserialize, Default)]
#[serde(default)]
pub struct ResponsesError {
    pub code: Option<String>,
    pub message: Option<String>,
}

/// Upstream protocol used to talk to an OpenAI-compatible endpoint for a
/// given model.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum UpstreamApi {
    ChatCompletions,
    Responses,
}

/// How a provider picks the upstream API for a request.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum UpstreamApiMode {
    /// Select the upstream API from the model name.
    #[default]
    Auto,
    /// Always use the Responses API.
    Responses,
    /// Always use Chat Completions.
    ChatCompletions,
}

/// Parses the version segment of an OpenAI model name, e.g. `"5.6"` from
/// `"gpt-5.6-sol"`.
///
/// Namespaced names such as `"openai/gpt-5.6-sol"` are supported by looking at
/// the last path segment.
fn gpt_model_version(model: &str) -> Option<(u32, u32)> {
    let name = model.trim();
    let name = name.rsplit('/').next().unwrap_or(name);
    let rest = name.strip_prefix("gpt-")?;
    let version: String = rest
        .chars()
        .take_while(|ch| ch.is_ascii_digit() || *ch == '.')
        .collect();
    let mut parts = version.split('.');
    let major = parts.next()?.parse::<u32>().ok()?;
    let minor = parts.next().unwrap_or("0").parse::<u32>().ok()?;
    Some((major, minor))
}

/// Returns `true` for models that only speak the Responses API: GPT-5.6 and
/// later, plus Codex models of the GPT family.
///
/// These are reasoning models that only accept the default sampling
/// parameters, so the translation drops `temperature` / `top_p` for them as
/// well.
fn requires_responses_api(model: &str) -> bool {
    if model.contains("gpt") && model.contains("codex") {
        return true;
    }
    matches!(
        gpt_model_version(model),
        Some((major, minor)) if major > 5 || (major == 5 && minor >= 6)
    )
}

/// Returns `true` when the model accepts the `temperature` / `top_p` sampling
/// parameters.
fn supports_sampling_params(model: &str) -> bool {
    !requires_responses_api(model)
}

/// Selects the upstream API for a model.
pub fn upstream_api_for_model(model: &str) -> UpstreamApi {
    if requires_responses_api(model) {
        UpstreamApi::Responses
    } else {
        UpstreamApi::ChatCompletions
    }
}

/// Resolves the upstream API for a request given the provider configuration.
pub fn resolve_upstream_api(mode: UpstreamApiMode, model: &str) -> UpstreamApi {
    match mode {
        UpstreamApiMode::Auto => upstream_api_for_model(model),
        UpstreamApiMode::Responses => UpstreamApi::Responses,
        UpstreamApiMode::ChatCompletions => UpstreamApi::ChatCompletions,
    }
}

/// Returns `true` when a request must be sent to the Responses API.
///
/// Auto mode stops selecting Responses once the upstream has rejected the
/// Responses endpoint for this provider.
pub fn should_route_to_responses(
    mode: UpstreamApiMode,
    responses_unsupported: bool,
    model: &str,
) -> bool {
    if mode == UpstreamApiMode::Auto && responses_unsupported {
        return false;
    }
    resolve_upstream_api(mode, model) == UpstreamApi::Responses
}

/// Returns `true` when a rejected Responses request may be replayed through
/// Chat Completions.
///
/// An explicit `upstream_api=responses` configuration is never downgraded.
pub fn can_fallback_to_chat_completions(mode: UpstreamApiMode) -> bool {
    mode == UpstreamApiMode::Auto
}

/// Returns `true` when the upstream rejected the Responses API itself rather
/// than the request content, so the request can be replayed via Chat
/// Completions.
///
/// Only "endpoint does not exist" statuses qualify: a `400` means the request
/// itself was rejected and must not be silently replayed on another API.
pub fn is_responses_unsupported_status(status: StatusCode) -> bool {
    matches!(
        status,
        StatusCode::NOT_FOUND | StatusCode::METHOD_NOT_ALLOWED | StatusCode::NOT_IMPLEMENTED
    )
}

/// Metadata of a streamed function call item.
#[derive(Debug, Clone)]
struct FunctionCallMeta {
    /// Client-visible identifier of the call (`call_id` of the Responses API).
    call_id: String,
    /// Function name of the call.
    name: String,
}

/// Accumulates streaming context while translating Responses API SSE events
/// into [`UnifiedEvent`] values.
#[derive(Debug, Default)]
pub struct ResponsesStreamState {
    request_id: String,
    model: String,
    created_at: String,
    started: bool,
    /// Function call metadata keyed by the Responses item id (`fc_...`).
    function_calls: HashMap<String, FunctionCallMeta>,
    /// Tool call ids that already emitted a `ToolCallDone` event.
    completed_tool_calls: HashSet<String>,
}

impl ResponsesStreamState {
    /// Resolves the client-visible tool call id for a Responses item.
    ///
    /// The Responses API identifies a call twice: the item id (`fc_...`) and
    /// the call id (`call_...`). Only the call id is exposed to clients, and
    /// argument events reference the item id, so the item id is translated
    /// back to the call id here.
    fn tool_call_id(&self, item_id: Option<&str>, call_id: Option<&str>) -> String {
        if let Some(call_id) = call_id.map(str::trim).filter(|id| !id.is_empty()) {
            return call_id.to_string();
        }
        let Some(item_id) = item_id.map(str::trim).filter(|id| !id.is_empty()) else {
            return String::new();
        };
        match self.function_calls.get(item_id) {
            Some(meta) if !meta.call_id.is_empty() => meta.call_id.clone(),
            _ => item_id.to_string(),
        }
    }

    /// Resolves the function name of a streamed function call item.
    fn tool_call_name(&self, item_id: Option<&str>) -> String {
        item_id
            .and_then(|item_id| self.function_calls.get(item_id))
            .map(|meta| meta.name.clone())
            .unwrap_or_default()
    }

    /// Marks a tool call as completed, returning `false` when it was already
    /// reported.
    fn mark_tool_call_completed(&mut self, id: &str) -> bool {
        self.completed_tool_calls.insert(id.to_string())
    }
}

/// Translates a chat completions request into a Responses API request.
///
/// Content parts of user messages are mapped to the Responses API input
/// content types (`input_text`, `input_image`, `input_audio`, `input_file`).
/// Tool messages keep their text as a string and switch to a list of input
/// content parts when the tool returned media, which is the shape the
/// Responses API expects for multimodal tool results.
///
/// Assistant messages are sent as plain string content on purpose: the
/// Responses API validates content part types against the message role and
/// only accepts `output_text` and `refusal` parts for assistant messages, so
/// a string content avoids that validation entirely. Media parts in
/// assistant, system and developer messages are rejected instead of being
/// dropped silently.
pub fn chat_to_responses_request(
    request: &ChatCompletionRequest,
) -> Result<ResponsesRequest, ProviderError> {
    if request.stop.is_some() {
        return Err(invalid_request(
            "stop is not supported by the responses api",
        ));
    }

    let mut input = Vec::new();
    let mut instructions: Vec<String> = Vec::new();

    for message in &request.messages {
        match message.role {
            Role::System | Role::Developer => {
                ensure_text_only(&message.content, message.role.as_str())?;
                let Some(text) = message_text(&message.content) else {
                    // Empty system messages carry no information.
                    continue;
                };
                if input.is_empty() {
                    // Leading system messages become the Responses
                    // `instructions` field, which keeps the usual system
                    // prompt semantics.
                    instructions.push(text);
                } else {
                    // System messages that appear inside the conversation keep
                    // their position as input items.
                    input.push(ResponseInputItem::message(
                        message.role.clone(),
                        Value::String(text),
                    ));
                }
            }
            Role::User => {
                input.push(ResponseInputItem::message(
                    Role::User,
                    message_content_value(&message.content)?,
                ));
            }
            Role::Assistant => {
                ensure_text_only(&message.content, "assistant")?;
                if let Some(text) = message_text(&message.content) {
                    input.push(ResponseInputItem::message(
                        Role::Assistant,
                        Value::String(text),
                    ));
                }
                if let Some(tool_calls) = &message.tool_calls {
                    for call in tool_calls {
                        input.push(ResponseInputItem::FunctionCall {
                            call_id: call.id.clone().unwrap_or_default(),
                            name: call.function.name.clone(),
                            arguments: call.function.arguments.clone(),
                        });
                    }
                }
            }
            Role::Tool => {
                let output = tool_output_value(&message.content)?;
                input.push(ResponseInputItem::FunctionCallOutput {
                    call_id: message.tool_call_id.clone().unwrap_or_default(),
                    output,
                });
            }
        }
    }

    if input.is_empty() {
        return Err(invalid_request(
            "responses api requires at least one user or assistant message",
        ));
    }

    let reasoning_effort = request
        .reasoning_effort
        .clone()
        .map(|effort| ResponsesReasoning {
            effort: Some(effort),
            summary: None,
        });
    let reasoning_disabled = request
        .thinking
        .as_ref()
        .is_some_and(|thinking| thinking.kind == ThinkingType::Disabled);
    let supports_sampling = supports_sampling_params(&request.model);

    Ok(ResponsesRequest {
        model: request.model.clone(),
        input,
        instructions: (!instructions.is_empty()).then(|| instructions.join("\n\n")),
        max_output_tokens: request.max_completion_tokens,
        // GPT-5.6 and later only accept the default sampling parameters.
        temperature: if supports_sampling {
            request.temperature
        } else {
            None
        },
        top_p: if supports_sampling {
            request.top_p
        } else {
            None
        },
        reasoning: if reasoning_disabled {
            Some(ResponsesReasoning {
                effort: Some("none".to_string()),
                summary: None,
            })
        } else {
            reasoning_effort
        },
        text: Some(ResponsesTextConfig {
            format: response_format_value(request.response_format.as_ref()),
            verbosity: request.verbosity.clone(),
        }),
        tools: request
            .tools
            .as_ref()
            .map(|tools| tools.iter().map(function_tool).collect()),
        tool_choice: request.tool_choice.as_ref().map(tool_choice_value),
        parallel_tool_calls: request.parallel_tool_calls,
        stream: request.stream,
        service_tier: request.service_tier.clone(),
    })
}

fn message_text(content: &Content) -> Option<String> {
    match content {
        Content::Text(text) if !text.is_empty() => Some(text.clone()),
        Content::Text(_) => None,
        Content::Parts(parts) => {
            let mut joined = String::new();
            for part in parts {
                if let ContentPart::Text { text } = part {
                    joined.push_str(text);
                }
            }
            (!joined.is_empty()).then_some(joined)
        }
    }
}

/// Ensures a message only carries text parts.
///
/// The Responses API restricts the content parts of a message by role: an
/// assistant message only accepts `output_text` and `refusal` parts, and media
/// parts belong to user messages. Non-text parts are therefore rejected
/// instead of being dropped silently.
fn ensure_text_only(content: &Content, role: &str) -> Result<(), ProviderError> {
    if let Content::Parts(parts) = content {
        for part in parts {
            if !matches!(part, ContentPart::Text { .. }) {
                return Err(invalid_request(format!(
                    "{role} messages do not support image, audio or file parts in the responses api; move the media part to a user message"
                )));
            }
        }
    }
    Ok(())
}

fn message_content_value(content: &Content) -> Result<Value, ProviderError> {
    match content {
        Content::Text(text) => Ok(Value::String(text.clone())),
        Content::Parts(parts) => {
            let mut values = Vec::new();
            for part in parts {
                values.push(message_content_part_value(part)?);
            }
            Ok(Value::Array(values))
        }
    }
}

/// Maps a single chat completions content part to a Responses API input
/// content part.
fn message_content_part_value(part: &ContentPart) -> Result<Value, ProviderError> {
    let value = match part {
        ContentPart::Text { text } => serde_json::json!({
            "type": "input_text",
            "text": text,
        }),
        ContentPart::Image { image_url } => {
            let mut value = serde_json::json!({
                "type": "input_image",
                "image_url": image_url.url,
            });
            if let Some(detail) = &image_url.detail {
                value["detail"] = Value::String(detail.clone());
            }
            value
        }
        ContentPart::InputAudio { input_audio } => serde_json::json!({
            "type": "input_audio",
            "input_audio": {
                "data": input_audio.data,
                "format": input_audio.format,
            },
        }),
        ContentPart::File { .. } => file_content_value(part)?,
    };
    Ok(value)
}

/// Maps a tool message content to a `function_call_output` payload.
///
/// The Responses API accepts either plain text or a list of input content
/// parts, which lets a tool return images or files back to the model.
fn tool_output_value(content: &Content) -> Result<Value, ProviderError> {
    let Content::Parts(parts) = content else {
        return Ok(Value::String(match content {
            Content::Text(text) => text.clone(),
            Content::Parts(_) => String::new(),
        }));
    };

    let has_media = parts
        .iter()
        .any(|part| !matches!(part, ContentPart::Text { .. }));
    if !has_media {
        // Plain text output stays a string for maximum upstream compatibility.
        return Ok(Value::String(message_text(content).unwrap_or_default()));
    }

    let mut values = Vec::new();
    for part in parts {
        values.push(message_content_part_value(part)?);
    }
    Ok(Value::Array(values))
}

/// Maps a chat completions file content part to a Responses API `input_file`
/// part.
///
/// The Chat Completions shape nests the file fields in a `file` object, while
/// the Responses API expects them flat next to the `type` tag and accepts only
/// one source per part: `file_id`, `file_url`, or `filename` with `file_data`.
/// The `mime_type` field has no Responses counterpart and is dropped; the
/// media type is part of the `file_data` data URL.
fn file_content_value(part: &ContentPart) -> Result<Value, ProviderError> {
    let ContentPart::File {
        file,
        file_id,
        file_data,
        filename,
        file_url,
        ..
    } = part
    else {
        return Err(invalid_request("file content part is required"));
    };

    // The deserializer accepts both the nested `file` object and the flat
    // form, so both sources are read.
    let file_id = file.file_id.clone().or_else(|| file_id.clone());
    let file_data = file.file_data.clone().or_else(|| file_data.clone());
    let filename = file.filename.clone().or_else(|| filename.clone());
    let file_url = file.file_url.clone().or_else(|| file_url.clone());

    let mut value = serde_json::json!({ "type": "input_file" });

    match (
        file_id.filter(|id| !id.trim().is_empty()),
        file_url.filter(|url| !url.trim().is_empty()),
        filename.filter(|name| !name.trim().is_empty()),
        file_data.filter(|data| !data.trim().is_empty()),
    ) {
        (Some(file_id), None, None, None) => {
            value["file_id"] = Value::String(file_id);
        }
        (None, Some(file_url), None, None) => {
            value["file_url"] = Value::String(file_url);
        }
        (None, None, Some(filename), Some(file_data)) => {
            value["filename"] = Value::String(filename);
            value["file_data"] = Value::String(file_data);
        }
        (None, None, None, Some(_)) => {
            return Err(invalid_request(
                "file content part with file_data requires a filename",
            ));
        }
        _ => {
            return Err(invalid_request(
                "file content part must provide exactly one of file_id, file_url or filename with file_data",
            ));
        }
    }

    Ok(value)
}

fn response_format_value(format: Option<&ResponseFormat>) -> Option<Value> {
    match format {
        Some(ResponseFormat::Text) => Some(serde_json::json!({ "type": "text" })),
        Some(ResponseFormat::JsonObject) => Some(serde_json::json!({ "type": "json_object" })),
        Some(ResponseFormat::JsonSchema { json_schema }) => Some(serde_json::json!({
            "type": "json_schema",
            "name": json_schema.name,
            "schema": json_schema.schema,
            "strict": json_schema.strict,
        })),
        None => None,
    }
}

fn function_tool(tool: &ToolDefinition) -> ResponsesFunctionTool {
    ResponsesFunctionTool {
        r#type: "function".to_string(),
        name: tool.function.name.clone(),
        description: tool.function.description.clone(),
        parameters: match &tool.function.parameters {
            Value::Null => None,
            parameters => Some(parameters.clone()),
        },
        strict: tool.function.strict,
    }
}

fn tool_choice_value(choice: &ToolChoice) -> Value {
    match choice {
        ToolChoice::Name(name) => match name.as_str() {
            "none" | "auto" | "required" => Value::String(name.clone()),
            _ => serde_json::json!({ "type": "function", "name": name }),
        },
        ToolChoice::Object { function, .. } => {
            serde_json::json!({ "type": "function", "name": function.name })
        }
    }
}

fn invalid_request(message: impl Into<String>) -> ProviderError {
    ProviderError::Public {
        status: StatusCode::BAD_REQUEST,
        error: ErrorDetail {
            message: message.into(),
            r#type: "invalid_request_error".to_string(),
            code: None,
            param: None,
        },
    }
}

/// Maps a non-streaming Responses API response into a [`UnifiedResponse`].
pub fn map_responses_to_unified(
    response: ResponsesResponse,
) -> Result<UnifiedResponse, ProviderError> {
    if let Some(error) = &response.error {
        return Err(ProviderError::internal(format!(
            "responses error: {}",
            error
                .message
                .clone()
                .unwrap_or_else(|| error.code.clone().unwrap_or_else(|| "unknown".to_string()))
        )));
    }

    let mut output_text = String::new();
    let mut reasoning: Vec<String> = Vec::new();
    let mut tool_calls = Vec::new();
    for item in &response.output {
        let item_type = item.get("type").and_then(Value::as_str).unwrap_or_default();
        match item_type {
            "message" => {
                output_text.push_str(&output_message_text(item));
            }
            "reasoning" => {
                if let Some(texts) = reasoning_texts(item) {
                    reasoning.push(texts);
                }
            }
            "function_call" => {
                tool_calls.push(function_call_tool(item)?);
            }
            _ => {}
        }
    }

    let finish_reason = map_response_finish_reason(&response, !tool_calls.is_empty());
    let usage = responses_usage(response.usage.as_ref());

    Ok(UnifiedResponse {
        request_id: response.id.clone().unwrap_or_default(),
        created_at: response_created_at(response.created_at),
        model: response.model.clone().unwrap_or_default(),
        output_text,
        reasoning_content: (!reasoning.is_empty())
            .then(|| reasoning.join("\n"))
            .filter(|content| !content.is_empty()),
        tool_calls: (!tool_calls.is_empty()).then_some(tool_calls),
        finish_reason,
        usage,
    })
}

fn output_message_text(item: &Value) -> String {
    let Some(content) = item.get("content").and_then(Value::as_array) else {
        return String::new();
    };
    content
        .iter()
        .filter(|part| part.get("type").and_then(Value::as_str) == Some("output_text"))
        .filter_map(|part| part.get("text").and_then(Value::as_str))
        .collect()
}

fn reasoning_texts(item: &Value) -> Option<String> {
    let summary = item
        .get("summary")
        .and_then(Value::as_array)
        .map(|parts| {
            parts
                .iter()
                .filter(|part| part.get("type").and_then(Value::as_str) == Some("summary_text"))
                .filter_map(|part| part.get("text").and_then(Value::as_str))
                .collect::<Vec<_>>()
                .join("\n")
        })
        .unwrap_or_default();
    let content = item
        .get("content")
        .and_then(Value::as_array)
        .map(|parts| {
            parts
                .iter()
                .filter(|part| part.get("type").and_then(Value::as_str) == Some("reasoning_text"))
                .filter_map(|part| part.get("text").and_then(Value::as_str))
                .collect::<Vec<_>>()
                .join("\n")
        })
        .unwrap_or_default();

    let text = if summary.is_empty() { content } else { summary };
    (!text.is_empty()).then_some(text)
}

fn function_call_tool(item: &Value) -> Result<ToolCall, ProviderError> {
    Ok(ToolCall {
        id: item
            .get("call_id")
            .and_then(Value::as_str)
            .map(str::to_string),
        name: item
            .get("name")
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_string(),
        description: String::new(),
        arguments: item
            .get("arguments")
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_string(),
    })
}

fn map_response_finish_reason(response: &ResponsesResponse, has_tool_calls: bool) -> FinishReason {
    if response.status.as_deref() == Some("incomplete") {
        let reason = response
            .incomplete_details
            .as_ref()
            .and_then(|details| details.get("reason"))
            .and_then(Value::as_str)
            .unwrap_or_default();
        return match reason {
            "max_output_tokens" => FinishReason::Length,
            "content_filter" => FinishReason::ContentFilter,
            _ => FinishReason::Other,
        };
    }

    if has_tool_calls {
        FinishReason::ToolCalls
    } else {
        FinishReason::Stop
    }
}

/// Builds the usage of a Responses API response.
///
/// The token counts always carry a value so the usage survives the round trip
/// through the chat completions usage shape; an upstream that does not report
/// usage is reported as zero consumed tokens.
fn responses_usage(usage: Option<&Value>) -> EndpointUsage {
    let parsed = usage
        .and_then(|usage| serde_json::from_value::<ResponsesUsageShape>(usage.clone()).ok())
        .unwrap_or_default();
    let input_tokens = parsed.input_tokens.unwrap_or(0);
    let output_tokens = parsed.output_tokens.unwrap_or(0);
    let total_tokens = parsed
        .total_tokens
        .unwrap_or_else(|| input_tokens.saturating_add(output_tokens));
    EndpointUsage::Responses(ResponsesUsage {
        input_tokens: Some(input_tokens),
        input_tokens_details: parsed.input_tokens_details.map(|details| {
            ResponsesInputTokensDetails {
                cached_tokens: details.cached_tokens,
            }
        }),
        output_tokens: Some(output_tokens),
        output_tokens_details: parsed.output_tokens_details.map(|details| {
            ResponsesOutputTokensDetails {
                reasoning_tokens: details.reasoning_tokens,
            }
        }),
        total_tokens: Some(total_tokens),
    })
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(default)]
struct ResponsesUsageShape {
    input_tokens: Option<i64>,
    input_tokens_details: Option<ResponsesUsageInputDetails>,
    output_tokens: Option<i64>,
    output_tokens_details: Option<ResponsesUsageOutputDetails>,
    total_tokens: Option<i64>,
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(default)]
struct ResponsesUsageInputDetails {
    cached_tokens: Option<i64>,
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(default)]
struct ResponsesUsageOutputDetails {
    reasoning_tokens: Option<i64>,
}

fn response_created_at(created_at: Option<f64>) -> String {
    let timestamp = created_at.map(|ts| ts as i64).filter(|ts| *ts > 0);
    timestamp
        .and_then(|ts| chrono::DateTime::from_timestamp(ts, 0).map(|dt| dt.to_rfc3339()))
        .unwrap_or_else(|| chrono::Utc::now().to_rfc3339())
}

/// Summary of the `response` object carried by streaming terminal events.
#[derive(Debug, Deserialize, Default)]
#[serde(default)]
struct StreamResponseSummary {
    id: Option<String>,
    model: Option<String>,
    created_at: Option<f64>,
    status: Option<String>,
    output: Vec<Value>,
    usage: Option<Value>,
    error: Option<ResponsesError>,
    incomplete_details: Option<Value>,
}

/// Translates one Responses API SSE `data` payload into unified events.
pub fn map_stream_event(
    data: &str,
    state: &mut ResponsesStreamState,
) -> Result<Vec<UnifiedEvent>, ProviderError> {
    #[derive(Debug, Deserialize)]
    struct StreamEvent {
        #[serde(rename = "type")]
        kind: String,
        #[serde(default)]
        response: Option<StreamResponseSummary>,
        #[serde(default)]
        item: Option<Value>,
        #[serde(default)]
        item_id: Option<String>,
        #[serde(default)]
        call_id: Option<String>,
        #[serde(default)]
        delta: Option<String>,
        #[serde(default)]
        arguments: Option<String>,
        #[serde(default)]
        code: Option<String>,
        #[serde(default)]
        message: Option<String>,
    }

    let event: StreamEvent = serde_json::from_str(data)
        .map_err(|err| ProviderError::internal(format!("invalid responses stream: {err}")))?;

    if state.request_id.is_empty() {
        if let Some(response) = &event.response {
            if let Some(id) = response.id.as_deref().filter(|id| !id.trim().is_empty()) {
                state.request_id = id.to_string();
                state.model = response.model.clone().unwrap_or_default();
                state.created_at = response_created_at(response.created_at);
            }
        }
    }

    match event.kind.as_str() {
        "error" => Err(responses_stream_error(
            event.code.as_deref(),
            event.message.as_deref(),
        )),
        "response.failed" => {
            let error = event
                .response
                .as_ref()
                .and_then(|response| response.error.as_ref());
            Err(responses_stream_error(
                error.and_then(|error| error.code.as_deref()),
                error.and_then(|error| error.message.as_deref()),
            ))
        }
        "response.created" | "response.in_progress" => {
            if state.started {
                return Ok(Vec::new());
            }
            state.started = true;
            Ok(vec![
                UnifiedEvent::ResponseCreated {
                    id: state.request_id.clone(),
                    model: state.model.clone(),
                    created_at: state.created_at.clone(),
                },
                UnifiedEvent::ResponseInProgress {
                    id: state.request_id.clone(),
                    model: state.model.clone(),
                    created_at: state.created_at.clone(),
                },
                UnifiedEvent::MessageStart {
                    id: state.request_id.clone(),
                    role: "assistant".to_string(),
                },
            ])
        }
        "response.output_text.delta" => Ok(to_message_delta(&state.request_id, event.delta)),
        "response.reasoning_summary_text.delta" | "response.reasoning_text.delta" => {
            let Some(delta) = event.delta else {
                return Ok(Vec::new());
            };
            Ok(vec![UnifiedEvent::ThinkingDelta {
                id: state.request_id.clone(),
                delta,
            }])
        }
        "response.output_item.added" => {
            let Some(item) = event.item else {
                return Ok(Vec::new());
            };
            match item.get("type").and_then(Value::as_str).unwrap_or_default() {
                "function_call" => {
                    let item_id = item
                        .get("id")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_string();
                    let call_id = item
                        .get("call_id")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_string();
                    let name = item
                        .get("name")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_string();
                    if !item_id.is_empty() {
                        state.function_calls.insert(
                            item_id.clone(),
                            FunctionCallMeta {
                                call_id: call_id.clone(),
                                name: name.clone(),
                            },
                        );
                    }
                    let id = state.tool_call_id(Some(&item_id), Some(&call_id));
                    if id.is_empty() {
                        return Ok(Vec::new());
                    }
                    Ok(vec![UnifiedEvent::ToolCallDelta {
                        id,
                        name,
                        arguments_delta: String::new(),
                    }])
                }
                _ => Ok(Vec::new()),
            }
        }
        "response.function_call_arguments.delta" => {
            let id = state.tool_call_id(event.item_id.as_deref(), event.call_id.as_deref());
            if id.is_empty() {
                return Ok(Vec::new());
            }
            let name = state.tool_call_name(event.item_id.as_deref());
            Ok(to_tool_call_delta(&id, &name, event.delta.as_deref()))
        }
        "response.function_call_arguments.done" => {
            let id = state.tool_call_id(event.item_id.as_deref(), event.call_id.as_deref());
            if id.is_empty() {
                return Ok(Vec::new());
            }
            if !state.mark_tool_call_completed(&id) {
                return Ok(Vec::new());
            }
            let name = state.tool_call_name(event.item_id.as_deref());
            Ok(vec![UnifiedEvent::ToolCallDone {
                id,
                name,
                arguments: event.arguments.unwrap_or_default(),
            }])
        }
        "response.output_item.done" => {
            let Some(item) = event.item else {
                return Ok(Vec::new());
            };
            match item.get("type").and_then(Value::as_str).unwrap_or_default() {
                "message" => Ok(vec![UnifiedEvent::MessageStop {
                    id: state.request_id.clone(),
                    stop_reason: None,
                }]),
                "reasoning" => {
                    let summary = reasoning_texts(&item);
                    Ok(vec![UnifiedEvent::ThinkingDone {
                        id: state.request_id.clone(),
                        summary,
                    }])
                }
                "function_call" => {
                    let item_id = item.get("id").and_then(Value::as_str);
                    let call_id = item.get("call_id").and_then(Value::as_str);
                    let id = state.tool_call_id(item_id, call_id);
                    if id.is_empty() {
                        return Ok(Vec::new());
                    }
                    if !state.mark_tool_call_completed(&id) {
                        return Ok(Vec::new());
                    }
                    Ok(vec![UnifiedEvent::ToolCallDone {
                        id,
                        name: item
                            .get("name")
                            .and_then(Value::as_str)
                            .unwrap_or_default()
                            .to_string(),
                        arguments: item
                            .get("arguments")
                            .and_then(Value::as_str)
                            .unwrap_or_default()
                            .to_string(),
                    }])
                }
                _ => Ok(Vec::new()),
            }
        }
        "response.completed" | "response.incomplete" => {
            let Some(response) = &event.response else {
                return Ok(Vec::new());
            };
            let mut events = Vec::new();
            let has_tool_calls = response
                .output
                .iter()
                .any(|item| item.get("type").and_then(Value::as_str) == Some("function_call"));
            // Successful and incomplete responses always report usage.
            events.push(UnifiedEvent::Usage {
                usage: responses_usage(response.usage.as_ref()),
            });
            match event.kind.as_str() {
                "response.incomplete" => events.push(UnifiedEvent::Completed {
                    finish_reason: Some(incomplete_finish_reason(response)),
                }),
                _ => events.push(UnifiedEvent::Completed {
                    finish_reason: Some(finish_reason_string(has_tool_calls)),
                }),
            }
            Ok(events)
        }
        _ => Ok(Vec::new()),
    }
}

/// Returns a stream error rather than an event so agent loops stop before
/// executing any tool calls collected from a failed response.
fn responses_stream_error(code: Option<&str>, message: Option<&str>) -> ProviderError {
    let code = code
        .filter(|code| !code.trim().is_empty())
        .unwrap_or("error");
    let message = message
        .filter(|message| !message.trim().is_empty())
        .unwrap_or("response failed");
    ProviderError::internal(format!("responses stream error: {code}: {message}"))
}

fn to_message_delta(id: &str, delta: Option<String>) -> Vec<UnifiedEvent> {
    delta
        .filter(|delta| !delta.is_empty())
        .map(|delta| {
            vec![UnifiedEvent::MessageDelta {
                id: id.to_string(),
                delta,
            }]
        })
        .unwrap_or_default()
}

fn to_tool_call_delta(id: &str, name: &str, delta: Option<&str>) -> Vec<UnifiedEvent> {
    delta
        .filter(|delta| !delta.is_empty())
        .map(|delta| {
            vec![UnifiedEvent::ToolCallDelta {
                id: id.to_string(),
                name: name.to_string(),
                arguments_delta: delta.to_string(),
            }]
        })
        .unwrap_or_default()
}

fn incomplete_finish_reason(response: &StreamResponseSummary) -> String {
    let details = response.incomplete_details.as_ref();
    let reason = details
        .and_then(|details| details.get("reason"))
        .and_then(Value::as_str)
        .unwrap_or_default();
    match reason {
        "max_output_tokens" => "length".to_string(),
        "content_filter" => "content_filter".to_string(),
        _ => "stop".to_string(),
    }
}

fn finish_reason_string(has_tool_calls: bool) -> String {
    if has_tool_calls {
        "tool_calls".to_string()
    } else {
        "stop".to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::openai_types::{Content, Message, Role};

    #[test]
    fn upstream_api_selection_for_models() {
        assert_eq!(
            upstream_api_for_model("gpt-5.6-sol"),
            UpstreamApi::Responses
        );
        assert_eq!(upstream_api_for_model("gpt-6"), UpstreamApi::Responses);
        assert_eq!(
            upstream_api_for_model("gpt-5.3-codex"),
            UpstreamApi::Responses
        );
        assert_eq!(
            upstream_api_for_model("gpt-5.6-codex"),
            UpstreamApi::Responses
        );
        assert_eq!(
            upstream_api_for_model("openai/gpt-5.6-sol"),
            UpstreamApi::Responses
        );
        assert_eq!(
            upstream_api_for_model("codex-mini-latest"),
            UpstreamApi::ChatCompletions
        );
        assert_eq!(
            upstream_api_for_model("gpt-5.5"),
            UpstreamApi::ChatCompletions
        );
        assert_eq!(
            upstream_api_for_model("gpt-4.1-mini"),
            UpstreamApi::ChatCompletions
        );
        assert_eq!(
            upstream_api_for_model("gpt-4o"),
            UpstreamApi::ChatCompletions
        );
        assert_eq!(
            upstream_api_for_model("claude-sonnet-5"),
            UpstreamApi::ChatCompletions
        );
        assert_eq!(upstream_api_for_model(""), UpstreamApi::ChatCompletions);
    }

    #[test]
    fn resolve_upstream_api_honors_explicit_mode() {
        assert_eq!(
            resolve_upstream_api(UpstreamApiMode::Responses, "gpt-4o"),
            UpstreamApi::Responses
        );
        assert_eq!(
            resolve_upstream_api(UpstreamApiMode::ChatCompletions, "gpt-5.6-sol"),
            UpstreamApi::ChatCompletions
        );
        assert_eq!(
            resolve_upstream_api(UpstreamApiMode::Auto, "gpt-5.6-sol"),
            UpstreamApi::Responses
        );
    }

    #[test]
    fn responses_routing_respects_mode_and_unsupported_flag() {
        assert!(should_route_to_responses(
            UpstreamApiMode::Auto,
            false,
            "gpt-6.1-sol"
        ));
        // Once the upstream rejected Responses, auto mode stops retrying it.
        assert!(!should_route_to_responses(
            UpstreamApiMode::Auto,
            true,
            "gpt-6.1-sol"
        ));
        assert!(!should_route_to_responses(
            UpstreamApiMode::ChatCompletions,
            false,
            "gpt-6.1-sol"
        ));
        assert!(should_route_to_responses(
            UpstreamApiMode::Responses,
            false,
            "gpt-4o"
        ));

        assert!(can_fallback_to_chat_completions(UpstreamApiMode::Auto));
        // An explicit Responses configuration is never downgraded.
        assert!(!can_fallback_to_chat_completions(
            UpstreamApiMode::Responses
        ));
        assert!(!can_fallback_to_chat_completions(
            UpstreamApiMode::ChatCompletions
        ));
    }

    #[test]
    fn responses_unsupported_status_covers_replay_conditions() {
        assert!(is_responses_unsupported_status(StatusCode::NOT_FOUND));
        assert!(is_responses_unsupported_status(
            StatusCode::METHOD_NOT_ALLOWED
        ));
        assert!(is_responses_unsupported_status(StatusCode::NOT_IMPLEMENTED));
        // A rejected request must not be replayed on another API.
        assert!(!is_responses_unsupported_status(StatusCode::BAD_REQUEST));
        assert!(!is_responses_unsupported_status(StatusCode::UNAUTHORIZED));
        assert!(!is_responses_unsupported_status(
            StatusCode::TOO_MANY_REQUESTS
        ));
    }

    fn chat_request(messages: Vec<Message>) -> ChatCompletionRequest {
        ChatCompletionRequest {
            model: "gpt-5.6-sol".to_string(),
            messages,
            ..Default::default()
        }
    }

    fn user_message(text: &str) -> Message {
        Message {
            role: Role::User,
            content: Content::Text(text.to_string()),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: None,
        }
    }

    #[test]
    fn rejects_stop_instead_of_silently_dropping_it() {
        for stop in [vec!["END".to_string()], Vec::new()] {
            let mut request = chat_request(vec![user_message("hi")]);
            request.stop = Some(stop);

            let error = chat_to_responses_request(&request).expect_err("stop must be rejected");

            let ProviderError::Public { status, error } = error else {
                panic!("expected public bad request");
            };
            assert_eq!(status, StatusCode::BAD_REQUEST);
            assert_eq!(error.r#type, "invalid_request_error");
            assert!(error.message.contains("stop is not supported"));
        }
    }

    #[test]
    fn translates_plain_text_conversation() {
        let request = chat_request(vec![
            Message {
                role: Role::System,
                content: Content::Text("be terse".to_string()),
                reasoning_content: None,
                tool_call_id: None,
                tool_calls: None,
            },
            user_message("hi"),
            Message {
                role: Role::Assistant,
                content: Content::Text("hello".to_string()),
                reasoning_content: None,
                tool_call_id: None,
                tool_calls: None,
            },
        ]);

        let translated = chat_to_responses_request(&request).expect("translation");

        assert_eq!(translated.input.len(), 2);
        assert_eq!(
            translated.input[0],
            ResponseInputItem::Message {
                role: Role::User,
                content: Value::String("hi".to_string()),
            }
        );
        assert_eq!(
            translated.input[1],
            ResponseInputItem::Message {
                role: Role::Assistant,
                content: Value::String("hello".to_string()),
            }
        );
        assert_eq!(translated.instructions.as_deref(), Some("be terse"));
        assert_eq!(translated.reasoning, None);
    }

    #[test]
    fn keeps_system_messages_inside_the_conversation_as_input_items() {
        let request = chat_request(vec![
            user_message("hi"),
            Message {
                role: Role::System,
                content: Content::Text("be brief".to_string()),
                reasoning_content: None,
                tool_call_id: None,
                tool_calls: None,
            },
        ]);

        let translated = chat_to_responses_request(&request).expect("translation");

        assert_eq!(translated.instructions, None);
        assert_eq!(
            translated.input[1],
            ResponseInputItem::Message {
                role: Role::System,
                content: Value::String("be brief".to_string()),
            }
        );
    }

    #[test]
    fn rejects_conversation_without_user_or_assistant_messages() {
        let request = chat_request(vec![Message {
            role: Role::System,
            content: Content::Text("be terse".to_string()),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: None,
        }]);

        let error = chat_to_responses_request(&request).expect_err("empty input must fail");

        let ProviderError::Public { status, error } = error else {
            panic!("expected public bad request");
        };
        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert!(
            error
                .message
                .contains("at least one user or assistant message")
        );
    }

    fn user_parts_message(parts: Vec<ContentPart>) -> Message {
        Message {
            role: Role::User,
            content: Content::Parts(parts),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: None,
        }
    }

    fn serialized_input(translated: &ResponsesRequest) -> Value {
        serde_json::to_value(translated).expect("serialize")["input"].clone()
    }

    #[test]
    fn maps_user_image_parts_to_input_image() {
        let request = chat_request(vec![user_parts_message(vec![
            ContentPart::Text {
                text: "look".to_string(),
            },
            ContentPart::Image {
                image_url: crate::openai_types::ImageUrl {
                    url: "https://example.com/a.png".to_string(),
                    detail: Some("high".to_string()),
                },
            },
        ])]);

        let translated = chat_to_responses_request(&request).expect("translation");
        let input = serialized_input(&translated);

        assert_eq!(input[0]["content"][0]["type"], "input_text");
        assert_eq!(input[0]["content"][0]["text"], "look");
        assert_eq!(input[0]["content"][1]["type"], "input_image");
        assert_eq!(
            input[0]["content"][1]["image_url"],
            "https://example.com/a.png"
        );
        assert_eq!(input[0]["content"][1]["detail"], "high");
    }

    #[test]
    fn maps_user_audio_parts_to_input_audio() {
        let request = chat_request(vec![user_parts_message(vec![ContentPart::InputAudio {
            input_audio: crate::openai_types::InputAudio {
                data: "QUJD".to_string(),
                format: "wav".to_string(),
            },
        }])]);

        let translated = chat_to_responses_request(&request).expect("translation");
        let input = serialized_input(&translated);

        assert_eq!(input[0]["content"][0]["type"], "input_audio");
        assert_eq!(input[0]["content"][0]["input_audio"]["data"], "QUJD");
        assert_eq!(input[0]["content"][0]["input_audio"]["format"], "wav");
    }

    fn file_part(file: crate::openai_types::FileContent) -> ContentPart {
        ContentPart::File {
            file,
            file_id: None,
            file_data: None,
            filename: None,
            file_url: None,
            mime_type: None,
        }
    }

    #[test]
    fn maps_file_id_to_flat_input_file() {
        let request = chat_request(vec![user_parts_message(vec![file_part(
            crate::openai_types::FileContent {
                file_id: Some("file-abc".to_string()),
                ..Default::default()
            },
        )])]);

        let translated = chat_to_responses_request(&request).expect("translation");
        let input = serialized_input(&translated);

        assert_eq!(input[0]["content"][0]["type"], "input_file");
        assert_eq!(input[0]["content"][0]["file_id"], "file-abc");
        assert_eq!(input[0]["content"][0].get("filename"), None);
        assert_eq!(input[0]["content"][0].get("file_data"), None);
    }

    #[test]
    fn maps_filename_and_file_data_to_flat_input_file() {
        let request = chat_request(vec![user_parts_message(vec![file_part(
            crate::openai_types::FileContent {
                file_data: Some("data:application/pdf;base64,QUJD".to_string()),
                filename: Some("doc.pdf".to_string()),
                mime_type: Some("application/pdf".to_string()),
                ..Default::default()
            },
        )])]);

        let translated = chat_to_responses_request(&request).expect("translation");
        let input = serialized_input(&translated);

        assert_eq!(input[0]["content"][0]["type"], "input_file");
        assert_eq!(input[0]["content"][0]["filename"], "doc.pdf");
        assert_eq!(
            input[0]["content"][0]["file_data"],
            "data:application/pdf;base64,QUJD"
        );
        // The Responses API has no mime type field.
        assert_eq!(input[0]["content"][0].get("mime_type"), None);
    }

    #[test]
    fn maps_file_url_to_flat_input_file() {
        let request = chat_request(vec![user_parts_message(vec![file_part(
            crate::openai_types::FileContent {
                file_url: Some("https://example.com/doc.pdf".to_string()),
                ..Default::default()
            },
        )])]);

        let translated = chat_to_responses_request(&request).expect("translation");
        let input = serialized_input(&translated);

        assert_eq!(input[0]["content"][0]["type"], "input_file");
        assert_eq!(
            input[0]["content"][0]["file_url"],
            "https://example.com/doc.pdf"
        );
    }

    #[test]
    fn maps_flat_file_fields_to_input_file() {
        let request = chat_request(vec![user_parts_message(vec![ContentPart::File {
            file: Default::default(),
            file_id: Some("file-abc".to_string()),
            file_data: None,
            filename: None,
            file_url: None,
            mime_type: None,
        }])]);

        let translated = chat_to_responses_request(&request).expect("translation");
        let input = serialized_input(&translated);

        assert_eq!(input[0]["content"][0]["type"], "input_file");
        assert_eq!(input[0]["content"][0]["file_id"], "file-abc");
    }

    #[test]
    fn rejects_file_part_without_source() {
        let request = chat_request(vec![user_parts_message(vec![
            file_part(Default::default()),
        ])]);

        let error = chat_to_responses_request(&request).expect_err("file must fail");

        let ProviderError::Public { status, error } = error else {
            panic!("expected public bad request");
        };
        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert!(error.message.contains("exactly one of file_id"));
    }

    #[test]
    fn rejects_file_data_without_filename() {
        let request = chat_request(vec![user_parts_message(vec![file_part(
            crate::openai_types::FileContent {
                file_data: Some("QUJD".to_string()),
                ..Default::default()
            },
        )])]);

        let error = chat_to_responses_request(&request).expect_err("file must fail");

        let ProviderError::Public { error, .. } = error else {
            panic!("expected public bad request");
        };
        assert!(error.message.contains("requires a filename"));
    }

    #[test]
    fn rejects_file_part_with_conflicting_sources() {
        let request = chat_request(vec![user_parts_message(vec![file_part(
            crate::openai_types::FileContent {
                file_id: Some("file-abc".to_string()),
                filename: Some("doc.pdf".to_string()),
                ..Default::default()
            },
        )])]);

        let error = chat_to_responses_request(&request).expect_err("file must fail");

        let ProviderError::Public { error, .. } = error else {
            panic!("expected public bad request");
        };
        assert!(error.message.contains("exactly one of file_id"));
    }

    #[test]
    fn rejects_audio_parts_in_non_user_messages() {
        let request = chat_request(vec![Message {
            role: Role::Assistant,
            content: Content::Parts(vec![ContentPart::InputAudio {
                input_audio: crate::openai_types::InputAudio {
                    data: "AAAA".to_string(),
                    format: "wav".to_string(),
                },
            }]),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: None,
        }]);

        let error = chat_to_responses_request(&request).expect_err("audio must fail");

        let ProviderError::Public { error, .. } = error else {
            panic!("expected public bad request");
        };
        assert!(error.message.contains("assistant messages do not support"));
    }

    #[test]
    fn maps_tool_output_with_image_to_content_parts() {
        let request = chat_request(vec![
            user_message("take a screenshot"),
            Message {
                role: Role::Tool,
                content: Content::Parts(vec![
                    ContentPart::Text {
                        text: "Screenshot captured".to_string(),
                    },
                    ContentPart::Image {
                        image_url: crate::openai_types::ImageUrl {
                            url: "data:image/png;base64,QUJD".to_string(),
                            detail: Some("low".to_string()),
                        },
                    },
                ]),
                reasoning_content: None,
                tool_call_id: Some("call_1".to_string()),
                tool_calls: None,
            },
        ]);

        let translated = chat_to_responses_request(&request).expect("translation");
        let input = serialized_input(&translated);

        assert_eq!(input[1]["type"], "function_call_output");
        assert_eq!(input[1]["call_id"], "call_1");
        assert_eq!(input[1]["output"][0]["type"], "input_text");
        assert_eq!(input[1]["output"][0]["text"], "Screenshot captured");
        assert_eq!(input[1]["output"][1]["type"], "input_image");
        assert_eq!(
            input[1]["output"][1]["image_url"],
            "data:image/png;base64,QUJD"
        );
        assert_eq!(input[1]["output"][1]["detail"], "low");
    }

    #[test]
    fn keeps_plain_text_tool_output_as_string() {
        let request = chat_request(vec![
            user_message("weather?"),
            Message {
                role: Role::Tool,
                content: Content::Text("{\"temp\":20}".to_string()),
                reasoning_content: None,
                tool_call_id: Some("call_1".to_string()),
                tool_calls: None,
            },
        ]);

        let translated = chat_to_responses_request(&request).expect("translation");
        let input = serialized_input(&translated);

        assert_eq!(input[1]["type"], "function_call_output");
        assert_eq!(input[1]["output"], "{\"temp\":20}");
    }

    #[test]
    fn rejects_non_text_parts_in_assistant_messages() {
        let request = chat_request(vec![
            user_message("hi"),
            Message {
                role: Role::Assistant,
                content: Content::Parts(vec![ContentPart::Image {
                    image_url: crate::openai_types::ImageUrl {
                        url: "https://example.com/a.png".to_string(),
                        detail: None,
                    },
                }]),
                reasoning_content: None,
                tool_call_id: None,
                tool_calls: None,
            },
        ]);

        let error = chat_to_responses_request(&request).expect_err("image must fail");

        let ProviderError::Public { status, error } = error else {
            panic!("expected public bad request");
        };
        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert!(error.message.contains("assistant messages do not support"));
        assert!(
            error
                .message
                .contains("move the media part to a user message")
        );
    }

    #[test]
    fn skips_empty_system_messages() {
        let request = chat_request(vec![
            Message {
                role: Role::System,
                content: Content::Text(String::new()),
                reasoning_content: None,
                tool_call_id: None,
                tool_calls: None,
            },
            user_message("hi"),
        ]);

        let translated = chat_to_responses_request(&request).expect("translation");

        assert_eq!(translated.instructions, None);
        assert_eq!(translated.input.len(), 1);
    }

    #[test]
    fn drops_sampling_params_for_reasoning_models() {
        let mut request = chat_request(vec![user_message("hi")]);
        request.temperature = Some(0.5);
        request.top_p = Some(0.9);

        let translated = chat_to_responses_request(&request).expect("translation");

        assert_eq!(translated.temperature, None);
        assert_eq!(translated.top_p, None);
    }

    #[test]
    fn keeps_sampling_params_for_models_that_support_them() {
        let mut request = chat_request(vec![user_message("hi")]);
        request.model = "gpt-4o".to_string();
        request.temperature = Some(0.5);
        request.top_p = Some(0.9);

        let translated = chat_to_responses_request(&request).expect("translation");

        assert_eq!(translated.temperature, Some(0.5));
        assert_eq!(translated.top_p, Some(0.9));
    }

    #[test]
    fn translates_tool_loop_to_function_items() {
        use crate::openai_types::{ToolCall, ToolCallFunction};

        let request = chat_request(vec![
            user_message("weather?"),
            Message {
                role: Role::Assistant,
                content: Content::Text(String::new()),
                reasoning_content: None,
                tool_call_id: None,
                tool_calls: Some(vec![ToolCall {
                    id: Some("call_1".to_string()),
                    r#type: Some("function".to_string()),
                    function: ToolCallFunction {
                        name: "get_weather".to_string(),
                        arguments: "{\"city\":\"sf\"}".to_string(),
                        description: None,
                    },
                }]),
            },
            Message {
                role: Role::Tool,
                content: Content::Text("{\"temp\":20}".to_string()),
                reasoning_content: None,
                tool_call_id: Some("call_1".to_string()),
                tool_calls: None,
            },
        ]);

        let translated = chat_to_responses_request(&request).expect("translation");

        assert_eq!(translated.input.len(), 3);
        assert_eq!(
            translated.input[1],
            ResponseInputItem::FunctionCall {
                call_id: "call_1".to_string(),
                name: "get_weather".to_string(),
                arguments: "{\"city\":\"sf\"}".to_string(),
            }
        );
        assert_eq!(
            translated.input[2],
            ResponseInputItem::FunctionCallOutput {
                call_id: "call_1".to_string(),
                output: Value::String("{\"temp\":20}".to_string()),
            }
        );
    }

    #[test]
    fn maps_response_with_tool_calls_and_usage() {
        let response: ResponsesResponse = serde_json::from_value(serde_json::json!({
            "id": "resp_1",
            "created_at": 1_741_476_777.0,
            "status": "completed",
            "model": "gpt-5.6-sol",
            "output": [
                { "type": "reasoning", "summary": [{"type": "summary_text", "text": "thinking..."}] },
                { "type": "function_call", "call_id": "call_1", "name": "get_weather", "arguments": "{}" },
                { "type": "message", "content": [{"type": "output_text", "text": "done"}] }
            ],
            "usage": { "input_tokens": 10, "output_tokens": 5, "total_tokens": 15 }
        }))
        .expect("response fixture");

        let unified = map_responses_to_unified(response).expect("mapping");

        assert_eq!(unified.request_id, "resp_1");
        assert_eq!(unified.output_text, "done");
        assert_eq!(unified.reasoning_content.as_deref(), Some("thinking..."));
        assert_eq!(unified.finish_reason, FinishReason::ToolCalls);
        assert_eq!(unified.tool_calls.as_ref().unwrap()[0].name, "get_weather");
        let usage = &unified.usage;
        let EndpointUsage::Responses(responses_usage) = usage else {
            panic!("expected responses usage");
        };
        assert_eq!(responses_usage.input_tokens, Some(10));
        assert_eq!(responses_usage.output_tokens, Some(5));
    }

    #[test]
    fn maps_incomplete_response_to_length_finish() {
        let response: ResponsesResponse = serde_json::from_value(serde_json::json!({
            "id": "resp_2",
            "status": "incomplete",
            "model": "gpt-5.6-sol",
            "incomplete_details": { "reason": "max_output_tokens" },
            "output": []
        }))
        .expect("response fixture");

        let unified = map_responses_to_unified(response).expect("mapping");

        assert_eq!(unified.finish_reason, FinishReason::Length);
    }

    #[test]
    fn maps_stream_event_sequence() {
        let mut state = ResponsesStreamState::default();

        let events = map_stream_event(
            r#"{"type":"response.created","response":{"id":"resp_1","model":"gpt-5.6-sol","created_at":1741476777.0}}"#,
            &mut state,
        )
        .expect("mapping");
        assert!(matches!(&events[0], UnifiedEvent::ResponseCreated { id, .. } if id == "resp_1"));

        let events = map_stream_event(
            r#"{"type":"response.output_text.delta","item_id":"msg_1","delta":"hi"}"#,
            &mut state,
        )
        .expect("mapping");
        assert!(matches!(&events[0], UnifiedEvent::MessageDelta { delta, .. } if delta == "hi"));

        let events = map_stream_event(
            r#"{"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"get_weather"}}"#,
            &mut state,
        )
        .expect("mapping");
        assert!(
            matches!(&events[0], UnifiedEvent::ToolCallDelta { id, name, .. } if id == "call_1" && name == "get_weather")
        );

        // The Responses API references the item id in argument events, the
        // client-visible id stays the call id.
        let events = map_stream_event(
            r#"{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"city\""}"#,
            &mut state,
        )
        .expect("mapping");
        assert!(
            matches!(&events[0], UnifiedEvent::ToolCallDelta { id, arguments_delta, .. } if id == "call_1" && arguments_delta == "{\"city\"")
        );

        let events = map_stream_event(
            r#"{"type":"response.function_call_arguments.done","item_id":"fc_1","arguments":"{}"}"#,
            &mut state,
        )
        .expect("mapping");
        assert!(
            matches!(&events[0], UnifiedEvent::ToolCallDone { id, name, arguments, .. } if id == "call_1" && name == "get_weather" && arguments == "{}")
        );

        let events = map_stream_event(
            r#"{"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.6-sol","status":"completed","output":[{"type":"function_call","call_id":"call_1"}],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}"#,
            &mut state,
        )
        .expect("mapping");
        assert!(matches!(&events[0], UnifiedEvent::Usage { .. }));
        assert!(
            matches!(&events[1], UnifiedEvent::Completed { finish_reason: Some(reason), .. } if reason == "tool_calls")
        );
    }

    #[test]
    fn stream_content_events_keep_response_id_instead_of_item_id() {
        let mut state = ResponsesStreamState::default();
        map_stream_event(
            r#"{"type":"response.created","response":{"id":"resp_1","model":"gpt-5.6-sol"}}"#,
            &mut state,
        )
        .expect("response created");

        for data in [
            r#"{"type":"response.output_text.delta","item_id":"msg_1","delta":"hi"}"#,
            r#"{"type":"response.reasoning_summary_text.delta","item_id":"rs_1","delta":"hmm"}"#,
            r#"{"type":"response.reasoning_text.delta","item_id":"rs_1","delta":"hmm"}"#,
            r#"{"type":"response.output_item.done","item":{"id":"msg_1","type":"message"}}"#,
            r#"{"type":"response.output_item.done","item":{"id":"rs_1","type":"reasoning"}}"#,
        ] {
            let events = map_stream_event(data, &mut state).expect("content event");
            assert_eq!(events.len(), 1);
            match &events[0] {
                UnifiedEvent::MessageDelta { id, .. }
                | UnifiedEvent::ThinkingDelta { id, .. }
                | UnifiedEvent::MessageStop { id, .. }
                | UnifiedEvent::ThinkingDone { id, .. } => assert_eq!(id, "resp_1"),
                event => panic!("unexpected event: {event:?}"),
            }
        }
        assert_eq!(state.request_id, "resp_1");
    }

    #[test]
    fn failed_response_returns_error_after_tool_call() {
        let mut state = ResponsesStreamState::default();
        let events = map_stream_event(
            r#"{"type":"response.output_item.done","item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{}"}}"#,
            &mut state,
        )
        .expect("tool call");
        assert!(matches!(&events[0], UnifiedEvent::ToolCallDone { .. }));

        let error = map_stream_event(
            r#"{"type":"response.failed","response":{"id":"resp_1","error":{"code":"server_error","message":"upstream unavailable"}}}"#,
            &mut state,
        )
        .expect_err("failed response must abort the stream");

        assert!(matches!(&error, ProviderError::Internal { .. }));
        assert!(error.to_string().contains("server_error"));
        assert!(error.to_string().contains("upstream unavailable"));
    }

    #[test]
    fn stream_failure_without_details_still_returns_error() {
        for data in [
            r#"{"type":"response.failed"}"#,
            r#"{"type":"response.failed","response":{}}"#,
            r#"{"type":"response.failed","response":{"error":{}}}"#,
            r#"{"type":"error"}"#,
            r#"{"type":"error","code":" ","message":" "}"#,
        ] {
            let error = map_stream_event(data, &mut ResponsesStreamState::default())
                .expect_err("failure without details must not be ignored");
            assert!(matches!(&error, ProviderError::Internal { .. }));
            assert!(error.to_string().contains("response failed"));
        }
    }

    #[test]
    fn standalone_stream_error_preserves_code_and_message() {
        let error = map_stream_event(
            r#"{"type":"error","code":"rate_limit_exceeded","message":"please retry later"}"#,
            &mut ResponsesStreamState::default(),
        )
        .expect_err("standalone error must abort the stream");

        assert!(matches!(&error, ProviderError::Internal { .. }));
        assert!(error.to_string().contains("rate_limit_exceeded"));
        assert!(error.to_string().contains("please retry later"));
    }

    #[test]
    fn reports_tool_call_once_when_item_done_repeats_the_call() {
        let mut state = ResponsesStreamState::default();

        map_stream_event(
            r#"{"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"get_weather"}}"#,
            &mut state,
        )
        .expect("mapping");

        let events = map_stream_event(
            r#"{"type":"response.function_call_arguments.done","item_id":"fc_1","arguments":"{}"}"#,
            &mut state,
        )
        .expect("mapping");
        assert_eq!(events.len(), 1);

        // `response.output_item.done` repeats the call and must not emit a
        // second completion event for the same call.
        let events = map_stream_event(
            r#"{"type":"response.output_item.done","item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{}"}}"#,
            &mut state,
        )
        .expect("mapping");
        assert!(events.is_empty());
    }

    #[test]
    fn reports_usage_when_upstream_omits_it() {
        let mut state = ResponsesStreamState::default();

        let events = map_stream_event(
            r#"{"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.6-sol","status":"completed","output":[]}}"#,
            &mut state,
        )
        .expect("mapping");

        let UnifiedEvent::Usage { usage } = &events[0] else {
            panic!("expected usage event, got {:?}", events[0]);
        };
        let EndpointUsage::Responses(usage) = usage else {
            panic!("expected responses usage");
        };
        assert_eq!(usage.input_tokens, Some(0));
        assert_eq!(usage.output_tokens, Some(0));
        assert_eq!(usage.total_tokens, Some(0));
    }

    #[test]
    fn maps_reasoning_summary_deltas() {
        let mut state = ResponsesStreamState::default();
        let events = map_stream_event(
            r#"{"type":"response.reasoning_summary_text.delta","item_id":"rs_1","delta":"hmm"}"#,
            &mut state,
        )
        .expect("mapping");
        assert!(matches!(&events[0], UnifiedEvent::ThinkingDelta { delta, .. } if delta == "hmm"));
    }

    #[test]
    fn responses_request_serializes_with_type_tags() {
        let request = chat_request(vec![user_message("hi")]);
        let translated = chat_to_responses_request(&request).expect("translation");
        let value = serde_json::to_value(&translated).expect("serialize");

        assert_eq!(value["model"], "gpt-5.6-sol");
        assert_eq!(value["input"][0]["type"], "message");
        assert_eq!(value["input"][0]["role"], "user");
        assert_eq!(value["input"][0]["content"], "hi");
    }

    #[test]
    fn responses_request_serializes_max_output_tokens_as_integer() {
        let mut request = chat_request(vec![user_message("hi")]);
        request.max_completion_tokens = Some(8192);

        let translated = chat_to_responses_request(&request).expect("translation");
        let value = serde_json::to_value(&translated).expect("serialize");

        assert!(value["max_output_tokens"].is_i64());
        assert_eq!(value["max_output_tokens"], 8192);
    }

    #[test]
    fn responses_request_serializes_tools_without_null_fields() {
        use crate::openai_types::{FunctionDefinition, ToolDefinition};

        let mut request = chat_request(vec![user_message("hi")]);
        request.tools = Some(vec![ToolDefinition {
            r#type: "function".to_string(),
            function: FunctionDefinition {
                name: "get_weather".to_string(),
                description: None,
                strict: None,
                parameters: serde_json::json!({"type": "object"}),
            },
        }]);

        let translated = chat_to_responses_request(&request).expect("translation");
        let value = serde_json::to_value(&translated).expect("serialize");

        assert_eq!(value["tools"][0]["type"], "function");
        assert_eq!(value["tools"][0]["name"], "get_weather");
        assert_eq!(value["tools"][0].get("description"), None);
        assert_eq!(value["tools"][0].get("strict"), None);
        assert_eq!(value["tools"][0]["parameters"]["type"], "object");
    }
}
