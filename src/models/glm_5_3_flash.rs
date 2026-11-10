use std::io::{self, Write};

use serde_json::{Value, json};

use crate::dialects::Dialect;
use crate::dialects::openai_chat_completions::mapper::request_to_chat;
use crate::models::{ModelProfile, ModelProfileError, from_dialect_conversion_error};
use crate::openai_types::{ChatCompletionRequest, ThinkingConfig, ThinkingType};
use crate::provider::{
    ReasoningEffort, UnifiedContentPart, UnifiedContentPartKind, UnifiedDocumentSource,
    UnifiedInputItem, UnifiedInputItemKind, UnifiedMediaSource, UnifiedRequest, UnifiedRole,
    UnifiedToolOutput,
};
use crate::serve_config::ModelBinding;
use crate::upstreams::Upstream;

// This profile follows Tencent Cloud's GLM guide for the TokenHub Chat route:
// https://cloud.tencent.com/document/product/1823/132061#62f5454e-cb70-4280-8c28-5a4b6a2f181b
const MAX_OUTPUT_TOKENS: i64 = 131_072;
const MAX_REQUEST_BODY_BYTES: usize = 100_000_000;
const SUPPORTED_IMAGE_MIMES: &[&str] = &["image/jpeg", "image/jpg", "image/png", "image/webp"];
const SUPPORTED_IMAGE_EXTENSIONS: &[&str] = &["jpg", "jpeg", "png", "webp"];
const SUPPORTED_VIDEO_MIMES: &[&str] = &[
    "video/mp4",
    "video/mpeg",
    "video/quicktime",
    "video/x-msvideo",
    "video/webm",
    "video/x-ms-wmv",
    "video/3gpp",
];
const SUPPORTED_VIDEO_EXTENSIONS: &[&str] = &[
    "mp4", "mpeg", "mpg", "mov", "avi", "webm", "wmv", "3gp", "3gpp",
];
const SUPPORTED_FILE_MIMES: &[&str] = &["application/pdf", "text/plain", "application/msword"];
const SUPPORTED_FILE_EXTENSIONS: &[&str] = &["pdf", "txt", "doc"];

#[derive(Clone, Copy, PartialEq, Eq)]
enum InputMediaKind {
    Image,
    Video,
    File,
}

/// GLM-5.3-Flash request behavior for the exact `glm-5.3-flash` public model.
pub struct Glm53FlashProfile;

impl ModelProfile for Glm53FlashProfile {
    fn to_openai_chat_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        // The referenced GLM guide describes TokenHub's endpoint. Do not apply
        // those endpoint-specific rules to an Ark or other binding with this name.
        if binding.upstream != Upstream::Tokenhub
            || binding.dialect != Some(Dialect::OpenAiChatCompletions)
        {
            return Ok(None);
        }

        validate_request(request)?;
        let mut normalized_request = request.clone();
        if let Some(instructions) = &mut normalized_request.instructions {
            normalize_items(instructions)?;
        }
        normalize_items(&mut normalized_request.input)?;

        let mut chat_request =
            request_to_chat(&normalized_request).map_err(from_dialect_conversion_error)?;
        if let Some(top_k) = normalized_request.options.top_k {
            chat_request.extra.insert("top_k".to_owned(), json!(top_k));
        }
        apply_glm_defaults(&mut chat_request);
        validate_serialized_request_size(&chat_request)?;
        Ok(Some(chat_request))
    }
}

fn validate_request(request: &UnifiedRequest) -> Result<(), ModelProfileError> {
    if let Some(max_output_tokens) = request.options.max_output_tokens {
        if max_output_tokens > MAX_OUTPUT_TOKENS {
            return Err(ModelProfileError::InvalidRequest {
                message: format!(
                    "glm-5.3-flash supports at most {MAX_OUTPUT_TOKENS} output tokens; requested {max_output_tokens}"
                ),
                param: Some("max_output_tokens".to_owned()),
            });
        }
    }

    if request.options.audio.is_some()
        || request
            .options
            .modalities
            .as_ref()
            .is_some_and(|modalities| {
                modalities
                    .iter()
                    .any(|modality| !modality.eq_ignore_ascii_case("text"))
            })
    {
        return Err(unsupported_capability(
            "glm-5.3-flash only supports text output",
            "modalities",
        ));
    }

    validate_reasoning_controls(request)?;
    let mut input_modality = None;
    if let Some(instructions) = &request.instructions {
        validate_items(instructions, &mut input_modality)?;
    }
    validate_items(&request.input, &mut input_modality)
}

fn validate_reasoning_controls(request: &UnifiedRequest) -> Result<(), ModelProfileError> {
    if let Some(effort) = &request.options.reasoning_effort {
        match effort.as_str() {
            Some("low" | "high" | "max") => {}
            Some(value) => {
                return Err(unsupported_capability(
                    format!("glm-5.3-flash does not support reasoning_effort `{value}`"),
                    "reasoning_effort",
                ));
            }
            None => {
                return Err(unsupported_capability(
                    "glm-5.3-flash requires a named reasoning_effort value",
                    "reasoning_effort",
                ));
            }
        }
    }

    let Some(thinking) = &request.options.thinking else {
        return Ok(());
    };
    let Some(fields) = thinking.as_object() else {
        return Err(ModelProfileError::InvalidRequest {
            message: "thinking must be an object".to_owned(),
            param: Some("thinking".to_owned()),
        });
    };
    if fields
        .keys()
        .any(|field| !matches!(field.as_str(), "type" | "clear_thinking"))
    {
        return Err(unsupported_capability(
            "glm-5.3-flash only supports thinking.type and thinking.clear_thinking",
            "thinking",
        ));
    }
    match fields.get("type").and_then(Value::as_str) {
        None | Some("enabled") => {}
        Some("disabled") => {
            return Err(unsupported_capability(
                "glm-5.3-flash does not support disabling thinking",
                "thinking.type",
            ));
        }
        Some(value) => {
            return Err(unsupported_capability(
                format!("glm-5.3-flash does not support thinking.type `{value}`"),
                "thinking.type",
            ));
        }
    }
    if fields
        .get("clear_thinking")
        .is_some_and(|clear_thinking| !clear_thinking.is_boolean())
    {
        return Err(ModelProfileError::InvalidRequest {
            message: "thinking.clear_thinking must be a boolean".to_owned(),
            param: Some("thinking.clear_thinking".to_owned()),
        });
    }
    Ok(())
}

fn validate_items(
    items: &[UnifiedInputItem],
    modality: &mut Option<InputMediaKind>,
) -> Result<(), ModelProfileError> {
    for item in items {
        match &item.kind {
            UnifiedInputItemKind::Message { role, content, .. } => {
                validate_content_parts(content, modality, *role == UnifiedRole::Assistant)?;
            }
            UnifiedInputItemKind::FunctionCallOutput {
                output: UnifiedToolOutput::Content(content),
                ..
            } => validate_content_parts(content, modality, false)?,
            _ => {}
        }
    }
    Ok(())
}

fn validate_content_parts(
    parts: &[UnifiedContentPart],
    modality: &mut Option<InputMediaKind>,
    skip_assistant_reasoning: bool,
) -> Result<(), ModelProfileError> {
    for part in parts {
        match &part.kind {
            UnifiedContentPartKind::Text { .. } | UnifiedContentPartKind::Refusal { .. } => {}
            UnifiedContentPartKind::Thinking { .. } if skip_assistant_reasoning => {}
            UnifiedContentPartKind::RedactedThinking { .. } if skip_assistant_reasoning => {}
            UnifiedContentPartKind::Thinking { .. } => {}
            UnifiedContentPartKind::Image { source, .. } => {
                register_input_modality(modality, InputMediaKind::Image)?;
                validate_image_source(source)?;
            }
            UnifiedContentPartKind::File {
                source, filename, ..
            } => {
                register_input_modality(modality, InputMediaKind::File)?;
                validate_file_source(source, filename.as_deref())?;
            }
            UnifiedContentPartKind::Document { source, title, .. } => {
                validate_document_source(source, title.as_deref(), modality)?;
            }
            UnifiedContentPartKind::Audio { .. } => {
                return Err(unsupported_capability(
                    "glm-5.3-flash does not support audio input",
                    "messages",
                ));
            }
            UnifiedContentPartKind::Other { part_type, payload } if part_type == "video_url" => {
                register_input_modality(modality, InputMediaKind::Video)?;
                validate_video_url(payload)?;
            }
            UnifiedContentPartKind::Other { part_type, payload } if part_type == "file_url" => {
                register_input_modality(modality, InputMediaKind::File)?;
                validate_file_url_payload(payload)?;
            }
            UnifiedContentPartKind::Other { part_type, .. } => {
                return Err(unsupported_capability(
                    format!("glm-5.3-flash does not support input content type `{part_type}`"),
                    "messages",
                ));
            }
            UnifiedContentPartKind::RedactedThinking { .. } => {
                return Err(ModelProfileError::UnsupportedConversion {
                    message:
                        "provider-specific redacted thinking cannot be converted to glm-5.3-flash"
                            .to_owned(),
                    param: Some("messages".to_owned()),
                });
            }
        }
    }
    Ok(())
}

fn register_input_modality(
    current: &mut Option<InputMediaKind>,
    incoming: InputMediaKind,
) -> Result<(), ModelProfileError> {
    if current.is_some_and(|current| current != incoming) {
        return Err(ModelProfileError::InvalidRequest {
            message: "GLM-5.3-Flash does not allow image, video, and file inputs to be mixed in one request".to_owned(),
            param: Some("messages".to_owned()),
        });
    }
    *current = Some(incoming);
    Ok(())
}

fn validate_document_source(
    source: &UnifiedDocumentSource,
    filename: Option<&str>,
    modality: &mut Option<InputMediaKind>,
) -> Result<(), ModelProfileError> {
    match source {
        UnifiedDocumentSource::Media { source } => {
            register_input_modality(modality, InputMediaKind::File)?;
            validate_file_source(source, filename)
        }
        UnifiedDocumentSource::Text { media_type, .. } => {
            if normalize_mime(media_type).starts_with("text/") {
                Ok(())
            } else {
                Err(unsupported_capability(
                    format!(
                        "glm-5.3-flash does not support inline document MIME type `{media_type}`"
                    ),
                    "messages",
                ))
            }
        }
        UnifiedDocumentSource::Content { content } => {
            validate_content_parts(content, modality, false)
        }
    }
}

fn validate_image_source(source: &UnifiedMediaSource) -> Result<(), ModelProfileError> {
    let mime = media_mime(source);
    if let Some(mime) = mime.as_deref() {
        if SUPPORTED_IMAGE_MIMES.contains(&mime) {
            return Ok(());
        }
        return Err(unsupported_capability(
            format!("glm-5.3-flash does not support image MIME type `{mime}`"),
            "messages",
        ));
    }

    if let Some(extension) = media_extension(source, None) {
        if SUPPORTED_IMAGE_EXTENSIONS.contains(&extension.as_str()) {
            return Ok(());
        }
        return Err(unsupported_capability(
            format!("glm-5.3-flash does not support image extension `.{extension}`"),
            "messages",
        ));
    }

    if matches!(source, UnifiedMediaSource::FileId { .. }) {
        return Err(unsupported_capability(
            "glm-5.3-flash image inputs require an image URL or inline image data",
            "messages",
        ));
    }
    Ok(())
}

fn validate_file_source(
    source: &UnifiedMediaSource,
    filename: Option<&str>,
) -> Result<(), ModelProfileError> {
    let UnifiedMediaSource::Url { url } = source else {
        return Err(unsupported_capability(
            "glm-5.3-flash file inputs require a URL; Base64, Data URI and file IDs are unsupported",
            "messages",
        ));
    };
    validate_file_url(url, filename, None)
}

fn validate_file_url_payload(payload: &Value) -> Result<(), ModelProfileError> {
    let file_url = &payload["file_url"];
    let url = file_url
        .get("url")
        .and_then(Value::as_str)
        .or_else(|| file_url.as_str())
        .or_else(|| payload.get("url").and_then(Value::as_str))
        .filter(|url| !url.trim().is_empty())
        .ok_or_else(|| ModelProfileError::InvalidRequest {
            message: "file_url requires a non-empty URL".to_owned(),
            param: Some("messages".to_owned()),
        })?;
    let mime = file_url
        .get("mime_type")
        .and_then(Value::as_str)
        .or_else(|| payload.get("mime_type").and_then(Value::as_str));
    validate_file_url(url, None, mime)
}

fn validate_file_url(
    url: &str,
    filename: Option<&str>,
    mime: Option<&str>,
) -> Result<(), ModelProfileError> {
    if !url.starts_with("http://") && !url.starts_with("https://") {
        return Err(unsupported_capability(
            "glm-5.3-flash file inputs require a remote URL",
            "messages",
        ));
    }
    if let Some(mime) = mime.map(normalize_mime) {
        if !SUPPORTED_FILE_MIMES.contains(&mime.as_str()) {
            return Err(unsupported_capability(
                format!("glm-5.3-flash does not support file MIME type `{mime}`"),
                "messages",
            ));
        }
    }
    if let Some(extension) = filename
        .and_then(path_extension)
        .or_else(|| path_extension(url))
    {
        if !SUPPORTED_FILE_EXTENSIONS.contains(&extension.as_str()) {
            return Err(unsupported_capability(
                format!("glm-5.3-flash does not support file extension `.{extension}`"),
                "messages",
            ));
        }
    }
    Ok(())
}

fn validate_video_url(payload: &Value) -> Result<(), ModelProfileError> {
    let video = &payload["video_url"];
    let url = video
        .get("url")
        .and_then(Value::as_str)
        .or_else(|| video.as_str())
        .or_else(|| payload.get("url").and_then(Value::as_str))
        .filter(|url| !url.trim().is_empty())
        .ok_or_else(|| ModelProfileError::InvalidRequest {
            message: "video_url requires a non-empty URL".to_owned(),
            param: Some("messages".to_owned()),
        })?;
    if !url.starts_with("http://") && !url.starts_with("https://") {
        return Err(unsupported_capability(
            "glm-5.3-flash video inputs require a remote URL",
            "messages",
        ));
    }
    let mime = video
        .get("mime_type")
        .and_then(Value::as_str)
        .or_else(|| payload.get("mime_type").and_then(Value::as_str))
        .map(normalize_mime);
    if let Some(mime) = mime {
        if SUPPORTED_VIDEO_MIMES.contains(&mime.as_str()) {
            return Ok(());
        }
        return Err(unsupported_capability(
            format!("glm-5.3-flash does not support video MIME type `{mime}`"),
            "messages",
        ));
    }
    if let Some(extension) = path_extension(url) {
        if SUPPORTED_VIDEO_EXTENSIONS.contains(&extension.as_str()) {
            return Ok(());
        }
        return Err(unsupported_capability(
            format!("glm-5.3-flash does not support video extension `.{extension}`"),
            "messages",
        ));
    }
    Ok(())
}

fn media_mime(source: &UnifiedMediaSource) -> Option<String> {
    match source {
        UnifiedMediaSource::Base64 { media_type, .. } => Some(normalize_mime(media_type)),
        UnifiedMediaSource::DataUrl { url } => url
            .strip_prefix("data:")
            .and_then(|data| data.split_once(',').map(|(metadata, _)| metadata))
            .map(|metadata| normalize_mime(metadata.split(';').next().unwrap_or(metadata))),
        UnifiedMediaSource::Url { .. } | UnifiedMediaSource::FileId { .. } => None,
    }
}

fn media_extension(source: &UnifiedMediaSource, filename: Option<&str>) -> Option<String> {
    filename.and_then(path_extension).or_else(|| match source {
        UnifiedMediaSource::Url { url } => path_extension(url),
        UnifiedMediaSource::DataUrl { .. }
        | UnifiedMediaSource::Base64 { .. }
        | UnifiedMediaSource::FileId { .. } => None,
    })
}

fn path_extension(value: &str) -> Option<String> {
    let path = value
        .split(|character| character == '?' || character == '#')
        .next()?;
    let name = path.rsplit('/').next()?;
    name.rsplit_once('.')
        .map(|(_, extension)| extension.to_ascii_lowercase())
        .filter(|extension| !extension.is_empty())
}

fn normalize_mime(value: &str) -> String {
    value
        .split(';')
        .next()
        .unwrap_or(value)
        .trim()
        .to_ascii_lowercase()
}

fn apply_glm_defaults(request: &mut ChatCompletionRequest) {
    request.temperature.get_or_insert(1.0);
    request.top_p.get_or_insert(0.95);
    if request.reasoning_effort.is_none() {
        request.reasoning_effort = Some(ReasoningEffort::from("max"));
    }
    match &mut request.thinking {
        Some(thinking) => {
            thinking.clear_thinking.get_or_insert(false);
        }
        None => {
            request.thinking = Some(ThinkingConfig {
                kind: ThinkingType::Enabled,
                budget_tokens: None,
                clear_thinking: Some(false),
            });
        }
    }
}

fn validate_serialized_request_size(
    request: &ChatCompletionRequest,
) -> Result<(), ModelProfileError> {
    let mut counter = JsonByteCounter::default();
    serde_json::to_writer(&mut counter, request).map_err(|error| ModelProfileError::Internal {
        message: format!("serialize glm-5.3-flash request for size validation: {error}"),
    })?;
    validate_request_body_size(counter.bytes)
}

fn validate_request_body_size(size: usize) -> Result<(), ModelProfileError> {
    if size > MAX_REQUEST_BODY_BYTES {
        return Err(ModelProfileError::InvalidRequest {
            message: format!(
                "glm-5.3-flash request body exceeds the 100 MB limit ({} bytes)",
                size
            ),
            param: Some("messages".to_owned()),
        });
    }
    Ok(())
}

#[derive(Default)]
struct JsonByteCounter {
    bytes: usize,
}

impl Write for JsonByteCounter {
    fn write(&mut self, buffer: &[u8]) -> io::Result<usize> {
        self.bytes = self.bytes.saturating_add(buffer.len());
        Ok(buffer.len())
    }

    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}

fn normalize_items(items: &mut Vec<UnifiedInputItem>) -> Result<(), ModelProfileError> {
    for item in items.iter_mut() {
        match &mut item.kind {
            UnifiedInputItemKind::Message { role, content, .. } => {
                if *role == UnifiedRole::Assistant {
                    content.retain(|part| {
                        !matches!(
                            &part.kind,
                            UnifiedContentPartKind::Thinking { .. }
                                | UnifiedContentPartKind::RedactedThinking { .. }
                        )
                    });
                }
                normalize_content_parts(content)?;
            }
            UnifiedInputItemKind::FunctionCallOutput {
                output: UnifiedToolOutput::Content(content),
                ..
            } => normalize_content_parts(content)?,
            _ => {}
        }
    }
    items.retain(|item| match &item.kind {
        UnifiedInputItemKind::Reasoning { .. } => false,
        UnifiedInputItemKind::Message {
            role: UnifiedRole::Assistant,
            content,
            ..
        } => !content.is_empty(),
        _ => true,
    });
    Ok(())
}

fn normalize_content_parts(parts: &mut Vec<UnifiedContentPart>) -> Result<(), ModelProfileError> {
    let mut normalized = Vec::with_capacity(parts.len());
    for part in std::mem::take(parts) {
        let UnifiedContentPart {
            cache_control,
            source_extensions,
            kind,
        } = part;
        match kind {
            UnifiedContentPartKind::Document {
                source,
                title,
                context,
                citations,
            } => {
                let note = document_note(title.as_deref(), context.as_deref(), citations.as_ref());
                match source {
                    UnifiedDocumentSource::Media { source } => {
                        if let Some(note) = note {
                            normalized.push(UnifiedContentPart::text(note));
                        }
                        let UnifiedMediaSource::Url { url } = source else {
                            return Err(unsupported_capability(
                                "glm-5.3-flash file inputs require a URL",
                                "messages",
                            ));
                        };
                        normalized.push(file_url_part(url, cache_control, source_extensions));
                    }
                    UnifiedDocumentSource::Text { text, .. } => {
                        let text = match note {
                            Some(note) => format!("{note}\n{text}"),
                            None => text,
                        };
                        normalized.push(UnifiedContentPart {
                            cache_control,
                            source_extensions,
                            kind: UnifiedContentPartKind::Text {
                                text,
                                annotations: Vec::new(),
                                citations: None,
                                logprobs: None,
                            },
                        });
                    }
                    UnifiedDocumentSource::Content { mut content } => {
                        if let Some(note) = note {
                            normalized.push(UnifiedContentPart::text(note));
                        }
                        normalize_content_parts(&mut content)?;
                        normalized.extend(content);
                    }
                }
            }
            UnifiedContentPartKind::File {
                source: UnifiedMediaSource::Url { url },
                ..
            } => normalized.push(file_url_part(url, cache_control, source_extensions)),
            kind => normalized.push(UnifiedContentPart {
                cache_control,
                source_extensions,
                kind,
            }),
        }
    }
    *parts = normalized;
    Ok(())
}

fn file_url_part(
    url: String,
    cache_control: Option<crate::provider::UnifiedCacheControl>,
    source_extensions: Option<crate::provider::UnifiedSourceExtensions>,
) -> UnifiedContentPart {
    UnifiedContentPart {
        cache_control,
        source_extensions,
        kind: UnifiedContentPartKind::Other {
            part_type: "file_url".to_owned(),
            payload: json!({"type":"file_url", "file_url":{"url":url}}),
        },
    }
}

fn document_note(
    title: Option<&str>,
    context: Option<&str>,
    citations: Option<&Value>,
) -> Option<String> {
    let mut note = Vec::new();
    if let Some(title) = title.filter(|title| !title.trim().is_empty()) {
        note.push(format!("Document title: {title}"));
    }
    if let Some(context) = context.filter(|context| !context.trim().is_empty()) {
        note.push(format!("Document context: {context}"));
    }
    if let Some(citations) = citations.filter(|citations| !citations.is_null()) {
        note.push(format!("Document citations: {citations}"));
    }
    (!note.is_empty()).then(|| note.join("\n"))
}

fn unsupported_capability(message: impl Into<String>, param: &str) -> ModelProfileError {
    ModelProfileError::UnsupportedCapability {
        message: message.into(),
        param: Some(param.to_owned()),
    }
}

#[cfg(test)]
mod tests {
    use serde_json::{Value, json};

    use super::{Glm53FlashProfile, MAX_REQUEST_BODY_BYTES, validate_request_body_size};
    use crate::api;
    use crate::dialects::Dialect;
    use crate::models::{ModelProfile, ModelProfileError, ModelProfileRegistry};
    use crate::openai_types::{Content, ContentPart};
    use crate::provider::{ReasoningEffort, UnifiedSourceProtocol};
    use crate::serve_config::ModelBinding;
    use crate::upstreams::Upstream;

    fn binding(upstream: Upstream) -> ModelBinding {
        ModelBinding {
            public_model: "glm-5.3-flash".to_owned(),
            model: "glm-5.3-flash".to_owned(),
            provider: "qcloud.cn".to_owned(),
            upstream,
            upstream_model: "glm-5.3-flash".to_owned(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "qcloud.cn.glm-5.3-flash".to_owned(),
        }
    }

    fn chat_request(value: &Value) -> crate::provider::UnifiedRequest {
        api::request_from_value(value, UnifiedSourceProtocol::ChatCompletions)
            .expect("parse Chat Completions request")
    }

    fn convert(
        value: &Value,
    ) -> Result<Option<crate::openai_types::ChatCompletionRequest>, ModelProfileError> {
        let request = chat_request(value);
        Glm53FlashProfile.to_openai_chat_request(&request, &binding(Upstream::Tokenhub))
    }

    #[test]
    fn builtin_registry_selects_profile_by_exact_public_model_and_tokenhub_binding() {
        let registry = ModelProfileRegistry::default();
        let request = chat_request(&json!({
            "model":"glm-5.3-flash",
            "messages":[{"role":"user", "content":"hello"}]
        }));

        let tokenhub = registry
            .resolve("glm-5.3-flash")
            .to_openai_chat_request(&request, &binding(Upstream::Tokenhub))
            .expect("TokenHub profile converts");
        assert!(tokenhub.is_some());

        let ark = Glm53FlashProfile
            .to_openai_chat_request(&request, &binding(Upstream::VolcengineArk))
            .expect("Ark binding uses default conversion");
        assert!(ark.is_none());
    }

    #[test]
    fn injects_glm_defaults_and_preserves_supported_sampling_tools_and_extensions() {
        let request = json!({
            "model":"glm-5.3-flash",
            "messages":[{"role":"user", "content":"hello"}],
            "tools":[{"type":"function", "function":{"name":"lookup", "parameters":{"type":"object"}}}],
            "tool_stream":true
        });
        let converted = convert(&request)
            .expect("convert request")
            .expect("TokenHub profile handles request");

        assert_eq!(converted.temperature, Some(1.0));
        assert_eq!(converted.top_p, Some(0.95));
        assert_eq!(
            converted
                .reasoning_effort
                .as_ref()
                .and_then(ReasoningEffort::as_str),
            Some("max")
        );
        assert_eq!(
            converted
                .thinking
                .as_ref()
                .map(|thinking| thinking.kind.clone()),
            Some(crate::openai_types::ThinkingType::Enabled)
        );
        assert_eq!(
            converted
                .thinking
                .as_ref()
                .and_then(|thinking| thinking.clear_thinking),
            Some(false)
        );
        assert_eq!(converted.tools.as_ref().map(Vec::len), Some(1));
        assert_eq!(converted.extra.get("tool_stream"), Some(&json!(true)));
    }

    #[test]
    fn preserves_supported_top_k_sampling_and_reasoning_effort() {
        let request = json!({
            "model":"glm-5.3-flash",
            "messages":[{"role":"user", "content":"hello"}],
            "temperature":0.8,
            "top_p":0.9,
            "top_k":40,
            "reasoning_effort":"low"
        });
        let converted = convert(&request)
            .expect("convert request")
            .expect("TokenHub profile handles request");
        assert_eq!(converted.temperature, Some(0.8));
        assert_eq!(converted.top_p, Some(0.9));
        assert_eq!(converted.extra.get("top_k"), Some(&json!(40)));
        assert_eq!(
            converted
                .reasoning_effort
                .as_ref()
                .and_then(ReasoningEffort::as_str),
            Some("low")
        );
    }

    #[test]
    fn rejects_disabled_thinking_and_efforts_outside_glm_values() {
        for value in [
            json!({"model":"glm-5.3-flash", "messages":[{"role":"user", "content":"hi"}], "thinking":{"type":"disabled"}}),
            json!({"model":"glm-5.3-flash", "messages":[{"role":"user", "content":"hi"}], "reasoning_effort":"medium"}),
            json!({"model":"glm-5.3-flash", "messages":[{"role":"user", "content":"hi"}], "reasoning_effort":"none"}),
        ] {
            assert!(matches!(
                convert(&value),
                Err(ModelProfileError::UnsupportedCapability { .. })
            ));
        }
    }

    #[test]
    fn removes_prior_assistant_reasoning_and_reasoning_items() {
        let request = api::request_from_value(
            &json!({
                "model":"glm-5.3-flash",
                "input":[
                    {"type":"reasoning", "id":"rs_1", "summary":[{"type":"summary_text", "text":"private"}], "content":[{"type":"reasoning_text", "text":"private"}]},
                    {"type":"message", "role":"assistant", "content":[{"type":"reasoning_text", "text":"private"}, {"type":"output_text", "text":"previous answer"}]},
                    {"type":"message", "role":"user", "content":[{"type":"input_text", "text":"continue"}]}
                ]
            }),
            UnifiedSourceProtocol::Responses,
        )
        .expect("parse Responses history");
        let converted = Glm53FlashProfile
            .to_openai_chat_request(&request, &binding(Upstream::Tokenhub))
            .expect("convert history")
            .expect("profile handles request");

        assert_eq!(converted.messages.len(), 2);
        let Content::Parts(parts) = &converted.messages[0].content else {
            panic!("expected assistant message parts");
        };
        assert_eq!(parts.len(), 1);
        assert!(matches!(&parts[0], ContentPart::Text { text } if text == "previous answer"));
    }

    #[test]
    fn preserves_glm_image_video_and_file_url_blocks() {
        for content in [
            json!({"type":"image_url", "image_url":{"url":"data:image/webp;base64,aGVsbG8="}}),
            json!({"type":"video_url", "video_url":{"url":"https://example.com/clip.webm"}}),
            json!({"type":"file_url", "file_url":{"url":"https://example.com/report.pdf"}}),
        ] {
            let request = json!({
                "model":"glm-5.3-flash",
                "messages":[{"role":"user", "content":[content]}]
            });
            let converted = convert(&request)
                .expect("convert multimodal request")
                .expect("profile handles request");
            assert!(matches!(converted.messages[0].content, Content::Parts(_)));
        }
    }

    #[test]
    fn rejects_mixed_modalities_base64_files_and_audio() {
        for value in [
            json!({
                "model":"glm-5.3-flash",
                "messages":[{"role":"user", "content":[
                    {"type":"image_url", "image_url":{"url":"https://example.com/a.png"}},
                    {"type":"video_url", "video_url":{"url":"https://example.com/a.mp4"}}
                ]}]
            }),
            json!({"model":"glm-5.3-flash", "messages":[{"role":"user", "content":[{"type":"file", "file":{"file_data":"data:application/pdf;base64,JVBERi0x", "mime_type":"application/pdf"}}]}]}),
            json!({"model":"glm-5.3-flash", "messages":[{"role":"user", "content":[{"type":"input_audio", "input_audio":{"data":"aGVsbG8=", "format":"wav"}}]}]}),
        ] {
            assert!(matches!(
                convert(&value),
                Err(ModelProfileError::InvalidRequest { .. })
                    | Err(ModelProfileError::UnsupportedCapability { .. })
            ));
        }
    }

    #[test]
    fn enforces_output_and_body_limits_but_leaves_context_to_upstream() {
        let request = json!({
            "model":"glm-5.3-flash",
            "messages":[{"role":"user", "content":"hello"}],
            "max_tokens":131_072
        });
        let converted = convert(&request)
            .expect("maximum output accepted")
            .expect("profile handles request");
        assert_eq!(converted.max_completion_tokens, Some(131_072));

        let too_many_output_tokens = json!({
            "model":"glm-5.3-flash",
            "messages":[{"role":"user", "content":"hello"}],
            "max_tokens":131_073
        });
        assert!(matches!(
            convert(&too_many_output_tokens),
            Err(ModelProfileError::InvalidRequest { .. })
        ));

        assert!(validate_request_body_size(MAX_REQUEST_BODY_BYTES).is_ok());
        assert!(matches!(
            validate_request_body_size(MAX_REQUEST_BODY_BYTES + 1),
            Err(ModelProfileError::InvalidRequest { .. })
        ));
    }
}
