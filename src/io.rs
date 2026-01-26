use std::{fmt::Debug, io::ErrorKind};

use anyhow::{Result, bail};
use log::{error, trace};
use serde::{Deserialize, Serialize};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt, copy},
    spawn,
};

/// Send bytes with length-prefixed framing
pub async fn send_bytes(stream: &mut (impl AsyncWriteExt + Unpin), bytes: &[u8]) -> Result<()> {
    trace!("send bytes: {:?}", bytes);
    stream.write_u32(bytes.len() as u32).await?;
    stream.write_all(bytes).await?;
    stream.flush().await?;
    Ok(())
}

/// Receive bytes with length-prefixed framing
pub async fn receive_bytes(stream: &mut (impl AsyncReadExt + Unpin)) -> Result<Option<Vec<u8>>> {
    let size = match stream.read_u32().await {
        Ok(size) => size,
        Err(e) => {
            if e.kind() == ErrorKind::UnexpectedEof {
                return Ok(None);
            }
            bail!("receive bytes error: {}", e);
        }
    };

    let mut bytes = vec![0; size as usize];
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
pub(crate) fn pipe_streams<R1, W1, R2, W2>(mut r1: R1, mut w1: W1, mut r2: R2, mut w2: W2)
where
    R1: AsyncReadExt + Unpin + Send + 'static,
    W1: AsyncWriteExt + Unpin + Send + 'static,
    R2: AsyncReadExt + Unpin + Send + 'static,
    W2: AsyncWriteExt + Unpin + Send + 'static,
{
    spawn(async move {
        if let Err(e) = copy(&mut r1, &mut w2).await {
            error!("copy request stream error: {}", e);
        }
        w2.shutdown().await.ok();
    });

    spawn(async move {
        if let Err(e) = copy(&mut r2, &mut w1).await {
            error!("copy response stream error: {}", e);
        }
        w1.shutdown().await.ok();
    });
}
