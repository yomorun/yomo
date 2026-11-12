//! AWS Bedrock Converse provider and client lifecycle.

pub mod mapper;

pub use mapper::{BedrockConverseRequest, ir_to_converse_request};
use mapper::{
    ConverseRequestParts, ConverseStreamState, converse_output_to_unified,
    unified_to_converse_request,
};

use std::pin::Pin;
use std::sync::Arc;

use async_trait::async_trait;
use aws_sdk_bedrockruntime::config::{BehaviorVersion, Token};
use aws_sdk_bedrockruntime::error::DisplayErrorContext;
use axum::http::StatusCode;
use futures_core::Stream;
use secrecy::{ExposeSecret, SecretString};
use tokio::sync::OnceCell;

use crate::models::{ModelProfile, default_model_profile};
use crate::openai_types::ErrorDetail;
use crate::provider::UpstreamLog;
use crate::provider::{
    Provider, ProviderError, UnifiedEvent, UnifiedEventKind, UnifiedRequest, UnifiedResponse,
};
use crate::serve_config::{BedrockParams, ModelBinding};

/// `bedrock-converse` dialect client.
#[derive(Clone)]
pub struct BedrockConverseProvider {
    bedrock_model: String,
    region: String,
    bearer_token: SecretString,
    client: Arc<OnceCell<aws_sdk_bedrockruntime::Client>>,
    binding: Option<ModelBinding>,
    profile: Arc<dyn ModelProfile>,
}

impl BedrockConverseProvider {
    pub fn new(params: &BedrockParams) -> Self {
        Self::construct(params, None, default_model_profile())
    }

    /// Create a fixed Bedrock integration with a caller-provided model profile.
    pub fn new_bound_with_profile(
        params: &BedrockParams,
        binding: ModelBinding,
        profile: Arc<dyn ModelProfile>,
    ) -> Self {
        Self::construct(params, Some(binding), profile)
    }

    fn construct(
        params: &BedrockParams,
        binding: Option<ModelBinding>,
        profile: Arc<dyn ModelProfile>,
    ) -> Self {
        Self {
            bedrock_model: params.model.clone(),
            region: params.aws_region.clone(),
            bearer_token: params.aws_bearer_token.clone(),
            client: Arc::new(OnceCell::new()),
            binding,
            profile,
        }
    }

    fn unified_to_request(
        &self,
        request: &mut UnifiedRequest,
    ) -> Result<ConverseRequestParts, ProviderError> {
        if let Some(binding) = &self.binding {
            if let Some(request) = self.profile.to_bedrock_converse_request(request, binding)? {
                return mapper::bedrock_request_to_sdk_parts(request)
                    .map_err(Self::invalid_request);
            }
            self.profile.adapt_bedrock_request(request, binding)?;
        }

        let mut bedrock_request =
            unified_to_converse_request(request).map_err(Self::invalid_request)?;
        let profile_model = self
            .binding
            .as_ref()
            .map(|binding| binding.public_model.as_str())
            .unwrap_or(&self.bedrock_model);
        bedrock_request.additional_model_request_fields =
            mapper::additional_model_request_fields(request, profile_model)
                .map_err(Self::invalid_request)?;
        Ok(bedrock_request)
    }

    async fn client(&self) -> Result<&aws_sdk_bedrockruntime::Client, ProviderError> {
        self.client
            .get_or_try_init(|| async {
                let config = aws_sdk_bedrockruntime::Config::builder()
                    .behavior_version(BehaviorVersion::latest())
                    .region(aws_types::region::Region::new(self.region.clone()))
                    .bearer_token(Token::new(
                        self.bearer_token.expose_secret().to_string(),
                        None,
                    ))
                    .build();
                Ok::<_, ProviderError>(aws_sdk_bedrockruntime::Client::from_conf(config))
            })
            .await
    }

    fn map_sdk_error(err: impl std::fmt::Debug) -> ProviderError {
        ProviderError::internal(format!("bedrock converse failed: {err:?}"))
    }

    fn invalid_request(message: String) -> ProviderError {
        ProviderError::Public {
            status: StatusCode::BAD_REQUEST,
            error: ErrorDetail {
                message,
                r#type: "invalid_request_error".into(),
                code: Some("unsupported_capability".into()),
                param: None,
            },
        }
    }
}

#[async_trait]
impl<M> Provider<M> for BedrockConverseProvider {
    fn model_id(&self) -> &str {
        &self.bedrock_model
    }

    fn binding(&self) -> Option<&ModelBinding> {
        self.binding.as_ref()
    }

    async fn complete(
        &self,
        mut request: UnifiedRequest,
    ) -> Result<UnifiedResponse, ProviderError> {
        let bedrock_request = self.unified_to_request(&mut request)?;
        let ConverseRequestParts {
            messages,
            system,
            tool_config,
            inference,
            additional_model_request_fields,
        } = bedrock_request;
        let client = self.client().await?;

        let diagnostics = UpstreamLog::new("bedrock-converse", &self.bedrock_model);
        diagnostics.sdk(
            "upstream.request",
            &(
                &messages,
                &system,
                &tool_config,
                &inference,
                &additional_model_request_fields,
            ),
        );

        let mut builder = client
            .converse()
            .model_id(&self.bedrock_model)
            .set_messages(Some(messages))
            .set_system(system)
            .set_inference_config(Some(inference));
        if let Some(tool_config) = tool_config {
            builder = builder.tool_config(tool_config);
        }
        if let Some(fields) = additional_model_request_fields {
            builder = builder.additional_model_request_fields(fields);
        }

        let output = builder.send().await.map_err(|err| {
            if let Some(service) = err.as_service_error() {
                return crate::upstreams::aws_bedrock::sdk_service_error(service);
            }
            ProviderError::internal(format!(
                "bedrock converse failed: {}",
                DisplayErrorContext(&err)
            ))
        })?;

        diagnostics.sdk("upstream.response_body", &output);
        converse_output_to_unified(output, &self.bedrock_model)
    }

    async fn stream<'a>(
        &'a self,
        mut request: UnifiedRequest,
    ) -> Result<
        Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    > {
        let bedrock_request = self.unified_to_request(&mut request)?;
        let ConverseRequestParts {
            messages,
            system,
            tool_config,
            inference,
            additional_model_request_fields,
        } = bedrock_request;
        let client = self.client().await?;

        let diagnostics = UpstreamLog::new("bedrock-converse", &self.bedrock_model);
        diagnostics.sdk(
            "upstream.request",
            &(
                &messages,
                &system,
                &tool_config,
                &inference,
                &additional_model_request_fields,
            ),
        );

        let mut builder = client
            .converse_stream()
            .model_id(&self.bedrock_model)
            .set_messages(Some(messages))
            .set_system(system)
            .set_inference_config(Some(inference));
        if let Some(tool_config) = tool_config {
            builder = builder.tool_config(tool_config);
        }
        if let Some(fields) = additional_model_request_fields {
            builder = builder.additional_model_request_fields(fields);
        }

        let output = builder.send().await.map_err(|err| {
            if let Some(service) = err.as_service_error() {
                return crate::upstreams::aws_bedrock::sdk_service_error(service);
            }
            ProviderError::internal(format!(
                "bedrock converse_stream failed: {}",
                DisplayErrorContext(&err)
            ))
        })?;

        let model = self.bedrock_model.clone();
        let mut stream = output.stream;
        let output_stream = async_stream::try_stream! {
            let mut state = ConverseStreamState::new(model);
            loop {
                match stream.recv().await {
                    Ok(Some(event)) => {
                        diagnostics.sdk("upstream.stream_event", &event);
                        for unified in state.on_event(event) {
                            yield unified;
                        }
                    }
                    Ok(None) => {
                        yield UnifiedEvent::new(UnifiedEventKind::Completed { response: None });
                        break;
                    },
                    Err(err) => {
                        Err(BedrockConverseProvider::map_sdk_error(err))?;
                    }
                }
            }
        };
        Ok(Box::pin(output_stream))
    }
}
