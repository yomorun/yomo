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
    io::{AsyncWriteExt, ReadHalf, SimplexStream, WriteHalf},
    spawn,
    sync::{Mutex, RwLock, mpsc::UnboundedReceiver},
};

use crate::{
    auth::{Auth, AuthImpl},
    bridge::Bridge,
    connector::QuicConnector,
    io::{receive_frame, send_frame},
    metadata::{Metadata, MetadataMgr, MetadataMgrImpl},
    router::{Router, RouterImpl},
    tls::{TlsConfig, new_tls},
    tool_mgr::{ToolMgr, ToolMgrImpl},
    types::{HandshakeRequest, HandshakeResponse, RequestHeaders},
};

/// Zipper: Manages all registered Tool connections
#[derive(Clone, Builder)]
pub struct Zipper {
    #[builder(default = Arc::new(AuthImpl::new(None)))]
    auth: Arc<dyn Auth>,

    #[builder(default = Arc::new(MetadataMgrImpl::new()))]
    metadata_mgr: Arc<dyn MetadataMgr>,

    #[builder(default = Arc::new(RouterImpl::new()))]
    router: Arc<dyn Router>,

    #[builder(default = Arc::new(ToolMgrImpl::new()))]
    tool_mgr: Arc<dyn ToolMgr>,

    #[builder(default = Arc::default())]
    all_conns: Arc<RwLock<HashMap<u64, Handle>>>,
}

impl Zipper {
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

        if let Some(stream) = conn.accept_bidirectional_stream().await? {
            // Handshake: get client name
            let client_name = self.handle_handshake(conn_id, stream).await?;

            info!("new client connected: client_name={}", client_name);

            // save connection
            self.all_conns.write().await.insert(conn_id, conn.handle());
        } else {
            info!("conn closed: {}", conn_id);
            return Ok(());
        }

        // receive streams and forward, keep connection alive
        let quic_bridge = ZipperQuicBridge::new(self.clone(), conn);
        quic_bridge.serve_bridge().await;
        info!("conn closed: {}", conn_id);

        // Clean up registration
        self.router.remove(conn_id).await;
        self.all_conns.write().await.remove(&conn_id);

        Ok(())
    }

    /// Handle handshake protocol: read client name
    async fn handle_handshake(
        &self,
        conn_id: u64,
        mut stream: BidirectionalStream,
    ) -> Result<String> {
        let req = receive_frame::<HandshakeRequest>(&mut stream)
            .await?
            .ok_or(anyhow!("receive handshake request failed"))?;

        match self.handle_handshake_request(conn_id, &req).await {
            Ok((metadata, existed_conn)) => {
                if let Some(json_schema) = req.json_schema {
                    self.tool_mgr
                        .upsert_tool(req.name.to_owned(), json_schema, &metadata)
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

                Ok(req.name)
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
    ) -> Result<(Metadata, Option<u64>)> {
        let auth_info = self.auth.authenticate(&req.credential).await?;
        let metadata = self.metadata_mgr.new_from_auth_info(&auth_info)?;
        let existed_conn = self.router.register(conn_id, &req.name, &metadata).await?;
        Ok((metadata, existed_conn))
    }

    async fn route(&self, name: &str, metadata: &Metadata) -> Result<Option<QuicConnector>> {
        if let Some(conn_id) = self.router.route(&name, metadata).await? {
            if let Some(conn) = self.all_conns.read().await.get(&conn_id) {
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
        let headers = headers.as_ref().ok_or(anyhow!("no headers"))?;
        let metadata = self
            .zipper
            .metadata_mgr
            .new_from_extension(&headers.extension)?;
        self.zipper.route(&headers.name, &metadata).await
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
        let headers = headers.as_ref().ok_or(anyhow!("no headers"))?;
        let metadata = self
            .zipper
            .metadata_mgr
            .new_from_extension(&headers.extension)?;
        self.zipper.route(&headers.name, &metadata).await
    }
}
