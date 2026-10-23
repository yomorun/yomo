use serde::de::Error as DeError;
use serde::{Deserialize, Deserializer, Serialize};
use serde_json::Value;
use std::collections::HashMap;

use crate::provider::ReasoningEffort;

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

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
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
    #[serde(
        default,
        deserialize_with = "string_or_vec",
        skip_serializing_if = "Option::is_none"
    )]
    pub stop: Option<Vec<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub response_format: Option<ResponseFormat>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thinking: Option<ThinkingConfig>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reasoning_effort: Option<ReasoningEffort>,
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
    /// Fidelity channel for unmodeled top-level request fields, retained when
    /// encoding requests in the same API format.
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
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
    /// Optional maximum thinking-token budget supported by some providers.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub budget_tokens: Option<u32>,
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

/// IR reasoning block: reasoning content passed across protocols.
///
/// Carries responses reasoning items (summary + encrypted_content) and
/// anthropic thinking / redacted_thinking blocks; encrypted payloads must pass through unchanged.
#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub struct ReasoningBlock {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub id: Option<String>,
    /// responses summary parts, passed through unchanged.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub summary: Option<Vec<Value>>,
    /// anthropic thinking text.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub thinking: Option<String>,
    /// anthropic thinking signature (must be returned unchanged).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub signature: Option<String>,
    /// Encrypted reasoning payload (responses encrypted_content / anthropic redacted_thinking data).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub encrypted_content: Option<String>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

#[derive(Debug, Clone, Serialize)]
pub struct Message {
    pub role: Role,
    #[serde(default, deserialize_with = "null_to_default")]
    pub content: Content,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reasoning_content: Option<String>,
    /// IR extension: structured reasoning block (encrypted passthrough item).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub reasoning_blocks: Option<Vec<ReasoningBlock>>,
    /// IR extension: cache hint (semantic marker equivalent to anthropic cache_control).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cache_control: Option<Value>,
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
            reasoning_content: Option<String>,
            reasoning: Option<String>,
            reasoning_blocks: Option<Vec<ReasoningBlock>>,
            cache_control: Option<Value>,
            tool_call_id: Option<String>,
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
            reasoning_content: raw.reasoning_content.or(raw.reasoning),
            reasoning_blocks: raw.reasoning_blocks,
            cache_control: raw.cache_control,
            tool_call_id: raw.tool_call_id,
            tool_calls: raw.tool_calls,
        })
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
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

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
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

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
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

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct ImageUrl {
    pub url: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub detail: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
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
    /// IR extension: fidelity channel for non-function tools (e.g. codex local_shell) raw fields.
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
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

#[derive(Debug, Clone, Serialize)]
pub struct ChatCompletionMessage {
    pub role: Role,
    pub content: Option<Content>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reasoning_content: Option<String>,
    #[serde(default, deserialize_with = "null_to_default")]
    pub annotations: Vec<Value>,
    pub refusal: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_calls: Option<Vec<ToolCall>>,
}

impl<'de> Deserialize<'de> for ChatCompletionMessage {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        #[derive(Deserialize)]
        struct RawChatCompletionMessage {
            role: Role,
            content: Option<Content>,
            reasoning_content: Option<String>,
            reasoning: Option<String>,
            #[serde(default, deserialize_with = "null_to_default")]
            annotations: Vec<Value>,
            refusal: Option<String>,
            tool_calls: Option<Vec<ToolCall>>,
        }

        let raw = RawChatCompletionMessage::deserialize(deserializer)?;
        Ok(ChatCompletionMessage {
            role: raw.role,
            content: raw.content,
            reasoning_content: raw.reasoning_content.or(raw.reasoning),
            annotations: raw.annotations,
            refusal: raw.refusal,
            tool_calls: raw.tool_calls,
        })
    }
}

fn null_to_default<'de, D, T>(deserializer: D) -> Result<T, D::Error>
where
    D: Deserializer<'de>,
    T: Deserialize<'de> + Default,
{
    Ok(Option::<T>::deserialize(deserializer)?.unwrap_or_default())
}

/// Accepts OpenAI's `string | string[] | null` shape and normalizes it to a list.
fn string_or_vec<'de, D>(deserializer: D) -> Result<Option<Vec<String>>, D::Error>
where
    D: Deserializer<'de>,
{
    #[derive(Deserialize)]
    #[serde(untagged)]
    enum StringOrVec {
        One(String),
        Many(Vec<String>),
    }

    Ok(
        Option::<StringOrVec>::deserialize(deserializer)?.map(|value| match value {
            StringOrVec::One(value) => vec![value],
            StringOrVec::Many(values) => values,
        }),
    )
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

#[derive(Debug, Clone, Serialize)]
pub struct ChatCompletionChunkDelta {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub role: Option<Role>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub content: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reasoning_content: Option<String>,
    pub refusal: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_calls: Option<Vec<ChatCompletionChunkToolCall>>,
}

impl<'de> Deserialize<'de> for ChatCompletionChunkDelta {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        #[derive(Deserialize)]
        struct RawChatCompletionChunkDelta {
            role: Option<Role>,
            content: Option<String>,
            reasoning_content: Option<String>,
            reasoning: Option<String>,
            refusal: Option<String>,
            tool_calls: Option<Vec<ChatCompletionChunkToolCall>>,
        }

        let raw = RawChatCompletionChunkDelta::deserialize(deserializer)?;
        Ok(ChatCompletionChunkDelta {
            role: raw.role,
            content: raw.content,
            reasoning_content: raw.reasoning_content.or(raw.reasoning),
            refusal: raw.refusal,
            tool_calls: raw.tool_calls,
        })
    }
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

impl ChatCompletionRequest {
    /// Normalize cross-vendor reasoning/thinking request fields into IR modeled fields.
    ///
    /// Top-level `reasoning_effort` takes priority, followed by top-level
    /// `thinking`, then `chat_template_kwargs`.
    pub fn normalize_reasoning_compat_fields(&mut self) {
        if self.thinking.is_none()
            && let Some(kind) = self
                .extra
                .get("thinking")
                .and_then(parse_thinking_type_from_value)
        {
            self.thinking = Some(ThinkingConfig {
                kind,
                budget_tokens: self
                    .extra
                    .get("thinking")
                    .and_then(|thinking| thinking.get("budget_tokens"))
                    .and_then(Value::as_u64)
                    .and_then(|budget| u32::try_from(budget).ok()),
            });
        }

        if self.reasoning_effort.is_none() {
            self.reasoning_effort = self
                .extra
                .get("reasoning_effort")
                .and_then(parse_effort)
                .or_else(|| {
                    self.extra
                        .get("reasoning")
                        .and_then(extract_reasoning_effort)
                });
        }

        let thinking = self
            .thinking
            .as_ref()
            .and_then(|thinking| serde_json::to_value(thinking).ok());
        let mut options = crate::provider::UnifiedRequestOptions {
            reasoning_effort: self.reasoning_effort.clone(),
            thinking,
            chat_template_kwargs: self.chat_template_kwargs.clone(),
            ..Default::default()
        };
        options.normalize_reasoning_controls();
        self.reasoning_effort = options.reasoning_effort;
        self.thinking = options
            .thinking
            .and_then(|thinking| serde_json::from_value(thinking).ok());
        self.chat_template_kwargs = options.chat_template_kwargs;
    }
}

fn parse_effort(value: &Value) -> Option<ReasoningEffort> {
    match value {
        Value::String(value) if !value.trim().is_empty() => Some(value.as_str().into()),
        Value::Number(_) => serde_json::from_value(value.clone()).ok(),
        _ => None,
    }
}

fn extract_reasoning_effort(value: &Value) -> Option<ReasoningEffort> {
    value
        .as_object()
        .and_then(|object| object.get("effort"))
        .and_then(parse_effort)
}

fn parse_thinking_type_from_value(value: &Value) -> Option<ThinkingType> {
    let raw = match value {
        Value::String(kind) => kind.as_str(),
        Value::Object(object) => object.get("type")?.as_str()?,
        _ => return None,
    };
    match raw.trim().to_ascii_lowercase().as_str() {
        "enabled" => Some(ThinkingType::Enabled),
        "adaptive" => Some(ThinkingType::Adaptive),
        "disabled" => Some(ThinkingType::Disabled),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn parse_stop(stop: &str) -> Option<Vec<String>> {
        let body = format!(r#"{{"model":"m","messages":[]{stop}}}"#);
        serde_json::from_str::<ChatCompletionRequest>(&body)
            .expect("parse request")
            .stop
    }

    #[test]
    fn chat_completion_request_stop_accepts_string_array_null_and_missing() {
        assert_eq!(
            parse_stop(r#","stop":"END""#),
            Some(vec!["END".to_string()])
        );
        assert_eq!(
            parse_stop(r#","stop":["END","STOP"]"#),
            Some(vec!["END".to_string(), "STOP".to_string()])
        );
        assert_eq!(parse_stop(r#","stop":null"#), None);
        assert_eq!(parse_stop(""), None);
    }

    #[test]
    fn chat_completion_request_stop_rejects_non_string_values() {
        let body = r#"{"model":"m","messages":[],"stop":1}"#;

        assert!(serde_json::from_str::<ChatCompletionRequest>(body).is_err());
    }

    #[test]
    fn chat_completion_request_serializes_string_stop_as_array() {
        let request: ChatCompletionRequest =
            serde_json::from_str(r#"{"model":"m","messages":[],"stop":"END"}"#)
                .expect("parse request");

        let value = serde_json::to_value(&request).expect("serialize request");

        assert_eq!(value["stop"], serde_json::json!(["END"]));
    }

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
    fn chat_completion_message_deserializes_reasoning_content() {
        let message = r#"{"role":"assistant","content":"ok","reasoning_content":"think"}"#;
        let parsed: ChatCompletionMessage = serde_json::from_str(message).expect("parse message");
        assert_eq!(parsed.reasoning_content.as_deref(), Some("think"));
    }

    #[test]
    fn message_deserializes_reasoning_alias() {
        let message = r#"{"role":"assistant","content":"ok","reasoning":"think"}"#;
        let parsed: Message = serde_json::from_str(message).expect("parse message");
        assert_eq!(parsed.reasoning_content.as_deref(), Some("think"));
    }

    #[test]
    fn message_deserializes_reasoning_and_reasoning_content_when_both_present() {
        let message = r#"{"role":"assistant","content":"ok","reasoning_content":"think","reasoning":"think"}"#;
        let parsed: Message = serde_json::from_str(message).expect("parse message");
        assert_eq!(parsed.reasoning_content.as_deref(), Some("think"));
    }

    #[test]
    fn message_prefers_reasoning_content_over_reasoning_when_both_present() {
        let message = r#"{"role":"assistant","content":"ok","reasoning_content":"primary","reasoning":"alias"}"#;
        let parsed: Message = serde_json::from_str(message).expect("parse message");
        assert_eq!(parsed.reasoning_content.as_deref(), Some("primary"));
    }

    #[test]
    fn chat_completion_message_deserializes_reasoning_alias() {
        let message = r#"{"role":"assistant","content":"ok","reasoning":"think"}"#;
        let parsed: ChatCompletionMessage = serde_json::from_str(message).expect("parse message");
        assert_eq!(parsed.reasoning_content.as_deref(), Some("think"));
    }

    #[test]
    fn chat_completion_message_deserializes_reasoning_and_reasoning_content_when_both_present() {
        let message = r#"{"role":"assistant","content":"ok","reasoning_content":"think","reasoning":"think"}"#;
        let parsed: ChatCompletionMessage = serde_json::from_str(message).expect("parse message");
        assert_eq!(parsed.reasoning_content.as_deref(), Some("think"));
    }

    #[test]
    fn chat_completion_message_prefers_reasoning_content_over_reasoning_when_both_present() {
        let message = r#"{"role":"assistant","content":"ok","reasoning_content":"primary","reasoning":"alias"}"#;
        let parsed: ChatCompletionMessage = serde_json::from_str(message).expect("parse message");
        assert_eq!(parsed.reasoning_content.as_deref(), Some("primary"));
    }

    #[test]
    fn chat_completion_chunk_deserializes_reasoning_alias() {
        let chunk = r#"{
            "id":"chunk-1",
            "created":1,
            "model":"DeepSeek-V4-Flash",
            "object":"chat.completion.chunk",
            "system_fingerprint":null,
            "obfuscation":null,
            "choices":[{
                "delta":{"reasoning":"step"},
                "finish_reason":null,
                "index":0,
                "logprobs":null
            }],
            "usage":null
        }"#;

        let parsed: ChatCompletionChunk = serde_json::from_str(chunk).expect("parse chunk");
        assert_eq!(
            parsed.choices[0].delta.reasoning_content.as_deref(),
            Some("step")
        );
    }

    #[test]
    fn chat_completion_chunk_deserializes_reasoning_and_reasoning_content_when_both_present() {
        let chunk = r#"{
            "id":"chunk-1",
            "created":1,
            "model":"DeepSeek-V4-Flash",
            "object":"chat.completion.chunk",
            "system_fingerprint":null,
            "obfuscation":null,
            "choices":[{
                "delta":{"reasoning_content":"step","reasoning":"step"},
                "finish_reason":null,
                "index":0,
                "logprobs":null
            }],
            "usage":null
        }"#;

        let parsed: ChatCompletionChunk = serde_json::from_str(chunk).expect("parse chunk");
        assert_eq!(
            parsed.choices[0].delta.reasoning_content.as_deref(),
            Some("step")
        );
    }

    #[test]
    fn chat_completion_chunk_prefers_reasoning_content_over_reasoning_when_both_present() {
        let chunk = r#"{
            "id":"chunk-1",
            "created":1,
            "model":"DeepSeek-V4-Flash",
            "object":"chat.completion.chunk",
            "system_fingerprint":null,
            "obfuscation":null,
            "choices":[{
                "delta":{"reasoning_content":"primary","reasoning":"alias"},
                "finish_reason":null,
                "index":0,
                "logprobs":null
            }],
            "usage":null
        }"#;

        let parsed: ChatCompletionChunk = serde_json::from_str(chunk).expect("parse chunk");
        assert_eq!(
            parsed.choices[0].delta.reasoning_content.as_deref(),
            Some("primary")
        );
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

    #[test]
    fn normalize_reasoning_compat_uses_extra_fields_when_modeled_fields_are_missing() {
        let mut request = ChatCompletionRequest {
            model: "glm-5.3-flash".to_string(),
            messages: vec![],
            extra: HashMap::from([
                (
                    "thinking".to_string(),
                    serde_json::json!({"type": "enabled"}),
                ),
                (
                    "reasoning".to_string(),
                    serde_json::json!({"effort": "max", "summary": "auto"}),
                ),
            ]),
            ..Default::default()
        };

        request.normalize_reasoning_compat_fields();

        assert_eq!(
            request
                .thinking
                .as_ref()
                .map(|thinking| thinking.kind.clone()),
            Some(ThinkingType::Enabled)
        );
        assert_eq!(
            request
                .reasoning_effort
                .as_ref()
                .and_then(ReasoningEffort::as_str),
            Some("max")
        );
    }

    #[test]
    fn normalize_reasoning_compat_prefers_top_level_effort_over_conflicting_mode() {
        let mut request = ChatCompletionRequest {
            model: "glm-5.3-flash".to_string(),
            messages: vec![],
            thinking: Some(ThinkingConfig {
                kind: ThinkingType::Disabled,
                budget_tokens: None,
            }),
            reasoning_effort: Some("high".into()),
            extra: HashMap::from([
                (
                    "thinking".to_string(),
                    serde_json::json!({"type": "enabled"}),
                ),
                (
                    "reasoning".to_string(),
                    serde_json::json!({"effort": "max"}),
                ),
            ]),
            ..Default::default()
        };

        request.normalize_reasoning_compat_fields();

        assert_eq!(
            request
                .thinking
                .as_ref()
                .map(|thinking| thinking.kind.clone()),
            Some(ThinkingType::Enabled)
        );
        assert_eq!(
            request
                .reasoning_effort
                .as_ref()
                .and_then(ReasoningEffort::as_str),
            Some("high")
        );
    }
}

#[cfg(test)]
mod reasoning_block_tests {
    use super::*;

    #[test]
    fn message_deserializes_reasoning_blocks_and_cache_control() {
        let message = r#"{
            "role": "assistant",
            "content": "ok",
            "reasoning_blocks": [
                {
                    "id": "rs_1",
                    "summary": [{"type": "summary_text", "text": "think"}],
                    "encrypted_content": "enc-blob"
                }
            ],
            "cache_control": {"type": "ephemeral"}
        }"#;

        let parsed: Message = serde_json::from_str(message).expect("parse message");
        let blocks = parsed.reasoning_blocks.expect("reasoning blocks");
        assert_eq!(blocks.len(), 1);
        assert_eq!(blocks[0].id.as_deref(), Some("rs_1"));
        assert_eq!(blocks[0].encrypted_content.as_deref(), Some("enc-blob"));
        assert_eq!(parsed.cache_control.unwrap()["type"], "ephemeral");
    }

    #[test]
    fn message_without_new_fields_serializes_without_new_keys() {
        let message: Message =
            serde_json::from_str(r#"{"role":"assistant","content":"ok"}"#).expect("parse");
        let value = serde_json::to_value(&message).expect("serialize");
        let object = value.as_object().expect("object");
        assert!(!object.contains_key("reasoning_blocks"));
        assert!(!object.contains_key("cache_control"));
        assert_eq!(object.get("content"), Some(&serde_json::json!("ok")));
    }

    #[test]
    fn reasoning_block_roundtrips_extra_fields() {
        let block: ReasoningBlock =
            serde_json::from_str(r#"{"thinking":"hmm","signature":"sig","future_field":1}"#)
                .expect("parse reasoning block");
        let value = serde_json::to_value(&block).expect("serialize");
        assert_eq!(value["thinking"], "hmm");
        assert_eq!(value["signature"], "sig");
        assert_eq!(value["future_field"], 1);
    }
}
