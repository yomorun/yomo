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
#[derive(Debug, Clone, Deserialize)]
#[serde(default)]
pub struct ServeConfig {
    pub auth_token: Option<String>,
    pub zipper: ZipperConfig,
    pub http_api: HttpApiConfig,
    #[serde(default = "default_llm_providers")]
    pub llm_providers: Vec<ProviderConfig>,
    #[serde(default = "default_llm_model")]
    pub llm_default_model_id: Option<String>,
    pub model_api: ModelApiConfig,
}

impl Default for ServeConfig {
    fn default() -> Self {
        Self {
            auth_token: None,
            zipper: ZipperConfig::default(),
            http_api: HttpApiConfig::default(),
            llm_providers: default_llm_providers(),
            llm_default_model_id: default_llm_model(),
            model_api: ModelApiConfig::default(),
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
pub struct ModelApiConfig {
    pub providers: Vec<ProviderConfig>,
    pub endpoints: Vec<ModelApiEndpointConfig>,
}

#[derive(Debug, Clone, Deserialize, Default)]
#[serde(default)]
pub struct ModelApiEndpointConfig {
    pub path: String,
    pub models: Vec<String>,
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

/// Default LLM providers
fn default_llm_providers() -> Vec<ProviderConfig> {
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

/// Default LLM model
fn default_llm_model() -> Option<String> {
    Some("ornith".to_string())
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
    pub fn validate(&self) -> Result<(), ConfigError> {
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

        if let Some(default_model) = &self.llm_default_model_id {
            if !self
                .llm_providers
                .iter()
                .any(|p| p.model_id == *default_model)
            {
                return Err(ConfigError::InvalidProvider(format!(
                    "llm_default_model_id not found in llm_providers: {}",
                    default_model
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
