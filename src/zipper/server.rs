use anyhow::{Context, bail};
use axum::{
    Router,
    extract::{Json, Path, State},
    http::{HeaderMap, StatusCode},
    routing::post,
};
use log::{debug, error, info};
use s2n_quic::{
    Connection, Server,
    provider::{io::TryInto, limits::Limits},
    stream::BidirectionalStream,
};
use serde::Deserialize;
use serde_json::{Value, json};
use std::{collections::HashMap, sync::Arc, time::Duration};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt, SimplexStream, WriteHalf, copy, simplex},
    net::TcpListener,
    select, spawn,
    sync::{
        Mutex, RwLock,
        mpsc::{UnboundedReceiver, UnboundedSender, unbounded_channel},
    },
};

use crate::{
    frame::{Frame, HandshakeAckPayload, read_frame, write_frame},
    metadata::RequestMetadata,
    tls::new_server_tls,
    types::{SfnRequest, SfnResponse},
    zipper::{
        config::{HttpConfig, QuicConfig},
        middleware::Middleware,
    },
};

// Sfn: Function struct used to forward requests to Sfn clients
#[derive(Clone)]
struct Sfn {
    tx: UnboundedSender<(SfnRequest, WriteHalf<SimplexStream>)>,
}

// Zipper: Manages all registered Sfn connections
#[derive(Clone)]
pub struct Zipper {
    middleware: Arc<RwLock<dyn Middleware>>,

    all_sfns: Arc<Mutex<HashMap<u64, Sfn>>>,
}

impl Zipper {
    pub fn new(middleware: impl Middleware + 'static) -> Self {
        Self {
            middleware: Arc::new(RwLock::new(middleware)),
            all_sfns: Arc::default(),
        }
    }
}

impl Zipper {
    // Start server: listen on both HTTP and QUIC ports
    pub async fn serve(
        self,
        quic_config: &QuicConfig,
        http_config: &HttpConfig,
    ) -> anyhow::Result<()> {
        select! {
            r = self.serve_quic(quic_config) => r,
            r = self.serve_http(http_config) => r,
        }
    }

    // HTTP server: listen and receive external requests
    async fn serve_http(&self, config: &HttpConfig) -> anyhow::Result<()> {
        let app = Router::new()
            .route("/sfn/{name}", post(handle_post))
            .with_state(self.clone());

        let listener = TcpListener::bind((config.host.to_owned(), config.port)).await?;

        info!("start http server: {}:{}", config.host, config.port);
        axum::serve(listener, app).await?;

        Ok(())
    }

    // QUIC server: accept remote Sfn connections
    async fn serve_quic(&self, config: &QuicConfig) -> anyhow::Result<()> {
        let tls = new_server_tls(&config.tls).context("failed to load tls certificates")?;

        let limits = Limits::new()
            .with_max_handshake_duration(Duration::from_secs(10))?
            .with_max_idle_timeout(Duration::from_secs(15))?
            .with_max_open_local_bidirectional_streams(200)?
            .with_max_open_local_unidirectional_streams(0)?
            .with_max_open_remote_bidirectional_streams(1)?
            .with_max_open_remote_unidirectional_streams(0)?;

        let mut server = Server::builder()
            .with_tls(tls)?
            .with_io(TryInto::try_into((config.host.as_str(), config.port))?)?
            .with_limits(limits)?
            .start()?;

        info!("start quic server: {}:{}/udp", config.host, config.port);

        // Start independent handling task for each connection
        while let Some(conn) = server.accept().await {
            let zipper = self.clone();
            spawn(async move {
                if let Err(e) = zipper.handle_connection(conn).await {
                    error!("Connection error: {:?}", e);
                }
            });
        }

        Ok(())
    }

    // Forward request to corresponding QUIC Sfn
    async fn proxy_request(
        &self,
        metadata: &RequestMetadata,
        name: &str,
        args: &str,
        context: &str,
    ) -> anyhow::Result<Option<SfnResponse>> {
        match self.middleware.read().await.route(&name, &metadata)? {
            Some(conn_id) => {
                // Create stream and send request through channel
                let sfn = self
                    .all_sfns
                    .lock()
                    .await
                    .get(&conn_id)
                    .ok_or(anyhow::Error::msg("sfn not found"))?
                    .to_owned();

                info!(
                    "[{}|{}] proxy request to sfn: {}",
                    metadata.trace_id, metadata.req_id, conn_id
                );

                // Send request through in-memory pipe
                let sfn_req = SfnRequest {
                    args: args.to_owned(),
                    context: context.to_owned(),
                };

                let mut stream = simplex(1024);
                sfn.tx.send((sfn_req, stream.1))?;

                // Read response and return
                let mut buf = Vec::new();
                stream.0.read_to_end(&mut buf).await?;
                let result = String::from_utf8_lossy(&buf).to_string();

                Ok(Some(SfnResponse { result }))
            }
            None => {
                info!("[{}|{}] sfn not found", metadata.trace_id, metadata.req_id);

                Ok(None)
            }
        }
    }

    // Handle QUIC connection: register Sfn
    async fn handle_connection(self, mut conn: Connection) -> anyhow::Result<()> {
        let conn_id = conn.id();

        // Create channel for inter-goroutine communication
        let (tx, rx) = unbounded_channel();

        // Register Sfn
        self.all_sfns.lock().await.insert(conn_id, Sfn { tx });

        if let Some(mut ctrl_stream) = conn.accept_bidirectional_stream().await? {
            // Handshake: get Sfn name
            let sfn_name = self.handle_handshake(conn_id, &mut ctrl_stream).await?;
            info!("new sfn connection {}: sfn_name={:?}", conn_id, sfn_name);

            // Start task to handle requests from HTTP
            let zipper = self.clone();
            tokio::spawn(async move {
                if let Err(e) = zipper.consume_requests(rx, conn).await {
                    error!("consume_requests error: {}", e);
                }
            });

            // Monitor control stream to keep connection alive
            loop {
                if let Err(e) = read_frame(&mut ctrl_stream).await {
                    error!("read_frame error: {:?}", e);
                    break;
                }
            }

            // Clean up Sfn registration when connection is disconnected
            self.all_sfns.lock().await.remove(&conn_id);
            self.middleware.write().await.remove_sfn(conn_id)?;
        }

        Ok(())
    }

    // Handshake protocol: read Sfn name
    async fn handle_handshake(
        &self,
        conn_id: u64,
        ctrl_stream: &mut BidirectionalStream,
    ) -> anyhow::Result<String> {
        let f = read_frame(ctrl_stream).await?;
        match f {
            Frame::Handshake { payload } => {
                let ack = match self.middleware.write().await.handshake(
                    conn_id,
                    payload.sfn_name.to_owned(),
                    payload.credential,
                    payload.metadata,
                ) {
                    Ok(exsited_conn_id) => {
                        if let Some(conn_id) = exsited_conn_id {
                            self.all_sfns.lock().await.remove(&conn_id);
                        }

                        HandshakeAckPayload {
                            ok: true,
                            ..Default::default()
                        }
                    }
                    Err(e) => HandshakeAckPayload {
                        ok: false,
                        reason: Some(e.to_string()),
                    },
                };

                let ok = ack.ok;
                let f = Frame::HandshakeAck { payload: ack };
                write_frame(ctrl_stream, &f).await?;

                if !ok {
                    bail!("handshake failed");
                }

                Ok(payload.sfn_name)
            }
            _ => bail!("invalid frame"),
        }
    }

    // Consume request queue: forward HTTP requests to remote Sfn via QUIC data streams
    async fn consume_requests(
        self,
        mut rx: UnboundedReceiver<(SfnRequest, WriteHalf<SimplexStream>)>,
        mut conn: Connection,
    ) -> anyhow::Result<()> {
        while let Some((sfn_req, mut writer)) = rx.recv().await {
            // Open new data stream
            let stream = conn.open_bidirectional_stream().await?;

            // Handle request asynchronously
            let zipper = self.clone();
            spawn(async move {
                if let Err(e) = zipper
                    .handle_data_stream(stream, sfn_req, &mut writer)
                    .await
                {
                    error!("handle_data_stream error: {:?}", e);
                }
                writer.shutdown().await.ok();
            });
        }

        // Close connection when channel is closed
        info!("conn {} closed", conn.id());
        conn.close(1_u32.into());

        Ok(())
    }

    // Handle data stream: send request parameters and forward response
    async fn handle_data_stream(
        self,
        stream: BidirectionalStream,
        sfn_req: SfnRequest,
        writer: &mut WriteHalf<SimplexStream>,
    ) -> anyhow::Result<()> {
        let (mut receive_stream, mut send_stream) = stream.split();

        // Send request parameters
        let buf = serde_json::to_vec(&sfn_req)?;
        send_stream.write_all(&buf).await?;
        send_stream.close().await?;

        // Receive and forward Sfn execution result response
        copy(&mut receive_stream, writer).await?;

        Ok(())
    }
}

#[derive(Deserialize)]
pub struct HttpRequest {
    pub args: String,

    #[serde(default)]
    pub context: String,
}

// HTTP request handler: forward request to corresponding QUIC Sfn
#[axum::debug_handler]
async fn handle_post(
    headers: HeaderMap,
    Path(name): Path<String>,
    State(zipper): State<Zipper>,
    Json(req): Json<HttpRequest>,
) -> Result<Json<Value>, (StatusCode, String)> {
    // Create metadata
    let metadata = zipper
        .middleware
        .read()
        .await
        .new_request_metadata(&headers)
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    info!(
        "[{}|{}] http new request: sfn_name={:?}, args={:?}, context={:?}",
        metadata.trace_id, metadata.req_id, name, req.args, req.context
    );

    match zipper
        .proxy_request(&metadata, &name, &req.args, &req.context)
        .await
    {
        Ok(res) => match res {
            Some(res) => {
                info!("[{}|{}] sfn success", metadata.trace_id, metadata.req_id);
                debug!(
                    "[{}|{}] sfn response: {}",
                    metadata.trace_id, metadata.req_id, res.result
                );
                Ok(json!({"result": res.result}).into())
            }
            None => Err((StatusCode::NOT_FOUND, "sfn not found".to_string())),
        },
        Err(e) => {
            error!(
                "[{}|{}] proxy_request error: {:?}",
                metadata.trace_id, metadata.req_id, e
            );
            Err((StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))
        }
    }
}
