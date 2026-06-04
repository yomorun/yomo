use async_trait::async_trait;
use axum::http::StatusCode;
use futures_core::Stream;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use serde_json::json;
use std::pin::Pin;

use crate::openai_types::{
    ChatCompletionRequest, CompletionTokensDetails, ErrorDetail, PromptTokensDetails, Usage,
};

#[derive(Debug, Clone, Default)]
pub struct UsageSummary {
    pub input_tokens: i64,
    pub output_tokens: i64,
    pub total_tokens: i64,
    pub cached_tokens: Option<i64>,
    pub reasoning_tokens: Option<i64>,
    pub input_audio_tokens: Option<i64>,
    pub output_audio_tokens: Option<i64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct InputOutputUsage {
    pub input_tokens: i64,
    pub output_tokens: i64,
    pub total_tokens: i64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cached_tokens: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reasoning_tokens: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub input_audio_tokens: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub output_audio_tokens: Option<i64>,
}

/// Aggregates usage payloads into a provider-neutral summary.
pub trait UsageAccumulator {
    fn accumulate_into(&self, total: &mut UsageSummary);
}

/// Maps endpoint usage payloads into OpenAI chat usage format.
pub trait ToOpenAIUsage {
    fn to_openai_usage(&self) -> Usage;
}

impl UsageAccumulator for Usage {
    fn accumulate_into(&self, total: &mut UsageSummary) {
        total.input_tokens += i64::from(self.prompt_tokens);
        total.output_tokens += i64::from(self.completion_tokens);
        total.total_tokens += i64::from(self.total_tokens);
        if let Some(details) = &self.prompt_tokens_details {
            total.cached_tokens =
                Some(total.cached_tokens.unwrap_or(0) + i64::from(details.cached_tokens));
            total.input_audio_tokens =
                Some(total.input_audio_tokens.unwrap_or(0) + i64::from(details.audio_tokens));
        }
        if let Some(details) = &self.completion_tokens_details {
            total.reasoning_tokens =
                Some(total.reasoning_tokens.unwrap_or(0) + i64::from(details.reasoning_tokens));
            total.output_audio_tokens =
                Some(total.output_audio_tokens.unwrap_or(0) + i64::from(details.audio_tokens));
        }
    }
}

impl ToOpenAIUsage for Usage {
    fn to_openai_usage(&self) -> Usage {
        self.clone()
    }
}

impl UsageAccumulator for InputOutputUsage {
    fn accumulate_into(&self, total: &mut UsageSummary) {
        total.input_tokens += self.input_tokens;
        total.output_tokens += self.output_tokens;
        total.total_tokens += self.total_tokens;
        if let Some(cached_tokens) = self.cached_tokens {
            total.cached_tokens = Some(total.cached_tokens.unwrap_or(0) + cached_tokens);
        }
        if let Some(reasoning_tokens) = self.reasoning_tokens {
            total.reasoning_tokens = Some(total.reasoning_tokens.unwrap_or(0) + reasoning_tokens);
        }
        if let Some(input_audio_tokens) = self.input_audio_tokens {
            total.input_audio_tokens =
                Some(total.input_audio_tokens.unwrap_or(0) + input_audio_tokens);
        }
        if let Some(output_audio_tokens) = self.output_audio_tokens {
            total.output_audio_tokens =
                Some(total.output_audio_tokens.unwrap_or(0) + output_audio_tokens);
        }
    }
}

impl ToOpenAIUsage for InputOutputUsage {
    fn to_openai_usage(&self) -> Usage {
        Usage {
            prompt_tokens: self.input_tokens as i32,
            completion_tokens: self.output_tokens as i32,
            total_tokens: self.total_tokens as i32,
            prompt_tokens_details: Some(PromptTokensDetails {
                audio_tokens: self.input_audio_tokens.unwrap_or(0) as i32,
                cached_tokens: self.cached_tokens.unwrap_or(0) as i32,
            }),
            completion_tokens_details: Some(CompletionTokensDetails {
                accepted_prediction_tokens: 0,
                audio_tokens: self.output_audio_tokens.unwrap_or(0) as i32,
                reasoning_tokens: self.reasoning_tokens.unwrap_or(0) as i32,
                rejected_prediction_tokens: 0,
            }),
        }
    }
}

pub fn usage_summary_to_value(summary: &UsageSummary) -> Value {
    json!({
        "input_tokens": summary.input_tokens,
        "output_tokens": summary.output_tokens,
        "total_tokens": summary.total_tokens,
        "cached_tokens": summary.cached_tokens,
        "reasoning_tokens": summary.reasoning_tokens,
        "input_audio_tokens": summary.input_audio_tokens,
        "output_audio_tokens": summary.output_audio_tokens,
    })
}

#[cfg(test)]
mod tests {
    use super::{InputOutputUsage, ToOpenAIUsage};

    #[test]
    fn maps_input_output_usage_to_openai_usage() {
        let usage = InputOutputUsage {
            input_tokens: 10,
            output_tokens: 5,
            total_tokens: 15,
            cached_tokens: Some(2),
            reasoning_tokens: Some(1),
            input_audio_tokens: Some(4),
            output_audio_tokens: Some(7),
        };
        let mapped = usage.to_openai_usage();
        assert_eq!(mapped.prompt_tokens, 10);
        assert_eq!(mapped.completion_tokens, 5);
        assert_eq!(mapped.total_tokens, 15);
        assert_eq!(
            mapped
                .prompt_tokens_details
                .as_ref()
                .expect("prompt details")
                .cached_tokens,
            2
        );
        assert_eq!(
            mapped
                .prompt_tokens_details
                .as_ref()
                .expect("prompt details")
                .audio_tokens,
            4
        );
        assert_eq!(
            mapped
                .completion_tokens_details
                .as_ref()
                .expect("completion details")
                .reasoning_tokens,
            1
        );
        assert_eq!(
            mapped
                .completion_tokens_details
                .as_ref()
                .expect("completion details")
                .audio_tokens,
            7
        );
    }
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
    pub usage: Value,
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
        usage: Value,
    },
    Completed {
        finish_reason: Option<String>,
        usage: Option<Value>,
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
    Internal(String),
}

impl std::fmt::Display for ProviderError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ProviderError::Public { error, .. } => {
                write!(f, "provider error: {}", error.message)
            }
            ProviderError::Internal(message) => write!(f, "provider error: {message}"),
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
