use std::{io::Cursor, sync::Arc};

use anyhow::{Result, bail};
use bon::Builder;
use log::warn;
use rustls_pki_types::pem::PemObject;
#[allow(deprecated)]
use s2n_quic::provider::tls::rustls::rustls::{
    self as rustls_crate, Error as RustlsError, RootCertStore,
    pki_types::{CertificateDer, PrivateKeyDer},
    server::WebPkiClientVerifier,
};
use s2n_quic::provider::tls::{self as s2n_quic_tls_provider};
use serde::Deserialize;
use tokio::{fs::File, io::AsyncReadExt};

const YOMO_TLS_PROTOCOL: &[u8] = b"yomo-v2";

static DEV_CA_CERT: &[u8] = include_bytes!(concat!(env!("CARGO_MANIFEST_DIR"), "/certs/ca.pem"));
static DEV_SERVER_CERT: &[u8] =
    include_bytes!(concat!(env!("CARGO_MANIFEST_DIR"), "/certs/server.pem"));
static DEV_SERVER_KEY: &[u8] =
    include_bytes!(concat!(env!("CARGO_MANIFEST_DIR"), "/certs/server_key.pem"));

pub(crate) struct TlsProvider {
    ca_cert: Option<CertificateDer<'static>>,
    cert_pem: Option<Vec<u8>>,
    private_key: Option<PrivateKeyDer<'static>>,
    mutual: bool,
}

impl s2n_quic_tls_provider::Provider for TlsProvider {
    type Server = s2n_quic_tls_provider::rustls::Server;
    type Client = s2n_quic_tls_provider::rustls::Client;
    type Error = RustlsError;

    fn start_server(self) -> Result<Self::Server, Self::Error> {
        if let (Some(cert_pem), Some(private_key)) = (self.cert_pem, self.private_key) {
            let builder = rustls_crate::ServerConfig::builder_with_protocol_versions(
                &rustls_crate::ALL_VERSIONS,
            );

            let cert_chain = into_certificates(&cert_pem)?;

            let mut cfg = if self.mutual {
                let roots = match self.ca_cert {
                    Some(ref ca) => into_root_store(Some(ca.clone()), false)?,
                    None => {
                        return Err(Self::Error::General(
                            "CA cert is required for mutual TLS".to_string(),
                        ));
                    }
                };
                builder.with_client_cert_verifier(
                    WebPkiClientVerifier::builder(Arc::new(roots))
                        .build()
                        .map_err(|e| Self::Error::General(e.to_string()))?,
                )
            } else {
                builder.with_no_client_auth()
            }
            .with_single_cert(cert_chain, private_key)?;

            cfg.ignore_client_order = true;
            cfg.max_fragment_size = None;
            cfg.alpn_protocols = vec![YOMO_TLS_PROTOCOL.to_vec()];

            Ok(cfg.into())
        } else {
            return Err(Self::Error::General(
                "Server cert and private key are missing".to_string(),
            ));
        }
    }

    fn start_client(self) -> Result<Self::Client, Self::Error> {
        let roots = into_root_store(self.ca_cert, true)?;

        let builder = rustls_crate::ClientConfig::builder().with_root_certificates(roots);

        let mut cfg = if self.mutual {
            if let (Some(cert_pem), Some(private_key)) = (self.cert_pem, self.private_key) {
                let cert_chain = into_certificates(&cert_pem)?;
                builder.with_client_auth_cert(cert_chain, private_key)?
            } else {
                return Err(Self::Error::General(
                    "client cert and private key are required for mutual TLS".to_string(),
                ));
            }
        } else {
            builder.with_no_client_auth()
        };

        cfg.max_fragment_size = None;
        cfg.alpn_protocols = vec![YOMO_TLS_PROTOCOL.to_vec()];

        return Ok(cfg.into());
    }
}

impl TlsProvider {
    pub fn new(
        ca_cert_pem: Option<Vec<u8>>,
        cert_pem: Option<Vec<u8>>,
        key_pem: Option<Vec<u8>>,
        mutual: bool,
    ) -> Result<Self, RustlsError> {
        Ok(Self {
            ca_cert: if let Some(buf) = ca_cert_pem {
                Some(into_certificate(&buf)?)
            } else {
                None
            },
            cert_pem,
            private_key: if let Some(buf) = key_pem {
                Some(into_private_key(&buf)?)
            } else {
                None
            },
            mutual,
        })
    }
}

async fn read_file(path: &str) -> Result<Vec<u8>, RustlsError> {
    let mut f = File::open(path)
        .await
        .map_err(|e| RustlsError::General(e.to_string()))?;
    let mut buf = Vec::new();
    f.read_to_end(&mut buf)
        .await
        .map_err(|e| RustlsError::General(e.to_string()))?;
    Ok(buf)
}

fn into_certificate(buf: &[u8]) -> Result<CertificateDer<'static>, RustlsError> {
    let mut cursor = Cursor::new(buf);
    rustls_pki_types::CertificateDer::pem_reader_iter(&mut cursor)
        .next()
        .ok_or(RustlsError::General(
            "Could not read certificate".to_string(),
        ))?
        .map_err(|e| RustlsError::General(e.to_string()))
}

fn into_certificates(buf: &[u8]) -> Result<Vec<CertificateDer<'static>>, RustlsError> {
    let mut cursor = Cursor::new(buf);
    let mut certs = Vec::new();
    for cert_result in rustls_pki_types::CertificateDer::pem_reader_iter(&mut cursor) {
        certs.push(cert_result.map_err(|e| RustlsError::General(e.to_string()))?);
    }
    if certs.is_empty() {
        return Err(RustlsError::General(
            "Could not read any certificates".to_string(),
        ));
    }
    Ok(certs)
}

fn into_root_store(
    ca_cert: Option<CertificateDer<'static>>,
    load_native: bool,
) -> Result<RootCertStore, RustlsError> {
    let mut roots = RootCertStore::empty();
    if load_native {
        for cert in rustls_native_certs::load_native_certs().certs {
            roots.add(cert)?;
        }
    }
    if let Some(ca) = ca_cert {
        roots.add_parsable_certificates(vec![ca]);
    }
    Ok(roots)
}

fn into_private_key(buf: &[u8]) -> Result<PrivateKeyDer<'static>, RustlsError> {
    let mut cursor = Cursor::new(buf);

    macro_rules! parse_key {
        ($parser:ident, $key_type:expr) => {
            cursor.set_position(0);

            let keys: Result<Vec<_>, RustlsError> = $parser(&mut cursor)
                .map(|key| {
                    key.map_err(|_| {
                        RustlsError::General("Could not load any private keys".to_string())
                    })
                })
                .collect();
            match keys {
                // try the next parser
                Err(_) => (),
                // try the next parser
                Ok(keys) if keys.is_empty() => (),
                Ok(mut keys) if keys.len() == 1 => {
                    return Ok($key_type(keys.pop().unwrap()));
                }
                Ok(keys) => {
                    return Err(RustlsError::General(format!(
                        "Unexpected number of keys: {} (only 1 supported)",
                        keys.len()
                    )));
                }
            }
        };
    }

    // attempt to parse PKCS8 encoded key. Returns early if a key is found
    parse_key!(pkcs8_private_keys, PrivateKeyDer::Pkcs8);
    // attempt to parse RSA key. Returns early if a key is found
    parse_key!(rsa_private_keys, PrivateKeyDer::Pkcs1);
    // attempt to parse a SEC1-encoded EC key. Returns early if a key is found
    parse_key!(ec_private_keys, PrivateKeyDer::Sec1);

    Err(RustlsError::General(
        "could not load any valid private keys".to_string(),
    ))
}

// parser wrapper for pkcs #8 encoded private keys
fn pkcs8_private_keys<R: std::io::Read>(
    reader: &mut R,
) -> impl Iterator<
    Item = Result<rustls_pki_types::PrivatePkcs8KeyDer<'static>, rustls_pki_types::pem::Error>,
> + '_ {
    rustls_pki_types::PrivatePkcs8KeyDer::pem_reader_iter(reader)
        .map(|result| result.map(|key| key.clone_key()))
}

// parser wrapper for pkcs #1 encoded private keys
fn rsa_private_keys<R: std::io::Read>(
    reader: &mut R,
) -> impl Iterator<
    Item = Result<rustls_pki_types::PrivatePkcs1KeyDer<'static>, rustls_pki_types::pem::Error>,
> + '_ {
    rustls_pki_types::PrivatePkcs1KeyDer::pem_reader_iter(reader)
        .map(|result| result.map(|key| key.clone_key()))
}

// parser wrapper for sec1 encoded private keys
fn ec_private_keys<R: std::io::Read>(
    reader: &mut R,
) -> impl Iterator<
    Item = Result<rustls_pki_types::PrivateSec1KeyDer<'static>, rustls_pki_types::pem::Error>,
> + '_ {
    rustls_pki_types::PrivateSec1KeyDer::pem_reader_iter(reader)
        .map(|result| result.map(|key| key.clone_key()))
}

/// TLS configuration
#[derive(Debug, Clone, Deserialize, Default, Builder)]
pub struct TlsConfig {
    ca_cert: Option<String>,
    cert: Option<String>,
    key: Option<String>,
    #[serde(default)]
    mutual: bool,
}

/// Create TLS Provider from configuration
pub(crate) async fn new_tls(c: &TlsConfig, is_server: bool) -> Result<TlsProvider> {
    let mut ca_cert = DEV_CA_CERT.to_vec();
    if let Some(c) = &c.ca_cert {
        ca_cert = read_file(c).await?;
    };

    let (cert, key) = if is_server {
        if let (Some(cert), Some(key)) = (&c.cert, &c.key) {
            (Some(read_file(cert).await?), Some(read_file(key).await?))
        } else {
            warn!("using dev certs, please use your own certs for production");

            (
                Some(DEV_SERVER_CERT.to_vec()),
                Some(DEV_SERVER_KEY.to_vec()),
            )
        }
    } else {
        if c.mutual {
            if let (Some(cert), Some(key)) = (&c.cert, &c.key) {
                (Some(read_file(cert).await?), Some(read_file(key).await?))
            } else {
                bail!("client cert and private key are required for mutual TLS");
            }
        } else {
            (None, None)
        }
    };

    Ok(TlsProvider::new(Some(ca_cert), cert, key, c.mutual)?)
}
