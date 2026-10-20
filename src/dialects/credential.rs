//! Credentials and the shared HTTP client.
//!
//! `Credential` attaches authentication to upstream requests; `shared_http_client`
//! provides unified timeouts, connection pooling and User-Agent so providers
//! share a configured client rather than constructing one per provider.

use std::sync::{Arc, RwLock};
use std::time::{Duration, Instant};

use async_trait::async_trait;
use secrecy::ExposeSecret;
use tokio::sync::Mutex;

use crate::provider::ProviderError;
use crate::serve_config::AuthScheme;

/// Shared HTTP client: unified timeouts, connection pool, User-Agent.
pub fn shared_http_client() -> reqwest::Client {
    static CLIENT: std::sync::OnceLock<reqwest::Client> = std::sync::OnceLock::new();
    CLIENT
        .get_or_init(|| {
            reqwest::Client::builder()
                .connect_timeout(Duration::from_secs(10))
                .timeout(Duration::from_secs(DEFAULT_REQUEST_TIMEOUT_SECS))
                .pool_idle_timeout(Duration::from_secs(90))
                .user_agent(concat!("yomo-zipper/", env!("CARGO_PKG_VERSION")))
                .build()
                .expect("shared http client must build")
        })
        .clone()
}

pub const DEFAULT_REQUEST_TIMEOUT_SECS: u64 = 600;

/// Refresh margin: how long before expiry a GCP token is refreshed.
const TOKEN_REFRESH_MARGIN: Duration = Duration::from_secs(5 * 60);

/// Error raised while attaching authentication.
#[derive(Debug)]
pub enum CredentialError {
    NotImplemented(&'static str),
    TokenFetch(String),
}

impl std::fmt::Display for CredentialError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            CredentialError::NotImplemented(what) => write!(f, "not implemented: {what}"),
            CredentialError::TokenFetch(message) => write!(f, "token fetch failed: {message}"),
        }
    }
}

impl std::error::Error for CredentialError {}

impl From<CredentialError> for ProviderError {
    fn from(err: CredentialError) -> Self {
        match err {
            CredentialError::NotImplemented(what) => {
                ProviderError::internal(format!("credential not implemented: {what}"))
            }
            CredentialError::TokenFetch(message) => {
                ProviderError::internal(format!("credential token fetch failed: {message}"))
            }
        }
    }
}

/// Static header credential: bearer or api-key.
#[derive(Debug, Clone)]
pub struct StaticHeaderCredential {
    pub scheme: AuthScheme,
    pub api_key: secrecy::SecretString,
}

impl StaticHeaderCredential {
    /// Attach auth to the request: bearer -> `Authorization: Bearer`, api-key -> `api-key` header.
    pub fn apply(&self, builder: reqwest::RequestBuilder) -> reqwest::RequestBuilder {
        match self.scheme {
            AuthScheme::Bearer => builder.bearer_auth(self.api_key.expose_secret()),
            AuthScheme::ApiKey => builder.header("api-key", self.api_key.expose_secret()),
        }
    }
}

/// SigV4 credential placeholder; signing is not implemented.
#[derive(Debug, Clone, Default)]
pub struct SigV4Credential;

/// OAuth2 token credential (service account -> access token, cached + refreshed early).
#[derive(Clone)]
pub struct Oauth2TokenCredential {
    cache: Arc<GcpTokenCache>,
}

impl Oauth2TokenCredential {
    pub fn new(cache: Arc<GcpTokenCache>) -> Self {
        Self { cache }
    }

    pub async fn apply(&self, builder: reqwest::RequestBuilder) -> reqwest::RequestBuilder {
        match self.cache.token().await {
            Ok(token) => builder.bearer_auth(token),
            // A missing auth header is left for the upstream 401 path; never swallow the request because of a credential issue.
            Err(_) => builder,
        }
    }
}

/// Credential that attaches authentication to a request.
#[derive(Clone)]
pub enum Credential {
    StaticHeader(StaticHeaderCredential),
    /// SigV4 is a placeholder; `apply` returns an unimplemented error.
    SigV4(SigV4Credential),
    Oauth2Token(Oauth2TokenCredential),
}

impl Credential {
    pub async fn apply(
        &self,
        builder: reqwest::RequestBuilder,
    ) -> Result<reqwest::RequestBuilder, CredentialError> {
        match self {
            Credential::StaticHeader(credential) => Ok(credential.apply(builder)),
            Credential::SigV4(_) => Err(CredentialError::NotImplemented("SigV4 credential")),
            Credential::Oauth2Token(credential) => Ok(credential.apply(builder).await),
        }
    }
}

/// Token fetcher: implements the concrete OAuth2 flow (e.g. yup-oauth2 service account).
#[async_trait]
pub trait TokenFetcher: Send + Sync {
    /// Returns (token, validity duration).
    async fn fetch(&self) -> Result<(String, Duration), CredentialError>;
}

/// GCP token cache: refresh 5 minutes before expiry; concurrent callers trigger a single refresh.
pub struct GcpTokenCache {
    state: RwLock<Option<(String, Instant)>>,
    refresh_lock: Mutex<()>,
    fetcher: Arc<dyn TokenFetcher>,
}

impl GcpTokenCache {
    pub fn new(fetcher: Arc<dyn TokenFetcher>) -> Self {
        Self {
            state: RwLock::new(None),
            refresh_lock: Mutex::new(()),
            fetcher,
        }
    }

    /// Return a valid token; refresh when expired or close to expiry.
    pub async fn token(&self) -> Result<String, CredentialError> {
        if let Some(token) = self.valid_cached_token(Instant::now()) {
            return Ok(token);
        }

        // Single-flight: concurrent refreshes trigger exactly one fetch.
        let _guard = self.refresh_lock.lock().await;
        if let Some(token) = self.valid_cached_token(Instant::now()) {
            return Ok(token);
        }

        let (token, ttl) = self.fetcher.fetch().await?;
        let expires_at = Instant::now() + ttl;
        *self
            .state
            .write()
            .map_err(|_| CredentialError::TokenFetch("token cache poisoned".to_string()))? =
            Some((token.clone(), expires_at));
        Ok(token)
    }

    /// Pure function: whether the cached token is still valid (including the refresh margin).
    fn valid_cached_token(&self, now: Instant) -> Option<String> {
        let state = self.state.read().ok()?;
        let (token, expires_at) = state.as_ref()?;
        if *expires_at > now + TOKEN_REFRESH_MARGIN {
            Some(token.clone())
        } else {
            None
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicUsize, Ordering};

    struct CountingFetcher {
        calls: AtomicUsize,
        ttl: Duration,
    }

    #[async_trait]
    impl TokenFetcher for CountingFetcher {
        async fn fetch(&self) -> Result<(String, Duration), CredentialError> {
            let call = self.calls.fetch_add(1, Ordering::SeqCst);
            Ok((format!("token-{call}"), self.ttl))
        }
    }

    #[test]
    fn static_header_applies_bearer_auth() {
        let credential = StaticHeaderCredential {
            scheme: AuthScheme::Bearer,
            api_key: secrecy::SecretString::from("sk-test"),
        };
        let request = credential
            .apply(reqwest::Client::new().get("https://example.com"))
            .build()
            .expect("build request");
        assert_eq!(
            request.headers().get("authorization").unwrap(),
            "Bearer sk-test"
        );
        assert!(request.headers().get("api-key").is_none());
    }

    #[test]
    fn static_header_applies_api_key_header() {
        let credential = StaticHeaderCredential {
            scheme: AuthScheme::ApiKey,
            api_key: secrecy::SecretString::from("azure-key"),
        };
        let request = credential
            .apply(reqwest::Client::new().get("https://example.com"))
            .build()
            .expect("build request");
        assert_eq!(request.headers().get("api-key").unwrap(), "azure-key");
        assert!(request.headers().get("authorization").is_none());
    }

    #[tokio::test]
    async fn sigv4_apply_is_not_implemented() {
        let credential = Credential::SigV4(SigV4Credential);
        let err = credential
            .apply(reqwest::Client::new().get("https://example.com"))
            .await
            .expect_err("SigV4 must be unimplemented");
        assert!(matches!(
            err,
            CredentialError::NotImplemented("SigV4 credential")
        ));
    }

    #[tokio::test]
    async fn token_cache_reuses_valid_token_and_refreshes_once() {
        let fetcher = Arc::new(CountingFetcher {
            calls: AtomicUsize::new(0),
            ttl: Duration::from_secs(3600),
        });
        let cache = GcpTokenCache::new(fetcher.clone());

        let first = cache.token().await.expect("first token");
        let second = cache.token().await.expect("second token");
        assert_eq!(first, "token-0");
        assert_eq!(second, "token-0");
        assert_eq!(fetcher.calls.load(Ordering::SeqCst), 1);
    }

    #[tokio::test]
    async fn token_cache_refreshes_near_expiry() {
        let fetcher = Arc::new(CountingFetcher {
            calls: AtomicUsize::new(0),
            // ttl shorter than the refresh margin -> treated as near-expiry immediately
            ttl: Duration::from_secs(60),
        });
        let cache = GcpTokenCache::new(fetcher.clone());

        let first = cache.token().await.expect("first token");
        let second = cache.token().await.expect("second token");
        assert_eq!(first, "token-0");
        assert_eq!(second, "token-1");
        assert_eq!(fetcher.calls.load(Ordering::SeqCst), 2);
    }

    #[tokio::test]
    async fn concurrent_token_requests_trigger_single_fetch() {
        let fetcher = Arc::new(CountingFetcher {
            calls: AtomicUsize::new(0),
            ttl: Duration::from_secs(3600),
        });
        let cache = Arc::new(GcpTokenCache::new(fetcher.clone()));

        let mut handles = Vec::new();
        for _ in 0..8 {
            let cache = cache.clone();
            handles.push(tokio::spawn(
                async move { cache.token().await.expect("token") },
            ));
        }
        for handle in handles {
            handle.await.expect("join");
        }
        assert_eq!(fetcher.calls.load(Ordering::SeqCst), 1);
    }
}
