//! Model-specific conversion and adaptation from unified requests to upstream dialects.
//!
//! Client API parsing belongs to [`crate::api`]. Profiles are selected by the
//! configured public model and may override a dialect conversion after a route
//! has been resolved.

use std::collections::HashMap;
use std::sync::Arc;

use crate::dialects::bedrock_converse::BedrockConverseRequest;
use crate::dialects::google_vertex::types::VertexGenerateContentRequest;
use crate::openai_responses_types::ResponsesRequest;
use crate::openai_types::{ChatCompletionRequest, ErrorDetail};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::serve_config::ModelBinding;
use axum::http::StatusCode;

/// GLM-5.3 request profile.
pub mod glm_5_3;
/// GLM-5.3-Flash request profile.
pub mod glm_5_3_flash;
/// GPT-5.1 Codex Responses profile.
pub mod gpt_5_1_codex;
/// GPT-5.1 Codex-Max Responses profile.
pub mod gpt_5_1_codex_max;
/// GPT-5.2 Codex Responses profile.
pub mod gpt_5_2_codex;
/// GPT-5.3 Codex Responses profile.
pub mod gpt_5_3_codex;
/// GPT-5.4 Chat Completions profile.
pub mod gpt_5_4;
/// GPT-5.4 Mini Chat Completions profile.
pub mod gpt_5_4_mini;
/// GPT-5.4 Nano Chat Completions profile.
pub mod gpt_5_4_nano;
/// GPT-5.5 Chat Completions profile.
pub mod gpt_5_5;
/// GPT-5.6 Luna Responses profile.
pub mod gpt_5_6_luna;
/// GPT-5.6 Sol Responses profile.
pub mod gpt_5_6_sol;
/// GPT-5.6 Terra Responses profile.
pub mod gpt_5_6_terra;
/// GPT-6.1 Sol Responses profile.
pub mod gpt_6_1_sol;
/// GPT-6.1 Terra Responses profile.
pub mod gpt_6_1_terra;
/// GPT-6 Astra Responses profile.
pub mod gpt_6_astra;
/// GPT-6 Luna Responses profile.
pub mod gpt_6_luna;
/// GPT-6 Sol Responses profile.
pub mod gpt_6_sol;

/// Gemini 3.8 Flash request profile.
pub mod gemini_3_8_flash;
/// Gemini 4 Argon request profile.
pub mod gemini_4_argon;

/// Claude Fable 5 profile.
pub mod claude_fable_5;
/// Claude Fable 5.1 profile.
pub mod claude_fable_5_1;
/// Claude Haiku 4.5 profile.
pub mod claude_haiku_4_5;
/// Claude Haiku 5.5 profile.
pub mod claude_haiku_5_5;
/// Claude Opus 4.6 profile.
pub mod claude_opus_4_6;
/// Claude Opus 4.7 profile.
pub mod claude_opus_4_7;
/// Claude Opus 4.8 profile.
pub mod claude_opus_4_8;
/// Claude Opus 5 profile.
pub mod claude_opus_5;
/// Claude Opus 5.5 profile.
pub mod claude_opus_5_5;
/// Claude Sonnet 4.6 profile.
pub mod claude_sonnet_4_6;
/// Claude Sonnet 5 profile.
pub mod claude_sonnet_5;
/// Claude Sonnet 5.5 profile.
pub mod claude_sonnet_5_5;

/// DeepSeek V4.1 Flash profile.
pub mod deepseek_v4_1_flash;
/// DeepSeek V4 Flash profile.
pub mod deepseek_v4_flash;
/// DeepSeek V4 Flash Vision profile.
pub mod deepseek_v4_flash_vision_exp;
/// DeepSeek V4 Pro profile and its public aliases.
pub mod deepseek_v4_pro;
/// GLM-5.2 profile.
pub mod glm_5_2;
/// Kimi K2.6 profile.
pub mod kimi_k2_6;
/// Kimi K2.8 Preview profile.
pub mod kimi_k2_8_preview;
/// Kimi K3 profile.
pub mod kimi_k3;
/// MiniMax M2.7 profile.
pub mod minimax_m2_7;
/// MiniMax M3 profile.
pub mod minimax_m3;
/// Shared helpers used by model-specific profiles.
mod profile_support;

/// A model-profile conversion failure, independent of an HTTP response format.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ModelProfileError {
    /// The input request is invalid for the selected profile.
    InvalidRequest {
        /// Human-readable reason safe to return to the caller.
        message: String,
        /// Client request field associated with the error.
        param: Option<String>,
    },
    /// The requested semantics cannot be represented by the target dialect.
    UnsupportedConversion {
        /// Human-readable reason safe to return to the caller.
        message: String,
        /// Client request field associated with the conversion failure.
        param: Option<String>,
    },
    /// The selected model or upstream does not support the requested capability.
    UnsupportedCapability {
        /// Human-readable reason safe to return to the caller.
        message: String,
        /// Client request field associated with the capability failure.
        param: Option<String>,
    },
    /// The profile failed unexpectedly while constructing the dialect request.
    Internal {
        /// Internal diagnostic message; do not expose to API clients.
        message: String,
    },
}

impl std::fmt::Display for ModelProfileError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::InvalidRequest { message, .. }
            | Self::UnsupportedConversion { message, .. }
            | Self::UnsupportedCapability { message, .. }
            | Self::Internal { message } => formatter.write_str(message),
        }
    }
}

impl std::error::Error for ModelProfileError {}

impl ModelProfileError {
    pub(crate) fn public_error(&self) -> Option<(StatusCode, ErrorDetail)> {
        let (message, code, param) = match self {
            Self::InvalidRequest { message, param } => {
                (message, "invalid_request", param.as_deref())
            }
            Self::UnsupportedConversion { message, param } => {
                (message, "unsupported_conversion", param.as_deref())
            }
            Self::UnsupportedCapability { message, param } => {
                (message, "unsupported_capability", param.as_deref())
            }
            Self::Internal { .. } => return None,
        };
        Some((
            StatusCode::BAD_REQUEST,
            ErrorDetail {
                message: message.clone(),
                r#type: "invalid_request_error".into(),
                code: Some(code.into()),
                param: param.map(str::to_owned),
            },
        ))
    }
}

/// Adapt errors emitted by yomo's built-in Unified-to-dialect converters.
pub(crate) fn from_dialect_conversion_error(error: ProviderError) -> ModelProfileError {
    match error {
        ProviderError::Public { status, error } if status == StatusCode::BAD_REQUEST => {
            match error.code.as_deref() {
                Some("unsupported_conversion") => ModelProfileError::UnsupportedConversion {
                    message: error.message,
                    param: error.param,
                },
                Some("unsupported_capability") => ModelProfileError::UnsupportedCapability {
                    message: error.message,
                    param: error.param,
                },
                _ => ModelProfileError::InvalidRequest {
                    message: error.message,
                    param: error.param,
                },
            }
        }
        ProviderError::ModelProfile(error) => error,
        other => ModelProfileError::Internal {
            message: other.to_string(),
        },
    }
}

/// Converts or adapts a unified request at the upstream dialect boundary.
///
/// Dialect conversion methods use their own concrete output types because the
/// request shapes are not interchangeable. Returning `None` delegates to
/// yomo's built-in conversion. Vertex uses an in-place adapter hook after its
/// default conversion so stateful thought-signature handling is never skipped.
pub trait ModelProfile: Send + Sync {
    /// Convert to an OpenAI Chat Completions request, or use yomo's conversion.
    fn to_openai_chat_request(
        &self,
        _request: &UnifiedRequest,
        _binding: &ModelBinding,
    ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        Ok(None)
    }

    /// Convert to an OpenAI Responses request, or use yomo's conversion.
    fn to_openai_responses_request(
        &self,
        _request: &UnifiedRequest,
        _binding: &ModelBinding,
    ) -> Result<Option<ResponsesRequest>, ModelProfileError> {
        Ok(None)
    }

    /// Convert to a Bedrock Converse request, or use yomo's conversion.
    fn to_bedrock_converse_request(
        &self,
        _request: &UnifiedRequest,
        _binding: &ModelBinding,
    ) -> Result<Option<BedrockConverseRequest>, ModelProfileError> {
        Ok(None)
    }

    /// Adapt a unified request immediately before Bedrock Converse encoding.
    ///
    /// The hook runs after generic reasoning normalization and before the
    /// dialect builds messages, tools, and inference settings. Profiles may
    /// normalize model-specific thinking modes, remove deprecated sampling
    /// parameters, validate tool-choice restrictions, and reject conversation
    /// shapes the model does not accept.
    fn adapt_bedrock_request(
        &self,
        _request: &mut UnifiedRequest,
        _binding: &ModelBinding,
    ) -> Result<(), ModelProfileError> {
        Ok(())
    }

    /// Adapt yomo's generated Gemini request in place.
    ///
    /// The provider completes its default conversion, including thought
    /// signature handling, before invoking this hook.
    fn adapt_gemini_generate_content_request(
        &self,
        _request: &UnifiedRequest,
        _binding: &ModelBinding,
        _dialect_request: &mut VertexGenerateContentRequest,
    ) -> Result<(), ModelProfileError> {
        Ok(())
    }
}

/// Public-model keyed registry for caller-provided request conversion profiles.
#[derive(Clone)]
pub struct ModelProfileRegistry {
    profiles: HashMap<String, Arc<dyn ModelProfile>>,
    default: Arc<dyn ModelProfile>,
}

impl ModelProfileRegistry {
    /// Create a registry with built-in model profiles and default conversion behavior.
    pub fn new() -> Self {
        let mut profiles: HashMap<String, Arc<dyn ModelProfile>> = HashMap::new();
        profiles.insert(
            "glm-5.3-flash".to_owned(),
            Arc::new(glm_5_3_flash::Glm53FlashProfile),
        );
        profiles.insert("glm-5.3".to_owned(), Arc::new(glm_5_3::Glm53Profile));
        profiles.insert("gpt-5.5".to_owned(), Arc::new(gpt_5_5::PROFILE));
        profiles.insert("gpt-5.4".to_owned(), Arc::new(gpt_5_4::PROFILE));
        profiles.insert("gpt-5.4-mini".to_owned(), Arc::new(gpt_5_4_mini::PROFILE));
        profiles.insert("gpt-5.4-nano".to_owned(), Arc::new(gpt_5_4_nano::PROFILE));
        profiles.insert("gpt-5.3-codex".to_owned(), Arc::new(gpt_5_3_codex::PROFILE));
        profiles.insert("gpt-5.2-codex".to_owned(), Arc::new(gpt_5_2_codex::PROFILE));
        profiles.insert("gpt-5.1-codex".to_owned(), Arc::new(gpt_5_1_codex::PROFILE));
        profiles.insert(
            "gpt-5.1-codex-max".to_owned(),
            Arc::new(gpt_5_1_codex_max::PROFILE),
        );
        profiles.insert("gpt-5.6-sol".to_owned(), Arc::new(gpt_5_6_sol::PROFILE));
        profiles.insert("gpt-5.6-luna".to_owned(), Arc::new(gpt_5_6_luna::PROFILE));
        profiles.insert("gpt-5.6-terra".to_owned(), Arc::new(gpt_5_6_terra::PROFILE));
        profiles.insert("gpt-6-astra".to_owned(), Arc::new(gpt_6_astra::PROFILE));
        profiles.insert("gpt-6-sol".to_owned(), Arc::new(gpt_6_sol::PROFILE));
        profiles.insert("gpt-6-luna".to_owned(), Arc::new(gpt_6_luna::PROFILE));
        profiles.insert("gpt-6.1-sol".to_owned(), Arc::new(gpt_6_1_sol::PROFILE));
        profiles.insert("gpt-6.1-terra".to_owned(), Arc::new(gpt_6_1_terra::PROFILE));
        let gemini_38_flash: Arc<dyn ModelProfile> =
            Arc::new(gemini_3_8_flash::Gemini38FlashProfile);
        for public_model in [
            "gemini-3.8-flash",
            "gemini-2.5-flash",
            "gemini-2.5-pro",
            "gemini-3.1-pro-preview",
            "gemini-3.5-flash",
            "gemini-3.6-flash",
            "gemini-3.7-flash",
            "gemini-3-flash-preview",
        ] {
            profiles.insert(public_model.to_owned(), Arc::clone(&gemini_38_flash));
        }
        profiles.insert(
            "gemini-4-argon".to_owned(),
            Arc::new(gemini_4_argon::Gemini4ArgonProfile),
        );
        let claude_profiles: [(&str, Arc<dyn ModelProfile>); 12] = [
            (
                "claude-fable-5",
                Arc::new(claude_fable_5::ClaudeFable5Profile),
            ),
            (
                "claude-fable-5-1",
                Arc::new(claude_fable_5_1::ClaudeFable51Profile),
            ),
            (
                "claude-haiku-4-5",
                Arc::new(claude_haiku_4_5::ClaudeHaiku45Profile),
            ),
            (
                "claude-haiku-5-5",
                Arc::new(claude_haiku_5_5::ClaudeHaiku55Profile),
            ),
            (
                "claude-opus-4-6",
                Arc::new(claude_opus_4_6::ClaudeOpus46Profile),
            ),
            (
                "claude-opus-4-7",
                Arc::new(claude_opus_4_7::ClaudeOpus47Profile),
            ),
            (
                "claude-opus-4-8",
                Arc::new(claude_opus_4_8::ClaudeOpus48Profile),
            ),
            ("claude-opus-5", Arc::new(claude_opus_5::ClaudeOpus5Profile)),
            (
                "claude-opus-5-5",
                Arc::new(claude_opus_5_5::ClaudeOpus55Profile),
            ),
            (
                "claude-sonnet-4-6",
                Arc::new(claude_sonnet_4_6::ClaudeSonnet46Profile),
            ),
            (
                "claude-sonnet-5",
                Arc::new(claude_sonnet_5::ClaudeSonnet5Profile),
            ),
            (
                "claude-sonnet-5-5",
                Arc::new(claude_sonnet_5_5::ClaudeSonnet55Profile),
            ),
        ];
        for (public_model, profile) in claude_profiles {
            profiles.insert(public_model.to_owned(), profile);
        }
        profiles.insert(
            "deepseek-v4.1-flash".to_owned(),
            Arc::new(deepseek_v4_1_flash::DeepseekV41FlashProfile),
        );
        profiles.insert(
            "deepseek-v4-flash".to_owned(),
            Arc::new(deepseek_v4_flash::DeepseekV4FlashProfile),
        );
        let deepseek_v4_pro: Arc<dyn ModelProfile> =
            Arc::new(deepseek_v4_pro::DeepseekV4ProProfile);
        profiles.insert("deepseek-v4-pro".to_owned(), Arc::clone(&deepseek_v4_pro));
        profiles.insert("deepseek-v4-pro-0813".to_owned(), deepseek_v4_pro);
        profiles.insert("glm-5.2".to_owned(), Arc::new(glm_5_2::Glm52Profile));
        profiles.insert(
            "deepseek-v4-flash-vision-exp".to_owned(),
            Arc::new(deepseek_v4_flash_vision_exp::DeepseekV4FlashVisionExpProfile),
        );
        profiles.insert("kimi-k3".to_owned(), Arc::new(kimi_k3::KimiK3Profile));
        profiles.insert(
            "kimi-k2.8-preview".to_owned(),
            Arc::new(kimi_k2_8_preview::KimiK28PreviewProfile),
        );
        profiles.insert("kimi-k2.6".to_owned(), Arc::new(kimi_k2_6::KimiK26Profile));
        profiles.insert(
            "minimax-m3".to_owned(),
            Arc::new(minimax_m3::MinimaxM3Profile),
        );
        profiles.insert(
            "minimax-m2.7".to_owned(),
            Arc::new(minimax_m2_7::MinimaxM27Profile),
        );
        Self {
            profiles,
            default: Arc::new(DefaultModelProfile),
        }
    }

    /// Register a profile for one exact public model name.
    pub fn register(
        &mut self,
        public_model: impl Into<String>,
        profile: Arc<dyn ModelProfile>,
    ) -> Option<Arc<dyn ModelProfile>> {
        self.profiles.insert(public_model.into(), profile)
    }

    /// Replace the profile used when no public-model-specific profile exists.
    pub fn set_default(&mut self, profile: Arc<dyn ModelProfile>) {
        self.default = profile;
    }

    /// Resolve a profile by the route's public model name.
    pub fn resolve(&self, public_model: &str) -> Arc<dyn ModelProfile> {
        let Some(profile) = self.profiles.get(public_model) else {
            return Arc::clone(&self.default);
        };
        Arc::new(ResolvedModelProfile {
            profile: Arc::clone(profile),
            fallback: Arc::clone(&self.default),
        })
    }
}

impl Default for ModelProfileRegistry {
    fn default() -> Self {
        Self::new()
    }
}

pub(crate) fn default_model_profile() -> Arc<dyn ModelProfile> {
    Arc::new(DefaultModelProfile)
}

struct DefaultModelProfile;

impl ModelProfile for DefaultModelProfile {
    fn to_openai_chat_request(
        &self,
        request: &UnifiedRequest,
        _binding: &ModelBinding,
    ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        crate::dialects::openai_chat_completions::mapper::request_to_chat(request)
            .map(Some)
            .map_err(from_dialect_conversion_error)
    }

    fn to_openai_responses_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ResponsesRequest>, ModelProfileError> {
        crate::dialects::openai_responses::mapper::ir_to_responses_request(
            request,
            &binding.upstream_model,
        )
        .map(Some)
        .map_err(from_dialect_conversion_error)
    }
}

struct ResolvedModelProfile {
    profile: Arc<dyn ModelProfile>,
    fallback: Arc<dyn ModelProfile>,
}

impl ModelProfile for ResolvedModelProfile {
    fn to_openai_chat_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
        match self.profile.to_openai_chat_request(request, binding)? {
            Some(request) => Ok(Some(request)),
            None => self.fallback.to_openai_chat_request(request, binding),
        }
    }

    fn to_openai_responses_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<ResponsesRequest>, ModelProfileError> {
        match self.profile.to_openai_responses_request(request, binding)? {
            Some(request) => Ok(Some(request)),
            None => self.fallback.to_openai_responses_request(request, binding),
        }
    }

    fn to_bedrock_converse_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<Option<BedrockConverseRequest>, ModelProfileError> {
        match self.profile.to_bedrock_converse_request(request, binding)? {
            Some(request) => Ok(Some(request)),
            None => self.fallback.to_bedrock_converse_request(request, binding),
        }
    }

    fn adapt_gemini_generate_content_request(
        &self,
        request: &UnifiedRequest,
        binding: &ModelBinding,
        dialect_request: &mut VertexGenerateContentRequest,
    ) -> Result<(), ModelProfileError> {
        self.fallback
            .adapt_gemini_generate_content_request(request, binding, dialect_request)?;
        self.profile
            .adapt_gemini_generate_content_request(request, binding, dialect_request)
    }

    fn adapt_bedrock_request(
        &self,
        request: &mut UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<(), ModelProfileError> {
        self.fallback.adapt_bedrock_request(request, binding)?;
        self.profile.adapt_bedrock_request(request, binding)
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::ModelProfileError;
    use super::{ModelProfile, ModelProfileRegistry};
    use crate::dialects::Dialect;
    use crate::dialects::bedrock_converse::BedrockConverseRequest;
    use crate::dialects::google_vertex::types::{
        VertexContent, VertexFunctionCall, VertexGenerateContentRequest, VertexPart,
    };
    use crate::openai_types::ChatCompletionRequest;
    use crate::provider::UnifiedRequest;
    use crate::provider::UnifiedSourceProtocol;
    use crate::provider::UnifiedToolChoice;
    use crate::serve_config::ModelBinding;
    use crate::upstreams::Upstream;
    use serde_json::Value;
    use serde_json::json;

    struct AliasProfile;

    impl ModelProfile for AliasProfile {
        fn to_openai_chat_request(
            &self,
            _request: &UnifiedRequest,
            binding: &ModelBinding,
        ) -> Result<Option<ChatCompletionRequest>, ModelProfileError> {
            assert_eq!(binding.upstream, Upstream::AwsBedrock);
            Ok(Some(ChatCompletionRequest {
                model: binding.public_model.clone(),
                ..Default::default()
            }))
        }
    }

    fn binding(public_model: &str) -> ModelBinding {
        ModelBinding {
            public_model: public_model.to_owned(),
            model: "canonical-model".into(),
            provider: "connection".into(),
            upstream: Upstream::AwsBedrock,
            upstream_model: "deployment-model".into(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: "connection.deployment-model".into(),
        }
    }

    #[test]
    fn profile_registry_resolves_exact_public_model_aliases() {
        let mut profiles = ModelProfileRegistry::default();
        profiles.register("chat-alias", Arc::new(AliasProfile));

        let profile = profiles.resolve("chat-alias");
        let request = profile
            .to_openai_chat_request(&UnifiedRequest::default(), &binding("chat-alias"))
            .expect("profile conversion")
            .expect("registered profile handles conversion");
        assert_eq!(request.model, "chat-alias");

        let fallback = profiles
            .resolve("canonical-model")
            .to_openai_chat_request(&UnifiedRequest::default(), &binding("canonical-model"))
            .expect("default profile")
            .expect("built-in chat conversion");
        assert_eq!(fallback.model, "");

        let fallback = profiles
            .resolve("chat-alias")
            .to_openai_responses_request(&UnifiedRequest::default(), &binding("chat-alias"))
            .expect("default Responses conversion")
            .expect("registered profile falls back for Responses");
        assert_eq!(fallback.model, "deployment-model");
    }

    #[test]
    fn builtin_registry_resolves_both_deepseek_v4_pro_public_names() {
        let profiles = ModelProfileRegistry::default();
        for public_model in ["deepseek-v4-pro", "deepseek-v4-pro-0813"] {
            let request = crate::api::request_from_value(
                &json!({
                    "model": public_model,
                    "messages": [{"role": "user", "content": "hello"}],
                    "reasoning_effort": "medium"
                }),
                UnifiedSourceProtocol::ChatCompletions,
            )
            .expect("parse Chat request");
            let binding = ModelBinding {
                public_model: public_model.to_owned(),
                model: public_model.to_owned(),
                provider: "volcengine.cn".into(),
                upstream: Upstream::VolcengineArk,
                upstream_model: "deepseek-v4-pro-ga-260813".into(),
                dialect: Some(Dialect::OpenAiChatCompletions),
                label: "volcengine.cn.deepseek-v4-pro-0813".into(),
            };
            let converted = profiles
                .resolve(public_model)
                .to_openai_chat_request(&request, &binding)
                .expect("convert profile request")
                .expect("built-in model profile handles this public name");
            assert_eq!(
                converted
                    .reasoning_effort
                    .as_ref()
                    .and_then(|effort| effort.as_str()),
                Some("high")
            );
        }
    }

    #[test]
    fn builtin_gpt_responses_profiles_map_effort_and_fall_back_by_binding() {
        let profiles = ModelProfileRegistry::default();
        let models = [
            ("gpt-5.6-sol", true),
            ("gpt-5.6-luna", true),
            ("gpt-5.6-terra", true),
            ("gpt-6-astra", false),
            ("gpt-6-sol", true),
            ("gpt-6-luna", true),
            ("gpt-6.1-sol", false),
            ("gpt-6.1-terra", true),
        ];

        for (public_model, supports_none) in models {
            let binding = ModelBinding {
                public_model: public_model.to_owned(),
                model: public_model.to_owned(),
                provider: "azure.usa".to_owned(),
                upstream: Upstream::AzureOpenai,
                upstream_model: public_model.to_owned(),
                dialect: Some(Dialect::OpenAiResponses),
                label: format!("azure.usa.{public_model}"),
            };

            for (input_effort, expected_effort) in [
                ("minimal", "low"),
                ("none", if supports_none { "none" } else { "low" }),
                ("low", "low"),
                ("medium", "medium"),
                ("high", "high"),
                ("xhigh", "xhigh"),
                ("max", "max"),
            ] {
                let request = crate::api::request_from_value(
                    &json!({
                        "model": public_model,
                        "input": "hello",
                        "reasoning": {"effort": input_effort}
                    }),
                    UnifiedSourceProtocol::Responses,
                )
                .expect("parse Responses request");
                let converted = profiles
                    .resolve(public_model)
                    .to_openai_responses_request(&request, &binding)
                    .expect("convert GPT Responses request")
                    .expect("registered profile converts Responses");
                assert_eq!(
                    converted
                        .reasoning
                        .as_ref()
                        .and_then(|reasoning| reasoning.effort.as_deref()),
                    Some(expected_effort),
                    "unexpected effort for {public_model} from {input_effort}"
                );
            }

            let no_effort_request = crate::api::request_from_value(
                &json!({"model": public_model, "input": "hello"}),
                UnifiedSourceProtocol::Responses,
            )
            .expect("parse Responses request without an effort");
            let converted = profiles
                .resolve(public_model)
                .to_openai_responses_request(&no_effort_request, &binding)
                .expect("convert Responses request without an effort")
                .expect("registered profile converts Responses");
            assert!(converted.reasoning.is_none());

            let mut mismatched_upstream = binding.clone();
            mismatched_upstream.upstream = Upstream::OpenaiCompatible;
            let effort_request = crate::api::request_from_value(
                &json!({
                    "model": public_model,
                    "input": "hello",
                    "reasoning": {"effort": "minimal"}
                }),
                UnifiedSourceProtocol::Responses,
            )
            .expect("parse Responses request");
            let fallback = profiles
                .resolve(public_model)
                .to_openai_responses_request(&effort_request, &mismatched_upstream)
                .expect("fallback for a different upstream")
                .expect("default Responses converter handles other upstreams");
            assert_eq!(
                fallback
                    .reasoning
                    .as_ref()
                    .and_then(|reasoning| reasoning.effort.as_deref()),
                Some("minimal")
            );

            let mut mismatched_dialect = binding.clone();
            mismatched_dialect.dialect = Some(Dialect::OpenAiChatCompletions);
            let fallback = profiles
                .resolve(public_model)
                .to_openai_responses_request(&effort_request, &mismatched_dialect)
                .expect("fallback for a different dialect")
                .expect("default Responses converter handles other dialects");
            assert_eq!(
                fallback
                    .reasoning
                    .as_ref()
                    .and_then(|reasoning| reasoning.effort.as_deref()),
                Some("minimal")
            );
        }
    }

    fn gpt_binding(public_model: &str, dialect: Dialect) -> ModelBinding {
        ModelBinding {
            public_model: public_model.to_owned(),
            model: public_model.to_owned(),
            provider: "azure.usa".to_owned(),
            upstream: Upstream::AzureOpenai,
            upstream_model: public_model.to_owned(),
            dialect: Some(dialect),
            label: format!("azure.usa.{public_model}"),
        }
    }

    fn gpt_request(public_model: &str, dialect: Dialect, effort: Option<&str>) -> UnifiedRequest {
        let (value, source) = match (dialect, effort) {
            (Dialect::OpenAiChatCompletions, Some(effort)) => (
                json!({
                    "model": public_model,
                    "messages": [{"role": "user", "content": "hello"}],
                    "reasoning_effort": effort
                }),
                UnifiedSourceProtocol::ChatCompletions,
            ),
            (Dialect::OpenAiChatCompletions, None) => (
                json!({
                    "model": public_model,
                    "messages": [{"role": "user", "content": "hello"}]
                }),
                UnifiedSourceProtocol::ChatCompletions,
            ),
            (Dialect::OpenAiResponses, Some(effort)) => (
                json!({
                    "model": public_model,
                    "input": "hello",
                    "reasoning": {"effort": effort}
                }),
                UnifiedSourceProtocol::Responses,
            ),
            (Dialect::OpenAiResponses, None) => (
                json!({"model": public_model, "input": "hello"}),
                UnifiedSourceProtocol::Responses,
            ),
            _ => panic!("GPT profile test requires a Chat or Responses dialect"),
        };
        crate::api::request_from_value(&value, source).expect("parse GPT request")
    }

    fn converted_gpt_effort(
        profiles: &ModelProfileRegistry,
        request: &UnifiedRequest,
        binding: &ModelBinding,
        dialect: Dialect,
    ) -> Option<String> {
        let profile = profiles.resolve(&binding.public_model);
        match dialect {
            Dialect::OpenAiChatCompletions => profile
                .to_openai_chat_request(request, binding)
                .expect("convert GPT Chat request")
                .expect("default or model profile converts Chat")
                .reasoning_effort
                .as_ref()
                .and_then(|effort| effort.as_str())
                .map(str::to_owned),
            Dialect::OpenAiResponses => profile
                .to_openai_responses_request(request, binding)
                .expect("convert GPT Responses request")
                .expect("default or model profile converts Responses")
                .reasoning
                .and_then(|reasoning| reasoning.effort),
            _ => panic!("GPT profile test requires a Chat or Responses dialect"),
        }
    }

    #[test]
    fn builtin_gpt_chat_and_codex_profiles_map_effort_and_respect_bindings() {
        let profiles = ModelProfileRegistry::default();
        let models = [
            ("gpt-5.5", Dialect::OpenAiChatCompletions, true, true),
            ("gpt-5.4", Dialect::OpenAiChatCompletions, true, true),
            ("gpt-5.4-mini", Dialect::OpenAiChatCompletions, true, true),
            ("gpt-5.4-nano", Dialect::OpenAiChatCompletions, true, true),
            ("gpt-5.3-codex", Dialect::OpenAiResponses, false, true),
            ("gpt-5.2-codex", Dialect::OpenAiResponses, false, true),
            ("gpt-5.1-codex", Dialect::OpenAiResponses, true, false),
            ("gpt-5.1-codex-max", Dialect::OpenAiResponses, true, true),
        ];

        for (public_model, dialect, supports_none, supports_xhigh) in models {
            let binding = gpt_binding(public_model, dialect);
            let max_effort = if supports_xhigh { "xhigh" } else { "high" };
            for (input, expected) in [
                ("minimal", "low"),
                ("none", if supports_none { "none" } else { "low" }),
                ("low", "low"),
                ("medium", "medium"),
                ("high", "high"),
                ("xhigh", if supports_xhigh { "xhigh" } else { "high" }),
                ("max", max_effort),
            ] {
                let request = gpt_request(public_model, dialect, Some(input));
                assert_eq!(
                    converted_gpt_effort(&profiles, &request, &binding, dialect).as_deref(),
                    Some(expected),
                    "unexpected effort for {public_model} from {input}"
                );
            }

            let request = gpt_request(public_model, dialect, None);
            assert_eq!(
                converted_gpt_effort(&profiles, &request, &binding, dialect),
                None,
                "omitted effort must remain unset for {public_model}"
            );

            let request = gpt_request(public_model, dialect, Some("minimal"));
            let mut other_upstream = binding.clone();
            other_upstream.upstream = Upstream::OpenaiCompatible;
            assert_eq!(
                converted_gpt_effort(&profiles, &request, &other_upstream, dialect).as_deref(),
                Some("minimal"),
                "other upstream must use default conversion for {public_model}"
            );

            let mut other_dialect = binding.clone();
            other_dialect.dialect = Some(match dialect {
                Dialect::OpenAiChatCompletions => Dialect::OpenAiResponses,
                Dialect::OpenAiResponses => Dialect::OpenAiChatCompletions,
                _ => unreachable!(),
            });
            assert_eq!(
                converted_gpt_effort(&profiles, &request, &other_dialect, dialect).as_deref(),
                Some("minimal"),
                "other dialect must use default conversion for {public_model}"
            );
        }
    }

    struct BedrockAliasProfile;

    impl ModelProfile for BedrockAliasProfile {
        fn to_bedrock_converse_request(
            &self,
            _request: &UnifiedRequest,
            binding: &ModelBinding,
        ) -> Result<Option<BedrockConverseRequest>, ModelProfileError> {
            assert_eq!(binding.upstream, Upstream::AwsBedrock);
            Ok(Some(BedrockConverseRequest {
                messages: vec![json!({
                    "role": "user",
                    "content": [{"text": binding.public_model}]
                })],
                inference_config: json!({}),
                ..Default::default()
            }))
        }
    }

    #[test]
    fn profile_builds_bedrock_request_without_sdk_types_and_uses_public_alias() {
        let mut profiles = ModelProfileRegistry::default();
        profiles.register("bedrock-alias", Arc::new(BedrockAliasProfile));
        let mut binding = binding("bedrock-alias");
        binding.upstream_model = "deployment-model".into();
        binding.dialect = Some(Dialect::BedrockConverse);

        let request = profiles
            .resolve(&binding.public_model)
            .to_bedrock_converse_request(&UnifiedRequest::default(), &binding)
            .expect("profile conversion")
            .expect("registered Bedrock profile");
        assert_eq!(request.messages[0]["content"][0]["text"], "bedrock-alias");
    }

    struct VertexAdapter;

    impl ModelProfile for VertexAdapter {
        fn adapt_gemini_generate_content_request(
            &self,
            _request: &UnifiedRequest,
            _binding: &ModelBinding,
            dialect_request: &mut VertexGenerateContentRequest,
        ) -> Result<(), ModelProfileError> {
            dialect_request.generation_config = Some(Default::default());
            Ok(())
        }
    }

    #[test]
    fn vertex_profile_adapts_generated_request_without_dropping_thought_signatures() {
        let mut profiles = ModelProfileRegistry::default();
        profiles.register("vertex-alias", Arc::new(VertexAdapter));
        let binding = binding("vertex-alias");
        let mut dialect_request = VertexGenerateContentRequest {
            contents: vec![VertexContent {
                role: Some("model".into()),
                parts: vec![VertexPart {
                    function_call: Some(VertexFunctionCall {
                        name: "lookup".into(),
                        args: json!({}),
                    }),
                    thought_signature: Some("signature-from-default-converter".into()),
                    ..Default::default()
                }],
            }],
            system_instruction: None,
            generation_config: None,
            tools: None,
            tool_config: None,
        };

        profiles
            .resolve(&binding.public_model)
            .adapt_gemini_generate_content_request(
                &UnifiedRequest::default(),
                &binding,
                &mut dialect_request,
            )
            .expect("profile adaptation");

        assert_eq!(
            dialect_request.contents[0].parts[0]
                .thought_signature
                .as_deref(),
            Some("signature-from-default-converter")
        );
        assert!(dialect_request.generation_config.is_some());
    }

    enum Expected {
        Effort(&'static str),
        Rejected,
    }

    fn chat_binding(public_model: &str, upstream: Upstream) -> ModelBinding {
        ModelBinding {
            public_model: public_model.to_owned(),
            model: public_model.to_owned(),
            provider: "test".to_owned(),
            upstream,
            upstream_model: public_model.to_owned(),
            dialect: Some(Dialect::OpenAiChatCompletions),
            label: format!("test.{public_model}"),
        }
    }

    #[test]
    fn builtin_chat_profiles_are_registered_and_scope_their_bindings() {
        use super::deepseek_v4_1_flash::DeepseekV41FlashProfile;
        use super::deepseek_v4_flash::DeepseekV4FlashProfile;
        use super::deepseek_v4_flash_vision_exp::DeepseekV4FlashVisionExpProfile;
        use super::deepseek_v4_pro::DeepseekV4ProProfile;
        use super::glm_5_2::Glm52Profile;
        use super::kimi_k2_6::KimiK26Profile;
        use super::kimi_k2_8_preview::KimiK28PreviewProfile;
        use super::kimi_k3::KimiK3Profile;
        use super::minimax_m2_7::MinimaxM27Profile;
        use super::minimax_m3::MinimaxM3Profile;

        let profiles = ModelProfileRegistry::default();
        let cases: Vec<(
            &str,
            Arc<dyn ModelProfile>,
            Upstream,
            Upstream,
            Value,
            Expected,
        )> = vec![
            (
                "glm-5.2",
                Arc::new(Glm52Profile),
                Upstream::Tokenhub,
                Upstream::VolcengineArk,
                json!({"reasoning_effort": "medium"}),
                Expected::Effort("high"),
            ),
            (
                "deepseek-v4-flash",
                Arc::new(DeepseekV4FlashProfile),
                Upstream::VolcengineArk,
                Upstream::Tokenhub,
                json!({"reasoning_effort": "medium"}),
                Expected::Effort("high"),
            ),
            (
                "deepseek-v4-pro",
                Arc::new(DeepseekV4ProProfile),
                Upstream::VolcengineArk,
                Upstream::Tokenhub,
                json!({"reasoning_effort": "medium"}),
                Expected::Effort("high"),
            ),
            (
                "deepseek-v4-pro-0813",
                Arc::new(DeepseekV4ProProfile),
                Upstream::VolcengineArk,
                Upstream::Tokenhub,
                json!({"reasoning_effort": "medium"}),
                Expected::Effort("high"),
            ),
            (
                "deepseek-v4.1-flash",
                Arc::new(DeepseekV41FlashProfile),
                Upstream::VolcengineArk,
                Upstream::Tokenhub,
                json!({"reasoning_effort": "xhigh"}),
                Expected::Effort("max"),
            ),
            (
                "deepseek-v4-flash-vision-exp",
                Arc::new(DeepseekV4FlashVisionExpProfile),
                Upstream::Tokenhub,
                Upstream::VolcengineArk,
                json!({"reasoning_effort": "minimal"}),
                Expected::Effort("low"),
            ),
            (
                "kimi-k2.6",
                Arc::new(KimiK26Profile),
                Upstream::Tokenhub,
                Upstream::VolcengineArk,
                json!({"reasoning_effort": "high"}),
                Expected::Rejected,
            ),
            (
                "kimi-k2.8-preview",
                Arc::new(KimiK28PreviewProfile),
                Upstream::Tokenhub,
                Upstream::VolcengineArk,
                json!({"thinking": {"type": "enabled"}}),
                Expected::Rejected,
            ),
            (
                "kimi-k3",
                Arc::new(KimiK3Profile),
                Upstream::Tokenhub,
                Upstream::VolcengineArk,
                json!({"thinking": {"type": "enabled"}}),
                Expected::Rejected,
            ),
            (
                "minimax-m2.7",
                Arc::new(MinimaxM27Profile),
                Upstream::Tokenhub,
                Upstream::VolcengineArk,
                json!({"thinking": {"type": "disabled"}}),
                Expected::Rejected,
            ),
            (
                "minimax-m3",
                Arc::new(MinimaxM3Profile),
                Upstream::Tokenhub,
                Upstream::VolcengineArk,
                json!({"thinking": {"type": "enabled"}}),
                Expected::Rejected,
            ),
        ];

        for (model, profile, handled, foreign, probe, expected) in cases {
            let mut value = json!({
                "model": model,
                "messages": [{"role": "user", "content": "hello"}]
            });
            let Value::Object(probe) = probe else {
                panic!("{model} probe must be an object");
            };
            value
                .as_object_mut()
                .expect("base request must be an object")
                .extend(probe);
            let request =
                crate::api::request_from_value(&value, UnifiedSourceProtocol::ChatCompletions)
                    .expect("parse probe request");

            let handled_binding = chat_binding(model, handled);
            let direct = profile.to_openai_chat_request(&request, &handled_binding);
            let resolved = profiles
                .resolve(model)
                .to_openai_chat_request(&request, &handled_binding);

            match expected {
                Expected::Effort(expected_effort) => {
                    let direct = direct
                        .expect("direct profile conversion")
                        .expect("profile handles its binding");
                    let resolved = resolved
                        .expect("registered profile conversion")
                        .expect("registered profile handles its binding");
                    assert_eq!(
                        direct
                            .reasoning_effort
                            .as_ref()
                            .and_then(|effort| effort.as_str()),
                        Some(expected_effort),
                        "{model} should normalize the probe effort"
                    );
                    assert_eq!(
                        serde_json::to_value(&direct).expect("serialize direct request"),
                        serde_json::to_value(&resolved).expect("serialize resolved request"),
                        "{model} registry entry does not use the expected profile"
                    );
                }
                Expected::Rejected => {
                    assert!(
                        direct.is_err(),
                        "{model} profile must reject the probe request"
                    );
                    assert!(
                        resolved.is_err(),
                        "{model} registry entry must reject the probe request"
                    );
                }
            }

            let foreign_binding = chat_binding(model, foreign);
            assert!(
                profile
                    .to_openai_chat_request(&request, &foreign_binding)
                    .expect("foreign binding conversion")
                    .is_none(),
                "{model} profile must not claim a foreign upstream"
            );
        }
    }

    #[test]
    fn builtin_gemini_aliases_share_the_38_profile_and_argon_stays_separate() {
        use crate::dialects::google_vertex::types::{VertexGenerationConfig, VertexThinkingLevel};

        let profiles = ModelProfileRegistry::default();
        let aliases = [
            "gemini-3.8-flash",
            "gemini-2.5-flash",
            "gemini-2.5-pro",
            "gemini-3.1-pro-preview",
            "gemini-3.5-flash",
            "gemini-3.6-flash",
            "gemini-3.7-flash",
            "gemini-3-flash-preview",
        ];
        for public_model in aliases {
            let request = crate::api::request_from_value(
                &json!({
                    "model": public_model,
                    "messages": [{"role": "user", "content": "hello"}],
                    "reasoning_effort": "minimal"
                }),
                UnifiedSourceProtocol::ChatCompletions,
            )
            .expect("parse Gemini alias request");
            let binding = ModelBinding {
                public_model: public_model.to_owned(),
                model: public_model.to_owned(),
                provider: "google.global".to_owned(),
                upstream: Upstream::GoogleVertex,
                upstream_model: "gemini-3.8-flash".to_owned(),
                dialect: Some(Dialect::GeminiGenerateContent),
                label: "google.global.gemini-3.8-flash".to_owned(),
            };
            let mut body = VertexGenerateContentRequest {
                contents: Vec::new(),
                system_instruction: None,
                generation_config: Some(VertexGenerationConfig::default()),
                tools: None,
                tool_config: None,
            };
            profiles
                .resolve(public_model)
                .adapt_gemini_generate_content_request(&request, &binding, &mut body)
                .expect("adapt Gemini alias");
            let config = body
                .generation_config
                .expect("generation config")
                .thinking_config
                .expect("thinking config");
            assert_eq!(
                config.thinking_level,
                Some(VertexThinkingLevel::Low),
                "{public_model} must use the 3.8 profile"
            );
        }

        let request = crate::api::request_from_value(
            &json!({
                "model": "gemini-4-argon",
                "messages": [{"role": "user", "content": "hello"}],
                "reasoning_effort": "max"
            }),
            UnifiedSourceProtocol::ChatCompletions,
        )
        .expect("parse Argon request");
        let binding = ModelBinding {
            public_model: "gemini-4-argon".to_owned(),
            model: "gemini-4-argon".to_owned(),
            provider: "google.global".to_owned(),
            upstream: Upstream::GoogleVertex,
            upstream_model: "gemini-4-argon".to_owned(),
            dialect: Some(Dialect::GeminiGenerateContent),
            label: "google.global.gemini-4-argon".to_owned(),
        };
        let mut body = VertexGenerateContentRequest {
            contents: Vec::new(),
            system_instruction: None,
            generation_config: Some(VertexGenerationConfig::default()),
            tools: None,
            tool_config: None,
        };
        profiles
            .resolve("gemini-4-argon")
            .adapt_gemini_generate_content_request(&request, &binding, &mut body)
            .expect("adapt Argon");
        let config = body
            .generation_config
            .expect("generation config")
            .thinking_config
            .expect("thinking config");
        assert_eq!(config.thinking_level, Some(VertexThinkingLevel::High));
    }

    fn claude_binding(public_model: &str) -> ModelBinding {
        ModelBinding {
            public_model: public_model.to_owned(),
            model: public_model.to_owned(),
            provider: "aws.usa".to_owned(),
            upstream: Upstream::AwsBedrock,
            upstream_model: format!("global.anthropic.{public_model}"),
            dialect: Some(Dialect::BedrockConverse),
            label: format!("aws.usa.global.anthropic.{public_model}"),
        }
    }

    fn adapt_claude(
        profiles: &ModelProfileRegistry,
        public_model: &str,
        request: &mut UnifiedRequest,
    ) -> Result<(), ModelProfileError> {
        profiles
            .resolve(public_model)
            .adapt_bedrock_request(request, &claude_binding(public_model))
    }

    #[test]
    fn claude_47_plus_profiles_drop_sampling_and_map_enabled_to_adaptive() {
        let profiles = ModelProfileRegistry::default();
        for model in [
            "claude-fable-5",
            "claude-fable-5-1",
            "claude-opus-5",
            "claude-opus-5-5",
            "claude-sonnet-5",
            "claude-sonnet-5-5",
            "claude-haiku-5-5",
            "claude-opus-4-7",
            "claude-opus-4-8",
        ] {
            let mut request = UnifiedRequest::default();
            request.options.temperature = Some(0.7);
            request.options.top_p = Some(0.9);
            request.options.max_output_tokens = Some(8192);
            request.options.thinking = Some(json!({"type":"enabled","budget_tokens":4096}));
            request.options.reasoning_effort = Some("high".into());
            adapt_claude(&profiles, model, &mut request).expect(model);
            assert_eq!(request.options.temperature, None, "{model}");
            assert_eq!(request.options.top_p, None, "{model}");
            assert_eq!(
                request.options.thinking,
                Some(json!({"type":"adaptive"})),
                "{model}"
            );
            assert_eq!(
                request
                    .options
                    .reasoning_effort
                    .as_ref()
                    .and_then(|effort| effort.as_str()),
                Some("high"),
                "{model}"
            );
        }
    }

    #[test]
    fn claude_46_profiles_keep_extended_thinking_and_force_temperature_one() {
        let profiles = ModelProfileRegistry::default();
        for model in ["claude-opus-4-6", "claude-sonnet-4-6"] {
            let mut request = UnifiedRequest::default();
            request.options.temperature = Some(0.7);
            request.options.top_p = Some(0.9);
            request.options.max_output_tokens = Some(8192);
            request.options.thinking = Some(json!({
                "type":"enabled",
                "budget_tokens":4096,
                "display":"summarized"
            }));
            request.options.reasoning_effort = Some("xhigh".into());
            adapt_claude(&profiles, model, &mut request).expect(model);
            assert_eq!(request.options.temperature, Some(1.0), "{model}");
            assert_eq!(request.options.top_p, None, "{model}");
            assert_eq!(
                request.options.thinking,
                Some(json!({
                    "type":"enabled",
                    "budget_tokens":4096,
                    "display":"summarized"
                })),
                "{model}"
            );
            assert_eq!(
                request
                    .options
                    .reasoning_effort
                    .as_ref()
                    .and_then(|effort| effort.as_str()),
                Some("max"),
                "{model}"
            );
        }
    }

    #[test]
    fn claude_haiku_45_requires_extended_thinking_and_rejects_effort() {
        let profiles = ModelProfileRegistry::default();

        let mut adaptive = UnifiedRequest::default();
        adaptive.options.thinking = Some(json!({"type":"adaptive"}));
        assert!(
            adapt_claude(&profiles, "claude-haiku-4-5", &mut adaptive).is_err(),
            "Haiku 4.5 is extended-only"
        );

        let mut effort = UnifiedRequest::default();
        effort.options.reasoning_effort = Some("high".into());
        assert!(
            adapt_claude(&profiles, "claude-haiku-4-5", &mut effort).is_err(),
            "Haiku 4.5 does not support effort"
        );

        let mut disabled = UnifiedRequest::default();
        disabled.options.temperature = Some(0.7);
        disabled.options.top_p = Some(0.9);
        disabled.options.thinking = Some(json!({"type":"disabled"}));
        adapt_claude(&profiles, "claude-haiku-4-5", &mut disabled).expect("disabled thinking");
        assert_eq!(disabled.options.temperature, Some(0.7));
        assert_eq!(disabled.options.top_p, Some(0.9));

        let mut enabled = UnifiedRequest::default();
        enabled.options.temperature = Some(0.7);
        enabled.options.top_p = Some(0.9);
        enabled.options.max_output_tokens = Some(8192);
        enabled.options.thinking = Some(json!({"type":"enabled","budget_tokens":4096}));
        adapt_claude(&profiles, "claude-haiku-4-5", &mut enabled).expect("enabled thinking");
        assert_eq!(enabled.options.temperature, Some(1.0));
        assert_eq!(enabled.options.top_p, None);
    }

    #[test]
    fn claude_disabled_thinking_respects_each_route() {
        let profiles = ModelProfileRegistry::default();
        for model in ["claude-fable-5", "claude-fable-5-1", "claude-opus-5-5"] {
            let mut request = UnifiedRequest::default();
            request.options.thinking = Some(json!({"type":"disabled"}));
            assert!(
                adapt_claude(&profiles, model, &mut request).is_err(),
                "{model} is always on"
            );
        }

        let mut between_tools = UnifiedRequest::default();
        between_tools.options.thinking = Some(json!({"type":"disabled"}));
        adapt_claude(&profiles, "claude-sonnet-5-5", &mut between_tools)
            .expect("Sonnet 5.5 maps disabled to between_tools");
        assert_eq!(
            between_tools.options.thinking,
            Some(json!({"type":"between_tools"}))
        );

        for model in ["claude-opus-5", "claude-haiku-5-5"] {
            let mut disabled = UnifiedRequest::default();
            disabled.options.thinking = Some(json!({"type":"disabled"}));
            disabled.options.reasoning_effort = Some("high".into());
            adapt_claude(&profiles, model, &mut disabled).expect("disabled at high effort");

            let mut too_high = UnifiedRequest::default();
            too_high.options.thinking = Some(json!({"type":"disabled"}));
            too_high.options.reasoning_effort = Some("max".into());
            assert!(
                adapt_claude(&profiles, model, &mut too_high).is_err(),
                "{model} rejects disabled at max effort"
            );
        }

        for model in ["claude-sonnet-5", "claude-opus-4-7", "claude-opus-4-8"] {
            let mut request = UnifiedRequest::default();
            request.options.thinking = Some(json!({"type":"disabled"}));
            request.options.reasoning_effort = Some("max".into());
            adapt_claude(&profiles, model, &mut request).expect(model);
            assert_eq!(
                request.options.thinking,
                Some(json!({"type":"disabled"})),
                "{model}"
            );
        }
    }

    #[test]
    fn claude_forced_tool_choice_is_rejected_only_where_unsupported() {
        let profiles = ModelProfileRegistry::default();
        for model in ["claude-opus-5-5", "claude-fable-5-1", "claude-sonnet-5-5"] {
            let mut request = UnifiedRequest::default();
            request.options.thinking = Some(json!({"type":"adaptive"}));
            request.tool_choice = Some(UnifiedToolChoice::Function("lookup".into()));
            assert!(
                adapt_claude(&profiles, model, &mut request).is_err(),
                "{model} rejects forced tool choice"
            );
        }

        for model in ["claude-fable-5", "claude-opus-5"] {
            let mut request = UnifiedRequest::default();
            request.options.thinking = Some(json!({"type":"adaptive"}));
            request.tool_choice = Some(UnifiedToolChoice::Function("lookup".into()));
            adapt_claude(&profiles, model, &mut request).expect(model);
        }
    }

    #[test]
    fn claude_prefill_is_rejected_only_for_haiku_55() {
        let profiles = ModelProfileRegistry::default();
        for (model, expected_error) in [("claude-haiku-5-5", true), ("claude-opus-5-5", false)] {
            let mut request = crate::api::request_from_value(
                &json!({
                    "model": model,
                    "messages": [
                        {"role": "user", "content": "hello"},
                        {"role": "assistant", "content": "prefill"}
                    ]
                }),
                UnifiedSourceProtocol::ChatCompletions,
            )
            .expect("parse prefill request");
            let result = adapt_claude(&profiles, model, &mut request);
            assert_eq!(result.is_err(), expected_error, "{model}");
        }
    }
}
