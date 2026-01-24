use std::path::Path;

use anyhow::{Result, bail};
use bon::Builder;
use log::warn;
use s2n_quic::provider::tls::default::{Client, Server, callbacks::VerifyHostNameCallback};
use serde::Deserialize;

/// TLS configuration
#[derive(Debug, Clone, Deserialize, Default, Builder)]
pub struct TlsConfig {
    ca_cert: Option<String>,
    cert: Option<String>,
    key: Option<String>,
    #[serde(default)]
    mutual: bool,
}

const YOMO_TLS_PROTOCOL: [&str; 1] = ["yomo-v2"];

static LOCAL_CERT_PEM: &str = include_str!(concat!(env!("CARGO_MANIFEST_DIR"), "/certs/cert.pem"));
static LOCAL_KEY_PEM: &str = include_str!(concat!(env!("CARGO_MANIFEST_DIR"), "/certs/key.pem"));

/// Create server TLS configuration
pub(crate) fn new_server_tls(c: &TlsConfig) -> Result<Server> {
    let mut builder = Server::builder()
        .with_application_protocols(YOMO_TLS_PROTOCOL)?
        .with_trusted_certificate(LOCAL_CERT_PEM)?;

    if let Some(c) = &c.ca_cert {
        builder = builder.with_trusted_certificate(Path::new(c))?;
    }

    if let (Some(c), Some(k)) = (&c.cert, &c.key) {
        builder = builder.with_certificate(Path::new(c), Path::new(k))?;
    } else {
        builder = builder.with_certificate(LOCAL_CERT_PEM, LOCAL_KEY_PEM)?;
    }

    if c.mutual {
        builder = builder.with_client_authentication()?;
    }

    let tls = builder.build()?;

    Ok(tls)
}

/// Skip hostname verification (for insecure mode)
struct SkipVerify;

impl VerifyHostNameCallback for SkipVerify {
    fn verify_host_name(&self, _host_name: &str) -> bool {
        true
    }
}

/// Create client TLS configuration
pub(crate) fn new_client_tls(c: &TlsConfig, insecure: bool) -> Result<Client> {
    let mut builder = Client::builder()
        .with_application_protocols(YOMO_TLS_PROTOCOL)?
        .with_certificate(LOCAL_CERT_PEM)?;

    if insecure {
        warn!("tls insecure mode is enabled, please don't use it in production");

        builder = builder.with_verify_host_name_callback(SkipVerify {})?;
    }

    if let Some(c) = &c.ca_cert {
        builder = builder.with_certificate(Path::new(c))?;
    }

    if c.mutual {
        builder = match (&c.cert, &c.key) {
            (Some(c), Some(k)) => builder.with_client_identity(Path::new(c), Path::new(k))?,
            (None, None) => builder.with_client_identity(LOCAL_CERT_PEM, LOCAL_KEY_PEM)?,
            _ => {
                bail!("both cert and key must be provided");
            }
        }
    }

    let tls = builder.build()?;

    Ok(tls)
}
