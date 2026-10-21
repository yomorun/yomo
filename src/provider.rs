use std::{collections::HashMap, pin::Pin};

use async_trait::async_trait;
use axum::http::HeaderMap;
use axum::http::StatusCode;
use futures_core::Stream;
use log::{Level, debug, log_enabled};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use std::fmt::Debug;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};
use tracing::Span;

use crate::models::{self, Ctx};
use crate::openai_types::ErrorDetail;
use crate::serve_config::ResolvedBinding;
use crate::upstreams::{Dialect, EffortField};
use crate::usage_handler::EndpointUsage;

/// Diagnostic context retained across asynchronous response streaming.
pub struct UpstreamLog {
    call_id: String,
    provider: &'static str,
    model: String,
}

/// Correlate provider diagnostics with the current engine call span.
pub fn diagnostic_call_id(span: &Span) -> String {
    static NEXT_CALL: AtomicU64 = AtomicU64::new(1);
    span.id()
        .map(|id| format!("span-{}", id.into_u64()))
        .unwrap_or_else(|| format!("call-{}", NEXT_CALL.fetch_add(1, Ordering::Relaxed)))
}

impl UpstreamLog {
    pub fn new(provider: &'static str, model: &str) -> Self {
        Self {
            call_id: diagnostic_call_id(&Span::current()),
            provider,
            model: model.into(),
        }
    }

    /// Serialize only when DEBUG is enabled; never truncate diagnostic bodies.
    pub fn json<T: Serialize + ?Sized>(&self, event: &str, url: Option<&str>, body: &T) {
        if !log_enabled!(Level::Debug) {
            return;
        }
        match serde_json::to_value(body) {
            Ok(body) => self.emit(event, url, body),
            Err(error) => self.emit(event, url, json!({"serialization_error":error.to_string()})),
        }
    }

    /// Preserve original textual payloads, including invalid JSON and SSE sentinels.
    pub fn bytes(&self, event: &str, body: &[u8]) {
        if log_enabled!(Level::Debug) {
            self.emit(event, None, json!(String::from_utf8_lossy(body)));
        }
    }

    /// SDK diagnostics are explicitly distinguished from HTTP wire payloads.
    pub fn sdk<T: Debug + ?Sized>(&self, event: &str, body: &T) {
        if log_enabled!(Level::Debug) {
            self.emit(
                event,
                None,
                json!({"representation":"sdk_debug","data":format!("{body:?}")}),
            );
        }
    }

    pub fn response_start(&self, status: u16, headers: &HeaderMap) {
        if !log_enabled!(Level::Debug) {
            return;
        }
        let headers = diagnostic_response_headers(headers);
        self.emit(
            "upstream.response_start",
            None,
            json!({"status":status,"headers":headers}),
        );
    }

    fn emit(&self, event: &str, url: Option<&str>, body: Value) {
        debug!("{} {}", event, self.record(url, body));
    }

    fn record(&self, url: Option<&str>, body: Value) -> Value {
        json!({"call_id":self.call_id,"provider":self.provider,"model":self.model,"url":url,"body":body})
    }
}

fn diagnostic_response_headers(headers: &HeaderMap) -> HashMap<String, String> {
    [
        "content-type",
        "x-request-id",
        "request-id",
        "x-amzn-requestid",
        "x-ms-request-id",
    ]
    .into_iter()
    .filter_map(|name| {
        headers
            .get(name)
            .and_then(|v| v.to_str().ok())
            .map(|value| (name.into(), value.into()))
    })
    .collect()
}

/// Full unified data with explicit engine correlation, before billing or filtering.
pub fn log_unified<T: Serialize + ?Sized>(
    event: &str,
    call_id: &str,
    trace_id: &str,
    round: usize,
    model: &str,
    body: &T,
) {
    if !log_enabled!(Level::Debug) {
        return;
    }
    match serde_json::to_value(body) {
        Ok(body) => debug!(
            "{} {}",
            event,
            json!({"call_id":call_id,"trace_id":trace_id,"round":round,"model":model,"body":body})
        ),
        Err(error) => debug!("{} serialization_error={}", event, error),
    }
}

/// Function-call view used by the existing execution loop.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolCall {
    pub id: Option<String>,
    pub name: String,
    pub description: String,
    pub arguments: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum FinishReason {
    Stop,
    Length,
    ToolCalls,
    ContentFilter,
    Other,
}

/// Existing stop-reason mapping used by protocol serializers.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum StopReason {
    EndTurn,
    StopSequence,
    MaxTokens,
    ToolUse,
    ContentFilter,
    Other,
}
impl StopReason {
    pub fn from_anthropic(value: &str) -> Self {
        match value {
            "end_turn" => Self::EndTurn,
            "stop_sequence" => Self::StopSequence,
            "max_tokens" => Self::MaxTokens,
            "tool_use" => Self::ToolUse,
            "refusal" => Self::ContentFilter,
            _ => Self::Other,
        }
    }
    pub fn from_openai(value: &str) -> Self {
        match value {
            "stop" => Self::EndTurn,
            "length" => Self::MaxTokens,
            "tool_calls" | "function_call" => Self::ToolUse,
            "content_filter" => Self::ContentFilter,
            _ => Self::Other,
        }
    }
    pub fn to_anthropic(self) -> &'static str {
        match self {
            Self::MaxTokens => "max_tokens",
            Self::ToolUse => "tool_use",
            Self::StopSequence => "end_turn",
            _ => "end_turn",
        }
    }
    pub fn to_openai(self) -> &'static str {
        match self {
            Self::MaxTokens => "length",
            Self::ToolUse => "tool_calls",
            Self::ContentFilter => "content_filter",
            _ => "stop",
        }
    }
}

/// The protocol-neutral request passed through the execution engine.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct UnifiedRequest {
    pub model: String,
    pub instructions: Option<Vec<UnifiedInputItem>>,
    pub input: Vec<UnifiedInputItem>,
    #[serde(default)]
    pub context: UnifiedRequestContext,
    pub tools: Option<Vec<UnifiedTool>>,
    pub tool_choice: Option<UnifiedToolChoice>,
    #[serde(default)]
    pub options: UnifiedRequestOptions,
    #[serde(default)]
    pub controls: UnifiedRequestControls,
    /// Local tool context. Native serializers must never forward this upstream.
    pub execution_context: Option<Value>,
    pub source_extensions: Option<UnifiedSourceExtensions>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum UnifiedSourceProtocol {
    ChatCompletions,
    Responses,
    Messages,
}

/// Unmodeled native fields owned by this object. Merge only into the same
/// protocol, recursively, with typed fields taking precedence.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UnifiedSourceExtensions {
    pub protocol: UnifiedSourceProtocol,
    pub fields: HashMap<String, Value>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct UnifiedRequestContext {
    pub previous_response_id: Option<String>,
    pub conversation_id: Option<String>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct UnifiedRequestControls {
    pub stream: Option<bool>,
    pub stream_options: Option<Value>,
    pub store: Option<bool>,
    pub background: Option<bool>,
    pub include: Option<Vec<String>>,
}

/// Engine identity is distinct from the optional real upstream identity.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct UnifiedItemIdentity {
    pub internal_id: String,
    pub source_id: Option<String>,
}

/// Flattened items sharing message_id belong to one original message.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct UnifiedMessageBoundary {
    pub message_id: String,
    pub role: UnifiedRole,
    pub block_index: Option<u32>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum UnifiedRole {
    System,
    Developer,
    User,
    Assistant,
    Tool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UnifiedInputItem {
    pub identity: Option<UnifiedItemIdentity>,
    pub status: Option<String>,
    pub message: Option<UnifiedMessageBoundary>,
    pub source_extensions: Option<UnifiedSourceExtensions>,
    pub kind: UnifiedInputItemKind,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum UnifiedInputItemKind {
    Message {
        role: UnifiedRole,
        content: Vec<UnifiedContentPart>,
        phase: Option<String>,
    },
    FunctionCall {
        call_id: String,
        name: String,
        arguments: String,
        caller: Option<Value>,
        cache_control: Option<UnifiedCacheControl>,
    },
    FunctionCallOutput {
        call_id: String,
        output: UnifiedToolOutput,
        is_error: Option<bool>,
        cache_control: Option<UnifiedCacheControl>,
    },
    Reasoning {
        summary: Vec<Value>,
        content: Option<Vec<UnifiedContentPart>>,
        encrypted_content: Option<String>,
    },
    ItemReference {
        id: String,
    },
    Other {
        item_type: String,
        payload: Value,
    },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", content = "value", rename_all = "snake_case")]
pub enum UnifiedToolOutput {
    Missing,
    Text(String),
    Content(Vec<UnifiedContentPart>),
    Json(Value),
}

/// Cache breakpoints have protocol-specific semantics, not interchangeable TTLs.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "protocol", content = "value", rename_all = "snake_case")]
pub enum UnifiedCacheControl {
    Messages(Value),
    Responses(Value),
    ChatCompletions(Value),
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UnifiedContentPart {
    pub cache_control: Option<UnifiedCacheControl>,
    pub source_extensions: Option<UnifiedSourceExtensions>,
    pub kind: UnifiedContentPartKind,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum UnifiedContentPartKind {
    Text {
        text: String,
        annotations: Vec<Value>,
        citations: Option<Value>,
        logprobs: Option<Vec<Value>>,
    },
    Refusal {
        text: String,
    },
    Thinking {
        text: String,
        signature: Option<String>,
    },
    /// Opaque Anthropic data, never reinterpreted as Responses encrypted_content.
    RedactedThinking {
        data: String,
    },
    Image {
        source: UnifiedMediaSource,
        detail: Option<String>,
        transformations: Option<Value>,
    },
    File {
        source: UnifiedMediaSource,
        filename: Option<String>,
        detail: Option<String>,
    },
    Document {
        source: UnifiedDocumentSource,
        title: Option<String>,
        context: Option<String>,
        citations: Option<Value>,
    },
    Audio {
        audio_type: String,
        payload: Value,
    },
    Other {
        part_type: String,
        payload: Value,
    },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum UnifiedMediaSource {
    Url { url: String },
    FileId { file_id: String },
    Base64 { data: String, media_type: String },
    DataUrl { url: String },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum UnifiedDocumentSource {
    Media { source: UnifiedMediaSource },
    Text { text: String, media_type: String },
    Content { content: Vec<UnifiedContentPart> },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UnifiedTool {
    pub cache_control: Option<UnifiedCacheControl>,
    pub source_extensions: Option<UnifiedSourceExtensions>,
    pub kind: UnifiedToolKind,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "snake_case")]
pub enum UnifiedToolKind {
    Function {
        name: String,
        description: Option<String>,
        parameters: Value,
        strict: Option<bool>,
        allowed_callers: Option<Vec<String>>,
    },
    Server {
        tool_type: String,
        name: Option<String>,
        configuration: Value,
    },
    Other {
        tool_type: String,
        payload: Value,
    },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "mode", content = "value", rename_all = "snake_case")]
pub enum UnifiedToolChoice {
    Auto,
    None,
    Required,
    Function(String),
    Allowed { mode: String, tools: Vec<Value> },
    Other(Value),
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct UnifiedRequestOptions {
    pub temperature: Option<f64>,
    pub top_p: Option<f64>,
    pub top_k: Option<i64>,
    pub max_output_tokens: Option<i64>,
    pub stop_sequences: Option<Vec<String>>,
    pub parallel_tool_calls: Option<bool>,
    pub n: Option<u32>,
    pub presence_penalty: Option<f64>,
    pub frequency_penalty: Option<f64>,
    pub logprobs: Option<bool>,
    pub top_logprobs: Option<u32>,
    pub seed: Option<i64>,
    pub modalities: Option<Vec<String>>,
    pub reasoning_effort: Option<String>,
    pub reasoning_summary: Option<Value>,
    pub reasoning_extensions: Option<UnifiedSourceExtensions>,
    pub thinking: Option<Value>,
    pub response_format: Option<Value>,
    pub verbosity: Option<String>,
    pub truncation: Option<String>,
    pub service_tier: Option<String>,
    pub prediction: Option<Value>,
    pub metadata: Option<Value>,
    pub audio: Option<Value>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UnifiedResponse {
    pub id: String,
    pub created_at: Option<i64>,
    pub completed_at: Option<i64>,
    pub model: String,
    pub status: UnifiedResponseStatus,
    /// Alternative generations, not ordered output items. Messages/Responses use index 0.
    pub candidates: Vec<UnifiedCandidate>,
    pub usage: Option<EndpointUsage>,
    /// Original usage fields outside the billing schema, for same-protocol fidelity.
    pub source_usage: Option<UnifiedSourceExtensions>,
    pub metadata: Option<Value>,
    pub service_tier: Option<String>,
    pub source_extensions: Option<UnifiedSourceExtensions>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UnifiedCandidate {
    pub index: u32,
    pub output: Vec<UnifiedOutputItem>,
    pub ending: Option<UnifiedEnding>,
    pub logprobs: Option<Value>,
    pub source_extensions: Option<UnifiedSourceExtensions>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UnifiedEnding {
    pub reason: UnifiedStopReason,
    pub source_reason: Option<String>,
    pub stop_sequence: Option<String>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum UnifiedStopReason {
    EndTurn,
    StopSequence,
    MaxTokens,
    ToolUse,
    PauseTurn,
    Refusal,
    ContentFilter,
    ContextWindowExceeded,
    Compaction,
    Other,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "status", rename_all = "snake_case")]
pub enum UnifiedResponseStatus {
    Queued,
    InProgress,
    Completed,
    Incomplete { reason: Option<String> },
    Failed { error: UnifiedResponseError },
    Cancelled { reason: Option<String> },
    Other { value: String },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UnifiedResponseError {
    pub code: Option<String>,
    pub error_type: Option<String>,
    pub param: Option<String>,
    pub message: String,
    pub source_extensions: Option<UnifiedSourceExtensions>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UnifiedOutputItem {
    pub identity: UnifiedItemIdentity,
    pub status: Option<String>,
    pub message: Option<UnifiedMessageBoundary>,
    pub source_extensions: Option<UnifiedSourceExtensions>,
    pub kind: UnifiedOutputItemKind,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum UnifiedOutputItemKind {
    Message {
        role: UnifiedRole,
        content: Vec<UnifiedContentPart>,
        phase: Option<String>,
    },
    FunctionCall {
        call_id: String,
        name: String,
        arguments: String,
        caller: Option<Value>,
    },
    Reasoning {
        summary: Vec<Value>,
        content: Option<Vec<UnifiedContentPart>>,
        encrypted_content: Option<String>,
    },
    Other {
        item_type: String,
        payload: Value,
    },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UnifiedEvent {
    pub response_id: Option<String>,
    pub sequence_number: Option<i64>,
    pub target: Option<UnifiedEventTarget>,
    pub source_extensions: Option<UnifiedSourceExtensions>,
    pub kind: UnifiedEventKind,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct UnifiedEventTarget {
    pub candidate_index: u32,
    pub item_id: Option<String>,
    pub output_index: Option<u32>,
    pub part: Option<UnifiedPartLocation>,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(tag = "type", content = "index", rename_all = "snake_case")]
pub enum UnifiedPartLocation {
    Content(u32),
    ReasoningSummary(u32),
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum UnifiedUsageUpdate {
    Snapshot,
    Merge,
    Increment,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum UnifiedEventKind {
    ResponseQueued {
        response: Option<UnifiedResponse>,
    },
    ResponseCreated {
        response: UnifiedResponse,
    },
    ResponseInProgress {
        response: Option<UnifiedResponse>,
    },
    OutputItemAdded {
        item: UnifiedOutputItem,
    },
    OutputItemDone {
        item: UnifiedOutputItem,
    },
    ContentPartAdded {
        part: UnifiedContentPart,
    },
    ContentPartDone {
        part: UnifiedContentPart,
    },
    ContentPartDelta {
        delta: UnifiedContentDelta,
    },
    ItemDelta {
        delta: UnifiedItemDelta,
    },
    FunctionArgumentsDone {
        arguments: String,
    },
    ReasoningSummaryPartAdded {
        part: Value,
    },
    ReasoningSummaryPartDone {
        part: Value,
        status: Option<String>,
    },
    ReasoningSummaryTextDelta {
        delta: String,
    },
    ReasoningSummaryTextDone {
        text: String,
    },
    ContentValueDone {
        value: UnifiedContentDelta,
    },
    CandidateDone {
        ending: UnifiedEnding,
    },
    Usage {
        usage: EndpointUsage,
        update: UnifiedUsageUpdate,
        source_usage: Option<UnifiedSourceExtensions>,
    },
    Completed {
        response: Option<UnifiedResponse>,
    },
    Incomplete {
        reason: Option<String>,
        response: Option<UnifiedResponse>,
    },
    Failed {
        error: UnifiedResponseError,
        response: Option<UnifiedResponse>,
    },
    Cancelled {
        reason: Option<String>,
        response: Option<UnifiedResponse>,
    },
    RuntimeToolStarted {
        call_id: String,
        name: String,
        arguments: String,
    },
    RuntimeToolFinished {
        call_id: String,
        name: String,
        output: UnifiedToolOutput,
        is_error: bool,
    },
    Other {
        event_type: String,
        payload: Value,
    },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", content = "value", rename_all = "snake_case")]
pub enum UnifiedContentDelta {
    Text(String),
    Refusal(String),
    Thinking(String),
    Signature(String),
    Citation(Value),
    AnnotationAdded { index: u32, annotation: Value },
    Audio(Value),
    TextWithLogprobs { text: String, logprobs: Vec<Value> },
    Other(Value),
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", content = "value", rename_all = "snake_case")]
pub enum UnifiedItemDelta {
    FunctionArguments(String),
    FunctionName(String),
    CallId(String),
    Other(Value),
}

impl UnifiedContentPart {
    pub fn text(text: String) -> Self {
        Self {
            cache_control: None,
            source_extensions: None,
            kind: UnifiedContentPartKind::Text {
                text,
                annotations: vec![],
                citations: None,
                logprobs: None,
            },
        }
    }
}

impl UnifiedInputItem {
    pub fn new(kind: UnifiedInputItemKind) -> Self {
        Self {
            identity: None,
            status: None,
            message: None,
            source_extensions: None,
            kind,
        }
    }
}

impl UnifiedOutputItem {
    pub fn new(id: String, kind: UnifiedOutputItemKind) -> Self {
        Self {
            identity: UnifiedItemIdentity {
                internal_id: id,
                source_id: None,
            },
            status: None,
            message: None,
            source_extensions: None,
            kind,
        }
    }
    pub fn as_input(&self) -> UnifiedInputItem {
        let kind = match &self.kind {
            UnifiedOutputItemKind::Message {
                role,
                content,
                phase,
            } => UnifiedInputItemKind::Message {
                role: *role,
                content: content.clone(),
                phase: phase.clone(),
            },
            UnifiedOutputItemKind::FunctionCall {
                call_id,
                name,
                arguments,
                caller,
            } => UnifiedInputItemKind::FunctionCall {
                call_id: call_id.clone(),
                name: name.clone(),
                arguments: arguments.clone(),
                caller: caller.clone(),
                cache_control: None,
            },
            UnifiedOutputItemKind::Reasoning {
                summary,
                content,
                encrypted_content,
            } => UnifiedInputItemKind::Reasoning {
                summary: summary.clone(),
                content: content.clone(),
                encrypted_content: encrypted_content.clone(),
            },
            UnifiedOutputItemKind::Other { item_type, payload } => UnifiedInputItemKind::Other {
                item_type: item_type.clone(),
                payload: payload.clone(),
            },
        };
        UnifiedInputItem {
            identity: Some(self.identity.clone()),
            status: self.status.clone(),
            message: self.message.clone(),
            source_extensions: self.source_extensions.clone(),
            kind,
        }
    }
}

impl UnifiedResponse {
    /// Adapt existing provider mapping results without changing native parsing.
    pub fn from_completion(
        id: String,
        created_at: String,
        model: String,
        text: String,
        reasoning: Option<String>,
        calls: Option<Vec<ToolCall>>,
        finish: FinishReason,
        usage: EndpointUsage,
    ) -> Self {
        let mut response = Self::new(id.clone(), model);
        response.created_at = chrono::DateTime::parse_from_rfc3339(&created_at)
            .ok()
            .map(|time| time.timestamp());
        response.status = UnifiedResponseStatus::Completed;
        response.usage = Some(usage);
        let mut content = Vec::new();
        if let Some(reasoning) = reasoning {
            content.push(UnifiedContentPart {
                kind: UnifiedContentPartKind::Thinking {
                    text: reasoning,
                    signature: None,
                },
                cache_control: None,
                source_extensions: None,
            });
        }
        if !text.is_empty() {
            content.push(UnifiedContentPart::text(text));
        }
        let mut output = vec![UnifiedOutputItem::new(
            format!("{id}-message"),
            UnifiedOutputItemKind::Message {
                role: UnifiedRole::Assistant,
                content,
                phase: None,
            },
        )];
        for (index, call) in calls.into_iter().flatten().enumerate() {
            let call_id = call.id.unwrap_or_else(|| format!("{id}-tool-{index}"));
            output.push(UnifiedOutputItem::new(
                call_id.clone(),
                UnifiedOutputItemKind::FunctionCall {
                    call_id,
                    name: call.name,
                    arguments: call.arguments,
                    caller: None,
                },
            ));
        }
        for item in &mut output {
            item.message = Some(UnifiedMessageBoundary {
                message_id: id.clone(),
                role: UnifiedRole::Assistant,
                block_index: None,
            });
        }
        let reason = match finish {
            FinishReason::Stop => "stop",
            FinishReason::Length => "length",
            FinishReason::ToolCalls => "tool_calls",
            FinishReason::ContentFilter => "content_filter",
            FinishReason::Other => "other",
        };
        response.candidates.push(UnifiedCandidate {
            index: 0,
            output,
            ending: Some(UnifiedEnding::from_source(reason)),
            logprobs: None,
            source_extensions: None,
        });
        response
    }
    pub fn new(id: String, model: String) -> Self {
        Self {
            id,
            model,
            created_at: None,
            completed_at: None,
            status: UnifiedResponseStatus::InProgress,
            candidates: vec![],
            usage: None,
            source_usage: None,
            metadata: None,
            service_tier: None,
            source_extensions: None,
        }
    }
}

impl UnifiedEvent {
    pub fn new(kind: UnifiedEventKind) -> Self {
        Self {
            response_id: None,
            sequence_number: None,
            target: None,
            source_extensions: None,
            kind,
        }
    }
    pub fn located(
        mut self,
        candidate_index: u32,
        item_id: String,
        output_index: u32,
        part: Option<UnifiedPartLocation>,
    ) -> Self {
        self.target = Some(UnifiedEventTarget {
            candidate_index,
            item_id: Some(item_id),
            output_index: Some(output_index),
            part,
        });
        self
    }
}

impl UnifiedEnding {
    pub fn from_source(reason: &str) -> Self {
        let normalized = match reason {
            "stop" | "end_turn" | "completed" => UnifiedStopReason::EndTurn,
            "stop_sequence" => UnifiedStopReason::StopSequence,
            "length" | "max_tokens" | "max_output_tokens" => UnifiedStopReason::MaxTokens,
            "tool_calls" | "function_call" | "tool_use" => UnifiedStopReason::ToolUse,
            "pause_turn" => UnifiedStopReason::PauseTurn,
            "refusal" => UnifiedStopReason::Refusal,
            "content_filter" => UnifiedStopReason::ContentFilter,
            "model_context_window_exceeded" => UnifiedStopReason::ContextWindowExceeded,
            "compaction" => UnifiedStopReason::Compaction,
            _ => UnifiedStopReason::Other,
        };
        Self {
            reason: normalized,
            source_reason: Some(reason.to_owned()),
            stop_sequence: None,
        }
    }
    pub fn to_chat(&self) -> &str {
        match self.reason {
            UnifiedStopReason::MaxTokens | UnifiedStopReason::ContextWindowExceeded => "length",
            UnifiedStopReason::ToolUse => "tool_calls",
            UnifiedStopReason::ContentFilter | UnifiedStopReason::Refusal => "content_filter",
            _ => "stop",
        }
    }
    pub fn to_messages(&self) -> &str {
        match self.reason {
            UnifiedStopReason::EndTurn => "end_turn",
            UnifiedStopReason::StopSequence => "stop_sequence",
            UnifiedStopReason::MaxTokens => "max_tokens",
            UnifiedStopReason::ToolUse => "tool_use",
            UnifiedStopReason::PauseTurn => "pause_turn",
            UnifiedStopReason::Refusal | UnifiedStopReason::ContentFilter => "refusal",
            UnifiedStopReason::ContextWindowExceeded => "model_context_window_exceeded",
            UnifiedStopReason::Compaction => "compaction",
            UnifiedStopReason::Other => self.source_reason.as_deref().unwrap_or("end_turn"),
        }
    }
}

fn merge_usage_value(target: &mut Value, source: &Value, increment: bool) {
    if let (Some(target), Some(source)) = (target.as_object_mut(), source.as_object()) {
        for (key, value) in source {
            if value.is_null() {
                continue;
            }
            if let Some(existing) = target.get_mut(key) {
                if existing.is_object() && value.is_object() {
                    merge_usage_value(existing, value, increment);
                } else if increment && existing.is_number() && value.is_number() {
                    *existing = json!(
                        existing
                            .as_i64()
                            .unwrap_or(0)
                            .saturating_add(value.as_i64().unwrap_or(0))
                    );
                } else {
                    *existing = value.clone();
                }
            } else {
                target.insert(key.clone(), value.clone());
            }
        }
    }
}

/// Add independent round usages in the common billing view.
pub fn accumulate_usage(total: &mut EndpointUsage, usage: &EndpointUsage) {
    let mut combined = total.raw_payload();
    if let Some(usage) = usage.to_openai_usage() {
        merge_usage_value(&mut combined, &json!(usage), true);
        if let Ok(usage) = EndpointUsage::from_endpoint_payload("/chat/completions", combined) {
            *total = usage;
        }
    }
}

fn update_usage(
    response: &mut UnifiedResponse,
    usage: &EndpointUsage,
    update: UnifiedUsageUpdate,
    source: Option<&UnifiedSourceExtensions>,
) {
    let endpoint = match usage {
        EndpointUsage::Messages(_) => "/messages",
        EndpointUsage::Responses(_) => "/responses",
        EndpointUsage::GenerateContent(_) => "/models/model:generateContent",
        _ => "/chat/completions",
    };
    let mut payload = response
        .usage
        .as_ref()
        .map(EndpointUsage::raw_payload)
        .unwrap_or_else(|| json!({}));
    if update == UnifiedUsageUpdate::Snapshot {
        payload = usage.raw_payload();
    } else {
        merge_usage_value(
            &mut payload,
            &usage.raw_payload(),
            update == UnifiedUsageUpdate::Increment,
        );
    }
    response.usage = EndpointUsage::from_endpoint_payload(endpoint, payload).ok();
    if let Some(source) = source {
        if let Some(old) = response
            .source_usage
            .as_mut()
            .filter(|old| old.protocol == source.protocol)
        {
            let mut value = json!(old.fields);
            if update == UnifiedUsageUpdate::Snapshot {
                value = json!(source.fields);
            } else {
                merge_usage_value(
                    &mut value,
                    &json!(source.fields),
                    update == UnifiedUsageUpdate::Increment,
                );
            }
            old.fields = serde_json::from_value(value).unwrap_or_default();
        } else {
            response.source_usage = Some(source.clone());
        }
    }
}

fn candidate_mut(response: &mut UnifiedResponse, index: u32) -> &mut UnifiedCandidate {
    if let Some(position) = response.candidates.iter().position(|c| c.index == index) {
        return &mut response.candidates[position];
    }
    response.candidates.push(UnifiedCandidate {
        index,
        output: vec![],
        ending: None,
        logprobs: None,
        source_extensions: None,
    });
    response.candidates.sort_by_key(|c| c.index);
    let position = response
        .candidates
        .iter()
        .position(|c| c.index == index)
        .unwrap_or(0);
    &mut response.candidates[position]
}

fn item_mut<'a>(
    response: &'a mut UnifiedResponse,
    target: &UnifiedEventTarget,
) -> Result<&'a mut UnifiedOutputItem, String> {
    let index = target
        .output_index
        .ok_or("item event requires output_index")? as usize;
    let id = target
        .item_id
        .as_ref()
        .ok_or("item event requires internal item_id")?;
    let candidate = candidate_mut(response, target.candidate_index);
    while candidate.output.len() <= index {
        let slot = candidate.output.len();
        candidate.output.push(UnifiedOutputItem::new(
            format!("pending-{}-{slot}", target.candidate_index),
            UnifiedOutputItemKind::Message {
                role: UnifiedRole::Assistant,
                content: vec![],
                phase: None,
            },
        ));
    }
    let item = &mut candidate.output[index];
    if item.identity.internal_id.starts_with("pending-") {
        item.identity.internal_id = id.clone();
    }
    if item.identity.internal_id != *id {
        return Err("event item identity does not match output_index".into());
    }
    Ok(item)
}

fn parts_mut(item: &mut UnifiedOutputItem) -> Result<&mut Vec<UnifiedContentPart>, String> {
    match &mut item.kind {
        UnifiedOutputItemKind::Message { content, .. } => Ok(content),
        UnifiedOutputItemKind::Reasoning { content, .. } => {
            Ok(content.get_or_insert_with(Vec::new))
        }
        _ => Err("content event targets a non-content item".into()),
    }
}

fn delta_part(delta: &UnifiedContentDelta) -> UnifiedContentPart {
    let kind = match delta {
        UnifiedContentDelta::Thinking(_) | UnifiedContentDelta::Signature(_) => {
            UnifiedContentPartKind::Thinking {
                text: String::new(),
                signature: None,
            }
        }
        UnifiedContentDelta::Refusal(_) => UnifiedContentPartKind::Refusal {
            text: String::new(),
        },
        _ => UnifiedContentPartKind::Text {
            text: String::new(),
            annotations: vec![],
            citations: None,
            logprobs: None,
        },
    };
    UnifiedContentPart {
        kind,
        cache_control: None,
        source_extensions: None,
    }
}

fn apply_content_delta(
    part: &mut UnifiedContentPart,
    delta: &UnifiedContentDelta,
    replace: bool,
) -> Result<(), String> {
    fn text(target: &mut String, value: &str, replace: bool) {
        if replace {
            *target = value.into();
        } else {
            target.push_str(value);
        }
    }
    match (&mut part.kind, delta) {
        (UnifiedContentPartKind::Text { text: t, .. }, UnifiedContentDelta::Text(v))
        | (UnifiedContentPartKind::Thinking { text: t, .. }, UnifiedContentDelta::Thinking(v))
        | (UnifiedContentPartKind::Refusal { text: t }, UnifiedContentDelta::Refusal(v)) => {
            text(t, v, replace)
        }
        (UnifiedContentPartKind::Thinking { signature, .. }, UnifiedContentDelta::Signature(v)) => {
            text(signature.get_or_insert_with(String::new), v, replace)
        }
        (
            UnifiedContentPartKind::Text { annotations, .. },
            UnifiedContentDelta::AnnotationAdded { index, annotation },
        ) => {
            while annotations.len() <= *index as usize {
                annotations.push(Value::Null);
            }
            annotations[*index as usize] = annotation.clone();
        }
        (
            UnifiedContentPartKind::Text { citations, .. },
            UnifiedContentDelta::Citation(citation),
        ) => {
            citations
                .get_or_insert_with(|| json!([]))
                .as_array_mut()
                .ok_or("citations must be an array")?
                .push(citation.clone());
        }
        (
            UnifiedContentPartKind::Text {
                text: t, logprobs, ..
            },
            UnifiedContentDelta::TextWithLogprobs {
                text: v,
                logprobs: probs,
            },
        ) => {
            text(t, v, replace);
            logprobs.get_or_insert_with(Vec::new).extend(probs.clone());
        }
        (_, UnifiedContentDelta::Other(_) | UnifiedContentDelta::Audio(_)) => {}
        _ => return Err("delta type does not match content part".into()),
    }
    Ok(())
}

/// Pure unified stream aggregation, shared by native adapters and serializers.
/// Done snapshots replace increments; content and summary use separate indexes.
pub fn apply_event(response: &mut UnifiedResponse, event: &UnifiedEvent) -> Result<(), String> {
    if let Some(id) = &event.response_id {
        if !id.is_empty() {
            response.id = id.clone();
        }
    }
    match &event.kind {
        UnifiedEventKind::ResponseCreated { response: snapshot } => *response = snapshot.clone(),
        UnifiedEventKind::ResponseQueued { response: snapshot }
        | UnifiedEventKind::ResponseInProgress { response: snapshot } => {
            if let Some(snapshot) = snapshot {
                *response = snapshot.clone();
            }
        }
        UnifiedEventKind::Completed { response: snapshot }
        | UnifiedEventKind::Incomplete {
            response: snapshot, ..
        }
        | UnifiedEventKind::Failed {
            response: snapshot, ..
        }
        | UnifiedEventKind::Cancelled {
            response: snapshot, ..
        } => {
            if let Some(snapshot) = snapshot {
                *response = snapshot.clone();
            }
            response.status = match &event.kind {
                UnifiedEventKind::Completed { .. } => UnifiedResponseStatus::Completed,
                UnifiedEventKind::Incomplete { reason, .. } => UnifiedResponseStatus::Incomplete {
                    reason: reason.clone(),
                },
                UnifiedEventKind::Failed { error, .. } => UnifiedResponseStatus::Failed {
                    error: error.clone(),
                },
                UnifiedEventKind::Cancelled { reason, .. } => UnifiedResponseStatus::Cancelled {
                    reason: reason.clone(),
                },
                _ => unreachable!(),
            };
        }
        UnifiedEventKind::Usage {
            usage,
            update,
            source_usage,
        } => update_usage(response, usage, *update, source_usage.as_ref()),
        UnifiedEventKind::CandidateDone { ending } => {
            let target = event
                .target
                .as_ref()
                .ok_or("candidate completion requires target")?;
            candidate_mut(response, target.candidate_index).ending = Some(ending.clone());
        }
        UnifiedEventKind::OutputItemAdded { item } | UnifiedEventKind::OutputItemDone { item } => {
            let target = event.target.as_ref().ok_or("item event requires target")?;
            *item_mut(response, target)? = item.clone();
        }
        UnifiedEventKind::ItemDelta { delta } => {
            let target = event.target.as_ref().ok_or("item delta requires target")?;
            if let UnifiedOutputItemKind::FunctionCall {
                call_id,
                name,
                arguments,
                ..
            } = &mut item_mut(response, target)?.kind
            {
                match delta {
                    UnifiedItemDelta::FunctionArguments(v) => arguments.push_str(v),
                    UnifiedItemDelta::FunctionName(v) => name.push_str(v),
                    UnifiedItemDelta::CallId(v) => call_id.push_str(v),
                    _ => {}
                }
            } else {
                return Err("function delta targets a non-call item".into());
            }
        }
        UnifiedEventKind::FunctionArgumentsDone { arguments } => {
            let target = event
                .target
                .as_ref()
                .ok_or("arguments done requires target")?;
            if let UnifiedOutputItemKind::FunctionCall {
                arguments: value, ..
            } = &mut item_mut(response, target)?.kind
            {
                *value = arguments.clone();
            }
        }
        UnifiedEventKind::ContentPartAdded { part }
        | UnifiedEventKind::ContentPartDone { part } => {
            let target = event
                .target
                .as_ref()
                .ok_or("content event requires target")?;
            let Some(UnifiedPartLocation::Content(index)) = target.part else {
                return Err("content event requires content index".into());
            };
            let parts = parts_mut(item_mut(response, target)?)?;
            while parts.len() <= index as usize {
                parts.push(UnifiedContentPart::text(String::new()));
            }
            parts[index as usize] = part.clone();
        }
        UnifiedEventKind::ContentPartDelta { delta }
        | UnifiedEventKind::ContentValueDone { value: delta } => {
            let target = event
                .target
                .as_ref()
                .ok_or("content delta requires target")?;
            let Some(UnifiedPartLocation::Content(index)) = target.part else {
                return Err("content delta requires content index".into());
            };
            let parts = parts_mut(item_mut(response, target)?)?;
            while parts.len() <= index as usize {
                parts.push(UnifiedContentPart {
                    kind: UnifiedContentPartKind::Other {
                        part_type: "pending".into(),
                        payload: Value::Null,
                    },
                    cache_control: None,
                    source_extensions: None,
                });
            }
            if matches!(&parts[index as usize].kind, UnifiedContentPartKind::Other { part_type, .. } if part_type == "pending")
            {
                parts[index as usize] = delta_part(delta);
            }
            apply_content_delta(
                &mut parts[index as usize],
                delta,
                matches!(event.kind, UnifiedEventKind::ContentValueDone { .. }),
            )?;
        }
        UnifiedEventKind::ReasoningSummaryPartAdded { .. }
        | UnifiedEventKind::ReasoningSummaryPartDone { .. }
        | UnifiedEventKind::ReasoningSummaryTextDelta { .. }
        | UnifiedEventKind::ReasoningSummaryTextDone { .. } => {
            let target = event
                .target
                .as_ref()
                .ok_or("summary event requires target")?;
            let Some(UnifiedPartLocation::ReasoningSummary(index)) = target.part else {
                return Err("summary event requires summary index".into());
            };
            let item = item_mut(response, target)?;
            if !matches!(item.kind, UnifiedOutputItemKind::Reasoning { .. }) {
                item.kind = UnifiedOutputItemKind::Reasoning {
                    summary: vec![],
                    content: None,
                    encrypted_content: None,
                };
            }
            if let UnifiedOutputItemKind::Reasoning { summary, .. } = &mut item.kind {
                while summary.len() <= index as usize {
                    summary.push(json!({"type":"summary_text","text":""}));
                }
                let part = &mut summary[index as usize];
                match &event.kind {
                    UnifiedEventKind::ReasoningSummaryPartAdded { part: p }
                    | UnifiedEventKind::ReasoningSummaryPartDone { part: p, .. } => {
                        *part = p.clone()
                    }
                    UnifiedEventKind::ReasoningSummaryTextDelta { delta } => {
                        let mut text = part["text"].as_str().unwrap_or("").to_owned();
                        text.push_str(delta);
                        part["text"] = json!(text);
                    }
                    UnifiedEventKind::ReasoningSummaryTextDone { text } => {
                        part["text"] = json!(text)
                    }
                    _ => {}
                }
            }
        }
        _ => {}
    }
    Ok(())
}

/// Pure recursive merge for native fidelity; modeled values win conflicts.
pub fn merge_extensions(
    value: &mut Value,
    extensions: Option<&UnifiedSourceExtensions>,
    protocol: UnifiedSourceProtocol,
) {
    fn merge(target: &mut Value, source: &Value) {
        if let (Some(target), Some(source)) = (target.as_object_mut(), source.as_object()) {
            for (key, field) in source {
                if let Some(existing) = target.get_mut(key) {
                    merge(existing, field);
                } else {
                    target.insert(key.clone(), field.clone());
                }
            }
        }
    }
    if let Some(extensions) = extensions.filter(|extensions| extensions.protocol == protocol) {
        merge(value, &json!(extensions.fields));
    }
}

/// Retain unmodeled fields at the native object's scope.
pub fn source_extensions(
    value: &Value,
    keys: &[&str],
    protocol: UnifiedSourceProtocol,
) -> Option<UnifiedSourceExtensions> {
    let fields: HashMap<_, _> = value
        .as_object()?
        .iter()
        .filter(|(key, _)| !keys.contains(&key.as_str()))
        .map(|(key, value)| (key.clone(), value.clone()))
        .collect();
    (!fields.is_empty()).then_some(UnifiedSourceExtensions { protocol, fields })
}

pub fn value_string(value: &Value, key: &str) -> Option<String> {
    value.get(key)?.as_str().map(str::to_owned)
}

pub fn parse_role(value: &str) -> Result<UnifiedRole, String> {
    match value {
        "system" => Ok(UnifiedRole::System),
        "developer" => Ok(UnifiedRole::Developer),
        "user" => Ok(UnifiedRole::User),
        "assistant" => Ok(UnifiedRole::Assistant),
        "tool" => Ok(UnifiedRole::Tool),
        _ => Err(format!("unsupported role: {value}")),
    }
}
pub fn role_name(role: UnifiedRole) -> &'static str {
    match role {
        UnifiedRole::System => "system",
        UnifiedRole::Developer => "developer",
        UnifiedRole::User => "user",
        UnifiedRole::Assistant => "assistant",
        UnifiedRole::Tool => "tool",
    }
}

/// The common envelope has no protocol-specific field mappings.
pub fn input_item(
    value: &Value,
    kind: UnifiedInputItemKind,
    index: usize,
    keys: &[&str],
    protocol: UnifiedSourceProtocol,
) -> UnifiedInputItem {
    UnifiedInputItem {
        identity: Some(UnifiedItemIdentity {
            internal_id: format!("input-{index}"),
            source_id: value_string(value, "id"),
        }),
        status: value_string(value, "status"),
        message: None,
        source_extensions: source_extensions(value, keys, protocol),
        kind,
    }
}

pub fn output_from_input(input: UnifiedInputItem, id: String) -> UnifiedOutputItem {
    let kind = match input.kind {
        UnifiedInputItemKind::Message {
            role,
            content,
            phase,
        } => UnifiedOutputItemKind::Message {
            role,
            content,
            phase,
        },
        UnifiedInputItemKind::FunctionCall {
            call_id,
            name,
            arguments,
            caller,
            ..
        } => UnifiedOutputItemKind::FunctionCall {
            call_id,
            name,
            arguments,
            caller,
        },
        UnifiedInputItemKind::Reasoning {
            summary,
            content,
            encrypted_content,
        } => UnifiedOutputItemKind::Reasoning {
            summary,
            content,
            encrypted_content,
        },
        UnifiedInputItemKind::Other { item_type, payload } => {
            UnifiedOutputItemKind::Other { item_type, payload }
        }
        other => UnifiedOutputItemKind::Other {
            item_type: "input_item".into(),
            payload: json!(other),
        },
    };
    UnifiedOutputItem {
        identity: UnifiedItemIdentity {
            internal_id: id,
            source_id: input.identity.and_then(|identity| identity.source_id),
        },
        status: input.status,
        message: input.message,
        source_extensions: input.source_extensions,
        kind,
    }
}

/// Common response identity and billing data. Status/output interpretation stays
/// with each native protocol adapter.
pub fn response_envelope(
    value: &Value,
    protocol: UnifiedSourceProtocol,
    endpoint: &str,
    keys: &[&str],
) -> Result<UnifiedResponse, String> {
    let mut response = UnifiedResponse::new(
        value_string(value, "id").ok_or("response requires id")?,
        value_string(value, "model").unwrap_or_default(),
    );
    response.metadata = value.get("metadata").cloned();
    response.service_tier = value_string(value, "service_tier");
    if let Some(usage) = value.get("usage").filter(|value| !value.is_null()) {
        response.usage = EndpointUsage::from_endpoint_payload(endpoint, usage.clone()).ok();
        response.source_usage = Some(UnifiedSourceExtensions {
            protocol,
            fields: usage
                .as_object()
                .map(|object| object.iter().map(|(k, v)| (k.clone(), v.clone())).collect())
                .unwrap_or_default(),
        });
    }
    response.source_extensions = source_extensions(value, keys, protocol);
    Ok(response)
}

#[derive(Debug)]
pub enum ProviderError {
    Public {
        status: StatusCode,
        error: ErrorDetail,
    },
    Internal {
        upstream_http_status: StatusCode,
        message: String,
    },
}
impl std::fmt::Display for ProviderError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Public { error, .. } => write!(f, "provider error: {}", error.message),
            Self::Internal { message, .. } => write!(f, "provider error: {message}"),
        }
    }
}
impl ProviderError {
    pub fn internal(message: impl Into<String>) -> Self {
        Self::internal_with_upstream_status(StatusCode::INTERNAL_SERVER_ERROR, message)
    }
    pub fn internal_with_upstream_status(status: StatusCode, message: impl Into<String>) -> Self {
        Self::Internal {
            upstream_http_status: status,
            message: message.into(),
        }
    }
}
impl std::error::Error for ProviderError {}

/// A provider translates between the unified engine contract and its native API.
#[async_trait]
pub trait Provider<M>: Send + Sync {
    fn model_id(&self) -> &str;
    /// Return the fixed integration identity, if assembled from model maps.
    fn binding(&self) -> Option<&ResolvedBinding> {
        None
    }
    async fn complete(
        &self,
        request: UnifiedRequest,
        metadata: &M,
    ) -> Result<UnifiedResponse, ProviderError>;
    async fn stream<'a>(
        &'a self,
        request: UnifiedRequest,
        metadata: &M,
    ) -> Result<
        Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    >;
}

/// Fixed identity decorator; capability checking is performed by the engine.
pub struct BoundProvider<M> {
    inner: Arc<dyn Provider<M>>,
    binding: ResolvedBinding,
}

impl<M> BoundProvider<M> {
    /// Attach the startup-resolved identity without amending requests.
    pub fn new(inner: Arc<dyn Provider<M>>, binding: ResolvedBinding) -> Self {
        Self { inner, binding }
    }
}

#[async_trait]
impl<M: Send + Sync + 'static> Provider<M> for BoundProvider<M> {
    fn model_id(&self) -> &str {
        self.inner.model_id()
    }
    fn binding(&self) -> Option<&ResolvedBinding> {
        Some(&self.binding)
    }
    async fn complete(
        &self,
        request: UnifiedRequest,
        metadata: &M,
    ) -> Result<UnifiedResponse, ProviderError> {
        self.inner.complete(request, metadata).await
    }
    async fn stream<'a>(
        &'a self,
        request: UnifiedRequest,
        metadata: &M,
    ) -> Result<
        Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    > {
        self.inner.stream(request, metadata).await
    }
}

/// Validate and adapt a request immediately before an upstream call.
///
/// Dialect-level rules (state that a dialect cannot carry, malformed numeric
/// options) live here. Everything that depends on the model is decided by
/// [`models::adapt_request`], which may also rewrite the request.
pub fn adapt_bound_request(
    request: &mut UnifiedRequest,
    binding: &ResolvedBinding,
) -> Result<(), ProviderError> {
    let profile = models::profile(&binding.model, binding.upstream).ok_or_else(|| {
        ProviderError::internal(format!(
            "no model profile for {:?}: {}",
            binding.upstream, binding.model
        ))
    })?;
    let dialect = binding
        .dialect
        .ok_or_else(|| ProviderError::internal("conversation binding has no dialect"))?;
    if !profile.dialects.contains(&dialect) {
        return Err(ProviderError::internal(
            "resolved binding dialect is not supported by the model profile",
        ));
    }
    let effort_field = if dialect == Dialect::OpenAiResponses {
        EffortField::Reasoning
    } else {
        profile.effort_field
    };
    let invalid = |param: &str| ProviderError::Public {
        status: StatusCode::BAD_REQUEST,
        error: ErrorDetail {
            message: format!("{param} is not supported by {}", binding.model),
            r#type: "invalid_request_error".into(),
            code: Some("unsupported_capability".into()),
            param: Some(param.into()),
        },
    };
    if effort_field == EffortField::Unknown && request.options.reasoning_effort.is_some() {
        return Err(invalid("reasoning_effort mapping"));
    }
    if dialect != Dialect::OpenAiResponses {
        // "auto" requests no specific summary shape and reasoning summaries
        // reach the client through the response mapping, so it is a no-op for
        // upstreams without a summary control. Explicit summary values and
        // reasoning extensions have no target and stay rejected.
        let summary_auto = request
            .options
            .reasoning_summary
            .as_ref()
            .is_some_and(|value| value.as_str() == Some("auto"));
        if (request.options.reasoning_summary.is_some() && !summary_auto)
            || request.options.reasoning_extensions.is_some()
        {
            return Err(invalid("reasoning summary/extensions mapping"));
        }
        // Encrypted reasoning is never produced for these upstreams, so that
        // include entry is a satisfiable no-op; other entries cannot be
        // honored and must not be silently discarded.
        if request.controls.include.as_ref().is_some_and(|values| {
            values
                .iter()
                .any(|value| value != "reasoning.encrypted_content")
        }) {
            return Err(invalid("include mapping"));
        }
        if request
            .context
            .previous_response_id
            .as_ref()
            .is_some_and(|value| !value.is_empty())
        {
            return Err(invalid("previous_response_id"));
        }
        if request
            .context
            .conversation_id
            .as_ref()
            .is_some_and(|value| !value.is_empty())
        {
            return Err(invalid("conversation"));
        }
        if request.controls.background == Some(true) {
            return Err(invalid("background"));
        }
        if request.input.iter().any(|item| {
            matches!(
                &item.kind,
                UnifiedInputItemKind::ItemReference { .. }
                    | UnifiedInputItemKind::Reasoning {
                        encrypted_content: Some(_),
                        ..
                    }
            )
        }) {
            return Err(invalid("reasoning/reference state"));
        }
    }
    // Messages output_config.format and OpenAI response_format have different
    // schemas. Keep native fidelity, but reject unverified cross-format output.
    if request
        .options
        .response_format
        .as_ref()
        .is_some_and(|format| {
            format.get("type").and_then(Value::as_str) == Some("json_schema")
                && format.get("json_schema").is_none()
        })
        && dialect != Dialect::AnthropicMessages
    {
        return Err(invalid("Messages output_config.format mapping"));
    }
    if request
        .options
        .temperature
        .is_some_and(|value| !value.is_finite())
    {
        return Err(invalid("temperature"));
    }
    if request
        .options
        .top_p
        .is_some_and(|value| !value.is_finite())
    {
        return Err(invalid("top_p"));
    }
    if request
        .options
        .max_output_tokens
        .is_some_and(|value| value <= 0)
    {
        return Err(invalid("max_output_tokens"));
    }
    if request.options.top_k.is_some_and(|value| value < 0) {
        return Err(invalid("top_k"));
    }
    models::adapt_request(
        &Ctx {
            model: &binding.model,
            upstream: binding.upstream,
            dialect,
        },
        request,
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::upstreams::Upstream;

    #[test]
    fn fixed_tokenhub_binding_rejects_disabled_thinking_without_mutating_effort() {
        let binding = ResolvedBinding {
            public_model: "fast".into(),
            model: "glm-5.3-flash".into(),
            provider: "qcloud.cn".into(),
            upstream: Upstream::Tokenhub,
            upstream_model: "vendor-id".into(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "qcloud.cn.vendor-id".into(),
        };
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("max".into());
        request.options.thinking = Some(json!({"type":"disabled"}));
        assert!(matches!(
            adapt_bound_request(&mut request, &binding),
            Err(ProviderError::Public {
                status: StatusCode::BAD_REQUEST,
                ..
            })
        ));
        request.options.thinking = Some(json!({"type":"enabled"}));
        assert!(adapt_bound_request(&mut request, &binding).is_ok());
        assert_eq!(request.options.reasoning_effort.as_deref(), Some("max"));
    }

    #[test]
    fn responses_state_cannot_be_discarded_by_a_fixed_chat_binding() {
        let binding = ResolvedBinding {
            public_model: "glm-5.3-flash".into(),
            model: "glm-5.3-flash".into(),
            provider: "qcloud.cn".into(),
            upstream: Upstream::Tokenhub,
            upstream_model: "glm-5.3-flash".into(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "qcloud.cn.glm-5.3-flash".into(),
        };
        let mut request = UnifiedRequest::default();
        request.context.previous_response_id = Some("upstream-response-id".into());
        assert!(matches!(
            adapt_bound_request(&mut request, &binding),
            Err(ProviderError::Public { .. })
        ));
        assert_eq!(
            request.context.previous_response_id.as_deref(),
            Some("upstream-response-id")
        );
        request.context.previous_response_id = None;
        request.controls.background = Some(true);
        assert!(adapt_bound_request(&mut request, &binding).is_err());
    }

    #[test]
    fn bedrock_haiku55_applies_its_model_reasoning_matrix() {
        let binding = ResolvedBinding {
            public_model: "claude-haiku-5-5".into(),
            model: "claude-haiku-5-5".into(),
            provider: "aws.jpn".into(),
            upstream: Upstream::AwsBedrock,
            upstream_model: "global.anthropic.claude-haiku-5-5".into(),
            dialect: Some(Dialect::BedrockConverse),
            label: "aws.jpn.global.anthropic.claude-haiku-5-5".into(),
        };
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("high".into());
        request.options.thinking = Some(json!({"type": "adaptive"}));
        assert!(adapt_bound_request(&mut request, &binding).is_ok());

        request.options.thinking = Some(json!({"type": "disabled"}));
        assert!(adapt_bound_request(&mut request, &binding).is_ok());
        request.options.reasoning_effort = Some("xhigh".into());
        assert!(matches!(
            adapt_bound_request(&mut request, &binding),
            Err(ProviderError::Public { .. })
        ));

        request.options.reasoning_effort = Some("high".into());
        request.options.thinking = Some(json!({"type": "enabled"}));
        assert!(matches!(
            adapt_bound_request(&mut request, &binding),
            Err(ProviderError::Public { .. })
        ));
        request.options.thinking = Some(json!({"type": "adaptive", "budget_tokens": 4096}));
        assert!(matches!(
            adapt_bound_request(&mut request, &binding),
            Err(ProviderError::Public { .. })
        ));
    }

    #[test]
    fn bedrock_models_map_reasoning_controls() {
        let binding = ResolvedBinding {
            public_model: "claude-sonnet-4-6".into(),
            model: "claude-sonnet-4-6".into(),
            provider: "aws.jpn".into(),
            upstream: Upstream::AwsBedrock,
            upstream_model: "global.anthropic.claude-sonnet-4-6".into(),
            dialect: Some(Dialect::BedrockConverse),
            label: "aws.jpn.global.anthropic.claude-sonnet-4-6".into(),
        };
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("high".into());
        request.options.thinking = Some(json!({"type": "adaptive"}));
        assert!(adapt_bound_request(&mut request, &binding).is_ok());

        request.options.max_output_tokens = Some(2048);
        request.options.thinking = Some(json!({"type": "enabled", "budget_tokens": 1024}));
        assert!(adapt_bound_request(&mut request, &binding).is_ok());

        // The model layer still rejects combinations outside its rule set.
        request.options.thinking = Some(json!({"type": "between_tools"}));
        assert!(matches!(
            adapt_bound_request(&mut request, &binding),
            Err(ProviderError::Public { .. })
        ));
    }

    #[test]
    fn azure_tool_rule_reports_rule_identity_for_explicit_and_unset_effort() {
        let binding = ResolvedBinding {
            public_model: "gpt-5.6-sol".into(),
            model: "gpt-5.6-sol".into(),
            provider: "azure.eastus".into(),
            upstream: Upstream::AzureOpenai,
            upstream_model: "gpt-5.6-sol".into(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "azure.eastus.gpt-5.6-sol".into(),
        };
        let mut request = UnifiedRequest::default();
        request.tools = Some(vec![UnifiedTool {
            cache_control: None,
            source_extensions: None,
            kind: UnifiedToolKind::Function {
                name: "read".into(),
                description: None,
                parameters: json!({"type": "object"}),
                strict: None,
                allowed_callers: None,
            },
        }]);
        request.options.reasoning_effort = Some("high".into());
        match adapt_bound_request(&mut request, &binding) {
            Err(ProviderError::Public { error, .. }) => {
                assert!(
                    error
                        .message
                        .contains("gpt56-chat-tools-require-effort-none"),
                    "{}",
                    error.message
                );
                assert!(!error.message.contains("AzureOpenai"), "{}", error.message);
                assert_eq!(error.param.as_deref(), Some("reasoning_effort"));
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
        request.options.reasoning_effort = None;
        match adapt_bound_request(&mut request, &binding) {
            Err(ProviderError::Public { error, .. }) => {
                assert!(
                    error.message.contains("effort is unset"),
                    "{}",
                    error.message
                );
            }
            other => panic!("expected a public denial for unset effort, got {other:?}"),
        }
        request.options.reasoning_effort = Some("none".into());
        assert!(adapt_bound_request(&mut request, &binding).is_ok());
    }

    #[test]
    fn azure_gpt6_luna_maps_only_adaptive_thinking_with_explicit_effort() {
        let binding = ResolvedBinding {
            public_model: "gpt-6-luna".into(),
            model: "gpt-6-luna".into(),
            provider: "azure.jpn".into(),
            upstream: Upstream::AzureOpenai,
            upstream_model: "gpt-6-luna".into(),
            dialect: Some(Dialect::OpenAiResponses),
            label: "azure.jpn.gpt-6-luna".into(),
        };
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("high".into());
        request.options.thinking = Some(json!({"type": "adaptive"}));
        assert!(adapt_bound_request(&mut request, &binding).is_ok());

        request.options.thinking = Some(json!({"type": "enabled"}));
        assert!(matches!(
            adapt_bound_request(&mut request, &binding),
            Err(ProviderError::Public { .. })
        ));
        request.options.thinking = Some(json!({"type": "adaptive"}));
        request.options.reasoning_effort = None;
        assert!(matches!(
            adapt_bound_request(&mut request, &binding),
            Err(ProviderError::Public { .. })
        ));
    }

    #[test]
    fn azure_gpt56_responses_route_skips_chat_only_tool_constraint() {
        let binding = ResolvedBinding {
            public_model: "gpt-5.6-sol".into(),
            model: "gpt-5.6-sol".into(),
            provider: "azure.eastus".into(),
            upstream: Upstream::AzureOpenai,
            upstream_model: "gpt-5.6-sol".into(),
            dialect: Some(Dialect::OpenAiResponses),
            label: "azure.eastus.gpt-5.6-sol".into(),
        };
        let mut request = UnifiedRequest::default();
        request.tools = Some(vec![UnifiedTool {
            cache_control: None,
            source_extensions: None,
            kind: UnifiedToolKind::Function {
                name: "read".into(),
                description: None,
                parameters: json!({"type": "object"}),
                strict: None,
                allowed_callers: None,
            },
        }]);
        request.options.reasoning_effort = Some("high".into());
        assert!(adapt_bound_request(&mut request, &binding).is_ok());
    }

    #[test]
    fn glm_forced_tool_choice_with_thinking_stays_unknown_without_error() {
        let binding = ResolvedBinding {
            public_model: "glm-5.3-flash".into(),
            model: "glm-5.3-flash".into(),
            provider: "qcloud.cn".into(),
            upstream: Upstream::Tokenhub,
            upstream_model: "glm-5.3-flash".into(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "qcloud.cn.glm-5.3-flash".into(),
        };
        let mut request = UnifiedRequest::default();
        request.tools = Some(vec![UnifiedTool {
            cache_control: None,
            source_extensions: None,
            kind: UnifiedToolKind::Function {
                name: "read".into(),
                description: None,
                parameters: json!({"type": "object"}),
                strict: None,
                allowed_callers: None,
            },
        }]);
        request.tool_choice = Some(UnifiedToolChoice::Required);
        request.options.thinking = Some(json!({"type": "enabled"}));
        assert!(adapt_bound_request(&mut request, &binding).is_ok());
        assert!(matches!(
            request.tool_choice,
            Some(UnifiedToolChoice::Required)
        ));
        assert_eq!(request.options.thinking, Some(json!({"type": "enabled"})));
        request.options.thinking = Some(json!({"type": "disabled"}));
        match adapt_bound_request(&mut request, &binding) {
            Err(ProviderError::Public { error, .. }) => {
                assert!(
                    error
                        .message
                        .contains("glm-5-3-thinking-cannot-be-disabled"),
                    "{}",
                    error.message
                );
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
    }

    #[test]
    fn ark_flash_binding_rejects_disabled_thinking() {
        let binding = ResolvedBinding {
            public_model: "viv-fast".into(),
            model: "glm-5.3-flash".into(),
            provider: "volcengine.cn".into(),
            upstream: Upstream::VolcengineArk,
            upstream_model: "glm-5-3-flash-260828".into(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "volcengine.cn.glm-5.3-flash".into(),
        };
        let mut request = UnifiedRequest::default();
        request.options.thinking = Some(json!({"type": "disabled"}));
        match adapt_bound_request(&mut request, &binding) {
            Err(ProviderError::Public { error, .. }) => {
                assert!(
                    error
                        .message
                        .contains("glm-5-3-thinking-cannot-be-disabled"),
                    "{}",
                    error.message
                );
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
    }

    #[test]
    fn reasoning_summary_auto_passes_chat_binding_but_explicit_stays_rejected() {
        let binding = ResolvedBinding {
            public_model: "glm-5.3-flash".into(),
            model: "glm-5.3-flash".into(),
            provider: "qcloud.cn".into(),
            upstream: Upstream::Tokenhub,
            upstream_model: "glm-5.3-flash".into(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "qcloud.cn.glm-5.3-flash".into(),
        };
        let mut request = UnifiedRequest::default();
        request.options.reasoning_summary = Some(json!("auto"));
        assert!(adapt_bound_request(&mut request, &binding).is_ok());
        request.options.reasoning_summary = Some(json!("concise"));
        match adapt_bound_request(&mut request, &binding) {
            Err(ProviderError::Public { error, .. }) => {
                assert_eq!(
                    error.param.as_deref(),
                    Some("reasoning summary/extensions mapping")
                );
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
        request.options.reasoning_summary = None;
        request.controls.include = Some(vec!["reasoning.encrypted_content".into()]);
        assert!(adapt_bound_request(&mut request, &binding).is_ok());
        request.controls.include = Some(vec!["message.logprobs".into()]);
        match adapt_bound_request(&mut request, &binding) {
            Err(ProviderError::Public { error, .. }) => {
                assert_eq!(error.param.as_deref(), Some("include mapping"));
            }
            other => panic!("expected a public denial, got {other:?}"),
        }
    }

    #[test]
    fn upstream_diagnostics_use_the_engine_call_span() {
        tracing::subscriber::with_default(tracing_subscriber::registry(), || {
            let first = tracing::info_span!("llm.chat", round = 1);
            let second = tracing::info_span!("llm.chat", round = 2);
            let first_id = diagnostic_call_id(&first);
            let second_id = diagnostic_call_id(&second);
            assert_ne!(first_id, second_id);
            let _entered = first.enter();
            let diagnostics = UpstreamLog::new("openai-responses", "m");
            assert_eq!(diagnostics.call_id, first_id);
        });
    }

    #[test]
    fn diagnostic_record_preserves_long_and_multiline_payloads() {
        let diagnostics = UpstreamLog::new("openai-responses", "m");
        let body = format!("{}\ninvalid JSON\n[DONE]", "x".repeat(16384));
        let encoded = diagnostics
            .record(Some("https://example.com/responses"), json!(body))
            .to_string();
        assert!(!encoded.contains('\n'));
        let decoded: Value = serde_json::from_str(&encoded).expect("record");
        assert_eq!(decoded["body"], body);
        assert_eq!(decoded["model"], "m");
        assert_eq!(decoded["call_id"], diagnostics.call_id);
    }

    #[test]
    fn diagnostic_headers_keep_request_ids_without_authentication_data() {
        let mut headers = HeaderMap::new();
        headers.insert("content-type", "text/event-stream".parse().expect("header"));
        headers.insert("x-request-id", "request-123".parse().expect("header"));
        headers.insert("authorization", "Bearer secret".parse().expect("header"));
        headers.insert("set-cookie", "session=secret".parse().expect("header"));
        let selected = diagnostic_response_headers(&headers);
        assert_eq!(selected.len(), 2);
        assert_eq!(selected["x-request-id"], "request-123");
        assert_eq!(selected["content-type"], "text/event-stream");
    }

    #[test]
    fn reasoning_before_text_aggregates_independent_parts() {
        let mut response = UnifiedResponse::new("r".into(), "m".into());
        for (index, delta) in [
            (1, UnifiedContentDelta::Thinking("plan".into())),
            (0, UnifiedContentDelta::Text("answer".into())),
        ] {
            apply_event(
                &mut response,
                &UnifiedEvent::new(UnifiedEventKind::ContentPartDelta { delta }).located(
                    0,
                    "message".into(),
                    0,
                    Some(UnifiedPartLocation::Content(index)),
                ),
            )
            .expect("aggregate");
        }
        let UnifiedOutputItemKind::Message { content, .. } = &response.candidates[0].output[0].kind
        else {
            panic!("message")
        };
        assert!(
            matches!(&content[0].kind, UnifiedContentPartKind::Text { text, .. } if text == "answer")
        );
        assert!(
            matches!(&content[1].kind, UnifiedContentPartKind::Thinking { text, .. } if text == "plan")
        );
    }

    #[test]
    fn stream_aggregation_keeps_summary_and_content_independent() {
        let mut response = UnifiedResponse::new("r".into(), "m".into());
        let item = UnifiedOutputItem::new(
            "rs".into(),
            UnifiedOutputItemKind::Reasoning {
                summary: vec![],
                content: None,
                encrypted_content: Some("opaque".into()),
            },
        );
        apply_event(
            &mut response,
            &UnifiedEvent::new(UnifiedEventKind::OutputItemAdded { item }).located(
                0,
                "rs".into(),
                0,
                None,
            ),
        )
        .expect("item");
        apply_event(
            &mut response,
            &UnifiedEvent::new(UnifiedEventKind::ReasoningSummaryTextDelta {
                delta: "plan".into(),
            })
            .located(
                0,
                "rs".into(),
                0,
                Some(UnifiedPartLocation::ReasoningSummary(0)),
            ),
        )
        .expect("summary");
        for kind in [
            UnifiedEventKind::ContentPartDelta {
                delta: UnifiedContentDelta::Thinking("reason".into()),
            },
            UnifiedEventKind::ContentValueDone {
                value: UnifiedContentDelta::Thinking("reason".into()),
            },
        ] {
            apply_event(
                &mut response,
                &UnifiedEvent::new(kind).located(
                    0,
                    "rs".into(),
                    0,
                    Some(UnifiedPartLocation::Content(0)),
                ),
            )
            .expect("content");
        }
        let UnifiedOutputItemKind::Reasoning {
            summary,
            content,
            encrypted_content,
        } = &response.candidates[0].output[0].kind
        else {
            panic!("reasoning")
        };
        assert_eq!(summary[0]["text"], "plan");
        assert_eq!(encrypted_content.as_deref(), Some("opaque"));
        assert!(
            matches!(&content.as_ref().expect("content")[0].kind, UnifiedContentPartKind::Thinking { text, .. } if text == "reason")
        );
    }

    #[test]
    fn usage_merge_preserves_input_without_adding_cumulative_output() {
        let mut response = UnifiedResponse::new("r".into(), "m".into());
        for (raw, update) in [
            (
                json!({"input_tokens":10,"output_tokens":1}),
                UnifiedUsageUpdate::Snapshot,
            ),
            (json!({"output_tokens":4}), UnifiedUsageUpdate::Merge),
            (json!({"output_tokens":7}), UnifiedUsageUpdate::Merge),
        ] {
            let usage = EndpointUsage::from_endpoint_payload("/messages", raw).expect("usage");
            apply_event(
                &mut response,
                &UnifiedEvent::new(UnifiedEventKind::Usage {
                    usage,
                    update,
                    source_usage: None,
                }),
            )
            .expect("update");
        }
        let usage = response
            .usage
            .as_ref()
            .and_then(EndpointUsage::to_openai_usage)
            .expect("usage");
        assert_eq!(usage.prompt_tokens, 10);
        assert_eq!(usage.completion_tokens, 7);
    }

    #[test]
    fn extensions_merge_recursively_only_for_same_protocol() {
        let ext = UnifiedSourceExtensions {
            protocol: UnifiedSourceProtocol::Responses,
            fields: HashMap::from([("source".into(), json!({"url": "old", "future": 1}))]),
        };
        let mut value = json!({"source": {"url": "new"}});
        merge_extensions(&mut value, Some(&ext), UnifiedSourceProtocol::Messages);
        assert_eq!(value, json!({"source": {"url": "new"}}));
        merge_extensions(&mut value, Some(&ext), UnifiedSourceProtocol::Responses);
        assert_eq!(value, json!({"source": {"url": "new", "future": 1}}));
    }

    #[test]
    fn endings_preserve_pause_stop_sequence_and_unknown_reason() {
        assert_eq!(
            UnifiedEnding::from_source("pause_turn").to_messages(),
            "pause_turn"
        );
        let mut ending = UnifiedEnding::from_source("stop_sequence");
        ending.stop_sequence = Some("END".into());
        let decoded: UnifiedEnding = serde_json::from_value(json!(ending)).expect("ending");
        assert_eq!(decoded.stop_sequence.as_deref(), Some("END"));
        assert_eq!(
            UnifiedEnding::from_source("future")
                .source_reason
                .as_deref(),
            Some("future")
        );
    }

    #[test]
    fn output_replay_preserves_identity_boundary_phase_and_encryption() {
        let item = UnifiedOutputItem {
            identity: UnifiedItemIdentity {
                internal_id: "internal".into(),
                source_id: Some("rs-1".into()),
            },
            status: Some("completed".into()),
            message: Some(UnifiedMessageBoundary {
                message_id: "turn".into(),
                role: UnifiedRole::Assistant,
                block_index: Some(0),
            }),
            source_extensions: None,
            kind: UnifiedOutputItemKind::Reasoning {
                summary: vec![json!({"type":"summary_text", "text":"summary"})],
                content: None,
                encrypted_content: Some("opaque".into()),
            },
        };
        let replay = item.as_input();
        assert_eq!(replay.identity, Some(item.identity));
        assert_eq!(replay.message, item.message);
        assert!(
            matches!(replay.kind, UnifiedInputItemKind::Reasoning { encrypted_content: Some(data), content: None, .. } if data == "opaque")
        );
    }
}
