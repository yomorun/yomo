use std::{fmt::Debug, io::ErrorKind};

use anyhow::{Result, bail};
use log::{debug, error, trace};
use serde::{Deserialize, Serialize};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt, copy},
    spawn,
};
async fn receive_raw(stream: &mut (impl AsyncReadExt + Unpin)) -> Result<Option<Vec<u8>>> {
    let size = match stream.read_u32().await {
        Ok(size) => size,
        Err(e) => {
            if e.kind() == ErrorKind::UnexpectedEof {
                return Ok(None);
            }
            bail!("receive_raw error: {}", e);
        }
    };

    let mut buf = vec![0; size as usize];
    stream.read_exact(&mut buf).await?;
    trace!("recv bytes: {:?}", buf);
    Ok(Some(buf))
}

pub async fn send_frame<T: Serialize + Debug>(
    stream: &mut (impl AsyncWriteExt + Unpin),
    frame: &T,
) -> Result<()> {
    debug!("send frame: {:?}", frame);
    let buf = serde_json::to_vec(frame)?;
    stream.write_u32(buf.len() as u32).await?;
    stream.write_all(&buf).await?;
    stream.flush().await?;
    trace!("sent bytes: {:?}", buf);
    Ok(())
}

pub async fn receive_frame<T: for<'a> Deserialize<'a> + Debug>(
    stream: &mut (impl AsyncReadExt + Unpin),
) -> Result<Option<T>> {
    let raw = receive_raw(stream).await?;
    if let Some(raw) = raw {
        let frame: T = serde_json::from_slice(&raw)?;
        debug!("recv frame: {:?}", frame);
        Ok(Some(frame))
    } else {
        Ok(None)
    }
}

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
