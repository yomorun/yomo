use anyhow::{Context, Result, anyhow};
use log::{error, info};
use s2n_quic::{
    Connection, Server,
    connection::{self, Handle},
    provider::{io::TryInto, limits::Limits},
    stream::{BidirectionalStream, ReceiveStream, SendStream},
};
use std::{collections::HashMap, sync::Arc, time::Duration};
use tokio::{
    io::{AsyncWriteExt, ReadHalf, SimplexStream, WriteHalf},
    spawn,
    sync::{Mutex, RwLock, mpsc::UnboundedReceiver},
};

use crate::{
    bridge::Bridge,
    connector::QuicConnector,
    io::{receive_frame, send_frame},
    tls::{TlsConfig, new_server_tls},
    types::{HandshakeReq, HandshakeRes, RequestHeaders},
    zipper::router::Router,
};

// Zipper: Manages all registered sfn connections
#[derive(Clone)]
pub struct Zipper {
    router: Arc<RwLock<dyn Router>>,

    all_sfns: Arc<Mutex<HashMap<u64, Handle>>>,

    receiver: Arc<Mutex<UnboundedReceiver<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)>>>,
}

impl Zipper {
    pub fn new(
        router: impl Router + 'static,
        receiver: UnboundedReceiver<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)>,
    ) -> Self {
        Self {
            router: Arc::new(RwLock::new(router)),
            all_sfns: Arc::default(),
            receiver: Arc::new(Mutex::new(receiver)),
        }
    }

    // Start server: listen on QUIC port, accept remote sfn connections
    pub async fn listen_for_quic(
        &self,
        host: &str,
        port: u16,
        tls_config: &TlsConfig,
    ) -> Result<()> {
        let tls = new_server_tls(tls_config).context("failed to load tls certificates")?;

        let limits = Limits::new()
            .with_max_handshake_duration(Duration::from_secs(10))?
            .with_max_idle_timeout(Duration::from_secs(15))?
            .with_max_open_local_bidirectional_streams(200)?
            .with_max_open_local_unidirectional_streams(0)?
            .with_max_open_remote_bidirectional_streams(1)?
            .with_max_open_remote_unidirectional_streams(0)?;

        let mut server = Server::builder()
            .with_tls(tls)?
            .with_io(TryInto::try_into((host, port))?)?
            .with_limits(limits)?
            .start()?;

        info!("start quic server: {}:{}/udp", host, port);

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

    // Handle QUIC connection: register sfn
    async fn handle_connection(self, mut conn: Connection) -> Result<()> {
        let conn_id = conn.id();
        info!("new quic connection: {}", conn_id);

        if let Some(stream) = conn.accept_bidirectional_stream().await? {
            // Handshake: get sfn name
            let sfn_name = self.handle_handshake(conn_id, stream).await?;

            info!("new sfn connected: sfn_name={}", sfn_name);

            // save connection
            self.all_sfns.lock().await.insert(conn_id, conn.handle());
        } else {
            info!("conn closed: {}", conn_id);
            return Ok(());
        }

        // Keep connection alive
        loop {
            match conn.accept_bidirectional_stream().await {
                Ok(stream) => {
                    if let Some(mut stream) = stream {
                        // this should never happen
                        stream.close().await.ok();
                    } else {
                        self.all_sfns.lock().await.remove(&conn_id);
                        info!("conn closed: {}", conn_id);
                        return Ok(());
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
        self.router.write().await.remove_sfn(conn_id)?;
        self.all_sfns.lock().await.remove(&conn_id);

        Ok(())
    }

    // Handshake protocol: read sfn name
    async fn handle_handshake(
        &self,
        conn_id: u64,
        mut stream: BidirectionalStream,
    ) -> Result<String> {
        let req = receive_frame::<HandshakeReq>(&mut stream)
            .await?
            .ok_or(anyhow!("receive handshake request failed"))?;

        match self.router.write().await.handshake(conn_id, &req) {
            Ok(existed_conn) => {
                let res = HandshakeRes {
                    ok: true,
                    ..Default::default()
                };
                send_frame(&mut stream, &res).await?;
                stream.shutdown().await?;

                if let Some(conn_id) = existed_conn {
                    if let Some(conn) = self.all_sfns.lock().await.remove(&conn_id) {
                        info!(
                            "close existing connection {} for sfn_name: {}",
                            conn_id, req.sfn_name
                        );
                        conn.close(1_u32.into());
                    }
                }

                Ok(req.sfn_name)
            }
            Err(e) => {
                error!("handshake failed: {}", e);

                let res = HandshakeRes {
                    ok: false,
                    reason: e.to_string(),
                };
                send_frame(&mut stream, &res).await?;
                stream.shutdown().await?;

                Err(anyhow!("handshake failed: {}", e))
            }
        }
    }
}

#[async_trait::async_trait]
impl
    Bridge<
        QuicConnector,
        ReadHalf<SimplexStream>,
        WriteHalf<SimplexStream>,
        ReceiveStream,
        SendStream,
    > for Zipper
{
    async fn accept(
        &mut self,
    ) -> Result<Option<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)>> {
        Ok(self.receiver.lock().await.recv().await)
    }

    async fn find_downstream(&self, headers: &RequestHeaders) -> Result<Option<QuicConnector>> {
        if let Some(conn_id) = self.router.read().await.route(&headers)? {
            if let Some(conn) = self.all_sfns.lock().await.get(&conn_id) {
                info!(
                    "[{}|{}] proxy to sfn: {}",
                    headers.trace_id, headers.request_id, conn_id
                );

                return Ok(Some(QuicConnector::new(conn.clone())));
            }
        }

        Ok(None)
    }
}
