use async_trait::async_trait;
use axum::http::StatusCode;
use futures_core::Stream;
use serde::{Deserialize, Serialize};
use std::pin::Pin;

use crate::openai_types::{ChatCompletionRequest, ErrorDetail};
use crate::usage_handler::EndpointUsage;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum FinishReason {
    Stop,
    Length,
    ToolCalls,
    ContentFilter,
    Other,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolCall {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub id: Option<String>,
    pub name: String,
    pub description: String,
    pub arguments: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UnifiedResponse {
    pub request_id: String,
    pub created_at: String,
    pub model: String,
    pub output_text: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_calls: Option<Vec<ToolCall>>,
    pub finish_reason: FinishReason,
    pub usage: EndpointUsage,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum UnifiedEvent {
    ResponseCreated {
        id: String,
        model: String,
        created_at: String,
    },
    ResponseInProgress {
        id: String,
        model: String,
        created_at: String,
    },
    OutputItemAdded {
        id: String,
        item_type: String,
    },
    OutputItemDone {
        id: String,
        item_type: String,
    },
    ContentPartAdded {
        item_id: String,
        part_type: String,
    },
    ContentPartDelta {
        item_id: String,
        part_type: String,
        delta: String,
    },
    ContentPartDone {
        item_id: String,
        part_type: String,
    },
    ThinkingDelta {
        id: String,
        delta: String,
    },
    ThinkingDone {
        id: String,
        summary: Option<String>,
    },
    ToolCallDelta {
        id: String,
        name: String,
        arguments_delta: String,
    },
    ToolCallDone {
        id: String,
        name: String,
        arguments: String,
    },
    ServerToolCall {
        tool_call_id: String,
        name: String,
        arguments: String,
    },
    ServerToolCallResult {
        tool_call_id: String,
        name: String,
        #[serde(skip_serializing_if = "Option::is_none")]
        result: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        error: Option<String>,
    },
    MessageStart {
        id: String,
        role: String,
    },
    MessageDelta {
        id: String,
        delta: String,
    },
    MessageStop {
        id: String,
        stop_reason: Option<String>,
    },
    Usage {
        usage: EndpointUsage,
    },
    Completed {
        finish_reason: Option<String>,
    },
    Failed {
        code: String,
        message: String,
    },
    Cancelled {
        reason: String,
    },
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
            ProviderError::Public { error, .. } => {
                write!(f, "provider error: {}", error.message)
            }
            ProviderError::Internal { message, .. } => write!(f, "provider error: {message}"),
        }
    }
}

impl ProviderError {
    pub fn internal(message: impl Into<String>) -> Self {
        Self::Internal {
            upstream_http_status: StatusCode::INTERNAL_SERVER_ERROR,
            message: message.into(),
        }
    }

    pub fn internal_with_upstream_status(status: StatusCode, message: impl Into<String>) -> Self {
        Self::Internal {
            upstream_http_status: status,
            message: message.into(),
        }
    }
}

impl std::error::Error for ProviderError {}

#[async_trait]
pub trait Provider: Send + Sync {
    fn model_id(&self) -> &str;

    async fn complete(
        &self,
        request: ChatCompletionRequest,
    ) -> Result<UnifiedResponse, ProviderError>;

    async fn stream<'a>(
        &'a self,
        request: ChatCompletionRequest,
    ) -> Result<
        Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    >;
}
