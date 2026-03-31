use anyhow::{Result, bail};

/// Auth trait for validating tool handshake requests.
#[async_trait::async_trait]
pub trait Auth: Send + Sync {
    /// Validates a credential string from a handshake request.
    ///
    /// Returns an `auth_info` string that can be consumed by `MetadataMgr`
    /// to derive route/tool selection metadata.
    async fn authenticate(&self, credential: &str) -> Result<String>;
}

/// Default auth implementation based on optional shared token.
pub struct AuthImpl {
    auth_token: Option<String>,
}

impl AuthImpl {
    /// Creates the default token-based authenticator.
    ///
    /// When `auth_token` is `None`, authentication is effectively disabled.
    pub fn new(auth_token: Option<String>) -> Self {
        Self { auth_token }
    }
}

#[async_trait::async_trait]
impl Auth for AuthImpl {
    async fn authenticate(&self, credential: &str) -> Result<String> {
        if let Some(token) = &self.auth_token {
            if &credential != token {
                bail!("invalid credential");
            }
        }

        Ok(String::new())
    }
}
