use std::collections::HashMap;

use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::model_api_provider::MessagesUsage;

pub(super) const DEFAULT_ANTHROPIC_VERSION: &str = "2023-06-01";
pub(super) const DEFAULT_BEDROCK_ANTHROPIC_VERSION: &str = "bedrock-2023-05-31";
pub(super) const DEFAULT_MAX_TOKENS: i32 = 4096;

#[derive(Serialize)]
pub(super) struct AnthropicRequest {
    pub model: String,
    pub max_tokens: i32,
    pub messages: Vec<AnthropicMessage>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub system: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub top_p: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stop_sequences: Option<Vec<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stream: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tools: Option<Vec<AnthropicTool>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_choice: Option<AnthropicToolChoice>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thinking: Option<AnthropicThinking>,
}

#[derive(Serialize)]
pub(super) struct BedrockRequest {
    pub anthropic_version: String,
    pub max_tokens: i32,
    pub messages: Vec<AnthropicMessage>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub system: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub top_p: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stop_sequences: Option<Vec<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tools: Option<Vec<AnthropicTool>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_choice: Option<AnthropicToolChoice>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thinking: Option<AnthropicThinking>,
}

#[derive(Serialize)]
pub(super) struct AnthropicMessage {
    pub role: String,
    pub content: Vec<AnthropicContentBlock>,
}

#[derive(Serialize, Clone)]
#[serde(tag = "type", rename_all = "snake_case")]
pub(super) enum AnthropicContentBlock {
    Text {
        text: String,
    },
    Image {
        source: AnthropicImageSource,
    },
    ToolUse {
        id: String,
        name: String,
        input: Value,
    },
    ToolResult {
        tool_use_id: String,
        #[serde(skip_serializing_if = "Option::is_none")]
        content: Option<Vec<AnthropicContentBlock>>,
        #[serde(skip_serializing_if = "Option::is_none")]
        is_error: Option<bool>,
    },
}

#[derive(Serialize, Clone)]
#[serde(tag = "type", rename_all = "snake_case")]
pub(super) enum AnthropicImageSource {
    Url { url: String },
}

#[derive(Serialize)]
pub(super) struct AnthropicTool {
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    pub input_schema: Value,
}

#[derive(Serialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub(super) enum AnthropicToolChoice {
    Auto {
        #[serde(skip_serializing_if = "Option::is_none")]
        disable_parallel_tool_use: Option<bool>,
    },
    Any {
        #[serde(skip_serializing_if = "Option::is_none")]
        disable_parallel_tool_use: Option<bool>,
    },
    Tool {
        name: String,
        #[serde(skip_serializing_if = "Option::is_none")]
        disable_parallel_tool_use: Option<bool>,
    },
    None,
}

#[derive(Serialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub(super) enum AnthropicThinking {
    Adaptive,
    Enabled { budget_tokens: i32 },
    Disabled,
}

#[derive(Deserialize)]
pub(super) struct AnthropicResponse {
    pub id: String,
    pub model: String,
    pub content: Vec<AnthropicResponseContentBlock>,
    #[serde(default)]
    pub stop_reason: Option<String>,
    #[serde(default)]
    pub usage: Option<MessagesUsage>,
}

#[derive(Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub(super) enum AnthropicResponseContentBlock {
    Text {
        text: String,
    },
    Thinking {
        thinking: String,
    },
    ToolUse {
        id: String,
        name: String,
        input: Value,
    },
    #[serde(other)]
    Unknown,
}

#[derive(Deserialize)]
pub(super) struct AnthropicErrorEnvelope {
    pub error: AnthropicError,
}

#[derive(Deserialize)]
pub(super) struct AnthropicError {
    #[serde(default)]
    pub r#type: Option<String>,
    pub message: String,
}

#[derive(Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub(super) enum StreamEvent {
    MessageStart {
        message: StreamMessage,
    },
    ContentBlockStart {
        index: usize,
        content_block: StreamContentBlock,
    },
    ContentBlockDelta {
        index: usize,
        delta: StreamContentDelta,
    },
    ContentBlockStop {
        index: usize,
    },
    MessageDelta {
        #[serde(default)]
        delta: Option<StreamMessageDelta>,
        #[serde(default)]
        usage: Option<MessagesUsage>,
    },
    MessageStop,
    Ping,
    #[serde(other)]
    Unknown,
}

#[derive(Deserialize)]
pub(super) struct StreamMessage {
    pub id: String,
    pub model: String,
}

#[derive(Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub(super) enum StreamContentBlock {
    Text {},
    Thinking {},
    ToolUse {
        id: String,
        name: String,
        #[serde(default)]
        input: Value,
    },
    #[serde(other)]
    Unknown,
}

#[derive(Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub(super) enum StreamContentDelta {
    TextDelta {
        text: String,
    },
    ThinkingDelta {
        thinking: String,
    },
    InputJsonDelta {
        partial_json: String,
    },
    #[serde(other)]
    Unknown,
}

#[derive(Deserialize)]
pub(super) struct StreamMessageDelta {
    #[serde(default)]
    pub stop_reason: Option<String>,
}

#[derive(Default)]
pub(super) struct StreamState {
    pub response_id: String,
    pub model: String,
    pub created_at: String,
    pub started: bool,
    pub completed: bool,
    pub stop_reason: Option<String>,
    pub blocks: HashMap<usize, ActiveBlock>,
}

pub(super) enum ActiveBlock {
    ToolUse {
        id: String,
        name: String,
        arguments: String,
    },
    Other,
}

pub(super) struct RequestParts {
    pub model: String,
    pub max_tokens: i32,
    pub messages: Vec<AnthropicMessage>,
    pub system: Option<String>,
    pub temperature: Option<f32>,
    pub top_p: Option<f32>,
    pub stop_sequences: Option<Vec<String>>,
    pub tools: Option<Vec<AnthropicTool>>,
    pub tool_choice: Option<AnthropicToolChoice>,
    pub thinking: Option<AnthropicThinking>,
}
