use std::fmt::Debug;

use anyhow::Result;
use log::trace;
use rmp_serde::{from_slice, to_vec};
use serde::{Deserialize, Serialize};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt, copy},
    join,
};

use crate::frame::Frame;

pub async fn send_frame<T: Serialize + Debug>(
    send_stream: &mut (impl AsyncWriteExt + Unpin),
    frame: &Frame<T>,
) -> Result<()> {
    let buf = to_vec(frame)?;
    send_stream.write_u32(buf.len() as u32).await?;
    send_stream.write_all(&buf).await?;
    trace!("send frame: {:?}", frame);
    Ok(())
}

pub async fn receive_frame<T: for<'a> Deserialize<'a> + Debug>(
    read_stream: &mut (impl AsyncReadExt + Unpin),
) -> Result<Frame<T>> {
    let size = read_stream.read_u32().await?;
    let mut buf = vec![0; size as usize];
    read_stream.read_exact(&mut buf).await?;
    let frame: Frame<T> = from_slice(&buf)?;
    trace!("received frame: {:?}", frame);
    Ok(frame)
}

pub async fn pipe_stream(
    mut from_reader: impl AsyncReadExt + Unpin,
    mut from_writer: impl AsyncWriteExt + Unpin,
    mut to_reader: impl AsyncReadExt + Unpin,
    mut to_writer: impl AsyncWriteExt + Unpin,
) -> Result<()> {
    let res = join!(
        async move {
            copy(&mut from_reader, &mut to_writer).await?;
            to_writer.shutdown().await?;
            Ok::<(), anyhow::Error>(())
        },
        async move {
            copy(&mut to_reader, &mut from_writer).await?;
            from_writer.shutdown().await?;
            Ok::<(), anyhow::Error>(())
        }
    );
    res.0?;
    res.1?;
    Ok(())
}
