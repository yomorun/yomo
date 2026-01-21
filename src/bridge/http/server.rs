use std::{convert::Infallible, sync::Arc};

use anyhow::anyhow;
use axum::{
    Json, Router,
    body::Bytes,
    extract::{Path, State},
    http::{HeaderMap, StatusCode},
    response::{IntoResponse, Sse, sse::Event},
    routing::post,
};
use futures_util::{Stream, stream};
use log::{debug, error, info};
use serde::{Deserialize, Serialize};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt, simplex},
    net::TcpListener,
};

use crate::{
    bridge::{Bridge, http::middleware::HttpMiddleware},
    frame::{HandlerChunk, HandlerRequest, HandlerResponse},
    io::{receive_frame, send_frame},
};

const MAX_BUF_SIZE: usize = 16 * 1024;

#[derive(Debug, Clone, Deserialize)]
pub struct HttpBridgeConfig {
    #[serde(default = "default_host")]
    host: String,

    #[serde(default = "default_port")]
    port: u16,
}

impl Default for HttpBridgeConfig {
    fn default() -> Self {
        Self {
            host: default_host(),
            port: default_port(),
        }
    }
}

fn default_host() -> String {
    "127.0.0.1".to_string()
}

fn default_port() -> u16 {
    9001
}

struct AppError {
    status_code: StatusCode,
    msg: String,
}

impl IntoResponse for AppError {
    fn into_response(self) -> axum::response::Response {
        (self.status_code, self.msg).into_response()
    }
}

impl<E> From<E> for AppError
where
    E: Into<anyhow::Error>,
{
    fn from(err: E) -> Self {
        Self {
            status_code: StatusCode::INTERNAL_SERVER_ERROR,
            msg: err.into().to_string(),
        }
    }
}

#[derive(Deserialize, Serialize)]
struct HttpRequest {
    args: String,
}

// HTTP request handler: forward request to corresponding QUIC sfn
#[axum::debug_handler]
async fn handle_simple(
    headers: HeaderMap,
    Path(name): Path<String>,
    State((bridge, middleware)): State<(Arc<dyn Bridge>, Arc<dyn HttpMiddleware>)>,
    Json(req): Json<HttpRequest>,
) -> Result<Bytes, AppError> {
    info!("new request to [{}]", name);

    // Create metadata
    let metadata = middleware.new_metadata(&headers)?;

    let handler_req = HandlerRequest {
        args: req.args,
        stream: false,
    };

    let mut req_stream = simplex(MAX_BUF_SIZE);
    let mut res_stream = simplex(MAX_BUF_SIZE);

    if bridge
        .forward(&name, &metadata, req_stream.0, res_stream.1)
        .await?
    {
        send_frame(&mut req_stream.1, &handler_req).await?;
        req_stream.1.shutdown().await?;

        let handler_res = receive_frame::<HandlerResponse>(&mut res_stream.0)
            .await?
            .ok_or(anyhow!("Failed to receive response"))?;
        debug!("received response: {:?}", handler_res.result);
        Ok(handler_res.result.into())
    } else {
        Err(AppError {
            status_code: StatusCode::NOT_FOUND,
            msg: "sfn not found".to_string(),
        })
    }
}

// HTTP stream handler: forward request to corresponding QUIC sfn with SSE response
#[axum::debug_handler]
async fn handle_sse(
    headers: HeaderMap,
    Path(name): Path<String>,
    State((bridge, middleware)): State<(Arc<dyn Bridge>, Arc<dyn HttpMiddleware>)>,
    Json(req): Json<HttpRequest>,
) -> Result<Sse<impl Stream<Item = Result<Event, Infallible>>>, AppError> {
    info!("new request to [{}]", name);

    // Create metadata
    let metadata = middleware.new_metadata(&headers)?;

    let handler_req = HandlerRequest {
        args: req.args,
        stream: true,
    };

    let mut req_stream = simplex(MAX_BUF_SIZE);
    let res_stream = simplex(MAX_BUF_SIZE);

    if bridge
        .forward(&name, &metadata, req_stream.0, res_stream.1)
        .await?
    {
        send_frame(&mut req_stream.1, &handler_req).await?;
        req_stream.1.shutdown().await?;

        let stream = stream::unfold(res_stream.0, move |mut r| async move {
            match process_chunk(&mut r).await {
                Ok(chunk) => match chunk {
                    Some(chunk) => Some((Ok(chunk), r)),
                    None => None,
                },
                Err(e) => {
                    error!("Error receiving frame: {}", e);
                    None
                }
            }
        });

        Ok(Sse::new(stream))
    } else {
        Err(AppError {
            status_code: StatusCode::NOT_FOUND,
            msg: "sfn not found".to_string(),
        })
    }
}
async fn process_chunk(stream: &mut (impl AsyncReadExt + Unpin)) -> anyhow::Result<Option<Event>> {
    if let Some(data) = receive_frame::<HandlerChunk>(stream).await? {
        debug!("received chunk: {}", data.chunk);
        let event = Event::default().data(data.chunk);
        Ok(Some(event))
    } else {
        Ok(None)
    }
}

// HTTP server: listen and receive external requests
pub async fn serve_http_bridge(
    config: &HttpBridgeConfig,
    bridge: impl Bridge + 'static,
    middleware: impl HttpMiddleware + 'static,
) -> anyhow::Result<()> {
    let app = Router::new()
        .route("/sfn/{name}", post(handle_simple))
        .route("/sfn/{name}/sse", post(handle_sse))
        .with_state((Arc::new(bridge), Arc::new(middleware)));

    let listener = TcpListener::bind((config.host.to_owned(), config.port)).await?;

    info!("start http server: {}:{}", config.host, config.port);
    axum::serve(listener, app).await?;

    Ok(())
}
