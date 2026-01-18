use anyhow::{Context, Result};
use log::{error, info};
use s2n_quic::{
    Connection, Server,
    connection::{self, Handle},
    provider::{io::TryInto, limits::Limits},
    stream::BidirectionalStream,
};
use std::{collections::HashMap, sync::Arc, time::Duration};
use tokio::{
    io::{ReadHalf, SimplexStream, WriteHalf},
    spawn,
    sync::{Mutex, RwLock},
};

use crate::{
    bridge::Bridge,
    handshake::{HandshakeReq, HandshakeRes},
    io::{pipe_stream, receive_all, send_all},
    metadata::Metadata,
    tls::{TlsConfig, new_server_tls},
    zipper::{config::ZipperConfig, middleware::ZipperMiddleware},
};

// Zipper: Manages all registered Sfn connections
#[derive(Clone)]
pub struct Zipper {
    host: String,

    port: u16,

    tls: TlsConfig,

    middleware: Arc<RwLock<dyn ZipperMiddleware>>,

    all_sfns: Arc<Mutex<HashMap<u64, Handle>>>,
}

impl Zipper {
    pub fn new(config: ZipperConfig, middleware: impl ZipperMiddleware + 'static) -> Self {
        Self {
            host: config.host,
            port: config.port,
            tls: config.tls,
            middleware: Arc::new(RwLock::new(middleware)),
            all_sfns: Arc::default(),
        }
    }

    // Start server: listen on QUIC port, accept remote Sfn connections
    pub async fn serve(self) -> Result<()> {
        let tls = new_server_tls(&self.tls).context("failed to load tls certificates")?;

        let limits = Limits::new()
            .with_max_handshake_duration(Duration::from_secs(10))?
            .with_max_idle_timeout(Duration::from_secs(15))?
            .with_max_open_local_bidirectional_streams(200)?
            .with_max_open_local_unidirectional_streams(0)?
            .with_max_open_remote_bidirectional_streams(1)?
            .with_max_open_remote_unidirectional_streams(0)?;

        let mut server = Server::builder()
            .with_tls(tls)?
            .with_io(TryInto::try_into((self.host.as_str(), self.port))?)?
            .with_limits(limits)?
            .start()?;

        info!("start quic server: {}:{}/udp", self.host, self.port);

        // Start independent handling task for each connection
        while let Some(conn) = server.accept().await {
            let zipper = self.clone();
            spawn(async move {
                if let Err(e) = zipper.handle_connection(conn).await {
                    error!("Connection error: {}", e);
                }
            });
        }

        Ok(())
    }

    // Handle QUIC connection: register Sfn
    async fn handle_connection(self, mut conn: Connection) -> Result<()> {
        let conn_id = conn.id();

        // save connection
        self.all_sfns.lock().await.insert(conn_id, conn.handle());

        if let Some(stream) = conn.accept_bidirectional_stream().await? {
            // Handshake: get sfn name
            let sfn_name = self.handle_handshake(conn_id, stream).await?;
            info!("new sfn connection {}: sfn_name={:?}", conn_id, sfn_name);
        } else {
            return Ok(());
        }

        // Keep connection alive
        loop {
            match conn.accept_bidirectional_stream().await {
                Ok(stream) => {
                    if let Some(mut stream) = stream {
                        // this should never happen
                        stream.close().await.ok();
                    }
                }
                Err(e) => {
                    if let connection::Error::Application { error, .. } = e {
                        info!("conn closed with error_code: {}", u64::from(*error));
                        return Ok(());
                    }

                    error!("accept_bidirectional_stream error: {}", e);
                    break;
                }
            }
        }

        // Clean up sfn registration
        self.middleware.write().await.remove_sfn(conn_id)?;
        self.all_sfns.lock().await.remove(&conn_id);

        Ok(())
    }

    // Handshake protocol: read Sfn name
    async fn handle_handshake(&self, conn_id: u64, stream: BidirectionalStream) -> Result<String> {
        let (recv_stream, send_stream) = stream.split();
        let req: HandshakeReq = receive_all(recv_stream).await?;

        let ok =
            match self
                .middleware
                .write()
                .await
                .handshake(conn_id, &req.sfn_name, req.credential)
            {
                Ok(exsited_conn_id) => {
                    if let Some(conn_id) = exsited_conn_id {
                        if let Some(conn) = self.all_sfns.lock().await.remove(&conn_id) {
                            info!("close existing connection: {}", conn_id);
                            conn.close(1_u32.into());
                        }
                    }
                    true
                }
                Err(e) => {
                    error!("handshake error: {}", e);
                    false
                }
            };

        let res = HandshakeRes { ok };
        send_all(send_stream, &res).await?;

        Ok(req.sfn_name)
    }
}

#[async_trait::async_trait]
impl Bridge for Zipper {
    // Forward stream to corresponding sfn
    async fn forward(
        &self,
        sfn_name: &str,
        metadata: &Box<dyn Metadata>,
        from_reader: ReadHalf<SimplexStream>,
        from_writer: WriteHalf<SimplexStream>,
    ) -> Result<bool> {
        if let Some(conn_id) = self.middleware.read().await.route(&sfn_name, &metadata)? {
            if let Some(conn) = self.all_sfns.lock().await.get(&conn_id) {
                info!(
                    "[{}|{}] proxy to sfn: {}",
                    metadata.trace_id(),
                    metadata.req_id(),
                    conn_id
                );

                // Create new QUIC stream upon the sfn connection
                let mut conn = conn.clone();
                let quic_stream = conn.open_bidirectional_stream().await?;
                let (to_reader, to_writer) = quic_stream.split();

                // Proxy data between the original streams and the QUIC stream
                spawn(async move {
                    if let Err(e) =
                        pipe_stream(from_reader, from_writer, to_reader, to_writer).await
                    {
                        error!("proxy stream error: {}", e);
                    }
                });

                return Ok(true);
            }
        }

        Ok(false)
    }
}
