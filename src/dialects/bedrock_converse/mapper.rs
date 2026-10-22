//! AWS Bedrock Converse request, response, and stream-event mapping.

use std::collections::HashMap;

use aws_sdk_bedrockruntime::primitives::Blob;
use aws_sdk_bedrockruntime::types::{
    AnyToolChoice, AutoToolChoice, ContentBlock, ContentBlockDelta, ConversationRole,
    InferenceConfiguration, Message as ConverseMessage, ReasoningContentBlock, ReasoningTextBlock,
    SpecificToolChoice, StopReason, SystemContentBlock, Tool as ConverseTool,
    ToolChoice as ConverseToolChoice, ToolConfiguration, ToolInputSchema, ToolResultBlock,
    ToolResultContentBlock, ToolResultStatus, ToolSpecification, ToolUseBlock,
};
use aws_smithy_types::Document;
use base64::Engine;
use serde_json::{Map, Value, json};

use crate::openai_types::{
    ChatCompletionRequest, Content, ContentPart, ReasoningBlock, Role, ToolChoice as IrToolChoice,
};
use crate::provider::{FinishReason, ProviderError, ToolCall, UnifiedEvent, UnifiedResponse};
use crate::provider::{
    UnifiedContentDelta, UnifiedEnding, UnifiedEventKind, UnifiedEventTarget, UnifiedItemDelta,
    UnifiedOutputItem, UnifiedOutputItemKind, UnifiedPartLocation, UnifiedUsageUpdate,
};
use crate::provider::{
    UnifiedContentPart, UnifiedContentPartKind, UnifiedInputItemKind, UnifiedMediaSource,
    UnifiedRequest, UnifiedRole, UnifiedToolChoice, UnifiedToolKind, UnifiedToolOutput,
};
use crate::usage_handler::EndpointUsage;

// ---------------------------------------------------------------------------
// IR -> Converse request
// ---------------------------------------------------------------------------

pub type ConverseRequestParts = (
    Vec<ConverseMessage>,
    Option<Vec<SystemContentBlock>>,
    Option<ToolConfiguration>,
    InferenceConfiguration,
);

const CLAUDE_HAIKU_55_BEDROCK_MODEL: &str = "claude-haiku-5-5";
const CLAUDE_SONNET_55_BEDROCK_MODEL: &str = "claude-sonnet-5-5";
const CLAUDE_SONNET_46_BEDROCK_MODEL: &str = "claude-sonnet-4-6";
const CLAUDE_SONNET_5_BEDROCK_MODEL: &str = "claude-sonnet-5";
const CLAUDE_HAIKU_45_BEDROCK_MODEL: &str = "claude-haiku-4-5";
const CLAUDE_OPUS_46_BEDROCK_MODEL: &str = "claude-opus-4-6";
const CLAUDE_OPUS_47_BEDROCK_MODEL: &str = "claude-opus-4-7";
const CLAUDE_OPUS_48_BEDROCK_MODEL: &str = "claude-opus-4-8";
const CLAUDE_OPUS_55_BEDROCK_MODEL: &str = "claude-opus-5-5";
const CLAUDE_OPUS_5_BEDROCK_MODEL: &str = "claude-opus-5";
const CLAUDE_FABLE_51_BEDROCK_MODEL: &str = "claude-fable-5-1";
const CLAUDE_FABLE_5_BEDROCK_MODEL: &str = "claude-fable-5";

/// Thinking modes accepted by the adaptive-only and adaptive/disabled routes.
const THINKING_ADAPTIVE: &[&str] = &["adaptive"];
const THINKING_ADAPTIVE_DISABLED: &[&str] = &["adaptive", "disabled"];

/// Effort enums accepted by the Bedrock Claude routes.
const EFFORT_LOW_TO_MAX: &[&str] = &["low", "medium", "high", "xhigh", "max"];
const EFFORT_LOW_TO_MAX_NO_XHIGH: &[&str] = &["low", "medium", "high", "max"];

/// Reasoning controls accepted by one Bedrock Claude route.
struct ReasoningControls {
    thinking_modes: &'static [&'static str],
    efforts: &'static [&'static str],
}

/// Controls keyed by the Converse model id.
fn reasoning_controls(model: &str) -> Option<(&'static str, ReasoningControls)> {
    // Longer ids first so `claude-opus-5-5` never matches `claude-opus-5`.
    if model.contains(CLAUDE_OPUS_55_BEDROCK_MODEL) {
        return Some((
            "Opus 5.5",
            ReasoningControls {
                thinking_modes: THINKING_ADAPTIVE,
                efforts: EFFORT_LOW_TO_MAX,
            },
        ));
    }
    if model.contains(CLAUDE_FABLE_51_BEDROCK_MODEL) {
        return Some((
            "Fable 5.1",
            ReasoningControls {
                thinking_modes: THINKING_ADAPTIVE,
                efforts: EFFORT_LOW_TO_MAX,
            },
        ));
    }
    if model.contains(CLAUDE_FABLE_5_BEDROCK_MODEL) {
        return Some((
            "Fable 5",
            ReasoningControls {
                thinking_modes: THINKING_ADAPTIVE,
                efforts: EFFORT_LOW_TO_MAX,
            },
        ));
    }
    if model.contains(CLAUDE_OPUS_47_BEDROCK_MODEL) {
        return Some((
            "Opus 4.7",
            ReasoningControls {
                thinking_modes: THINKING_ADAPTIVE_DISABLED,
                efforts: EFFORT_LOW_TO_MAX,
            },
        ));
    }
    if model.contains(CLAUDE_OPUS_48_BEDROCK_MODEL) {
        return Some((
            "Opus 4.8",
            ReasoningControls {
                thinking_modes: THINKING_ADAPTIVE_DISABLED,
                efforts: EFFORT_LOW_TO_MAX,
            },
        ));
    }
    if model.contains(CLAUDE_OPUS_5_BEDROCK_MODEL) {
        return Some((
            "Opus 5",
            ReasoningControls {
                thinking_modes: THINKING_ADAPTIVE_DISABLED,
                efforts: EFFORT_LOW_TO_MAX,
            },
        ));
    }
    if model.contains(CLAUDE_SONNET_5_BEDROCK_MODEL) {
        return Some((
            "Sonnet 5",
            ReasoningControls {
                thinking_modes: THINKING_ADAPTIVE_DISABLED,
                efforts: EFFORT_LOW_TO_MAX,
            },
        ));
    }
    if model.contains(CLAUDE_SONNET_46_BEDROCK_MODEL) {
        return Some((
            "Sonnet 4.6",
            ReasoningControls {
                thinking_modes: &["enabled", "adaptive", "disabled"],
                efforts: EFFORT_LOW_TO_MAX_NO_XHIGH,
            },
        ));
    }
    if model.contains(CLAUDE_OPUS_46_BEDROCK_MODEL) {
        return Some((
            "Opus 4.6",
            ReasoningControls {
                thinking_modes: &["enabled", "adaptive", "disabled"],
                efforts: EFFORT_LOW_TO_MAX_NO_XHIGH,
            },
        ));
    }
    if model.contains(CLAUDE_HAIKU_55_BEDROCK_MODEL) {
        return Some((
            "Haiku 5.5",
            ReasoningControls {
                thinking_modes: THINKING_ADAPTIVE_DISABLED,
                efforts: EFFORT_LOW_TO_MAX,
            },
        ));
    }
    if model.contains(CLAUDE_HAIKU_45_BEDROCK_MODEL) {
        return Some((
            "Haiku 4.5",
            ReasoningControls {
                thinking_modes: &["enabled", "disabled"],
                efforts: &[],
            },
        ));
    }
    None
}

/// Encode the Claude controls supported by Bedrock Converse.
///
/// Each configured model has its own rule set. Any other Claude model id passes
/// its reasoning controls through unchanged and lets the upstream decide what
/// it supports.
pub(super) fn additional_model_request_fields(
    request: &UnifiedRequest,
    bedrock_model: &str,
) -> Result<Option<Document>, String> {
    let model = bedrock_model.to_ascii_lowercase();
    if model.contains(CLAUDE_SONNET_55_BEDROCK_MODEL) {
        return sonnet_5_5_fields(request);
    }
    if let Some((label, controls)) = reasoning_controls(&model) {
        return encode_claude_fields(request, &controls, label);
    }
    passthrough_claude_fields(request)
}

/// Claude Sonnet 5.5: adaptive-only; forward adaptive/between_tools thinking
/// with an optional Bedrock display, plus output_config.effort.
fn sonnet_5_5_fields(request: &UnifiedRequest) -> Result<Option<Document>, String> {
    let mut fields = Map::new();
    if let Some(effort) = request.options.reasoning_effort.as_deref() {
        if !matches!(effort, "low" | "medium" | "high" | "xhigh" | "max") {
            return Err(format!(
                "Claude Sonnet 5.5 does not map reasoning_effort {effort}"
            ));
        }
        fields.insert("output_config".into(), json!({"effort": effort}));
    }
    if let Some(thinking) = &request.options.thinking {
        if thinking.get("block_binding").is_some() {
            return Err("Claude Sonnet 5.5 rejects thinking.block_binding".into());
        }
        let type_name = claude_thinking_type(thinking, &["adaptive", "between_tools"])?;
        let mut value = json!({"type": type_name});
        if let Some(display) = claude_thinking_display(thinking)? {
            value["display"] = json!(display);
        }
        fields.insert("thinking".into(), value);
    }
    claude_fields_to_document(fields)
}

/// Forward reasoning controls of a Claude route without its own rules, without
/// dropping or rewriting them; the upstream reports what it rejects.
fn passthrough_claude_fields(request: &UnifiedRequest) -> Result<Option<Document>, String> {
    let mut fields = Map::new();
    if let Some(effort) = request.options.reasoning_effort.as_deref() {
        fields.insert("output_config".into(), json!({"effort": effort}));
    }
    if let Some(thinking) = &request.options.thinking {
        fields.insert("thinking".into(), thinking.clone());
    }
    claude_fields_to_document(fields)
}

/// Encode the controls accepted by one Bedrock Claude route. Budget bounds are
/// validated here before constructing the native request.
fn encode_claude_fields(
    request: &UnifiedRequest,
    controls: &ReasoningControls,
    label: &str,
) -> Result<Option<Document>, String> {
    let mut fields = Map::new();
    if let Some(effort) = request.options.reasoning_effort.as_deref() {
        if !controls.efforts.contains(&effort) {
            return Err(format!(
                "Claude {label} does not map reasoning_effort {effort}"
            ));
        }
        fields.insert("output_config".into(), json!({"effort": effort}));
    }
    if let Some(thinking) = &request.options.thinking {
        if thinking.get("block_binding").is_some() {
            return Err(format!("Claude {label} rejects thinking.block_binding"));
        }
        let type_name = claude_thinking_type(thinking, controls.thinking_modes)?;
        let mut value = json!({"type": type_name});
        if type_name == "enabled" {
            let budget = thinking
                .get("budget_tokens")
                .and_then(Value::as_u64)
                .ok_or_else(|| {
                    format!("Claude {label} requires thinking.budget_tokens for enabled")
                })?;
            if budget < 1024 {
                return Err(format!("Claude {label} requires budget_tokens >= 1024"));
            }
            if let Some(max) = request
                .options
                .max_output_tokens
                .filter(|max| budget >= *max as u64)
            {
                return Err(format!(
                    "Claude {label} requires budget_tokens below max_output_tokens ({max})"
                ));
            }
            value["budget_tokens"] = json!(budget);
        } else if thinking.get("budget_tokens").is_some() {
            return Err(format!(
                "Claude {label} accepts no thinking budget for {type_name} thinking"
            ));
        }
        if let Some(display) = claude_thinking_display(thinking)? {
            value["display"] = json!(display);
        }
        fields.insert("thinking".into(), value);
    }
    claude_fields_to_document(fields)
}

/// Read and defensively validate `thinking.type` against the model's supported
/// modes.
fn claude_thinking_type<'a>(thinking: &'a Value, supported: &[&str]) -> Result<&'a str, String> {
    let type_name = thinking
        .get("type")
        .and_then(Value::as_str)
        .ok_or("Claude thinking requires a type")?;
    if !supported.contains(&type_name) {
        return Err(format!(
            "Claude thinking type {type_name} is not mapped for this model"
        ));
    }
    Ok(type_name)
}

/// Read and defensively validate `thinking.display` (Bedrock only accepts
/// summarized/omitted; updates is a client-only value).
fn claude_thinking_display(thinking: &Value) -> Result<Option<&str>, String> {
    match thinking.get("display").and_then(Value::as_str) {
        None => Ok(None),
        Some(display @ ("summarized" | "omitted")) => Ok(Some(display)),
        Some(other) => Err(format!(
            "Claude thinking display {other} is not accepted; only summarized/omitted"
        )),
    }
}

fn claude_fields_to_document(fields: Map<String, Value>) -> Result<Option<Document>, String> {
    if fields.is_empty() {
        Ok(None)
    } else {
        Ok(Some(json_value_to_document(&Value::Object(fields))))
    }
}

pub(super) fn validate_model_conversation(
    bedrock_model: &str,
    messages: &[ConverseMessage],
) -> Result<(), String> {
    if bedrock_model
        .to_ascii_lowercase()
        .contains(CLAUDE_HAIKU_55_BEDROCK_MODEL)
        && messages
            .last()
            .is_some_and(|message| *message.role() == ConversationRole::Assistant)
    {
        return Err(
            "Claude Haiku 5.5 does not support assistant message prefill; conversation must end with a user message".into(),
        );
    }
    Ok(())
}

fn is_empty_reasoning_placeholder(
    summary: &[Value],
    content: Option<&[UnifiedContentPart]>,
    encrypted_content: Option<&str>,
) -> bool {
    encrypted_content.is_none()
        && content.is_none_or(|parts| {
            parts.iter().all(|part| match &part.kind {
                UnifiedContentPartKind::Text { text, .. }
                | UnifiedContentPartKind::Thinking { text, .. } => text.is_empty(),
                _ => false,
            })
        })
        && summary.iter().all(|part| {
            part.get("text").and_then(Value::as_str) == Some("")
                && part
                    .as_object()
                    .is_some_and(|fields| fields.keys().all(|key| key == "type" || key == "text"))
        })
}

/// Encode ordered unified items directly, preserving thinking signatures and
/// interleaved function calls rather than projecting through a Chat request.
pub fn unified_to_converse_request(
    request: &UnifiedRequest,
) -> Result<ConverseRequestParts, String> {
    let mut messages: Vec<ConverseMessage> = Vec::new();
    let mut system = Vec::new();
    let mut current: Option<(ConversationRole, Option<String>, Vec<ContentBlock>)> = None;
    for item in request.instructions.iter().flatten().chain(&request.input) {
        let boundary = item
            .message
            .as_ref()
            .map(|message| message.message_id.clone());
        if let UnifiedInputItemKind::Reasoning {
            summary,
            content,
            encrypted_content,
        } = &item.kind
        {
            if is_empty_reasoning_placeholder(
                summary,
                content.as_deref(),
                encrypted_content.as_deref(),
            ) {
                continue;
            }
            return Err("provider reasoning history cannot be represented".into());
        }
        let (role, blocks) = match &item.kind {
            UnifiedInputItemKind::Message {
                role: UnifiedRole::System | UnifiedRole::Developer,
                content,
                ..
            } => {
                for part in content {
                    let UnifiedContentPartKind::Text { text, .. } = &part.kind else {
                        return Err("system content requires text".into());
                    };
                    if !text.is_empty() {
                        system.push(SystemContentBlock::Text(text.clone()));
                    }
                }
                continue;
            }
            UnifiedInputItemKind::Message { role, content, .. } => {
                let role = match role {
                    UnifiedRole::Assistant => ConversationRole::Assistant,
                    UnifiedRole::User => ConversationRole::User,
                    _ => {
                        return Err("tool results require function_call_output items".into());
                    }
                };
                (
                    role,
                    content
                        .iter()
                        .filter(|part| {
                            !matches!(
                                &part.kind,
                                UnifiedContentPartKind::Text { text, .. } if text.is_empty()
                            ) && !matches!(
                                &part.kind,
                                UnifiedContentPartKind::Thinking {
                                    text,
                                    signature: None
                                } if text.is_empty()
                            )
                        })
                        .map(unified_content_block)
                        .collect::<Result<Vec<_>, _>>()?,
                )
            }
            UnifiedInputItemKind::FunctionCall {
                call_id,
                name,
                arguments,
                ..
            } => {
                let input: Value = serde_json::from_str(arguments)
                    .map_err(|error| format!("invalid function arguments: {error}"))?;
                let block = ToolUseBlock::builder()
                    .tool_use_id(call_id)
                    .name(name)
                    .input(json_value_to_document(&input))
                    .build()
                    .map_err(|error| error.to_string())?;
                (
                    ConversationRole::Assistant,
                    vec![ContentBlock::ToolUse(block)],
                )
            }
            UnifiedInputItemKind::FunctionCallOutput {
                call_id,
                output,
                is_error,
                ..
            } => {
                let content = match output {
                    UnifiedToolOutput::Text(text) => {
                        vec![ToolResultContentBlock::Text(text.clone())]
                    }
                    UnifiedToolOutput::Json(value) => {
                        vec![ToolResultContentBlock::Json(json_value_to_document(value))]
                    }
                    UnifiedToolOutput::Content(parts) => parts
                        .iter()
                        .map(|part| match &part.kind {
                            UnifiedContentPartKind::Text { text, .. } => {
                                Ok(ToolResultContentBlock::Text(text.clone()))
                            }
                            _ => Err("tool-result content mapping is not established".to_owned()),
                        })
                        .collect::<Result<Vec<_>, _>>()?,
                    UnifiedToolOutput::Missing => {
                        return Err("tool output is required".into());
                    }
                };
                let block = ToolResultBlock::builder()
                    .tool_use_id(call_id)
                    .set_content(Some(content))
                    .set_status(is_error.map(|error| {
                        if error {
                            ToolResultStatus::Error
                        } else {
                            ToolResultStatus::Success
                        }
                    }))
                    .build()
                    .map_err(|error| error.to_string())?;
                (
                    ConversationRole::User,
                    vec![ContentBlock::ToolResult(block)],
                )
            }
            _ => return Err("input item cannot be represented".into()),
        };
        let same_message = current
            .as_ref()
            .is_some_and(|(previous_role, previous_boundary, _)| {
                *previous_role == role
                    && (role == ConversationRole::Assistant
                        || (boundary.is_some() && *previous_boundary == boundary))
            });
        if !same_message {
            if let Some((role, _, blocks)) = current.take() {
                if !blocks.is_empty() {
                    messages.push(converse_message(role, blocks)?);
                }
            }
            current = Some((role, boundary, Vec::new()));
        }
        if let Some((_, _, content)) = &mut current {
            content.extend(blocks);
        }
    }
    if let Some((role, _, blocks)) = current {
        if !blocks.is_empty() {
            messages.push(converse_message(role, blocks)?);
        }
    }
    if messages.is_empty() {
        return Err("request has no convertible messages".into());
    }
    let mut tools = Vec::new();
    for tool in request.tools.iter().flatten() {
        let UnifiedToolKind::Function {
            name,
            description,
            parameters,
            ..
        } = &tool.kind
        else {
            return Err("cannot encode non-function tools".into());
        };
        let spec = ToolSpecification::builder()
            .name(name)
            .set_description(description.clone())
            .input_schema(ToolInputSchema::Json(json_value_to_document(parameters)))
            .build()
            .map_err(|error| error.to_string())?;
        tools.push(ConverseTool::ToolSpec(spec));
    }
    let tool_config =
        if tools.is_empty() || matches!(request.tool_choice, Some(UnifiedToolChoice::None)) {
            None
        } else {
            let choice = match &request.tool_choice {
                None | Some(UnifiedToolChoice::Auto) => {
                    ConverseToolChoice::Auto(AutoToolChoice::builder().build())
                }
                Some(UnifiedToolChoice::Required) => {
                    ConverseToolChoice::Any(AnyToolChoice::builder().build())
                }
                Some(UnifiedToolChoice::Function(name)) => ConverseToolChoice::Tool(
                    SpecificToolChoice::builder()
                        .name(name)
                        .build()
                        .map_err(|error| error.to_string())?,
                ),
                _ => return Err("tool-choice mapping is not established".into()),
            };
            Some(
                ToolConfiguration::builder()
                    .set_tools(Some(tools))
                    .tool_choice(choice)
                    .build()
                    .map_err(|error| error.to_string())?,
            )
        };
    let max_tokens = request
        .options
        .max_output_tokens
        .map(i32::try_from)
        .transpose()
        .map_err(|error| error.to_string())?;
    let inference = InferenceConfiguration::builder()
        .set_max_tokens(max_tokens)
        .set_temperature(request.options.temperature.map(|value| value as f32))
        .set_top_p(request.options.top_p.map(|value| value as f32))
        .set_stop_sequences(request.options.stop_sequences.clone())
        .build();
    Ok((
        messages,
        (!system.is_empty()).then_some(system),
        tool_config,
        inference,
    ))
}

fn unified_content_block(part: &UnifiedContentPart) -> Result<ContentBlock, String> {
    match &part.kind {
        UnifiedContentPartKind::Text { text, .. } => Ok(ContentBlock::Text(text.clone())),
        UnifiedContentPartKind::Thinking {
            text,
            signature: Some(signature),
        } => {
            let block = ReasoningTextBlock::builder()
                .text(text)
                .signature(signature)
                .build()
                .map_err(|error| error.to_string())?;
            Ok(ContentBlock::ReasoningContent(
                ReasoningContentBlock::ReasoningText(block),
            ))
        }
        UnifiedContentPartKind::Thinking {
            signature: None, ..
        } => Err("reasoning history requires a signature".into()),
        UnifiedContentPartKind::RedactedThinking { data } => {
            let bytes = base64::engine::general_purpose::STANDARD
                .decode(data)
                .map_err(|error| error.to_string())?;
            Ok(ContentBlock::ReasoningContent(
                ReasoningContentBlock::RedactedContent(Blob::new(bytes)),
            ))
        }
        UnifiedContentPartKind::Image {
            source: UnifiedMediaSource::DataUrl { url },
            ..
        }
        | UnifiedContentPartKind::Image {
            source: UnifiedMediaSource::Url { url },
            ..
        } => image_url_to_block(url),
        UnifiedContentPartKind::Image {
            source: UnifiedMediaSource::Base64 { data, media_type },
            ..
        } => image_url_to_block(&format!("data:{media_type};base64,{data}")),
        _ => Err("content part cannot be represented".into()),
    }
}

/// IR request -> Converse structures (messages / system / tools / inference config).
pub fn ir_to_converse_request(
    request: &ChatCompletionRequest,
) -> Result<ConverseRequestParts, String> {
    let mut messages: Vec<ConverseMessage> = Vec::new();
    let mut system_blocks: Vec<SystemContentBlock> = Vec::new();

    for message in &request.messages {
        match message.role {
            Role::System | Role::Developer => {
                let text = content_to_text(&message.content);
                if !text.is_empty() {
                    system_blocks.push(SystemContentBlock::Text(text));
                }
            }
            Role::User => {
                let mut blocks = Vec::new();
                match &message.content {
                    Content::Text(text) => {
                        if !text.is_empty() {
                            blocks.push(ContentBlock::Text(text.clone()));
                        }
                    }
                    Content::Parts(parts) => {
                        for part in parts {
                            blocks.push(content_part_to_block(part)?);
                        }
                    }
                }
                messages.push(converse_message(ConversationRole::User, blocks)?);
            }
            Role::Tool => {
                let tool_use_id = message
                    .tool_call_id
                    .clone()
                    .filter(|id| !id.trim().is_empty())
                    .ok_or_else(|| "tool message requires tool_call_id".to_string())?;
                let text = content_to_text(&message.content);
                let block = ContentBlock::ToolResult(
                    ToolResultBlock::builder()
                        .tool_use_id(tool_use_id)
                        .content(ToolResultContentBlock::Text(text))
                        .build()
                        .map_err(|err| format!("invalid tool result block: {err}"))?,
                );
                messages.push(converse_message(ConversationRole::User, vec![block])?);
            }
            Role::Assistant => {
                let mut blocks = Vec::new();
                if let Some(blocks_ir) = &message.reasoning_blocks {
                    for block in blocks_ir {
                        blocks.push(reasoning_block_to_content_block(block)?);
                    }
                }
                match &message.content {
                    Content::Text(text) => {
                        if !text.is_empty() {
                            blocks.push(ContentBlock::Text(text.clone()));
                        }
                    }
                    Content::Parts(parts) => {
                        for part in parts {
                            if let ContentPart::Text { text } = part {
                                blocks.push(ContentBlock::Text(text.clone()));
                            }
                        }
                    }
                }
                for call in message.tool_calls.iter().flatten() {
                    let input = json_value_to_document(
                        &serde_json::from_str::<Value>(&call.function.arguments)
                            .unwrap_or(Value::Object(Default::default())),
                    );
                    blocks.push(ContentBlock::ToolUse(
                        ToolUseBlock::builder()
                            .tool_use_id(
                                call.id
                                    .clone()
                                    .unwrap_or_else(|| format!("call_{}", call.function.name)),
                            )
                            .name(call.function.name.clone())
                            .input(input)
                            .build()
                            .map_err(|err| format!("invalid tool use block: {err}"))?,
                    ));
                }
                if blocks.is_empty() {
                    blocks.push(ContentBlock::Text(String::new()));
                }
                messages.push(converse_message(ConversationRole::Assistant, blocks)?);
            }
        }
    }

    if messages.is_empty() {
        return Err("request has no convertible messages".to_string());
    }

    let tool_config = request.tools.as_ref().map(|tools| {
        let specs = tools
            .iter()
            .map(|tool| {
                ConverseTool::ToolSpec(
                    ToolSpecification::builder()
                        .name(tool.function.name.clone())
                        .set_description(tool.function.description.clone())
                        .input_schema(ToolInputSchema::Json(json_value_to_document(
                            &tool.function.parameters,
                        )))
                        .build()
                        .expect("tool spec with name and schema must build"),
                )
            })
            .collect::<Vec<_>>();
        let mut builder = ToolConfiguration::builder().set_tools(Some(specs));
        if let Some(choice) = &request.tool_choice {
            builder = builder.tool_choice(ir_tool_choice_to_converse(choice));
        }
        builder.build().expect("tool config with tools must build")
    });

    let inference = InferenceConfiguration::builder()
        .set_max_tokens(request.max_completion_tokens.map(i32::from))
        .set_temperature(request.temperature.map(f32::from))
        .set_top_p(request.top_p.map(f32::from))
        .set_stop_sequences(request.stop.clone())
        .build();

    let system = (!system_blocks.is_empty()).then_some(system_blocks);
    Ok((messages, system, tool_config, inference))
}

fn converse_message(
    role: ConversationRole,
    content: Vec<ContentBlock>,
) -> Result<ConverseMessage, String> {
    ConverseMessage::builder()
        .role(role)
        .set_content(Some(content))
        .build()
        .map_err(|err| format!("invalid converse message: {err}"))
}

fn content_part_to_block(part: &ContentPart) -> Result<ContentBlock, String> {
    match part {
        ContentPart::Text { text } => Ok(ContentBlock::Text(text.clone())),
        ContentPart::Image { image_url } => image_url_to_block(&image_url.url),
        other => Err(format!(
            "bedrock converse does not support content part: {other:?}"
        )),
    }
}

/// image_url -> Converse Image block; supports base64 data URLs.
fn image_url_to_block(url: &str) -> Result<ContentBlock, String> {
    let Some((meta, data)) = url.split_once(",") else {
        return Err("bedrock converse image requires a base64 data URL".to_string());
    };
    let format = meta
        .strip_prefix("data:image/")
        .and_then(|meta| meta.split(';').next())
        .unwrap_or("png")
        .to_string();
    let bytes = base64::engine::general_purpose::STANDARD
        .decode(data.trim())
        .map_err(|err| format!("invalid base64 image: {err}"))?;
    let source = aws_sdk_bedrockruntime::types::ImageSource::Bytes(Blob::new(bytes));
    let block = aws_sdk_bedrockruntime::types::ImageBlock::builder()
        .format(aws_sdk_bedrockruntime::types::ImageFormat::from(
            format.as_str(),
        ))
        .set_source(Some(source))
        .build()
        .map_err(|err| format!("invalid image block: {err}"))?;
    Ok(ContentBlock::Image(block))
}

/// IR reasoning block → Converse ReasoningContent content block。
///
/// Redacted (encrypted) payloads are stored base64-encoded in `encrypted_content`
/// and decoded back into a Blob on the way out, keeping the round trip lossless.
fn reasoning_block_to_content_block(block: &ReasoningBlock) -> Result<ContentBlock, String> {
    if let Some(encrypted) = &block.encrypted_content {
        let bytes = base64::engine::general_purpose::STANDARD
            .decode(encrypted)
            .map_err(|err| format!("invalid redacted thinking payload: {err}"))?;
        return Ok(ContentBlock::ReasoningContent(
            ReasoningContentBlock::RedactedContent(Blob::new(bytes)),
        ));
    }
    let reasoning_text = ReasoningTextBlock::builder()
        .text(block.thinking.clone().unwrap_or_default())
        .set_signature(block.signature.clone())
        .build()
        .map_err(|err| format!("invalid reasoning block: {err}"))?;
    Ok(ContentBlock::ReasoningContent(
        ReasoningContentBlock::ReasoningText(reasoning_text),
    ))
}

fn ir_tool_choice_to_converse(choice: &IrToolChoice) -> ConverseToolChoice {
    match choice {
        IrToolChoice::Name(name) => match name.as_str() {
            "any" | "required" => ConverseToolChoice::Any(
                aws_sdk_bedrockruntime::types::AnyToolChoice::builder().build(),
            ),
            _ => ConverseToolChoice::Auto(
                aws_sdk_bedrockruntime::types::AutoToolChoice::builder().build(),
            ),
        },
        IrToolChoice::Object { function, .. } => ConverseToolChoice::Tool(
            aws_sdk_bedrockruntime::types::SpecificToolChoice::builder()
                .name(function.name.clone())
                .build()
                .expect("specific tool choice with name must build"),
        ),
    }
}

fn content_to_text(content: &Content) -> String {
    match content {
        Content::Text(text) => text.clone(),
        Content::Parts(parts) => parts
            .iter()
            .filter_map(|part| match part {
                ContentPart::Text { text } => Some(text.clone()),
                _ => None,
            })
            .collect::<Vec<_>>()
            .join(""),
    }
}

// ---------------------------------------------------------------------------
// Document ↔ JSON
// ---------------------------------------------------------------------------

fn json_value_to_document(value: &Value) -> Document {
    match value {
        Value::Null => Document::Null,
        Value::Bool(flag) => Document::Bool(*flag),
        Value::Number(number) => {
            if let Some(int) = number.as_i64() {
                if int >= 0 {
                    Document::Number(aws_smithy_types::Number::PosInt(int as u64))
                } else {
                    Document::Number(aws_smithy_types::Number::NegInt(int))
                }
            } else {
                Document::Number(aws_smithy_types::Number::Float(
                    number.as_f64().unwrap_or(0.0),
                ))
            }
        }
        Value::String(text) => Document::String(text.clone()),
        Value::Array(items) => Document::Array(items.iter().map(json_value_to_document).collect()),
        Value::Object(map) => Document::Object(
            map.iter()
                .map(|(key, value)| (key.clone(), json_value_to_document(value)))
                .collect(),
        ),
    }
}

fn document_to_json_value(document: &Document) -> Value {
    match document {
        Document::Null => Value::Null,
        Document::Bool(flag) => Value::Bool(*flag),
        Document::Number(number) => match number {
            aws_smithy_types::Number::PosInt(value) => Value::Number((*value).into()),
            aws_smithy_types::Number::NegInt(value) => Value::Number((*value).into()),
            aws_smithy_types::Number::Float(value) => Value::Number(
                serde_json::Number::from_f64(*value).unwrap_or(serde_json::Number::from(0)),
            ),
        },
        Document::String(text) => Value::String(text.clone()),
        Document::Array(items) => Value::Array(items.iter().map(document_to_json_value).collect()),
        Document::Object(map) => Value::Object(
            map.iter()
                .map(|(key, value)| (key.clone(), document_to_json_value(value)))
                .collect(),
        ),
    }
}

// ---------------------------------------------------------------------------
// Converse response -> IR
// ---------------------------------------------------------------------------

pub(super) fn converse_output_to_unified(
    output: aws_sdk_bedrockruntime::operation::converse::ConverseOutput,
    model: &str,
) -> Result<UnifiedResponse, ProviderError> {
    let mut output_text = String::new();
    let mut tool_calls = Vec::new();
    let mut reasoning_blocks = Vec::new();

    if let aws_sdk_bedrockruntime::types::ConverseOutput::Message(message) = output
        .output
        .as_ref()
        .unwrap_or_else(|| panic!("converse output must carry a message"))
    {
        for block in message.content() {
            match block {
                ContentBlock::Text(text) => output_text.push_str(text),
                ContentBlock::ToolUse(use_block) => {
                    tool_calls.push(ToolCall {
                        id: Some(use_block.tool_use_id.clone()),
                        name: use_block.name.clone(),
                        description: String::new(),
                        arguments: document_to_json_value(&use_block.input).to_string(),
                    });
                }
                ContentBlock::ReasoningContent(reasoning) => {
                    reasoning_blocks.push(converse_reasoning_to_block(reasoning));
                }
                _ => {}
            }
        }
    }

    let finish_reason = converse_stop_reason_to_finish(output.stop_reason.as_str());
    let usage = output
        .usage
        .map(|usage| {
            EndpointUsage::from_endpoint_payload(
                "/chat/completions",
                serde_json::json!({
                    "prompt_tokens": usage.input_tokens,
                    "completion_tokens": usage.output_tokens,
                    "total_tokens": usage.total_tokens,
                }),
            )
        })
        .transpose()
        .map_err(ProviderError::internal)?
        .unwrap_or_else(|| {
            EndpointUsage::from_endpoint_payload("/chat/completions", serde_json::json!({}))
                .expect("empty chat usage payload parses")
        });

    let request_id = format!(
        "bedrock-{}",
        chrono::Utc::now().timestamp_nanos_opt().unwrap_or_default()
    );
    let mut response = UnifiedResponse::from_completion(
        request_id.clone(),
        chrono::Utc::now().to_rfc3339(),
        model.into(),
        output_text,
        None,
        (!tool_calls.is_empty()).then_some(tool_calls),
        finish_reason,
        usage,
    );
    for block in reasoning_blocks {
        let part = crate::provider::UnifiedContentPart {
            kind: if let Some(data) = block.encrypted_content {
                crate::provider::UnifiedContentPartKind::RedactedThinking { data }
            } else {
                crate::provider::UnifiedContentPartKind::Thinking {
                    text: block.thinking.unwrap_or_default(),
                    signature: block.signature,
                }
            },
            cache_control: None,
            source_extensions: None,
        };
        if let Some(candidate) = response.candidates.first_mut() {
            if let Some(item) = candidate.output.first_mut() {
                if let UnifiedOutputItemKind::Message { content, .. } = &mut item.kind {
                    content.insert(0, part);
                }
            }
        }
    }
    Ok(response)
}

/// Converse reasoning block -> IR reasoning block.
pub(super) fn converse_reasoning_to_block(reasoning: &ReasoningContentBlock) -> ReasoningBlock {
    match reasoning {
        aws_sdk_bedrockruntime::types::ReasoningContentBlock::ReasoningText(text) => {
            ReasoningBlock {
                id: None,
                summary: None,
                thinking: Some(text.text.clone()),
                signature: text.signature.clone(),
                encrypted_content: None,
                extra: HashMap::new(),
            }
        }
        aws_sdk_bedrockruntime::types::ReasoningContentBlock::RedactedContent(blob) => {
            ReasoningBlock {
                id: None,
                summary: None,
                thinking: None,
                signature: None,
                encrypted_content: Some(
                    base64::engine::general_purpose::STANDARD.encode(blob.as_ref()),
                ),
                extra: HashMap::new(),
            }
        }
        _ => ReasoningBlock::default(),
    }
}

pub(super) fn converse_stop_reason_to_finish(stop_reason: &str) -> FinishReason {
    match StopReason::from(stop_reason) {
        StopReason::MaxTokens => FinishReason::Length,
        StopReason::ToolUse => FinishReason::ToolCalls,
        StopReason::ContentFiltered => FinishReason::ContentFilter,
        _ => FinishReason::Stop,
    }
}

// ---------------------------------------------------------------------------
// ConverseStream events -> IR
// ---------------------------------------------------------------------------

/// Stream event aggregation state.
#[derive(Default)]
pub(super) struct ConverseStreamState {
    model: String,
    started: bool,
    request_id: String,
    /// content block index → (tool_use_id, name)
    tool_use_blocks: HashMap<i32, (String, String)>,
    tool_arguments: HashMap<i32, String>,
    tool_output_indexes: HashMap<i32, u32>,
}

impl ConverseStreamState {
    pub(super) fn new(model: String) -> Self {
        Self {
            model,
            ..Default::default()
        }
    }

    fn request_id(&mut self) -> String {
        if self.request_id.is_empty() {
            self.request_id = format!(
                "bedrock-{}",
                chrono::Utc::now().timestamp_nanos_opt().unwrap_or_default()
            );
        }
        self.request_id.clone()
    }

    pub(super) fn on_event(
        &mut self,
        event: aws_sdk_bedrockruntime::types::ConverseStreamOutput,
    ) -> Vec<UnifiedEvent> {
        use aws_sdk_bedrockruntime::types::ConverseStreamOutput as StreamEvent;
        let mut out = Vec::new();

        if !self.started {
            self.started = true;
            let request_id = self.request_id();
            let mut response = UnifiedResponse::new(request_id.clone(), self.model.clone());
            response.created_at = Some(chrono::Utc::now().timestamp());
            out.push(UnifiedEvent::new(UnifiedEventKind::ResponseCreated {
                response,
            }));
            out.push(
                UnifiedEvent::new(UnifiedEventKind::OutputItemAdded {
                    item: UnifiedOutputItem::new(
                        request_id.clone(),
                        UnifiedOutputItemKind::Message {
                            role: crate::provider::UnifiedRole::Assistant,
                            content: vec![],
                            phase: None,
                        },
                    ),
                })
                .located(0, request_id, 0, None),
            );
        }

        match event {
            StreamEvent::MessageStart(_) => {}
            StreamEvent::ContentBlockStart(start) => {
                let index = start.content_block_index;
                if let Some(aws_sdk_bedrockruntime::types::ContentBlockStart::ToolUse(use_block)) =
                    start.start.as_ref()
                {
                    self.tool_use_blocks.insert(
                        index,
                        (use_block.tool_use_id.clone(), use_block.name.clone()),
                    );
                    self.tool_arguments.insert(index, String::new());
                    let output_index = self.tool_output_indexes.len() as u32 + 1;
                    self.tool_output_indexes.insert(index, output_index);
                    let id = use_block.tool_use_id.clone();
                    let item = UnifiedOutputItem::new(
                        id.clone(),
                        UnifiedOutputItemKind::FunctionCall {
                            call_id: id.clone(),
                            name: use_block.name.clone(),
                            arguments: String::new(),
                            caller: None,
                        },
                    );
                    out.push(
                        UnifiedEvent::new(UnifiedEventKind::OutputItemAdded { item }).located(
                            0,
                            id,
                            output_index,
                            None,
                        ),
                    );
                }
            }
            StreamEvent::ContentBlockDelta(delta) => {
                let index = delta.content_block_index;
                if let Some(delta_block) = delta.delta.as_ref() {
                    match delta_block {
                        ContentBlockDelta::Text(text) => {
                            out.push(
                                UnifiedEvent::new(UnifiedEventKind::ContentPartDelta {
                                    delta: UnifiedContentDelta::Text(text.clone()),
                                })
                                .located(
                                    0,
                                    self.request_id(),
                                    0,
                                    Some(UnifiedPartLocation::Content(index as u32)),
                                ),
                            );
                        }
                        ContentBlockDelta::ToolUse(use_delta) => {
                            let (tool_use_id, _name) = self
                                .tool_use_blocks
                                .get(&index)
                                .cloned()
                                .unwrap_or_default();
                            self.tool_arguments
                                .entry(index)
                                .or_default()
                                .push_str(&use_delta.input);
                            out.push(
                                UnifiedEvent::new(UnifiedEventKind::ItemDelta {
                                    delta: UnifiedItemDelta::FunctionArguments(
                                        use_delta.input.clone(),
                                    ),
                                })
                                .located(
                                    0,
                                    tool_use_id,
                                    self.tool_output_indexes[&index],
                                    None,
                                ),
                            );
                        }
                        ContentBlockDelta::ReasoningContent(reasoning_delta) => {
                            if let aws_sdk_bedrockruntime::types::ReasoningContentBlockDelta::Text(
                                text,
                            ) = reasoning_delta
                            {
                                out.push(
                                    UnifiedEvent::new(UnifiedEventKind::ContentPartDelta {
                                        delta: UnifiedContentDelta::Thinking(text.clone()),
                                    })
                                    .located(
                                        0,
                                        self.request_id(),
                                        0,
                                        Some(UnifiedPartLocation::Content(index as u32)),
                                    ),
                                );
                            }
                        }
                        _ => {}
                    }
                }
            }
            StreamEvent::ContentBlockStop(stop) => {
                if let Some((tool_use_id, name)) =
                    self.tool_use_blocks.remove(&stop.content_block_index)
                {
                    let arguments = self
                        .tool_arguments
                        .remove(&stop.content_block_index)
                        .unwrap_or_default();
                    let item = UnifiedOutputItem::new(
                        tool_use_id.clone(),
                        UnifiedOutputItemKind::FunctionCall {
                            call_id: tool_use_id.clone(),
                            name,
                            arguments,
                            caller: None,
                        },
                    );
                    out.push(
                        UnifiedEvent::new(UnifiedEventKind::OutputItemDone { item }).located(
                            0,
                            tool_use_id,
                            self.tool_output_indexes[&stop.content_block_index],
                            None,
                        ),
                    );
                }
            }
            StreamEvent::MessageStop(stop) => {
                let mut event = UnifiedEvent::new(UnifiedEventKind::CandidateDone {
                    ending: UnifiedEnding::from_source(
                        &converse_stop_reason_to_finish(stop.stop_reason.as_str()).to_snake(),
                    ),
                });
                event.target = Some(UnifiedEventTarget {
                    candidate_index: 0,
                    item_id: None,
                    output_index: None,
                    part: None,
                });
                out.push(event);
                out.push(UnifiedEvent::new(UnifiedEventKind::Completed {
                    response: None,
                }));
            }
            StreamEvent::Metadata(metadata) => {
                if let Some(usage) = metadata.usage {
                    if let Ok(usage) = EndpointUsage::from_endpoint_payload(
                        "/chat/completions",
                        serde_json::json!({
                            "prompt_tokens": usage.input_tokens,
                            "completion_tokens": usage.output_tokens,
                            "total_tokens": usage.total_tokens,
                        }),
                    ) {
                        out.push(UnifiedEvent::new(UnifiedEventKind::Usage {
                            usage,
                            update: UnifiedUsageUpdate::Snapshot,
                            source_usage: None,
                        }));
                    }
                }
            }
            _ => {}
        }
        out
    }
}

impl FinishReason {
    fn to_snake(self) -> String {
        match self {
            FinishReason::Stop => "stop".to_string(),
            FinishReason::Length => "length".to_string(),
            FinishReason::ToolCalls => "tool_calls".to_string(),
            FinishReason::ContentFilter => "content_filter".to_string(),
            FinishReason::Other => "other".to_string(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::provider::{UnifiedInputItem, UnifiedMessageBoundary};

    #[test]
    fn haiku_55_rejects_assistant_prefill() {
        let message = converse_message(
            ConversationRole::Assistant,
            vec![ContentBlock::Text("continued response".into())],
        )
        .expect("assistant message");
        assert!(
            validate_model_conversation("global.anthropic.claude-haiku-5-5", &[message.clone()])
                .is_err()
        );
        assert!(
            validate_model_conversation("us.anthropic.claude-haiku-5-5-v1:0", &[message.clone()])
                .is_err()
        );
        assert!(validate_model_conversation("global.anthropic.claude-opus-5", &[message]).is_ok());
    }

    #[test]
    fn haiku_55_additional_fields_match_bedrock_parameters() {
        let expected = json!({
            "thinking": {"type":"adaptive"},
            "output_config": {"effort":"high"}
        });
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("high".into());
        request.options.thinking = Some(json!({"type": "adaptive"}));
        let fields = additional_model_request_fields(&request, "global.anthropic.claude-haiku-5-5")
            .expect("supported Bedrock parameters")
            .expect("additional fields");
        assert_eq!(document_to_json_value(&fields), expected);
        // Any inference-profile prefix resolves to the same model.
        let fields =
            additional_model_request_fields(&request, "us.anthropic.claude-haiku-5-5-v1:0")
                .expect("supported Bedrock parameters")
                .expect("additional fields");
        assert_eq!(document_to_json_value(&fields), expected);

        request.options.reasoning_effort = Some("medium".into());
        request.options.thinking = Some(json!({"type": "disabled"}));
        let fields = additional_model_request_fields(&request, "global.anthropic.claude-haiku-5-5")
            .expect("disabled thinking")
            .expect("additional fields");
        assert_eq!(
            document_to_json_value(&fields),
            json!({"thinking":{"type":"disabled"}, "output_config":{"effort":"medium"}})
        );

        request.options.thinking = Some(json!({"type":"adaptive", "display":"omitted"}));
        let fields = additional_model_request_fields(&request, "global.anthropic.claude-haiku-5-5")
            .expect("Bedrock accepts omitted display")
            .expect("additional fields");
        assert_eq!(
            document_to_json_value(&fields),
            json!({"thinking":{"type":"adaptive", "display":"omitted"}, "output_config":{"effort":"medium"}})
        );
    }

    #[test]
    fn haiku_55_additional_fields_reject_unsupported_parameter_variants() {
        for thinking in [
            json!({"type":"enabled"}),
            json!({"type":"adaptive", "budget_tokens":4096}),
            json!({"type":"disabled", "budget_tokens":0}),
            json!({"type":"adaptive", "display":"updates"}),
            json!({"type":"between_tools"}),
        ] {
            let mut request = UnifiedRequest::default();
            request.options.thinking = Some(thinking);
            assert!(
                additional_model_request_fields(&request, "global.anthropic.claude-haiku-5-5",)
                    .is_err()
            );
        }
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("none".into());
        assert!(
            additional_model_request_fields(&request, "global.anthropic.claude-haiku-5-5",)
                .is_err()
        );
    }

    #[test]
    fn haiku_55_rejects_block_binding() {
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({
            "type":"adaptive",
            "block_binding":{"prefix_mismatch_behavior":"drop_block"}
        }));
        assert!(
            additional_model_request_fields(&request, "global.anthropic.claude-haiku-5-5",)
                .is_err()
        );
    }

    #[test]
    fn mapped_claude_routes_encode_thinking_and_effort() {
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("xhigh".into());
        request.options.thinking = Some(json!({"type": "adaptive"}));
        let fields = additional_model_request_fields(&request, "global.anthropic.claude-sonnet-5")
            .expect("supported Bedrock parameters")
            .expect("additional fields");
        assert_eq!(
            document_to_json_value(&fields),
            json!({"thinking": {"type": "adaptive"}, "output_config": {"effort": "xhigh"}})
        );

        // Opus 5.5 is adaptive-only and must not be confused with Opus 5.
        request.options.reasoning_effort = Some("max".into());
        let fields =
            additional_model_request_fields(&request, "global.anthropic.claude-opus-5-5-v1")
                .expect("supported Bedrock parameters")
                .expect("additional fields");
        assert_eq!(
            document_to_json_value(&fields),
            json!({"thinking": {"type": "adaptive"}, "output_config": {"effort": "max"}})
        );

        let mut budget = UnifiedRequest::default();
        budget.options.max_output_tokens = Some(4096);
        budget.options.thinking = Some(json!({
            "type": "enabled",
            "budget_tokens": 1024,
            "display": "summarized"
        }));
        let fields = additional_model_request_fields(
            &budget,
            "global.anthropic.claude-haiku-4-5-20251001-v1:0",
        )
        .expect("supported Bedrock parameters")
        .expect("additional fields");
        assert_eq!(
            document_to_json_value(&fields),
            json!({"thinking": {"type": "enabled", "budget_tokens": 1024, "display": "summarized"}})
        );
    }

    #[test]
    fn mapped_claude_routes_reject_unsupported_parameter_variants() {
        let cases = [
            (
                "global.anthropic.claude-haiku-4-5-20251001-v1:0",
                json!({"type": "adaptive"}),
            ),
            (
                "global.anthropic.claude-sonnet-5",
                json!({"type": "enabled", "budget_tokens": 4096}),
            ),
            (
                "global.anthropic.claude-opus-4-7",
                json!({"type": "between_tools"}),
            ),
            (
                "global.anthropic.claude-opus-5-5",
                json!({"type": "disabled"}),
            ),
            (
                "global.anthropic.claude-fable-5-1",
                json!({"type": "adaptive", "budget_tokens": 4096}),
            ),
            (
                "global.anthropic.claude-opus-4-6",
                json!({"type": "adaptive", "display": "updates"}),
            ),
        ];
        for (model, thinking) in cases {
            let mut request = UnifiedRequest::default();
            request.options.max_output_tokens = Some(4096);
            let label = thinking.to_string();
            request.options.thinking = Some(thinking);
            assert!(
                additional_model_request_fields(&request, model).is_err(),
                "{model} accepted {label}"
            );
        }

        // Budget bounds and the missing effort parameter of Haiku 4.5.
        let haiku = "global.anthropic.claude-haiku-4-5-20251001-v1:0";
        let mut too_small = UnifiedRequest::default();
        too_small.options.max_output_tokens = Some(4096);
        too_small.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 512}));
        assert!(additional_model_request_fields(&too_small, haiku).is_err());

        let mut not_below_max = UnifiedRequest::default();
        not_below_max.options.max_output_tokens = Some(1024);
        not_below_max.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 1024}));
        assert!(additional_model_request_fields(&not_below_max, haiku).is_err());

        let mut effort = UnifiedRequest::default();
        effort.options.reasoning_effort = Some("high".into());
        assert!(additional_model_request_fields(&effort, haiku).is_err());

        // xhigh is outside the Opus 4.6 enum.
        let mut opus_46 = UnifiedRequest::default();
        opus_46.options.reasoning_effort = Some("xhigh".into());
        assert!(
            additional_model_request_fields(&opus_46, "global.anthropic.claude-opus-4-6-v1")
                .is_err()
        );
    }

    #[test]
    fn unmapped_claude_routes_pass_reasoning_controls_through() {
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("ultra".into());
        request.options.thinking = Some(json!({
            "type": "enabled",
            "budget_tokens": 512,
            "display": "updates",
            "block_binding": {"prefix_mismatch_behavior": "error"}
        }));
        let fields = additional_model_request_fields(&request, "global.anthropic.claude-opus-9")
            .expect("unmapped routes pass controls through")
            .expect("additional fields");
        assert_eq!(
            document_to_json_value(&fields),
            json!({
                "thinking": {
                    "type": "enabled",
                    "budget_tokens": 512,
                    "display": "updates",
                    "block_binding": {"prefix_mismatch_behavior": "error"}
                },
                "output_config": {"effort": "ultra"}
            })
        );

        let empty = UnifiedRequest::default();
        assert!(
            additional_model_request_fields(&empty, "global.anthropic.claude-opus-9")
                .expect("nothing to forward")
                .is_none()
        );
    }

    #[test]
    fn unified_converse_drops_empty_text_but_preserves_assistant_tool_use() {
        let mut assistant = UnifiedInputItem::new(UnifiedInputItemKind::Message {
            role: UnifiedRole::Assistant,
            content: vec![
                UnifiedContentPart {
                    kind: UnifiedContentPartKind::Text {
                        text: String::new(),
                        annotations: Vec::new(),
                        citations: None,
                        logprobs: None,
                    },
                    cache_control: None,
                    source_extensions: None,
                },
                UnifiedContentPart {
                    kind: UnifiedContentPartKind::Thinking {
                        text: String::new(),
                        signature: None,
                    },
                    cache_control: None,
                    source_extensions: None,
                },
            ],
            phase: None,
        });
        let boundary = UnifiedMessageBoundary {
            message_id: "assistant-tool-call".into(),
            role: UnifiedRole::Assistant,
            block_index: None,
        };
        assistant.message = Some(boundary.clone());
        let mut tool_call = UnifiedInputItem::new(UnifiedInputItemKind::FunctionCall {
            call_id: "call-1".into(),
            name: "read".into(),
            arguments: "{}".into(),
            caller: None,
            cache_control: None,
        });
        tool_call.message = Some(boundary);

        let mut request = UnifiedRequest::default();
        request.instructions = Some(vec![UnifiedInputItem::new(UnifiedInputItemKind::Message {
            role: UnifiedRole::System,
            content: vec![UnifiedContentPart {
                kind: UnifiedContentPartKind::Text {
                    text: String::new(),
                    annotations: Vec::new(),
                    citations: None,
                    logprobs: None,
                },
                cache_control: None,
                source_extensions: None,
            }],
            phase: None,
        })]);
        request.input = vec![
            UnifiedInputItem::new(UnifiedInputItemKind::Message {
                role: UnifiedRole::User,
                content: vec![UnifiedContentPart::text("read the file".into())],
                phase: None,
            }),
            UnifiedInputItem::new(UnifiedInputItemKind::Reasoning {
                summary: vec![json!({"type":"summary_text","text":""})],
                content: None,
                encrypted_content: None,
            }),
            assistant,
            tool_call,
        ];

        let (messages, system, _, _) = unified_to_converse_request(&request).expect("Converse");
        assert!(system.is_none());
        assert_eq!(messages.len(), 2);
        let blocks = messages[1].content();
        assert_eq!(blocks.len(), 1);
        assert!(matches!(
            blocks.first(),
            Some(ContentBlock::ToolUse(call)) if call.tool_use_id == "call-1"
        ));
    }

    #[test]
    fn unified_converse_merges_consecutive_assistant_items_for_tool_results() {
        let request = UnifiedRequest {
            input: vec![
                UnifiedInputItem::new(UnifiedInputItemKind::Message {
                    role: UnifiedRole::User,
                    content: vec![UnifiedContentPart::text("read file".into())],
                    phase: None,
                }),
                UnifiedInputItem::new(UnifiedInputItemKind::FunctionCall {
                    call_id: "call-1".into(),
                    name: "read".into(),
                    arguments: "{}".into(),
                    caller: None,
                    cache_control: None,
                }),
                UnifiedInputItem::new(UnifiedInputItemKind::Message {
                    role: UnifiedRole::Assistant,
                    content: vec![UnifiedContentPart::text("I read the file.".into())],
                    phase: None,
                }),
                UnifiedInputItem::new(UnifiedInputItemKind::FunctionCallOutput {
                    call_id: "call-1".into(),
                    output: UnifiedToolOutput::Text("file contents".into()),
                    is_error: None,
                    cache_control: None,
                }),
            ],
            ..Default::default()
        };

        let (messages, _, _, _) = unified_to_converse_request(&request).expect("Converse");
        assert_eq!(messages.len(), 3);
        let assistant_blocks = messages[1].content();
        assert_eq!(assistant_blocks.len(), 2);
        assert!(matches!(&assistant_blocks[0], ContentBlock::ToolUse(_)));
        assert!(
            matches!(&assistant_blocks[1], ContentBlock::Text(text) if text == "I read the file.")
        );
        assert!(matches!(
            messages[2].content().first(),
            Some(ContentBlock::ToolResult(result)) if result.tool_use_id == "call-1"
        ));
    }

    #[test]
    fn unified_converse_rejects_unsigned_nonempty_thinking_history() {
        let request = UnifiedRequest {
            input: vec![
                UnifiedInputItem::new(UnifiedInputItemKind::Message {
                    role: UnifiedRole::User,
                    content: vec![UnifiedContentPart::text("continue".into())],
                    phase: None,
                }),
                UnifiedInputItem::new(UnifiedInputItemKind::Message {
                    role: UnifiedRole::Assistant,
                    content: vec![UnifiedContentPart {
                        kind: UnifiedContentPartKind::Thinking {
                            text: "prior thought".into(),
                            signature: None,
                        },
                        cache_control: None,
                        source_extensions: None,
                    }],
                    phase: None,
                }),
            ],
            ..Default::default()
        };
        let error =
            unified_to_converse_request(&request).expect_err("unsigned thought cannot be replayed");
        assert!(error.contains("requires a signature"));
        assert!(!error.contains("Bedrock"));
        assert!(!error.contains("Converse"));
    }

    #[test]
    fn unified_converse_rejects_request_with_only_empty_text() {
        let request = UnifiedRequest {
            input: vec![UnifiedInputItem::new(UnifiedInputItemKind::Message {
                role: UnifiedRole::User,
                content: vec![UnifiedContentPart {
                    kind: UnifiedContentPartKind::Text {
                        text: String::new(),
                        annotations: Vec::new(),
                        citations: None,
                        logprobs: None,
                    },
                    cache_control: None,
                    source_extensions: None,
                }],
                phase: None,
            })],
            ..Default::default()
        };
        assert_eq!(
            unified_to_converse_request(&request).expect_err("empty input must be rejected"),
            "request has no convertible messages"
        );
    }

    #[test]
    fn unified_converse_preserves_interleaved_thinking_text_and_calls() {
        let request = crate::api::request_from_value(&serde_json::json!({
            "model":"claude-sonnet-4-6", "max_tokens":8192,
            "messages":[
                {"role":"assistant","content":[
                    {"type":"thinking","thinking":"plan","signature":"opaque-signature"},
                    {"type":"text","text":"before"},
                    {"type":"tool_use","id":"call-1","name":"lookup","input":{"x":1}},
                    {"type":"text","text":"after"}
                ]},
                {"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":"done"}]}
            ]
        }), crate::provider::UnifiedSourceProtocol::Messages).expect("unified request");
        let (messages, _, _, inference) = unified_to_converse_request(&request).expect("Converse");
        assert_eq!(messages.len(), 2);
        let blocks = &messages[0].content;
        assert_eq!(blocks.len(), 4);
        let ContentBlock::ReasoningContent(ReasoningContentBlock::ReasoningText(thinking)) =
            &blocks[0]
        else {
            panic!("thinking block");
        };
        assert_eq!(thinking.text, "plan");
        assert_eq!(thinking.signature.as_deref(), Some("opaque-signature"));
        assert!(matches!(&blocks[1], ContentBlock::Text(text) if text == "before"));
        assert!(matches!(&blocks[2], ContentBlock::ToolUse(call) if call.tool_use_id == "call-1"));
        assert!(matches!(&blocks[3], ContentBlock::Text(text) if text == "after"));
        assert_eq!(inference.max_tokens, Some(8192));
    }

    #[test]
    fn unified_converse_rejects_invalid_function_arguments_without_replacing_them() {
        let mut request = UnifiedRequest::default();
        request.input.push(crate::provider::UnifiedInputItem::new(
            UnifiedInputItemKind::FunctionCall {
                call_id: "call-1".into(),
                name: "lookup".into(),
                arguments: "invalid-json".into(),
                caller: None,
                cache_control: None,
            },
        ));
        assert!(unified_to_converse_request(&request).is_err());
    }
    use crate::openai_types::Message;
    use aws_sdk_bedrockruntime::types::{
        ContentBlockDeltaEvent, ContentBlockStartEvent, ContentBlockStopEvent, ConverseOutput,
        ConverseStreamMetadataEvent, ConverseStreamOutput, MessageStopEvent, TokenUsage,
        ToolUseBlockDelta,
    };
    use serde_json::json;

    fn ir_request() -> ChatCompletionRequest {
        ChatCompletionRequest {
            model: "client-model".to_string(),
            messages: vec![
                Message {
                    role: Role::System,
                    content: Content::Text("be terse".to_string()),
                    reasoning_content: None,
                    reasoning_blocks: None,
                    cache_control: None,
                    tool_call_id: None,
                    tool_calls: None,
                },
                Message {
                    role: Role::User,
                    content: Content::Text("run shell".to_string()),
                    reasoning_content: None,
                    reasoning_blocks: None,
                    cache_control: None,
                    tool_call_id: None,
                    tool_calls: None,
                },
                Message {
                    role: Role::Assistant,
                    content: Content::Text(String::new()),
                    reasoning_content: None,
                    reasoning_blocks: None,
                    cache_control: None,
                    tool_call_id: None,
                    tool_calls: Some(vec![crate::openai_types::ToolCall {
                        id: Some("call_1".to_string()),
                        r#type: Some("function".to_string()),
                        function: crate::openai_types::ToolCallFunction {
                            name: "shell".to_string(),
                            arguments: r#"{"cmd":"ls"}"#.to_string(),
                            description: None,
                        },
                    }]),
                },
                Message {
                    role: Role::Tool,
                    content: Content::Text(r#"{"out":"ok"}"#.to_string()),
                    reasoning_content: None,
                    reasoning_blocks: None,
                    cache_control: None,
                    tool_call_id: Some("call_1".to_string()),
                    tool_calls: None,
                },
            ],
            tools: Some(vec![crate::openai_types::ToolDefinition {
                r#type: "function".to_string(),
                function: crate::openai_types::FunctionDefinition {
                    name: "shell".to_string(),
                    description: Some("run shell".to_string()),
                    strict: None,
                    parameters: json!({"type": "object"}),
                },

                extra: Default::default(),
            }]),
            max_completion_tokens: Some(512),
            temperature: Some(0.3),
            stop: Some(vec!["END".to_string()]),
            ..Default::default()
        }
    }

    #[test]
    fn ir_to_converse_maps_roles_tools_and_inference() {
        let (messages, system, tool_config, inference) =
            ir_to_converse_request(&ir_request()).expect("convert");

        assert_eq!(messages.len(), 3);
        // user message text
        assert_eq!(*messages[0].role(), ConversationRole::User);
        assert!(matches!(
            messages[0].content().first(),
            Some(ContentBlock::Text(text)) if text == "run shell"
        ));
        // assistant message with tool use
        assert_eq!(*messages[1].role(), ConversationRole::Assistant);
        assert!(messages[1].content().iter().any(|block| matches!(
            block,
            ContentBlock::ToolUse(use_block)
                if use_block.tool_use_id == "call_1" && use_block.name == "shell"
        )));
        // tool result message (role User + ToolResult block)
        assert!(messages[2].content().iter().any(|block| matches!(
            block,
            ContentBlock::ToolResult(result) if result.tool_use_id == "call_1"
        )));
        // system
        let system = system.expect("system blocks");
        assert!(matches!(&system[0], SystemContentBlock::Text(text) if text == "be terse"));
        // tools
        let tool_config = tool_config.expect("tool config");
        assert_eq!(tool_config.tools().len(), 1);
        assert!(matches!(
            tool_config.tools().first(),
            Some(ConverseTool::ToolSpec(spec)) if spec.name() == "shell"
        ));
        // inference
        assert_eq!(inference.max_tokens(), Some(512));
        assert_eq!(inference.stop_sequences(), ["END"]);
    }

    #[test]
    fn tool_message_requires_tool_call_id() {
        let mut request = ir_request();
        request.messages[3].tool_call_id = None;
        let err = ir_to_converse_request(&request).expect_err("missing id must be rejected");
        assert!(err.contains("tool_call_id"));
    }

    #[test]
    fn document_json_roundtrip() {
        let value = json!({"a": 1, "b": [true, "x", 1.5], "c": null});
        let document = json_value_to_document(&value);
        assert_eq!(document_to_json_value(&document), value);
    }

    #[test]
    fn stop_reason_maps_to_finish_reason() {
        assert_eq!(
            converse_stop_reason_to_finish("end_turn"),
            FinishReason::Stop
        );
        assert_eq!(
            converse_stop_reason_to_finish("max_tokens"),
            FinishReason::Length
        );
        assert_eq!(
            converse_stop_reason_to_finish("tool_use"),
            FinishReason::ToolCalls
        );
        assert_eq!(
            converse_stop_reason_to_finish("content_filtered"),
            FinishReason::ContentFilter
        );
    }

    #[test]
    fn converse_reasoning_roundtrips_through_ir_block() {
        // text + signature
        let text_block = ReasoningContentBlock::ReasoningText(
            ReasoningTextBlock::builder()
                .text("thinking")
                .signature("sig")
                .build()
                .expect("reasoning text"),
        );
        let block = converse_reasoning_to_block(&text_block);
        assert_eq!(block.thinking.as_deref(), Some("thinking"));
        assert_eq!(block.signature.as_deref(), Some("sig"));

        // redacted (encrypted) payload base64 round trip
        let blob_block = ReasoningContentBlock::RedactedContent(Blob::new(b"opaque".to_vec()));
        let block = converse_reasoning_to_block(&blob_block);
        let encoded = block.encrypted_content.as_deref().expect("encoded");
        let decoded = base64::engine::general_purpose::STANDARD
            .decode(encoded)
            .expect("decode");
        assert_eq!(decoded, b"opaque");

        // reverse: IR block -> content block
        let content_block = reasoning_block_to_content_block(&block).expect("rebuild");
        assert!(matches!(
            content_block,
            ContentBlock::ReasoningContent(ReasoningContentBlock::RedactedContent(_))
        ));
    }

    #[test]
    fn converse_stream_state_maps_event_sequence() {
        let mut state = ConverseStreamState::new("global.anthropic.claude-opus-5".to_string());

        let events = state.on_event(ConverseStreamOutput::MessageStart(
            aws_sdk_bedrockruntime::types::MessageStartEvent::builder()
                .role(ConversationRole::Assistant)
                .build()
                .expect("message start event"),
        ));
        assert!(matches!(
            &events[0].kind,
            UnifiedEventKind::ResponseCreated { .. }
        ));
        assert!(matches!(
            &events[1].kind,
            UnifiedEventKind::OutputItemAdded { .. }
        ));

        // toolUse block start
        let events = state.on_event(ConverseStreamOutput::ContentBlockStart(
            ContentBlockStartEvent::builder()
                .content_block_index(1)
                .start(aws_sdk_bedrockruntime::types::ContentBlockStart::ToolUse(
                    aws_sdk_bedrockruntime::types::ToolUseBlockStart::builder()
                        .tool_use_id("call_1")
                        .name("shell")
                        .build()
                        .expect("tool use block start"),
                ))
                .build()
                .expect("block start event"),
        ));
        assert!(matches!(
            events[0].kind,
            UnifiedEventKind::OutputItemAdded { .. }
        ));

        // text delta
        let events = state.on_event(ConverseStreamOutput::ContentBlockDelta(
            ContentBlockDeltaEvent::builder()
                .content_block_index(0)
                .delta(ContentBlockDelta::Text("hi".to_string()))
                .build()
                .expect("text delta event"),
        ));
        assert!(matches!(
            &events[0].kind,
            UnifiedEventKind::ContentPartDelta { delta: UnifiedContentDelta::Text(delta) } if delta == "hi"
        ));

        // toolUse input delta
        let events = state.on_event(ConverseStreamOutput::ContentBlockDelta(
            ContentBlockDeltaEvent::builder()
                .content_block_index(1)
                .delta(ContentBlockDelta::ToolUse(
                    ToolUseBlockDelta::builder()
                        .input(r#"{"cmd""#.to_string())
                        .build()
                        .expect("tool use delta"),
                ))
                .build()
                .expect("tool delta event"),
        ));
        match &events[0].kind {
            UnifiedEventKind::ItemDelta {
                delta: UnifiedItemDelta::FunctionArguments(arguments_delta),
            } => {
                let target = events[0].target.as_ref().expect("target");
                assert_eq!(target.item_id.as_deref(), Some("call_1"));
                assert_eq!(arguments_delta, r#"{"cmd""#);
                assert_eq!(target.output_index, Some(1));
            }
            other => panic!("expected tool call delta: {other:?}"),
        }

        // toolUse block stop → ToolCallDone
        let events = state.on_event(ConverseStreamOutput::ContentBlockStop(
            ContentBlockStopEvent::builder()
                .content_block_index(1)
                .build()
                .expect("block stop event"),
        ));
        assert!(matches!(
            &events[0].kind,
            UnifiedEventKind::OutputItemDone { item } if matches!(&item.kind, UnifiedOutputItemKind::FunctionCall { name, .. } if name == "shell")
        ));

        // messageStop → Completed
        let events = state.on_event(ConverseStreamOutput::MessageStop(
            MessageStopEvent::builder()
                .stop_reason(aws_sdk_bedrockruntime::types::StopReason::ToolUse)
                .build()
                .expect("message stop event"),
        ));
        assert!(events.iter().any(|event| matches!(
            &event.kind,
            UnifiedEventKind::CandidateDone { ending } if ending.to_chat() == "tool_calls"
        )));

        // metadata → Usage
        let events = state.on_event(ConverseStreamOutput::Metadata(
            ConverseStreamMetadataEvent::builder()
                .usage(
                    TokenUsage::builder()
                        .input_tokens(10)
                        .output_tokens(5)
                        .total_tokens(15)
                        .build()
                        .expect("token usage"),
                )
                .build(),
        ));
        assert!(matches!(
            &events[0].kind,
            UnifiedEventKind::Usage { usage, .. } if matches!(usage, EndpointUsage::ChatCompletions(_))
        ));
    }

    #[test]
    fn first_tool_block_has_distinct_output_index() {
        let mut state = ConverseStreamState::new("m".into());
        let events = state.on_event(ConverseStreamOutput::ContentBlockStart(
            ContentBlockStartEvent::builder()
                .content_block_index(0)
                .start(aws_sdk_bedrockruntime::types::ContentBlockStart::ToolUse(
                    aws_sdk_bedrockruntime::types::ToolUseBlockStart::builder()
                        .tool_use_id("call")
                        .name("lookup")
                        .build()
                        .expect("tool"),
                ))
                .build()
                .expect("start"),
        ));
        let mut response = UnifiedResponse::new(String::new(), "m".into());
        for event in &events {
            crate::provider::apply_event(&mut response, event).expect("aggregate");
        }
        assert_eq!(response.candidates[0].output.len(), 2);
        let events = state.on_event(ConverseStreamOutput::ContentBlockStop(
            ContentBlockStopEvent::builder()
                .content_block_index(0)
                .build()
                .expect("stop"),
        ));
        for event in &events {
            crate::provider::apply_event(&mut response, event).expect("aggregate done");
        }
        assert!(
            matches!(&response.candidates[0].output[1].kind, UnifiedOutputItemKind::FunctionCall { call_id, .. } if call_id == "call")
        );
    }

    #[test]
    fn converse_output_message_maps_to_unified() {
        let message = ConverseMessage::builder()
            .role(ConversationRole::Assistant)
            .content(ContentBlock::Text("answer".to_string()))
            .content(ContentBlock::ToolUse(
                ToolUseBlock::builder()
                    .tool_use_id("call_1")
                    .name("shell")
                    .input(json_value_to_document(&json!({"cmd": "ls"})))
                    .build()
                    .expect("tool use"),
            ))
            .build()
            .expect("message");
        let output = aws_sdk_bedrockruntime::operation::converse::ConverseOutput::builder()
            .output(ConverseOutput::Message(message))
            .stop_reason(StopReason::ToolUse)
            .usage(
                aws_sdk_bedrockruntime::types::TokenUsage::builder()
                    .input_tokens(10)
                    .output_tokens(4)
                    .total_tokens(14)
                    .build()
                    .expect("token usage"),
            )
            .build()
            .expect("converse output");

        let unified = converse_output_to_unified(output, "global.anthropic.claude-opus-5")
            .expect("map output");
        let chat = crate::openai_http_mapping::map_openai_response(unified.clone());
        assert_eq!(
            chat.choices[0].message.content,
            Some(Content::Text("answer".into()))
        );
        let calls = chat.choices[0]
            .message
            .tool_calls
            .as_ref()
            .expect("tool calls");
        assert_eq!(calls[0].function.name, "shell");
        assert_eq!(calls[0].function.arguments, r#"{"cmd":"ls"}"#);
        assert_eq!(chat.choices[0].finish_reason.as_deref(), Some("tool_calls"));
        assert!(matches!(
            unified.usage,
            Some(EndpointUsage::ChatCompletions(_))
        ));
    }
}
