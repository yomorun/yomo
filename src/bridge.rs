use anyhow::Result;
use axum::http::StatusCode;
use log::{error, info};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt},
    spawn,
};

use crate::{
    connector::Connector,
    io::{pipe_streams, receive_frame, send_frame},
    types::{BodyFormat, RequestHeaders, ResponseHeaders},
};

/// Bridge trait for forwarding requests between protocols
#[async_trait::async_trait]
pub trait Bridge<C, R1, W1, R2, W2>: Clone + Send + Sync + 'static
where
    C: Connector<R2, W2>,
    R1: AsyncReadExt + Unpin + Send + 'static,
    W1: AsyncWriteExt + Unpin + Send + 'static,
    R2: AsyncReadExt + Unpin + Send + 'static,
    W2: AsyncWriteExt + Unpin + Send + 'static,
{
    async fn accept(&mut self) -> Result<Option<(R1, W1)>>;

    async fn find_downstream(&self, req_headers: &RequestHeaders) -> Result<Option<C>>;

    /// Start bridge service to accept and forward requests
    async fn serve_bridge(mut self) {
        while let Ok(Some((mut r1, mut w1))) = self.accept().await {
            let bridge = self.clone();

            spawn(async move {
                match receive_frame(&mut r1).await {
                    Ok(Some(req_headers)) => match bridge.find_downstream(&req_headers).await {
                        Ok(Some(connector)) => {
                            info!(
                                "[{}|{}] forward '{}' to downstream",
                                req_headers.trace_id, req_headers.request_id, req_headers.sfn_name
                            );

                            match connector.open_new_stream().await {
                                Ok((r2, mut w2)) => {
                                    if let Err(e) = send_frame(&mut w2, &req_headers).await {
                                        Self::response_error_headers(
                                            &mut w1,
                                            StatusCode::INTERNAL_SERVER_ERROR,
                                            &format!("send request headers error: {}", e),
                                        )
                                        .await;
                                    } else {
                                        pipe_streams(r1, w1, r2, w2);
                                    }
                                }
                                Err(e) => {
                                    Self::response_error_headers(
                                        &mut w1,
                                        StatusCode::INTERNAL_SERVER_ERROR,
                                        &format!("open new stream error: {}", e),
                                    )
                                    .await;
                                }
                            }
                        }
                        Ok(None) => {
                            Self::response_error_headers(
                                &mut w1,
                                StatusCode::NOT_FOUND,
                                &format!("downstream '{}' not found", req_headers.sfn_name),
                            )
                            .await;
                        }
                        Err(e) => {
                            Self::response_error_headers(
                                &mut w1,
                                StatusCode::INTERNAL_SERVER_ERROR,
                                &format!("find downstream '{}' error: {}", req_headers.sfn_name, e),
                            )
                            .await;
                        }
                    },
                    _ => {
                        Self::response_error_headers(
                            &mut w1,
                            StatusCode::BAD_REQUEST,
                            "failed to receive request headers",
                        )
                        .await;
                    }
                }
            });
        }
    }

    async fn response_error_headers(w1: &mut W1, status_code: StatusCode, error_msg: &str) {
        error!(
            "forward to downstream error: {}, {}",
            status_code, error_msg
        );
        send_frame(
            w1,
            &ResponseHeaders {
                status_code: status_code.as_u16(),
                error_msg: error_msg.to_owned(),
                body_format: BodyFormat::Null,
                ..Default::default()
            },
        )
        .await
        .ok();
        w1.shutdown().await.ok();
    }
}
