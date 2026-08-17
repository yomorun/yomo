use std::sync::Arc;
use std::time::Duration;

use anyhow::anyhow;
use axum::http::{HeaderMap, HeaderValue};
use reqwest::header::{AUTHORIZATION, CONTENT_TYPE};
use tokio::sync::OnceCell;
use yup_oauth2::authenticator::DefaultAuthenticator;
use yup_oauth2::{ServiceAccountAuthenticator, read_service_account_key};

#[derive(Clone)]
pub struct VertexAIClient {
    http: reqwest::Client,
    project_id: String,
    location: String,
    credentials_file: String,
    authenticator: Arc<OnceCell<DefaultAuthenticator>>,
}

impl VertexAIClient {
    pub fn new(
        project_id: String,
        location: String,
        credentials_file: String,
    ) -> Result<Self, anyhow::Error> {
        let http = reqwest::Client::builder()
            .timeout(Duration::from_secs(300))
            .build()?;
        Ok(Self {
            http,
            project_id,
            location,
            credentials_file,
            authenticator: Arc::new(OnceCell::new()),
        })
    }

    pub fn http(&self) -> &reqwest::Client {
        &self.http
    }

    pub async fn post_json_with_headers(
        &self,
        model: &str,
        body: Vec<u8>,
        stream: bool,
        mut headers: HeaderMap,
    ) -> Result<reqwest::Response, anyhow::Error> {
        let token = self.access_token().await?;
        let url = self.generate_content_url(model, stream);
        headers.insert(
            AUTHORIZATION,
            format!("Bearer {token}").parse::<HeaderValue>()?,
        );
        headers.insert(CONTENT_TYPE, "application/json".parse::<HeaderValue>()?);
        let response = self
            .http
            .post(url)
            .headers(headers)
            .body(body)
            .send()
            .await?;
        Ok(response)
    }

    async fn access_token(&self) -> Result<String, anyhow::Error> {
        let authenticator = self
            .authenticator
            .get_or_try_init(|| async {
                let service_account_key = read_service_account_key(&self.credentials_file).await?;
                let authenticator = ServiceAccountAuthenticator::builder(service_account_key)
                    .build()
                    .await?;
                Ok::<DefaultAuthenticator, anyhow::Error>(authenticator)
            })
            .await?;

        let token = authenticator
            .token(&["https://www.googleapis.com/auth/cloud-platform"])
            .await?;
        token
            .token()
            .map(ToString::to_string)
            .ok_or_else(|| anyhow!("missing google access token"))
    }

    fn generate_content_url(&self, model: &str, stream: bool) -> String {
        let action = if stream {
            "streamGenerateContent"
        } else {
            "generateContent"
        };
        let base = if self.location == "global" {
            format!(
                "https://aiplatform.googleapis.com/v1/projects/{}/locations/{}/publishers/google/models/{}:{}",
                self.project_id, self.location, model, action
            )
        } else {
            format!(
                "https://{}-aiplatform.googleapis.com/v1/projects/{}/locations/{}/publishers/google/models/{}:{}",
                self.location, self.project_id, self.location, model, action
            )
        };
        if stream {
            format!("{base}?alt=sse")
        } else {
            base
        }
    }
}
