use std::sync::Arc;

use axum::{
    Json, Router,
    extract::{Path, State},
    http::{HeaderMap, StatusCode},
    response::{IntoResponse, Sse, sse::Event},
    routing::post,
};
use futures_util::stream;
use log::{error, info};
use tokio::{
    io::{AsyncWriteExt, ReadHalf, SimplexStream, simplex},
    net::TcpListener,
};

use crate::{
    bridge::Bridge,
    io::{receive_frame, send_frame},
    types::{RequestBody, RequestHeaders, ResponseBody},
    zipper::server::Zipper,
};

const MAX_BUF_SIZE: usize = 16 * 1024;

struct CustomError {
    status_code: StatusCode,
    msg: String,
}

impl IntoResponse for CustomError {
    fn into_response(self) -> axum::response::Response {
        (self.status_code, self.msg).into_response()
    }
}

impl<E> From<E> for CustomError
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
        Some(value) => value.to_str().unwrap_or_default(),
        None => "",
    }
    .to_string()
}

fn new_request_headers(sfn_name: &str, http_headers: &HeaderMap) -> RequestHeaders {
    RequestHeaders {
        sfn_name: sfn_name.to_owned(),
        trace_id: parse_http_headers(http_headers, "traceparent"),
        req_id: parse_http_headers(http_headers, "X-Request-Id"),
        stream: parse_http_headers(http_headers, "X-Stream-Response").to_lowercase() == "true",
        extension: parse_http_headers(http_headers, "X-Extension"),
    }
}

struct CustomResponse {
    body: Option<ResponseBody>,
    r2: Option<ReadHalf<SimplexStream>>,
}

impl IntoResponse for CustomResponse {
    fn into_response(self) -> axum::response::Response {
        if let Some(body) = self.body {
            match serde_json::to_string(&body) {
                Ok(buf) => (StatusCode::OK, buf).into_response(),
                Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response(),
            }
        } else if let Some(r2) = self.r2 {
            let stream = stream::unfold(r2, move |mut r| async move {
                match receive_frame::<ResponseBody>(&mut r).await {
                    Ok(Some(chunk)) => {
                        info!("recv chunk: {:?}", chunk);
                        if let Some(data) = serde_json::to_string(&chunk).ok() {
                            Some((Ok(Event::default().data(data)), r))
                        } else {
                            None
                        }
                    }
                    Ok(None) => None,
                    Err(e) => {
                        error!("receiving frame error: {}", e);
                        Some((Err(anyhow::anyhow!("receiving frame error: {}", e)), r))
                    }
                }
            });

            Sse::new(stream).into_response()
        } else {
            (StatusCode::OK, "").into_response()
        }
    }
}

// HTTP stream handler: forward request to corresponding QUIC sfn with SSE response
#[axum::debug_handler]
async fn handle(
    http_headers: HeaderMap,
    Path(sfn_name): Path<String>,
    State(zipper): State<Arc<Zipper>>,
    Json(body): Json<RequestBody>,
) -> Result<CustomResponse, CustomError> {
    info!("new request to [{}]", sfn_name);

    let request_headers = new_request_headers(&sfn_name, &http_headers);

    let (r1, mut w1) = simplex(MAX_BUF_SIZE);
    let (mut r2, w2) = simplex(MAX_BUF_SIZE);

    // send request headers
    send_frame(&mut w1, &request_headers).await?;

    if zipper.forward(r1, w2).await? {
        // send request body
        send_frame(&mut w1, &body).await?;
        w1.shutdown().await?;

        if request_headers.stream {
            // Stream response using SSE
            Ok(CustomResponse {
                body: None,
                r2: Some(r2),
            })
        } else {
            let response: ResponseBody = receive_frame(&mut r2)
                .await?
                .ok_or(anyhow::anyhow!("Failed to receive response"))?;

            Ok(CustomResponse {
                body: Some(response),
                r2: None,
            })
        }
    } else {
        Err(CustomError {
            status_code: StatusCode::NOT_FOUND,
            msg: format!("sfn [{}] not found", sfn_name),
        })
    }
}

// HTTP server: listen and receive external requests
pub async fn serve_http(host: &str, port: u16, zipper: Arc<Zipper>) -> anyhow::Result<()> {
    let app = Router::new()
        .route("/sfn/{sfn_name}", post(handle))
        .with_state(zipper);

    let listener = TcpListener::bind((host.to_owned(), port)).await?;

    info!("start http server: {}:{}", host, port);
    axum::serve(listener, app).await?;

    Ok(())
}
