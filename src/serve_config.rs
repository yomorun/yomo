use serde::Deserialize;
use std::collections::{HashMap, HashSet};
use std::error::Error;
use std::fmt;
use std::path::Path;

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
#[derive(Debug, Clone, Deserialize, Default)]
#[serde(default, rename_all = "snake_case")]
pub struct ServeConfig {
    pub zipper: ZipperConfig,
    pub http_api: HttpApiConfig,
    #[serde(default)]
    pub llm_providers: Vec<ProviderConfig>,
    #[serde(default)]
    pub model_api: ModelApiConfig,
}

#[derive(Debug, Deserialize, Clone)]
pub struct ProviderConfig {
    #[serde(rename = "type")]
    pub provider_type: String,
    pub model_id: String,
    #[serde(default)]
    pub default: bool,
    #[serde(default)]
    pub params: HashMap<String, String>,
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(default)]
pub struct ModelApiConfig {
    pub providers: Vec<ProviderConfig>,
    pub endpoints: Vec<ModelApiEndpointConfig>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct ModelApiEndpointConfig {
    pub path: String,
    #[serde(default)]
    pub models: Vec<String>,
    #[serde(default)]
    pub default_model: Option<String>,
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

/// Default LLM base URL
fn default_llm_base_url() -> String {
    "http://127.0.0.1:11434".to_string()
}

#[derive(Debug, Clone, Deserialize)]
pub struct ZipperConfig {
    #[serde(default = "default_host")]
    pub host: String,

    #[serde(default = "default_zipper_port")]
    pub port: u16,

    #[serde(default)]
    pub tls: TlsConfig,

    #[serde(default)]
    pub auth_token: Option<String>,
}

impl Default for ZipperConfig {
    fn default() -> Self {
        Self {
            host: default_host(),
            port: default_zipper_port(),
            tls: TlsConfig::default(),
            auth_token: None,
        }
    }
}

#[derive(Debug, Clone, Deserialize)]
pub struct LlmConfig {
    #[serde(default = "default_llm_base_url")]
    pub base_url: String,

    #[serde(default)]
    pub api_key: String,
}

impl Default for LlmConfig {
    fn default() -> Self {
        Self {
            base_url: default_llm_base_url(),
            api_key: String::new(),
        }
    }
}

#[derive(Debug, Clone, Deserialize)]
pub struct HttpApiConfig {
    #[serde(default = "default_host")]
    pub host: String,

    #[serde(default = "default_http_api_port")]
    pub port: u16,

    #[serde(default)]
    pub enable_tool_api: bool,
}

impl Default for HttpApiConfig {
    fn default() -> Self {
        Self {
            host: default_host(),
            port: default_http_api_port(),
            enable_tool_api: false,
        }
    }
}


impl ServeConfig {
    pub fn load(path: impl AsRef<Path>) -> Result<Self, ConfigError> {
        let path = path.as_ref();
        let config = config::Config::builder()
            .add_source(config::File::from(path))
            .build()
            .map_err(|err| ConfigError::Load(err.to_string()))?;
        let parsed: Self = config
            .try_deserialize()
            .map_err(|err| ConfigError::Load(err.to_string()))?;
        Ok(parsed)
    }

    pub fn validate(&self) -> Result<(), ConfigError> {
        if self.llm_providers.is_empty() {
            return Err(ConfigError::InvalidProvider(
                "llm_providers list is empty".to_string(),
            ));
        }

        let mut model_ids = HashSet::new();
        for provider in &self.llm_providers {
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

        if !self.model_api.providers.is_empty() || !self.model_api.endpoints.is_empty() {
            let mut model_ids = HashSet::new();
            for provider in &self.model_api.providers {
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

            for endpoint in &self.model_api.endpoints {
                if endpoint.path.trim().is_empty() {
                    return Err(ConfigError::InvalidProvider(
                        "model_api endpoint path is required".to_string(),
                    ));
                }
                if let Some(default_model) = &endpoint.default_model {
                    if default_model.trim().is_empty() {
                        return Err(ConfigError::InvalidProvider(
                            "model_api default_model is empty".to_string(),
                        ));
                    }
                    if !self
                        .model_api
                        .providers
                        .iter()
                        .any(|provider| provider.model_id == *default_model)
                    {
                        return Err(ConfigError::InvalidProvider(format!(
                            "model_api default_model not found: {}",
                            default_model
                        )));
                    }
                }
                for model in &endpoint.models {
                    if !self
                        .model_api
                        .providers
                        .iter()
                        .any(|provider| provider.model_id == *model)
                    {
                        return Err(ConfigError::InvalidProvider(format!(
                            "model_api endpoint model not found: {}",
                            model
                        )));
                    }
                }
            }
        }

        Ok(())
    }
}
