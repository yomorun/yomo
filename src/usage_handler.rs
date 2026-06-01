use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::llm_provider::{ToOpenAIUsage, parse_usage_payload};
use crate::model_api_provider::{
    AudioSpeechUsage, AudioTranscriptionsUsage, EmbeddingsUsage, GenerateContentUsage, ImagesUsage,
    MessagesUsage, RerankUsage, ResponsesUsage,
};
use crate::openai_types::Usage as OpenAIUsage;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", content = "payload", rename_all = "snake_case")]
pub enum EndpointUsage {
    ChatCompletions(OpenAIUsage),
    Messages(MessagesUsage),
    Responses(ResponsesUsage),
    GenerateContent(GenerateContentUsage),
    Embeddings(EmbeddingsUsage),
    Rerank(RerankUsage),
    AudioSpeech(AudioSpeechUsage),
    AudioTranscriptions(AudioTranscriptionsUsage),
    Images(ImagesUsage),
    Unknown(Value),
}

impl EndpointUsage {
    pub fn from_endpoint_payload(endpoint: &str, payload: Value) -> Self {
        match endpoint {
            "/chat/completions" => {
                if let Ok(usage) = serde_json::from_value::<OpenAIUsage>(payload.clone()) {
                    return Self::ChatCompletions(usage);
                }
                if let Some(usage) = parse_usage_payload(&payload) {
                    return Self::ChatCompletions(usage.to_openai_usage());
                }
            }
            "/messages" => {
                if let Ok(usage) = serde_json::from_value::<MessagesUsage>(payload.clone()) {
                    return Self::Messages(usage);
                }
            }
            "/responses" => {
                if let Ok(usage) = serde_json::from_value::<ResponsesUsage>(payload.clone()) {
                    return Self::Responses(usage);
                }
            }
            "/embeddings" => {
                if let Ok(usage) = serde_json::from_value::<EmbeddingsUsage>(payload.clone()) {
                    return Self::Embeddings(usage);
                }
            }
            "/rerank" => {
                if let Ok(usage) = serde_json::from_value::<RerankUsage>(payload.clone()) {
                    return Self::Rerank(usage);
                }
            }
            "/audio/speech" => {
                if let Ok(usage) = serde_json::from_value::<AudioSpeechUsage>(payload.clone()) {
                    return Self::AudioSpeech(usage);
                }
            }
            "/audio/transcriptions" => {
                if let Ok(usage) =
                    serde_json::from_value::<AudioTranscriptionsUsage>(payload.clone())
                {
                    return Self::AudioTranscriptions(usage);
                }
            }
            "/images/generations" | "/images/edits" => {
                if let Ok(usage) = serde_json::from_value::<ImagesUsage>(payload.clone()) {
                    return Self::Images(usage);
                }
            }
            _ => {
                if endpoint.starts_with("/models/") && endpoint.ends_with(":generateContent") {
                    if let Ok(usage) =
                        serde_json::from_value::<GenerateContentUsage>(payload.clone())
                    {
                        return Self::GenerateContent(usage);
                    }
                }
            }
        }
        Self::Unknown(payload)
    }

    pub fn into_payload(self, endpoint: &str) -> Value {
        match self {
            Self::ChatCompletions(usage) => serde_json::to_value(usage).unwrap_or(Value::Null),
            Self::Messages(usage) => serde_json::to_value(usage).unwrap_or(Value::Null),
            Self::Responses(usage) => serde_json::to_value(usage).unwrap_or(Value::Null),
            Self::GenerateContent(usage) => serde_json::to_value(usage).unwrap_or(Value::Null),
            Self::Embeddings(usage) => serde_json::to_value(usage).unwrap_or(Value::Null),
            Self::Rerank(usage) => serde_json::to_value(usage).unwrap_or(Value::Null),
            Self::AudioSpeech(usage) => serde_json::to_value(usage).unwrap_or(Value::Null),
            Self::AudioTranscriptions(usage) => serde_json::to_value(usage).unwrap_or(Value::Null),
            Self::Images(usage) => serde_json::to_value(usage).unwrap_or(Value::Null),
            Self::Unknown(usage) => map_unknown_for_endpoint(endpoint, usage),
        }
    }
}

fn map_unknown_for_endpoint(endpoint: &str, payload: Value) -> Value {
    let Some(usage) = parse_usage_payload(&payload) else {
        return payload;
    };
    match endpoint {
        "/chat/completions" => serde_json::to_value(usage.to_openai_usage()).unwrap_or(Value::Null),
        "/embeddings" => serde_json::json!({
            "prompt_tokens": usage.input_tokens,
            "total_tokens": usage.total_tokens,
        }),
        "/messages"
        | "/responses"
        | "/audio/speech"
        | "/audio/transcriptions"
        | "/images/generations"
        | "/images/edits"
        | "/rerank" => {
            serde_json::json!({
                "input_tokens": usage.input_tokens,
                "output_tokens": usage.output_tokens,
                "total_tokens": usage.total_tokens,
                "cached_tokens": usage.cached_tokens,
                "reasoning_tokens": usage.reasoning_tokens,
            })
        }
        _ if endpoint.starts_with("/models/") && endpoint.ends_with(":generateContent") => {
            serde_json::json!({
                "prompt_token_count": usage.input_tokens,
                "candidates_token_count": usage.output_tokens,
                "total_token_count": usage.total_tokens,
            })
        }
        _ => serde_json::json!({
            "input_tokens": usage.input_tokens,
            "output_tokens": usage.output_tokens,
            "total_tokens": usage.total_tokens,
            "cached_tokens": usage.cached_tokens,
            "reasoning_tokens": usage.reasoning_tokens,
        }),
    }
}

#[async_trait]
pub trait UsageHandler<M>: Send + Sync {
    async fn on_usage(
        &self,
        endpoint: &str,
        model_id: &str,
        label: Option<&str>,
        request_id: &str,
        trace_id: &str,
        metadata: M,
        usage: EndpointUsage,
    ) -> EndpointUsage;
}

#[derive(Clone, Default)]
pub struct NoopUsageHandler;

#[async_trait]
impl<M> UsageHandler<M> for NoopUsageHandler
where
    M: Send + Sync + 'static,
{
    async fn on_usage(
        &self,
        _endpoint: &str,
        _model_id: &str,
        _label: Option<&str>,
        _request_id: &str,
        _trace_id: &str,
        _metadata: M,
        usage: EndpointUsage,
    ) -> EndpointUsage {
        usage
    }
}

#[cfg(test)]
mod tests {
    use super::EndpointUsage;

    #[test]
    fn input_output_maps_to_openai_for_chat_completions() {
        let payload = EndpointUsage::Unknown(serde_json::json!({
            "input_tokens": 11,
            "output_tokens": 7,
            "total_tokens": 18,
            "cached_tokens": 2,
            "reasoning_tokens": 1
        }))
        .into_payload("/chat/completions");

        assert_eq!(
            payload.get("prompt_tokens").and_then(|v| v.as_i64()),
            Some(11)
        );
        assert_eq!(
            payload.get("completion_tokens").and_then(|v| v.as_i64()),
            Some(7)
        );
    }

    #[test]
    fn input_output_maps_to_generate_content_shape() {
        let payload = EndpointUsage::Unknown(serde_json::json!({
            "input_tokens": 11,
            "output_tokens": 7,
            "total_tokens": 18
        }))
        .into_payload("/models/gemini-2.5:generateContent");

        assert_eq!(
            payload.get("prompt_token_count").and_then(|v| v.as_i64()),
            Some(11)
        );
        assert_eq!(
            payload
                .get("candidates_token_count")
                .and_then(|v| v.as_i64()),
            Some(7)
        );
    }
}
