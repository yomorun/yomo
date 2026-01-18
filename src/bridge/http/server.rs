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
use log::{error, info};
use serde::{Deserialize, Serialize};
use tokio::{
    io::{AsyncReadExt, simplex},
    net::TcpListener,
};

use crate::{
    bridge::{
        Bridge,
        http::{config::HttpBridgeConfig, middleware::HttpMiddleware},
    },
    io::{receive_all, receive_chunk, send_all},
    types::{Request, Response},
};

const MAX_BUF_SIZE: usize = 64 * 1024;

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
    data: serde_json::Value,
}

// HTTP request handler: forward request to corresponding QUIC Sfn
#[axum::debug_handler]
async fn handle_simple(
    headers: HeaderMap,
    Path(name): Path<String>,
    State((middleware, bridge)): State<(Arc<dyn HttpMiddleware>, Arc<dyn Bridge>)>,
    Json(req): Json<HttpRequest>,
) -> Result<Bytes, AppError> {
    info!("new request to [{}]", name);

    // Create metadata
    let metadata = middleware.new_metadata(&headers)?;

    let request = Request {
        data: serde_json::to_vec(&req.data)?,
        stream: false,
    };

    let req_stream = simplex(MAX_BUF_SIZE);
    let res_stream = simplex(MAX_BUF_SIZE);

    if bridge
        .forward(&name, &metadata, req_stream.0, res_stream.1)
        .await?
    {
        send_all(req_stream.1, &request).await?;

        if let Response::Data(data) = receive_all::<Response>(res_stream.0).await? {
            Ok(data.into())
        } else {
            Err(anyhow!("invalid response format").into())
        }
    } else {
        Err(AppError {
            status_code: StatusCode::NOT_FOUND,
            msg: "sfn not found".to_string(),
        })
    }
}

// HTTP request handler: forward request to corresponding QUIC Sfn
#[axum::debug_handler]
async fn handle_sse(
    headers: HeaderMap,
    Path(name): Path<String>,
    State((middleware, bridge)): State<(Arc<dyn HttpMiddleware>, Arc<dyn Bridge>)>,
    Json(req): Json<HttpRequest>,
) -> Result<Sse<impl Stream<Item = Result<Event, Infallible>>>, AppError> {
    info!("new request to [{}]", name);

    // Create metadata
    let metadata = middleware.new_metadata(&headers)?;

    let request = Request {
        data: serde_json::to_vec(&req.data)?,
        stream: true,
    };

    let req_stream = simplex(MAX_BUF_SIZE);
    let res_stream = simplex(MAX_BUF_SIZE);

    if bridge
        .forward(&name, &metadata, req_stream.0, res_stream.1)
        .await?
    {
        send_all(req_stream.1, &request).await?;

        let stream = stream::unfold(res_stream.0, move |mut r| async move {
            match process_chunk(&mut r).await {
                Ok(chunk) => match chunk {
                    Some(chunk) => Some((Ok(chunk), r)),
                    None => None,
                },
                Err(e) => {
                    error!("Error receiving chunk: {}", e);
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
    if let Some(Response::Data(data)) = receive_chunk::<Response>(stream).await? {
        let data: serde_json::Value = serde_json::from_slice(&data)?;
        let data = serde_json::to_string(&data)?;
        let event = Event::default().data(data);
        Ok(Some(event))
    } else {
        Ok(None)
    }
}

// HTTP server: listen and receive external requests
pub async fn serve_http_bridge(
    config: &HttpBridgeConfig,
    middleware: Arc<dyn HttpMiddleware>,
    bridge: Arc<dyn Bridge>,
) -> anyhow::Result<()> {
    let app = Router::new()
        .route("/sfn/{name}", post(handle_simple))
        .route("/sfn/{name}/sse", post(handle_sse))
        .with_state((middleware, bridge));

    let listener = TcpListener::bind((config.host.to_owned(), config.port)).await?;

    info!("start http server: {}:{}", config.host, config.port);
    axum::serve(listener, app).await?;

    Ok(())
}
