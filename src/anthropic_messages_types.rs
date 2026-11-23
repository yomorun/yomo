//! Anthropic Messages native protocol types (request / response / stream events).
//!
//! These types are the native byte contract shared by the `/messages` API
//! and the `anthropic-messages` dialect: parse and serialize must be symmetric,
//! with unknown fields passed through losslessly via `extra`.
//! Schema reference: `docs/anthropic_messages.md`.

use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;

/// Prompt caching breakpoint marker allowed on Anthropic content blocks.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct CacheControl {
    #[serde(rename = "type")]
    pub kind: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub ttl: Option<String>,
}

/// Request message (input direction).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AnthropicMessage {
    pub role: String,
    pub content: AnthropicMessageContent,
}

/// message content: a string is shorthand for a single text block.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum AnthropicMessageContent {
    Text(String),
    Blocks(Vec<AnthropicContentBlock>),
}

/// system parameter: string or array of text blocks.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum AnthropicSystemPrompt {
    Text(String),
    Blocks(Vec<AnthropicTextBlock>),
}

/// text block used for system (with cache_control).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AnthropicTextBlock {
    #[serde(rename = "type")]
    pub kind: String,
    pub text: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cache_control: Option<CacheControl>,
}

/// Input-direction content block (inside request messages).
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum AnthropicContentBlock {
    Text {
        text: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        citations: Option<Value>,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        cache_control: Option<CacheControl>,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    Image {
        source: AnthropicImageSource,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        cache_control: Option<CacheControl>,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    Document {
        source: AnthropicDocumentSource,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        citations: Option<Value>,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        context: Option<String>,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        title: Option<String>,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        cache_control: Option<CacheControl>,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    /// thinking block: returned as-is; the signature must not be modified.
    Thinking {
        signature: String,
        thinking: String,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    /// Encrypted thinking block: opaque, passed through unchanged.
    RedactedThinking {
        data: String,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    ToolUse {
        id: String,
        name: String,
        #[serde(default)]
        input: Value,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        caller: Option<Value>,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        cache_control: Option<CacheControl>,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    ToolResult {
        tool_use_id: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        content: Option<AnthropicToolResultContent>,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        is_error: Option<bool>,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        cache_control: Option<CacheControl>,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    /// Unknown block type: parses tolerantly but is not byte-faithful (keep out of golden samples).
    #[serde(other)]
    Unknown,
}

/// tool_result content: string or array of content blocks.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum AnthropicToolResultContent {
    Text(String),
    Blocks(Vec<AnthropicContentBlock>),
}

/// Image source.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum AnthropicImageSource {
    Base64 {
        media_type: String,
        data: String,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    Url {
        url: String,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    File {
        file_id: String,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
}

/// Document source (PDF / plain text / URL / file / content).
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum AnthropicDocumentSource {
    Base64 {
        media_type: String,
        data: String,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    Text {
        data: String,
        media_type: String,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    Url {
        url: String,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    File {
        file_id: String,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    Content {
        content: Value,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
}

/// Request tool definition (expresses both client function tools and server tools).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AnthropicTool {
    pub name: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    /// Required for function tools; server tools (e.g. web_search) may omit it.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub input_schema: Option<Value>,
    /// Server tool type name (e.g. `web_search_20250305`).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cache_control: Option<CacheControl>,
    #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
    extra: HashMap<String, Value>,
}

/// tool_choice。
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum AnthropicToolChoice {
    Auto {
        #[serde(default, skip_serializing_if = "Option::is_none")]
        disable_parallel_tool_use: Option<bool>,
    },
    Any {
        #[serde(default, skip_serializing_if = "Option::is_none")]
        disable_parallel_tool_use: Option<bool>,
    },
    Tool {
        name: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        disable_parallel_tool_use: Option<bool>,
    },
    #[serde(rename = "none")]
    None,
}

/// thinking configuration.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum AnthropicThinkingConfig {
    Adaptive,
    Enabled { budget_tokens: i64 },
    Disabled,
}

/// `/messages` request body.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AnthropicMessagesRequest {
    pub model: String,
    pub max_tokens: i64,
    pub messages: Vec<AnthropicMessage>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub system: Option<AnthropicSystemPrompt>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub top_p: Option<f64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub top_k: Option<i64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub stop_sequences: Option<Vec<String>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub stream: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tools: Option<Vec<AnthropicTool>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tool_choice: Option<AnthropicToolChoice>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub thinking: Option<AnthropicThinkingConfig>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub metadata: Option<Value>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub service_tier: Option<String>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// usage (billing contract, feeds `EndpointUsage`).
#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub struct AnthropicUsage {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub input_tokens: Option<i64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub output_tokens: Option<i64>,
    #[serde(
        default,
        skip_serializing_if = "Option::is_none",
        rename = "cache_creation_input_tokens"
    )]
    pub cache_creation_input_tokens: Option<i64>,
    #[serde(
        default,
        skip_serializing_if = "Option::is_none",
        rename = "cache_read_input_tokens"
    )]
    pub cache_read_input_tokens: Option<i64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub server_tool_use: Option<Value>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub service_tier: Option<String>,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// Content block in responses / stream events (output direction).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum AnthropicResponseContentBlock {
    Text {
        text: String,
        /// null and absence are semantically equivalent; treated as absent, preserved via Option when present.
        #[serde(default, skip_serializing_if = "Option::is_none")]
        citations: Option<Value>,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    Thinking {
        thinking: String,
        signature: String,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    RedactedThinking {
        data: String,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    ToolUse {
        id: String,
        name: String,
        #[serde(default)]
        input: Value,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        caller: Option<Value>,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    ServerToolUse {
        id: String,
        name: String,
        #[serde(default)]
        input: Value,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    WebSearchToolResult {
        tool_use_id: String,
        #[serde(default)]
        content: Value,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    #[serde(other)]
    Unknown,
}

/// `/messages` non-streaming response body (the message_start message reuses this struct).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct AnthropicMessagesResponse {
    pub id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    pub role: String,
    pub model: String,
    #[serde(default)]
    pub content: Vec<AnthropicResponseContentBlock>,
    #[serde(default)]
    pub stop_reason: Option<String>,
    #[serde(default)]
    pub stop_sequence: Option<String>,
    #[serde(default)]
    pub usage: AnthropicUsage,
    #[serde(flatten, default, skip_serializing_if = "HashMap::is_empty")]
    pub extra: HashMap<String, Value>,
}

/// Stream event delta body (inside `message_delta`).
///
/// Anthropic always returns `stop_reason` / `stop_sequence` (possibly null),
/// so these are never skip-serialized to keep byte fidelity.
#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub struct AnthropicMessageDeltaBody {
    #[serde(default)]
    pub stop_reason: Option<String>,
    #[serde(default)]
    pub stop_sequence: Option<String>,
}

/// content block delta.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum AnthropicContentDelta {
    TextDelta {
        text: String,
    },
    InputJsonDelta {
        partial_json: String,
    },
    ThinkingDelta {
        thinking: String,
    },
    SignatureDelta {
        signature: String,
    },
    CitationsDelta {
        citation: Value,
    },
    #[serde(other)]
    Unknown,
}

/// Streaming error body.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct AnthropicErrorBody {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    pub message: String,
}

/// `/messages` streaming SSE events (JSON on `data:` lines, dispatched by `type`).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum AnthropicStreamEvent {
    MessageStart {
        message: AnthropicMessagesResponse,
    },
    MessageDelta {
        #[serde(default)]
        delta: AnthropicMessageDeltaBody,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        usage: Option<AnthropicUsage>,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    MessageStop,
    ContentBlockStart {
        index: i64,
        content_block: AnthropicResponseContentBlock,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    ContentBlockDelta {
        index: i64,
        delta: AnthropicContentDelta,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    ContentBlockStop {
        index: i64,
        #[serde(flatten, skip_serializing_if = "HashMap::is_empty")]
        extra: HashMap<String, Value>,
    },
    Ping,
    Error {
        error: AnthropicErrorBody,
    },
    #[serde(other)]
    Unknown,
}

#[cfg(test)]
mod tests {
    use super::*;

    /// After parse -> serialize the sample must be semantically equal (deep Value comparison); unknown fields survive.
    fn assert_fidelity(sample: &str) {
        let original: Value = serde_json::from_str(sample).expect("sample must be valid json");
        let roundtrip = match serde_json::from_str::<AnthropicMessagesRequest>(sample) {
            Ok(request) => serde_json::to_value(&request).expect("serialize request"),
            Err(_) => match serde_json::from_str::<AnthropicMessagesResponse>(sample) {
                Ok(response) => serde_json::to_value(&response).expect("serialize response"),
                Err(_) => {
                    let event: AnthropicStreamEvent =
                        serde_json::from_str(sample).expect("parse stream event");
                    assert!(
                        !matches!(event, AnthropicStreamEvent::Unknown),
                        "sample must match a known event variant"
                    );
                    serde_json::to_value(&event).expect("serialize event")
                }
            },
        };
        assert_eq!(original, roundtrip);
    }

    #[test]
    fn fidelity_request_with_all_block_types() {
        assert_fidelity(
            r#"{
                "model": "claude-opus-5",
                "max_tokens": 8192,
                "system": [
                    {"type": "text", "text": "you are helpful", "cache_control": {"type": "ephemeral", "ttl": "1h"}}
                ],
                "messages": [
                    {"role": "user", "content": "hello"},
                    {"role": "assistant", "content": [
                        {"type": "thinking", "thinking": "let me think", "signature": "sig-abc"},
                        {"type": "redacted_thinking", "data": "opaque-blob"},
                        {"type": "text", "text": "hi"}
                    ]},
                    {"role": "user", "content": [
                        {"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "aGk="}},
                        {"type": "document", "source": {"type": "url", "url": "https://example.com/a.pdf"}, "title": "a"},
                        {"type": "tool_result", "tool_use_id": "toolu_1", "content": [
                            {"type": "text", "text": "result", "cache_control": {"type": "ephemeral"}}
                        ], "is_error": false}
                    ]}
                ],
                "temperature": 0.7,
                "top_p": 0.9,
                "top_k": 40,
                "stop_sequences": ["END"],
                "stream": true,
                "tools": [
                    {"name": "get_weather", "description": "weather", "input_schema": {"type": "object", "properties": {}}},
                    {"type": "web_search_20250305", "name": "web_search", "max_uses": 3}
                ],
                "tool_choice": {"type": "auto", "disable_parallel_tool_use": true},
                "thinking": {"type": "enabled", "budget_tokens": 2048},
                "metadata": {"user_id": "u1"},
                "some_future_field": {"nested": true}
            }"#,
        );
    }

    #[test]
    fn fidelity_response_with_usage_and_extra() {
        assert_fidelity(
            r#"{
                "id": "msg_01",
                "type": "message",
                "role": "assistant",
                "model": "claude-opus-5",
                "content": [
                    {"type": "text", "text": "answer", "citations": [{"type": "char_location", "cited_text": "a", "document_index": 0, "document_title": "d", "end_char_index": 1, "start_char_index": 0}]},
                    {"type": "tool_use", "id": "toolu_1", "name": "get_weather", "input": {"city": "tokyo"}},
                    {"type": "thinking", "thinking": "hmm", "signature": "sig"}
                ],
                "stop_reason": "tool_use",
                "stop_sequence": null,
                "usage": {
                    "input_tokens": 100,
                    "output_tokens": 42,
                    "cache_creation_input_tokens": 10,
                    "cache_read_input_tokens": 5
                },
                "container": {"id": "c1"}
            }"#,
        );
    }

    #[test]
    fn fidelity_stream_events() {
        assert_fidelity(
            r#"{"type": "message_start", "message": {"id": "msg_01", "type": "message", "role": "assistant", "model": "claude-opus-5", "content": [], "stop_reason": null, "stop_sequence": null, "usage": {"input_tokens": 10, "output_tokens": 1}}}"#,
        );
        assert_fidelity(
            r#"{"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}"#,
        );
        assert_fidelity(
            r#"{"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "hi"}}"#,
        );
        assert_fidelity(
            r#"{"type": "content_block_delta", "index": 1, "delta": {"type": "input_json_delta", "partial_json": "{\"a\":"}}"#,
        );
        assert_fidelity(
            r#"{"type": "content_block_delta", "index": 2, "delta": {"type": "thinking_delta", "thinking": "step"}}"#,
        );
        assert_fidelity(
            r#"{"type": "content_block_delta", "index": 2, "delta": {"type": "signature_delta", "signature": "sig"}}"#,
        );
        assert_fidelity(r#"{"type": "content_block_stop", "index": 0}"#);
        assert_fidelity(
            r#"{"type": "message_delta", "delta": {"stop_reason": "end_turn", "stop_sequence": null}, "usage": {"output_tokens": 42}}"#,
        );
        assert_fidelity(r#"{"type": "message_stop"}"#);
        assert_fidelity(r#"{"type": "ping"}"#);
        assert_fidelity(
            r#"{"type": "error", "error": {"type": "overloaded_error", "message": "overloaded"}}"#,
        );
    }

    #[test]
    fn stream_event_tolerates_unknown_type_and_fields() {
        let event: AnthropicStreamEvent =
            serde_json::from_str(r#"{"type": "some_future_event", "payload": 1}"#)
                .expect("unknown event must parse");
        assert!(matches!(event, AnthropicStreamEvent::Unknown));

        let delta_event: AnthropicStreamEvent = serde_json::from_str(
            r#"{"type": "content_block_delta", "index": 0, "delta": {"type": "new_delta_kind", "x": 1}, "future": true}"#,
        )
        .expect("unknown delta must parse");
        match delta_event {
            AnthropicStreamEvent::ContentBlockDelta {
                index,
                delta,
                extra,
            } => {
                assert_eq!(index, 0);
                assert!(matches!(delta, AnthropicContentDelta::Unknown));
                assert_eq!(extra.get("future"), Some(&Value::Bool(true)));
            }
            other => panic!("unexpected event: {other:?}"),
        }
    }

    #[test]
    fn request_tolerates_unknown_block_type() {
        let request: AnthropicMessagesRequest = serde_json::from_str(
            r#"{
                "model": "claude-opus-5",
                "max_tokens": 100,
                "messages": [{"role": "user", "content": [{"type": "brand_new_block", "x": 1}]}]
            }"#,
        )
        .expect("unknown block type must parse");
        let AnthropicMessageContent::Blocks(blocks) = &request.messages[0].content else {
            panic!("expected blocks");
        };
        assert!(matches!(blocks[0], AnthropicContentBlock::Unknown));
    }

    #[test]
    fn tool_choice_and_thinking_roundtrip() {
        let choice: AnthropicToolChoice =
            serde_json::from_str(r#"{"type": "tool", "name": "get_weather"}"#)
                .expect("parse tool choice");
        assert_eq!(
            choice,
            AnthropicToolChoice::Tool {
                name: "get_weather".to_string(),
                disable_parallel_tool_use: None
            }
        );

        let none: AnthropicToolChoice =
            serde_json::from_str(r#"{"type": "none"}"#).expect("parse none choice");
        assert_eq!(none, AnthropicToolChoice::None);

        let adaptive: AnthropicThinkingConfig =
            serde_json::from_str(r#"{"type": "adaptive"}"#).expect("parse adaptive thinking");
        assert_eq!(adaptive, AnthropicThinkingConfig::Adaptive);
    }
}
