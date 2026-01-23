use std::{convert::Infallible, sync::Arc};

use axum::{
    Json, Router,
    body::Bytes,
    extract::{Path, State},
    http::{HeaderMap, StatusCode},
    response::{IntoResponse, Sse, sse::Event},
    routing::post,
};
use futures_util::{Stream, stream};
use log::{error, info};
use serde::Deserialize;
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt, simplex},
    net::TcpListener,
};

use crate::{
    bridge::Bridge,
    io::receive_raw,
    types::{Request, RequestBody, RequestHeaders},
    zipper::server::Zipper,
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

// HTTP request handler: forward request to corresponding QUIC sfn
#[axum::debug_handler]
async fn handle_simple(
    _headers: HeaderMap,
    Path(name): Path<String>,
    State(zipper): State<Arc<Zipper>>,
    Json(body): Json<RequestBody>,
) -> Result<Bytes, AppError> {
    info!("new request to [{}]", name);

    let request = Request {
        headers: RequestHeaders {
            sfn_name: name.to_owned(),
            stream: false,
            ..Default::default()
        },
        body,
    };

    let (r1, mut w1) = simplex(MAX_BUF_SIZE);
    let (mut r2, w2) = simplex(MAX_BUF_SIZE);

    if zipper.forward(&request.headers, r1, w2).await? {
        let buf = serde_json::to_vec(&request)?;
        w1.write_all(&buf).await?;
        w1.shutdown().await?;

        let mut buf = Vec::new();
        let _ = r2.read_to_end(&mut buf).await?;
        let response = Bytes::from(buf);

        Ok(response)
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
    _headers: HeaderMap,
    Path(name): Path<String>,
    State(zipper): State<Arc<Zipper>>,
    Json(body): Json<RequestBody>,
) -> Result<Sse<impl Stream<Item = Result<Event, Infallible>>>, AppError> {
    info!("new request to [{}]", name);

    let request = Request {
        headers: RequestHeaders {
            sfn_name: name.to_owned(),
            stream: true,
            ..Default::default()
        },
        body,
    };

    let (r1, mut w1) = simplex(MAX_BUF_SIZE);
    let (r2, w2) = simplex(MAX_BUF_SIZE);

    if zipper.forward(&request.headers, r1, w2).await? {
        let buf = serde_json::to_vec(&request)?;
        w1.write_all(&buf).await?;
        w1.shutdown().await?;

        let stream = stream::unfold(r2, move |mut r| async move {
            match receive_raw(&mut r).await {
                Ok(Some(chunk)) => {
                    let event = Event::default().data(chunk);
                    Some((Ok(event), r))
                }
                Ok(None) => None,
                Err(e) => {
                    error!("receiving frame error: {}", e);
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

// HTTP server: listen and receive external requests
pub async fn serve_http(config: &HttpBridgeConfig, zipper: Arc<Zipper>) -> anyhow::Result<()> {
    let app = Router::new()
        .route("/sfn/{name}", post(handle_simple))
        .route("/sfn/{name}/sse", post(handle_sse))
        .with_state(zipper);

    let listener = TcpListener::bind((config.host.to_owned(), config.port)).await?;

    info!("start http server: {}:{}", config.host, config.port);
    axum::serve(listener, app).await?;

    Ok(())
}
