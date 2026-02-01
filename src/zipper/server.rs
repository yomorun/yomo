use std::{collections::HashMap, sync::Arc, time::Duration};

use anyhow::{Result, anyhow};
use axum::http::StatusCode;
use log::{error, info};
use s2n_quic::{
    Connection, Server,
    connection::Handle,
    provider::{io::TryInto, limits::Limits},
    stream::{BidirectionalStream, ReceiveStream, SendStream},
};
use tokio::{
    io::{AsyncWriteExt, ReadHalf, SimplexStream, WriteHalf},
    spawn,
    sync::{Mutex, RwLock, mpsc::UnboundedReceiver},
};

use crate::{
    bridge::Bridge,
    connector::QuicConnector,
    io::{receive_frame, send_frame},
    tls::{TlsConfig, new_tls},
    types::{HandshakeRequest, HandshakeResponse, RequestHeaders},
    zipper::router::Router,
};

/// Zipper: Manages all registered SFN connections
#[derive(Clone)]
pub struct Zipper {
    router: Arc<RwLock<dyn Router>>,

    all_sfns: Arc<RwLock<HashMap<u64, Handle>>>,
}

impl Zipper {
    pub fn new(router: impl Router + 'static) -> Self {
        Self {
            router: Arc::new(RwLock::new(router)),
            all_sfns: Arc::default(),
        }
    }

    /// Start QUIC server: listen for remote SFN connections
    pub async fn serve(&self, host: &str, port: u16, tls_config: &TlsConfig) -> Result<()> {
        // todo: configurable
        let limits = Limits::new()
            .with_max_handshake_duration(Duration::from_secs(10))?
            .with_max_idle_timeout(Duration::from_secs(10))?
            .with_max_keep_alive_period(Duration::from_secs(5))?
            .with_max_active_connection_ids(2000)?
            .with_max_open_local_bidirectional_streams(1000)?
            .with_max_open_local_unidirectional_streams(0)?
            .with_max_open_remote_bidirectional_streams(1000)?
            .with_max_open_remote_unidirectional_streams(0)?;

        let mut server = Server::builder()
            .with_tls(new_tls(tls_config, true).await?)?
            .with_io(TryInto::try_into((host, port))?)?
            .with_limits(limits)?
            .start()?;

        info!("start quic server: {}:{}/udp", host, port);

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

    /// Handle QUIC connection: register SFN
    async fn handle_connection(self, mut conn: Connection) -> Result<()> {
        let conn_id = conn.id();
        info!("new quic connection: {}", conn_id);

        if let Some(stream) = conn.accept_bidirectional_stream().await? {
            // Handshake: get sfn name
            let sfn_name = self.handle_handshake(conn_id, stream).await?;

            info!("new sfn connected: sfn_name={}", sfn_name);

            // save connection
            self.all_sfns.write().await.insert(conn_id, conn.handle());
        } else {
            info!("conn closed: {}", conn_id);
            return Ok(());
        }

        // receive streams and forward, keep connection alive
        let quic_bridge = ZipperQuicBridge::new(self.clone(), conn);
        quic_bridge.serve_bridge().await;
        info!("conn closed: {}", conn_id);

        // Clean up sfn registration
        self.router.write().await.remove_sfn(conn_id);
        self.all_sfns.write().await.remove(&conn_id);

        Ok(())
    }

    /// Handle handshake protocol: read SFN name
    async fn handle_handshake(
        &self,
        conn_id: u64,
        mut stream: BidirectionalStream,
    ) -> Result<String> {
        let req = receive_frame::<HandshakeRequest>(&mut stream)
            .await?
            .ok_or(anyhow!("receive handshake request failed"))?;

        match self.router.write().await.handshake(conn_id, &req) {
            Ok(existed_conn) => {
                let res = HandshakeResponse {
                    status_code: StatusCode::OK.as_u16(),
                    ..Default::default()
                };
                send_frame(&mut stream, &res).await?;
                stream.shutdown().await?;

                if let Some(conn_id) = existed_conn {
                    if let Some(conn) = self.all_sfns.write().await.remove(&conn_id) {
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

                let res = HandshakeResponse {
                    status_code: StatusCode::UNAUTHORIZED.as_u16(),
                    error_msg: e.to_string(),
                };
                send_frame(&mut stream, &res).await?;
                stream.shutdown().await?;

                Err(anyhow!("handshake failed: {}", e))
            }
        }
    }

    async fn route(&self, headers: &RequestHeaders) -> Result<Option<QuicConnector>> {
        if let Some(conn_id) = self.router.read().await.route(&headers)? {
            if let Some(conn) = self.all_sfns.read().await.get(&conn_id) {
                return Ok(Some(QuicConnector::new(conn.clone())));
            }
        }

        Ok(None)
    }
}

#[derive(Clone)]
pub struct ZipperMemoryBridge {
    zipper: Zipper,

    receiver: Arc<Mutex<UnboundedReceiver<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)>>>,
}

impl ZipperMemoryBridge {
    pub fn new(
        zipper: Zipper,
        receiver: UnboundedReceiver<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)>,
    ) -> Self {
        Self {
            zipper,
            receiver: Arc::new(Mutex::new(receiver)),
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
    > for ZipperMemoryBridge
{
    async fn accept(
        &mut self,
    ) -> Result<Option<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)>> {
        Ok(self.receiver.lock().await.recv().await)
    }

    fn skip_headers(&self) -> bool {
        false
    }

    async fn find_downstream(
        &self,
        headers: &Option<RequestHeaders>,
    ) -> Result<Option<QuicConnector>> {
        self.zipper
            .route(headers.as_ref().ok_or(anyhow!("no headers"))?)
            .await
    }
}

#[derive(Clone)]
struct ZipperQuicBridge {
    zipper: Zipper,
    conn: Arc<Mutex<Connection>>,
}

impl ZipperQuicBridge {
    pub fn new(zipper: Zipper, conn: Connection) -> Self {
        Self {
            zipper,
            conn: Arc::new(Mutex::new(conn)),
        }
    }
}

#[async_trait::async_trait]
impl Bridge<QuicConnector, ReceiveStream, SendStream, ReceiveStream, SendStream>
    for ZipperQuicBridge
{
    async fn accept(&mut self) -> Result<Option<(ReceiveStream, SendStream)>> {
        Ok(self
            .conn
            .lock()
            .await
            .accept_bidirectional_stream()
            .await?
            .map(|stream| stream.split()))
    }

    fn skip_headers(&self) -> bool {
        false
    }

    async fn find_downstream(
        &self,
        headers: &Option<RequestHeaders>,
    ) -> Result<Option<QuicConnector>> {
        self.zipper
            .route(headers.as_ref().ok_or(anyhow!("no headers"))?)
            .await
    }
}
