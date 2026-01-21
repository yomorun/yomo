use std::{fmt::Debug, io::ErrorKind};

use anyhow::Result;
use log::{debug, error, trace};
use serde::{Deserialize, Serialize};
use serde_json::{from_slice, to_vec};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt, copy},
    join,
};

pub async fn send_frame<T: Serialize + Debug>(
    send_stream: &mut (impl AsyncWriteExt + Unpin),
    frame: &T,
) -> Result<()> {
    debug!("send frame: {:?}", frame);
    let buf = to_vec(frame)?;
    trace!("send frame bytes: {:?}", buf);
    send_stream.write_u32(buf.len() as u32).await?;
    send_stream.write_all(&buf).await?;
    Ok(())
}

pub async fn receive_frame<T: for<'a> Deserialize<'a> + Debug>(
    read_stream: &mut (impl AsyncReadExt + Unpin),
) -> Result<Option<T>> {
    match read_stream.read_u32().await {
        Ok(size) => {
            let mut buf = vec![0; size as usize];
            read_stream.read_exact(&mut buf).await?;
            trace!("recv frame bytes: {:?}", buf);
            let frame: T = from_slice(&buf)?;
            debug!("recv frame: {:?}", frame);
            Ok(Some(frame))
        }
        Err(e) => {
            if e.kind() == ErrorKind::UnexpectedEof {
                return Ok(None);
            }
            error!("receive_frame error: {}", e);
            Ok(None)
        }
    }
}

pub async fn pipe_stream(
    mut from_reader: impl AsyncReadExt + Unpin,
    mut from_writer: impl AsyncWriteExt + Unpin,
    mut to_reader: impl AsyncReadExt + Unpin,
    mut to_writer: impl AsyncWriteExt + Unpin,
) {
    join!(
        async move {
            if let Err(e) = copy(&mut from_reader, &mut to_writer).await {
                error!("pipe_stream forward error: {}", e);
            }
            to_writer.shutdown().await.ok();
        },
        async move {
            if let Err(e) = copy(&mut to_reader, &mut from_writer).await {
                error!("pipe_stream backward error: {}", e);
            }
            from_writer.shutdown().await.ok();
        }
    );
}
