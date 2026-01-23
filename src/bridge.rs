use anyhow::Result;
use tokio::io::{AsyncReadExt, AsyncWriteExt};

use crate::{connector::Connector, io::pipe_streams, types::RequestHeaders};

#[async_trait::async_trait]
pub trait Bridge<C, R1, W1, R2, W2>: Send + Sync + 'static
where
    C: Connector<R2, W2>,
    R1: AsyncReadExt + Unpin + Send + 'static,
    W1: AsyncWriteExt + Unpin + Send + 'static,
    R2: AsyncReadExt + Unpin + Send + 'static,
    W2: AsyncWriteExt + Unpin + Send + 'static,
{
    async fn find_downstream(&self, _headers: &RequestHeaders) -> Result<Option<C>> {
        Ok(None)
    }

    async fn forward(&self, headers: &RequestHeaders, r1: R1, w1: W1) -> Result<bool> {
        match self.find_downstream(headers).await? {
            Some(mut connector) => {
                let (r2, w2) = connector.open_new_stream().await?;

                // pipe request & response body streams
                pipe_streams(r1, w1, r2, w2);

                Ok(true)
            }
            None => Ok(false),
        }
    }
}
