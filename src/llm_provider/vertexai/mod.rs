use std::pin::Pin;
use std::time::{SystemTime, UNIX_EPOCH};

pub mod client;
pub mod types;

use async_stream::try_stream;
use async_trait::async_trait;
use base64::Engine as _;
use base64::engine::general_purpose::STANDARD as BASE64_STANDARD;
use futures_core::Stream;
use futures_util::StreamExt;
use reqwest::StatusCode;
use serde_json::Value;

use self::client::VertexAIClient;
use self::types::{
    VertexCandidate, VertexContent, VertexFunctionCall, VertexFunctionCallingConfig,
    VertexFunctionDeclaration, VertexGenerateContentRequest, VertexGenerateContentResponse,
    VertexGenerationConfig, VertexInlineData, VertexPart, VertexSystemInstruction, VertexTool,
    VertexToolConfig, VertexUsageMetadata,
};
use crate::llm_provider::provider::InputOutputUsage;
use crate::llm_provider::{
    FinishReason, Provider, ProviderError, ToolCall, UnifiedEvent, UnifiedResponse,
};
use crate::openai_http_mapping::validate_openai_request;
use crate::openai_types::{
    ChatCompletionRequest, Content, ContentPart, ResponseFormat, Role, ToolChoice,
};
use crate::serve_config::ConfigError;

const MAX_IMAGE_BYTES: usize = 10 * 1024 * 1024;

#[derive(Clone)]
pub struct VertexAIProvider {
    client: VertexAIClient,
    model_id: String,
}

impl VertexAIProvider {
    pub fn new(
        model_id: String,
        project_id: String,
        location: String,
        credentials_file: String,
    ) -> Result<Self, ConfigError> {
        let client = VertexAIClient::new(project_id, location, credentials_file)
            .map_err(|err| ConfigError::InvalidProvider(err.to_string()))?;
        Ok(Self { client, model_id })
    }
}

#[async_trait]
impl Provider for VertexAIProvider {
    fn model_id(&self) -> &str {
        &self.model_id
    }

    async fn complete(
        &self,
        request: ChatCompletionRequest,
    ) -> Result<UnifiedResponse, ProviderError> {
        validate_request(&request)?;
        let body = build_vertex_request(&request, self.client.http()).await?;
        let response = self
            .client
            .post_json_with_headers(
                &self.model_id,
                serde_json::to_vec(&body)
                    .map_err(|err| ProviderError::Internal(err.to_string()))?,
                false,
                axum::http::HeaderMap::new(),
            )
            .await
            .map_err(|err| ProviderError::Internal(err.to_string()))?;

        let status = response.status();
        let bytes = response
            .bytes()
            .await
            .map_err(|err| ProviderError::Internal(err.to_string()))?;
        if !status.is_success() {
            return Err(map_http_error(status, &bytes));
        }
        let value: VertexGenerateContentResponse = serde_json::from_slice(&bytes)
            .map_err(|err| ProviderError::Internal(format!("parse vertex response: {err}")))?;
        map_vertex_response(value, &self.model_id)
    }

    async fn stream<'a>(
        &'a self,
        request: ChatCompletionRequest,
    ) -> Result<
        Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    > {
        validate_request(&request)?;
        let body = build_vertex_request(&request, self.client.http()).await?;
        let response = self
            .client
            .post_json_with_headers(
                &self.model_id,
                serde_json::to_vec(&body)
                    .map_err(|err| ProviderError::Internal(err.to_string()))?,
                true,
                axum::http::HeaderMap::new(),
            )
            .await
            .map_err(|err| ProviderError::Internal(err.to_string()))?;

        let status = response.status();
        if !status.is_success() {
            let bytes = response
                .bytes()
                .await
                .map_err(|err| ProviderError::Internal(err.to_string()))?;
            return Err(map_http_error(status, &bytes));
        }

        let stream = response.bytes_stream();
        let model_id = self.model_id.clone();
        let output = try_stream! {
            futures_util::pin_mut!(stream);
            let mut state = VertexStreamState::default();
            state.model = model_id;

            let mut buffer = String::new();
            while let Some(item) = stream.next().await {
                let chunk = item.map_err(|err| ProviderError::Internal(err.to_string()))?;
                let text = String::from_utf8_lossy(&chunk);
                buffer.push_str(&text);

                while let Some(pos) = buffer.find('\n') {
                    let line = buffer[..pos].trim().to_string();
                    buffer.drain(..=pos);
                    if line.is_empty() {
                        continue;
                    }
                    if let Some(data) = line
                        .strip_prefix("data: ")
                        .or_else(|| line.strip_prefix("data:"))
                    {
                        if data.trim() == "[DONE]" {
                            if !state.completed {
                                yield UnifiedEvent::Completed {
                                    finish_reason: Some("stop".to_string()),
                                    usage: state.latest_usage.clone(),
                                };
                            }
                            return;
                        }

                        let value: VertexGenerateContentResponse = serde_json::from_str(data)
                            .map_err(|err| ProviderError::Internal(format!("parse vertex stream event: {err}")))?;

                        for event in map_vertex_stream_chunk(&value, &mut state) {
                            yield event;
                        }
                    }
                }
            }

            if !state.completed {
                yield UnifiedEvent::Completed {
                    finish_reason: Some("stop".to_string()),
                    usage: state.latest_usage.clone(),
                };
            }
        };

        Ok(Box::pin(output))
    }
}

#[derive(Default)]
struct VertexStreamState {
    started: bool,
    completed: bool,
    request_id: String,
    model: String,
    created_at: String,
    latest_usage: Option<Value>,
}

fn validate_request(request: &ChatCompletionRequest) -> Result<(), ProviderError> {
    validate_openai_request(request).map_err(ProviderError::Internal)
}

async fn build_vertex_request(
    request: &ChatCompletionRequest,
    http: &reqwest::Client,
) -> Result<VertexGenerateContentRequest, ProviderError> {
    let mut contents = Vec::<VertexContent>::new();
    let mut system_texts = Vec::<String>::new();

    for message in &request.messages {
        match message.role {
            Role::System | Role::Developer => {
                let text = extract_message_text(&message.content)?;
                if !text.trim().is_empty() {
                    system_texts.push(text);
                }
            }
            Role::User | Role::Assistant | Role::Tool => {
                let role = if message.role == Role::Assistant {
                    "model"
                } else {
                    "user"
                }
                .to_string();
                let parts = content_to_vertex_parts(&message.content, http).await?;
                if !parts.is_empty() {
                    contents.push(VertexContent {
                        role: Some(role),
                        parts,
                    });
                }
            }
        }
    }

    if contents.is_empty() {
        return Err(ProviderError::Internal(
            "vertexai request has no content".to_string(),
        ));
    }

    let system_instruction = if system_texts.is_empty() {
        None
    } else {
        Some(VertexSystemInstruction {
            parts: vec![VertexPart {
                text: Some(system_texts.join("\n")),
                ..Default::default()
            }],
        })
    };

    let mut generation_config = VertexGenerationConfig {
        temperature: request.temperature,
        top_p: request.top_p,
        max_output_tokens: request.max_completion_tokens,
        candidate_count: request.n,
        stop_sequences: request.stop.clone().filter(|stops| !stops.is_empty()),
        response_mime_type: None,
        response_schema: None,
    };
    if let Some(format) = &request.response_format {
        match format {
            ResponseFormat::JsonObject => {
                generation_config.response_mime_type = Some("application/json".to_string());
            }
            ResponseFormat::JsonSchema { json_schema } => {
                generation_config.response_mime_type = Some("application/json".to_string());
                generation_config.response_schema = Some(json_schema.schema.clone());
            }
            ResponseFormat::Text => {}
        }
    }
    let generation_config = if generation_config == VertexGenerationConfig::default() {
        None
    } else {
        Some(generation_config)
    };

    let tools = request.tools.as_ref().and_then(|tools| {
        if tools.is_empty() {
            None
        } else {
            Some(vec![VertexTool {
                function_declarations: tools
                    .iter()
                    .map(|tool| VertexFunctionDeclaration {
                        name: tool.function.name.clone(),
                        description: tool.function.description.clone(),
                        parameters: tool.function.parameters.clone(),
                    })
                    .collect::<Vec<_>>(),
            }])
        }
    });

    let tool_config = if let Some(tool_choice) = &request.tool_choice {
        Some(VertexToolConfig {
            function_calling_config: match tool_choice {
                ToolChoice::Name(name) if name == "none" => VertexFunctionCallingConfig {
                    mode: "NONE".to_string(),
                    allowed_function_names: None,
                },
                ToolChoice::Name(name) if name == "auto" => VertexFunctionCallingConfig {
                    mode: "AUTO".to_string(),
                    allowed_function_names: None,
                },
                ToolChoice::Name(name) if name == "required" => VertexFunctionCallingConfig {
                    mode: "ANY".to_string(),
                    allowed_function_names: None,
                },
                ToolChoice::Object { r#type, function } if r#type == "function" => {
                    VertexFunctionCallingConfig {
                        mode: "ANY".to_string(),
                        allowed_function_names: Some(vec![function.name.clone()]),
                    }
                }
                _ => {
                    return Err(ProviderError::Internal(
                        "invalid tool_choice for vertexai".to_string(),
                    ));
                }
            },
        })
    } else {
        None
    };

    Ok(VertexGenerateContentRequest {
        contents,
        system_instruction,
        generation_config,
        tools,
        tool_config,
    })
}

async fn content_to_vertex_parts(
    content: &Content,
    http: &reqwest::Client,
) -> Result<Vec<VertexPart>, ProviderError> {
    match content {
        Content::Text(text) => Ok(vec![VertexPart {
            text: Some(text.clone()),
            ..Default::default()
        }]),
        Content::Parts(parts) => {
            let mut out = Vec::new();
            for part in parts {
                match part {
                    ContentPart::Text { text } => out.push(VertexPart {
                        text: Some(text.clone()),
                        ..Default::default()
                    }),
                    ContentPart::Image { image_url } => {
                        let (mime_type, data) = image_to_inline_data(&image_url.url, http).await?;
                        out.push(VertexPart {
                            inline_data: Some(VertexInlineData { mime_type, data }),
                            ..Default::default()
                        });
                    }
                    ContentPart::InputAudio { .. } | ContentPart::File { .. } => {
                        return Err(ProviderError::Internal(
                            "vertexai provider does not support input_audio/file yet".to_string(),
                        ));
                    }
                }
            }
            Ok(out)
        }
    }
}

async fn image_to_inline_data(
    url: &str,
    http: &reqwest::Client,
) -> Result<(String, String), ProviderError> {
    if let Some((mime_type, data)) = parse_data_url(url) {
        return Ok((mime_type, data));
    }

    if url.starts_with("http://") || url.starts_with("https://") {
        let response = http
            .get(url)
            .send()
            .await
            .map_err(|err| ProviderError::Internal(format!("download image failed: {err}")))?;
        if !response.status().is_success() {
            return Err(ProviderError::Internal(format!(
                "download image failed with status {}",
                response.status()
            )));
        }

        let mime_type = response
            .headers()
            .get(reqwest::header::CONTENT_TYPE)
            .and_then(|v| v.to_str().ok())
            .unwrap_or("application/octet-stream")
            .to_string();
        if !mime_type.starts_with("image/") {
            return Err(ProviderError::Internal(
                "image_url content-type is not image/*".to_string(),
            ));
        }

        let bytes = response
            .bytes()
            .await
            .map_err(|err| ProviderError::Internal(format!("read image body failed: {err}")))?;
        if bytes.len() > MAX_IMAGE_BYTES {
            return Err(ProviderError::Internal(format!(
                "image is too large (>{MAX_IMAGE_BYTES} bytes)"
            )));
        }

        return Ok((mime_type, BASE64_STANDARD.encode(bytes)));
    }

    Err(ProviderError::Internal(
        "vertexai provider supports image_url as data URL or http(s) URL".to_string(),
    ))
}

fn extract_message_text(content: &Content) -> Result<String, ProviderError> {
    match content {
        Content::Text(text) => Ok(text.clone()),
        Content::Parts(parts) => {
            let mut buf = String::new();
            for part in parts {
                match part {
                    ContentPart::Text { text } => {
                        if !buf.is_empty() {
                            buf.push('\n');
                        }
                        buf.push_str(text);
                    }
                    _ => {
                        return Err(ProviderError::Internal(
                            "system/developer message only supports text".to_string(),
                        ));
                    }
                }
            }
            Ok(buf)
        }
    }
}

fn parse_data_url(url: &str) -> Option<(String, String)> {
    let trimmed = url.trim();
    if !trimmed.starts_with("data:") {
        return None;
    }
    let (meta, data) = trimmed.split_once(',')?;
    let meta = meta.strip_prefix("data:")?;
    let (mime, encoding) = meta.split_once(';')?;
    if encoding.eq_ignore_ascii_case("base64") {
        Some((mime.to_string(), data.to_string()))
    } else {
        None
    }
}

fn map_vertex_response(
    value: VertexGenerateContentResponse,
    default_model: &str,
) -> Result<UnifiedResponse, ProviderError> {
    let request_id = value.response_id.unwrap_or_else(new_response_id);
    let model = value
        .model_version
        .unwrap_or_else(|| default_model.to_string());

    let candidate = value
        .candidates
        .as_ref()
        .and_then(|arr| arr.first())
        .cloned()
        .unwrap_or_default();

    let output_text = extract_text_from_candidate(&candidate);
    let tool_calls = extract_tool_calls_from_candidate(&candidate);
    let finish_reason = map_finish_reason(
        candidate.finish_reason.as_deref().unwrap_or("STOP"),
        tool_calls.as_ref(),
    );
    let usage = map_usage_from_usage_metadata(value.usage_metadata.as_ref());

    Ok(UnifiedResponse {
        request_id,
        created_at: chrono::Utc::now().to_rfc3339(),
        model,
        output_text,
        tool_calls,
        finish_reason,
        usage: serde_json::to_value(usage).unwrap_or(Value::Null),
    })
}

fn map_vertex_stream_chunk(
    value: &VertexGenerateContentResponse,
    state: &mut VertexStreamState,
) -> Vec<UnifiedEvent> {
    let mut events = Vec::new();

    if !state.started {
        state.started = true;
        state.request_id = value.response_id.clone().unwrap_or_else(new_response_id);
        if let Some(model) = &value.model_version {
            state.model = model.clone();
        }
        state.created_at = chrono::Utc::now().to_rfc3339();
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

    let candidate = value
        .candidates
        .as_ref()
        .and_then(|arr| arr.first())
        .cloned()
        .unwrap_or_default();

    let delta = extract_text_from_candidate(&candidate);
    if !delta.is_empty() {
        events.push(UnifiedEvent::MessageDelta {
            id: state.request_id.clone(),
            delta,
        });
    }

    if let Some(calls) = extract_tool_calls_from_candidate(&candidate) {
        for (idx, call) in calls.into_iter().enumerate() {
            let tool_id = call
                .id
                .clone()
                .unwrap_or_else(|| format!("{}-tool-{}", state.request_id, idx));
            events.push(UnifiedEvent::ToolCallDelta {
                id: tool_id.clone(),
                name: call.name.clone(),
                arguments_delta: call.arguments.clone(),
            });
            events.push(UnifiedEvent::ToolCallDone {
                id: tool_id,
                name: call.name,
                arguments: call.arguments,
            });
        }
    }

    let usage = map_usage_from_usage_metadata(value.usage_metadata.as_ref());
    if usage.input_tokens > 0 || usage.output_tokens > 0 || usage.total_tokens > 0 {
        let usage = serde_json::to_value(&usage).unwrap_or(Value::Null);
        state.latest_usage = Some(usage.clone());
        events.push(UnifiedEvent::Usage { usage });
    }

    if let Some(reason) = candidate.finish_reason.as_deref() {
        let finish_reason = map_finish_reason_string(reason, candidate.content.as_ref());
        events.push(UnifiedEvent::MessageStop {
            id: state.request_id.clone(),
            stop_reason: Some(finish_reason.clone()),
        });
        events.push(UnifiedEvent::Completed {
            finish_reason: Some(finish_reason),
            usage: state.latest_usage.clone(),
        });
        state.completed = true;
    }

    events
}

fn map_usage_from_usage_metadata(usage: Option<&VertexUsageMetadata>) -> InputOutputUsage {
    let input_tokens = usage
        .and_then(|value| value.prompt_token_count)
        .unwrap_or(0);
    let output_tokens = usage
        .and_then(|value| value.candidates_token_count)
        .unwrap_or(0);
    let total_tokens = usage
        .and_then(|value| value.total_token_count)
        .unwrap_or(input_tokens + output_tokens);

    InputOutputUsage {
        input_tokens: i64::from(input_tokens),
        output_tokens: i64::from(output_tokens),
        total_tokens: i64::from(total_tokens),
        cached_tokens: None,
        reasoning_tokens: None,
    }
}

fn extract_text_from_candidate(candidate: &VertexCandidate) -> String {
    let Some(parts) = candidate
        .content
        .as_ref()
        .map(|value| value.parts.as_slice())
    else {
        return String::new();
    };

    parts
        .iter()
        .filter_map(|part| part.text.as_deref())
        .collect::<String>()
}

fn extract_tool_calls_from_candidate(candidate: &VertexCandidate) -> Option<Vec<ToolCall>> {
    let parts = candidate
        .content
        .as_ref()
        .map(|value| value.parts.as_slice())?;

    let calls = parts
        .iter()
        .filter_map(|call| {
            let VertexFunctionCall { name, args } = call.function_call.clone()?;
            Some(ToolCall {
                id: None,
                name,
                description: String::new(),
                arguments: if args.is_null() {
                    "{}".to_string()
                } else {
                    args.to_string()
                },
            })
        })
        .collect::<Vec<_>>();

    if calls.is_empty() { None } else { Some(calls) }
}

fn map_finish_reason(reason: &str, tool_calls: Option<&Vec<ToolCall>>) -> FinishReason {
    if tool_calls.is_some() {
        return FinishReason::ToolCalls;
    }
    match reason {
        "STOP" => FinishReason::Stop,
        "MAX_TOKENS" => FinishReason::Length,
        "SAFETY" | "BLOCKLIST" | "PROHIBITED_CONTENT" => FinishReason::ContentFilter,
        _ => FinishReason::Other,
    }
}

fn map_finish_reason_string(reason: &str, content: Option<&VertexContent>) -> String {
    let has_tool_call = content
        .map(|value| value.parts.iter().any(|part| part.function_call.is_some()))
        .unwrap_or(false);
    if has_tool_call {
        return "tool_calls".to_string();
    }
    match reason {
        "STOP" => "stop",
        "MAX_TOKENS" => "length",
        "SAFETY" | "BLOCKLIST" | "PROHIBITED_CONTENT" => "content_filter",
        _ => "other",
    }
    .to_string()
}

fn map_http_error(status: StatusCode, body: &[u8]) -> ProviderError {
    let text = String::from_utf8_lossy(body).to_string();
    ProviderError::Internal(format!(
        "vertexai request failed with status {status}: {text}"
    ))
}

fn new_response_id() -> String {
    let ts = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis())
        .unwrap_or(0);
    format!("vertexai-{ts}")
}

pub fn build_vertexai_provider(
    params: &std::collections::HashMap<String, String>,
) -> Result<VertexAIProvider, ConfigError> {
    let model = params
        .get("model")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("model is required".to_string()))?;
    let project_id = params
        .get("project_id")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("project_id is required".to_string()))?;
    let location = params
        .get("location")
        .cloned()
        .unwrap_or_else(|| "global".to_string());
    let credentials_file = params
        .get("credentials_file")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("credentials_file is required".to_string()))?;

    VertexAIProvider::new(model, project_id, location, credentials_file)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_data_url_supports_base64() {
        let raw = "data:image/png;base64,aGVsbG8=";
        let parsed = parse_data_url(raw);
        assert!(parsed.is_some());
        let (mime, data) = parsed.expect("must parse data url");
        assert_eq!(mime, "image/png");
        assert_eq!(data, "aGVsbG8=");
    }

    #[test]
    fn parse_data_url_rejects_non_base64() {
        let raw = "data:image/png;utf8,hello";
        assert!(parse_data_url(raw).is_none());
    }

    #[test]
    fn map_usage_from_usage_metadata_works() {
        let usage = VertexUsageMetadata {
            prompt_token_count: Some(12),
            candidates_token_count: Some(8),
            total_token_count: Some(20),
        };
        let mapped = map_usage_from_usage_metadata(Some(&usage));
        assert_eq!(mapped.input_tokens, 12);
        assert_eq!(mapped.output_tokens, 8);
        assert_eq!(mapped.total_tokens, 20);
    }
}
