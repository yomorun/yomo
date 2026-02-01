use anyhow::Result;
use axum::http::StatusCode;
use log::{debug, error};
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

    fn skip_headers(&self) -> bool {
        true
    }

    async fn find_downstream(&self, req_headers: &Option<RequestHeaders>) -> Result<Option<C>>;

    async fn forward(&self, r1: &mut R1) -> Result<Option<(R2, W2)>> {
        let req_headers = if self.skip_headers() {
            None
        } else {
            receive_frame::<RequestHeaders>(r1).await?
        };

        if let Some(connector) = self.find_downstream(&req_headers).await? {
            let (r2, mut w2) = connector.open_new_stream().await?;

            if let Some(req_headers) = req_headers {
                send_frame(&mut w2, &req_headers).await?;
            }

            Ok(Some((r2, w2)))
        } else {
            Ok(None)
        }
    }

    /// Start bridge service to accept and forward requests
    async fn serve_bridge(mut self) {
        while let Ok(Some((mut r1, mut w1))) = self.accept().await {
            let bridge = self.clone();

            spawn(async move {
                match bridge.forward(&mut r1).await {
                    Ok(Some((r2, w2))) => {
                        pipe_streams(r1, w1, r2, w2).await;
                    }
                    Ok(None) => {
                        debug!("downstream not found");
                        Self::response_error_headers(
                            &mut w1,
                            StatusCode::NOT_FOUND,
                            "downstream not found",
                        )
                        .await;
                    }
                    Err(e) => {
                        error!("forward error: {}", e);
                        Self::response_error_headers(
                            &mut w1,
                            StatusCode::INTERNAL_SERVER_ERROR,
                            &e.to_string(),
                        )
                        .await;
                    }
                }
            });
        }
    }

    async fn response_error_headers(w1: &mut W1, status_code: StatusCode, error_msg: &str) {
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
