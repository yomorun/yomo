use std::{collections::HashMap, sync::Arc, time::Duration};

use anyhow::{Result, anyhow};
use axum::http::StatusCode;
use bon::Builder;
use log::{error, info};
use s2n_quic::{
    Connection, Server,
    connection::Handle,
    provider::{io::TryInto, limits::Limits},
    stream::{BidirectionalStream, ReceiveStream, SendStream},
};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt, ReadHalf, SimplexStream, WriteHalf},
    spawn,
    sync::{Mutex, RwLock, mpsc::UnboundedReceiver},
};

use crate::{
    auth::Auth,
    bridge::Bridge,
    connector::QuicConnector,
    io::{receive_frame, send_frame},
    metadata_mgr::MetadataMgr,
    router::Router,
    tls::{TlsConfig, new_tls},
    tool_mgr::ToolMgr,
    types::{HandshakeRequest, HandshakeResponse, RequestHeaders},
};

/// Zipper: Manages all registered Tool connections
#[derive(Clone, Builder)]
pub struct Zipper<A, M>
where
    A: Clone + Send + Sync + 'static,
    M: Clone + Send + Sync + 'static,
{
    auth: Arc<dyn Auth<A>>,

    metadata_mgr: Arc<dyn MetadataMgr<A, M>>,

    router: Arc<dyn Router<A, M>>,

    tool_mgr: Arc<dyn ToolMgr<A, M>>,

    #[builder(default = Arc::default())]
    all_conns: Arc<RwLock<HashMap<u64, Handle>>>,
}

impl<A, M> Zipper<A, M>
where
    A: Clone + Send + Sync + 'static,
    M: Clone + Send + Sync + 'static,
{
    /// Start QUIC server: listen for remote Tool connections
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

        info!("start zipper quic server: {}:{}/udp", host, port);
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

    /// Handle QUIC connection: register Tool
    async fn handle_connection(self, mut conn: Connection) -> Result<()> {
        let conn_id = conn.id();
        info!("new quic connection: {}", conn_id);

        let Some(stream) = conn.accept_bidirectional_stream().await? else {
            info!("conn closed: {}", conn_id);
            return Ok(());
        };

        // Handshake: get client name
        let (auth_info, client_name) = self.handle_handshake(conn_id, stream).await?;

        info!("new client connected: client_name={}", client_name);

        // save connection
        self.all_conns.write().await.insert(conn_id, conn.handle());

        // receive streams and forward, keep connection alive
        let quic_bridge = ZipperBridge::new(self.clone(), QuicSource::new(conn), auth_info);
        quic_bridge.serve_bridge().await;
        info!("conn closed: {}", conn_id);

        // Clean up registration
        self.router.remove(conn_id).await;
        self.all_conns.write().await.remove(&conn_id);

        Ok(())
    }

    /// Handles the handshake stream for a new tool connection.
    ///
    /// Successful handshakes may register or replace existing routes and can
    /// also persist the tool schema in `ToolMgr`.
    async fn handle_handshake(
        &self,
        conn_id: u64,
        mut stream: BidirectionalStream,
    ) -> Result<(A, String)> {
        let req = receive_frame::<HandshakeRequest>(&mut stream)
            .await?
            .ok_or(anyhow!("receive handshake request failed"))?;

        match self.handle_handshake_request(conn_id, &req).await {
            Ok((auth_info, existed_conn)) => {
                if let Some(json_schema) = req.json_schema {
                    self.tool_mgr
                        .upsert_tool(req.name.to_owned(), json_schema, &auth_info)
                        .await?;
                }

                let res = HandshakeResponse {
                    status_code: StatusCode::OK.as_u16(),
                    ..Default::default()
                };
                send_frame(&mut stream, &res).await?;
                stream.shutdown().await?;

                if let Some(conn_id) = existed_conn {
                    if let Some(conn) = self.all_conns.write().await.remove(&conn_id) {
                        info!(
                            "close existing connection {} for client_name: {}",
                            conn_id, req.name
                        );
                        conn.close(1_u32.into());
                    }
                }

                Ok((auth_info, req.name))
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

    async fn handle_handshake_request(
        &self,
        conn_id: u64,
        req: &HandshakeRequest,
    ) -> Result<(A, Option<u64>)> {
        let auth_info = self.auth.authenticate(&req.credential).await?;
        let existed_conn = self.router.register(conn_id, &req.name, &auth_info).await?;
        Ok((auth_info, existed_conn))
    }

    async fn route(&self, name: &str, metadata: &M) -> Result<Option<QuicConnector>> {
        if let Some(conn_id) = self.router.route(&name, metadata).await? {
            if let Some(conn) = self.all_conns.read().await.get(&conn_id) {
                return Ok(Some(QuicConnector::new(conn.clone())));
            }
        }

        Ok(None)
    }
}

#[async_trait::async_trait]
pub trait UpstreamSource: Clone + Send + Sync + 'static {
    type R: AsyncReadExt + Unpin + Send + 'static;
    type W: AsyncWriteExt + Unpin + Send + 'static;

    async fn accept(&self) -> Result<Option<(Self::R, Self::W)>>;
}

#[derive(Clone)]
pub struct MemorySource {
    receiver: Arc<Mutex<UnboundedReceiver<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)>>>,
}

impl MemorySource {
    pub fn new(
        receiver: UnboundedReceiver<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)>,
    ) -> Self {
        Self {
            receiver: Arc::new(Mutex::new(receiver)),
        }
    }
}

#[async_trait::async_trait]
impl UpstreamSource for MemorySource {
    type R = ReadHalf<SimplexStream>;
    type W = WriteHalf<SimplexStream>;

    async fn accept(&self) -> Result<Option<(Self::R, Self::W)>> {
        Ok(self.receiver.lock().await.recv().await)
    }
}

#[derive(Clone)]
struct QuicSource {
    conn: Arc<Mutex<Connection>>,
}

impl QuicSource {
    fn new(conn: Connection) -> Self {
        Self {
            conn: Arc::new(Mutex::new(conn)),
        }
    }
}

#[async_trait::async_trait]
impl UpstreamSource for QuicSource {
    type R = ReceiveStream;
    type W = SendStream;

    async fn accept(&self) -> Result<Option<(Self::R, Self::W)>> {
        Ok(self
            .conn
            .lock()
            .await
            .accept_bidirectional_stream()
            .await?
            .map(|stream| stream.split()))
    }
}

#[derive(Clone)]
pub struct ZipperBridge<S, A, M>
where
    S: UpstreamSource,
    A: Clone + Send + Sync + 'static,
    M: Clone + Send + Sync + 'static,
{
    zipper: Zipper<A, M>,
    source: S,
    auth_info: A,
}

impl<S, A, M> ZipperBridge<S, A, M>
where
    S: UpstreamSource,
    A: Clone + Send + Sync + 'static,
    M: Clone + Send + Sync + 'static,
{
    pub fn new(zipper: Zipper<A, M>, source: S, auth_info: A) -> Self {
        Self {
            zipper,
            source,
            auth_info,
        }
    }
}

#[async_trait::async_trait]
impl<S, A, M> Bridge<QuicConnector, S::R, S::W, ReceiveStream, SendStream> for ZipperBridge<S, A, M>
where
    S: UpstreamSource,
    A: Clone + Send + Sync + 'static,
    M: Clone + Send + Sync + 'static,
{
    async fn accept(&mut self) -> Result<Option<(S::R, S::W)>> {
        self.source.accept().await
    }

    fn skip_headers(&self) -> bool {
        false
    }

    async fn find_downstream(
        &self,
        headers: &Option<RequestHeaders>,
    ) -> Result<Option<QuicConnector>> {
        let headers = headers.as_ref().ok_or(anyhow!("headers cannot be empty"))?;
        let metadata = self
            .zipper
            .metadata_mgr
            .new_from_extension(&self.auth_info, &headers.extension)?;
        self.zipper.route(&headers.name, &metadata).await
    }
}
