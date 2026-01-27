use anyhow::Result;
use s2n_quic::{
    connection::Handle,
    stream::{ReceiveStream, SendStream},
};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt, ReadHalf, SimplexStream, WriteHalf, simplex},
    net::{
        TcpStream,
        tcp::{OwnedReadHalf, OwnedWriteHalf},
    },
    sync::mpsc::UnboundedSender,
};

/// Abstract connector for opening new streams
#[async_trait::async_trait]
pub trait Connector<R, W>: Send
where
    R: AsyncReadExt + Unpin + Send + 'static,
    W: AsyncWriteExt + Unpin + Send + 'static,
{
    async fn open_new_stream(&self) -> Result<(R, W)>;
}

/// TCP connector for establishing TCP connections
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
    async fn open_new_stream(&self) -> Result<(OwnedReadHalf, OwnedWriteHalf)> {
        let stream = TcpStream::connect(&self.tcp_addr).await?;
        Ok(stream.into_split())
    }
}

/// QUIC connector for opening streams on existing QUIC connection
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
    async fn open_new_stream(&self) -> Result<(ReceiveStream, SendStream)> {
        let stream = self.handle.clone().open_bidirectional_stream().await?;
        Ok(stream.split())
    }
}

/// Memory connector for in-process communication via channel
#[derive(Clone)]
pub struct MemoryConnector {
    sender: UnboundedSender<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)>,
    max_buf_size: usize,
}

impl MemoryConnector {
    pub fn new(
        sender: UnboundedSender<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)>,
        max_buf_size: usize,
    ) -> Self {
        Self {
            sender,
            max_buf_size,
        }
    }
}

#[async_trait::async_trait]
impl Connector<ReadHalf<SimplexStream>, WriteHalf<SimplexStream>> for MemoryConnector {
    async fn open_new_stream(&self) -> Result<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)> {
        let (r1, w1) = simplex(self.max_buf_size);
        let (r2, w2) = simplex(self.max_buf_size);

        self.sender.send((r1, w2))?;

        Ok((r2, w1))
    }
}
