use serde::Deserialize;

use crate::tls::TlsConfig;

#[derive(Debug, Clone, Deserialize)]
pub struct QuicConfig {
    #[serde(default = "default_host")]
    pub host: String,

    #[serde(default = "default_quic_port")]
    pub port: u16,

    #[serde(default)]
    pub tls: TlsConfig,
}

impl Default for QuicConfig {
    fn default() -> Self {
        Self {
            host: default_host(),
            port: default_quic_port(),
            tls: TlsConfig::default(),
        }
    }
}

#[derive(Debug, Clone, Deserialize)]
pub struct HttpConfig {
    #[serde(default = "default_host")]
    pub host: String,

    #[serde(default = "default_http_port")]
    pub port: u16,
}

impl Default for HttpConfig {
    fn default() -> Self {
        Self {
            host: default_host(),
            port: default_http_port(),
        }
    }
}

#[derive(Debug, Clone, Deserialize, Default)]
pub struct MiddlewareConfig {
    #[serde(default)]
    pub auth_token: Option<String>,
}

#[derive(Debug, Clone, Deserialize, Default)]
pub struct ZipperConfig<T> {
    #[serde(default)]
    pub quic: QuicConfig,

    #[serde(default)]
    pub http: HttpConfig,

    #[serde(default)]
    pub middleware: T,
}

fn default_host() -> String {
    "0.0.0.0".to_string()
}

fn default_quic_port() -> u16 {
    9000
}

fn default_http_port() -> u16 {
    9001
}
