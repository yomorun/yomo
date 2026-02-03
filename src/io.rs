use std::{fmt::Debug, io::ErrorKind};

use anyhow::{Result, bail};
use log::{error, trace};
use serde::{Deserialize, Serialize};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt, copy},
    join,
};

const MAX_FRAME_SIZE: u32 = 64 * 1024 * 1024;

/// Send bytes with length-prefixed framing
pub async fn send_bytes(stream: &mut (impl AsyncWriteExt + Unpin), bytes: &[u8]) -> Result<()> {
    let length = bytes.len() as u32;
    if length > MAX_FRAME_SIZE {
        bail!("frame size too large: {}", length);
    }
    trace!("send bytes: {:?}", bytes);
    stream.write_u32(length).await?;
    stream.write_all(bytes).await?;
    Ok(())
}

/// Receive bytes with length-prefixed framing
pub async fn receive_bytes(stream: &mut (impl AsyncReadExt + Unpin)) -> Result<Option<Vec<u8>>> {
    let length = match stream.read_u32().await {
        Ok(size) => size,
        Err(e) => {
            if e.kind() == ErrorKind::UnexpectedEof {
                return Ok(None);
            }
            bail!("receive bytes error: {}", e);
        }
    };

    if length > MAX_FRAME_SIZE {
        bail!("frame size too large: {}", length);
    }

    let mut bytes = vec![0; length as usize];
    stream.read_exact(&mut bytes).await?;
    trace!("receive bytes: {:?}", bytes);
    Ok(Some(bytes))
}

/// Send a serialized frame
pub async fn send_frame<T: Serialize + Debug>(
    stream: &mut (impl AsyncWriteExt + Unpin),
    frame: &T,
) -> Result<()> {
    let bytes = serde_json::to_vec(frame)?;
    send_bytes(stream, &bytes).await?;
    Ok(())
}

/// Receive and deserialize a frame
pub async fn receive_frame<T: for<'a> Deserialize<'a> + Debug>(
    stream: &mut (impl AsyncReadExt + Unpin),
) -> Result<Option<T>> {
    if let Some(bytes) = receive_bytes(stream).await? {
        let frame: T = serde_json::from_slice(&bytes)?;
        Ok(Some(frame))
    } else {
        Ok(None)
    }
}

/// Bidirectional pipe between two streams
pub async fn pipe_streams<R1, W1, R2, W2>(mut r1: R1, mut w1: W1, mut r2: R2, mut w2: W2)
where
    R1: AsyncReadExt + Unpin + Send,
    W1: AsyncWriteExt + Unpin + Send,
    R2: AsyncReadExt + Unpin + Send,
    W2: AsyncWriteExt + Unpin + Send,
{
    join!(
        async move {
            match copy(&mut r1, &mut w2).await {
                Ok(n) => {
                    trace!("copied {} bytes from r1 to w2", n);
                }
                Err(e) => {
                    if e.kind() == ErrorKind::UnexpectedEof {
                        trace!("r1 EOF");
                    } else {
                        error!("copy r1 to w2 error: {}", e);
                    }
                }
            }
            w2.shutdown().await.ok();
        },
        async move {
            match copy(&mut r2, &mut w1).await {
                Ok(n) => {
                    trace!("copied {} bytes from r2 to w1", n);
                }
                Err(e) => {
                    if e.kind() == ErrorKind::UnexpectedEof {
                        trace!("r2 EOF");
                    } else {
                        error!("copy r2 to w1 error: {}", e);
                    }
                }
            }
            w1.shutdown().await.ok();
        },
    );
}
