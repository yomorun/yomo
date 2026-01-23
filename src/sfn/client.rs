use std::{net::ToSocketAddrs, sync::Arc, time::Duration};

use anyhow::{Result, anyhow, bail};
use bon::Builder;
use log::{debug, error, info};
use s2n_quic::{
    Client, Connection, client::Connect, connection, provider::limits::Limits,
    stream::BidirectionalStream,
};
use tokio::{io::AsyncWriteExt as _, spawn};

use crate::{
    bridge::Bridge,
    io::{receive_frame, send_frame},
    sfn::serverless::ServerlessHandler,
    tls::{TlsConfig, new_client_tls},
    types::{HandshakeReq, HandshakeRes, RequestHeaders},
};

#[derive(Clone, Builder)]
pub struct Sfn {
    sfn_name: String,

    handler: Arc<ServerlessHandler>,
}

impl Sfn {
    pub async fn run(
        self,
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
            .with_max_open_local_bidirectional_streams(1)?
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
        let server_port: u16 = server_port.parse()?;
        let addr = (server_name, server_port)
            .to_socket_addrs()?
            .next()
            .ok_or_else(|| anyhow!("no zipper ip found"))?;
        let mut conn = client
            .connect(Connect::new(addr).with_server_name(server_name))
            .await?;
        conn.keep_alive(true)?;
        info!("quic connected");

        self.process(conn, &credential).await
    }

    // process QUIC connection
    async fn process(&self, mut conn: Connection, credential: &str) -> Result<()> {
        // Send handshake request
        self.handshake(&mut conn, credential).await?;

        // Accept and process data streams (zipper creates new streams for each request)
        loop {
            match conn.accept_bidirectional_stream().await {
                Ok(stream) => {
                    if let Some(stream) = stream {
                        let sfn = self.clone();
                        spawn(async move {
                            let stream_id = stream.id();
                            debug!("stream ++: {}", stream_id);

                            if let Err(e) = sfn.handle_stream(stream).await {
                                error!("handle stream error: {}", e);
                            }

                            debug!("stream --: {}", stream_id);
                        });
                    } else {
                        info!("conn closed");
                        return Ok(());
                    }
                }
                Err(e) => {
                    if let connection::Error::Application { error, .. } = e {
                        info!("conn closed with error_code: {}", u64::from(*error));
                        return Ok(());
                    }
                    bail!("accept_bidirectional_stream error: {}", e);
                }
            }
        }
    }

    // Send handshake request
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

    // Handle data stream: receive request, execute processing, return response
    async fn handle_stream(&self, stream: BidirectionalStream) -> Result<()> {
        let (r1, w1) = stream.split();

        let headers = RequestHeaders::default();

        self.handler.forward(&headers, r1, w1).await?;

        Ok(())
    }
}
