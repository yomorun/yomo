use serde::Deserialize;
use std::collections::{HashMap, HashSet};
use std::error::Error;
use std::fmt;

use crate::tls::TlsConfig;

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
#[derive(Debug, Clone, Deserialize)]
#[serde(default)]
pub struct ServeConfig {
    pub auth_token: Option<String>,
    pub zipper: ZipperConfig,
    pub http_api: HttpApiConfig,
    #[serde(default = "default_providers")]
    pub providers: Vec<ProviderConfig>,
    #[serde(default = "default_endpoints")]
    pub endpoints: Vec<EndpointConfig>,
}

impl Default for ServeConfig {
    fn default() -> Self {
        Self {
            auth_token: None,
            zipper: ZipperConfig::default(),
            http_api: HttpApiConfig::default(),
            providers: default_providers(),
            endpoints: default_endpoints(),
        }
    }
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(default)]
pub struct ProviderConfig {
    #[serde(rename = "type")]
    pub provider_type: String,
    pub model_id: String,
    pub label: Option<String>,
    pub params: HashMap<String, String>,
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

/// Default providers
fn default_providers() -> Vec<ProviderConfig> {
    vec![ProviderConfig {
        provider_type: "openai-compatible".to_string(),
        model_id: "ornith".to_string(),
        params: [(
            "base_url".to_string(),
            "http://127.0.0.1:11434/v1".to_string(),
        )]
        .into(),
        ..Default::default()
    }]
}

/// Default endpoint mapping
fn default_endpoints() -> Vec<EndpointConfig> {
    vec![EndpointConfig {
        path: "/chat/completions".to_string(),
        models: vec!["ornith".to_string()],
        default_model: Some("ornith".to_string()),
    }]
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
            if provider.provider_type.trim().is_empty() {
                return Err(ConfigError::InvalidProvider(format!(
                    "provider type is required for {}",
                    provider.model_id
                )));
            }

            if provider.model_id.trim().is_empty() {
                return Err(ConfigError::InvalidProvider(
                    "model_id is required for provider".to_string(),
                ));
            }

            let normalized_model_id = provider.model_id.to_ascii_lowercase();
            if !model_ids.insert(normalized_model_id) {
                return Err(ConfigError::InvalidProvider(format!(
                    "duplicate model_id: {}",
                    provider.model_id
                )));
            }
        }

        for endpoint in &self.endpoints {
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
                    .any(|provider| provider.model_id == *default_model)
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
                    .any(|provider| provider.model_id == *model)
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
