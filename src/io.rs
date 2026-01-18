use anyhow::Result;
use log::trace;
use rmp_serde::{from_slice, to_vec};
use serde::{Deserialize, Serialize};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt, copy},
    join,
};

pub async fn send_all<T: Serialize>(
    mut send_stream: impl AsyncWriteExt + Unpin,
    data: &T,
) -> Result<()> {
    let buf = to_vec(data)?;
    trace!("send bytes: {:?}", buf);
    send_stream.write_all(&buf).await?;
    send_stream.shutdown().await?;
    Ok(())
}

pub async fn receive_all<T: for<'a> Deserialize<'a>>(
    mut read_stream: impl AsyncReadExt + Unpin,
) -> Result<T> {
    let mut buf = Vec::new();
    read_stream.read_to_end(&mut buf).await?;
    trace!("received bytes: {:?}", buf);
    let res: T = from_slice(&buf)?;
    Ok(res)
}

pub async fn send_chunk<T: Serialize>(
    send_stream: &mut (impl AsyncWriteExt + Unpin),
    data: &T,
) -> Result<()> {
    let buf = to_vec(data)?;
    trace!("send chunk bytes: {:?}", buf);
    send_stream.write_u32(buf.len() as u32).await?;
    send_stream.write_all(&buf).await?;
    Ok(())
}

pub async fn send_chunk_done(mut send_stream: impl AsyncWriteExt + Unpin) -> Result<()> {
    trace!("send chunk done");
    send_stream.write_u32(0).await?;
    send_stream.shutdown().await?;
    Ok(())
}

pub async fn receive_chunk<T: for<'a> Deserialize<'a>>(
    read_stream: &mut (impl AsyncReadExt + Unpin),
) -> Result<Option<T>> {
    let size = read_stream.read_u32().await?;
    if size == 0 {
        let mut buf = Vec::new();
        read_stream.read_to_end(&mut buf).await?;
        return Ok(None);
    }
    let mut buf = vec![0; size as usize];
    read_stream.read_exact(&mut buf).await?;
    trace!("received chunk bytes: {:?}", buf);
    let res: T = from_slice(&buf)?;
    Ok(Some(res))
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
