use std::{fmt::Debug, io::ErrorKind};

use anyhow::Result;
use log::{debug, error, trace};
use serde::{Deserialize, Serialize};
use tokio::{
    io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt, BufReader, copy},
    spawn,
};

pub async fn receive_raw(stream: &mut (impl AsyncReadExt + Unpin)) -> Result<Option<String>> {
    let mut r = BufReader::new(stream);
    let mut buf = String::new();

    match r.read_line(&mut buf).await {
        Ok(size) => {
            if size == 0 {
                return Ok(None);
            }
            let buf = buf.trim_end_matches("\n").trim_end_matches("\r");
            trace!("recv raw: {}", buf);
            Ok(Some(buf.to_owned()))
        }
        Err(e) => {
            if e.kind() == ErrorKind::UnexpectedEof {
                return Ok(None);
            }
            error!("receive_raw error: {}", e);
            Ok(None)
        }
    }
}

pub async fn send_frame<T: Serialize + Debug>(
    stream: &mut (impl AsyncWriteExt + Unpin),
    frame: &T,
) -> Result<()> {
    debug!("send frame: {:?}", frame);
    let buf = serde_json::to_string(frame)? + "\n";
    trace!("send frame bytes: {}", buf);
    stream.write_all(&buf.as_bytes()).await?;
    Ok(())
}

pub async fn receive_frame<T: for<'a> Deserialize<'a> + Debug>(
    stream: &mut (impl AsyncReadExt + Unpin),
) -> Result<Option<T>> {
    let raw = receive_raw(stream).await?;
    if let Some(raw) = raw {
        let frame: T = serde_json::from_str(&raw)?;
        debug!("recv frame: {:?}", frame);
        Ok(Some(frame))
    } else {
        Ok(None)
    }
}

pub fn pipe_streams<R1, W1, R2, W2>(mut r1: R1, mut w1: W1, mut r2: R2, mut w2: W2)
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
