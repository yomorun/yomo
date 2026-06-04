use serde::{Deserialize, Serialize};
use serde_json::Value;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct VertexGenerateContentRequest {
    pub contents: Vec<VertexContent>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub system_instruction: Option<VertexSystemInstruction>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub generation_config: Option<VertexGenerationConfig>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tools: Option<Vec<VertexTool>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_config: Option<VertexToolConfig>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct VertexSystemInstruction {
    pub parts: Vec<VertexPart>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct VertexGenerationConfig {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub top_p: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_output_tokens: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub candidate_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stop_sequences: Option<Vec<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub response_mime_type: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub response_schema: Option<Value>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct VertexTool {
    #[serde(rename = "functionDeclarations")]
    pub function_declarations: Vec<VertexFunctionDeclaration>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct VertexFunctionDeclaration {
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    pub parameters: Value,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct VertexToolConfig {
    pub function_calling_config: VertexFunctionCallingConfig,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct VertexFunctionCallingConfig {
    pub mode: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub allowed_function_names: Option<Vec<String>>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct VertexGenerateContentResponse {
    #[serde(rename = "responseId")]
    pub response_id: Option<String>,
    #[serde(rename = "modelVersion")]
    pub model_version: Option<String>,
    pub candidates: Option<Vec<VertexCandidate>>,
    #[serde(rename = "usageMetadata")]
    pub usage_metadata: Option<VertexUsageMetadata>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct VertexCandidate {
    pub content: Option<VertexContent>,
    #[serde(rename = "finishReason")]
    pub finish_reason: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct VertexContent {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub role: Option<String>,
    pub parts: Vec<VertexPart>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct VertexPart {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub text: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub inline_data: Option<VertexInlineData>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub function_call: Option<VertexFunctionCall>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub function_response: Option<VertexFunctionResponse>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thought_signature: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct VertexInlineData {
    pub mime_type: String,
    pub data: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct VertexFunctionCall {
    pub name: String,
    #[serde(default)]
    pub args: Value,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct VertexFunctionResponse {
    pub name: String,
    pub response: Value,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct VertexUsageMetadata {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub prompt_token_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub candidates_token_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cached_content_token_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_use_prompt_token_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thoughts_token_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub total_token_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cache_tokens_details: Option<Vec<VertexModalityTokenCount>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub prompt_tokens_details: Option<Vec<VertexModalityTokenCount>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub candidates_tokens_details: Option<Vec<VertexModalityTokenCount>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_use_prompt_tokens_details: Option<Vec<VertexModalityTokenCount>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub traffic_type: Option<VertexTrafficType>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct VertexModalityTokenCount {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub modality: Option<VertexMediaModality>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub token_count: Option<i32>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum VertexMediaModality {
    ModalityUnspecified,
    Text,
    Image,
    Video,
    Audio,
    Document,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum VertexTrafficType {
    TrafficTypeUnspecified,
    OnDemand,
    OnDemandPriority,
    OnDemandFlex,
    ProvisionedThroughput,
}

#[cfg(test)]
mod tests {
    use super::{VertexMediaModality, VertexTrafficType, VertexUsageMetadata};

    #[test]
    fn vertex_usage_metadata_deserializes_extended_camel_case_fields() {
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

        let usage: VertexUsageMetadata =
            serde_json::from_value(payload).expect("must parse usage metadata");

        assert_eq!(usage.prompt_token_count, Some(11));
        assert_eq!(usage.candidates_token_count, Some(7));
        assert_eq!(usage.cached_content_token_count, Some(3));
        assert_eq!(usage.tool_use_prompt_token_count, Some(2));
        assert_eq!(usage.thoughts_token_count, Some(1));
        assert_eq!(usage.total_token_count, Some(21));
        assert_eq!(usage.prompt_tokens_details.as_ref().map(Vec::len), Some(1));
        assert!(matches!(
            usage.traffic_type,
            Some(VertexTrafficType::OnDemand)
        ));
    }

    #[test]
    fn vertex_usage_metadata_serializes_modality_and_traffic_as_enum_strings() {
        let usage = VertexUsageMetadata {
            prompt_tokens_details: Some(vec![super::VertexModalityTokenCount {
                modality: Some(VertexMediaModality::Audio),
                token_count: Some(5),
            }]),
            traffic_type: Some(VertexTrafficType::ProvisionedThroughput),
            ..Default::default()
        };

        let value = serde_json::to_value(usage).expect("must serialize usage metadata");

        assert_eq!(value["promptTokensDetails"][0]["modality"], "AUDIO");
        assert_eq!(value["trafficType"], "PROVISIONED_THROUGHPUT");
    }
}
