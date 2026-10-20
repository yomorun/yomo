//! OpenAI Responses native protocol types (request / response object / stream events).
//!
//! Native byte contract shared by the `/responses` API and the
//! `openai-responses` dialect: parse and serialize must be symmetric, unknown
//! fields pass through losslessly via `extra`, and unknown item types are
//! preserved whole via `Other(Value)`. Schema reference: `docs/openai_responses.md`.

use serde::de::Error as DeError;
use serde::{Deserialize, Deserializer, Serialize};
use serde_json::Value;
use std::collections::HashMap;

/// Deserialize helper dispatching explicitly on the `type` field: unknown kinds fall into `Other` intact.
fn deserialize_tagged_value<'de, D, T, F>(deserializer: D, dispatch: F) -> Result<T, D::Error>
where
    D: Deserializer<'de>,
    F: FnOnce(&str, Value) -> Result<T, D::Error>,
{
    let value = Value::deserialize(deserializer)?;
    let kind = value
        .get("type")
        .and_then(Value::as_str)
        .unwrap_or("")
        .to_string();
    dispatch(&kind, value)
}

// ---------------------------------------------------------------------------
// Request
// ---------------------------------------------------------------------------

/// The `input` parameter of the `/responses` request body: string or list of input items.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(untagged)]
pub enum ResponsesInput {
    Text(String),
    Items(Vec<ResponsesInputItem>),
}

/// Request input item。
///
/// Dispatched explicitly by the `type` field; unknown item kinds are preserved whole via `Other`.
/// (The untagged derive matches too loosely; a hand-written Deserialize keeps dispatch exact.)
#[derive(Debug, Clone, PartialEq, Serialize)]
#[serde(untagged)]
pub enum ResponsesInputItem {
    FunctionCall(ResponsesFunctionCallItem),
    FunctionCallOutput(ResponsesFunctionCallOutputItem),
    Reasoning(ResponsesReasoningItem),
    ItemReference(ResponsesItemReferenceItem),
    Message(ResponsesEasyInputMessage),
    Other(Value),
}

impl<'de> Deserialize<'de> for ResponsesInputItem {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        deserialize_tagged_value(deserializer, |kind, value| match kind {
            "function_call" => serde_json::from_value(value)
                .map(Self::FunctionCall)
                .map_err(DeError::custom),
            "function_call_output" => serde_json::from_value(value)
                .map(Self::FunctionCallOutput)
                .map_err(DeError::custom),
            "reasoning" => serde_json::from_value(value)
                .map(Self::Reasoning)
                .map_err(DeError::custom),
            "item_reference" => serde_json::from_value(value)
                .map(Self::ItemReference)
                .map_err(DeError::custom),
            "message" => serde_json::from_value(value)
                .map(Self::Message)
                .map_err(DeError::custom),
            // An input item without type is message shorthand; preserved whole when unrecognized.
            _ => match serde_json::from_value::<ResponsesEasyInputMessage>(value.clone()) {
                Ok(message) => Ok(Self::Message(message)),
                Err(_) => Ok(Self::Other(value)),
            },
        })
    }
}

/// function_call input item (a tool call the model issued in a previous round).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesFunctionCallItem {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub id: Option<String>,
    pub call_id: String,
    pub name: String,
    pub arguments: String,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// function_call_output input item (the client's tool result).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesFunctionCallOutputItem {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub id: Option<String>,
    pub call_id: String,
    pub output: ResponsesFunctionCallOutput,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// The `output` of function_call_output: string or structured content.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(untagged)]
pub enum ResponsesFunctionCallOutput {
    Text(String),
    Structured(Value),
}

/// reasoning input item (encrypted reasoning passthrough, key to codex fidelity).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesReasoningItem {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub id: Option<String>,
    #[serde(default)]
    pub summary: Vec<ResponsesReasoningSummaryPart>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub content: Option<Vec<ResponsesReasoningContentPart>>,
    /// Encrypted reasoning payload; passes through unchanged and must not be modified.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub encrypted_content: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub status: Option<String>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// item_reference input item (references a historical item).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesItemReferenceItem {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    pub id: String,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// message input item (`type` may be omitted).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesEasyInputMessage {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    pub role: String,
    pub content: ResponsesMessageContent,
    /// commentary / final_answer marker on assistant messages.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub phase: Option<String>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// message content: string or list of content parts.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(untagged)]
pub enum ResponsesMessageContent {
    Text(String),
    Parts(Vec<ResponsesInputContentPart>),
}

/// Input content part (text / image / file).
#[derive(Debug, Clone, PartialEq, Serialize)]
#[serde(untagged)]
pub enum ResponsesInputContentPart {
    Text(ResponsesInputTextPart),
    Image(ResponsesInputImagePart),
    File(ResponsesInputFilePart),
    Other(Value),
}

impl<'de> Deserialize<'de> for ResponsesInputContentPart {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        deserialize_tagged_value(deserializer, |kind, value| match kind {
            "input_text" => serde_json::from_value(value)
                .map(Self::Text)
                .map_err(DeError::custom),
            "input_image" => serde_json::from_value(value)
                .map(Self::Image)
                .map_err(DeError::custom),
            "input_file" => serde_json::from_value(value)
                .map(Self::File)
                .map_err(DeError::custom),
            _ => Ok(Self::Other(value)),
        })
    }
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesInputTextPart {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    pub text: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub prompt_cache_breakpoint: Option<Value>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesInputImagePart {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub detail: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub file_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub image_url: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub prompt_cache_breakpoint: Option<Value>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesInputFilePart {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub detail: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub file_data: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub file_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub file_url: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub filename: Option<String>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// reasoning configuration (shared by request and response).
///
/// The API always returns `effort` / `summary` on the response object (possibly null); never skip-serialized for fidelity.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesReasoningConfig {
    #[serde(default)]
    pub effort: Option<String>,
    #[serde(default)]
    pub summary: Option<Value>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// Tool definition: server tools other than function tools are preserved via `extra`.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesToolDefinition {
    #[serde(rename = "type")]
    pub kind: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub parameters: Option<Value>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub strict: Option<bool>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// tool_choice: a string mode or a specific function.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(untagged)]
pub enum ResponsesToolChoice {
    Mode(String),
    Function(ResponsesToolChoiceFunction),
    Other(Value),
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesToolChoiceFunction {
    #[serde(rename = "type", default)]
    pub kind: String,
    pub name: String,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// `text` configuration (format / verbosity).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesTextConfig {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub format: Option<Value>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub verbosity: Option<String>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// `/responses` request body (Create a model response).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesRequest {
    pub model: String,
    pub input: ResponsesInput,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub instructions: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tools: Option<Vec<ResponsesToolDefinition>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tool_choice: Option<ResponsesToolChoice>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub parallel_tool_calls: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub reasoning: Option<ResponsesReasoningConfig>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub include: Option<Vec<Value>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_output_tokens: Option<i64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub previous_response_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub store: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub top_p: Option<f64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub truncation: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub metadata: Option<Value>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub text: Option<ResponsesTextConfig>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub stream: Option<bool>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

// ---------------------------------------------------------------------------
// Response object
// ---------------------------------------------------------------------------

/// usage (Responses shape, feeds `EndpointUsage`).
#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub struct ResponsesUsage {
    #[serde(default)]
    pub input_tokens: i64,
    #[serde(default)]
    pub output_tokens: i64,
    #[serde(default)]
    pub total_tokens: i64,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub input_tokens_details: Option<ResponsesInputTokensDetails>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub output_tokens_details: Option<ResponsesOutputTokensDetails>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub struct ResponsesInputTokensDetails {
    #[serde(default)]
    pub cached_tokens: i64,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cache_write_tokens: Option<i64>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub struct ResponsesOutputTokensDetails {
    #[serde(default)]
    pub reasoning_tokens: i64,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// Body of the response error field.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesErrorBody {
    #[serde(default)]
    pub code: Option<String>,
    pub message: String,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// Body of the incomplete_details field.
#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub struct ResponsesIncompleteDetails {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub reason: Option<String>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// Response output content part (output_text / refusal).
#[derive(Debug, Clone, PartialEq, Serialize)]
#[serde(untagged)]
pub enum ResponsesOutputContentPart {
    OutputText(ResponsesOutputTextPart),
    Refusal(ResponsesRefusalPart),
    Other(Value),
}

impl<'de> Deserialize<'de> for ResponsesOutputContentPart {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        deserialize_tagged_value(deserializer, |kind, value| match kind {
            "output_text" => serde_json::from_value(value)
                .map(Self::OutputText)
                .map_err(DeError::custom),
            "refusal" => serde_json::from_value(value)
                .map(Self::Refusal)
                .map_err(DeError::custom),
            _ => Ok(Self::Other(value)),
        })
    }
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesOutputTextPart {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    pub text: String,
    #[serde(default)]
    pub annotations: Vec<Value>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub logprobs: Option<Vec<Value>>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesRefusalPart {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    pub refusal: String,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// Response output item.
///
/// Dispatched explicitly by the `type` field; unknown item kinds (e.g. web_search_call) preserved whole via `Other`.
#[derive(Debug, Clone, PartialEq, Serialize)]
#[serde(untagged)]
pub enum ResponsesOutputItem {
    FunctionCall(ResponsesFunctionCallItem),
    Reasoning(ResponsesReasoningOutputItem),
    Message(ResponsesOutputMessage),
    Other(Value),
}

impl<'de> Deserialize<'de> for ResponsesOutputItem {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        deserialize_tagged_value(deserializer, |kind, value| match kind {
            "function_call" => serde_json::from_value(value)
                .map(Self::FunctionCall)
                .map_err(DeError::custom),
            "reasoning" => serde_json::from_value(value)
                .map(Self::Reasoning)
                .map_err(DeError::custom),
            "message" => serde_json::from_value(value)
                .map(Self::Message)
                .map_err(DeError::custom),
            _ => Ok(Self::Other(value)),
        })
    }
}

/// message output item in a response.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesOutputMessage {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    pub id: String,
    pub role: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub status: Option<String>,
    #[serde(default)]
    pub content: Vec<ResponsesOutputContentPart>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// reasoning output item in a response.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesReasoningOutputItem {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    pub id: String,
    #[serde(default)]
    pub summary: Vec<ResponsesReasoningSummaryPart>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub content: Option<Vec<ResponsesReasoningContentPart>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub encrypted_content: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub status: Option<String>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// reasoning summary part(summary_text)。
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesReasoningSummaryPart {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    pub text: String,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// reasoning content part(reasoning_text)。
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesReasoningContentPart {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    pub text: String,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// Responses response object.
///
/// Fields the API always returns (even when null) are always serialized for fidelity;
/// sporadic fields (`completed_at` / `access_programs`) are not written back when absent.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ResponsesResponse {
    pub id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub object: Option<String>,
    pub created_at: i64,
    pub status: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub completed_at: Option<i64>,
    #[serde(default)]
    pub error: Option<ResponsesErrorBody>,
    #[serde(default)]
    pub incomplete_details: Option<ResponsesIncompleteDetails>,
    #[serde(default)]
    pub instructions: Option<String>,
    #[serde(default)]
    pub max_output_tokens: Option<i64>,
    pub model: String,
    #[serde(default)]
    pub output: Vec<ResponsesOutputItem>,
    #[serde(default)]
    pub parallel_tool_calls: Option<bool>,
    #[serde(default)]
    pub previous_response_id: Option<String>,
    #[serde(default)]
    pub reasoning: Option<ResponsesReasoningConfig>,
    #[serde(default)]
    pub store: Option<bool>,
    #[serde(default)]
    pub temperature: Option<f64>,
    #[serde(default)]
    pub text: Option<ResponsesTextConfig>,
    #[serde(default)]
    pub tool_choice: Option<Value>,
    #[serde(default)]
    pub tools: Vec<Value>,
    #[serde(default)]
    pub top_p: Option<f64>,
    #[serde(default)]
    pub truncation: Option<String>,
    #[serde(default)]
    pub usage: Option<ResponsesUsage>,
    #[serde(default)]
    pub user: Option<Value>,
    #[serde(default)]
    pub metadata: Option<Value>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub access_programs: Option<Value>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

// ---------------------------------------------------------------------------
// Stream events
// ---------------------------------------------------------------------------

/// `/responses` streaming SSE events (JSON on `data:` lines, dispatched by `type`).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum ResponsesStreamEvent {
    #[serde(rename = "response.queued")]
    ResponseQueued {
        response: ResponsesResponse,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.created")]
    ResponseCreated {
        response: ResponsesResponse,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.in_progress")]
    ResponseInProgress {
        response: ResponsesResponse,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.completed")]
    ResponseCompleted {
        response: ResponsesResponse,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.failed")]
    ResponseFailed {
        response: ResponsesResponse,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.incomplete")]
    ResponseIncomplete {
        response: ResponsesResponse,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.output_item.added")]
    OutputItemAdded {
        output_index: i64,
        item: ResponsesOutputItem,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.output_item.done")]
    OutputItemDone {
        output_index: i64,
        item: ResponsesOutputItem,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.content_part.added")]
    ContentPartAdded {
        item_id: String,
        output_index: i64,
        content_index: i64,
        part: ResponsesOutputContentPart,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.content_part.done")]
    ContentPartDone {
        item_id: String,
        output_index: i64,
        content_index: i64,
        part: ResponsesOutputContentPart,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.output_text.delta")]
    OutputTextDelta {
        item_id: String,
        output_index: i64,
        content_index: i64,
        delta: String,
        sequence_number: i64,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        logprobs: Option<Vec<Value>>,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.output_text.done")]
    OutputTextDone {
        item_id: String,
        output_index: i64,
        content_index: i64,
        text: String,
        sequence_number: i64,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        logprobs: Option<Vec<Value>>,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.output_text.annotation.added")]
    OutputTextAnnotationAdded {
        item_id: String,
        output_index: i64,
        content_index: i64,
        annotation_index: i64,
        annotation: Value,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.refusal.delta")]
    RefusalDelta {
        item_id: String,
        output_index: i64,
        content_index: i64,
        delta: String,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.refusal.done")]
    RefusalDone {
        item_id: String,
        output_index: i64,
        content_index: i64,
        refusal: String,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.function_call_arguments.delta")]
    FunctionCallArgumentsDelta {
        item_id: String,
        output_index: i64,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        content_index: Option<i64>,
        delta: String,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.function_call_arguments.done")]
    FunctionCallArgumentsDone {
        item_id: String,
        output_index: i64,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        content_index: Option<i64>,
        arguments: String,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.reasoning_summary_part.added")]
    ReasoningSummaryPartAdded {
        item_id: String,
        output_index: i64,
        summary_index: i64,
        part: ResponsesReasoningSummaryPart,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.reasoning_summary_part.done")]
    ReasoningSummaryPartDone {
        item_id: String,
        output_index: i64,
        summary_index: i64,
        part: ResponsesReasoningSummaryPart,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.reasoning_summary_text.delta")]
    ReasoningSummaryTextDelta {
        item_id: String,
        output_index: i64,
        summary_index: i64,
        delta: String,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.reasoning_summary_text.done")]
    ReasoningSummaryTextDone {
        item_id: String,
        output_index: i64,
        summary_index: i64,
        text: String,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.reasoning_text.delta")]
    ReasoningTextDelta {
        item_id: String,
        output_index: i64,
        content_index: i64,
        delta: String,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.reasoning_text.done")]
    ReasoningTextDone {
        item_id: String,
        output_index: i64,
        content_index: i64,
        text: String,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "response.error")]
    ResponseError {
        code: Option<String>,
        message: String,
        #[serde(default)]
        param: Option<String>,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(rename = "error")]
    Error {
        code: Option<String>,
        message: String,
        #[serde(default)]
        param: Option<String>,
        sequence_number: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    /// Unknown event: parses tolerantly but is not byte-faithful (keep out of golden samples).
    #[serde(other)]
    Unknown,
}

#[cfg(test)]
mod tests {
    use super::*;

    /// After parse -> serialize the sample must be semantically equal (deep Value comparison); unknown fields survive.
    fn assert_fidelity(sample: &str) {
        let original: Value = serde_json::from_str(sample).expect("sample must be valid json");
        let roundtrip = match serde_json::from_str::<ResponsesRequest>(sample) {
            Ok(request) => serde_json::to_value(&request).expect("serialize request"),
            Err(_) => match serde_json::from_str::<ResponsesResponse>(sample) {
                Ok(response) => serde_json::to_value(&response).expect("serialize response"),
                Err(_) => {
                    let event: ResponsesStreamEvent =
                        serde_json::from_str(sample).expect("parse event");
                    assert!(
                        !matches!(event, ResponsesStreamEvent::Unknown),
                        "sample must match a known event variant"
                    );
                    serde_json::to_value(&event).expect("serialize event")
                }
            },
        };
        assert_eq!(original, roundtrip);
    }

    #[test]
    fn fidelity_request_with_function_and_reasoning_items() {
        assert_fidelity(
            r#"{
                "model": "gpt-5.3-codex",
                "instructions": "you are codex",
                "input": [
                    {"type": "message", "role": "user", "content": [
                        {"type": "input_text", "text": "hello"}
                    ]},
                    {"type": "reasoning", "id": "rs_1", "summary": [
                        {"type": "summary_text", "text": "thinking"}
                    ], "encrypted_content": "enc-blob"},
                    {"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "shell", "arguments": "{\"cmd\":\"ls\"}"},
                    {"type": "function_call_output", "call_id": "call_1", "output": "{\"out\":\"ok\"}"}
                ],
                "tools": [
                    {"type": "function", "name": "shell", "description": "run shell", "parameters": {"type": "object"}, "strict": false}
                ],
                "tool_choice": "auto",
                "parallel_tool_calls": false,
                "reasoning": {"effort": "high", "summary": "auto"},
                "include": ["reasoning.encrypted_content"],
                "store": false,
                "stream": true,
                "temperature": 1.0,
                "prompt_cache_key": "conv-1"
            }"#,
        );
    }

    #[test]
    fn fidelity_request_with_plain_string_input_and_text_config() {
        assert_fidelity(
            r#"{
                "model": "gpt-5.3",
                "input": "how much wood?",
                "text": {"format": {"type": "text"}, "verbosity": "medium"},
                "truncation": "disabled",
                "metadata": {"foo": "bar"}
            }"#,
        );
    }

    #[test]
    fn fidelity_response_object_full() {
        assert_fidelity(
            r#"{
                "id": "resp_1",
                "object": "response",
                "created_at": 1741477868,
                "status": "completed",
                "error": null,
                "incomplete_details": null,
                "instructions": null,
                "max_output_tokens": null,
                "model": "gpt-5.3-codex",
                "output": [
                    {"type": "reasoning", "id": "rs_1", "summary": [
                        {"type": "summary_text", "text": "thinking"}
                    ], "encrypted_content": "enc-blob"},
                    {"type": "message", "id": "msg_1", "status": "completed", "role": "assistant", "content": [
                        {"type": "output_text", "text": "answer", "annotations": [], "logprobs": []}
                    ]},
                    {"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "shell", "arguments": "{}", "status": "completed"},
                    {"type": "web_search_call", "id": "ws_1", "status": "completed", "action": {"type": "search", "query": "q"}}
                ],
                "parallel_tool_calls": true,
                "previous_response_id": null,
                "reasoning": {"effort": "high", "summary": "auto"},
                "store": true,
                "temperature": 1.0,
                "text": {"format": {"type": "text"}},
                "tool_choice": "auto",
                "tools": [],
                "top_p": 1.0,
                "truncation": "disabled",
                "usage": {
                    "input_tokens": 81,
                    "input_tokens_details": {"cached_tokens": 0, "cache_write_tokens": 0},
                    "output_tokens": 1035,
                    "output_tokens_details": {"reasoning_tokens": 832},
                    "total_tokens": 1116
                },
                "user": null,
                "metadata": {}
            }"#,
        );
    }

    #[test]
    fn fidelity_stream_events_full_cycle() {
        assert_fidelity(
            r#"{"type":"response.created","response":{"id":"resp_1","object":"response","created_at":1741290958,"status":"in_progress","error":null,"incomplete_details":null,"instructions":"hi","max_output_tokens":null,"model":"gpt-5.3-codex","output":[],"parallel_tool_calls":true,"previous_response_id":null,"reasoning":{"effort":null,"summary":null},"store":true,"temperature":1.0,"text":{"format":{"type":"text"}},"tool_choice":"auto","tools":[],"top_p":1.0,"truncation":"disabled","usage":null,"user":null,"metadata":{}},"sequence_number":0}"#,
        );
        assert_fidelity(
            r#"{"type":"response.output_item.added","output_index":0,"item":{"id":"msg_1","type":"message","status":"in_progress","role":"assistant","content":[]},"sequence_number":2}"#,
        );
        assert_fidelity(
            r#"{"type":"response.content_part.added","item_id":"msg_1","output_index":0,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]},"sequence_number":3}"#,
        );
        assert_fidelity(
            r#"{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"Hi","sequence_number":4,"logprobs":[]}"#,
        );
        assert_fidelity(
            r#"{"type":"response.output_text.done","item_id":"msg_1","output_index":0,"content_index":0,"text":"Hi","sequence_number":10,"logprobs":[]}"#,
        );
        assert_fidelity(
            r#"{"type":"response.content_part.done","item_id":"msg_1","output_index":0,"content_index":0,"part":{"type":"output_text","text":"Hi","annotations":[]},"sequence_number":11}"#,
        );
        assert_fidelity(
            r#"{"type":"response.output_item.done","output_index":0,"item":{"id":"msg_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Hi","annotations":[]}]},"sequence_number":12}"#,
        );
        assert_fidelity(
            r#"{"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":1,"delta":"{\"a\"","sequence_number":13}"#,
        );
        assert_fidelity(
            r#"{"type":"response.function_call_arguments.done","item_id":"fc_1","output_index":1,"arguments":"{\"a\":1}","sequence_number":14}"#,
        );
        assert_fidelity(
            r#"{"type":"response.reasoning_summary_part.added","item_id":"rs_1","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":""},"sequence_number":5}"#,
        );
        assert_fidelity(
            r#"{"type":"response.reasoning_summary_text.delta","item_id":"rs_1","output_index":0,"summary_index":0,"delta":"hmm","sequence_number":6}"#,
        );
        assert_fidelity(
            r#"{"type":"response.reasoning_summary_text.done","item_id":"rs_1","output_index":0,"summary_index":0,"text":"hmm","sequence_number":7}"#,
        );
        assert_fidelity(
            r#"{"type":"response.reasoning_summary_part.done","item_id":"rs_1","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":"hmm"},"sequence_number":8}"#,
        );
        assert_fidelity(
            r#"{"type":"response.reasoning_text.delta","item_id":"rs_1","output_index":0,"content_index":0,"delta":"hmm","sequence_number":9}"#,
        );
        assert_fidelity(
            r#"{"type":"response.refusal.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"no","sequence_number":15}"#,
        );
        assert_fidelity(
            r#"{"type":"response.refusal.done","item_id":"msg_1","output_index":0,"content_index":0,"refusal":"no","sequence_number":16}"#,
        );
        assert_fidelity(
            r#"{"type":"response.in_progress","response":{"id":"resp_1","object":"response","created_at":1741290958,"status":"in_progress","error":null,"incomplete_details":null,"instructions":null,"max_output_tokens":null,"model":"gpt-5.3","output":[],"parallel_tool_calls":true,"previous_response_id":null,"reasoning":{"effort":null,"summary":null},"store":true,"temperature":1.0,"text":{"format":{"type":"text"}},"tool_choice":"auto","tools":[],"top_p":1.0,"truncation":"disabled","usage":null,"user":null,"metadata":{}},"sequence_number":1}"#,
        );
        assert_fidelity(
            r#"{"type":"response.incomplete","response":{"id":"resp_1","object":"response","created_at":1,"status":"incomplete","error":null,"incomplete_details":{"reason":"max_output_tokens"},"instructions":null,"max_output_tokens":null,"model":"gpt-5.3","output":[],"parallel_tool_calls":true,"previous_response_id":null,"reasoning":{"effort":null,"summary":null},"store":true,"temperature":1.0,"text":{"format":{"type":"text"}},"tool_choice":"auto","tools":[],"top_p":1.0,"truncation":"disabled","usage":{"input_tokens":1,"input_tokens_details":{"cached_tokens":0},"output_tokens":2,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":3},"user":null,"metadata":{}},"sequence_number":17}"#,
        );
        assert_fidelity(
            r#"{"type":"response.failed","response":{"id":"resp_1","object":"response","created_at":1,"status":"failed","error":{"code":"server_error","message":"boom"},"incomplete_details":null,"instructions":null,"max_output_tokens":null,"model":"gpt-5.3","output":[],"parallel_tool_calls":true,"previous_response_id":null,"reasoning":{"effort":null,"summary":null},"store":true,"temperature":1.0,"text":{"format":{"type":"text"}},"tool_choice":"auto","tools":[],"top_p":1.0,"truncation":"disabled","usage":null,"user":null,"metadata":{}},"sequence_number":18}"#,
        );
        assert_fidelity(
            r#"{"type":"response.error","code":"server_error","message":"boom","param":null,"sequence_number":19}"#,
        );
    }

    #[test]
    fn stream_event_tolerates_unknown_type() {
        let event: ResponsesStreamEvent =
            serde_json::from_str(r#"{"type": "response.image_gen_call.partial_image", "x": 1}"#)
                .expect("unknown event must parse");
        assert!(matches!(event, ResponsesStreamEvent::Unknown));
    }

    #[test]
    fn request_input_items_preserve_unknown_item_type() {
        let request: ResponsesRequest = serde_json::from_str(
            r#"{
                "model": "gpt-5.3",
                "input": [
                    {"type": "item_reference", "id": "itm_1"},
                    {"type": "some_future_item", "payload": 1}
                ]
            }"#,
        )
        .expect("parse request");

        let ResponsesInput::Items(items) = &request.input else {
            panic!("expected items");
        };
        assert!(matches!(
            items[0],
            ResponsesInputItem::ItemReference(ResponsesItemReferenceItem { .. })
        ));
        match &items[1] {
            ResponsesInputItem::Other(value) => {
                assert_eq!(value["type"], "some_future_item");
            }
            other => panic!("expected other item: {other:?}"),
        }
    }

    #[test]
    fn message_input_item_tolerates_missing_type() {
        let request: ResponsesRequest = serde_json::from_str(
            r#"{"model": "gpt-5.3", "input": [{"role": "user", "content": "hi"}]}"#,
        )
        .expect("parse request");

        let ResponsesInput::Items(items) = &request.input else {
            panic!("expected items");
        };
        match &items[0] {
            ResponsesInputItem::Message(message) => {
                assert_eq!(message.role, "user");
                assert!(message.r#type.is_none());
            }
            other => panic!("expected message item: {other:?}"),
        }
    }

    #[test]
    fn response_output_items_keep_unknown_type_via_other() {
        let response: ResponsesResponse = serde_json::from_str(
            r#"{
                "id": "resp_1",
                "created_at": 1,
                "status": "completed",
                "model": "gpt-5.3",
                "output": [
                    {"type": "file_search_call", "id": "fs_1", "status": "completed", "queries": ["q"]}
                ]
            }"#,
        )
        .expect("parse response");

        match &response.output[0] {
            ResponsesOutputItem::Other(value) => {
                assert_eq!(value["type"], "file_search_call");
            }
            other => panic!("expected other output item: {other:?}"),
        }
    }

    #[test]
    fn tool_choice_accepts_function_object_and_modes() {
        let auto: ResponsesToolChoice = serde_json::from_str(r#""auto""#).expect("parse auto");
        assert_eq!(auto, ResponsesToolChoice::Mode("auto".to_string()));

        let function: ResponsesToolChoice =
            serde_json::from_str(r#"{"type": "function", "name": "shell"}"#)
                .expect("parse function choice");
        assert_eq!(
            function,
            ResponsesToolChoice::Function(ResponsesToolChoiceFunction {
                kind: "function".to_string(),
                name: "shell".to_string(),
                extra: HashMap::new(),
            })
        );
    }

    #[test]
    fn untagged_input_item_is_not_confused_between_variants() {
        let item: ResponsesInputItem = serde_json::from_str(
            r#"{"type": "function_call", "call_id": "call_1", "name": "shell", "arguments": "{}"}"#,
        )
        .expect("parse function call item");
        assert!(matches!(item, ResponsesInputItem::FunctionCall(_)));

        let item: ResponsesInputItem = serde_json::from_str(
            r#"{"type": "function_call_output", "call_id": "call_1", "output": "ok"}"#,
        )
        .expect("parse function call output item");
        assert!(matches!(item, ResponsesInputItem::FunctionCallOutput(_)));
    }
}
