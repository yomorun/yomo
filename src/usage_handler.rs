use async_trait::async_trait;
use log::warn;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::BTreeMap;

use crate::model_api_provider::{
    AudioSpeechUsage, AudioTranscriptionsUsage, EmbeddingsUsage, GenerateContentUsage, ImagesUsage,
    MessagesUsage, RerankUsage, ResponsesUsage,
};
use crate::openai_types::{CompletionTokensDetails, PromptTokensDetails, Usage as OpenAIUsage};
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
}

impl EndpointUsage {
    pub fn from_endpoint_payload(endpoint: &str, payload: Value) -> Result<Self, String> {
        match endpoint {
            "/chat/completions" => {
                if let Ok(usage) = serde_json::from_value::<OpenAIUsage>(payload.clone()) {
                    return Ok(Self::ChatCompletions(usage));
                }
            }
            "/messages" => {
                if let Ok(usage) = serde_json::from_value::<MessagesUsage>(payload.clone()) {
                    return Ok(Self::Messages(usage));
                }
            }
            "/responses" => {
                if let Ok(usage) = serde_json::from_value::<ResponsesUsage>(payload.clone()) {
                    return Ok(Self::Responses(usage));
                }
            }
            "/embeddings" => {
                if let Ok(usage) = serde_json::from_value::<EmbeddingsUsage>(payload.clone()) {
                    return Ok(Self::Embeddings(usage));
                }
            }
            "/rerank" => {
                if let Ok(usage) = serde_json::from_value::<RerankUsage>(payload.clone()) {
                    return Ok(Self::Rerank(usage));
                }
            }
            "/audio/speech" => {
                if let Ok(usage) = serde_json::from_value::<AudioSpeechUsage>(payload.clone()) {
                    return Ok(Self::AudioSpeech(usage));
                }
            }
            "/audio/transcriptions" => {
                if let Ok(usage) =
                    serde_json::from_value::<AudioTranscriptionsUsage>(payload.clone())
                {
                    return Ok(Self::AudioTranscriptions(usage));
                }
            }
            "/images/generations" | "/images/edits" => {
                if let Ok(usage) = serde_json::from_value::<ImagesUsage>(payload.clone()) {
                    return Ok(Self::Images(usage));
                }
            }
            _ => {
                if endpoint.starts_with("/models/") && endpoint.ends_with(":generateContent") {
                    if let Ok(usage) =
                        serde_json::from_value::<GenerateContentUsage>(payload.clone())
                    {
                        return Ok(Self::GenerateContent(usage));
                    }
                }
            }
        }
        Err(format!(
            "failed to parse endpoint usage; endpoint={endpoint}; payload={}",
            format_payload_for_log(&payload)
        ))
    }

    pub fn into_payload(self, endpoint: &str) -> Value {
        if endpoint == "/chat/completions" {
            if let Some(usage) = self.to_openai_usage() {
                return serde_json::to_value(usage).unwrap_or(Value::Null);
            }
            return Value::Null;
        }

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
        }
    }

    pub fn raw_payload(&self) -> Value {
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
        }
    }

    pub(crate) fn to_openai_usage(&self) -> Option<OpenAIUsage> {
        match self {
            Self::ChatCompletions(usage) => Some(usage.clone()),
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
                Some(OpenAIUsage {
                    prompt_tokens: input_tokens,
                    completion_tokens: output_tokens,
                    total_tokens: input_tokens.checked_add(output_tokens)?,
                    prompt_tokens_details: Some(PromptTokensDetails {
                        audio_tokens: 0,
                        cached_tokens: cache_read,
                    }),
                    completion_tokens_details: Some(CompletionTokensDetails {
                        accepted_prediction_tokens: 0,
                        audio_tokens: 0,
                        reasoning_tokens: 0,
                        rejected_prediction_tokens: 0,
                    }),
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
                let input_tokens = usage.input_tokens.unwrap_or(0);
                let output_tokens = usage.output_tokens.unwrap_or(0);
                let total_tokens = usage
                    .total_tokens
                    .or_else(|| input_tokens.checked_add(output_tokens))
                    .unwrap_or(input_tokens.saturating_add(output_tokens));
                Some(OpenAIUsage {
                    prompt_tokens: input_tokens,
                    completion_tokens: output_tokens,
                    total_tokens,
                    prompt_tokens_details: Some(PromptTokensDetails {
                        audio_tokens: 0,
                        cached_tokens: usage
                            .input_tokens_details
                            .as_ref()
                            .and_then(|details| details.cached_tokens)
                            .unwrap_or(0),
                    }),
                    completion_tokens_details: Some(CompletionTokensDetails {
                        accepted_prediction_tokens: 0,
                        audio_tokens: 0,
                        reasoning_tokens: usage
                            .output_tokens_details
                            .as_ref()
                            .and_then(|details| details.reasoning_tokens)
                            .unwrap_or(0),
                        rejected_prediction_tokens: 0,
                    }),
                })
            }
            Self::GenerateContent(usage) => {
                if usage.prompt_token_count.is_none()
                    && usage.candidates_token_count.is_none()
                    && usage.cached_content_token_count.is_none()
                    && usage.tool_use_prompt_token_count.is_none()
                    && usage.thoughts_token_count.is_none()
                    && usage.total_token_count.is_none()
                    && usage.cache_tokens_details.is_none()
                    && usage.prompt_tokens_details.is_none()
                    && usage.candidates_tokens_details.is_none()
                    && usage.tool_use_prompt_tokens_details.is_none()
                    && usage.traffic_type.is_none()
                {
                    return None;
                }
                let input_tokens = usage.prompt_token_count.unwrap_or(0);
                let output_tokens = usage.candidates_token_count.unwrap_or(0);
                let total_tokens = usage
                    .total_token_count
                    .or_else(|| input_tokens.checked_add(output_tokens))
                    .unwrap_or(input_tokens.saturating_add(output_tokens));
                Some(OpenAIUsage {
                    prompt_tokens: input_tokens,
                    completion_tokens: output_tokens,
                    total_tokens,
                    prompt_tokens_details: Some(PromptTokensDetails {
                        audio_tokens: 0,
                        cached_tokens: usage.cached_content_token_count.unwrap_or(0),
                    }),
                    completion_tokens_details: Some(CompletionTokensDetails {
                        accepted_prediction_tokens: 0,
                        audio_tokens: 0,
                        reasoning_tokens: usage.thoughts_token_count.unwrap_or(0),
                        rejected_prediction_tokens: 0,
                    }),
                })
            }
            Self::Embeddings(usage) => {
                if usage.prompt_tokens.is_none() && usage.total_tokens.is_none() {
                    return None;
                }
                let input_tokens = usage.prompt_tokens.unwrap_or(0);
                let total_tokens = usage.total_tokens.unwrap_or(input_tokens);
                Some(OpenAIUsage {
                    prompt_tokens: input_tokens,
                    completion_tokens: 0,
                    total_tokens,
                    prompt_tokens_details: Some(PromptTokensDetails {
                        audio_tokens: 0,
                        cached_tokens: 0,
                    }),
                    completion_tokens_details: Some(CompletionTokensDetails {
                        accepted_prediction_tokens: 0,
                        audio_tokens: 0,
                        reasoning_tokens: 0,
                        rejected_prediction_tokens: 0,
                    }),
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
                let input_tokens = ceil_f64_to_i64(usage.input_tokens).unwrap_or(0);
                let output_tokens = ceil_f64_to_i64(usage.output_tokens).unwrap_or(0);
                Some(OpenAIUsage {
                    prompt_tokens: input_tokens,
                    completion_tokens: output_tokens,
                    total_tokens: input_tokens.saturating_add(output_tokens),
                    prompt_tokens_details: Some(PromptTokensDetails {
                        audio_tokens: 0,
                        cached_tokens: ceil_f64_to_i64(usage.cached_tokens).unwrap_or(0),
                    }),
                    completion_tokens_details: Some(CompletionTokensDetails {
                        accepted_prediction_tokens: 0,
                        audio_tokens: 0,
                        reasoning_tokens: 0,
                        rejected_prediction_tokens: 0,
                    }),
                })
            }
            Self::AudioSpeech(usage) => {
                if usage.input_tokens.is_none()
                    && usage.output_tokens.is_none()
                    && usage.total_tokens.is_none()
                {
                    return None;
                }
                let input_tokens = usage.input_tokens.unwrap_or(0);
                let output_tokens = usage.output_tokens.unwrap_or(0);
                let total_tokens = usage
                    .total_tokens
                    .or_else(|| input_tokens.checked_add(output_tokens))
                    .unwrap_or(input_tokens.saturating_add(output_tokens));
                Some(OpenAIUsage {
                    prompt_tokens: input_tokens,
                    completion_tokens: output_tokens,
                    total_tokens,
                    prompt_tokens_details: Some(PromptTokensDetails {
                        audio_tokens: 0,
                        cached_tokens: 0,
                    }),
                    completion_tokens_details: Some(CompletionTokensDetails {
                        accepted_prediction_tokens: 0,
                        audio_tokens: 0,
                        reasoning_tokens: 0,
                        rejected_prediction_tokens: 0,
                    }),
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
                let input_tokens = usage.input_tokens.unwrap_or(0);
                let output_tokens = usage.output_tokens.unwrap_or(0);
                let total_tokens = usage
                    .total_tokens
                    .or_else(|| input_tokens.checked_add(output_tokens))
                    .unwrap_or(input_tokens.saturating_add(output_tokens));
                Some(OpenAIUsage {
                    prompt_tokens: input_tokens,
                    completion_tokens: output_tokens,
                    total_tokens,
                    prompt_tokens_details: Some(PromptTokensDetails {
                        audio_tokens: usage
                            .input_token_details
                            .as_ref()
                            .and_then(|details| details.audio_tokens)
                            .unwrap_or(0),
                        cached_tokens: 0,
                    }),
                    completion_tokens_details: Some(CompletionTokensDetails {
                        accepted_prediction_tokens: 0,
                        audio_tokens: 0,
                        reasoning_tokens: 0,
                        rejected_prediction_tokens: 0,
                    }),
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
                let input_tokens = usage.input_tokens.unwrap_or(0);
                let output_tokens = usage.output_tokens.unwrap_or(0);
                let total_tokens = usage
                    .total_tokens
                    .or_else(|| input_tokens.checked_add(output_tokens))
                    .unwrap_or(input_tokens.saturating_add(output_tokens));
                Some(OpenAIUsage {
                    prompt_tokens: input_tokens,
                    completion_tokens: output_tokens,
                    total_tokens,
                    prompt_tokens_details: Some(PromptTokensDetails {
                        audio_tokens: 0,
                        cached_tokens: 0,
                    }),
                    completion_tokens_details: Some(CompletionTokensDetails {
                        accepted_prediction_tokens: 0,
                        audio_tokens: 0,
                        reasoning_tokens: 0,
                        rejected_prediction_tokens: 0,
                    }),
                })
            }
        }
    }
}

pub fn flatten_usage_quantities_for_usage(usage: &EndpointUsage) -> Vec<(String, i64)> {
    let payload = usage.raw_payload();
    if matches!(usage, EndpointUsage::GenerateContent(_)) {
        return flatten_generate_content_usage_quantities(&payload);
    }

    flatten_usage_quantities(&payload)
}

fn flatten_usage_quantities(payload: &Value) -> Vec<(String, i64)> {
    let mut out = Vec::new();
    collect_usage_quantities(String::new(), payload, &mut out, array_index_usage_key);
    out
}

fn flatten_generate_content_usage_quantities(payload: &Value) -> Vec<(String, i64)> {
    let mut out = Vec::new();
    collect_usage_quantities(String::new(), payload, &mut out, array_modality_usage_key);

    merge_usage_quantities(out)
}

fn merge_usage_quantities(entries: Vec<(String, i64)>) -> Vec<(String, i64)> {
    let mut merged: BTreeMap<String, i64> = BTreeMap::new();
    for (path, quantity) in entries {
        merged
            .entry(path)
            .and_modify(|existing| *existing = existing.saturating_add(quantity))
            .or_insert(quantity);
    }

    merged.into_iter().collect()
}

fn collect_usage_quantities(
    path: String,
    value: &Value,
    out: &mut Vec<(String, i64)>,
    array_key_resolver: fn(usize, &Value) -> String,
) {
    match value {
        Value::Null | Value::Bool(_) | Value::String(_) => {}
        Value::Number(number) => {
            if let Some(quantity) = quantity_from_json_number(number) {
                if !path.is_empty() {
                    out.push((path, quantity));
                }
            }
        }
        Value::Array(items) => {
            for (index, item) in items.iter().enumerate() {
                let array_key = array_key_resolver(index, item);
                let next_path = if path.is_empty() {
                    array_key
                } else {
                    format!("{path}.{array_key}")
                };
                collect_usage_quantities(next_path, item, out, array_key_resolver);
            }
        }
        Value::Object(map) => {
            for (key, item) in map {
                let next_path = if path.is_empty() {
                    key.to_string()
                } else {
                    format!("{path}.{key}")
                };
                collect_usage_quantities(next_path, item, out, array_key_resolver);
            }
        }
    }
}

fn array_index_usage_key(index: usize, _item: &Value) -> String {
    index.to_string()
}

fn array_modality_usage_key(index: usize, item: &Value) -> String {
    let Some(map) = item.as_object() else {
        return index.to_string();
    };

    let Some(modality) = map.get("modality").and_then(|value| value.as_str()) else {
        return index.to_string();
    };

    if modality.is_empty() {
        return index.to_string();
    }

    if modality == "MODALITY_UNSPECIFIED" {
        return "TEXT".to_string();
    }

    modality.to_string()
}

fn quantity_from_json_number(number: &serde_json::Number) -> Option<i64> {
    if let Some(value) = number.as_i64() {
        return (value > 0).then_some(value);
    }

    if let Some(value) = number.as_u64() {
        if value == 0 {
            return None;
        }
        return i64::try_from(value).ok();
    }

    let value = number.as_f64()?;
    if !value.is_finite() || value <= 0.0 {
        return None;
    }
    let rounded = value.ceil();
    if rounded > i64::MAX as f64 {
        return None;
    }
    Some(rounded as i64)
}

pub(crate) fn aggregate_to_openai(endpoint: &str, usages: &[Value]) -> OpenAIUsage {
    let mut total = OpenAIUsage {
        prompt_tokens: 0,
        completion_tokens: 0,
        total_tokens: 0,
        prompt_tokens_details: Some(PromptTokensDetails {
            audio_tokens: 0,
            cached_tokens: 0,
        }),
        completion_tokens_details: Some(CompletionTokensDetails {
            accepted_prediction_tokens: 0,
            audio_tokens: 0,
            reasoning_tokens: 0,
            rejected_prediction_tokens: 0,
        }),
    };

    for payload in usages {
        if payload.is_null() {
            continue;
        }
        let Some(usage) = endpoint_payload_to_openai(endpoint, payload) else {
            warn!(
                "unsupported usage payload; endpoint={endpoint}; payload={}",
                format_payload_for_log(payload)
            );
            continue;
        };
        accumulate_openai_usage(&mut total, &usage);
    }

    total
}

fn ceil_f64_to_i64(value: Option<f64>) -> Option<i64> {
    let value = value?;
    if !value.is_finite() {
        return None;
    }
    if value < 0.0 {
        warn!("negative usage token value received; clamped to zero");
        return Some(0);
    }
    Some(value.ceil() as i64)
}

fn format_payload_for_log(payload: &Value) -> String {
    let payload = payload.to_string();
    truncate_for_log(&payload)
}

fn endpoint_payload_to_openai(endpoint: &str, payload: &Value) -> Option<OpenAIUsage> {
    let endpoint_usage = EndpointUsage::from_endpoint_payload(endpoint, payload.clone()).ok()?;
    endpoint_usage.to_openai_usage()
}

fn accumulate_openai_usage(total: &mut OpenAIUsage, usage: &OpenAIUsage) {
    total.prompt_tokens += usage.prompt_tokens;
    total.completion_tokens += usage.completion_tokens;
    total.total_tokens += usage.total_tokens;

    let total_prompt = total
        .prompt_tokens_details
        .get_or_insert_with(PromptTokensDetails::default);
    let usage_prompt = usage
        .prompt_tokens_details
        .as_ref()
        .cloned()
        .unwrap_or_default();
    total_prompt.audio_tokens += usage_prompt.audio_tokens;
    total_prompt.cached_tokens += usage_prompt.cached_tokens;

    let total_completion = total
        .completion_tokens_details
        .get_or_insert_with(CompletionTokensDetails::default);
    let usage_completion = usage
        .completion_tokens_details
        .as_ref()
        .cloned()
        .unwrap_or_default();
    total_completion.accepted_prediction_tokens += usage_completion.accepted_prediction_tokens;
    total_completion.audio_tokens += usage_completion.audio_tokens;
    total_completion.reasoning_tokens += usage_completion.reasoning_tokens;
    total_completion.rejected_prediction_tokens += usage_completion.rejected_prediction_tokens;
}

#[async_trait]
pub trait UsageHandler<M>: Send + Sync {
    async fn on_usage(
        &self,
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
    use super::{EndpointUsage, flatten_usage_quantities_for_usage};
    use crate::model_api_provider::{
        GenerateContentUsage, MessagesUsage, ResponsesUsage, TrafficType,
    };
    use crate::openai_types::{PromptTokensDetails, Usage as OpenAIUsage};

    #[test]
    fn from_endpoint_payload_rejects_invalid_chat_completions_shape() {
        let result = EndpointUsage::from_endpoint_payload(
            "/chat/completions",
            serde_json::json!({
                "input_tokens": 11,
                "output_tokens": 7,
                "total_tokens": 18
            }),
        );

        assert!(result.is_err());
    }

    #[test]
    fn from_endpoint_payload_rejects_invalid_generate_content_shape() {
        let result = EndpointUsage::from_endpoint_payload(
            "/models/gemini-2.5:generateContent",
            serde_json::json!({
                "promptTokenCount": "invalid"
            }),
        );

        assert!(result.is_err());
    }

    #[test]
    fn generate_content_usage_parses_camel_case_usage_metadata() {
        let payload = serde_json::json!({
            "promptTokenCount": 11,
            "candidatesTokenCount": 7,
            "cachedContentTokenCount": 3,
            "toolUsePromptTokenCount": 2,
            "thoughtsTokenCount": 1,
            "totalTokenCount": 21,
            "promptTokensDetails": [
                {
                    "modality": "TEXT",
                    "tokenCount": 11
                }
            ],
            "trafficType": "ON_DEMAND"
        });

        let usage =
            EndpointUsage::from_endpoint_payload("/models/gemini-2.5:generateContent", payload)
                .expect("expected generate content usage");

        let EndpointUsage::GenerateContent(usage) = usage else {
            panic!("expected generate content usage");
        };
        assert_eq!(usage.prompt_token_count, Some(11));
        assert_eq!(usage.candidates_token_count, Some(7));
        assert_eq!(usage.cached_content_token_count, Some(3));
        assert_eq!(usage.tool_use_prompt_token_count, Some(2));
        assert_eq!(usage.thoughts_token_count, Some(1));
        assert_eq!(usage.total_token_count, Some(21));
        assert!(usage.prompt_tokens_details.is_some());
        assert!(usage.traffic_type.is_some());
    }

    #[test]
    fn to_openai_usage_for_generate_content_maps_cached_content_tokens() {
        let usage = EndpointUsage::GenerateContent(GenerateContentUsage {
            prompt_token_count: Some(11),
            candidates_token_count: Some(7),
            cached_content_token_count: Some(3),
            tool_use_prompt_token_count: Some(2),
            thoughts_token_count: Some(1),
            total_token_count: Some(21),
            cache_tokens_details: None,
            prompt_tokens_details: None,
            candidates_tokens_details: None,
            tool_use_prompt_tokens_details: None,
            traffic_type: None,
        });

        let mapped = usage.to_openai_usage().expect("usage must map");

        assert_eq!(mapped.prompt_tokens, 11);
        assert_eq!(mapped.completion_tokens, 7);
        assert_eq!(mapped.total_tokens, 21);
        assert_eq!(
            mapped
                .prompt_tokens_details
                .as_ref()
                .map(|details| details.cached_tokens),
            Some(3)
        );
        assert_eq!(
            mapped
                .completion_tokens_details
                .as_ref()
                .map(|details| details.reasoning_tokens),
            Some(1)
        );
    }

    #[test]
    fn raw_payload_for_generate_content_preserves_usage_metadata_shape() {
        let usage = EndpointUsage::GenerateContent(GenerateContentUsage {
            prompt_token_count: Some(120),
            candidates_token_count: Some(80),
            cached_content_token_count: Some(24),
            tool_use_prompt_token_count: Some(16),
            thoughts_token_count: Some(4),
            total_token_count: Some(220),
            cache_tokens_details: None,
            prompt_tokens_details: None,
            candidates_tokens_details: None,
            tool_use_prompt_tokens_details: None,
            traffic_type: None,
        });

        let payload = usage.raw_payload();

        assert_eq!(payload["promptTokenCount"], 120);
        assert_eq!(payload["candidatesTokenCount"], 80);
        assert!(payload.get("prompt_tokens").is_none());
    }

    #[test]
    fn flatten_usage_quantities_for_generate_content_uses_modality_paths() {
        let usage = EndpointUsage::from_endpoint_payload(
            "/models/gemini-2.5:generateContent",
            serde_json::json!({
                "promptTokenCount": 120,
                "promptTokensDetails": [
                    {"modality": "TEXT", "tokenCount": 100},
                    {"modality": "TEXT", "tokenCount": 2},
                    {"modality": "IMAGE", "tokenCount": 20},
                    {"modality": "MODALITY_UNSPECIFIED", "tokenCount": 7}
                ]
            }),
        )
        .expect("expected generate content usage");

        let mut entries = flatten_usage_quantities_for_usage(&usage);
        entries.sort_by(|left, right| left.0.cmp(&right.0));

        let mut expected = vec![
            ("promptTokenCount".to_string(), 120),
            ("promptTokensDetails.IMAGE.tokenCount".to_string(), 20),
            ("promptTokensDetails.TEXT.tokenCount".to_string(), 109),
        ];
        expected.sort_by(|left, right| left.0.cmp(&right.0));

        assert_eq!(entries, expected);
    }

    #[test]
    fn into_payload_for_chat_completions_maps_generate_content_usage() {
        let payload = EndpointUsage::GenerateContent(GenerateContentUsage {
            prompt_token_count: None,
            candidates_token_count: None,
            cached_content_token_count: None,
            tool_use_prompt_token_count: None,
            thoughts_token_count: None,
            total_token_count: None,
            cache_tokens_details: None,
            prompt_tokens_details: None,
            candidates_tokens_details: None,
            tool_use_prompt_tokens_details: None,
            traffic_type: Some(TrafficType::OnDemand),
        })
        .into_payload("/chat/completions");

        assert_eq!(payload["prompt_tokens"], 0);
        assert_eq!(payload["completion_tokens"], 0);
        assert_eq!(payload["total_tokens"], 0);
        assert_eq!(payload["prompt_tokens_details"]["cached_tokens"], 0);
        assert_eq!(payload["completion_tokens_details"]["reasoning_tokens"], 0);
    }

    #[test]
    fn to_openai_usage_for_chat_completions_preserves_audio_and_cache() {
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

        let mapped = usage.to_openai_usage().expect("usage must map");

        assert_eq!(mapped.prompt_tokens, 10);
        assert_eq!(mapped.completion_tokens, 4);
        assert_eq!(mapped.total_tokens, 14);
        assert_eq!(
            mapped
                .prompt_tokens_details
                .as_ref()
                .map(|details| details.cached_tokens),
            Some(2)
        );
        assert_eq!(
            mapped
                .prompt_tokens_details
                .as_ref()
                .map(|details| details.audio_tokens),
            Some(3)
        );
    }

    #[test]
    fn to_openai_usage_for_messages_includes_cache_creation_and_read() {
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

        let mapped = usage.to_openai_usage().expect("usage must map");

        assert_eq!(mapped.prompt_tokens, 15);
        assert_eq!(mapped.completion_tokens, 2);
        assert_eq!(mapped.total_tokens, 17);
        assert_eq!(
            mapped
                .prompt_tokens_details
                .as_ref()
                .map(|details| details.cached_tokens),
            Some(3)
        );
    }

    #[test]
    fn to_openai_usage_for_responses_maps_totals_and_reasoning() {
        let usage = EndpointUsage::Responses(ResponsesUsage {
            input_tokens: Some(11),
            input_tokens_details: None,
            output_tokens: Some(6),
            output_tokens_details: Some(crate::model_api_provider::ResponsesOutputTokensDetails {
                reasoning_tokens: Some(4),
            }),
            total_tokens: Some(17),
        });

        let mapped = usage.to_openai_usage().expect("usage must map");

        assert_eq!(mapped.prompt_tokens, 11);
        assert_eq!(mapped.completion_tokens, 6);
        assert_eq!(mapped.total_tokens, 17);
        assert_eq!(
            mapped
                .completion_tokens_details
                .as_ref()
                .map(|details| details.reasoning_tokens),
            Some(4)
        );
    }
}
