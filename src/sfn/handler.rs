use std::time::Duration;

use anyhow::Result;
use log::info;
use serde::{Deserialize, Serialize};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt, ReadHalf, SimplexStream, WriteHalf, simplex},
    spawn,
    time::sleep,
};

use crate::{
    io::{receive_all, send_all, send_chunk, send_chunk_done},
    types::{Request, Response},
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
        let req_stream = simplex(MAX_BUF_SIZE);
        let res_stream = simplex(MAX_BUF_SIZE);

        // open socket connection

        spawn(async move {
            if let Err(e) = mock_handler(req_stream.0, res_stream.1).await {
                eprintln!("Error in mock handler: {}", e);
            }
        });

        Ok((res_stream.0, req_stream.1))
    }
}

#[derive(Deserialize)]
struct HandlerRequest {
    args: String,
}

#[derive(Serialize)]
struct HandlerResponse {
    result: String,
}

#[derive(Serialize)]
struct HandlerDelta {
    delta: String,
}

async fn mock_handler(
    req_stream: impl AsyncReadExt + Unpin,
    mut res_stream: impl AsyncWriteExt + Unpin,
) -> Result<()> {
    let req: Request = receive_all(req_stream).await?;

    info!(
        "received request: {}, stream: {}",
        String::from_utf8_lossy(&req.data),
        req.stream
    );

    let handler_req: HandlerRequest = serde_json::from_slice(&req.data)?;
    let result = handler_req.args.to_ascii_uppercase();

    if req.stream {
        for delta in result.split_inclusive(' ') {
            let handler_delta = HandlerDelta {
                delta: delta.to_owned(),
            };

            let res = Response::Data(serde_json::to_vec(&handler_delta)?);

            send_chunk(&mut res_stream, &res).await?;

            // Add a delay to simulate streaming
            sleep(Duration::from_secs(1)).await;
        }

        send_chunk_done(res_stream).await?;
    } else {
        let handler_res = HandlerResponse { result };
        let data = serde_json::to_vec(&handler_res)?;
        let res = Response::Data(data);

        send_all(res_stream, &res).await?;
    }

    Ok(())
}
