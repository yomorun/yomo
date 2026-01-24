use std::{net::ToSocketAddrs, sync::Arc, time::Duration};

use anyhow::{Result, anyhow, bail};
use log::{debug, info};
use s2n_quic::{
    Client, Connection,
    client::Connect,
    provider::limits::Limits,
    stream::{ReceiveStream, SendStream},
};
use tokio::{
    io::AsyncWriteExt,
    net::tcp::{OwnedReadHalf, OwnedWriteHalf},
    sync::Mutex,
};

use crate::{
    bridge::Bridge,
    connector::TcpConnector,
    io::{receive_frame, send_frame},
    sfn::serverless::ServerlessHandler,
    tls::{TlsConfig, new_client_tls},
    types::{HandshakeReq, HandshakeRes, RequestHeaders},
};

/// Serverless Function (SFN) client
#[derive(Clone)]
pub struct Sfn {
    sfn_name: String,

    quic_conn: Option<Arc<Mutex<Connection>>>,

    serverless_handler: ServerlessHandler,
}

impl Sfn {
    pub fn new(sfn_name: String, serverless_handler: ServerlessHandler) -> Self {
        Self {
            sfn_name,
            quic_conn: None,
            serverless_handler,
        }
    }
}

impl Sfn {
    /// Connect to Zipper service
    pub async fn connect_zipper(
        &mut self,
        zipper: &str,
        credential: &str,
        tls_config: &TlsConfig,
        tls_insecure: bool,
    ) -> Result<()> {
        info!("start sfn: {}", self.sfn_name);

        let tls = new_client_tls(tls_config, tls_insecure)?;

        let limits = Limits::new()
            .with_max_handshake_duration(Duration::from_secs(10))?
            .with_max_idle_timeout(Duration::from_secs(10))?
            .with_max_open_local_bidirectional_streams(200)?
            .with_max_open_local_unidirectional_streams(0)?
            .with_max_open_remote_bidirectional_streams(200)?
            .with_max_open_remote_unidirectional_streams(0)?;

        let client = Client::builder()
            .with_tls(tls)?
            .with_io("0.0.0.0:0")?
            .with_limits(limits)?
            .start()?;

        // Connect to zipper service
        let (server_name, server_port) = zipper
            .split_once(':')
            .ok_or_else(|| anyhow!("invalid zipper addr format"))?;
        debug!("server_name: {}, server_port: {}", server_name, server_port);

        let server_port: u16 = server_port.parse()?;
        let addr = (server_name, server_port)
            .to_socket_addrs()?
            .next()
            .ok_or_else(|| anyhow!("no zipper ip found"))?;
        debug!("zipper socket addr: {}", addr);

        let mut conn = client
            .connect(Connect::new(addr).with_server_name(server_name))
            .await?;
        conn.keep_alive(true)?;
        info!("connected to zipper: {}/udp", addr);

        // Send handshake request
        self.handshake(&mut conn, credential).await?;

        self.quic_conn = Some(Arc::new(Mutex::new(conn)));

        Ok(())
    }

    /// Send handshake request to Zipper
    async fn handshake(&self, conn: &mut Connection, credential: &str) -> Result<()> {
        let mut stream = conn.open_bidirectional_stream().await?;

        let req = HandshakeReq {
            sfn_name: self.sfn_name.to_owned(),
            credential: credential.to_owned(),
        };

        send_frame(&mut stream, &req).await?;
        stream.shutdown().await?;

        let res = receive_frame::<HandshakeRes>(&mut stream)
            .await?
            .ok_or(anyhow!("receive handshake response failed"))?;

        if !res.ok {
            bail!("handshake failed: {}", res.reason);
        }

        Ok(())
    }
}

#[async_trait::async_trait]
impl Bridge<TcpConnector, ReceiveStream, SendStream, OwnedReadHalf, OwnedWriteHalf> for Sfn {
    async fn accept(&mut self) -> Result<Option<(ReceiveStream, SendStream)>> {
        if let Some(conn) = &self.quic_conn {
            if let Some(stream) = conn.lock().await.accept_bidirectional_stream().await? {
                debug!("new quic stream: {}", stream.id());

                return Ok(Some(stream.split()));
            }
        }

        Ok(None)
    }

    async fn find_downstream(&self, _headers: &RequestHeaders) -> Result<Option<TcpConnector>> {
        self.serverless_handler.get_connector().await
    }
}
