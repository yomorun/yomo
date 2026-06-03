use async_trait::async_trait;
use log::warn;
use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::llm_provider::ToOpenAIUsage;
use crate::llm_provider::provider::InputOutputUsage;
use crate::model_api_provider::{
    AudioSpeechUsage, AudioTranscriptionsUsage, EmbeddingsUsage, GenerateContentUsage, ImagesUsage,
    MessagesUsage, RerankUsage, ResponsesUsage,
};
use crate::openai_types::Usage as OpenAIUsage;
use crate::utils::truncate_for_log;

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

    pub(crate) fn to_input_output_usage(&self) -> Option<InputOutputUsage> {
        match self {
            Self::ChatCompletions(usage) => Some(InputOutputUsage {
                input_tokens: i64::from(usage.prompt_tokens),
                output_tokens: i64::from(usage.completion_tokens),
                total_tokens: i64::from(usage.total_tokens),
                cached_tokens: usage
                    .prompt_tokens_details
                    .as_ref()
                    .map(|details| i64::from(details.cached_tokens)),
                reasoning_tokens: usage
                    .completion_tokens_details
                    .as_ref()
                    .map(|details| i64::from(details.reasoning_tokens)),
                input_audio_tokens: usage
                    .prompt_tokens_details
                    .as_ref()
                    .map(|details| i64::from(details.audio_tokens)),
                output_audio_tokens: usage
                    .completion_tokens_details
                    .as_ref()
                    .map(|details| i64::from(details.audio_tokens)),
            }),
            Self::Messages(usage) => {
                if usage.input_tokens.is_none()
                    && usage.output_tokens.is_none()
                    && usage.cache_creation_input_tokens.is_none()
                    && usage.cache_read_input_tokens.is_none()
                    && usage.cache_creation.is_none()
                {
                    return None;
                }
                let base_input = usage.input_tokens.unwrap_or(0);
                let cache_creation = usage
                    .cache_creation_input_tokens
                    .or_else(|| {
                        usage.cache_creation.as_ref().and_then(|cache| {
                            let five_min = cache.ephemeral_5m_input_tokens.unwrap_or(0);
                            let one_hour = cache.ephemeral_1h_input_tokens.unwrap_or(0);
                            five_min.checked_add(one_hour)
                        })
                    })
                    .unwrap_or(0);
                let cache_read = usage.cache_read_input_tokens.unwrap_or(0);
                let input_tokens = base_input
                    .checked_add(cache_creation)?
                    .checked_add(cache_read)?;
                let output_tokens = usage.output_tokens.unwrap_or(0);
                Some(InputOutputUsage {
                    input_tokens: i64::try_from(input_tokens).ok()?,
                    output_tokens: i64::try_from(output_tokens).ok()?,
                    total_tokens: i64::try_from(input_tokens.checked_add(output_tokens)?).ok()?,
                    cached_tokens: i64::try_from(cache_read).ok(),
                    reasoning_tokens: None,
                    input_audio_tokens: None,
                    output_audio_tokens: None,
                })
            }
            Self::Responses(usage) => {
                if usage.input_tokens.is_none()
                    && usage.output_tokens.is_none()
                    && usage.total_tokens.is_none()
                    && usage.input_tokens_details.is_none()
                    && usage.output_tokens_details.is_none()
                {
                    return None;
                }
                let input_tokens = i64::try_from(usage.input_tokens.unwrap_or(0)).ok()?;
                let output_tokens = i64::try_from(usage.output_tokens.unwrap_or(0)).ok()?;
                let total_tokens = usage
                    .total_tokens
                    .map(i64::try_from)
                    .transpose()
                    .ok()?
                    .unwrap_or(input_tokens + output_tokens);
                Some(InputOutputUsage {
                    input_tokens,
                    output_tokens,
                    total_tokens,
                    cached_tokens: usage
                        .input_tokens_details
                        .as_ref()
                        .and_then(|details| details.cached_tokens)
                        .and_then(|value| i64::try_from(value).ok()),
                    reasoning_tokens: usage
                        .output_tokens_details
                        .as_ref()
                        .and_then(|details| details.reasoning_tokens)
                        .and_then(|value| i64::try_from(value).ok()),
                    input_audio_tokens: None,
                    output_audio_tokens: None,
                })
            }
            Self::GenerateContent(usage) => {
                if usage.prompt_token_count.is_none()
                    && usage.candidates_token_count.is_none()
                    && usage.thoughts_token_count.is_none()
                    && usage.total_token_count.is_none()
                {
                    return None;
                }
                let input_tokens = i64::try_from(usage.prompt_token_count.unwrap_or(0)).ok()?;
                let output_tokens =
                    i64::try_from(usage.candidates_token_count.unwrap_or(0)).ok()?;
                let total_tokens = usage
                    .total_token_count
                    .map(i64::try_from)
                    .transpose()
                    .ok()?
                    .unwrap_or(input_tokens + output_tokens);
                Some(InputOutputUsage {
                    input_tokens,
                    output_tokens,
                    total_tokens,
                    cached_tokens: None,
                    reasoning_tokens: usage
                        .thoughts_token_count
                        .and_then(|value| i64::try_from(value).ok()),
                    input_audio_tokens: None,
                    output_audio_tokens: None,
                })
            }
            Self::Embeddings(usage) => {
                if usage.prompt_tokens.is_none() && usage.total_tokens.is_none() {
                    return None;
                }
                let input_tokens = i64::try_from(usage.prompt_tokens.unwrap_or(0)).ok()?;
                let total_tokens = usage
                    .total_tokens
                    .map(i64::try_from)
                    .transpose()
                    .ok()?
                    .unwrap_or(input_tokens);
                Some(InputOutputUsage {
                    input_tokens,
                    output_tokens: 0,
                    total_tokens,
                    cached_tokens: None,
                    reasoning_tokens: None,
                    input_audio_tokens: None,
                    output_audio_tokens: None,
                })
            }
            Self::Rerank(usage) => {
                if usage.input_tokens.is_none()
                    && usage.output_tokens.is_none()
                    && usage.cached_tokens.is_none()
                    && usage.billed_units.is_none()
                {
                    return None;
                }
                let input_tokens = ceil_f64_to_i64(usage.input_tokens)?;
                let output_tokens = ceil_f64_to_i64(usage.output_tokens).unwrap_or(0);
                Some(InputOutputUsage {
                    input_tokens,
                    output_tokens,
                    total_tokens: input_tokens + output_tokens,
                    cached_tokens: ceil_f64_to_i64(usage.cached_tokens),
                    reasoning_tokens: None,
                    input_audio_tokens: None,
                    output_audio_tokens: None,
                })
            }
            Self::AudioSpeech(usage) => {
                if usage.input_tokens.is_none()
                    && usage.output_tokens.is_none()
                    && usage.total_tokens.is_none()
                {
                    return None;
                }
                let input_tokens = i64::try_from(usage.input_tokens.unwrap_or(0)).ok()?;
                let output_tokens = i64::try_from(usage.output_tokens.unwrap_or(0)).ok()?;
                let total_tokens = usage
                    .total_tokens
                    .map(i64::try_from)
                    .transpose()
                    .ok()?
                    .unwrap_or(input_tokens + output_tokens);
                Some(InputOutputUsage {
                    input_tokens,
                    output_tokens,
                    total_tokens,
                    cached_tokens: None,
                    reasoning_tokens: None,
                    input_audio_tokens: None,
                    output_audio_tokens: None,
                })
            }
            Self::AudioTranscriptions(usage) => {
                if usage.input_tokens.is_none()
                    && usage.output_tokens.is_none()
                    && usage.total_tokens.is_none()
                    && usage.input_token_details.is_none()
                    && usage.seconds.is_none()
                    && usage.usage_type.is_none()
                {
                    return None;
                }
                let input_tokens = i64::try_from(usage.input_tokens.unwrap_or(0)).ok()?;
                let output_tokens = i64::try_from(usage.output_tokens.unwrap_or(0)).ok()?;
                let total_tokens = usage
                    .total_tokens
                    .map(i64::try_from)
                    .transpose()
                    .ok()?
                    .unwrap_or(input_tokens + output_tokens);
                Some(InputOutputUsage {
                    input_tokens,
                    output_tokens,
                    total_tokens,
                    cached_tokens: None,
                    reasoning_tokens: None,
                    input_audio_tokens: usage
                        .input_token_details
                        .as_ref()
                        .and_then(|details| details.audio_tokens)
                        .and_then(|value| i64::try_from(value).ok()),
                    output_audio_tokens: None,
                })
            }
            Self::Images(usage) => {
                if usage.input_tokens.is_none()
                    && usage.output_tokens.is_none()
                    && usage.total_tokens.is_none()
                    && usage.input_tokens_details.is_none()
                    && usage.output_tokens_details.is_none()
                {
                    return None;
                }
                let input_tokens = i64::try_from(usage.input_tokens.unwrap_or(0)).ok()?;
                let output_tokens = i64::try_from(usage.output_tokens.unwrap_or(0)).ok()?;
                let total_tokens = usage
                    .total_tokens
                    .map(i64::try_from)
                    .transpose()
                    .ok()?
                    .unwrap_or(input_tokens + output_tokens);
                Some(InputOutputUsage {
                    input_tokens,
                    output_tokens,
                    total_tokens,
                    cached_tokens: None,
                    reasoning_tokens: None,
                    input_audio_tokens: None,
                    output_audio_tokens: None,
                })
            }
            Self::Unknown(_) => None,
        }
    }
}

pub(crate) fn parse_endpoint_usage_as_input_output(
    endpoint: &str,
    payload: &Value,
    model_id: Option<&str>,
    trace_id: Option<&str>,
) -> Option<InputOutputUsage> {
    if payload.is_null() {
        return None;
    }

    let endpoint_usage = EndpointUsage::from_endpoint_payload(endpoint, payload.clone());
    if let Some(usage) = endpoint_usage.to_input_output_usage() {
        return Some(usage);
    }
    if let Ok(usage) = serde_json::from_value::<InputOutputUsage>(payload.clone()) {
        return Some(usage);
    }
    warn!(
        "unsupported usage payload; endpoint={endpoint}; model_id={}; trace_id={}; payload={}",
        model_id.unwrap_or(""),
        trace_id.unwrap_or(""),
        format_payload_for_log(payload)
    );
    Some(InputOutputUsage {
        input_tokens: 0,
        output_tokens: 0,
        total_tokens: 0,
        cached_tokens: None,
        reasoning_tokens: None,
        input_audio_tokens: None,
        output_audio_tokens: None,
    })
}

fn ceil_f64_to_i64(value: Option<f64>) -> Option<i64> {
    let value = value?;
    if !value.is_finite() || value < 0.0 {
        return None;
    }
    Some(value.ceil() as i64)
}

fn format_payload_for_log(payload: &Value) -> String {
    let payload = payload.to_string();
    truncate_for_log(&payload)
}

fn map_unknown_for_endpoint(endpoint: &str, payload: Value) -> Value {
    let Some(usage) = parse_endpoint_usage_as_input_output(endpoint, &payload, None, None) else {
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
    use crate::model_api_provider::{MessagesUsage, ResponsesUsage};
    use crate::openai_types::{PromptTokensDetails, Usage as OpenAIUsage};

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

    #[test]
    fn to_input_output_usage_for_chat_completions_preserves_audio_and_cache() {
        let usage = EndpointUsage::ChatCompletions(OpenAIUsage {
            prompt_tokens: 10,
            completion_tokens: 4,
            total_tokens: 14,
            prompt_tokens_details: Some(PromptTokensDetails {
                audio_tokens: 3,
                cached_tokens: 2,
            }),
            completion_tokens_details: None,
        });

        let mapped = usage.to_input_output_usage().expect("usage must map");

        assert_eq!(mapped.input_tokens, 10);
        assert_eq!(mapped.output_tokens, 4);
        assert_eq!(mapped.total_tokens, 14);
        assert_eq!(mapped.cached_tokens, Some(2));
        assert_eq!(mapped.input_audio_tokens, Some(3));
        assert_eq!(mapped.output_audio_tokens, None);
    }

    #[test]
    fn to_input_output_usage_for_messages_includes_cache_creation_and_read() {
        let usage = EndpointUsage::Messages(MessagesUsage {
            input_tokens: Some(5),
            output_tokens: Some(2),
            cache_creation_input_tokens: Some(7),
            cache_read_input_tokens: Some(3),
            cache_creation: None,
            inference_geo: None,
            service_tier: None,
            server_tool_use: None,
        });

        let mapped = usage.to_input_output_usage().expect("usage must map");

        assert_eq!(mapped.input_tokens, 15);
        assert_eq!(mapped.output_tokens, 2);
        assert_eq!(mapped.total_tokens, 17);
        assert_eq!(mapped.cached_tokens, Some(3));
    }

    #[test]
    fn to_input_output_usage_for_responses_maps_totals_and_reasoning() {
        let usage = EndpointUsage::Responses(ResponsesUsage {
            input_tokens: Some(11),
            input_tokens_details: None,
            output_tokens: Some(6),
            output_tokens_details: Some(crate::model_api_provider::ResponsesOutputTokensDetails {
                reasoning_tokens: Some(4),
            }),
            total_tokens: Some(17),
        });

        let mapped = usage.to_input_output_usage().expect("usage must map");

        assert_eq!(mapped.input_tokens, 11);
        assert_eq!(mapped.output_tokens, 6);
        assert_eq!(mapped.total_tokens, 17);
        assert_eq!(mapped.reasoning_tokens, Some(4));
    }
}
