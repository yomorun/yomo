use std::fmt::Debug;

use anyhow::Result;
use log::{debug, trace};
use serde::{Deserialize, Serialize};
use tokio::io::{AsyncReadExt, AsyncWriteExt};

pub(crate) async fn read_packet<T: for<'a> Deserialize<'a> + Debug>(
    reader: &mut (impl AsyncReadExt + Unpin),
) -> Result<T> {
    let length = reader.read_u32().await?;
    let mut raw = vec![0; length as usize];
    reader.read_exact(&mut raw).await?;
    let f: T = serde_json::from_slice(&raw)?;
    debug!("read packet: {:?}", f);
    trace!("read packet raw: {}", String::from_utf8_lossy(&raw));
    Ok(f)
}

pub(crate) async fn write_packet<T: Serialize + Debug>(
    writer: &mut (impl AsyncWriteExt + Unpin),
    packet: &T,
) -> Result<()> {
    let raw = serde_json::to_vec(packet)?;
    writer.write_u32(raw.len() as u32).await?;
    writer.write_all(&raw).await?;
    debug!("write packet: {:?}", packet);
    trace!("write packet raw: {}", String::from_utf8_lossy(&raw));
    Ok(())
}
