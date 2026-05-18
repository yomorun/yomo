use std::sync::Arc;

use async_trait::async_trait;
use aws_config::BehaviorVersion;
use aws_credential_types::Credentials;
use aws_sdk_bedrockruntime::primitives::Blob;
use aws_types::region::Region;
use axum::body::Bytes;
use axum::http::{HeaderMap, StatusCode};
use tokio::sync::OnceCell;

use crate::model_api_provider::provider::{
    ModelApiProvider, ProviderBody, ProviderRequest, ProviderResponse, parse_stream_flag,
    rewrite_messages_body,
};
use crate::serve_config::{ConfigError, ProviderConfig};

#[derive(Clone)]
pub struct MessagesClient {
    model_id: String,
    bedrock_model: String,
    aws_region: String,
    anthropic_version: String,
    default_max_tokens: u64,
    aws_access_key_id: Option<String>,
    aws_secret_access_key: Option<String>,
    aws_session_token: Option<String>,
    bedrock_client: Arc<OnceCell<aws_sdk_bedrockruntime::Client>>,
}

impl MessagesClient {
    pub fn new(
        model_id: String,
        bedrock_model: String,
        aws_region: String,
        anthropic_version: String,
        default_max_tokens: u64,
        aws_access_key_id: Option<String>,
        aws_secret_access_key: Option<String>,
        aws_session_token: Option<String>,
    ) -> Self {
        Self {
            model_id,
            bedrock_model,
            aws_region,
            anthropic_version,
            default_max_tokens,
            aws_access_key_id,
            aws_secret_access_key,
            aws_session_token,
            bedrock_client: Arc::new(OnceCell::new()),
        }
    }

    async fn client(&self) -> Result<&aws_sdk_bedrockruntime::Client, anyhow::Error> {
        self.bedrock_client
            .get_or_try_init(|| async {
                let mut loader = aws_config::defaults(BehaviorVersion::latest())
                    .region(Region::new(self.aws_region.clone()));

                if let (Some(access_key_id), Some(secret_access_key)) = (
                    self.aws_access_key_id.as_ref(),
                    self.aws_secret_access_key.as_ref(),
                ) {
                    let credentials = Credentials::new(
                        access_key_id,
                        secret_access_key,
                        self.aws_session_token.clone(),
                        None,
                        "model-api-messages",
                    );
                    loader = loader.credentials_provider(credentials);
                }

                let config = loader.load().await;
                Ok::<aws_sdk_bedrockruntime::Client, anyhow::Error>(
                    aws_sdk_bedrockruntime::Client::new(&config),
                )
            })
            .await
    }
}

#[async_trait]
impl ModelApiProvider for MessagesClient {
    fn model_id(&self) -> &str {
        &self.model_id
    }

    async fn execute(&self, req: ProviderRequest) -> Result<ProviderResponse, anyhow::Error> {
        let stream = parse_stream_flag(&req.body);
        let body =
            rewrite_messages_body(&req.body, &self.anthropic_version, self.default_max_tokens)?;
        let client = self.client().await?;

        if stream {
            let response = client
                .invoke_model_with_response_stream()
                .model_id(&self.bedrock_model)
                .content_type("application/json")
                .body(Blob::new(body.to_vec()))
                .send()
                .await?;

            let mut stream = response.body;
            let mapped = async_stream::stream! {
                loop {
                    match stream.recv().await {
                        Ok(Some(event)) => {
                            if let Ok(chunk) = event.as_chunk() {
                                if let Some(payload) = chunk.bytes.as_ref() {
                                    let frame = format!(
                                        "data: {}\n\n",
                                        String::from_utf8_lossy(payload.as_ref())
                                    );
                                    yield Ok::<Bytes, std::io::Error>(Bytes::from(frame));
                                }
                            }
                        }
                        Ok(None) => {
                            break;
                        }
                        Err(err) => {
                            yield Err(std::io::Error::new(std::io::ErrorKind::Other, err.to_string()));
                            break;
                        }
                    }
                }
                yield Ok(Bytes::from_static(b"data: [DONE]\n\n"));
            };

            let mut headers = HeaderMap::new();
            headers.insert(
                axum::http::header::CONTENT_TYPE,
                "text/event-stream"
                    .parse()
                    .expect("static header value must be valid"),
            );
            headers.insert(
                axum::http::header::CACHE_CONTROL,
                "no-cache"
                    .parse()
                    .expect("static header value must be valid"),
            );
            headers.insert(
                axum::http::header::CONNECTION,
                "keep-alive"
                    .parse()
                    .expect("static header value must be valid"),
            );

            Ok(ProviderResponse {
                status: StatusCode::OK,
                headers,
                body: ProviderBody::Stream(Box::pin(mapped)),
            })
        } else {
            let response = client
                .invoke_model()
                .model_id(&self.bedrock_model)
                .content_type("application/json")
                .accept("application/json")
                .body(Blob::new(body.to_vec()))
                .send()
                .await?;

            let mut headers = HeaderMap::new();
            headers.insert(
                axum::http::header::CONTENT_TYPE,
                "application/json"
                    .parse()
                    .expect("static header value must be valid"),
            );
            Ok(ProviderResponse {
                status: StatusCode::OK,
                headers,
                body: ProviderBody::Full(Bytes::from(response.body.into_inner())),
            })
        }
    }
}

pub fn build_client(provider: &ProviderConfig) -> Result<Arc<dyn ModelApiProvider>, ConfigError> {
    if provider.provider_type != "bedrock-messages" {
        return Err(ConfigError::UnknownProviderType(
            provider.provider_type.clone(),
        ));
    }
    let bedrock_model = provider
        .params
        .get("model")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("model is required".to_string()))?;
    let aws_region = provider
        .params
        .get("aws_region")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("aws_region is required".to_string()))?;
    let anthropic_version = provider
        .params
        .get("anthropic_version")
        .cloned()
        .unwrap_or_else(|| "bedrock-2023-05-31".to_string());
    let default_max_tokens = provider
        .params
        .get("max_tokens")
        .and_then(|raw| raw.parse::<u64>().ok())
        .unwrap_or(4096);
    let aws_access_key_id = provider.params.get("aws_access_key_id").cloned();
    let aws_secret_access_key = provider.params.get("aws_secret_access_key").cloned();
    let aws_session_token = provider.params.get("aws_session_token").cloned();

    Ok(Arc::new(MessagesClient::new(
        provider.model_id.clone(),
        bedrock_model,
        aws_region,
        anthropic_version,
        default_max_tokens,
        aws_access_key_id,
        aws_secret_access_key,
        aws_session_token,
    )))
}
