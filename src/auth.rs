use anyhow::{Result, bail};

/// Auth trait for validating tool handshake requests.
#[async_trait::async_trait]
pub trait Auth: Send + Sync {
    async fn authenticate(&self, credential: &str) -> Result<String>;
}

/// Default auth implementation based on optional shared token.
pub struct AuthImpl {
    auth_token: Option<String>,
}

impl AuthImpl {
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
