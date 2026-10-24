use std::collections::{HashMap, VecDeque};
use std::sync::{Arc, Mutex};
use std::time::{SystemTime, UNIX_EPOCH};

use axum::http::HeaderMap;
use base64::Engine as _;
use base64::engine::general_purpose::STANDARD as BASE64_STANDARD;
use reqwest::StatusCode;
use serde_json::Value;

use super::types::{
    VertexCandidate, VertexContent, VertexFunctionCall, VertexFunctionCallingConfig,
    VertexFunctionDeclaration, VertexFunctionResponse, VertexGenerateContentRequest,
    VertexGenerateContentResponse, VertexGenerationConfig, VertexInlineData, VertexPart,
    VertexSystemInstruction, VertexThinkingConfig, VertexThinkingLevel, VertexTool,
    VertexToolConfig, VertexUsageMetadata,
};
use crate::openai_types::ErrorDetail;
#[cfg(test)]
use crate::openai_types::{
    ChatCompletionRequest, Content, ContentPart, ResponseFormat, Role, ToolChoice,
};
use crate::provider::{FinishReason, ProviderError, ToolCall, UnifiedEvent, UnifiedResponse};
use crate::provider::{
    UnifiedContentDelta, UnifiedEnding, UnifiedEventKind, UnifiedEventTarget, UnifiedOutputItem,
    UnifiedOutputItemKind, UnifiedPartLocation, UnifiedUsageUpdate,
};
use crate::provider::{
    UnifiedContentPart, UnifiedContentPartKind, UnifiedInputItemKind, UnifiedMediaSource,
    UnifiedRequest, UnifiedRole, UnifiedToolChoice, UnifiedToolKind, UnifiedToolOutput,
};
use crate::usage_handler::EndpointUsage;
use crate::usage_types::GenerateContentUsage;
use crate::utils::ensure_tool_call_id;

const MAX_IMAGE_BYTES: usize = 10 * 1024 * 1024;
// Gemini inline data limit for non-image file parts (e.g. PDF).
const MAX_FILE_BYTES: usize = 20 * 1024 * 1024;
const GEMINI_MAX_OUTPUT_TOKENS_65K: i32 = 65_536;
const GEMINI_MAX_OUTPUT_TOKENS_32K: i32 = 32_768;

#[derive(Default)]
pub(crate) struct ThoughtSignatureStore {
    max_entries: usize,
    by_tool_call_id: HashMap<String, String>,
    insertion_order: VecDeque<String>,
}

impl ThoughtSignatureStore {
    pub(crate) fn new(max_entries: usize) -> Self {
        Self {
            max_entries,
            by_tool_call_id: HashMap::new(),
            insertion_order: VecDeque::new(),
        }
    }

    fn insert(&mut self, tool_call_id: String, signature: String) {
        if self.by_tool_call_id.contains_key(&tool_call_id) {
            self.by_tool_call_id.insert(tool_call_id, signature);
            return;
        }

        self.by_tool_call_id.insert(tool_call_id.clone(), signature);
        self.insertion_order.push_back(tool_call_id);
        while self.by_tool_call_id.len() > self.max_entries {
            if let Some(oldest) = self.insertion_order.pop_front() {
                self.by_tool_call_id.remove(&oldest);
            }
        }
    }

    fn get(&self, tool_call_id: &str) -> Option<String> {
        self.by_tool_call_id.get(tool_call_id).cloned()
    }
}

#[derive(Default)]
pub(crate) struct VertexStreamState {
    pub(crate) started: bool,
    pub(crate) completed: bool,
    saw_tool_call: bool,
    pub(crate) request_id: String,
    pub(crate) model: String,
    pub(crate) created_at: String,
}

/// Encode Unified IR directly, keeping ordered text/function parts and native
/// thought signatures intact across server-tool rounds.
pub(crate) async fn build_unified_vertex_request(
    request: &UnifiedRequest,
    http: &reqwest::Client,
    signatures: &Arc<Mutex<ThoughtSignatureStore>>,
) -> Result<VertexGenerateContentRequest, ProviderError> {
    let invalid = |message: &str| ProviderError::Public {
        status: StatusCode::BAD_REQUEST,
        error: ErrorDetail {
            message: message.into(),
            r#type: "invalid_request_error".into(),
            code: Some("unsupported_conversion".into()),
            param: None,
        },
    };
    let thinking_config = map_vertex_thinking_config(request)?;
    let mut contents: Vec<VertexContent> = Vec::new();
    let mut system_parts = Vec::new();
    let mut tool_names = HashMap::new();
    let mut current_boundary: Option<String> = None;
    for item in request.instructions.iter().flatten().chain(&request.input) {
        let boundary = item
            .message
            .as_ref()
            .map(|message| message.message_id.clone());
        let (role, parts) = match &item.kind {
            UnifiedInputItemKind::Message {
                role: UnifiedRole::System | UnifiedRole::Developer,
                content,
                ..
            } => {
                for part in content {
                    let UnifiedContentPartKind::Text { text, .. } = &part.kind else {
                        return Err(invalid("system instruction requires text"));
                    };
                    system_parts.push(VertexPart {
                        text: Some(text.clone()),
                        ..Default::default()
                    });
                }
                continue;
            }
            UnifiedInputItemKind::Message { role, content, .. } => {
                let role = match role {
                    UnifiedRole::User => "user",
                    UnifiedRole::Assistant => "model",
                    _ => return Err(invalid("tool result requires a function output item")),
                };
                let mut parts = Vec::new();
                for part in content {
                    parts.push(unified_vertex_part(part, http).await?);
                }
                (role, parts)
            }
            UnifiedInputItemKind::FunctionCall {
                call_id,
                name,
                arguments,
                ..
            } => {
                let args = serde_json::from_str(arguments)
                    .map_err(|error| invalid(&format!("invalid function arguments: {error}")))?;
                tool_names.insert(call_id.clone(), name.clone());
                let thought_signature = item
                    .source_extensions
                    .as_ref()
                    .and_then(|source| source.fields.get("thoughtSignature"))
                    .and_then(Value::as_str)
                    .map(str::to_owned)
                    .or_else(|| signatures.lock().ok().and_then(|store| store.get(call_id)));
                (
                    "model",
                    vec![VertexPart {
                        function_call: Some(VertexFunctionCall {
                            name: name.clone(),
                            args,
                        }),
                        thought_signature,
                        ..Default::default()
                    }],
                )
            }
            UnifiedInputItemKind::FunctionCallOutput {
                call_id, output, ..
            } => {
                let name = tool_names
                    .get(call_id)
                    .cloned()
                    .ok_or_else(|| invalid("tool output has no matching function call"))?;
                let response = match output {
                    UnifiedToolOutput::Json(value) => {
                        if value.is_object() {
                            value.clone()
                        } else {
                            serde_json::json!({"result":value})
                        }
                    }
                    UnifiedToolOutput::Text(text) => ensure_vertex_struct(
                        serde_json::from_str(text).unwrap_or_else(|_| Value::String(text.clone())),
                    ),
                    UnifiedToolOutput::Content(parts) => {
                        let mut text = String::new();
                        for part in parts {
                            let UnifiedContentPartKind::Text { text: value, .. } = &part.kind
                            else {
                                return Err(invalid(
                                    "non-text tool result conversion is not established",
                                ));
                            };
                            text.push_str(value);
                        }
                        ensure_vertex_struct(
                            serde_json::from_str(&text).unwrap_or(Value::String(text)),
                        )
                    }
                    UnifiedToolOutput::Missing => return Err(invalid("tool result is required")),
                };
                (
                    "user",
                    vec![VertexPart {
                        function_response: Some(VertexFunctionResponse { name, response }),
                        ..Default::default()
                    }],
                )
            }
            _ => return Err(invalid("input item has no supported representation")),
        };
        let same_message = boundary.is_some()
            && current_boundary == boundary
            && contents
                .last()
                .is_some_and(|content| content.role.as_deref() == Some(role));
        let consecutive_tool_outputs =
            matches!(item.kind, UnifiedInputItemKind::FunctionCallOutput { .. })
                && contents.last().is_some_and(is_function_response_content);
        if same_message || consecutive_tool_outputs {
            if let Some(content) = contents.last_mut() {
                content.parts.extend(parts);
            }
        } else {
            contents.push(VertexContent {
                role: Some(role.into()),
                parts,
            });
        }
        current_boundary = boundary;
    }
    if contents.is_empty() {
        return Err(invalid("request requires content"));
    }
    let mut generation = VertexGenerationConfig {
        temperature: request.options.temperature.map(|value| value as f32),
        top_p: request.options.top_p.map(|value| value as f32),
        max_output_tokens: clamp_gemini_max_output_tokens(
            &request.model,
            request
                .options
                .max_output_tokens
                .map(i32::try_from)
                .transpose()
                .map_err(|_| invalid("output budget exceeds supported integer range"))?,
        ),
        candidate_count: request
            .options
            .n
            .and_then(|value| i32::try_from(value).ok()),
        stop_sequences: request.options.stop_sequences.clone(),
        thinking_config,
        ..Default::default()
    };
    if let Some(format) = &request.options.response_format {
        match format.get("type").and_then(Value::as_str) {
            Some("text") => {}
            Some("json_object") => generation.response_mime_type = Some("application/json".into()),
            Some("json_schema") => {
                let schema = format
                    .get("json_schema")
                    .and_then(|value| value.get("schema"))
                    .ok_or_else(|| invalid("JSON Schema is required"))?;
                generation.response_mime_type = Some("application/json".into());
                generation.response_schema = Some(sanitize_vertex_schema(schema));
            }
            _ => {
                return Err(invalid("output format has no verified representation"));
            }
        }
    }
    let mut declarations = Vec::new();
    for tool in request.tools.iter().flatten() {
        let UnifiedToolKind::Function {
            name,
            description,
            parameters,
            ..
        } = &tool.kind
        else {
            return Err(invalid("only function tools are supported in this adapter"));
        };
        declarations.push(VertexFunctionDeclaration {
            name: name.clone(),
            description: description.clone(),
            parameters: ensure_vertex_object_schema(sanitize_vertex_schema(parameters)),
        });
    }
    let tools = (!declarations.is_empty()).then_some(vec![VertexTool {
        function_declarations: declarations,
    }]);
    let tool_config = match &request.tool_choice {
        None => None,
        Some(choice) => {
            let (mode, allowed_function_names) = match choice {
                UnifiedToolChoice::Auto => ("AUTO", None),
                UnifiedToolChoice::None => ("NONE", None),
                UnifiedToolChoice::Required => ("ANY", None),
                UnifiedToolChoice::Function(name) => ("ANY", Some(vec![name.clone()])),
                _ => return Err(invalid("tool choice has no verified representation")),
            };
            Some(VertexToolConfig {
                function_calling_config: VertexFunctionCallingConfig {
                    mode: mode.into(),
                    allowed_function_names,
                },
            })
        }
    };
    Ok(VertexGenerateContentRequest {
        contents,
        system_instruction: (!system_parts.is_empty()).then_some(VertexSystemInstruction {
            parts: system_parts,
        }),
        generation_config: (generation != VertexGenerationConfig::default()).then_some(generation),
        tools,
        tool_config,
    })
}

async fn unified_vertex_part(
    part: &UnifiedContentPart,
    http: &reqwest::Client,
) -> Result<VertexPart, ProviderError> {
    match &part.kind {
        UnifiedContentPartKind::Text { text, .. } => Ok(VertexPart {
            text: Some(text.clone()),
            ..Default::default()
        }),
        UnifiedContentPartKind::Thinking { text, signature } => Ok(VertexPart {
            text: Some(text.clone()),
            thought: Some(true),
            thought_signature: signature.clone(),
            ..Default::default()
        }),
        UnifiedContentPartKind::Image { source, .. }
        | UnifiedContentPartKind::File { source, .. } => {
            let (mime_type, data) = match source {
                UnifiedMediaSource::Base64 { data, media_type } => {
                    if matches!(part.kind, UnifiedContentPartKind::Image { .. }) {
                        image_to_inline_data(&format!("data:{media_type};base64,{data}"), http)
                            .await?
                    } else {
                        file_to_inline_data(data, Some(media_type), None)?
                    }
                }
                UnifiedMediaSource::DataUrl { url } => file_to_inline_data(url, None, None)?,
                UnifiedMediaSource::Url { url }
                    if matches!(part.kind, UnifiedContentPartKind::Image { .. }) =>
                {
                    image_to_inline_data(url, http).await?
                }
                _ => {
                    return Err(ProviderError::internal(
                        "media source has no verified representation",
                    ));
                }
            };
            Ok(VertexPart {
                inline_data: Some(VertexInlineData { mime_type, data }),
                ..Default::default()
            })
        }
        _ => Err(ProviderError::internal(
            "content part has no verified representation",
        )),
    }
}

#[cfg(test)]
pub(crate) async fn build_vertex_request(
    request: &ChatCompletionRequest,
    http: &reqwest::Client,
    thought_signatures: &Arc<Mutex<ThoughtSignatureStore>>,
) -> Result<VertexGenerateContentRequest, ProviderError> {
    let mut contents = Vec::<VertexContent>::new();
    let mut system_texts = Vec::<String>::new();
    let mut tool_name_by_id = HashMap::<String, String>::new();

    for message in &request.messages {
        match message.role {
            Role::System | Role::Developer => {
                let text = extract_message_text(&message.content)?;
                if !text.trim().is_empty() {
                    system_texts.push(text);
                }
            }
            Role::User => {
                let parts = content_to_vertex_parts(&message.content, http).await?;
                if !parts.is_empty() {
                    contents.push(VertexContent {
                        role: Some("user".to_string()),
                        parts,
                    });
                }
            }
            Role::Assistant => {
                if let Some(tool_calls) = &message.tool_calls {
                    let mut parts = Vec::new();
                    for call in tool_calls {
                        let args = parse_function_call_args(&call.function.arguments);
                        let tool_call_id = call.id.clone().unwrap_or_default();
                        let thought_signature = if tool_call_id.is_empty() {
                            None
                        } else {
                            thought_signatures
                                .lock()
                                .ok()
                                .and_then(|store| store.get(&tool_call_id))
                        };
                        if !tool_call_id.is_empty() {
                            tool_name_by_id.insert(tool_call_id, call.function.name.clone());
                        }
                        parts.push(VertexPart {
                            function_call: Some(VertexFunctionCall {
                                name: call.function.name.clone(),
                                args,
                            }),
                            thought_signature,
                            ..Default::default()
                        });
                    }
                    if !parts.is_empty() {
                        contents.push(VertexContent {
                            role: Some("model".to_string()),
                            parts,
                        });
                    }
                } else {
                    let parts = content_to_vertex_parts(&message.content, http).await?;
                    if !parts.is_empty() {
                        contents.push(VertexContent {
                            role: Some("model".to_string()),
                            parts,
                        });
                    }
                }
            }
            Role::Tool => {
                let tool_call_id = message.tool_call_id.as_deref().unwrap_or("").trim();
                if tool_call_id.is_empty() {
                    continue;
                }
                let tool_name = tool_name_by_id
                    .get(tool_call_id)
                    .cloned()
                    .unwrap_or_else(|| "unknown_tool".to_string());
                let response = parse_tool_response_payload(&message.content);
                let part = VertexPart {
                    function_response: Some(VertexFunctionResponse {
                        name: tool_name,
                        response,
                    }),
                    ..Default::default()
                };
                // Gemini requires all responses to a parallel function call turn
                // in a single content, so merge consecutive tool messages.
                match contents.last_mut() {
                    Some(last) if is_function_response_content(last) => last.parts.push(part),
                    _ => contents.push(VertexContent {
                        role: Some("user".to_string()),
                        parts: vec![part],
                    }),
                }
            }
        }
    }

    if contents.is_empty() {
        return Err(ProviderError::internal(
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
        // The legacy Chat-shaped encoder only feeds fixture tests; the real
        // thinking encoding lives in the unified path.
        thinking_config: None,
    };
    if let Some(format) = &request.response_format {
        match format {
            ResponseFormat::JsonObject => {
                generation_config.response_mime_type = Some("application/json".to_string());
            }
            ResponseFormat::JsonSchema { json_schema } => {
                generation_config.response_mime_type = Some("application/json".to_string());
                generation_config.response_schema =
                    Some(sanitize_vertex_schema(&json_schema.schema));
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
                        parameters: ensure_vertex_object_schema(sanitize_vertex_schema(
                            &tool.function.parameters,
                        )),
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
                    return Err(ProviderError::internal(
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

#[cfg(test)]
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
                    ContentPart::File {
                        file,
                        file_url,
                        file_id,
                        file_data,
                        filename,
                        mime_type,
                        ..
                    } => {
                        if file_url.is_some()
                            || file_id.is_some()
                            || file.file_url.is_some()
                            || file.file_id.is_some()
                        {
                            return Err(ProviderError::internal(
                                "model does not support file_url/file_id, only file_data is supported"
                                    .to_string(),
                            ));
                        }
                        // Fields may appear top-level or nested inside `file`;
                        // prefer the nested variant.
                        let file_data = file
                            .file_data
                            .as_deref()
                            .or(file_data.as_deref())
                            .unwrap_or_default();
                        let mime_type = file.mime_type.as_deref().or(mime_type.as_deref());
                        let filename = file.filename.as_deref().or(filename.as_deref());
                        let (mime_type, data) =
                            file_to_inline_data(file_data, mime_type, filename)?;
                        out.push(VertexPart {
                            inline_data: Some(VertexInlineData { mime_type, data }),
                            ..Default::default()
                        });
                    }
                    ContentPart::InputAudio { .. } => {
                        return Err(ProviderError::internal(
                            "model does not support input_audio yet".to_string(),
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
            .map_err(|err| ProviderError::internal(format!("download image failed: {err}")))?;
        if !response.status().is_success() {
            return Err(ProviderError::internal(format!(
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
            return Err(ProviderError::internal(
                "image_url content-type is not image/*".to_string(),
            ));
        }

        let bytes = response
            .bytes()
            .await
            .map_err(|err| ProviderError::internal(format!("read image body failed: {err}")))?;
        if bytes.len() > MAX_IMAGE_BYTES {
            return Err(ProviderError::internal(format!(
                "image is too large (>{MAX_IMAGE_BYTES} bytes)"
            )));
        }

        return Ok((mime_type, BASE64_STANDARD.encode(bytes)));
    }

    Err(ProviderError::internal(
        "model supports image_url as data URL or http(s) URL".to_string(),
    ))
}

/// Resolves the Gemini `inline_data` fields for a file content part.
///
/// The MIME type is resolved with the following priority:
/// 1. the explicit `mime_type` field,
/// 2. the media type embedded in a `data:` URL `file_data`,
/// 3. the extension of `filename`.
fn file_to_inline_data(
    file_data: &str,
    mime_type: Option<&str>,
    filename: Option<&str>,
) -> Result<(String, String), ProviderError> {
    let file_data = file_data.trim();
    if file_data.is_empty() {
        return Err(ProviderError::internal(
            "model requires file_data (base64) for file parts".to_string(),
        ));
    }
    let (mime_type, data) = match parse_data_url(file_data) {
        Some((data_url_mime, data)) => {
            let mime_type = mime_type
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(str::to_string)
                .unwrap_or(data_url_mime);
            (mime_type, data)
        }
        None => {
            let mime_type = infer_file_mime_type(mime_type, filename).ok_or_else(|| {
                ProviderError::internal(
                    "model requires mime_type for file parts; set the mime_type field, send \
                     file_data as a data: URL (e.g. data:application/pdf;base64,...), or provide \
                     a filename to infer the type from"
                        .to_string(),
                )
            })?;
            (mime_type, file_data.to_string())
        }
    };
    let bytes = BASE64_STANDARD
        .decode(data.trim())
        .map_err(|err| ProviderError::internal(format!("invalid base64 file_data: {err}")))?;
    if bytes.len() > MAX_FILE_BYTES {
        return Err(ProviderError::internal(format!(
            "file is too large (>{MAX_FILE_BYTES} bytes)"
        )));
    }
    Ok((mime_type, data.trim().to_string()))
}

/// Resolves the MIME type for a file part, preferring the explicit
/// `mime_type` field over the `filename` extension.
fn infer_file_mime_type(mime_type: Option<&str>, filename: Option<&str>) -> Option<String> {
    if let Some(mime_type) = mime_type.map(str::trim).filter(|value| !value.is_empty()) {
        return Some(mime_type.to_string());
    }
    let ext = filename?.trim().rsplit_once('.')?.1;
    extension_mime_type(ext).map(str::to_string)
}

/// Maps a lowercase file extension to its MIME type.
fn extension_mime_type(ext: &str) -> Option<&'static str> {
    match ext.to_ascii_lowercase().as_str() {
        "pdf" => Some("application/pdf"),
        "png" => Some("image/png"),
        "jpg" | "jpeg" => Some("image/jpeg"),
        "gif" => Some("image/gif"),
        "webp" => Some("image/webp"),
        "heic" | "heif" => Some("image/heic"),
        "mp3" => Some("audio/mpeg"),
        "wav" => Some("audio/wav"),
        "ogg" => Some("audio/ogg"),
        "aiff" | "aif" => Some("audio/aiff"),
        "mp4" => Some("video/mp4"),
        "mpeg" | "mpg" => Some("video/mpeg"),
        "webm" => Some("video/webm"),
        "mov" => Some("video/quicktime"),
        "txt" => Some("text/plain"),
        "html" | "htm" => Some("text/html"),
        "css" => Some("text/css"),
        "csv" => Some("text/csv"),
        "md" => Some("text/markdown"),
        "json" => Some("application/json"),
        "xml" => Some("application/xml"),
        "doc" => Some("application/msword"),
        "docx" => Some("application/vnd.openxmlformats-officedocument.wordprocessingml.document"),
        "xls" => Some("application/vnd.ms-excel"),
        "xlsx" => Some("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"),
        "ppt" => Some("application/vnd.ms-powerpoint"),
        "pptx" => Some("application/vnd.openxmlformats-officedocument.presentationml.presentation"),
        _ => None,
    }
}

#[cfg(test)]
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
                        return Err(ProviderError::internal(
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

fn sanitize_vertex_schema(value: &Value) -> Value {
    let mut defs_keys = std::collections::HashSet::new();
    let sanitized = sanitize_vertex_schema_with_defs(value, &mut defs_keys);
    prune_invalid_refs(sanitized, &defs_keys)
}

fn ensure_vertex_object_schema(value: Value) -> Value {
    match value {
        Value::Object(mut map) => {
            let is_object = map
                .get("type")
                .and_then(|value| value.as_str())
                .is_some_and(|value| value == "object");
            if !is_object {
                map.insert("type".to_string(), Value::String("object".to_string()));
            }
            Value::Object(map)
        }
        _ => serde_json::json!({"type": "object", "properties": {}}),
    }
}

fn sanitize_vertex_schema_with_defs(
    value: &Value,
    defs_keys: &mut std::collections::HashSet<String>,
) -> Value {
    match value {
        Value::Object(map) => {
            let sanitized = map
                .iter()
                .filter_map(|(k, v)| {
                    let mapped_key = match k.as_str() {
                        "$ref" => "ref",
                        "$defs" => "defs",
                        // Vertex has no oneOf. Dropping it would leave the property with no
                        // fields at all, while anyOf keeps the variants and matches the same
                        // arguments when each variant pins a distinct enum value.
                        "oneOf" if !map.contains_key("anyOf") => "anyOf",
                        _ => k.as_str(),
                    };
                    if is_vertex_schema_key_allowed(mapped_key) {
                        let sanitized_value = match mapped_key {
                            "properties" => sanitize_vertex_schema_map(v, defs_keys),
                            "defs" => sanitize_vertex_schema_defs(v, defs_keys),
                            "ref" => sanitize_vertex_schema_ref(v),
                            "items" => sanitize_vertex_schema_items(v, defs_keys),
                            "anyOf" => sanitize_vertex_schema_anyof(v, defs_keys),
                            "enum" => sanitize_vertex_schema_enum(v),
                            "required" => sanitize_vertex_schema_required(v),
                            _ => Some(sanitize_vertex_schema_with_defs(v, defs_keys)),
                        };
                        sanitized_value.map(|value| (mapped_key.to_string(), value))
                    } else {
                        None
                    }
                })
                .collect();
            Value::Object(sanitized)
        }
        Value::Array(items) => Value::Array(
            items
                .iter()
                .map(|item| sanitize_vertex_schema_with_defs(item, defs_keys))
                .collect(),
        ),
        _ => value.clone(),
    }
}

fn sanitize_vertex_schema_map(
    value: &Value,
    defs_keys: &mut std::collections::HashSet<String>,
) -> Option<Value> {
    match value {
        Value::Object(map) => Some(Value::Object(
            map.iter()
                .filter(|(k, _)| !k.trim().is_empty())
                .map(|(k, v)| (k.clone(), sanitize_vertex_schema_with_defs(v, defs_keys)))
                .collect(),
        )),
        _ => None,
    }
}

fn sanitize_vertex_schema_defs(
    value: &Value,
    defs_keys: &mut std::collections::HashSet<String>,
) -> Option<Value> {
    let map = value.as_object()?;
    for key in map.keys() {
        defs_keys.insert(key.clone());
    }
    Some(Value::Object(
        map.iter()
            .map(|(k, v)| (k.clone(), sanitize_vertex_schema_with_defs(v, defs_keys)))
            .collect(),
    ))
}

fn sanitize_vertex_schema_items(
    value: &Value,
    defs_keys: &mut std::collections::HashSet<String>,
) -> Option<Value> {
    match value {
        Value::Object(_) => Some(sanitize_vertex_schema_with_defs(value, defs_keys)),
        _ => None,
    }
}

fn sanitize_vertex_schema_anyof(
    value: &Value,
    defs_keys: &mut std::collections::HashSet<String>,
) -> Option<Value> {
    match value {
        Value::Array(items) => Some(Value::Array(
            items
                .iter()
                .map(|item| sanitize_vertex_schema_with_defs(item, defs_keys))
                .collect(),
        )),
        _ => None,
    }
}

fn sanitize_vertex_schema_enum(value: &Value) -> Option<Value> {
    match value {
        Value::Array(_) => Some(value.clone()),
        _ => None,
    }
}

fn sanitize_vertex_schema_required(value: &Value) -> Option<Value> {
    match value {
        Value::Array(items) if items.iter().all(|item| item.is_string()) => Some(value.clone()),
        _ => None,
    }
}

fn sanitize_vertex_schema_ref(value: &Value) -> Option<Value> {
    let raw = value.as_str()?;
    let normalized = normalize_vertex_schema_ref(raw)?;
    Some(Value::String(normalized))
}

fn normalize_vertex_schema_ref(value: &str) -> Option<String> {
    let value = value.trim();
    let rest = value.strip_prefix("#/")?;
    let rest = rest
        .strip_prefix("$defs/")
        .or_else(|| rest.strip_prefix("defs/"))?;
    if rest.is_empty() || rest.contains('/') {
        return None;
    }
    Some(format!("#/defs/{rest}"))
}

fn prune_invalid_refs(value: Value, defs_keys: &std::collections::HashSet<String>) -> Value {
    match value {
        Value::Object(map) => {
            let sanitized = map
                .into_iter()
                .filter_map(|(k, v)| {
                    if k == "ref" {
                        let ref_value = v.as_str()?;
                        let def_key = ref_value.strip_prefix("#/defs/")?;
                        if defs_keys.contains(def_key) {
                            return Some((k, Value::String(ref_value.to_string())));
                        }
                        return None;
                    }

                    Some((k, prune_invalid_refs(v, defs_keys)))
                })
                .collect();
            Value::Object(sanitized)
        }
        Value::Array(items) => Value::Array(
            items
                .into_iter()
                .map(|item| prune_invalid_refs(item, defs_keys))
                .collect(),
        ),
        _ => value,
    }
}

fn is_vertex_schema_key_allowed(key: &str) -> bool {
    matches!(
        key,
        "type"
            | "nullable"
            | "required"
            | "format"
            | "description"
            | "properties"
            | "items"
            | "enum"
            | "anyOf"
            | "ref"
            | "defs"
    )
}

pub(crate) fn map_vertex_response(
    value: VertexGenerateContentResponse,
    default_model: &str,
    thought_signatures: &Arc<Mutex<ThoughtSignatureStore>>,
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
    let reasoning_content = extract_reasoning_from_candidate(&candidate);
    let extracted_tool_calls = extract_tool_calls_from_candidate(&candidate, &request_id);
    cache_thought_signatures(thought_signatures, &extracted_tool_calls);
    let tool_calls = map_extracted_tool_calls(extracted_tool_calls);
    let finish_reason = map_finish_reason(
        candidate.finish_reason.as_deref().unwrap_or("STOP"),
        tool_calls.as_ref(),
    );
    let usage = map_usage_from_usage_metadata(value.usage_metadata.as_ref())
        .expect("vertexai expected generateContent usage metadata");

    Ok(UnifiedResponse::from_completion(
        request_id,
        chrono::Utc::now().to_rfc3339(),
        model,
        output_text,
        reasoning_content,
        tool_calls,
        finish_reason,
        usage,
    ))
}

pub(crate) fn map_vertex_stream_chunk(
    value: &VertexGenerateContentResponse,
    state: &mut VertexStreamState,
    thought_signatures: &Arc<Mutex<ThoughtSignatureStore>>,
) -> Vec<UnifiedEvent> {
    let mut events = Vec::new();

    if !state.started {
        state.started = true;
        state.request_id = value.response_id.clone().unwrap_or_else(new_response_id);
        if let Some(model) = &value.model_version {
            state.model = model.clone();
        }
        state.created_at = chrono::Utc::now().to_rfc3339();
        let mut response = UnifiedResponse::new(state.request_id.clone(), state.model.clone());
        response.created_at = Some(chrono::Utc::now().timestamp());
        events.push(UnifiedEvent::new(UnifiedEventKind::ResponseCreated {
            response: response.clone(),
        }));
        events.push(UnifiedEvent::new(UnifiedEventKind::ResponseInProgress {
            response: Some(response),
        }));
    }

    let candidate = value
        .candidates
        .as_ref()
        .and_then(|arr| arr.first())
        .cloned()
        .unwrap_or_default();

    let delta = extract_text_from_candidate(&candidate);
    if !delta.is_empty() {
        events.push(
            UnifiedEvent::new(UnifiedEventKind::ContentPartDelta {
                delta: UnifiedContentDelta::Text(delta),
            })
            .located(
                0,
                format!("{}-message", state.request_id),
                0,
                Some(UnifiedPartLocation::Content(0)),
            ),
        );
    }

    let thinking_delta = extract_reasoning_from_candidate(&candidate);
    if let Some(thinking_delta) = thinking_delta {
        events.push(
            UnifiedEvent::new(UnifiedEventKind::ContentPartDelta {
                delta: UnifiedContentDelta::Thinking(thinking_delta),
            })
            .located(
                0,
                format!("{}-message", state.request_id),
                0,
                Some(UnifiedPartLocation::Content(1)),
            ),
        );
    }

    let calls = extract_tool_calls_from_candidate(&candidate, &state.request_id);
    cache_thought_signatures(thought_signatures, &calls);
    if let Some(calls) = map_extracted_tool_calls(calls) {
        state.saw_tool_call = true;
        for (idx, call) in calls.into_iter().enumerate() {
            let tool_id = call
                .id
                .clone()
                .unwrap_or_else(|| format!("{}-tool-{}", state.request_id, idx));
            let item = UnifiedOutputItem::new(
                tool_id.clone(),
                UnifiedOutputItemKind::FunctionCall {
                    call_id: tool_id.clone(),
                    name: call.name,
                    arguments: call.arguments,
                    caller: None,
                },
            );
            events.push(
                UnifiedEvent::new(UnifiedEventKind::OutputItemAdded { item: item.clone() })
                    .located(0, tool_id.clone(), idx as u32 + 1, None),
            );
            events.push(
                UnifiedEvent::new(UnifiedEventKind::OutputItemDone { item }).located(
                    0,
                    tool_id,
                    idx as u32 + 1,
                    None,
                ),
            );
        }
    }

    if vertex_usage_metadata_has_usage(value.usage_metadata.as_ref()) {
        if let Some(usage) = map_usage_from_usage_metadata(value.usage_metadata.as_ref()) {
            events.push(UnifiedEvent::new(UnifiedEventKind::Usage {
                usage,
                update: UnifiedUsageUpdate::Snapshot,
                source_usage: None,
            }));
        }
    }

    if let Some(reason) = candidate.finish_reason.as_deref() {
        // Gemini sends functionCall parts and finishReason in separate chunks,
        // so decide from every chunk seen, not only the final one.
        let finish_reason = map_finish_reason_string(reason, state.saw_tool_call);
        let mut event = UnifiedEvent::new(UnifiedEventKind::CandidateDone {
            ending: UnifiedEnding::from_source(&finish_reason),
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
        state.completed = true;
    }

    events
}

fn map_usage_from_usage_metadata(usage: Option<&VertexUsageMetadata>) -> Option<EndpointUsage> {
    let payload = serde_json::to_value(usage?).ok()?;
    let usage = serde_json::from_value::<GenerateContentUsage>(payload).ok()?;
    Some(EndpointUsage::GenerateContent(usage))
}

fn vertex_usage_metadata_has_usage(usage: Option<&VertexUsageMetadata>) -> bool {
    let Some(usage) = usage else {
        return false;
    };

    is_positive_usage_token(usage.total_token_count)
        || is_positive_usage_token(usage.prompt_token_count)
        || is_positive_usage_token(usage.candidates_token_count)
        || is_positive_usage_token(usage.cached_content_token_count)
        || is_positive_usage_token(usage.tool_use_prompt_token_count)
        || is_positive_usage_token(usage.thoughts_token_count)
}

fn is_positive_usage_token(value: Option<i64>) -> bool {
    matches!(value, Some(count) if count > 0)
}

fn extract_text_from_candidate(candidate: &VertexCandidate) -> String {
    extract_text_from_parts(candidate, false).unwrap_or_default()
}

fn extract_reasoning_from_candidate(candidate: &VertexCandidate) -> Option<String> {
    extract_text_from_parts(candidate, true)
}

fn extract_text_from_parts(candidate: &VertexCandidate, thought: bool) -> Option<String> {
    let parts = candidate
        .content
        .as_ref()
        .map(|value| value.parts.as_slice())?;

    let text = parts
        .iter()
        .filter(|part| part.thought.unwrap_or(false) == thought)
        .filter_map(|part| part.text.as_deref())
        .collect::<String>();
    if text.is_empty() { None } else { Some(text) }
}

/// Encode thinking controls using Vertex's native request shape.
fn map_vertex_thinking_config(
    request: &UnifiedRequest,
) -> Result<Option<VertexThinkingConfig>, ProviderError> {
    let effort = request
        .options
        .reasoning_effort
        .as_ref()
        .map(|effort| {
            effort.as_str().ok_or_else(|| ProviderError::Public {
                status: StatusCode::BAD_REQUEST,
                error: ErrorDetail {
                    message: "numeric reasoning_effort is not supported by Google Vertex".into(),
                    r#type: "invalid_request_error".into(),
                    code: Some("unsupported_conversion".into()),
                    param: Some("reasoning_effort".into()),
                },
            })
        })
        .transpose()?;
    let thinking = request.options.thinking.as_ref();
    if effort == Some("none") {
        return Ok(Some(VertexThinkingConfig {
            thinking_budget: Some(0),
            thinking_level: None,
            include_thoughts: None,
        }));
    }
    match (effort, thinking) {
        (None, None) => Ok(None),
        (Some(level), None) => Ok(Some(VertexThinkingConfig {
            thinking_level: Some(vertex_thinking_level(level)?),
            thinking_budget: None,
            include_thoughts: Some(true),
        })),
        (Some(_), Some(_)) => Err(ProviderError::Public {
            status: StatusCode::BAD_REQUEST,
            error: ErrorDetail {
                message: "conflicting reasoning controls reached the encoder".into(),
                r#type: "invalid_request_error".into(),
                code: Some("unsupported_conversion".into()),
                param: Some("reasoning/thinking".into()),
            },
        }),
        (None, Some(thinking)) => {
            let Some(thinking) = thinking.as_object() else {
                return Err(ProviderError::Public {
                    status: StatusCode::BAD_REQUEST,
                    error: ErrorDetail {
                        message: "thinking must be an object".into(),
                        r#type: "invalid_request_error".into(),
                        code: Some("unsupported_conversion".into()),
                        param: Some("thinking".into()),
                    },
                });
            };
            match thinking.get("type").and_then(Value::as_str) {
                Some("disabled") => Ok(Some(VertexThinkingConfig {
                    thinking_budget: Some(0),
                    thinking_level: None,
                    include_thoughts: None,
                })),
                Some("enabled" | "adaptive") => {
                    let budget = thinking
                        .get("budget_tokens")
                        .and_then(Value::as_i64)
                        .and_then(|value| i32::try_from(value).ok());
                    Ok(budget.map(|thinking_budget| VertexThinkingConfig {
                        thinking_budget: Some(thinking_budget),
                        thinking_level: None,
                        include_thoughts: None,
                    }))
                }
                _ => Err(ProviderError::Public {
                    status: StatusCode::BAD_REQUEST,
                    error: ErrorDetail {
                        message: "unknown thinking type reached the encoder".into(),
                        r#type: "invalid_request_error".into(),
                        code: Some("unsupported_conversion".into()),
                        param: Some("thinking.type".into()),
                    },
                }),
            }
        }
    }
}

fn clamp_gemini_max_output_tokens(model: &str, max_output_tokens: Option<i32>) -> Option<i32> {
    let requested = max_output_tokens?;
    let model = model.strip_prefix("google/").unwrap_or(model);
    if !model.starts_with("gemini-") {
        return Some(requested);
    }
    let limit = match model {
        "gemini-3-pro-image" | "gemini-3.1-flash-image" => GEMINI_MAX_OUTPUT_TOKENS_32K,
        _ => GEMINI_MAX_OUTPUT_TOKENS_65K,
    };
    Some(requested.min(limit))
}

/// Map a validated effort level string onto the Vertex enum.
fn vertex_thinking_level(level: &str) -> Result<VertexThinkingLevel, ProviderError> {
    let level = match level {
        "minimal" => VertexThinkingLevel::Minimal,
        "low" => VertexThinkingLevel::Low,
        "medium" => VertexThinkingLevel::Medium,
        "high" => VertexThinkingLevel::High,
        other => {
            return Err(ProviderError::Public {
                status: StatusCode::BAD_REQUEST,
                error: ErrorDetail {
                    message: format!("unknown thinking level reached the encoder: {other}"),
                    r#type: "invalid_request_error".into(),
                    code: Some("unsupported_conversion".into()),
                    param: Some("reasoning_effort".into()),
                },
            });
        }
    };
    Ok(level)
}

fn extract_tool_calls_from_candidate(
    candidate: &VertexCandidate,
    request_id: &str,
) -> Vec<(ToolCall, Option<String>)> {
    let Some(parts) = candidate
        .content
        .as_ref()
        .map(|value| value.parts.as_slice())
    else {
        return Vec::new();
    };

    parts
        .iter()
        .enumerate()
        .filter_map(|(index, call)| {
            let VertexFunctionCall { name, args } = call.function_call.clone()?;
            Some(ToolCall {
                id: Some(ensure_tool_call_id(request_id, index)),
                name,
                description: String::new(),
                arguments: if args.is_null() {
                    "{}".to_string()
                } else {
                    args.to_string()
                },
            })
            .map(|tool_call| (tool_call, call.thought_signature.clone()))
        })
        .collect::<Vec<_>>()
}

fn map_extracted_tool_calls(calls: Vec<(ToolCall, Option<String>)>) -> Option<Vec<ToolCall>> {
    let calls = calls.into_iter().map(|(call, _)| call).collect::<Vec<_>>();
    if calls.is_empty() { None } else { Some(calls) }
}

fn cache_thought_signatures(
    thought_signatures: &Arc<Mutex<ThoughtSignatureStore>>,
    calls: &[(ToolCall, Option<String>)],
) {
    let Ok(mut store) = thought_signatures.lock() else {
        return;
    };
    for (call, signature) in calls {
        let (Some(tool_call_id), Some(signature)) = (call.id.as_ref(), signature.as_ref()) else {
            continue;
        };
        if signature.trim().is_empty() {
            continue;
        }
        store.insert(tool_call_id.clone(), signature.clone());
    }
}

#[cfg(test)]
fn parse_function_call_args(arguments: &str) -> Value {
    serde_json::from_str::<Value>(arguments)
        .unwrap_or_else(|_| Value::String(arguments.to_string()))
}

#[cfg(test)]
fn parse_tool_response_payload(content: &Content) -> Value {
    let payload = match content {
        Content::Text(text) => {
            serde_json::from_str::<Value>(text).unwrap_or_else(|_| Value::String(text.clone()))
        }
        Content::Parts(parts) => Value::String(
            parts
                .iter()
                .filter_map(|part| match part {
                    ContentPart::Text { text } => Some(text.clone()),
                    _ => None,
                })
                .collect::<Vec<_>>()
                .join(""),
        ),
    };

    ensure_vertex_struct(payload)
}

fn is_function_response_content(content: &VertexContent) -> bool {
    content.role.as_deref() == Some("user")
        && !content.parts.is_empty()
        && content
            .parts
            .iter()
            .all(|part| part.function_response.is_some())
}

fn ensure_vertex_struct(value: Value) -> Value {
    match value {
        Value::Object(_) => value,
        _ => serde_json::json!({"content": value}),
    }
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

fn map_finish_reason_string(reason: &str, has_tool_call: bool) -> String {
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

#[cfg(test)]
pub(crate) fn map_http_error(status: StatusCode, body: &[u8]) -> ProviderError {
    map_http_error_with_headers(status, &HeaderMap::new(), body)
}
pub(crate) fn map_http_error_with_headers(
    status: StatusCode,
    headers: &HeaderMap,
    body: &[u8],
) -> ProviderError {
    let error = crate::upstreams::google_vertex::parse_error(status, headers, body);
    ProviderError::Public {
        status: error.status,
        error: ErrorDetail {
            message: error.message,
            r#type: error.error_type.unwrap_or_else(|| "upstream_error".into()),
            code: error.code,
            param: error.param,
        },
    }
}

fn new_response_id() -> String {
    let ts = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis())
        .unwrap_or(0);
    format!("vertexai-{ts}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn unified_vertex_merges_parallel_tool_results_across_message_boundaries() {
        let request = crate::api::request_from_value(
            &serde_json::json!({
                "model": "gemini-3.8-flash",
                "messages": [
                    {"role": "user", "content": "Compare the weather"},
                    {"role": "assistant", "tool_calls": [
                        {"id": "call-a", "type": "function", "function": {
                            "name": "weather", "arguments": "{\"city\":\"Tokyo\"}"
                        }},
                        {"id": "call-b", "type": "function", "function": {
                            "name": "weather", "arguments": "{\"city\":\"Paris\"}"
                        }}
                    ]},
                    {"role": "tool", "tool_call_id": "call-a", "content": "{\"temp\":\"15C\"}"},
                    {"role": "tool", "tool_call_id": "call-b", "content": "{\"temp\":\"12C\"}"},
                    {"role": "user", "content": "Explain the difference"}
                ]
            }),
            crate::provider::UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("request");
        let signatures = Arc::new(Mutex::new(ThoughtSignatureStore::new(16)));
        let body = build_unified_vertex_request(&request, &reqwest::Client::new(), &signatures)
            .await
            .expect("Vertex request");

        assert_eq!(body.contents.len(), 4);
        assert_eq!(body.contents[1].parts.len(), 2);
        assert!(
            body.contents[1]
                .parts
                .iter()
                .all(|part| part.function_call.is_some())
        );
        let results = &body.contents[2];
        assert_eq!(results.role.as_deref(), Some("user"));
        assert_eq!(results.parts.len(), 2);
        for (part, expected) in results.parts.iter().zip(["15C", "12C"]) {
            let result = part.function_response.as_ref().expect("function response");
            assert_eq!(result.name, "weather");
            assert_eq!(result.response, serde_json::json!({"temp": expected}));
        }
        assert_eq!(body.contents[3].parts.len(), 1);
        assert_eq!(
            body.contents[3].parts[0].text.as_deref(),
            Some("Explain the difference")
        );
    }

    #[tokio::test]
    async fn unified_vertex_preserves_interleaved_parts_and_cached_signature() {
        let request = crate::api::request_from_value(&serde_json::json!({
            "model":"gemini-3.5-flash", "max_tokens":8192,
            "messages":[
                {"role":"assistant","content":[
                    {"type":"text","text":"before"},
                    {"type":"tool_use","id":"call-1","name":"lookup","input":{"x":1}},
                    {"type":"text","text":"after"}
                ]},
                {"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":"done"}]}
            ]
        }), crate::provider::UnifiedSourceProtocol::Messages).expect("request");
        let signatures = Arc::new(Mutex::new(ThoughtSignatureStore::new(16)));
        signatures
            .lock()
            .expect("store")
            .insert("call-1".into(), "opaque-signature".into());
        let body = build_unified_vertex_request(&request, &reqwest::Client::new(), &signatures)
            .await
            .expect("Vertex");
        assert_eq!(body.contents.len(), 2);
        assert_eq!(body.contents[0].parts.len(), 3);
        assert_eq!(body.contents[0].parts[0].text.as_deref(), Some("before"));
        assert_eq!(
            body.contents[0].parts[1].thought_signature.as_deref(),
            Some("opaque-signature")
        );
        assert_eq!(body.contents[0].parts[2].text.as_deref(), Some("after"));
        assert_eq!(
            body.contents[1].parts[0]
                .function_response
                .as_ref()
                .expect("result")
                .name,
            "lookup"
        );
        assert_eq!(
            body.generation_config
                .expect("generation")
                .max_output_tokens,
            Some(8192)
        );
    }
    use crate::openai_types::{
        ChatCompletionRequest, Content, FunctionDefinition, Message, Role, ToolCall,
        ToolCallFunction, ToolDefinition,
    };

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
    fn file_to_inline_data_returns_mime_and_base64_data() {
        let data = BASE64_STANDARD.encode("hello pdf");
        let (mime, out) =
            file_to_inline_data(&data, Some("application/pdf"), None).expect("must map file data");
        assert_eq!(mime, "application/pdf");
        assert_eq!(out, data);
    }

    #[test]
    fn file_to_inline_data_rejects_empty_file_data() {
        let err = file_to_inline_data("", Some("application/pdf"), None).expect_err("must fail");
        assert!(err.to_string().contains("requires file_data"));
    }

    #[test]
    fn file_to_inline_data_rejects_missing_mime_type() {
        let data = BASE64_STANDARD.encode("hello");
        let err = file_to_inline_data(&data, None, None).expect_err("must fail");
        assert!(err.to_string().contains("requires mime_type"));
    }

    #[test]
    fn file_to_inline_data_uses_mime_from_data_url() {
        let data = BASE64_STANDARD.encode("hello pdf");
        let raw = format!("data:application/pdf;base64,{data}");
        let (mime, out) = file_to_inline_data(&raw, None, None).expect("must map file data");
        assert_eq!(mime, "application/pdf");
        assert_eq!(out, data);
    }

    #[test]
    fn file_to_inline_data_prefers_explicit_mime_over_data_url() {
        let data = BASE64_STANDARD.encode("hello pdf");
        let raw = format!("data:application/pdf;base64,{data}");
        let (mime, out) =
            file_to_inline_data(&raw, Some("text/plain"), None).expect("must map file data");
        assert_eq!(mime, "text/plain");
        assert_eq!(out, data);
    }

    #[test]
    fn file_to_inline_data_infers_mime_from_filename() {
        let data = BASE64_STANDARD.encode("hello pdf");
        let (mime, out) =
            file_to_inline_data(&data, None, Some("report.PDF")).expect("must map file data");
        assert_eq!(mime, "application/pdf");
        assert_eq!(out, data);
    }

    #[test]
    fn file_to_inline_data_prefers_explicit_mime_over_filename() {
        let data = BASE64_STANDARD.encode("hello");
        let (mime, out) = file_to_inline_data(&data, Some("text/plain"), Some("doc.pdf"))
            .expect("must map file data");
        assert_eq!(mime, "text/plain");
        assert_eq!(out, data);
    }

    #[test]
    fn file_to_inline_data_rejects_unknown_extension() {
        let data = BASE64_STANDARD.encode("hello");
        let err = file_to_inline_data(&data, None, Some("doc.xyz")).expect_err("must fail");
        assert!(err.to_string().contains("requires mime_type"));
    }

    #[test]
    fn file_to_inline_data_rejects_filename_without_extension() {
        let data = BASE64_STANDARD.encode("hello");
        let err = file_to_inline_data(&data, None, Some("noext")).expect_err("must fail");
        assert!(err.to_string().contains("requires mime_type"));
    }

    #[test]
    fn file_to_inline_data_rejects_invalid_base64() {
        let err = file_to_inline_data("not!!base64!!", Some("application/pdf"), None)
            .expect_err("must fail");
        assert!(err.to_string().contains("invalid base64"));
    }

    #[test]
    fn file_to_inline_data_rejects_oversized_file() {
        let oversized = vec![b'a'; MAX_FILE_BYTES + 1];
        let data = BASE64_STANDARD.encode(oversized);
        let err = file_to_inline_data(&data, Some("application/pdf"), None).expect_err("must fail");
        assert!(err.to_string().contains("too large"));
    }

    #[test]
    fn file_to_inline_data_accepts_exactly_at_size_limit() {
        let bytes = vec![b'a'; MAX_FILE_BYTES];
        let data = BASE64_STANDARD.encode(bytes);
        assert!(file_to_inline_data(&data, Some("application/pdf"), None).is_ok());
    }

    #[test]
    fn content_to_vertex_parts_rejects_file_without_file_data() {
        let content = Content::Parts(vec![ContentPart::File {
            file: crate::openai_types::FileContent::default(),
            file_id: None,
            file_data: None,
            filename: None,
            file_url: Some("https://example.com/doc.pdf".to_string()),
            mime_type: Some("application/pdf".to_string()),
        }]);
        let client = reqwest::Client::new();
        let err = tokio::runtime::Runtime::new()
            .expect("runtime")
            .block_on(content_to_vertex_parts(&content, &client))
            .expect_err("must fail");
        assert!(
            err.to_string()
                .contains("does not support file_url/file_id")
        );
    }

    #[test]
    fn map_usage_from_usage_metadata_works() {
        let usage = VertexUsageMetadata {
            prompt_token_count: Some(12),
            candidates_token_count: Some(8),
            total_token_count: Some(20),
            ..Default::default()
        };
        let mapped = map_usage_from_usage_metadata(Some(&usage)).expect("usage must map");
        let EndpointUsage::GenerateContent(mapped) = mapped else {
            panic!("expected generate content usage");
        };
        assert_eq!(mapped.prompt_token_count, Some(12));
        assert_eq!(mapped.candidates_token_count, Some(8));
        assert_eq!(mapped.total_token_count, Some(20));
    }

    #[test]
    fn vertex_usage_metadata_has_usage_checks_all_relevant_fields() {
        assert!(!vertex_usage_metadata_has_usage(None));
        assert!(!vertex_usage_metadata_has_usage(Some(
            &VertexUsageMetadata::default()
        )));

        let with_cached = VertexUsageMetadata {
            cached_content_token_count: Some(1),
            ..Default::default()
        };
        assert!(vertex_usage_metadata_has_usage(Some(&with_cached)));

        let with_total = VertexUsageMetadata {
            total_token_count: Some(3),
            ..Default::default()
        };
        assert!(vertex_usage_metadata_has_usage(Some(&with_total)));
    }

    #[test]
    fn map_vertex_stream_chunk_skips_empty_usage_events() {
        let mut state = VertexStreamState::default();
        state.model = "gemini-2.5-flash".to_string();
        let thought_signatures = Arc::new(Mutex::new(ThoughtSignatureStore::new(16)));

        let chunk = VertexGenerateContentResponse {
            response_id: Some("resp-1".to_string()),
            usage_metadata: Some(VertexUsageMetadata::default()),
            ..Default::default()
        };

        let events = map_vertex_stream_chunk(&chunk, &mut state, &thought_signatures);

        assert!(
            events
                .iter()
                .all(|event| !matches!(event.kind, UnifiedEventKind::Usage { .. }))
        );
    }

    #[test]
    fn map_vertex_stream_chunk_emits_usage_when_tokens_present() {
        let mut state = VertexStreamState::default();
        state.model = "gemini-2.5-flash".to_string();
        let thought_signatures = Arc::new(Mutex::new(ThoughtSignatureStore::new(16)));

        let chunk = VertexGenerateContentResponse {
            response_id: Some("resp-2".to_string()),
            usage_metadata: Some(VertexUsageMetadata {
                total_token_count: Some(9),
                ..Default::default()
            }),
            ..Default::default()
        };

        let events = map_vertex_stream_chunk(&chunk, &mut state, &thought_signatures);

        assert!(
            events
                .iter()
                .any(|event| matches!(event.kind, UnifiedEventKind::Usage { .. }))
        );
    }

    #[test]
    fn map_vertex_stream_chunk_reports_tool_calls_when_stop_arrives_in_later_chunk() {
        let mut state = VertexStreamState::default();
        state.model = "gemini-3.8-flash".to_string();
        let thought_signatures = Arc::new(Mutex::new(ThoughtSignatureStore::new(16)));

        let call_chunk: VertexGenerateContentResponse = serde_json::from_value(serde_json::json!({
            "candidates": [{"content": {"role": "model", "parts": [{
                "functionCall": {"name": "glob_files", "args": {"pattern": "*"}},
                "thoughtSignature": "sig-a"
            }]}}],
            "responseId": "resp-3"
        }))
        .expect("call chunk");
        let stop_chunk: VertexGenerateContentResponse = serde_json::from_value(serde_json::json!({
            "candidates": [{
                "content": {"role": "model", "parts": [{"text": ""}]},
                "finishReason": "STOP"
            }],
            "responseId": "resp-3"
        }))
        .expect("stop chunk");

        map_vertex_stream_chunk(&call_chunk, &mut state, &thought_signatures);
        let events = map_vertex_stream_chunk(&stop_chunk, &mut state, &thought_signatures);

        let finish_reason = events.iter().find_map(|event| match &event.kind {
            UnifiedEventKind::CandidateDone { ending } => Some(ending.to_chat()),
            _ => None,
        });
        assert_eq!(finish_reason.as_deref(), Some("tool_calls"));
    }

    #[test]
    fn vertex_encodes_a_single_thinking_control() {
        // Effort-only requests encode a thinking level (Gemini 3).
        let level_only = UnifiedRequest {
            options: crate::provider::UnifiedRequestOptions {
                reasoning_effort: Some("high".into()),
                ..Default::default()
            },
            ..Default::default()
        };
        let config = map_vertex_thinking_config(&level_only)
            .expect("validated effort encodes")
            .expect("effort carries a level");
        assert_eq!(config.thinking_level, Some(VertexThinkingLevel::High));
        assert_eq!(config.thinking_budget, None);

        // Budget requests encode thinkingBudget (2.5); dynamic budgets omit.
        let budget = UnifiedRequest {
            options: crate::provider::UnifiedRequestOptions {
                thinking: Some(serde_json::json!({"type": "enabled", "budget_tokens": 1024})),
                ..Default::default()
            },
            ..Default::default()
        };
        let config = map_vertex_thinking_config(&budget)
            .expect("validated budget encodes")
            .expect("budget carries a limit");
        assert_eq!(config.thinking_budget, Some(1024));
        assert_eq!(config.thinking_level, None);
        let dynamic = UnifiedRequest {
            options: crate::provider::UnifiedRequestOptions {
                thinking: Some(serde_json::json!({"type": "adaptive"})),
                ..Default::default()
            },
            ..Default::default()
        };
        assert!(
            map_vertex_thinking_config(&dynamic)
                .expect("dynamic thinking omits the config")
                .is_none()
        );

        // Disabled encodes a zero budget (only reachable for 2.5-flash).
        let disabled = UnifiedRequest {
            options: crate::provider::UnifiedRequestOptions {
                thinking: Some(serde_json::json!({"type": "disabled"})),
                ..Default::default()
            },
            ..Default::default()
        };
        let config = map_vertex_thinking_config(&disabled)
            .expect("validated disable encodes")
            .expect("disable carries a zero budget");
        assert_eq!(config.thinking_budget, Some(0));

        // Two controls together violate the adaptation contract.
        let conflicting = UnifiedRequest {
            options: crate::provider::UnifiedRequestOptions {
                reasoning_effort: Some("high".into()),
                thinking: Some(serde_json::json!({"type": "enabled", "budget_tokens": 8192})),
                ..Default::default()
            },
            ..Default::default()
        };
        assert!(map_vertex_thinking_config(&conflicting).is_err());
        assert!(
            map_vertex_thinking_config(&UnifiedRequest::default())
                .expect("no reasoning selection")
                .is_none()
        );
    }

    #[test]
    fn gemini_output_tokens_are_clamped_to_model_limits() {
        assert_eq!(
            clamp_gemini_max_output_tokens("google/gemini-3.8-flash", Some(128_000)),
            Some(GEMINI_MAX_OUTPUT_TOKENS_65K)
        );
        assert_eq!(
            clamp_gemini_max_output_tokens("gemini-3-pro-image", Some(65_536)),
            Some(GEMINI_MAX_OUTPUT_TOKENS_32K)
        );
        assert_eq!(
            clamp_gemini_max_output_tokens("claude-opus-4-6", Some(128_000)),
            Some(128_000)
        );
        assert_eq!(
            clamp_gemini_max_output_tokens("gemini-5-preview", None),
            None
        );
    }

    #[test]
    fn extract_reasoning_from_candidate_uses_thought_parts_only() {
        let candidate = VertexCandidate {
            content: Some(VertexContent {
                role: Some("model".to_string()),
                parts: vec![
                    VertexPart {
                        text: Some("final answer".to_string()),
                        thought: Some(false),
                        ..Default::default()
                    },
                    VertexPart {
                        text: Some("hidden chain".to_string()),
                        thought: Some(true),
                        ..Default::default()
                    },
                ],
            }),
            ..Default::default()
        };

        assert_eq!(extract_text_from_candidate(&candidate), "final answer");
        assert_eq!(
            extract_reasoning_from_candidate(&candidate).as_deref(),
            Some("hidden chain")
        );
    }

    #[test]
    fn sanitize_vertex_schema_removes_unsupported_fields_recursively() {
        let original = serde_json::json!({
            "$schema": "https://json-schema.org/draft/2020-12/schema",
            "type": "object",
            "properties": {
                "name": {
                    "$schema": "https://json-schema.org/draft/2020-12/schema",
                    "type": "string",
                    "patternProperties": {
                        "^foo_": {"type": "string"}
                    }
                }
            },
            "allOf": [
                {
                    "$schema": "https://json-schema.org/draft/2020-12/schema",
                    "type": "object"
                }
            ]
        });

        let sanitized = sanitize_vertex_schema(&original);
        assert!(sanitized.get("$schema").is_none());
        assert!(sanitized.get("allOf").is_none());
        assert!(sanitized["properties"]["name"].get("$schema").is_none());
        assert!(
            sanitized["properties"]["name"]
                .get("patternProperties")
                .is_none()
        );
        assert_eq!(sanitized["type"], serde_json::json!("object"));
        assert_eq!(
            sanitized["properties"]["name"]["type"],
            serde_json::json!("string")
        );
    }

    #[test]
    fn sanitize_vertex_schema_maps_oneof_to_anyof() {
        // Trimmed from the fx agent's shell tool: the field names exist only in the variants.
        let original = serde_json::json!({
            "type": "object",
            "properties": {
                "request": {
                    "oneOf": [
                        {
                            "type": "object",
                            "properties": {
                                "action": {"type": "string", "enum": ["run"]},
                                "command": {"type": "string", "maxLength": 65536}
                            },
                            "additionalProperties": false,
                            "required": ["action", "command"]
                        },
                        {
                            "type": "object",
                            "properties": {
                                "action": {"type": "string", "enum": ["stop"]},
                                "session_id": {"type": "string"}
                            },
                            "additionalProperties": false,
                            "required": ["action", "session_id"]
                        }
                    ]
                }
            },
            "additionalProperties": false,
            "required": ["request"]
        });

        let sanitized = sanitize_vertex_schema(&original);
        let request = &sanitized["properties"]["request"];
        assert!(request.get("oneOf").is_none());
        assert_eq!(
            request["anyOf"],
            serde_json::json!([
                {
                    "type": "object",
                    "properties": {
                        "action": {"type": "string", "enum": ["run"]},
                        "command": {"type": "string"}
                    },
                    "required": ["action", "command"]
                },
                {
                    "type": "object",
                    "properties": {
                        "action": {"type": "string", "enum": ["stop"]},
                        "session_id": {"type": "string"}
                    },
                    "required": ["action", "session_id"]
                }
            ])
        );
    }

    #[test]
    fn sanitize_vertex_schema_keeps_anyof_over_oneof() {
        let original = serde_json::json!({
            "anyOf": [{"type": "string"}],
            "oneOf": [{"type": "integer"}]
        });

        let sanitized = sanitize_vertex_schema(&original);
        assert_eq!(
            sanitized,
            serde_json::json!({"anyOf": [{"type": "string"}]})
        );
    }

    #[test]
    fn sanitize_vertex_schema_maps_ref_and_defs_keys() {
        let original = serde_json::json!({
            "type": "object",
            "$defs": {
                "Location": {
                    "type": "object",
                    "properties": {
                        "city": {"type": "string"}
                    }
                }
            },
            "properties": {
                "loc": {"$ref": r"#/$defs/Location"}
            }
        });

        let sanitized = sanitize_vertex_schema(&original);
        assert!(sanitized.get("$defs").is_none());
        assert!(sanitized["properties"]["loc"].get("$ref").is_none());
        assert!(sanitized.get("defs").is_some());
        assert_eq!(
            sanitized["properties"]["loc"]["ref"],
            serde_json::json!("#/defs/Location")
        );
    }

    #[test]
    fn sanitize_vertex_schema_drops_invalid_ref() {
        let original = serde_json::json!({
            "type": "object",
            "properties": {
                "loc": {"$ref": "#/defs/Location/City"}
            }
        });

        let sanitized = sanitize_vertex_schema(&original);
        assert!(sanitized["properties"]["loc"].get("ref").is_none());
    }

    #[test]
    fn sanitize_vertex_schema_drops_non_object_defs() {
        let original = serde_json::json!({
            "type": "object",
            "$defs": "not-an-object",
            "properties": {
                "loc": {"$ref": "#/defs/Location"}
            }
        });

        let sanitized = sanitize_vertex_schema(&original);
        assert!(sanitized.get("defs").is_none());
    }

    #[test]
    fn sanitize_vertex_schema_drops_non_object_items() {
        let original = serde_json::json!({
            "type": "array",
            "items": "not-an-object"
        });

        let sanitized = sanitize_vertex_schema(&original);
        assert!(sanitized.get("items").is_none());
    }

    #[test]
    fn sanitize_vertex_schema_drops_non_array_anyof() {
        let original = serde_json::json!({
            "type": "object",
            "anyOf": {"type": "string"}
        });

        let sanitized = sanitize_vertex_schema(&original);
        assert!(sanitized.get("anyOf").is_none());
    }

    #[test]
    fn sanitize_vertex_schema_drops_ref_with_missing_def() {
        let original = serde_json::json!({
            "type": "object",
            "$defs": {
                "Location": {"type": "object"}
            },
            "properties": {
                "loc": {"$ref": "#/defs/Unknown"}
            }
        });

        let sanitized = sanitize_vertex_schema(&original);
        assert!(sanitized["properties"]["loc"].get("ref").is_none());
    }

    #[test]
    fn ensure_vertex_object_schema_forces_object_type() {
        let original = serde_json::json!({
            "type": "string",
            "description": "not object"
        });
        let ensured = ensure_vertex_object_schema(original);

        assert_eq!(ensured["type"], serde_json::json!("object"));
        assert_eq!(ensured["description"], serde_json::json!("not object"));
    }

    #[test]
    fn ensure_vertex_object_schema_handles_non_object_value() {
        let ensured = ensure_vertex_object_schema(serde_json::json!("oops"));

        assert_eq!(ensured["type"], serde_json::json!("object"));
        assert!(ensured["properties"].is_object());
    }

    #[test]
    fn sanitize_vertex_schema_drops_empty_property_keys() {
        let original = serde_json::json!({
            "type": "object",
            "properties": {
                "": {"type": "string"},
                "  ": {"type": "string"},
                "valid": {"type": "string"}
            }
        });

        let sanitized = sanitize_vertex_schema(&original);
        assert!(sanitized["properties"].get("").is_none());
        assert!(sanitized["properties"].get("  ").is_none());
        assert!(sanitized["properties"].get("valid").is_some());
    }

    #[test]
    fn sanitize_vertex_schema_drops_non_array_enum() {
        let original = serde_json::json!({
            "type": "string",
            "enum": "not-an-array"
        });

        let sanitized = sanitize_vertex_schema(&original);
        assert!(sanitized.get("enum").is_none());
    }

    #[test]
    fn sanitize_vertex_schema_drops_non_array_required() {
        let original = serde_json::json!({
            "type": "object",
            "required": "name"
        });

        let sanitized = sanitize_vertex_schema(&original);
        assert!(sanitized.get("required").is_none());
    }

    #[test]
    fn sanitize_vertex_schema_drops_required_with_non_string_items() {
        let original = serde_json::json!({
            "type": "object",
            "required": ["name", 2]
        });

        let sanitized = sanitize_vertex_schema(&original);
        assert!(sanitized.get("required").is_none());
    }

    #[test]
    fn parse_tool_response_payload_wraps_plain_text_as_object() {
        let payload = parse_tool_response_payload(&Content::Text("hello world".to_string()));
        assert_eq!(payload, serde_json::json!({"content": "hello world"}));
    }

    #[test]
    fn parse_tool_response_payload_keeps_object_payload() {
        let payload =
            parse_tool_response_payload(&Content::Text("{\"status\":\"ok\"}".to_string()));
        assert_eq!(payload, serde_json::json!({"status": "ok"}));
    }

    #[test]
    fn extracts_and_caches_thought_signature() {
        let candidate = VertexCandidate {
            content: Some(VertexContent {
                role: Some("model".to_string()),
                parts: vec![VertexPart {
                    function_call: Some(VertexFunctionCall {
                        name: "check_flight".to_string(),
                        args: serde_json::json!({"flight": "AA100"}),
                    }),
                    thought_signature: Some("sig-a".to_string()),
                    ..Default::default()
                }],
            }),
            finish_reason: Some("STOP".to_string()),
        };

        let extracted = extract_tool_calls_from_candidate(&candidate, "resp-1");
        assert_eq!(extracted.len(), 1);
        let (call, signature) = &extracted[0];
        assert_eq!(call.id.as_deref(), Some("resp-1-tool-0"));
        assert_eq!(signature.as_deref(), Some("sig-a"));

        let signatures = Arc::new(Mutex::new(ThoughtSignatureStore::new(16)));
        cache_thought_signatures(&signatures, &extracted);

        let cached = signatures.lock().expect("lock store").get("resp-1-tool-0");
        assert_eq!(cached.as_deref(), Some("sig-a"));
    }

    #[tokio::test]
    async fn build_vertex_request_reinjects_cached_signature_for_assistant_tool_call() {
        let signatures = Arc::new(Mutex::new(ThoughtSignatureStore::new(16)));
        signatures
            .lock()
            .expect("lock store")
            .insert("call-1".to_string(), "sig-a".to_string());

        let request = ChatCompletionRequest {
            model: "gemini".to_string(),
            messages: vec![
                Message {
                    role: Role::User,
                    content: Content::Text("check flight".to_string()),
                    reasoning_content: None,
                    reasoning_blocks: None,
                    cache_control: None,
                    tool_call_id: None,
                    tool_calls: None,
                },
                Message {
                    role: Role::Assistant,
                    content: Content::Text("Tool call".to_string()),
                    reasoning_content: None,
                    reasoning_blocks: None,
                    cache_control: None,
                    tool_call_id: None,
                    tool_calls: Some(vec![ToolCall {
                        id: Some("call-1".to_string()),
                        r#type: Some("function".to_string()),
                        function: ToolCallFunction {
                            name: "check_flight".to_string(),
                            arguments: "{\"flight\":\"AA100\"}".to_string(),
                            description: None,
                        },
                    }]),
                },
                Message {
                    role: Role::Tool,
                    content: Content::Text("{\"status\":\"delayed\"}".to_string()),
                    reasoning_content: None,
                    reasoning_blocks: None,
                    cache_control: None,
                    tool_call_id: Some("call-1".to_string()),
                    tool_calls: None,
                },
            ],
            n: None,
            temperature: None,
            top_p: None,
            presence_penalty: None,
            frequency_penalty: None,
            logprobs: None,
            top_logprobs: None,
            modalities: None,
            audio: None,
            max_completion_tokens: None,
            stop: None,
            response_format: None,
            thinking: None,
            reasoning_effort: None,
            chat_template_kwargs: None,
            prediction: None,
            verbosity: None,
            tools: Some(vec![ToolDefinition {
                r#type: "function".to_string(),
                function: FunctionDefinition {
                    name: "check_flight".to_string(),
                    description: None,
                    strict: None,
                    parameters: serde_json::json!({"type": "object"}),
                },

                extra: Default::default(),
            }]),
            tool_choice: None,
            allowed_tools: None,
            parallel_tool_calls: None,
            service_tier: None,
            seed: None,
            stream: None,
            stream_options: None,
            metadata: None,
            agent_context: None,
            extra: Default::default(),
        };

        let http = reqwest::Client::new();
        let mapped = build_vertex_request(&request, &http, &signatures)
            .await
            .expect("build vertex request");

        let assistant_part = mapped
            .contents
            .iter()
            .find_map(|content| {
                if content.role.as_deref() == Some("model") {
                    content.parts.first()
                } else {
                    None
                }
            })
            .expect("assistant function call part");
        assert_eq!(assistant_part.thought_signature.as_deref(), Some("sig-a"));

        let tool_part = mapped
            .contents
            .iter()
            .find_map(|content| {
                content
                    .parts
                    .iter()
                    .find(|part| part.function_response.is_some())
            })
            .and_then(|part| part.function_response.as_ref())
            .expect("tool function response part");
        assert_eq!(tool_part.name, "check_flight");
        assert_eq!(tool_part.response, serde_json::json!({"status": "delayed"}));
    }

    #[tokio::test]
    async fn build_vertex_request_strips_schema_from_tool_parameters() {
        let signatures = Arc::new(Mutex::new(ThoughtSignatureStore::new(16)));
        let request = ChatCompletionRequest {
            model: "gemini".to_string(),
            messages: vec![Message {
                role: Role::User,
                content: Content::Text("hello".to_string()),
                reasoning_content: None,
                reasoning_blocks: None,
                cache_control: None,
                tool_call_id: None,
                tool_calls: None,
            }],
            n: None,
            temperature: None,
            top_p: None,
            presence_penalty: None,
            frequency_penalty: None,
            logprobs: None,
            top_logprobs: None,
            modalities: None,
            audio: None,
            max_completion_tokens: None,
            stop: None,
            response_format: None,
            thinking: None,
            reasoning_effort: None,
            chat_template_kwargs: None,
            prediction: None,
            verbosity: None,
            tools: Some(vec![ToolDefinition {
                r#type: "function".to_string(),
                function: FunctionDefinition {
                    name: "tool_a".to_string(),
                    description: None,
                    strict: None,
                    parameters: serde_json::json!({
                        "$schema": "https://json-schema.org/draft/2020-12/schema",
                        "type": "object",
                        "properties": {
                            "city": {
                                "$schema": "https://json-schema.org/draft/2020-12/schema",
                                "type": "string"
                            }
                        }
                    }),
                },

                extra: Default::default(),
            }]),
            tool_choice: None,
            allowed_tools: None,
            parallel_tool_calls: None,
            service_tier: None,
            seed: None,
            stream: None,
            stream_options: None,
            metadata: None,
            agent_context: None,
            extra: Default::default(),
        };

        let http = reqwest::Client::new();
        let mapped = build_vertex_request(&request, &http, &signatures)
            .await
            .expect("build vertex request");
        let parameters = &mapped
            .tools
            .as_ref()
            .expect("tools")
            .first()
            .expect("first tool")
            .function_declarations
            .first()
            .expect("first declaration")
            .parameters;

        assert!(parameters.get("$schema").is_none());
        assert!(parameters["properties"]["city"].get("$schema").is_none());
    }

    #[tokio::test]
    async fn build_vertex_request_parallel_tool_calls_only_reinjects_existing_signature() {
        let signatures = Arc::new(Mutex::new(ThoughtSignatureStore::new(16)));
        signatures
            .lock()
            .expect("lock store")
            .insert("call-paris".to_string(), "sig-paris".to_string());

        let request = ChatCompletionRequest {
            model: "gemini".to_string(),
            messages: vec![
                Message {
                    role: Role::User,
                    content: Content::Text("Check weather in Paris and London".to_string()),
                    reasoning_content: None,
                    reasoning_blocks: None,
                    cache_control: None,
                    tool_call_id: None,
                    tool_calls: None,
                },
                Message {
                    role: Role::Assistant,
                    content: Content::Text("Tool call".to_string()),
                    reasoning_content: None,
                    reasoning_blocks: None,
                    cache_control: None,
                    tool_call_id: None,
                    tool_calls: Some(vec![
                        ToolCall {
                            id: Some("call-paris".to_string()),
                            r#type: Some("function".to_string()),
                            function: ToolCallFunction {
                                name: "get_current_temperature".to_string(),
                                arguments: "{\"location\":\"Paris\"}".to_string(),
                                description: None,
                            },
                        },
                        ToolCall {
                            id: Some("call-london".to_string()),
                            r#type: Some("function".to_string()),
                            function: ToolCallFunction {
                                name: "get_current_temperature".to_string(),
                                arguments: "{\"location\":\"London\"}".to_string(),
                                description: None,
                            },
                        },
                    ]),
                },
                Message {
                    role: Role::Tool,
                    content: Content::Text("{\"temp\":\"15C\"}".to_string()),
                    reasoning_content: None,
                    reasoning_blocks: None,
                    cache_control: None,
                    tool_call_id: Some("call-paris".to_string()),
                    tool_calls: None,
                },
                Message {
                    role: Role::Tool,
                    content: Content::Text("{\"temp\":\"12C\"}".to_string()),
                    reasoning_content: None,
                    reasoning_blocks: None,
                    cache_control: None,
                    tool_call_id: Some("call-london".to_string()),
                    tool_calls: None,
                },
            ],
            n: None,
            temperature: None,
            top_p: None,
            presence_penalty: None,
            frequency_penalty: None,
            logprobs: None,
            top_logprobs: None,
            modalities: None,
            audio: None,
            max_completion_tokens: None,
            stop: None,
            response_format: None,
            thinking: None,
            reasoning_effort: None,
            chat_template_kwargs: None,
            prediction: None,
            verbosity: None,
            tools: Some(vec![ToolDefinition {
                r#type: "function".to_string(),
                function: FunctionDefinition {
                    name: "get_current_temperature".to_string(),
                    description: None,
                    strict: None,
                    parameters: serde_json::json!({"type": "object"}),
                },

                extra: Default::default(),
            }]),
            tool_choice: None,
            allowed_tools: None,
            parallel_tool_calls: None,
            service_tier: None,
            seed: None,
            stream: None,
            stream_options: None,
            metadata: None,
            agent_context: None,
            extra: Default::default(),
        };

        let http = reqwest::Client::new();
        let mapped = build_vertex_request(&request, &http, &signatures)
            .await
            .expect("build vertex request");

        let assistant_model_content = mapped
            .contents
            .iter()
            .find(|content| content.role.as_deref() == Some("model"))
            .expect("assistant model content");
        assert_eq!(assistant_model_content.parts.len(), 2);
        assert_eq!(
            assistant_model_content.parts[0]
                .thought_signature
                .as_deref(),
            Some("sig-paris")
        );
        assert_eq!(assistant_model_content.parts[1].thought_signature, None);

        // Gemini rejects the turn unless every response to a parallel call sits
        // in the single content that follows the function call content.
        assert_eq!(mapped.contents.len(), 3);
        let response_content = &mapped.contents[2];
        assert_eq!(response_content.role.as_deref(), Some("user"));
        let function_response_parts = response_content
            .parts
            .iter()
            .filter_map(|part| part.function_response.as_ref())
            .collect::<Vec<_>>();
        assert_eq!(function_response_parts.len(), 2);
        assert_eq!(
            function_response_parts[0].response,
            serde_json::json!({"temp": "15C"})
        );
        assert_eq!(
            function_response_parts[1].response,
            serde_json::json!({"temp": "12C"})
        );
    }

    #[test]
    fn encoder_rejection_messages_do_not_name_the_platform() {
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("high".into());
        request.options.thinking = Some(serde_json::json!({"type": "enabled"}));

        match map_vertex_thinking_config(&request) {
            Err(ProviderError::Public { error, .. }) => {
                assert!(!error.message.contains("Vertex"), "{}", error.message);
            }
            other => panic!("expected public bad request, got {other:?}"),
        }
    }

    #[test]
    fn map_http_error_maps_missing_thought_signature_error_type() {
        let body = r#"{
          "error": {
            "code": 400,
            "message": "Function call is missing a thought_signature in functionCall parts. This is required for tools to work correctly, and missing thought_signature may lead to degraded model performance. Additional data, function call `default_api:session_status` , position 26.",
            "status": "INVALID_ARGUMENT"
          }
        }"#;

        let err = map_http_error(StatusCode::BAD_REQUEST, body.as_bytes());
        match err {
            ProviderError::Public { status, error } => {
                assert_eq!(status, StatusCode::BAD_REQUEST);
                assert_eq!(error.r#type, "missing_thought_signature_error");
                assert_eq!(error.code.as_deref(), Some("missing_thought_signature"));
                assert!(error.message.contains("missing a thought_signature"));
            }
            other => panic!("expected public bad request, got {other:?}"),
        }
    }

    #[test]
    fn map_http_error_maps_invalid_argument_type_from_vertex_status() {
        let body = r#"{
          "error": {
            "code": 400,
            "message": "Unsupported parameter.",
            "status": "INVALID_ARGUMENT"
          }
        }"#;

        let err = map_http_error(StatusCode::BAD_REQUEST, body.as_bytes());
        match err {
            ProviderError::Public { status, error } => {
                assert_eq!(status, StatusCode::BAD_REQUEST);
                assert_eq!(error.r#type, "invalid_argument_error");
                assert_eq!(error.code.as_deref(), Some("INVALID_ARGUMENT"));
                assert_eq!(error.message, "Unsupported parameter.");
            }
            other => panic!("expected public bad request, got {other:?}"),
        }
    }
}
