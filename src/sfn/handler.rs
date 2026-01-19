use std::time::Duration;

use anyhow::{Result, anyhow};
use log::{error, info};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt, ReadHalf, SimplexStream, WriteHalf, simplex},
    spawn,
    time::sleep,
};

use crate::{
    frame::{Frame, HandlerDelta, HandlerRequest, HandlerResponse},
    io::{receive_frame, send_frame},
};

#[async_trait::async_trait]
pub trait Handler: Send + Sync {
    async fn open(&self) -> Result<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)>;
}

#[derive(Default)]
pub struct HandlerImpl {}

const MAX_BUF_SIZE: usize = 64 * 1024;

#[async_trait::async_trait]
impl Handler for HandlerImpl {
    async fn open(&self) -> Result<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)> {
        let mut req_stream = simplex(MAX_BUF_SIZE);
        let mut res_stream = simplex(MAX_BUF_SIZE);

        // todo: open socket connection

        spawn(async move {
            if let Err(e) = mock_handler(&mut req_stream.0, &mut res_stream.1).await {
                error!("Error in mock handler: {}", e);
            }
            res_stream.1.shutdown().await.ok();
        });

        Ok((res_stream.0, req_stream.1))
    }
}

async fn mock_handler(
    req_stream: &mut (impl AsyncReadExt + Unpin),
    res_stream: &mut (impl AsyncWriteExt + Unpin),
) -> Result<()> {
    if let Frame::Packet(req) = receive_frame::<HandlerRequest>(req_stream).await? {
        info!("received request: args={}, stream={}", req.args, req.stream);

        let result = req.args.to_ascii_uppercase();

        if req.stream {
            let mut count = 0;
            for delta in result.split_inclusive(' ') {
                count += 1;

                send_frame(
                    res_stream,
                    &Frame::Chunk(
                        count,
                        Some(HandlerDelta {
                            delta: delta.to_owned(),
                        }),
                    ),
                )
                .await?;

                // Add a delay to simulate streaming
                sleep(Duration::from_secs(1)).await;
            }

            send_frame::<HandlerDelta>(res_stream, &Frame::ChunkDone(count)).await?;
        } else {
            send_frame(res_stream, &Frame::Packet(HandlerResponse { result })).await?;
        }

        return Ok(());
    }

    Err(anyhow!("invalid request format"))
}
