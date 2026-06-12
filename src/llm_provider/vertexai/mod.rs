use std::collections::{HashMap, VecDeque};
use std::pin::Pin;
use std::sync::{Arc, Mutex};
use std::time::{SystemTime, UNIX_EPOCH};

pub mod client;
pub mod types;

use async_stream::try_stream;
use async_trait::async_trait;
use base64::Engine as _;
use base64::engine::general_purpose::STANDARD as BASE64_STANDARD;
use futures_core::Stream;
use futures_util::StreamExt;
use log::debug;
use reqwest::StatusCode;
use serde_json::Value;

use self::client::VertexAIClient;
use self::types::{
    VertexCandidate, VertexContent, VertexFunctionCall, VertexFunctionCallingConfig,
    VertexFunctionDeclaration, VertexFunctionResponse, VertexGenerateContentRequest,
    VertexGenerateContentResponse, VertexGenerationConfig, VertexInlineData, VertexPart,
    VertexSystemInstruction, VertexThinkingConfig, VertexThinkingLevel, VertexTool,
    VertexToolConfig, VertexUsageMetadata,
};
use crate::llm_provider::openai_compatible::mapper::ensure_tool_call_id;
use crate::llm_provider::{
    FinishReason, Provider, ProviderError, ToolCall, UnifiedEvent, UnifiedResponse,
};
use crate::model_api_provider::GenerateContentUsage;
use crate::openai_http_mapping::validate_openai_request;
use crate::openai_types::{
    ChatCompletionRequest, Content, ContentPart, ErrorDetail, ResponseFormat, Role, ToolChoice,
};
use crate::serve_config::ConfigError;
use crate::usage_handler::EndpointUsage;
use crate::utils::{MAX_LOG_BODY_BYTES, truncate_for_log};

const MAX_IMAGE_BYTES: usize = 10 * 1024 * 1024;

#[derive(Clone)]
pub struct VertexAIProvider {
    client: VertexAIClient,
    model_id: String,
    thought_signatures: Arc<Mutex<ThoughtSignatureStore>>,
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
        Ok(Self {
            client,
            model_id,
            thought_signatures: Arc::new(Mutex::new(ThoughtSignatureStore::new(
                MAX_THOUGHT_SIGNATURES,
            ))),
        })
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
        let body =
            build_vertex_request(&request, self.client.http(), &self.thought_signatures).await?;
        let response = self
            .client
            .post_json_with_headers(
                &self.model_id,
                serde_json::to_vec(&body)
                    .map_err(|err| ProviderError::internal(err.to_string()))?,
                false,
                axum::http::HeaderMap::new(),
            )
            .await
            .map_err(|err| ProviderError::internal(err.to_string()))?;

        let status = response.status();
        let bytes = response
            .bytes()
            .await
            .map_err(|err| ProviderError::internal(err.to_string()))?;
        debug_response_json("non_stream", Some(status), &bytes);
        if !status.is_success() {
            return Err(map_http_error(status, &bytes));
        }
        let value: VertexGenerateContentResponse = serde_json::from_slice(&bytes)
            .map_err(|err| ProviderError::internal(format!("parse vertex response: {err}")))?;
        map_vertex_response(value, &self.model_id, &self.thought_signatures)
    }

    async fn stream<'a>(
        &'a self,
        request: ChatCompletionRequest,
    ) -> Result<
        Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    > {
        validate_request(&request)?;
        let body =
            build_vertex_request(&request, self.client.http(), &self.thought_signatures).await?;
        let response = self
            .client
            .post_json_with_headers(
                &self.model_id,
                serde_json::to_vec(&body)
                    .map_err(|err| ProviderError::internal(err.to_string()))?,
                true,
                axum::http::HeaderMap::new(),
            )
            .await
            .map_err(|err| ProviderError::internal(err.to_string()))?;

        let status = response.status();
        if !status.is_success() {
            let bytes = response
                .bytes()
                .await
                .map_err(|err| ProviderError::internal(err.to_string()))?;
            debug_response_json("stream", Some(status), &bytes);
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
                let chunk = item.map_err(|err| ProviderError::internal(err.to_string()))?;
                debug_response_json("stream_chunk", None, &chunk);
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
                        debug_stream_event_json(data);
                        if data.trim() == "[DONE]" {
                            if !state.completed {
                                yield UnifiedEvent::Completed {
                                    finish_reason: Some("stop".to_string()),
                                };
                            }
                            return;
                        }

                        let value: VertexGenerateContentResponse = serde_json::from_str(data)
                            .map_err(|err| ProviderError::internal(format!("parse vertex stream event: {err}")))?;

                        for event in map_vertex_stream_chunk(&value, &mut state, &self.thought_signatures) {
                            yield event;
                        }
                    }
                }
            }

            if !state.completed {
                yield UnifiedEvent::Completed {
                    finish_reason: Some("stop".to_string()),
                };
            }
        };

        Ok(Box::pin(output))
    }
}

const MAX_THOUGHT_SIGNATURES: usize = 4096;

#[derive(Default)]
struct ThoughtSignatureStore {
    max_entries: usize,
    by_tool_call_id: HashMap<String, String>,
    insertion_order: VecDeque<String>,
}

impl ThoughtSignatureStore {
    fn new(max_entries: usize) -> Self {
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

fn debug_body(bytes: &[u8]) -> String {
    let body = String::from_utf8_lossy(bytes);
    let compact = compact_json_string(&body);
    truncate_for_log(&compact)
}

fn debug_body_value(bytes: &[u8]) -> Value {
    let body = String::from_utf8_lossy(bytes);
    serde_json::from_str::<Value>(&body).unwrap_or_else(|_| Value::String(debug_body(bytes)))
}

fn debug_response_json(event: &str, status: Option<StatusCode>, body: &[u8]) {
    let truncated = body.len() > MAX_LOG_BODY_BYTES;
    let payload = serde_json::json!({
        "target": "vertexai.client.response",
        "event": event,
        "status": status.map(|value| value.as_u16()),
        "body": debug_body_value(body),
        "truncated": truncated,
    });
    debug!("{}", payload);
}

fn debug_stream_event_json(data: &str) {
    let compact = compact_json_string(data);
    let data_value = serde_json::from_str::<Value>(&compact)
        .unwrap_or_else(|_| Value::String(truncate_for_log(&compact)));
    let payload = serde_json::json!({
        "target": "vertexai.client.response",
        "event": "stream_event",
        "data": data_value,
        "truncated": compact.len() > MAX_LOG_BODY_BYTES,
    });
    debug!("{}", payload);
}

fn compact_json_string(value: &str) -> String {
    serde_json::from_str::<Value>(value)
        .map(|json| json.to_string())
        .unwrap_or_else(|_| value.to_string())
}

#[derive(Default)]
struct VertexStreamState {
    started: bool,
    completed: bool,
    request_id: String,
    model: String,
    created_at: String,
}

fn validate_request(request: &ChatCompletionRequest) -> Result<(), ProviderError> {
    validate_openai_request(request).map_err(ProviderError::internal)
}

async fn build_vertex_request(
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
                contents.push(VertexContent {
                    role: Some("user".to_string()),
                    parts: vec![VertexPart {
                        function_response: Some(VertexFunctionResponse {
                            name: tool_name,
                            response,
                        }),
                        ..Default::default()
                    }],
                });
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
        thinking_config: map_vertex_thinking_config(request),
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
                        return Err(ProviderError::internal(
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

fn map_vertex_response(
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

    Ok(UnifiedResponse {
        request_id,
        created_at: chrono::Utc::now().to_rfc3339(),
        model,
        output_text,
        reasoning_content,
        tool_calls,
        finish_reason,
        usage,
    })
}

fn map_vertex_stream_chunk(
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

    let thinking_delta = extract_reasoning_from_candidate(&candidate);
    if let Some(thinking_delta) = thinking_delta {
        events.push(UnifiedEvent::ThinkingDelta {
            id: state.request_id.clone(),
            delta: thinking_delta,
        });
    }

    let calls = extract_tool_calls_from_candidate(&candidate, &state.request_id);
    cache_thought_signatures(thought_signatures, &calls);
    if let Some(calls) = map_extracted_tool_calls(calls) {
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

    if let Some(usage) = map_usage_from_usage_metadata(value.usage_metadata.as_ref()) {
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
        });
        state.completed = true;
    }

    events
}

fn map_usage_from_usage_metadata(usage: Option<&VertexUsageMetadata>) -> Option<EndpointUsage> {
    let payload = serde_json::to_value(usage?).ok()?;
    let usage = serde_json::from_value::<GenerateContentUsage>(payload).ok()?;
    Some(EndpointUsage::GenerateContent(usage))
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

fn map_vertex_thinking_config(request: &ChatCompletionRequest) -> Option<VertexThinkingConfig> {
    if should_disable_thinking_config_for_model(&request.model) {
        return None;
    }

    let thinking_level = request
        .reasoning_effort
        .as_deref()
        .and_then(map_reasoning_effort_to_vertex_level)?;

    Some(VertexThinkingConfig {
        thinking_budget: None,
        thinking_level: Some(thinking_level),
    })
}

fn should_disable_thinking_config_for_model(model: &str) -> bool {
    let model = model.strip_prefix("google/").unwrap_or(model);
    model.starts_with("gemini-2.5-pro") || model.starts_with("gemini-2.5-flash")
}

fn map_reasoning_effort_to_vertex_level(effort: &str) -> Option<VertexThinkingLevel> {
    match effort {
        "minimal" => Some(VertexThinkingLevel::Minimal),
        "low" => Some(VertexThinkingLevel::Low),
        "medium" => Some(VertexThinkingLevel::Medium),
        "high" | "max" => Some(VertexThinkingLevel::High),
        _ => None,
    }
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

fn parse_function_call_args(arguments: &str) -> Value {
    serde_json::from_str::<Value>(arguments)
        .unwrap_or_else(|_| Value::String(arguments.to_string()))
}

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
    if status.as_u16() == 400 {
        let mapped = map_vertex_bad_request(body).unwrap_or_else(|| ErrorDetail {
            message: text.clone(),
            r#type: "invalid_request_error".to_string(),
            code: None,
            param: None,
        });
        return ProviderError::Public {
            status: StatusCode::BAD_REQUEST,
            error: mapped,
        };
    }
    ProviderError::internal_with_upstream_status(
        status,
        format!("vertexai request failed with status {status}: {text}"),
    )
}

#[derive(serde::Deserialize)]
struct VertexErrorEnvelope {
    error: Option<VertexErrorPayload>,
}

#[derive(serde::Deserialize)]
struct VertexErrorPayload {
    code: Option<i64>,
    message: Option<String>,
    status: Option<String>,
}

fn map_vertex_bad_request(body: &[u8]) -> Option<ErrorDetail> {
    let parsed = serde_json::from_slice::<VertexErrorEnvelope>(body).ok()?;
    let payload = parsed.error?;
    let message = payload.message?.trim().to_string();
    if message.is_empty() {
        return None;
    }

    let normalized_status = payload
        .status
        .as_deref()
        .map(normalize_vertex_error_status_code);
    let message_lower = message.to_ascii_lowercase();
    let (error_type, code) = if message_lower.contains("missing a thought_signature") {
        (
            "missing_thought_signature_error".to_string(),
            Some("missing_thought_signature".to_string()),
        )
    } else {
        (
            map_vertex_error_type(payload.status.as_deref()).to_string(),
            normalized_status,
        )
    };

    let code = code.or_else(|| payload.code.map(|value| value.to_string()));

    Some(ErrorDetail {
        message,
        r#type: error_type,
        code,
        param: None,
    })
}

fn map_vertex_error_type(status: Option<&str>) -> &'static str {
    match status {
        Some("UNAUTHENTICATED") => "authentication_error",
        Some("PERMISSION_DENIED") => "permission_denied_error",
        Some("RESOURCE_EXHAUSTED") => "rate_limit_error",
        Some("FAILED_PRECONDITION") => "failed_precondition_error",
        Some("INVALID_ARGUMENT") => "invalid_argument_error",
        _ => "invalid_request_error",
    }
}

fn normalize_vertex_error_status_code(status: &str) -> String {
    status.trim().to_ascii_lowercase()
}

fn new_response_id() -> String {
    let ts = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis())
        .unwrap_or(0);
    format!("vertexai-{ts}")
}

pub fn build_vertexai_provider(
    params: &HashMap<String, String>,
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
    fn map_reasoning_effort_to_vertex_level_works() {
        assert_eq!(
            map_reasoning_effort_to_vertex_level("minimal"),
            Some(VertexThinkingLevel::Minimal)
        );
        assert_eq!(
            map_reasoning_effort_to_vertex_level("low"),
            Some(VertexThinkingLevel::Low)
        );
        assert_eq!(
            map_reasoning_effort_to_vertex_level("medium"),
            Some(VertexThinkingLevel::Medium)
        );
        assert_eq!(
            map_reasoning_effort_to_vertex_level("high"),
            Some(VertexThinkingLevel::High)
        );
        assert_eq!(
            map_reasoning_effort_to_vertex_level("max"),
            Some(VertexThinkingLevel::High)
        );
        assert_eq!(map_reasoning_effort_to_vertex_level("unknown"), None);
    }

    #[test]
    fn map_vertex_thinking_config_only_uses_reasoning_effort() {
        let high_effort: ChatCompletionRequest = serde_json::from_value(serde_json::json!({
            "model": "gemini-2.5-flash",
            "messages": [{"role": "user", "content": "hi"}],
            "reasoning_effort": "high"
        }))
        .expect("parse effort request");
        assert!(map_vertex_thinking_config(&high_effort).is_none());

        let high_effort_other_model: ChatCompletionRequest =
            serde_json::from_value(serde_json::json!({
                "model": "gemini-3.0-pro",
                "messages": [{"role": "user", "content": "hi"}],
                "reasoning_effort": "high"
            }))
            .expect("parse effort request");
        let config =
            map_vertex_thinking_config(&high_effort_other_model).expect("config must exist");
        assert_eq!(config.thinking_budget, None);
        assert_eq!(config.thinking_level, Some(VertexThinkingLevel::High));
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
                    tool_call_id: None,
                    tool_calls: None,
                },
                Message {
                    role: Role::Assistant,
                    content: Content::Text("Tool call".to_string()),
                    reasoning_content: None,
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
                    tool_call_id: None,
                    tool_calls: None,
                },
                Message {
                    role: Role::Assistant,
                    content: Content::Text("Tool call".to_string()),
                    reasoning_content: None,
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
                    tool_call_id: Some("call-paris".to_string()),
                    tool_calls: None,
                },
                Message {
                    role: Role::Tool,
                    content: Content::Text("{\"temp\":\"12C\"}".to_string()),
                    reasoning_content: None,
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

        let function_response_parts = mapped
            .contents
            .iter()
            .flat_map(|content| content.parts.iter())
            .filter_map(|part| part.function_response.as_ref())
            .collect::<Vec<_>>();
        assert_eq!(function_response_parts.len(), 2);
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
                assert_eq!(error.code.as_deref(), Some("invalid_argument"));
                assert_eq!(error.message, "Unsupported parameter.");
            }
            other => panic!("expected public bad request, got {other:?}"),
        }
    }
}
