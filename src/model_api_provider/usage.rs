use serde::{Deserialize, Serialize};
use serde_json::Value;

#[derive(Debug, Clone, Serialize)]
#[serde(tag = "endpoint", rename_all = "snake_case")]
pub enum Usage {
    Messages(MessagesUsage),
    Responses(ResponsesUsage),
    ChatCompletions(ChatCompletionsUsage),
    GenerateContent(GenerateContentUsage),
    Embeddings(EmbeddingsUsage),
    Rerank(RerankUsage),
    AudioSpeech(AudioSpeechUsage),
    AudioTranscriptions(AudioTranscriptionsUsage),
    Images(ImagesUsage),
    Unknown(UnknownUsage),
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MessagesUsage {
    pub input_tokens: Option<u64>,
    pub output_tokens: Option<u64>,
    pub cache_creation_input_tokens: Option<u64>,
    pub cache_read_input_tokens: Option<u64>,
    pub cache_creation: Option<MessagesCacheCreation>,
    pub inference_geo: Option<String>,
    pub service_tier: Option<String>,
    pub server_tool_use: Option<MessagesServerToolUse>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MessagesCacheCreation {
    #[serde(rename = "ephemeral_5m_input_tokens")]
    pub ephemeral_5m_input_tokens: Option<u64>,
    #[serde(rename = "ephemeral_1h_input_tokens")]
    pub ephemeral_1h_input_tokens: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MessagesServerToolUse {
    pub web_search_requests: Option<u64>,
    pub web_fetch_requests: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ResponsesUsage {
    pub input_tokens: Option<u64>,
    pub input_tokens_details: Option<ResponsesInputTokensDetails>,
    pub output_tokens: Option<u64>,
    pub output_tokens_details: Option<ResponsesOutputTokensDetails>,
    pub total_tokens: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ResponsesInputTokensDetails {
    pub cached_tokens: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ResponsesOutputTokensDetails {
    pub reasoning_tokens: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChatCompletionsUsage {
    pub prompt_tokens: Option<u64>,
    pub completion_tokens: Option<u64>,
    pub total_tokens: Option<u64>,
    pub prompt_tokens_details: Option<ChatCompletionsPromptTokensDetails>,
    pub completion_tokens_details: Option<ChatCompletionsCompletionTokensDetails>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChatCompletionsPromptTokensDetails {
    pub cached_tokens: Option<u64>,
    pub audio_tokens: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChatCompletionsCompletionTokensDetails {
    pub reasoning_tokens: Option<u64>,
    pub audio_tokens: Option<u64>,
    pub accepted_prediction_tokens: Option<u64>,
    pub rejected_prediction_tokens: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GenerateContentUsage {
    pub prompt_token_count: Option<u64>,
    pub candidates_token_count: Option<u64>,
    pub cached_content_token_count: Option<u64>,
    pub tool_use_prompt_token_count: Option<u64>,
    pub thoughts_token_count: Option<u64>,
    pub total_token_count: Option<u64>,
    pub cache_tokens_details: Option<Vec<ModalityTokenCount>>,
    pub prompt_tokens_details: Option<Vec<ModalityTokenCount>>,
    pub candidates_tokens_details: Option<Vec<ModalityTokenCount>>,
    pub tool_use_prompt_tokens_details: Option<Vec<ModalityTokenCount>>,
    pub traffic_type: Option<TrafficType>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ModalityTokenCount {
    pub modality: Option<MediaModality>,
    pub token_count: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum MediaModality {
    ModalityUnspecified,
    Text,
    Image,
    Video,
    Audio,
    Document,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum TrafficType {
    TrafficTypeUnspecified,
    OnDemand,
    OnDemandPriority,
    OnDemandFlex,
    ProvisionedThroughput,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EmbeddingsUsage {
    pub prompt_tokens: Option<u64>,
    pub total_tokens: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RerankUsage {
    pub input_tokens: Option<f64>,
    pub output_tokens: Option<f64>,
    pub cached_tokens: Option<f64>,
    pub billed_units: Option<RerankBilledUnits>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RerankBilledUnits {
    pub images: Option<f64>,
    pub input_tokens: Option<f64>,
    pub image_tokens: Option<f64>,
    pub output_tokens: Option<f64>,
    pub search_units: Option<f64>,
    pub classifications: Option<f64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AudioSpeechUsage {
    pub input_tokens: Option<u64>,
    pub output_tokens: Option<u64>,
    pub total_tokens: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AudioTranscriptionsUsage {
    #[serde(rename = "type")]
    pub usage_type: Option<String>,
    pub input_tokens: Option<u64>,
    pub input_token_details: Option<AudioTranscriptionsInputTokenDetails>,
    pub output_tokens: Option<u64>,
    pub total_tokens: Option<u64>,
    pub seconds: Option<f64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AudioTranscriptionsInputTokenDetails {
    pub audio_tokens: Option<u64>,
    pub text_tokens: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ImagesUsage {
    pub total_tokens: Option<u64>,
    pub input_tokens: Option<u64>,
    pub output_tokens: Option<u64>,
    pub input_tokens_details: Option<ImagesInputTokensDetails>,
    pub output_tokens_details: Option<ImagesOutputTokensDetails>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ImagesInputTokensDetails {
    pub text_tokens: Option<u64>,
    pub image_tokens: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ImagesOutputTokensDetails {
    pub text_tokens: Option<u64>,
    pub image_tokens: Option<u64>,
}

#[derive(Debug, Clone, Serialize)]
pub struct UnknownUsage {
    pub raw: Value,
}
