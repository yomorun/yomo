use anyhow::{Result, anyhow};
use axum::http::StatusCode;
use log::error;
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
    async fn accept(&mut self) -> Result<Option<(R1, W1)>> {
        Ok(None)
    }

    async fn find_downstream(&self, _headers: &RequestHeaders) -> Result<Option<C>> {
        Ok(None)
    }

    async fn forward(&self, mut r1: R1, mut w1: W1) -> Result<()> {
        let headers: RequestHeaders = receive_frame(&mut r1)
            .await?
            .ok_or(anyhow!("failed to parse headers"))?;

        match self.find_downstream(&headers).await? {
            Some(connector) => {
                let (r2, mut w2) = connector.open_new_stream().await?;

                send_frame(&mut w2, &headers).await?;

                // pipe request & response body streams
                pipe_streams(r1, w1, r2, w2);
            }
            None => {
                send_frame(
                    &mut w1,
                    &ResponseHeaders {
                        status_code: StatusCode::NOT_FOUND.as_u16(),
                        error_msg: "downstream not found".to_owned(),
                        body_format: BodyFormat::Null,
                        ..Default::default()
                    },
                )
                .await?;
                w1.shutdown().await?;
            }
        }

        Ok(())
    }

    /// Start bridge service to accept and forward requests
    async fn serve_bridge(mut self) -> Result<()> {
        while let Some((r1, w1)) = self.accept().await? {
            let bridge = self.clone();
            spawn(async move {
                if let Err(e) = bridge.forward(r1, w1).await {
                    error!("forward error: {}", e);
                }
            });
        }

        Ok(())
    }
}
