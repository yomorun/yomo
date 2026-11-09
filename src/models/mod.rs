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
    /// Create a registry with the built-in default conversion behavior.
    pub fn new() -> Self {
        Self {
            profiles: HashMap::new(),
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
    use crate::serve_config::ModelBinding;
    use crate::upstreams::Upstream;
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
}
