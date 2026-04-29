use std::marker::PhantomData;
use std::sync::Arc;

use async_trait::async_trait;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use crate::connector::Connector;
use crate::io::{receive_frame, send_frame};
use crate::types::{RequestHeaders, ResponseHeaders, ToolRequest, ToolResponse};

#[async_trait]
pub trait ToolInvoker<M>: Send + Sync {
    async fn invoke(
        &self,
        metadata: &M,
        headers: RequestHeaders,
        request: ToolRequest,
    ) -> ToolResponse;
}

pub struct ConnToolInvoker<Metadata, C, R, W> {
    connector: Arc<C>,
    _marker: PhantomData<(Metadata, R, W)>,
}

impl<Metadata, C, R, W> ConnToolInvoker<Metadata, C, R, W> {
    pub fn new(connector: Arc<C>) -> Self {
        Self {
            connector,
            _marker: PhantomData,
        }
    }
}

#[async_trait]
impl<Metadata, C, R, W> ToolInvoker<Metadata> for ConnToolInvoker<Metadata, C, R, W>
where
    Metadata: Send + Sync + 'static,
    C: Connector<R, W> + Send + Sync + 'static,
    R: AsyncReadExt + Unpin + Send + Sync + 'static,
    W: AsyncWriteExt + Unpin + Send + Sync + 'static,
{
    async fn invoke(
        &self,
        _metadata: &Metadata,
        headers: RequestHeaders,
        request: ToolRequest,
    ) -> ToolResponse {
        let connector = self.connector.clone();

        let (mut reader, mut writer) = match connector.open_new_stream().await {
            Ok(streams) => streams,
            Err(err) => {
                return ToolResponse {
                    result: None,
                    error_msg: Some(format!("tool_connection_error: {err}")),
                };
            }
        };

        if let Err(err) = send_frame(&mut writer, &headers).await {
            return ToolResponse {
                result: None,
                error_msg: Some(format!("tool_request_error: {err}")),
            };
        }
        if let Err(err) = send_frame(&mut writer, &request).await {
            return ToolResponse {
                result: None,
                error_msg: Some(format!("tool_request_error: {err}")),
            };
        }
        if let Err(err) = writer.shutdown().await {
            return ToolResponse {
                result: None,
                error_msg: Some(format!("tool_request_error: {err}")),
            };
        }

        let response_headers: ResponseHeaders = match receive_frame(&mut reader).await {
            Ok(Some(headers)) => headers,
            Ok(None) => {
                return ToolResponse {
                    result: None,
                    error_msg: Some("tool_response_error: missing response headers".to_string()),
                };
            }
            Err(err) => {
                return ToolResponse {
                    result: None,
                    error_msg: Some(format!("tool_response_error: {err}")),
                };
            }
        };

        if response_headers.status_code != 200 {
            return ToolResponse {
                result: None,
                error_msg: Some(response_headers.error_msg),
            };
        }

        match receive_frame(&mut reader).await {
            Ok(Some(response)) => response,
            Ok(None) => ToolResponse {
                result: None,
                error_msg: Some("tool_response_error: missing tool response".to_string()),
            },
            Err(err) => ToolResponse {
                result: None,
                error_msg: Some(format!("tool_response_error: {err}")),
            },
        }
    }
}
