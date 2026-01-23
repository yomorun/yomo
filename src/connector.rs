use anyhow::Result;
use s2n_quic::{
    connection::Handle,
    stream::{ReceiveStream, SendStream},
};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt},
    net::{
        TcpStream,
        tcp::{OwnedReadHalf, OwnedWriteHalf},
    },
};

#[async_trait::async_trait]
pub trait Connector<R, W>: Send
where
    R: AsyncReadExt + Unpin + Send + 'static,
    W: AsyncWriteExt + Unpin + Send + 'static,
{
    async fn open_new_stream(&mut self) -> Result<(R, W)>;
}

pub struct TcpConnector {
    tcp_addr: String,
}

impl TcpConnector {
    pub fn new(tcp_addr: &str) -> Self {
        Self {
            tcp_addr: tcp_addr.to_owned(),
        }
    }
}

#[async_trait::async_trait]
impl Connector<OwnedReadHalf, OwnedWriteHalf> for TcpConnector {
    async fn open_new_stream(&mut self) -> Result<(OwnedReadHalf, OwnedWriteHalf)> {
        let stream = TcpStream::connect(&self.tcp_addr).await?;
        Ok(stream.into_split())
    }
}

pub struct QuicConnector {
    handle: Handle,
}

impl QuicConnector {
    pub fn new(handle: Handle) -> Self {
        Self { handle }
    }
}

#[async_trait::async_trait]
impl Connector<ReceiveStream, SendStream> for QuicConnector {
    async fn open_new_stream(&mut self) -> Result<(ReceiveStream, SendStream)> {
        let stream = self.handle.open_bidirectional_stream().await?;
        Ok(stream.split())
    }
}
