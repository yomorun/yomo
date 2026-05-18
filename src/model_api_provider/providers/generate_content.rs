use std::sync::Arc;

use async_trait::async_trait;
use futures_util::StreamExt;

use crate::llm_provider::vertexai::client::VertexAIClient;
use crate::model_api_provider::provider::{
    ModelApiProvider, ProviderBody, ProviderRequest, ProviderResponse, filter_request_headers,
    filter_response_headers, parse_stream_flag,
};
use crate::serve_config::{ConfigError, ProviderConfig};

#[derive(Clone)]
pub struct GenerateContentClient {
    model_id: String,
    upstream_model: String,
    client: VertexAIClient,
}

impl GenerateContentClient {
    pub fn new(model_id: String, upstream_model: String, client: VertexAIClient) -> Self {
        Self {
            model_id,
            upstream_model,
            client,
        }
    }
}

#[async_trait]
impl ModelApiProvider for GenerateContentClient {
    fn model_id(&self) -> &str {
        &self.model_id
    }

    async fn execute(&self, req: ProviderRequest) -> Result<ProviderResponse, anyhow::Error> {
        let stream = parse_stream_flag(&req.body);
        let headers = filter_request_headers(req.headers);
        let response = self
            .client
            .post_json_with_headers(&self.upstream_model, req.body.to_vec(), stream, headers)
            .await?;
        let status = response.status();
        let mut resp_headers = filter_response_headers(response.headers());

        if stream {
            resp_headers.remove(axum::http::header::CONTENT_LENGTH);
            let body_stream = response.bytes_stream().map(|chunk| match chunk {
                Ok(bytes) => Ok(bytes),
                Err(err) => Err(std::io::Error::new(std::io::ErrorKind::Other, err)),
            });

            Ok(ProviderResponse {
                status,
                headers: resp_headers,
                body: ProviderBody::Stream(Box::pin(body_stream)),
            })
        } else {
            let bytes = response.bytes().await?;
            Ok(ProviderResponse {
                status,
                headers: resp_headers,
                body: ProviderBody::Full(bytes),
            })
        }
    }
}

pub fn build_client(provider: &ProviderConfig) -> Result<Arc<dyn ModelApiProvider>, ConfigError> {
    if provider.provider_type != "generate_content" {
        return Err(ConfigError::UnknownProviderType(
            provider.provider_type.clone(),
        ));
    }
    let project_id = provider
        .params
        .get("project_id")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("project_id is required".to_string()))?;
    let location = provider
        .params
        .get("location")
        .cloned()
        .unwrap_or_else(|| "global".to_string());
    let credentials_file = provider
        .params
        .get("credentials_file")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("credentials_file is required".to_string()))?;
    let upstream_model = provider
        .params
        .get("model")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("model is required".to_string()))?;

    let client = VertexAIClient::new(project_id, location, credentials_file)
        .map_err(|err| ConfigError::InvalidProvider(err.to_string()))?;
    Ok(Arc::new(GenerateContentClient::new(
        provider.model_id.clone(),
        upstream_model,
        client,
    )))
}
