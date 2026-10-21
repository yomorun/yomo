use secrecy::SecretString;
use serde::de::{MapAccess, Visitor};
use serde::{Deserialize, Deserializer};
use serde_json::Value;
use std::collections::{BTreeMap, HashSet};
use std::error::Error;
use std::fmt;
use std::marker::PhantomData;
use std::path::PathBuf;
use url::Url;

use crate::models;
use crate::tls::TlsConfig;
use crate::upstreams::{Dialect, Upstream};

/// Environment-specific connection instance, independent of a model.
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ConnectionConfig {
    /// Explicit vendor integration selected without URL inference.
    pub upstream: Upstream,
    /// Base API endpoint for HTTP integrations.
    pub base_url: Option<Url>,
    /// Authentication configuration.
    pub auth: Option<AuthConfig>,
    /// AWS region for Bedrock.
    pub aws_region: Option<String>,
    /// Bedrock bearer credentials.
    pub aws_bearer_token: Option<SecretString>,
    /// Vertex project identity.
    pub project_id: Option<String>,
    /// Vertex region/location.
    pub location: Option<String>,
    /// Vertex service-account credentials path.
    pub credentials_file: Option<PathBuf>,
}

/// One logical model and its unique upstream connection.
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ModelConfig {
    /// Canonical model identity; defaults to the public map key.
    pub model: Option<String>,
    /// Exactly one upstream binding.
    pub upstream: ModelUpstreamConfig,
    /// Explicit upstream API dialect for this model route; defaults to the integration rule.
    pub dialect: Option<Dialect>,
    /// Explicit paths for opaque, non-conversation forwarding.
    pub serve: Option<Vec<String>>,
    /// Public response identity policy.
    pub response_model: Option<String>,
}

/// Environment binding; route-level dialect selection is validated against the concrete integration.
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ModelUpstreamConfig {
    /// Connection instance map key.
    pub provider: String,
    /// Vendor model/deployment identity; defaults to canonical model identity.
    pub model: Option<String>,
    /// Optional diagnostic name, derived from connection and upstream model otherwise.
    pub label: Option<String>,
}

/// New map-based configuration, independently validated before runtime migration.
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ModelRegistryConfig {
    /// Environment connection instances.
    #[serde(default, deserialize_with = "deserialize_unique_map")]
    pub providers: BTreeMap<String, ConnectionConfig>,
    /// Each public model has exactly one upstream connection.
    #[serde(default, deserialize_with = "deserialize_unique_map")]
    pub models: BTreeMap<String, ModelConfig>,
}

fn deserialize_unique_map<'de, D, T>(deserializer: D) -> Result<BTreeMap<String, T>, D::Error>
where
    D: Deserializer<'de>,
    T: Deserialize<'de>,
{
    struct UniqueMap<T>(PhantomData<T>);
    impl<'de, T: Deserialize<'de>> Visitor<'de> for UniqueMap<T> {
        type Value = BTreeMap<String, T>;
        fn expecting(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
            formatter.write_str("a map with unique connection/model names")
        }
        fn visit_map<A: MapAccess<'de>>(self, mut access: A) -> Result<Self::Value, A::Error> {
            use serde::de::Error;
            let mut values = BTreeMap::new();
            while let Some((key, value)) = access.next_entry::<String, T>()? {
                if values.insert(key.clone(), value).is_some() {
                    return Err(A::Error::custom(format!(
                        "duplicate configuration name: {key}"
                    )));
                }
            }
            Ok(values)
        }
    }
    deserializer.deserialize_map(UniqueMap(PhantomData))
}

impl ModelRegistryConfig {
    /// Compile startup bindings without falling back to legacy configuration.
    pub fn compile(&self) -> Result<Vec<ProviderConfig>, ConfigError> {
        compile_model_entries(&self.providers, &self.models)
    }
}

/// Model identity and stable connection resolved without inspecting a request.
#[derive(Debug, Clone)]
pub struct ResolvedBinding {
    /// Public API model identity.
    pub public_model: String,
    /// Canonical model identity used for capability lookup.
    pub model: String,
    /// Connection key.
    pub provider: String,
    /// Explicit vendor integration.
    pub upstream: Upstream,
    /// Vendor deployment/model ID.
    pub upstream_model: String,
    /// Selected once, or absent for opaque forwarding.
    pub dialect: Option<Dialect>,
    /// Stable diagnostic label.
    pub label: String,
}

/// Resolve unique model bindings and derived labels using the new map schema.
pub fn resolve_model_bindings(
    providers: &BTreeMap<String, ConnectionConfig>,
    models: &BTreeMap<String, ModelConfig>,
) -> Result<Vec<ResolvedBinding>, ConfigError> {
    for name in providers.keys() {
        if name.trim().is_empty() {
            return Err(ConfigError::InvalidProvider(
                "connection name cannot be empty".into(),
            ));
        }
    }
    let mut normalized = HashSet::new();
    let mut bindings = Vec::with_capacity(models.len());
    for (public_model, model) in models {
        if public_model.trim().is_empty() || !normalized.insert(public_model.to_ascii_lowercase()) {
            return Err(ConfigError::InvalidProvider(format!(
                "empty or duplicate public model: {public_model}"
            )));
        }
        let provider = providers.get(&model.upstream.provider).ok_or_else(|| {
            ConfigError::InvalidProvider(format!(
                "unknown connection {} for {public_model}",
                model.upstream.provider
            ))
        })?;
        let canonical = model.model.as_deref().unwrap_or(public_model);
        let upstream_model = model.upstream.model.as_deref().unwrap_or(canonical);
        if canonical.trim().is_empty() || upstream_model.trim().is_empty() {
            return Err(ConfigError::InvalidProvider(format!(
                "empty model identity for {public_model}"
            )));
        }
        if model
            .upstream
            .label
            .as_ref()
            .is_some_and(|label| label.trim().is_empty())
        {
            return Err(ConfigError::InvalidProvider(format!(
                "empty label for {public_model}"
            )));
        }
        parse_response_model_mode(model.response_model.as_deref())
            .map_err(ConfigError::InvalidProvider)?;
        let dialect = if let Some(paths) = &model.serve {
            if model.dialect.is_some() {
                return Err(ConfigError::InvalidProvider(format!(
                    "dialect cannot be set for opaque forwarding model {public_model}"
                )));
            }
            if paths.is_empty() {
                return Err(ConfigError::InvalidProvider(format!(
                    "empty forwarding paths for {public_model}"
                )));
            }
            for path in paths {
                EndpointKind::from_config_path(path)?;
                if is_chat_endpoint_path(path) {
                    return Err(ConfigError::InvalidProvider(format!(
                        "opaque forwarding cannot serve {path}"
                    )));
                }
            }
            None
        } else {
            let integration = models::profile(canonical, provider.upstream).ok_or_else(|| {
                ConfigError::InvalidProvider(format!(
                    "no model profile for {:?}: {canonical}",
                    provider.upstream
                ))
            })?;
            if integration.requires_explicit_dialect && model.dialect.is_none() {
                return Err(ConfigError::InvalidProvider(format!(
                    "dialect must be explicitly declared for model route {public_model}"
                )));
            }
            let selected = model.dialect.unwrap_or(integration.default_dialect);
            if !integration.dialects.contains(&selected) {
                return Err(ConfigError::InvalidProvider(format!(
                    "dialect {selected:?} is not supported for {canonical} on {:?}",
                    provider.upstream
                )));
            }
            Some(selected)
        };
        let label = model
            .upstream
            .label
            .clone()
            .unwrap_or_else(|| format!("{}.{}", model.upstream.provider, upstream_model));
        bindings.push(ResolvedBinding {
            public_model: public_model.clone(),
            model: canonical.to_owned(),
            provider: model.upstream.provider.clone(),
            upstream: provider.upstream,
            upstream_model: upstream_model.to_owned(),
            dialect,
            label,
        });
    }
    Ok(bindings)
}

/// Compile the new connection/model maps into startup-resolved clients.
/// The returned entries are internal connection facts, not a legacy YAML schema.
pub fn compile_model_entries(
    connections: &BTreeMap<String, ConnectionConfig>,
    models: &BTreeMap<String, ModelConfig>,
) -> Result<Vec<ProviderConfig>, ConfigError> {
    let bindings = resolve_model_bindings(connections, models)?;
    let mut entries = Vec::with_capacity(bindings.len());
    for binding in bindings {
        let connection = &connections[&binding.provider];
        let model = &models[&binding.public_model];
        let missing = |field: &str| {
            ConfigError::InvalidProvider(format!("{} requires {field}", binding.provider))
        };
        let base_url = || {
            connection
                .base_url
                .clone()
                .ok_or_else(|| missing("base_url"))
        };
        let kind = match binding.dialect {
            Some(Dialect::OpenAiChatCompletions) => ProviderKind::OpenAiChatCompletions,
            Some(Dialect::OpenAiResponses) => ProviderKind::OpenAiResponses,
            Some(Dialect::AnthropicMessages) => ProviderKind::AnthropicMessages,
            Some(Dialect::BedrockConverse) => ProviderKind::BedrockConverse,
            Some(Dialect::GeminiGenerateContent) => ProviderKind::GoogleVertex,
            None => ProviderKind::Passthrough,
        };
        let params = match kind {
            ProviderKind::OpenAiChatCompletions | ProviderKind::OpenAiResponses => {
                let params = OpenAiParams {
                    base_url: base_url()?,
                    auth: connection.auth.clone(),
                    model: binding.upstream_model.clone(),
                    response_model: model.response_model.clone(),
                };
                if kind == ProviderKind::OpenAiChatCompletions {
                    ProviderKindParams::OpenAiChatCompletions(params)
                } else {
                    ProviderKindParams::OpenAiResponses(params)
                }
            }
            ProviderKind::AnthropicMessages => {
                ProviderKindParams::AnthropicMessages(AnthropicParams {
                    base_url: base_url()?,
                    auth: connection.auth.clone().ok_or_else(|| missing("auth"))?,
                    model: binding.upstream_model.clone(),
                })
            }
            ProviderKind::BedrockConverse => ProviderKindParams::BedrockConverse(BedrockParams {
                model: binding.upstream_model.clone(),
                aws_region: connection
                    .aws_region
                    .clone()
                    .ok_or_else(|| missing("aws_region"))?,
                aws_bearer_token: connection
                    .aws_bearer_token
                    .clone()
                    .ok_or_else(|| missing("aws_bearer_token"))?,
            }),
            ProviderKind::GoogleVertex => ProviderKindParams::GoogleVertex(GoogleVertexParams {
                model: binding.upstream_model.clone(),
                project_id: connection
                    .project_id
                    .clone()
                    .ok_or_else(|| missing("project_id"))?,
                location: connection
                    .location
                    .clone()
                    .ok_or_else(|| missing("location"))?,
                credentials_file: connection
                    .credentials_file
                    .clone()
                    .ok_or_else(|| missing("credentials_file"))?,
            }),
            ProviderKind::Passthrough => {
                let google_vertex = if connection.upstream == Upstream::GoogleVertex {
                    Some(GoogleVertexPassthroughAuth {
                        project_id: connection
                            .project_id
                            .clone()
                            .ok_or_else(|| missing("project_id"))?,
                        location: connection
                            .location
                            .clone()
                            .ok_or_else(|| missing("location"))?,
                        credentials_file: connection
                            .credentials_file
                            .clone()
                            .ok_or_else(|| missing("credentials_file"))?,
                    })
                } else {
                    None
                };
                ProviderKindParams::Passthrough(PassthroughParams {
                    base_url: connection.base_url.clone(),
                    auth: connection.auth.clone(),
                    google_vertex,
                    model: binding.upstream_model.clone(),
                })
            }
        };
        entries.push(ProviderConfig {
            model_id: binding.public_model.clone(),
            kind,
            label: Some(binding.label.clone()),
            serve: model.serve.clone(),
            params,
            binding: Some(binding),
        });
    }
    Ok(entries)
}

#[cfg(test)]
mod binding_tests {
    use super::*;

    #[test]
    fn duplicate_model_and_connection_names_are_rejected_before_overwriting() {
        for raw in [
            r#"{"providers":{"connection":{"upstream":"custom"},"connection":{"upstream":"custom"}}}"#,
            r#"{"models":{"m":{"upstream":{"provider":"p"}},"m":{"upstream":{"provider":"p"}}}}"#,
        ] {
            assert!(serde_json::from_str::<ModelRegistryConfig>(raw).is_err());
            assert!(serde_json::from_str::<ServeConfig>(raw).is_err());
        }
    }

    #[test]
    fn ambiguous_or_missing_credential_sources_are_rejected_without_exposing_secrets() {
        for value in [
            serde_json::json!({"scheme":"bearer"}),
            serde_json::json!({"scheme":"bearer", "api_key_env":""}),
            serde_json::json!({"scheme":"bearer", "api_key":"private-test-value", "api_key_env":"TOKEN"}),
        ] {
            let error = serde_json::from_value::<AuthConfig>(value).expect_err("invalid source");
            assert!(!error.to_string().contains("private-test-value"));
        }
    }

    fn connection() -> ConnectionConfig {
        serde_json::from_value(serde_json::json!({
            "upstream": "tokenhub", "base_url": "https://example.test/v1"
        }))
        .expect("connection")
    }

    #[test]
    fn alias_and_vendor_model_resolve_without_affecting_capability_identity() {
        let providers = BTreeMap::from([("qcloud.cn".to_owned(), connection())]);
        let models = serde_json::from_value(serde_json::json!({
            "fast": {"model":"glm-5.3-flash", "upstream": {
                "provider":"qcloud.cn", "model":"vendor-model-id"
            }}
        }))
        .expect("models");
        let bindings = resolve_model_bindings(&providers, &models).expect("binding");
        let binding = &bindings[0];
        assert_eq!(binding.public_model, "fast");
        assert_eq!(binding.model, "glm-5.3-flash");
        assert_eq!(binding.upstream_model, "vendor-model-id");
        assert_eq!(binding.dialect, Some(Dialect::OpenAiChatCompletions));
        assert_eq!(binding.label, "qcloud.cn.vendor-model-id");
    }

    #[test]
    fn defaults_and_explicit_labels_preserve_stable_identity() {
        let providers = BTreeMap::from([("qcloud.cn".to_owned(), connection())]);
        let models = serde_json::from_value(serde_json::json!({
            "glm-5.3-flash": {"upstream":{"provider":"qcloud.cn", "label":"existing-label"}}
        }))
        .expect("models");
        let bindings = resolve_model_bindings(&providers, &models).expect("binding");
        assert_eq!(bindings[0].model, "glm-5.3-flash");
        assert_eq!(bindings[0].upstream_model, "glm-5.3-flash");
        assert_eq!(bindings[0].label, "existing-label");
    }

    #[test]
    fn unregistered_models_and_missing_connections_are_rejected() {
        let providers = BTreeMap::from([("qcloud.cn".to_owned(), connection())]);
        for value in [
            serde_json::json!({"unknown-model":{"upstream":{"provider":"qcloud.cn"}}}),
            serde_json::json!({"glm-5.3-flash":{"upstream":{"provider":"missing"}}}),
        ] {
            let models = serde_json::from_value(value).expect("model schema");
            assert!(resolve_model_bindings(&providers, &models).is_err());
        }
    }

    #[test]
    fn forwarding_does_not_invent_conversation_capabilities() {
        let providers = BTreeMap::from([("qcloud.cn".to_owned(), connection())]);
        let models = serde_json::from_value(serde_json::json!({
            "embedding": {"upstream":{"provider":"qcloud.cn"}, "serve":["/embeddings"]}
        }))
        .expect("model schema");
        let bindings = resolve_model_bindings(&providers, &models).expect("opaque binding");
        assert_eq!(bindings[0].dialect, None);
    }

    #[test]
    fn explicit_route_dialect_is_selected_and_checked_against_model_upstream_support() {
        let azure = serde_json::from_value(serde_json::json!({
            "upstream": "azure-openai",
            "base_url": "https://example.test/v1"
        }))
        .expect("Azure connection");
        let providers = BTreeMap::from([("azure".to_owned(), azure)]);
        let models = serde_json::from_value(serde_json::json!({
            "gpt-5.6-sol": {
                "upstream": {"provider":"azure"},
                "dialect":"openai-responses"
            }
        }))
        .expect("models");
        let bindings = resolve_model_bindings(&providers, &models).expect("binding");
        assert_eq!(bindings[0].dialect, Some(Dialect::OpenAiResponses));
        let entries = compile_model_entries(&providers, &models).expect("compile");
        assert_eq!(entries[0].kind, ProviderKind::OpenAiResponses);

        let missing_dialect = serde_json::from_value(serde_json::json!({
            "gpt-5.6-sol": {"upstream":{"provider":"azure"}}
        }))
        .expect("models");
        assert!(resolve_model_bindings(&providers, &missing_dialect).is_err());

        let unsupported_models = serde_json::from_value(serde_json::json!({
            "gpt-5.5": {
                "upstream": {"provider":"azure"},
                "dialect":"openai-responses"
            }
        }))
        .expect("models");
        assert!(resolve_model_bindings(&providers, &unsupported_models).is_err());
        assert!(
            serde_json::from_value::<ModelConfig>(serde_json::json!({
                "upstream":{"provider":"azure"},
                "dialect":"openai-future"
            }))
            .is_err()
        );
    }

    #[test]
    fn explicit_dialect_is_not_allowed_for_opaque_forwarding() {
        let providers = BTreeMap::from([("qcloud.cn".to_owned(), connection())]);
        let models = serde_json::from_value(serde_json::json!({
            "embedding": {
                "upstream":{"provider":"qcloud.cn"},
                "serve":["/embeddings"],
                "dialect":"openai-responses"
            }
        }))
        .expect("models");
        assert!(resolve_model_bindings(&providers, &models).is_err());
    }

    #[test]
    fn binding_rejects_upstream_protocol_selection_and_multiple_connections() {
        for value in [
            serde_json::json!({"upstream":{"provider":"qcloud.cn", "protocol":"openai-responses"}}),
            serde_json::json!({"upstreams":[{"provider":"qcloud.cn"}]}),
        ] {
            assert!(serde_json::from_value::<ModelConfig>(value).is_err());
        }
    }

    #[test]
    fn compiles_single_binding_without_changing_public_or_vendor_identity() {
        let providers = BTreeMap::from([("qcloud.cn".to_owned(), connection())]);
        let models = serde_json::from_value(serde_json::json!({
            "fast": {"model":"glm-5.3-flash", "upstream":{"provider":"qcloud.cn", "model":"vendor-id"}}
        })).expect("models");
        let entries = compile_model_entries(&providers, &models).expect("compile");
        assert_eq!(entries.len(), 1);
        assert_eq!(entries[0].model_id, "fast");
        assert_eq!(entries[0].kind, ProviderKind::OpenAiChatCompletions);
        let ProviderKindParams::OpenAiChatCompletions(params) = &entries[0].params else {
            panic!("chat parameters");
        };
        assert_eq!(params.model, "vendor-id");
        assert_eq!(entries[0].label.as_deref(), Some("qcloud.cn.vendor-id"));
    }
}

#[derive(Debug)]
pub enum ConfigError {
    Load(String),
    InvalidProvider(String),
    UnknownProviderType(String),
}

impl fmt::Display for ConfigError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ConfigError::Load(message) => write!(f, "config load error: {message}"),
            ConfigError::InvalidProvider(message) => write!(f, "invalid provider: {message}"),
            ConfigError::UnknownProviderType(name) => {
                write!(f, "unknown provider type: {name}")
            }
        }
    }
}

impl Error for ConfigError {}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum EndpointKind {
    ChatCompletions,
    SystemOne,
    Messages,
    Responses,
    Embeddings,
    Rerank,
    AudioSpeech,
    AudioTranscriptions,
    ImagesGenerations,
    ImagesEdits,
    ModelsGenerateContent,
}

impl EndpointKind {
    pub fn as_path(&self) -> &'static str {
        match self {
            EndpointKind::ChatCompletions => "/chat/completions",
            EndpointKind::SystemOne => "/systemone",
            EndpointKind::Messages => "/messages",
            EndpointKind::Responses => "/responses",
            EndpointKind::Embeddings => "/embeddings",
            EndpointKind::Rerank => "/rerank",
            EndpointKind::AudioSpeech => "/audio/speech",
            EndpointKind::AudioTranscriptions => "/audio/transcriptions",
            EndpointKind::ImagesGenerations => "/images/generations",
            EndpointKind::ImagesEdits => "/images/edits",
            EndpointKind::ModelsGenerateContent => "/models/:generateContent",
        }
    }

    pub fn from_config_path(path: &str) -> Result<Self, ConfigError> {
        Self::from_path(path)
            .ok_or_else(|| ConfigError::InvalidProvider(format!("unknown endpoint path: {path}")))
    }

    pub fn from_request_path(path: &str) -> Option<Self> {
        if parse_generate_content_model(path).is_some() {
            return Some(Self::ModelsGenerateContent);
        }
        Self::from_path(path)
    }

    fn from_path(path: &str) -> Option<Self> {
        match path {
            "/chat/completions" => Some(Self::ChatCompletions),
            "/systemone" => Some(Self::SystemOne),
            "/messages" => Some(Self::Messages),
            "/responses" => Some(Self::Responses),
            "/embeddings" => Some(Self::Embeddings),
            "/rerank" => Some(Self::Rerank),
            "/audio/speech" => Some(Self::AudioSpeech),
            "/audio/transcriptions" => Some(Self::AudioTranscriptions),
            "/images/generations" => Some(Self::ImagesGenerations),
            "/images/edits" => Some(Self::ImagesEdits),
            "/models/:generateContent" => Some(Self::ModelsGenerateContent),
            _ => None,
        }
    }
}
#[derive(Debug, Clone)]
pub struct ServeConfig {
    pub auth_token: Option<String>,
    pub zipper: ZipperConfig,
    pub http_api: HttpApiConfig,
    pub defaults: DefaultsConfig,
    pub providers: Vec<ProviderConfig>,
    /// Named connection definitions used to compile the internal entries.
    pub connections: BTreeMap<String, ConnectionConfig>,
    /// Public model definitions with one upstream each.
    pub models: BTreeMap<String, ModelConfig>,
}

impl<'de> Deserialize<'de> for ServeConfig {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        use serde::de::Error as DeError;
        #[derive(Deserialize, Default)]
        #[serde(default)]
        struct Fields {
            auth_token: Option<String>,
            zipper: ZipperConfig,
            http_api: HttpApiConfig,
            defaults: DefaultsConfig,
            #[serde(deserialize_with = "deserialize_unique_map")]
            providers: BTreeMap<String, ConnectionConfig>,
            #[serde(deserialize_with = "deserialize_unique_map")]
            models: BTreeMap<String, ModelConfig>,
            endpoints: Option<Value>,
        }
        let fields = Fields::deserialize(deserializer)?;
        if fields.endpoints.is_some() {
            return Err(DeError::custom(
                "endpoints configuration is no longer supported; use models and serve",
            ));
        }
        let providers =
            compile_model_entries(&fields.providers, &fields.models).map_err(DeError::custom)?;
        Ok(Self {
            auth_token: fields.auth_token,
            zipper: fields.zipper,
            http_api: fields.http_api,
            defaults: fields.defaults,
            providers,
            connections: fields.providers,
            models: fields.models,
        })
    }
}

impl Default for ServeConfig {
    fn default() -> Self {
        Self {
            auth_token: None,
            zipper: ZipperConfig::default(),
            http_api: HttpApiConfig::default(),
            defaults: DefaultsConfig::default(),
            providers: default_providers(),
            connections: BTreeMap::new(),
            models: BTreeMap::new(),
        }
    }
}

/// Upstream dialect, split by protocol rather than vendor and mapped one-to-one to config `type`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProviderKind {
    OpenAiChatCompletions,
    OpenAiResponses,
    AnthropicMessages,
    BedrockConverse,
    GoogleVertex,
    Passthrough,
}

impl ProviderKind {
    /// The config `type` name (kebab-case).
    pub fn as_config_type(&self) -> &'static str {
        match self {
            ProviderKind::OpenAiChatCompletions => "openai-chat-completions",
            ProviderKind::OpenAiResponses => "openai-responses",
            ProviderKind::AnthropicMessages => "anthropic-messages",
            ProviderKind::BedrockConverse => "bedrock-converse",
            ProviderKind::GoogleVertex => "google-vertex",
            ProviderKind::Passthrough => "passthrough",
        }
    }

    /// Parse from the config `type` name (strict full-word match).
    pub fn from_config_type(value: &str) -> Option<Self> {
        match value {
            "openai-chat-completions" => Some(ProviderKind::OpenAiChatCompletions),
            "openai-responses" => Some(ProviderKind::OpenAiResponses),
            "anthropic-messages" => Some(ProviderKind::AnthropicMessages),
            "bedrock-converse" | "bedrock-converse-stream" => Some(ProviderKind::BedrockConverse),
            "google-vertex" => Some(ProviderKind::GoogleVertex),
            "passthrough" => Some(ProviderKind::Passthrough),
            _ => None,
        }
    }

    /// Whether this kind may bind to the chat endpoints.
    pub fn is_chat_kind(&self) -> bool {
        !matches!(self, ProviderKind::Passthrough)
    }
}

/// Chat endpoint predicate (used by registry validation).
pub fn is_chat_endpoint_path(path: &str) -> bool {
    matches!(path, "/chat/completions" | "/responses" | "/messages")
}

/// Authentication scheme.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum AuthScheme {
    Bearer,
    ApiKey,
}

/// Authentication config. `api_key` is sensitive; Debug output redacts it automatically.
#[derive(Debug, Clone)]
pub struct AuthConfig {
    pub scheme: AuthScheme,
    pub api_key: SecretString,
}

impl<'de> Deserialize<'de> for AuthConfig {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        use serde::de::Error as DeError;
        #[derive(Deserialize)]
        #[serde(deny_unknown_fields)]
        struct Fields {
            scheme: AuthScheme,
            api_key: Option<SecretString>,
            api_key_env: Option<String>,
        }
        let fields = Fields::deserialize(deserializer)?;
        let api_key = match (fields.api_key, fields.api_key_env) {
            (Some(key), None) => key,
            (None, Some(name)) if !name.trim().is_empty() => {
                let value = std::env::var(&name).map_err(|_| {
                    DeError::custom(format!("missing credential environment variable {name}"))
                })?;
                if value.trim().is_empty() {
                    return Err(DeError::custom(format!(
                        "empty credential environment variable {name}"
                    )));
                }
                SecretString::from(value)
            }
            _ => {
                return Err(DeError::custom(
                    "auth requires exactly one of api_key or api_key_env",
                ));
            }
        };
        Ok(Self {
            scheme: fields.scheme,
            api_key,
        })
    }
}

/// Connection params for the openai-family dialects (openai-chat-completions / openai-responses).
#[derive(Debug, Clone, Deserialize)]
pub struct OpenAiParams {
    pub base_url: Url,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub auth: Option<AuthConfig>,
    pub model: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub response_model: Option<String>,
}

/// anthropic-messages connection params. This provider kind is not currently implemented.
#[derive(Debug, Clone, Deserialize)]
pub struct AnthropicParams {
    pub base_url: Url,
    pub auth: AuthConfig,
    pub model: String,
}

/// bedrock-converse connection params.
#[derive(Debug, Clone, Deserialize)]
pub struct BedrockParams {
    pub model: String,
    pub aws_region: String,
    pub aws_bearer_token: SecretString,
}

/// google-vertex connection params.
#[derive(Debug, Clone, Deserialize)]
pub struct GoogleVertexParams {
    pub model: String,
    pub project_id: String,
    pub location: String,
    pub credentials_file: PathBuf,
}

/// passthrough connection params.
///
/// Exactly one of `base_url` / `google_vertex` must be present:
/// `base_url` for static-forward upstreams, `google_vertex` for GCP
/// generateContent passthrough (SA OAuth + URL assembly).
#[derive(Debug, Clone, Deserialize)]
pub struct PassthroughParams {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub base_url: Option<Url>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub auth: Option<AuthConfig>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub google_vertex: Option<GoogleVertexPassthroughAuth>,
    pub model: String,
}

/// GCP generateContent passthrough auth (SA OAuth).
#[derive(Debug, Clone, Deserialize)]
pub struct GoogleVertexPassthroughAuth {
    pub project_id: String,
    pub location: String,
    pub credentials_file: PathBuf,
}

/// Global defaults: `defaults.model` is the single fallback model
/// used when a chat request omits `model`.
#[derive(Debug, Clone, Deserialize, Default, PartialEq)]
#[serde(default)]
pub struct DefaultsConfig {
    pub model: Option<String>,
}

/// Connection params per dialect kind, dispatched to concrete structs by kind and validated by deserialization.
#[derive(Debug, Clone)]
pub enum ProviderKindParams {
    OpenAiChatCompletions(OpenAiParams),
    OpenAiResponses(OpenAiParams),
    /// This provider kind is not currently implemented; `build_client` returns an error.
    AnthropicMessages(AnthropicParams),
    BedrockConverse(BedrockParams),
    GoogleVertex(GoogleVertexParams),
    Passthrough(PassthroughParams),
}

/// Strongly typed provider connection configuration.
///
/// `type`/`model_id`/`serve` sit at the entry top level with connection params nested under `params`;
/// the concrete params struct is decided by kind and mismatches error with field names.
#[derive(Debug, Clone)]
pub struct ProviderConfig {
    pub model_id: String,
    pub kind: ProviderKind,
    /// Display label (o11y / agent-loop), optional.
    pub label: Option<String>,
    /// Used by passthrough only.
    pub serve: Option<Vec<String>>,
    pub params: ProviderKindParams,
    /// Explicit vendor/model identity retained after startup compilation.
    pub binding: Option<ResolvedBinding>,
}

impl<'de> Deserialize<'de> for ProviderConfig {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        use serde::de::Error as DeError;

        let value = Value::deserialize(deserializer)?;
        let object = value
            .as_object()
            .ok_or_else(|| DeError::custom("provider entry must be a mapping"))?;

        let model_id = object
            .get("model_id")
            .and_then(Value::as_str)
            .ok_or_else(|| DeError::missing_field("model_id"))?
            .to_string();
        let type_name = object
            .get("type")
            .and_then(Value::as_str)
            .ok_or_else(|| DeError::missing_field("type"))?;
        let kind = ProviderKind::from_config_type(type_name)
            .ok_or_else(|| DeError::custom(format!("unknown provider type: {type_name}")))?;
        let serve = match object.get("serve") {
            Some(serve) => Some(
                serde_json::from_value::<Vec<String>>(serve.clone()).map_err(DeError::custom)?,
            ),
            None => None,
        };
        let params_value = object
            .get("params")
            .cloned()
            .ok_or_else(|| DeError::missing_field("params"))?;
        let params = match kind {
            ProviderKind::OpenAiChatCompletions => ProviderKindParams::OpenAiChatCompletions(
                serde_json::from_value(params_value).map_err(DeError::custom)?,
            ),
            ProviderKind::OpenAiResponses => ProviderKindParams::OpenAiResponses(
                serde_json::from_value(params_value).map_err(DeError::custom)?,
            ),
            ProviderKind::AnthropicMessages => ProviderKindParams::AnthropicMessages(
                serde_json::from_value(params_value).map_err(DeError::custom)?,
            ),
            ProviderKind::BedrockConverse => ProviderKindParams::BedrockConverse(
                serde_json::from_value(params_value).map_err(DeError::custom)?,
            ),
            ProviderKind::GoogleVertex => ProviderKindParams::GoogleVertex(
                serde_json::from_value(params_value).map_err(DeError::custom)?,
            ),
            ProviderKind::Passthrough => ProviderKindParams::Passthrough(
                serde_json::from_value(params_value).map_err(DeError::custom)?,
            ),
        };

        let label = object
            .get("label")
            .and_then(Value::as_str)
            .map(str::to_string);
        Ok(Self {
            model_id,
            kind,
            label,
            serve,
            params,
            binding: None,
        })
    }
}

/// Derive the endpoint table from typed entries.
///
/// Dialect kinds automatically serve the three chat endpoints; passthrough
/// serves only its `serve` list. `defaults.model` becomes the default model
/// of the three chat endpoints (the single global fallback).
pub fn derive_endpoints_from_typed(
    typed: &[ProviderConfig],
    defaults: &DefaultsConfig,
) -> Vec<EndpointConfig> {
    use std::collections::HashMap;

    fn ensure<'a>(
        by_path: &'a mut HashMap<String, EndpointConfig>,
        path: &str,
    ) -> &'a mut EndpointConfig {
        by_path
            .entry(path.to_string())
            .or_insert_with(|| EndpointConfig {
                path: path.to_string(),
                models: Vec::new(),
                default_model: None,
            })
    }
    let mut by_path: HashMap<String, EndpointConfig> = HashMap::new();

    let chat_paths = [
        EndpointKind::ChatCompletions.as_path(),
        EndpointKind::Responses.as_path(),
        EndpointKind::Messages.as_path(),
    ];

    for provider in typed {
        match &provider.params {
            ProviderKindParams::Passthrough(_) => {
                for path in provider.serve.iter().flatten() {
                    if EndpointKind::from_config_path(path).is_ok() {
                        ensure(&mut by_path, path)
                            .models
                            .push(provider.model_id.clone());
                    }
                }
            }
            _ => {
                for path in chat_paths {
                    ensure(&mut by_path, path)
                        .models
                        .push(provider.model_id.clone());
                }
            }
        }
    }

    let default_model = defaults
        .model
        .as_deref()
        .map(str::trim)
        .filter(|model| !model.is_empty())
        .map(str::to_string);
    if let Some(default_model) = default_model {
        for path in chat_paths {
            if let Some(endpoint) = by_path.get_mut(path) {
                endpoint.default_model = Some(default_model.clone());
            }
        }
    }

    let mut endpoints: Vec<EndpointConfig> = by_path.into_values().collect();
    endpoints.sort_by(|a, b| a.path.cmp(&b.path));
    endpoints
}

impl ProviderConfig {
    pub fn model_id(&self) -> &str {
        &self.model_id
    }
}

/// How the `model` field in responses is chosen.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ResponseModelMode {
    /// Preserve the upstream model name, falling back to the routed model.
    Upstream,
    /// Always return the configured model name.
    Fixed(String),
}

/// Parse `response_model`: unset preserves the upstream name, and any
/// non-empty string fixes the returned name.
pub fn parse_response_model_mode(value: Option<&str>) -> Result<ResponseModelMode, String> {
    let Some(value) = value else {
        return Ok(ResponseModelMode::Upstream);
    };
    let value = value.trim();
    if value.is_empty() {
        return Err("response_model cannot be empty".to_string());
    }
    Ok(ResponseModelMode::Fixed(value.to_string()))
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(default)]
pub struct EndpointConfig {
    pub path: String,
    pub models: Vec<String>,
    pub default_model: Option<String>,
}

impl EndpointConfig {
    pub fn endpoint(&self) -> Result<EndpointKind, ConfigError> {
        EndpointKind::from_config_path(self.path.as_str())
    }
}

pub fn parse_generate_content_model(endpoint: &str) -> Option<String> {
    endpoint
        .strip_prefix("/models/")
        .and_then(|value| value.strip_suffix(":generateContent"))
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_string)
}

/// Default host address
fn default_host() -> String {
    "127.0.0.1".to_string()
}

/// Default Zipper QUIC port
fn default_zipper_port() -> u16 {
    9000
}

/// Default Http API HTTP port
fn default_http_api_port() -> u16 {
    9001
}

/// Default providers: empty. Configs must declare providers explicitly;
/// an empty providers table surfaces as a clear startup error instead of
/// silently serving a sample model.
fn default_providers() -> Vec<ProviderConfig> {
    Vec::new()
}

/// Default tool api prefix
fn default_tool_api_prefix() -> Option<String> {
    Some("/tool".to_string())
}

#[derive(Debug, Clone, Deserialize)]
pub struct ZipperConfig {
    #[serde(default = "default_host")]
    pub host: String,

    #[serde(default = "default_zipper_port")]
    pub port: u16,

    #[serde(default)]
    pub tls: TlsConfig,
}

impl Default for ZipperConfig {
    fn default() -> Self {
        Self {
            host: default_host(),
            port: default_zipper_port(),
            tls: TlsConfig::default(),
        }
    }
}

#[derive(Debug, Clone, Deserialize)]
pub struct HttpApiConfig {
    #[serde(default = "default_host")]
    pub host: String,

    #[serde(default = "default_http_api_port")]
    pub port: u16,

    #[serde(default = "default_tool_api_prefix")]
    pub tool_api_prefix: Option<String>,
}

impl Default for HttpApiConfig {
    fn default() -> Self {
        Self {
            host: default_host(),
            port: default_http_api_port(),
            tool_api_prefix: default_tool_api_prefix(),
        }
    }
}

impl ServeConfig {
    pub fn validate(&self) -> Result<(), ConfigError> {
        let mut model_ids = HashSet::new();
        for provider in &self.providers {
            if provider.model_id().trim().is_empty() {
                return Err(ConfigError::InvalidProvider(
                    "model_id is required for provider".to_string(),
                ));
            }

            let normalized_model_id = provider.model_id().to_ascii_lowercase();
            if !model_ids.insert(normalized_model_id) {
                return Err(ConfigError::InvalidProvider(format!(
                    "duplicate model_id: {}",
                    provider.model_id()
                )));
            }
        }

        // defaults.model must reference a known model.
        if let Some(default_model) = self
            .defaults
            .model
            .as_deref()
            .map(str::trim)
            .filter(|model| !model.is_empty())
        {
            if !self
                .providers
                .iter()
                .any(|provider| provider.model_id().eq_ignore_ascii_case(default_model))
            {
                return Err(ConfigError::InvalidProvider(format!(
                    "defaults.model not found: {default_model}"
                )));
            }
        }

        for endpoint in &derive_endpoints_from_typed(&self.providers, &self.defaults) {
            if endpoint.path.trim().is_empty() {
                return Err(ConfigError::InvalidProvider(
                    "endpoint path is required".to_string(),
                ));
            }
            endpoint.endpoint()?;
            if let Some(default_model) = &endpoint.default_model {
                if default_model.trim().is_empty() {
                    return Err(ConfigError::InvalidProvider(
                        "default_model is empty".to_string(),
                    ));
                }
                if !self
                    .providers
                    .iter()
                    .any(|provider| provider.model_id().eq_ignore_ascii_case(default_model))
                {
                    return Err(ConfigError::InvalidProvider(format!(
                        "endpoint default_model not found: {}",
                        default_model
                    )));
                }
            }
            for model in &endpoint.models {
                if !self
                    .providers
                    .iter()
                    .any(|provider| provider.model_id() == *model)
                {
                    return Err(ConfigError::InvalidProvider(format!(
                        "endpoint model not found: {}",
                        model
                    )));
                }
            }
        }

        Ok(())
    }
}

#[cfg(test)]
mod typed_provider_schema_tests {
    use super::{AuthScheme, ProviderConfig, ProviderKind, ProviderKindParams};

    fn parse_typed(json: &str) -> Result<ProviderConfig, serde_json::Error> {
        serde_json::from_str(json)
    }

    #[test]
    fn all_kinds_parse_valid_entries() {
        let entries: Vec<ProviderConfig> = serde_json::from_str(
            r#"[
                {"type": "openai-chat-completions", "model_id": "glm-5.3-flash",
                 "params": {"base_url": "https://tokenhub.example.com/v1",
                            "auth": {"scheme": "bearer", "api_key": "sk-1"}, "model": "glm-5.3-flash"}},
                {"type": "openai-responses", "model_id": "gpt-5.3-codex",
                 "params": {"base_url": "https://vgllm.example.com/openai/v1",
                            "auth": {"scheme": "api-key", "api_key": "k"}, "model": "gpt-5.3-codex"}},
                {"type": "anthropic-messages", "model_id": "claude-direct",
                 "params": {"base_url": "https://api.anthropic.com",
                            "auth": {"scheme": "bearer", "api_key": "k"}, "model": "claude-opus-5"}},
                {"type": "bedrock-converse", "model_id": "claude-opus-5",
                 "params": {"model": "global.anthropic.claude-opus-5", "aws_region": "ap-northeast-3",
                            "aws_bearer_token": "tok"}},
                {"type": "google-vertex", "model_id": "gemini-3.5-flash",
                 "params": {"model": "gemini-3.5-flash", "project_id": "pc-1", "location": "global",
                            "credentials_file": "/path/sa.json"}},
                {"type": "passthrough", "model_id": "BAAI/bge-m3", "serve": ["/embeddings"],
                 "params": {"base_url": "http://127.0.0.1:8001/v1", "model": "BAAI/bge-m3"}}
            ]"#,
        )
        .expect("all kinds must parse");

        assert_eq!(entries.len(), 6);
    }

    #[test]
    fn kind_params_map_to_provider_kind() {
        let typed: ProviderConfig = parse_typed(
            r#"{"type": "openai-chat-completions", "model_id": "m",
                "params": {"base_url": "https://x.example.com/v1",
                           "auth": {"scheme": "bearer", "api_key": "k"}, "model": "m"}}"#,
        )
        .expect("parse");
        assert_eq!(typed.kind, ProviderKind::OpenAiChatCompletions);
        match &typed.params {
            ProviderKindParams::OpenAiChatCompletions(params) => {
                assert_eq!(params.base_url.as_str(), "https://x.example.com/v1");
                let auth = params.auth.as_ref().expect("auth");
                assert_eq!(auth.scheme, AuthScheme::Bearer);
                assert_eq!(params.model, "m");
                assert!(params.response_model.is_none());
            }
            other => panic!("unexpected kind: {other:?}"),
        }
    }

    #[test]
    fn passthrough_parses_serve_list_and_optional_auth() {
        let typed: ProviderConfig = parse_typed(
            r#"{"type": "passthrough", "model_id": "img", "serve": ["/images/generations", "/images/edits"],
                "params": {"base_url": "https://x.example.com",
                           "auth": {"scheme": "api-key", "api_key": "k"}, "model": "gpt-image-1"}}"#,
        )
        .expect("parse");
        assert_eq!(
            typed.serve.as_deref(),
            Some(
                &[
                    "/images/generations".to_string(),
                    "/images/edits".to_string()
                ][..]
            )
        );
        match &typed.params {
            ProviderKindParams::Passthrough(params) => {
                let auth = params.auth.as_ref().expect("auth");
                assert_eq!(auth.scheme, AuthScheme::ApiKey);
            }
            other => panic!("unexpected kind: {other:?}"),
        }
    }

    #[test]
    fn missing_required_param_field_is_rejected() {
        let err = parse_typed(
            r#"{"type": "openai-chat-completions", "model_id": "m",
                "params": {"model": "m"}}"#,
        )
        .expect_err("missing base_url must be rejected");
        let message = err.to_string();
        assert!(message.contains("base_url"), "got: {message}");
    }

    #[test]
    fn openai_kind_auth_is_optional() {
        let typed: ProviderConfig = parse_typed(
            r#"{"type": "openai-chat-completions", "model_id": "m",
                "params": {"base_url": "https://x.example.com/v1", "model": "m"}}"#,
        )
        .expect("parse");
        match &typed.params {
            ProviderKindParams::OpenAiChatCompletions(params) => {
                assert!(params.auth.is_none());
            }
            other => panic!("unexpected kind: {other:?}"),
        }
    }

    #[test]
    fn missing_params_is_rejected() {
        let err = parse_typed(r#"{"type": "openai-chat-completions", "model_id": "m"}"#)
            .expect_err("missing params must be rejected");
        assert!(err.to_string().contains("params"));
    }

    #[test]
    fn unknown_type_does_not_parse_as_typed() {
        let err = serde_json::from_str::<ProviderConfig>(
            r#"{"type": "not-a-kind", "model_id": "m", "params": {}}"#,
        )
        .expect_err("unknown type must be rejected");
        assert!(err.to_string().contains("unknown provider type"));
    }

    #[test]
    fn old_provider_type_is_rejected() {
        let err = serde_json::from_str::<Vec<ProviderConfig>>(
            r#"[{"type": "tokenhub", "model_id": "glm", "params": {"api_key": "k"}}]"#,
        )
        .expect_err("old provider type must be rejected");
        assert!(err.to_string().contains("unknown provider type: tokenhub"));
    }

    #[test]
    fn explicit_endpoint_configuration_is_rejected() {
        let err = serde_json::from_str::<super::ServeConfig>(r#"{"endpoints": []}"#)
            .expect_err("endpoint bindings must be derived");
        assert!(
            err.to_string()
                .contains("endpoints configuration is no longer supported")
        );
    }

    #[test]
    fn provider_kind_config_type_roundtrip() {
        for kind in [
            ProviderKind::OpenAiChatCompletions,
            ProviderKind::OpenAiResponses,
            ProviderKind::AnthropicMessages,
            ProviderKind::BedrockConverse,
            ProviderKind::GoogleVertex,
            ProviderKind::Passthrough,
        ] {
            assert_eq!(
                ProviderKind::from_config_type(kind.as_config_type()),
                Some(kind)
            );
        }
        assert_eq!(
            ProviderKind::from_config_type("bedrock-converse-stream"),
            Some(ProviderKind::BedrockConverse)
        );
        assert_eq!(ProviderKind::from_config_type("tokenhub"), None);
    }
}

#[cfg(test)]
mod defaults_tests {
    use super::{DefaultsConfig, ServeConfig};

    #[test]
    fn defaults_model_parses_and_defaults_to_none() {
        let config: ServeConfig =
            serde_json::from_str(r#"{"defaults": {"model": "glm-5.3-flash"}, "providers": {}}"#)
                .expect("parse");
        assert_eq!(
            config.defaults,
            DefaultsConfig {
                model: Some("glm-5.3-flash".to_string())
            }
        );

        let config: ServeConfig = serde_json::from_str(r#"{}"#).expect("parse empty");
        assert_eq!(config.defaults, DefaultsConfig::default());
        assert!(config.defaults.model.is_none());
    }

    #[test]
    fn defaults_model_must_reference_known_model() {
        let config: ServeConfig = serde_json::from_str(
            r#"{
                "defaults": {"model": "unknown-model"},
                "providers": {}
            }"#,
        )
        .expect("parse");
        let err = config
            .validate()
            .expect_err("unknown default must be rejected");
        assert!(err.to_string().contains("defaults.model not found"));
    }

    #[test]
    fn defaults_model_known_model_passes_validation() {
        let config: ServeConfig = serde_json::from_str(
            r#"{
                "defaults": {"model": "GLM-Test"},
                "providers": {"qcloud.cn":{"upstream":"tokenhub", "base_url":"https://x.example.com/v1"}},
                "models": {"glm-test":{"model":"glm-5.3-flash", "upstream":{"provider":"qcloud.cn", "model":"m"}}}
            }"#,
        )
        .expect("parse");
        config
            .validate()
            .expect("known default passes (case-insensitive)");
    }

    #[test]
    fn old_provider_lists_are_not_accepted_by_the_new_schema() {
        assert!(serde_json::from_str::<super::ModelRegistryConfig>(r#"{"providers":[]}"#).is_err());
    }
}

#[cfg(test)]
mod tests {
    use super::{EndpointKind, ResponseModelMode, parse_response_model_mode};

    #[test]
    fn endpoint_kind_maps_systemone_path() {
        let endpoint = EndpointKind::from_request_path("/systemone");

        assert_eq!(endpoint, Some(EndpointKind::SystemOne));
        assert_eq!(EndpointKind::SystemOne.as_path(), "/systemone");
    }

    #[test]
    fn parse_response_model_mode_defaults_to_upstream() {
        let mode = parse_response_model_mode(None).expect("default response model mode");

        assert_eq!(mode, ResponseModelMode::Upstream);
    }

    #[test]
    fn parse_response_model_mode_parses_plain_string() {
        let fixed = parse_response_model_mode(Some("gpt-4.1")).expect("fixed model");

        assert_eq!(fixed, ResponseModelMode::Fixed("gpt-4.1".to_string()));
    }

    #[test]
    fn parse_response_model_mode_rejects_empty_values() {
        assert!(parse_response_model_mode(Some("   ")).is_err());
    }
}

#[cfg(test)]
mod derive_endpoints_tests {
    use super::{DefaultsConfig, EndpointConfig, ProviderConfig, derive_endpoints_from_typed};

    fn parse_typed(json: &str) -> ProviderConfig {
        serde_json::from_str(json).expect("typed entry")
    }

    fn find<'a>(endpoints: &'a [EndpointConfig], path: &str) -> &'a EndpointConfig {
        endpoints
            .iter()
            .find(|endpoint| endpoint.path == path)
            .unwrap_or_else(|| panic!("endpoint {path} missing"))
    }

    #[test]
    fn dialect_models_serve_three_chat_endpoints() {
        let typed = vec![parse_typed(
            r#"{"type": "openai-chat-completions", "model_id": "glm",
                "params": {"base_url": "https://x.example.com/v1",
                           "auth": {"scheme": "bearer", "api_key": "k"}, "model": "glm-up"}}"#,
        )];
        let endpoints = derive_endpoints_from_typed(&typed, &DefaultsConfig::default());

        for path in ["/chat/completions", "/responses", "/messages"] {
            assert_eq!(find(&endpoints, path).models, vec!["glm".to_string()]);
        }
        // passthrough paths are only derived when a passthrough entry declares them
        assert!(
            !endpoints
                .iter()
                .any(|endpoint| endpoint.path == "/embeddings")
        );
    }

    #[test]
    fn passthrough_models_serve_only_their_list() {
        let typed = vec![parse_typed(
            r#"{"type": "passthrough", "model_id": "bge", "serve": ["/embeddings", "/rerank"],
                "params": {"base_url": "http://127.0.0.1:8001/v1", "model": "BAAI/bge-m3"}}"#,
        )];
        let endpoints = derive_endpoints_from_typed(&typed, &DefaultsConfig::default());

        assert_eq!(
            find(&endpoints, "/embeddings").models,
            vec!["bge".to_string()]
        );
        assert_eq!(find(&endpoints, "/rerank").models, vec!["bge".to_string()]);
        // chat endpoints are only derived for dialect entries
        assert!(
            !endpoints
                .iter()
                .any(|endpoint| endpoint.path == "/chat/completions")
        );
    }

    #[test]
    fn defaults_model_becomes_chat_default() {
        let typed = vec![parse_typed(
            r#"{"type": "openai-chat-completions", "model_id": "glm",
                "params": {"base_url": "https://x.example.com/v1",
                           "auth": {"scheme": "bearer", "api_key": "k"}, "model": "glm-up"}}"#,
        )];
        let defaults = DefaultsConfig {
            model: Some("glm".to_string()),
        };
        let endpoints = derive_endpoints_from_typed(&typed, &defaults);

        for path in ["/chat/completions", "/responses", "/messages"] {
            assert_eq!(find(&endpoints, path).default_model.as_deref(), Some("glm"));
        }
    }
}
