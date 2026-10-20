//! `google-vertex` dialect: Vertex AI Gemini chat text models.
//!
//! SA JSON -> `Credential::Oauth2Token` (`GcpTokenCache` caching + early refresh);
//! URLs are built from project/location; generateContent image traffic is handled
//! by passthrough. Gemini requests and responses are translated to and from the shared IR.

pub mod mapper;
pub mod types;

use std::pin::Pin;
use std::sync::Arc;

use async_trait::async_trait;
use futures_core::Stream;
use futures_util::StreamExt;
use std::sync::Mutex;

use self::mapper::{
    ThoughtSignatureStore, VertexStreamState, build_unified_vertex_request,
    map_http_error_with_headers, map_vertex_response, map_vertex_stream_chunk,
};
use self::types::VertexGenerateContentResponse;
use crate::dialects::credential::{
    CredentialError, GcpTokenCache, TokenFetcher, shared_http_client,
};
use crate::provider::UpstreamLog;
use crate::provider::{Provider, ProviderError, UnifiedEvent, UnifiedResponse};
use crate::provider::{UnifiedEventKind, UnifiedRequest};
use crate::serve_config::GoogleVertexParams;

const CLOUD_PLATFORM_SCOPE: &str = "https://www.googleapis.com/auth/cloud-platform";

/// `google-vertex` dialect client.
#[derive(Clone)]
pub struct GoogleVertexProvider {
    /// Upstream model name (params.model).
    model: String,
    project_id: String,
    location: String,
    token_cache: Arc<GcpTokenCache>,
    thought_signatures: Arc<Mutex<ThoughtSignatureStore>>,
}

impl GoogleVertexProvider {
    pub fn new(params: &GoogleVertexParams) -> Self {
        let fetcher = YupOauth2TokenFetcher::new(params.credentials_file.clone());
        Self {
            model: params.model.clone(),
            project_id: params.project_id.clone(),
            location: params.location.clone(),
            token_cache: Arc::new(GcpTokenCache::new(Arc::new(fetcher))),
            thought_signatures: Arc::new(Mutex::new(ThoughtSignatureStore::new(
                MAX_THOUGHT_SIGNATURES,
            ))),
        }
    }

    async fn access_token(&self) -> Result<String, ProviderError> {
        self.token_cache
            .token()
            .await
            .map_err(|err: CredentialError| ProviderError::from(err))
    }

    /// Build a POST request builder with auth and content-type attached.
    async fn post_builder(&self, url: String) -> Result<reqwest::RequestBuilder, ProviderError> {
        let token = self.access_token().await?;
        Ok(shared_http_client()
            .post(url)
            .bearer_auth(token)
            .header("content-type", "application/json"))
    }
}

/// MAX_THOUGHT_SIGNATURES matches the previous implementation.
const MAX_THOUGHT_SIGNATURES: usize = 4096;

/// yup-oauth2 service account token fetcher (`TokenFetcher` implementation).
pub struct YupOauth2TokenFetcher {
    credentials_file: std::path::PathBuf,
}

impl YupOauth2TokenFetcher {
    pub fn new(credentials_file: std::path::PathBuf) -> Self {
        Self { credentials_file }
    }
}

#[async_trait]
impl TokenFetcher for YupOauth2TokenFetcher {
    async fn fetch(&self) -> Result<(String, std::time::Duration), CredentialError> {
        use yup_oauth2::authenticator::DefaultAuthenticator;
        use yup_oauth2::{ServiceAccountAuthenticator, read_service_account_key};

        let service_account_key = read_service_account_key(&self.credentials_file)
            .await
            .map_err(|err| CredentialError::TokenFetch(format!("read SA key: {err}")))?;
        let authenticator: DefaultAuthenticator =
            ServiceAccountAuthenticator::builder(service_account_key)
                .build()
                .await
                .map_err(|err| {
                    CredentialError::TokenFetch(format!("build authenticator: {err}"))
                })?;
        let token = authenticator
            .token(&[CLOUD_PLATFORM_SCOPE])
            .await
            .map_err(|err| CredentialError::TokenFetch(format!("{err}")))?;
        let token_value = token.token().map(ToString::to_string).ok_or_else(|| {
            CredentialError::TokenFetch("missing google access token".to_string())
        })?;
        // GCP access tokens are valid for one hour; GcpTokenCache refreshes 5 minutes early.
        Ok((token_value, std::time::Duration::from_secs(3600)))
    }
}

/// Build a generateContent URL (pure function).
pub(crate) fn generate_content_url(
    location: &str,
    project_id: &str,
    model: &str,
    stream: bool,
) -> String {
    let action = if stream {
        "streamGenerateContent"
    } else {
        "generateContent"
    };
    let base = if location == "global" {
        format!(
            "https://aiplatform.googleapis.com/v1/projects/{project_id}/locations/{location}/publishers/google/models/{model}:{action}"
        )
    } else {
        format!(
            "https://{location}-aiplatform.googleapis.com/v1/projects/{project_id}/locations/{location}/publishers/google/models/{model}:{action}"
        )
    };
    if stream {
        format!("{base}?alt=sse")
    } else {
        base
    }
}

#[async_trait]
impl<M> Provider<M> for GoogleVertexProvider {
    fn model_id(&self) -> &str {
        &self.model
    }

    async fn complete(
        &self,
        request: UnifiedRequest,
        _metadata: &M,
    ) -> Result<UnifiedResponse, ProviderError> {
        let body =
            build_unified_vertex_request(&request, &shared_http_client(), &self.thought_signatures)
                .await?;
        let url = generate_content_url(&self.location, &self.project_id, &self.model, false);
        let diagnostics = UpstreamLog::new("google-vertex", &self.model);
        diagnostics.json("upstream.request", Some(&url), &body);
        let response = self
            .post_builder(url)
            .await?
            .body(
                serde_json::to_vec(&body)
                    .map_err(|err| ProviderError::internal(err.to_string()))?,
            )
            .send()
            .await
            .map_err(|err| ProviderError::internal(err.to_string()))?;

        let status = response.status();
        diagnostics.response_start(status.as_u16(), response.headers());
        let headers = response.headers().clone();
        let bytes = response
            .bytes()
            .await
            .map_err(|err| ProviderError::internal(err.to_string()))?;
        diagnostics.bytes("upstream.response_body", &bytes);
        if !status.is_success() {
            return Err(map_http_error_with_headers(status, &headers, &bytes));
        }
        let value: VertexGenerateContentResponse = serde_json::from_slice(&bytes)
            .map_err(|err| ProviderError::internal(format!("parse vertex response: {err}")))?;
        map_vertex_response(value, &self.model, &self.thought_signatures)
    }

    async fn stream<'a>(
        &'a self,
        request: UnifiedRequest,
        _metadata: &M,
    ) -> Result<
        Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    > {
        let body =
            build_unified_vertex_request(&request, &shared_http_client(), &self.thought_signatures)
                .await?;
        let url = generate_content_url(&self.location, &self.project_id, &self.model, true);
        let diagnostics = UpstreamLog::new("google-vertex", &self.model);
        diagnostics.json("upstream.request", Some(&url), &body);
        let response = self
            .post_builder(url)
            .await?
            .body(
                serde_json::to_vec(&body)
                    .map_err(|err| ProviderError::internal(err.to_string()))?,
            )
            .send()
            .await
            .map_err(|err| ProviderError::internal(err.to_string()))?;

        let status = response.status();
        diagnostics.response_start(status.as_u16(), response.headers());
        if !status.is_success() {
            let headers = response.headers().clone();
            let bytes = response
                .bytes()
                .await
                .map_err(|err| ProviderError::internal(err.to_string()))?;
            diagnostics.bytes("upstream.response_body", &bytes);
            return Err(map_http_error_with_headers(status, &headers, &bytes));
        }

        let stream = response.bytes_stream();
        let model_id = self.model.clone();
        let output = async_stream::try_stream! {
            futures_util::pin_mut!(stream);
            let mut state = VertexStreamState::default();
            state.model = model_id;

            let mut buffer = String::new();
            while let Some(item) = stream.next().await {
                let chunk = item.map_err(|err| ProviderError::internal(err.to_string()))?;
                let text = String::from_utf8_lossy(&chunk);
                buffer.push_str(&text);

                while let Some(pos) = buffer.find('\n') {
                    let line = buffer[..pos].trim().to_string();
                    buffer.drain(..=pos);
                    if line.is_empty() {
                        continue;
                    }
                    if let Some(data) = line
                        .strip_prefix("data: ")
                        .or_else(|| line.strip_prefix("data:"))
                    {
                        diagnostics.bytes("upstream.stream_event", data.as_bytes());
                        if data.trim() == "[DONE]" {
                            if !state.completed {
                                yield UnifiedEvent::new(UnifiedEventKind::Completed { response: None });
                            }
                            return;
                        }

                        let value: VertexGenerateContentResponse = serde_json::from_str(data)
                            .map_err(|err| ProviderError::internal(format!("parse vertex stream event: {err}")))?;

                        for event in
                            map_vertex_stream_chunk(&value, &mut state, &self.thought_signatures)
                        {
                            yield event;
                        }
                    }
                }
            }

            if !state.completed {
                yield UnifiedEvent::new(UnifiedEventKind::Completed { response: None });
            }
        };

        Ok(Box::pin(output))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn generate_content_url_builds_regional_and_global() {
        assert_eq!(
            generate_content_url("us-central1", "pc-1", "gemini-3.5-flash", false),
            "https://us-central1-aiplatform.googleapis.com/v1/projects/pc-1/locations/us-central1/publishers/google/models/gemini-3.5-flash:generateContent"
        );
        assert_eq!(
            generate_content_url("global", "pc-1", "gemini-3.5-flash", true),
            "https://aiplatform.googleapis.com/v1/projects/pc-1/locations/global/publishers/google/models/gemini-3.5-flash:streamGenerateContent?alt=sse"
        );
    }

    #[test]
    fn provider_builds_from_typed_params() {
        let params = GoogleVertexParams {
            model: "gemini-3.5-flash".to_string(),
            project_id: "pc-1".to_string(),
            location: "us-central1".to_string(),
            credentials_file: "/nonexistent/sa.json".into(),
        };
        let provider = GoogleVertexProvider::new(&params);
        assert_eq!(Provider::<()>::model_id(&provider), "gemini-3.5-flash");
    }
}
