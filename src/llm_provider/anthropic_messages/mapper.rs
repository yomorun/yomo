use log::warn;
use serde_json::{Value, json};

use crate::llm_provider::{FinishReason, ProviderError, ToolCall, UnifiedEvent, UnifiedResponse};
use crate::model_api_provider::MessagesUsage;
use crate::openai_types::{ChatCompletionRequest, Content, ContentPart, Role, ToolChoice};
use crate::usage_handler::EndpointUsage;

use super::types::{
    ActiveBlock, AnthropicContentBlock, AnthropicImageSource, AnthropicMessage, AnthropicResponse,
    AnthropicResponseContentBlock, AnthropicThinking, AnthropicTool, AnthropicToolChoice,
    RequestParts, StreamContentBlock, StreamContentDelta, StreamEvent, StreamState,
};

pub(super) fn map_request(
    request: ChatCompletionRequest,
    upstream_model: String,
    default_max_tokens: i32,
    sampling_compat: bool,
) -> Result<RequestParts, ProviderError> {
    let (temperature, top_p) = map_sampling_params(
        request.temperature,
        request.top_p,
        &request.model,
        sampling_compat,
    );
    let mut system_chunks = Vec::<String>::new();
    let mut messages = Vec::<AnthropicMessage>::new();

    for message in request.messages {
        match message.role {
            Role::System | Role::Developer => {
                let text = extract_text_from_content(&message.content);
                if !text.trim().is_empty() {
                    system_chunks.push(text);
                }
            }
            Role::User => {
                let blocks = map_regular_content(&message.content)?;
                if !blocks.is_empty() {
                    messages.push(AnthropicMessage {
                        role: "user".to_string(),
                        content: blocks,
                    });
                }
            }
            Role::Assistant => {
                let mut blocks = map_regular_content(&message.content)?;
                if let Some(tool_calls) = message.tool_calls {
                    for (index, call) in tool_calls.into_iter().enumerate() {
                        let id = call.id.unwrap_or_else(|| {
                            format!("toolu_{}_{}", uuid_suffix(&call.function.name), index)
                        });
                        let input = serde_json::from_str::<Value>(&call.function.arguments)
                            .unwrap_or_else(|_| json!({"input": call.function.arguments}));
                        blocks.push(AnthropicContentBlock::ToolUse {
                            id,
                            name: call.function.name,
                            input,
                        });
                    }
                }
                if !blocks.is_empty() {
                    messages.push(AnthropicMessage {
                        role: "assistant".to_string(),
                        content: blocks,
                    });
                }
            }
            Role::Tool => {
                let tool_use_id = message
                    .tool_call_id
                    .clone()
                    .filter(|id| !id.trim().is_empty())
                    .ok_or_else(|| {
                        ProviderError::internal("tool_call_id is required for tool messages")
                    })?;
                let result_text = extract_text_from_content(&message.content);
                let content = (!result_text.is_empty())
                    .then_some(vec![AnthropicContentBlock::Text { text: result_text }]);
                messages.push(AnthropicMessage {
                    role: "user".to_string(),
                    content: vec![AnthropicContentBlock::ToolResult {
                        tool_use_id,
                        content,
                        is_error: None,
                    }],
                });
            }
        }
    }

    let tools = request.tools.map(|tools| {
        tools
            .into_iter()
            .map(|tool| AnthropicTool {
                name: tool.function.name,
                description: tool.function.description,
                input_schema: tool.function.parameters,
            })
            .collect::<Vec<_>>()
    });

    let disable_parallel_tool_use = request.parallel_tool_calls.map(|parallel| !parallel);
    let tool_choice = request.tool_choice.as_ref().map(|choice| match choice {
        ToolChoice::Name(name) if name == "auto" => AnthropicToolChoice::Auto {
            disable_parallel_tool_use,
        },
        ToolChoice::Name(name) if name == "required" => AnthropicToolChoice::Any {
            disable_parallel_tool_use,
        },
        ToolChoice::Name(name) if name == "none" => AnthropicToolChoice::None,
        ToolChoice::Object { function, .. } => AnthropicToolChoice::Tool {
            name: function.name.clone(),
            disable_parallel_tool_use,
        },
        _ => AnthropicToolChoice::Auto {
            disable_parallel_tool_use,
        },
    });

    let thinking = map_thinking(
        &upstream_model,
        request.thinking.as_ref(),
        request.reasoning_effort.as_deref(),
    );

    Ok(RequestParts {
        model: upstream_model,
        max_tokens: request.max_completion_tokens.unwrap_or(default_max_tokens),
        messages,
        system: (!system_chunks.is_empty()).then(|| system_chunks.join("\n")),
        temperature,
        top_p,
        stop_sequences: request.stop,
        tools,
        tool_choice,
        thinking,
    })
}

fn map_sampling_params(
    temperature: Option<f32>,
    top_p: Option<f32>,
    model: &str,
    sampling_compat: bool,
) -> (Option<f32>, Option<f32>) {
    if !sampling_compat || !is_sampling_restricted_model(model) {
        return (temperature, top_p);
    }

    (
        filter_non_default_sampling_value(temperature),
        filter_non_default_sampling_value(top_p),
    )
}

fn filter_non_default_sampling_value(value: Option<f32>) -> Option<f32> {
    match value {
        Some(v) if (v - 1.0).abs() < f32::EPSILON => Some(v),
        Some(_) => None,
        None => None,
    }
}

fn is_sampling_restricted_model(model: &str) -> bool {
    matches!(
        model.to_ascii_lowercase().as_str(),
        "claude-sonnet-5"
            | "claude-opus-5"
            | "claude-opus-4-8"
            | "claude-opus-4-7"
            | "claude-fable-5"
            | "claude-mythos-5"
    )
}

pub(super) fn map_response(response: AnthropicResponse) -> UnifiedResponse {
    let mut output_text = String::new();
    let mut reasoning_content = String::new();
    let mut tool_calls = Vec::new();

    for block in response.content {
        match block {
            AnthropicResponseContentBlock::Text { text } => output_text.push_str(&text),
            AnthropicResponseContentBlock::Thinking { thinking } => {
                reasoning_content.push_str(&thinking)
            }
            AnthropicResponseContentBlock::ToolUse { id, name, input } => {
                tool_calls.push(ToolCall {
                    id: Some(id),
                    name,
                    description: String::new(),
                    arguments: input.to_string(),
                });
            }
            AnthropicResponseContentBlock::Unknown => {}
        }
    }

    if response.usage.is_none() {
        warn!(
            "anthropic messages upstream response missing usage; model={} request_id={}",
            response.model, response.id
        );
    }
    let usage = EndpointUsage::Messages(response.usage.unwrap_or(MessagesUsage {
        input_tokens: None,
        output_tokens: None,
        cache_creation_input_tokens: None,
        cache_read_input_tokens: None,
        cache_creation: None,
        inference_geo: None,
        service_tier: None,
        server_tool_use: None,
    }));

    UnifiedResponse {
        request_id: response.id,
        created_at: chrono::Utc::now().to_rfc3339(),
        model: response.model,
        output_text,
        reasoning_content: (!reasoning_content.is_empty()).then_some(reasoning_content),
        tool_calls: (!tool_calls.is_empty()).then_some(tool_calls),
        finish_reason: map_finish_reason(response.stop_reason.as_deref()),
        usage,
    }
}

pub(super) fn map_stream_event(event: StreamEvent, state: &mut StreamState) -> Vec<UnifiedEvent> {
    let mut out = Vec::new();

    match event {
        StreamEvent::MessageStart { message } => {
            state.response_id = message.id;
            state.model = message.model;
            state.created_at = chrono::Utc::now().to_rfc3339();
            state.started = true;
            out.push(UnifiedEvent::ResponseCreated {
                id: state.response_id.clone(),
                model: state.model.clone(),
                created_at: state.created_at.clone(),
            });
            out.push(UnifiedEvent::ResponseInProgress {
                id: state.response_id.clone(),
                model: state.model.clone(),
                created_at: state.created_at.clone(),
            });
            out.push(UnifiedEvent::MessageStart {
                id: state.response_id.clone(),
                role: "assistant".to_string(),
            });
        }
        StreamEvent::ContentBlockStart {
            index,
            content_block,
        } => match content_block {
            StreamContentBlock::ToolUse { id, name, input } => {
                let arguments = input.to_string();
                state.blocks.insert(
                    index,
                    ActiveBlock::ToolUse {
                        id: id.clone(),
                        name: name.clone(),
                        arguments: arguments.clone(),
                    },
                );
                if arguments != "{}" {
                    out.push(UnifiedEvent::ToolCallDelta {
                        id,
                        name,
                        arguments_delta: arguments,
                    });
                }
            }
            _ => {
                state.blocks.insert(index, ActiveBlock::Other);
            }
        },
        StreamEvent::ContentBlockDelta { index, delta } => match delta {
            StreamContentDelta::TextDelta { text } => {
                out.push(UnifiedEvent::MessageDelta {
                    id: state.response_id.clone(),
                    delta: text,
                });
            }
            StreamContentDelta::ThinkingDelta { thinking } => {
                out.push(UnifiedEvent::ThinkingDelta {
                    id: state.response_id.clone(),
                    delta: thinking,
                });
            }
            StreamContentDelta::InputJsonDelta { partial_json } => {
                if !partial_json.is_empty() {
                    if let Some(ActiveBlock::ToolUse {
                        id,
                        name,
                        arguments,
                    }) = state.blocks.get_mut(&index)
                    {
                        arguments.push_str(&partial_json);
                        out.push(UnifiedEvent::ToolCallDelta {
                            id: id.clone(),
                            name: name.clone(),
                            arguments_delta: partial_json,
                        });
                    }
                }
            }
            StreamContentDelta::Unknown => {}
        },
        StreamEvent::ContentBlockStop { index } => {
            if let Some(ActiveBlock::ToolUse {
                id,
                name,
                arguments,
            }) = state.blocks.remove(&index)
            {
                out.push(UnifiedEvent::ToolCallDone {
                    id,
                    name,
                    arguments,
                });
            }
        }
        StreamEvent::MessageDelta { delta, usage } => {
            if let Some(delta) = delta {
                state.stop_reason = delta.stop_reason;
            }
            if let Some(usage) = usage {
                state.usage_received = true;
                out.push(UnifiedEvent::Usage {
                    usage: EndpointUsage::Messages(usage),
                });
            }
        }
        StreamEvent::MessageStop => {
            if state.started && !state.completed {
                let stop_reason = state
                    .stop_reason
                    .clone()
                    .unwrap_or_else(|| "end_turn".to_string());
                out.push(UnifiedEvent::MessageStop {
                    id: state.response_id.clone(),
                    stop_reason: Some(map_stop_reason_string(&stop_reason).to_string()),
                });
                out.push(UnifiedEvent::Completed {
                    finish_reason: Some(map_stop_reason_string(&stop_reason).to_string()),
                });
                state.completed = true;
            }
        }
        StreamEvent::Ping | StreamEvent::Unknown => {}
    }

    out
}

fn map_regular_content(content: &Content) -> Result<Vec<AnthropicContentBlock>, ProviderError> {
    match content {
        Content::Text(text) => {
            if text.is_empty() {
                return Ok(Vec::new());
            }
            Ok(vec![AnthropicContentBlock::Text { text: text.clone() }])
        }
        Content::Parts(parts) => {
            let mut blocks = Vec::new();
            for part in parts {
                match part {
                    ContentPart::Text { text } => {
                        if !text.is_empty() {
                            blocks.push(AnthropicContentBlock::Text { text: text.clone() });
                        }
                    }
                    ContentPart::Image { image_url } => {
                        blocks.push(AnthropicContentBlock::Image {
                            source: AnthropicImageSource::Url {
                                url: image_url.url.clone(),
                            },
                        });
                    }
                    ContentPart::InputAudio { .. } | ContentPart::File { .. } => {
                        return Err(ProviderError::internal(
                            "anthropic messages provider does not support input_audio/file parts",
                        ));
                    }
                }
            }
            Ok(blocks)
        }
    }
}

fn map_thinking(
    model: &str,
    thinking: Option<&crate::openai_types::ThinkingConfig>,
    reasoning_effort: Option<&str>,
) -> Option<AnthropicThinking> {
    if is_thinking_unsupported_model(model) {
        return None;
    }

    if let Some(config) = thinking {
        return match config.kind {
            crate::openai_types::ThinkingType::Enabled => Some(AnthropicThinking::Enabled {
                budget_tokens: 2048,
            }),
            crate::openai_types::ThinkingType::Adaptive => Some(AnthropicThinking::Adaptive),
            crate::openai_types::ThinkingType::Disabled => Some(AnthropicThinking::Disabled),
        };
    }

    if reasoning_effort.is_some() {
        return Some(AnthropicThinking::Adaptive);
    }

    None
}

fn is_thinking_unsupported_model(model: &str) -> bool {
    model.to_ascii_lowercase().contains("claude-haiku")
}

fn extract_text_from_content(content: &Content) -> String {
    match content {
        Content::Text(text) => text.clone(),
        Content::Parts(parts) => parts
            .iter()
            .filter_map(|part| match part {
                ContentPart::Text { text } => Some(text.as_str()),
                _ => None,
            })
            .collect::<Vec<_>>()
            .join(""),
    }
}

fn map_finish_reason(reason: Option<&str>) -> FinishReason {
    match reason.unwrap_or("end_turn") {
        "end_turn" | "stop_sequence" => FinishReason::Stop,
        "max_tokens" => FinishReason::Length,
        "tool_use" => FinishReason::ToolCalls,
        "refusal" => FinishReason::ContentFilter,
        _ => FinishReason::Other,
    }
}

pub(super) fn map_stop_reason_string(reason: &str) -> &'static str {
    match reason {
        "end_turn" | "stop_sequence" => "stop",
        "max_tokens" => "length",
        "tool_use" => "tool_calls",
        "refusal" => "content_filter",
        _ => "other",
    }
}

fn uuid_suffix(name: &str) -> String {
    let trimmed = name.trim();
    if trimmed.is_empty() {
        return "call".to_string();
    }
    trimmed
        .chars()
        .filter(|ch| ch.is_ascii_alphanumeric() || *ch == '_')
        .take(32)
        .collect::<String>()
}
