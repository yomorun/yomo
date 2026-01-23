use std::{convert::Infallible, sync::Arc};

use anyhow::anyhow;
use axum::{
    Json, Router,
    extract::{Path, State},
    http::{HeaderMap, StatusCode},
    response::{IntoResponse, Sse, sse::Event},
    routing::post,
};
use futures_util::{Stream, stream};
use log::{error, info};
use tokio::{
    io::{AsyncWriteExt, simplex},
    net::TcpListener,
};

use crate::{
    bridge::Bridge,
    io::{receive_frame, send_frame},
    types::{RequestBody, RequestHeaders, ResponseBody},
    zipper::server::Zipper,
};

const MAX_BUF_SIZE: usize = 16 * 1024;

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

fn parse_http_headers(http_headers: &HeaderMap, key: &str) -> String {
    match http_headers.get(key) {
        Some(trace_id) => trace_id.to_str().unwrap_or_default(),
        None => "",
    }
    .to_string()
}

fn new_request_headers(sfn_name: &str, http_headers: &HeaderMap, stream: bool) -> RequestHeaders {
    RequestHeaders {
        sfn_name: sfn_name.to_owned(),
        stream,
        trace_id: parse_http_headers(http_headers, "traceparent"),
        req_id: parse_http_headers(http_headers, "X-Request-Id"),
        extra: serde_json::from_str(&parse_http_headers(http_headers, "X-YoMo-Extra"))
            .unwrap_or_default(),
    }
}

// HTTP request handler: forward request to corresponding QUIC sfn
#[axum::debug_handler]
async fn handle_simple(
    http_headers: HeaderMap,
    Path(sfn_name): Path<String>,
    State(zipper): State<Arc<Zipper>>,
    Json(body): Json<RequestBody>,
) -> Result<String, AppError> {
    info!("new request to [{}]", sfn_name);

    let request_headers = new_request_headers(&sfn_name, &http_headers, false);

    let (r1, mut w1) = simplex(MAX_BUF_SIZE);
    let (mut r2, w2) = simplex(MAX_BUF_SIZE);

    if zipper.forward(&request_headers, r1, w2).await? {
        send_frame(&mut w1, &request_headers).await?;
        send_frame(&mut w1, &body).await?;
        w1.shutdown().await?;

        let body: ResponseBody = receive_frame(&mut r2)
            .await?
            .ok_or(anyhow!("Failed to receive response"))?;
        info!("recv response: {:?}", body);

        Ok(serde_json::to_string(&body)?)
    } else {
        Err(AppError {
            status_code: StatusCode::NOT_FOUND,
            msg: format!("sfn [{}] not found", sfn_name),
        })
    }
}

// HTTP stream handler: forward request to corresponding QUIC sfn with SSE response
#[axum::debug_handler]
async fn handle_sse(
    http_headers: HeaderMap,
    Path(sfn_name): Path<String>,
    State(zipper): State<Arc<Zipper>>,
    Json(body): Json<RequestBody>,
) -> Result<Sse<impl Stream<Item = Result<Event, Infallible>>>, AppError> {
    info!("new request to [{}]", sfn_name);

    let request_headers = new_request_headers(&sfn_name, &http_headers, true);

    let (r1, mut w1) = simplex(MAX_BUF_SIZE);
    let (r2, w2) = simplex(MAX_BUF_SIZE);

    if zipper.forward(&request_headers, r1, w2).await? {
        send_frame(&mut w1, &request_headers).await?;
        send_frame(&mut w1, &body).await?;
        w1.shutdown().await?;

        let stream = stream::unfold(r2, move |mut r| async move {
            match receive_frame::<ResponseBody>(&mut r).await {
                Ok(Some(chunk)) => {
                    info!("recv chunk: {:?}", chunk);
                    let data = serde_json::to_string(&chunk).unwrap_or_default();
                    if data.len() == 0 {
                        return None;
                    }
                    Some((Ok(Event::default().data(data)), r))
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
            msg: format!("sfn [{}] not found", sfn_name),
        })
    }
}

// HTTP server: listen and receive external requests
pub async fn serve_http(host: &str, port: u16, zipper: Arc<Zipper>) -> anyhow::Result<()> {
    let app = Router::new()
        .route("/sfn/{sfn_name}", post(handle_simple))
        .route("/sfn/{sfn_name}/sse", post(handle_sse))
        .with_state(zipper);

    let listener = TcpListener::bind((host.to_owned(), port)).await?;

    info!("start http server: {}:{}", host, port);
    axum::serve(listener, app).await?;

    Ok(())
}
