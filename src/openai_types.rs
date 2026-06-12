use serde::de::Error as DeError;
use serde::{Deserialize, Deserializer, Serialize};
use serde_json::Value;
use std::collections::HashMap;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Usage {
    pub prompt_tokens: i64,
    pub completion_tokens: i64,
    pub total_tokens: i64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub prompt_tokens_details: Option<PromptTokensDetails>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub completion_tokens_details: Option<CompletionTokensDetails>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct PromptTokensDetails {
    #[serde(default)]
    pub audio_tokens: i64,
    #[serde(default)]
    pub cached_tokens: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct CompletionTokensDetails {
    #[serde(default)]
    pub accepted_prediction_tokens: i64,
    #[serde(default)]
    pub audio_tokens: i64,
    #[serde(default)]
    pub reasoning_tokens: i64,
    #[serde(default)]
    pub rejected_prediction_tokens: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChatCompletionRequest {
    #[serde(default)]
    pub model: String,
    pub messages: Vec<Message>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub n: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub top_p: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub presence_penalty: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub frequency_penalty: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub logprobs: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub top_logprobs: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub modalities: Option<Vec<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub audio: Option<AudioOutput>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_completion_tokens: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stop: Option<Vec<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub response_format: Option<ResponseFormat>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thinking: Option<ThinkingConfig>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reasoning_effort: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub chat_template_kwargs: Option<HashMap<String, Value>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub prediction: Option<Prediction>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub verbosity: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tools: Option<Vec<ToolDefinition>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_choice: Option<ToolChoice>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub allowed_tools: Option<AllowedTools>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub parallel_tool_calls: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub service_tier: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub seed: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stream: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stream_options: Option<StreamOptions>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub metadata: Option<HashMap<String, String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub agent_context: Option<Value>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ThinkingConfig {
    /// TokenHub deep thinking parameter.
    ///
    /// References:
    /// - https://cloud.tencent.com/document/product/1823/131208
    /// - https://cloud.tencent.com/document/product/1823/130079
    #[serde(rename = "type")]
    pub kind: ThinkingType,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum ThinkingType {
    /// Always enable provider-side reasoning.
    Enabled,
    /// Let provider adaptively decide whether to reason.
    Adaptive,
    /// Disable provider-side reasoning.
    Disabled,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum ResponseFormat {
    Text,
    JsonObject,
    JsonSchema { json_schema: JsonSchema },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct JsonSchema {
    pub name: String,
    pub schema: Value,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strict: Option<bool>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StreamOptions {
    pub include_usage: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub include_obfuscation: Option<bool>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum Role {
    System,
    Developer,
    User,
    Assistant,
    Tool,
}

impl Role {
    pub fn as_str(&self) -> &'static str {
        match self {
            Role::System => "system",
            Role::Developer => "developer",
            Role::User => "user",
            Role::Assistant => "assistant",
            Role::Tool => "tool",
        }
    }
}

#[derive(Debug, Clone, Serialize)]
pub struct Message {
    pub role: Role,
    #[serde(default, deserialize_with = "null_to_default")]
    pub content: Content,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reasoning_content: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_call_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_calls: Option<Vec<ToolCall>>,
}

impl<'de> Deserialize<'de> for Message {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        #[derive(Deserialize)]
        struct RawMessage {
            role: Role,
            #[serde(default, deserialize_with = "null_to_default")]
            content: Content,
            #[serde(skip_serializing_if = "Option::is_none")]
            reasoning_content: Option<String>,
            #[serde(skip_serializing_if = "Option::is_none")]
            tool_call_id: Option<String>,
            #[serde(skip_serializing_if = "Option::is_none")]
            tool_calls: Option<Vec<ToolCall>>,
        }

        let raw = RawMessage::deserialize(deserializer)?;
        if raw.role == Role::Tool && raw.tool_call_id.as_deref().unwrap_or("").trim().is_empty() {
            return Err(D::Error::custom(
                "tool_call_id is required for tool messages",
            ));
        }

        Ok(Message {
            role: raw.role,
            content: raw.content,
            reasoning_content: raw.reasoning_content,
            tool_call_id: raw.tool_call_id,
            tool_calls: raw.tool_calls,
        })
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum Content {
    Text(String),
    Parts(Vec<ContentPart>),
}

impl Default for Content {
    fn default() -> Self {
        Content::Text(String::new())
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum ContentPart {
    Text {
        text: String,
    },
    #[serde(rename = "image_url")]
    Image {
        image_url: ImageUrl,
    },
    InputAudio {
        input_audio: InputAudio,
    },
    File {
        #[serde(default)]
        file: FileContent,
        #[serde(default, skip_serializing)]
        file_id: Option<String>,
        #[serde(default, skip_serializing)]
        file_data: Option<String>,
        #[serde(default, skip_serializing)]
        filename: Option<String>,
        #[serde(default, skip_serializing)]
        file_url: Option<String>,
        #[serde(default, skip_serializing)]
        mime_type: Option<String>,
    },
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct FileContent {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub file_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub file_data: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub filename: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub file_url: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub mime_type: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ImageUrl {
    pub url: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub detail: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct InputAudio {
    pub data: String,
    pub format: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AudioOutput {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub voice: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub format: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Prediction {
    pub content: Vec<ContentPart>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolDefinition {
    pub r#type: String,
    pub function: FunctionDefinition,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AllowedTools {
    pub mode: String,
    pub allow: Vec<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FunctionDefinition {
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strict: Option<bool>,
    pub parameters: Value,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum ToolChoice {
    Name(String),
    Object {
        r#type: String,
        function: ToolChoiceFunction,
    },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolChoiceFunction {
    pub name: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChatCompletionResponse {
    pub id: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub created: Option<i64>,
    pub model: String,
    pub object: String,
    pub system_fingerprint: Option<String>,
    pub choices: Vec<ChatCompletionChoice>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub usage: Option<Usage>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChatCompletionChoice {
    pub message: ChatCompletionMessage,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub finish_reason: Option<String>,
    pub index: i32,
    pub logprobs: Option<Value>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChatCompletionMessage {
    pub role: Role,
    pub content: Option<Content>,
    #[serde(default, deserialize_with = "null_to_default")]
    pub annotations: Vec<Value>,
    pub refusal: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_calls: Option<Vec<ToolCall>>,
}

fn null_to_default<'de, D, T>(deserializer: D) -> Result<T, D::Error>
where
    D: Deserializer<'de>,
    T: Deserialize<'de> + Default,
{
    Ok(Option::<T>::deserialize(deserializer)?.unwrap_or_default())
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolCall {
    pub id: Option<String>,
    pub r#type: Option<String>,
    pub function: ToolCallFunction,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolCallFunction {
    pub name: String,
    pub arguments: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChatCompletionChunk {
    pub id: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub created: Option<i64>,
    pub model: String,
    pub object: String,
    pub system_fingerprint: Option<String>,
    pub obfuscation: Option<String>,
    pub choices: Vec<ChatCompletionChunkChoice>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub usage: Option<Usage>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChatCompletionChunkChoice {
    pub delta: ChatCompletionChunkDelta,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub finish_reason: Option<String>,
    pub index: i32,
    pub logprobs: Option<Value>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChatCompletionChunkDelta {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub role: Option<Role>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub content: Option<String>,
    pub refusal: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_calls: Option<Vec<ChatCompletionChunkToolCall>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChatCompletionChunkToolCall {
    pub index: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub function: Option<ChatCompletionChunkToolCallFunction>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChatCompletionChunkToolCallFunction {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub arguments: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ErrorResponse {
    pub error: ErrorDetail,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ErrorDetail {
    pub message: String,
    #[serde(rename = "type")]
    pub r#type: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub code: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub param: Option<String>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn chat_completion_message_allows_null_annotations() {
        let message = r#"{"role":"assistant","content":null,"annotations":null,"refusal":null,"tool_calls":null}"#;
        let parsed: ChatCompletionMessage = serde_json::from_str(message).expect("parse message");
        assert!(parsed.annotations.is_empty());
    }

    #[test]
    fn chat_completion_message_defaults_missing_annotations() {
        let message = r#"{"role":"assistant","content":null,"refusal":null,"tool_calls":null}"#;
        let parsed: ChatCompletionMessage = serde_json::from_str(message).expect("parse message");
        assert!(parsed.annotations.is_empty());
    }

    #[test]
    fn message_allows_null_content() {
        let message = r#"{"role":"assistant","content":null,"tool_calls":[]}"#;
        let parsed: Message = serde_json::from_str(message).expect("parse message");
        assert!(matches!(parsed.content, Content::Text(text) if text.is_empty()));
    }

    #[test]
    fn message_defaults_missing_content() {
        let message = r#"{"role":"assistant"}"#;
        let parsed: Message = serde_json::from_str(message).expect("parse message");
        assert!(matches!(parsed.content, Content::Text(text) if text.is_empty()));
    }

    #[test]
    fn tool_message_requires_tool_call_id() {
        let message = r#"{"role":"tool","content":"done"}"#;
        let err = serde_json::from_str::<Message>(message).expect_err("should reject missing id");
        assert!(
            err.to_string()
                .contains("tool_call_id is required for tool messages")
        );
    }

    #[test]
    fn tool_message_rejects_blank_tool_call_id() {
        let message = r#"{"role":"tool","content":"done","tool_call_id":"   "}"#;
        let err = serde_json::from_str::<Message>(message).expect_err("should reject blank id");
        assert!(
            err.to_string()
                .contains("tool_call_id is required for tool messages")
        );
    }

    #[test]
    fn tool_message_accepts_tool_call_id() {
        let message = r#"{"role":"tool","content":"done","tool_call_id":"call_123"}"#;
        let parsed: Message = serde_json::from_str(message).expect("parse tool message");
        assert_eq!(parsed.tool_call_id.as_deref(), Some("call_123"));
    }

    #[test]
    fn non_tool_message_allows_missing_tool_call_id() {
        let message = r#"{"role":"assistant","content":"ok"}"#;
        let parsed: Message = serde_json::from_str(message).expect("parse assistant message");
        assert!(parsed.tool_call_id.is_none());
    }

    #[test]
    fn message_content_parts_accept_image_url_type() {
        let message = r#"{
            "role":"user",
            "content":[
                {"type":"text","text":"Read this image"},
                {"type":"image_url","image_url":{"url":"data:image/png;base64,aGVsbG8="}}
            ]
        }"#;

        let parsed: Message = serde_json::from_str(message).expect("parse multimodal message");
        let Content::Parts(parts) = parsed.content else {
            panic!("expected content parts");
        };
        assert_eq!(parts.len(), 2);
        assert!(matches!(parts[0], ContentPart::Text { .. }));
        assert!(matches!(parts[1], ContentPart::Image { .. }));
    }

    #[test]
    fn message_content_parts_accept_nested_file_object() {
        let message = r#"{
            "role":"user",
            "content":[
                {"type":"file","file":{"file_id":"file-123","filename":"doc.txt"}}
            ]
        }"#;

        let parsed: Message = serde_json::from_str(message).expect("parse file message");
        let Content::Parts(parts) = parsed.content else {
            panic!("expected content parts");
        };
        let ContentPart::File { file, .. } = &parts[0] else {
            panic!("expected file content part");
        };
        assert_eq!(file.file_id.as_deref(), Some("file-123"));
        assert_eq!(file.filename.as_deref(), Some("doc.txt"));
    }

    #[test]
    fn message_content_parts_accept_legacy_flat_file_fields() {
        let message = r#"{
            "role":"user",
            "content":[
                {"type":"file","file_id":"file-legacy","filename":"legacy.txt"}
            ]
        }"#;

        let parsed: Message = serde_json::from_str(message).expect("parse legacy file message");
        let Content::Parts(parts) = parsed.content else {
            panic!("expected content parts");
        };
        let ContentPart::File {
            file,
            file_id,
            filename,
            ..
        } = &parts[0]
        else {
            panic!("expected file content part");
        };
        assert!(file.file_id.is_none());
        assert_eq!(file_id.as_deref(), Some("file-legacy"));
        assert_eq!(filename.as_deref(), Some("legacy.txt"));
    }
}
