//! Credentials and the shared HTTP client.
//!
//! `Credential` attaches authentication to upstream requests; `shared_http_client`
//! provides unified timeouts, connection pooling and User-Agent so providers
//! share a configured client rather than constructing one per provider.

use std::future::Future;
use std::io;
use std::pin::Pin;
use std::sync::{Arc, RwLock};
use std::task::{Context, Poll};
use std::time::{Duration, Instant};

use async_trait::async_trait;
use axum::http::{HeaderMap, Uri, header};
use hyper_util::client::legacy::Client;
use hyper_util::client::proxy::matcher::{Intercept, Matcher};
use hyper_util::rt::{TokioExecutor, TokioIo};
use secrecy::ExposeSecret;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream;
use tokio::sync::Mutex;
use tower::Service;
use yup_oauth2::CustomHyperClientBuilder;
use yup_oauth2::hyper_rustls::{HttpsConnector, HttpsConnectorBuilder};

use crate::provider::ProviderError;
use crate::serve_config::AuthScheme;

/// Remove credential headers from application-supplied transport headers.
/// Provider authentication is attached afterwards and must always win.
pub fn sanitized_transport_headers(headers: &HeaderMap) -> HeaderMap {
    let mut headers = headers.clone();
    headers.remove(header::AUTHORIZATION);
    headers.remove("api-key");
    headers
}

/// Shared HTTP client: unified timeouts, connection pool, User-Agent.
pub fn shared_http_client() -> reqwest::Client {
    static CLIENT: std::sync::OnceLock<reqwest::Client> = std::sync::OnceLock::new();
    CLIENT
        .get_or_init(|| build_shared_http_client(false))
        .clone()
}

/// Shared HTTP/1.1-only client for upstreams that require explicit protocol pinning.
pub fn shared_http1_client() -> reqwest::Client {
    static CLIENT: std::sync::OnceLock<reqwest::Client> = std::sync::OnceLock::new();
    CLIENT
        .get_or_init(|| build_shared_http_client(true))
        .clone()
}

fn build_shared_http_client(http1_only: bool) -> reqwest::Client {
    let builder = reqwest::Client::builder()
        .connect_timeout(Duration::from_secs(10))
        .timeout(Duration::from_secs(DEFAULT_REQUEST_TIMEOUT_SECS))
        .pool_idle_timeout(Duration::from_secs(90))
        .user_agent(concat!("yomo-zipper/", env!("CARGO_PKG_VERSION")));
    let builder = match system_proxy() {
        Some(proxy) => builder.proxy(proxy),
        None => builder,
    };
    let builder = if http1_only {
        builder.http1_only()
    } else {
        builder
    };
    builder.build().expect("shared http client must build")
}

/// Proxy for the shared HTTP client, taken from the system settings.
///
/// The environment is consulted first and the OS configuration second, matching
/// `reqwest`'s default system behaviour. `reqwest`'s own system matcher is not
/// used because it would also proxy loopback destinations, which breaks local
/// upstreams and test servers.
fn system_proxy() -> Option<reqwest::Proxy> {
    let matcher = Arc::new(Matcher::from_system());
    let probes = [
        Uri::from_static("http://proxy-check.invalid/"),
        Uri::from_static("https://proxy-check.invalid/"),
    ];
    let configured = probes.iter().find_map(|probe| matcher.intercept(probe))?;
    let mut proxy = reqwest::Proxy::custom(move |url| {
        let uri = url.as_str().parse::<Uri>().ok()?;
        proxy_target(&matcher, &uri)
    });
    if let Some((username, password)) = proxy_credentials(&configured) {
        proxy = proxy.basic_auth(&username, &password);
    }
    Some(proxy)
}

/// Resolve the proxy URL for one destination; loopback always stays direct.
fn proxy_target(matcher: &Matcher, uri: &Uri) -> Option<reqwest::Url> {
    let host = uri.host()?;
    if is_loopback_host(host) {
        return None;
    }
    let proxy = matcher.intercept(uri)?;
    reqwest::Url::parse(&proxy.uri().to_string()).ok()
}

/// Whether a destination host must never go through a proxy.
fn is_loopback_host(host: &str) -> bool {
    let host = host.trim_start_matches('[').trim_end_matches(']');
    host.eq_ignore_ascii_case("localhost")
        || host
            .parse::<std::net::IpAddr>()
            .is_ok_and(|ip| ip.is_loopback())
}

/// Credentials a configured proxy requires, if any.
fn proxy_credentials(proxy: &Intercept) -> Option<(String, String)> {
    if let Some(value) = proxy.basic_auth() {
        use base64::Engine;
        let encoded = value.to_str().ok()?.strip_prefix("Basic ")?.to_owned();
        let decoded = base64::engine::general_purpose::STANDARD
            .decode(encoded)
            .ok()?;
        let credentials = String::from_utf8(decoded).ok()?;
        let (username, password) = credentials.split_once(':')?;
        return Some((username.to_owned(), password.to_owned()));
    }
    proxy
        .raw_auth()
        .map(|(username, password)| (username.to_owned(), password.to_owned()))
}

pub const DEFAULT_REQUEST_TIMEOUT_SECS: u64 = 600;

/// Largest proxy `CONNECT` response head accepted before giving up.
const MAX_PROXY_RESPONSE_HEAD_BYTES: usize = 8 * 1024;

/// Connector used by the OAuth2 token client: TLS over the system proxy tunnel.
pub type OAuth2TokenConnector = HttpsConnector<ProxyTunnel>;

/// Build the `hyper` client builder that `yup-oauth2` uses to fetch tokens.
///
/// `yup-oauth2` creates its own `hyper` client and therefore ignores the proxy
/// settings that `reqwest` honours, so the token request needs its own
/// proxy-aware transport.
pub fn oauth2_token_hyper_client() -> CustomHyperClientBuilder<OAuth2TokenConnector> {
    let connector = HttpsConnectorBuilder::new()
        .with_provider_and_native_roots(rustls::crypto::ring::default_provider())
        .expect("native roots must load for the OAuth2 token client")
        .https_or_http()
        .enable_http1()
        .enable_http2()
        .wrap_connector(ProxyTunnel::system());
    let client = Client::builder(TokioExecutor::new())
        .pool_max_idle_per_host(0)
        .build::<_, String>(connector);
    CustomHyperClientBuilder::from(client)
}

/// Connector that tunnels connections through the detected HTTP proxy.
///
/// The proxy is resolved from the environment first and then from the OS
/// settings, matching `reqwest`'s system proxy behaviour. Without a proxy the
/// connector dials the destination directly, so behaviour is unchanged.
#[derive(Clone, Debug)]
pub struct ProxyTunnel {
    matcher: Arc<Matcher>,
}

impl ProxyTunnel {
    /// Use the process proxy settings (environment, then OS configuration).
    pub fn system() -> Self {
        Self::with_matcher(Matcher::from_system())
    }

    fn with_matcher(matcher: Matcher) -> Self {
        Self {
            matcher: Arc::new(matcher),
        }
    }
}

impl Service<Uri> for ProxyTunnel {
    type Response = TokioIo<TcpStream>;
    type Error = io::Error;
    type Future = Pin<Box<dyn Future<Output = Result<Self::Response, Self::Error>> + Send>>;

    fn poll_ready(&mut self, _cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        Poll::Ready(Ok(()))
    }

    fn call(&mut self, destination: Uri) -> Self::Future {
        let matcher = Arc::clone(&self.matcher);
        Box::pin(async move {
            match matcher.intercept(&destination) {
                Some(proxy) => {
                    let target = connect_authority(&destination)?;
                    Ok(TokioIo::new(tunnel_connect(&proxy, &target).await?))
                }
                None => Ok(TokioIo::new(direct_connect(&destination).await?)),
            }
        })
    }
}

/// Default port of a destination URI.
fn destination_port(uri: &Uri) -> Option<u16> {
    uri.port_u16().or_else(|| match uri.scheme_str() {
        Some("https") => Some(443),
        Some("http") => Some(80),
        _ => None,
    })
}

/// `host:port` sent in the `CONNECT` request; IPv6 hosts stay bracketed.
fn connect_authority(uri: &Uri) -> io::Result<String> {
    let host = uri
        .host()
        .ok_or_else(|| io::Error::other("destination has no host"))?;
    let port = destination_port(uri)
        .ok_or_else(|| io::Error::other(format!("destination {host} has no port")))?;
    if host.contains(':') {
        Ok(format!("[{host}]:{port}"))
    } else {
        Ok(format!("{host}:{port}"))
    }
}

/// Connect to the destination itself, bypassing the proxy.
async fn direct_connect(uri: &Uri) -> io::Result<TcpStream> {
    let host = uri
        .host()
        .ok_or_else(|| io::Error::other("destination has no host"))?;
    let port = destination_port(uri)
        .ok_or_else(|| io::Error::other(format!("destination {host} has no port")))?;
    let mut addresses = tokio::net::lookup_host((host, port)).await?;
    let address = addresses
        .next()
        .ok_or_else(|| io::Error::other(format!("no address for {host}:{port}")))?;
    TcpStream::connect(address).await
}

/// Open a TCP tunnel to `target` through the proxy.
async fn tunnel_connect(proxy: &Intercept, target: &str) -> io::Result<TcpStream> {
    let mut stream = direct_connect(proxy.uri()).await?;
    let mut request = format!("CONNECT {target} HTTP/1.1\r\nHost: {target}\r\n");
    if let Some(authorization) = proxy_authorization(proxy)? {
        request.push_str(&format!("Proxy-Authorization: {authorization}\r\n"));
    }
    request.push_str("\r\n");
    stream.write_all(request.as_bytes()).await?;
    stream.flush().await?;
    let status = read_connect_status(&mut stream).await?;
    if status != 200 {
        return Err(io::Error::other(format!(
            "proxy CONNECT failed with status {status}"
        )));
    }
    Ok(stream)
}

/// `Proxy-Authorization` value for a proxy that requires credentials.
fn proxy_authorization(proxy: &Intercept) -> io::Result<Option<String>> {
    if let Some(value) = proxy.basic_auth() {
        return value
            .to_str()
            .map(str::to_owned)
            .map(Some)
            .map_err(|error| {
                io::Error::other(format!("invalid proxy authorization header: {error}"))
            });
    }
    Ok(proxy.raw_auth().map(|(user, password)| {
        use base64::Engine;
        let credentials =
            base64::engine::general_purpose::STANDARD.encode(format!("{user}:{password}"));
        format!("Basic {credentials}")
    }))
}

/// Read and parse the proxy response status line for a `CONNECT` request.
async fn read_connect_status(stream: &mut TcpStream) -> io::Result<u16> {
    let mut head = Vec::new();
    let mut chunk = [0u8; 512];
    while !head.windows(4).any(|window| window == b"\r\n\r\n") {
        if head.len() > MAX_PROXY_RESPONSE_HEAD_BYTES {
            return Err(io::Error::other("proxy CONNECT response head too large"));
        }
        let read = stream.read(&mut chunk).await?;
        if read == 0 {
            return Err(io::Error::other(
                "proxy closed the connection before the CONNECT response",
            ));
        }
        head.extend_from_slice(&chunk[..read]);
    }
    let text = String::from_utf8_lossy(&head);
    let status_line = text.lines().next().unwrap_or_default();
    status_line
        .split_whitespace()
        .nth(1)
        .and_then(|code| code.parse().ok())
        .ok_or_else(|| io::Error::other(format!("malformed proxy CONNECT response: {status_line}")))
}

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

    #[test]
    fn sanitized_transport_headers_drop_credentials_and_keep_app_headers() {
        let mut headers = HeaderMap::new();
        headers.insert(header::AUTHORIZATION, "Bearer client-key".parse().unwrap());
        headers.insert("api-key", "client-key".parse().unwrap());
        headers.insert("x-client-header", "client-value".parse().unwrap());

        let sanitized = sanitized_transport_headers(&headers);

        assert!(sanitized.get(header::AUTHORIZATION).is_none());
        assert!(sanitized.get("api-key").is_none());
        assert_eq!(sanitized["x-client-header"], "client-value");
    }

    async fn accept_proxy_connection(listener: tokio::net::TcpListener) -> String {
        let (mut socket, _) = listener.accept().await.expect("accept proxy connection");
        let mut head = Vec::new();
        let mut chunk = [0u8; 512];
        while !head.windows(4).any(|window| window == b"\r\n\r\n") {
            let read = socket.read(&mut chunk).await.expect("read CONNECT request");
            if read == 0 {
                break;
            }
            head.extend_from_slice(&chunk[..read]);
        }
        socket
            .write_all(b"HTTP/1.1 200 Connection Established\r\n\r\n")
            .await
            .expect("write CONNECT response");
        String::from_utf8_lossy(&head).to_string()
    }

    #[tokio::test]
    async fn proxy_tunnel_sends_connect_with_proxy_authorization() {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .expect("bind proxy");
        let address = listener.local_addr().expect("proxy address");
        let proxy = tokio::spawn(accept_proxy_connection(listener));
        let matcher = Matcher::builder()
            .https(format!("http://Aladdin:opensesame@{address}"))
            .build();
        let mut tunnel = ProxyTunnel::with_matcher(matcher);

        let _stream = tunnel
            .call(Uri::from_static("https://oauth2.googleapis.com/token"))
            .await
            .expect("tunnel through the proxy");

        let head = proxy.await.expect("proxy task");
        assert!(
            head.starts_with("CONNECT oauth2.googleapis.com:443 HTTP/1.1\r\n"),
            "{head}"
        );
        assert!(
            head.contains("\r\nHost: oauth2.googleapis.com:443\r\n"),
            "{head}"
        );
        assert!(
            head.contains("\r\nProxy-Authorization: Basic QWxhZGRpbjpvcGVuc2VzYW1l\r\n"),
            "{head}"
        );
    }

    #[tokio::test]
    async fn proxy_tunnel_reports_non_200_connect_status() {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .expect("bind proxy");
        let address = listener.local_addr().expect("proxy address");
        tokio::spawn(async move {
            let (mut socket, _) = listener.accept().await.expect("accept proxy connection");
            let mut buffer = [0u8; 512];
            socket
                .read(&mut buffer)
                .await
                .expect("read CONNECT request");
            socket
                .write_all(b"HTTP/1.1 407 Proxy Authentication Required\r\n\r\n")
                .await
                .expect("write CONNECT response");
        });
        let matcher = Matcher::builder()
            .https(format!("http://{address}"))
            .build();
        let mut tunnel = ProxyTunnel::with_matcher(matcher);

        let error = tunnel
            .call(Uri::from_static("https://oauth2.googleapis.com/token"))
            .await
            .expect_err("407 must fail the tunnel");

        assert!(error.to_string().contains("407"), "{error}");
    }

    #[tokio::test]
    async fn proxy_tunnel_connects_directly_without_proxy() {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .expect("bind destination");
        let address = listener.local_addr().expect("destination address");
        let accepted = tokio::spawn(async move { listener.accept().await.expect("accept direct") });
        let mut tunnel = ProxyTunnel::with_matcher(Matcher::builder().build());
        let uri: Uri = format!("http://{address}/token")
            .parse()
            .expect("destination uri");

        let stream = tunnel.call(uri).await.expect("direct connection");

        drop(stream);
        accepted.await.expect("accept task");
    }

    #[test]
    fn proxy_target_keeps_loopback_and_unconfigured_destinations_direct() {
        let matcher = Matcher::builder().https("http://127.0.0.1:7890").build();

        assert!(
            proxy_target(
                &matcher,
                &Uri::from_static("https://oauth2.googleapis.com/token")
            )
            .is_some()
        );
        assert!(proxy_target(&matcher, &Uri::from_static("http://127.0.0.1:8080/v1")).is_none());
        assert!(proxy_target(&matcher, &Uri::from_static("http://localhost:8080/v1")).is_none());
        assert!(proxy_target(&matcher, &Uri::from_static("http://[::1]:8080/v1")).is_none());

        let unconfigured = Matcher::builder().build();
        assert!(proxy_target(&unconfigured, &Uri::from_static("https://example.com/")).is_none());
    }

    #[test]
    fn loopback_hosts_are_recognised() {
        for host in [
            "localhost",
            "LOCALHOST",
            "127.0.0.1",
            "127.9.8.7",
            "::1",
            "[::1]",
        ] {
            assert!(is_loopback_host(host), "{host}");
        }
        for host in ["example.com", "10.0.0.1", "google.com"] {
            assert!(!is_loopback_host(host), "{host}");
        }
    }

    #[test]
    fn proxy_matcher_selects_https_and_honours_no_proxy() {
        let matcher = Matcher::builder()
            .https("http://127.0.0.1:7890")
            .no("localhost")
            .build();

        assert!(
            matcher
                .intercept(&Uri::from_static("https://oauth2.googleapis.com/token"))
                .is_some()
        );
        assert!(
            matcher
                .intercept(&Uri::from_static("https://localhost/token"))
                .is_none()
        );
        assert!(
            matcher
                .intercept(&Uri::from_static("http://oauth2.googleapis.com/token"))
                .is_none()
        );
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
