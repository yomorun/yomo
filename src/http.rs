use std::sync::Arc;

use axum::{
    Router,
    body::Bytes,
    extract::{Path, State},
    http::{self, HeaderMap, StatusCode},
    response::{IntoResponse, Sse, sse::Event},
    routing::post,
};
use futures_util::stream;
use log::{error, info};
use tokio::{
    io::{AsyncWriteExt, ReadHalf, SimplexStream},
    net::TcpListener,
    sync::Mutex,
};

use crate::{
    connector::{Connector, MemoryConnector},
    io::{receive_bytes, receive_frame, send_bytes, send_frame},
    types::{RequestHeaders, ResponseHeaders},
};

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
        stream: false,
        trace_id: parse_http_headers(http_headers, "traceparent"),
        request_id: parse_http_headers(http_headers, "X-Request-Id"),
        extension: parse_http_headers(http_headers, "X-Extension"),
    }
}

struct CustomResponse {
    body: Option<Vec<u8>>,
    reader: Option<ReadHalf<SimplexStream>>,
}

impl IntoResponse for CustomResponse {
    fn into_response(self) -> axum::response::Response {
        if let Some(body) = self.body {
            (StatusCode::OK, body).into_response()
        } else if let Some(reader) = self.reader {
            let stream = stream::unfold(reader, move |mut r| async move {
                match receive_bytes(&mut r).await {
                    Ok(Some(chunk)) => {
                        let data = String::from_utf8_lossy(&chunk);
                        info!("recv chunk: {:?}", data);
                        Some((Ok(Event::default().data(data)), r))
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
    State(connector): State<Arc<Mutex<MemoryConnector>>>,
    body: Bytes,
) -> Result<CustomResponse, CustomError> {
    info!("new request to [{}]", sfn_name);

    let request_headers = new_request_headers(&sfn_name, &http_headers);

    let (mut reader, mut writer) = connector.lock().await.open_new_stream().await?;

    // send request headers
    send_frame(&mut writer, &request_headers).await?;

    // send request body
    send_bytes(&mut writer, &body.to_vec()).await?;
    writer.shutdown().await?;

    let response_headers: ResponseHeaders = receive_frame(&mut reader)
        .await?
        .ok_or(anyhow::anyhow!("Failed to receive response headers"))?;

    if response_headers.status_code != http::StatusCode::OK {
        return Err(CustomError {
            status_code: StatusCode::from_u16(response_headers.status_code)?,
            msg: response_headers.error_msg,
        });
    }

    if response_headers.stream {
        // Stream response using SSE
        Ok(CustomResponse {
            body: None,
            reader: Some(reader),
        })
    } else {
        let body = receive_bytes(&mut reader)
            .await?
            .ok_or(anyhow::anyhow!("Failed to receive response"))?;

        Ok(CustomResponse {
            body: Some(body),
            reader: None,
        })
    }
}

// HTTP server: listen and receive external requests
pub async fn serve_http(host: &str, port: u16, connector: MemoryConnector) -> anyhow::Result<()> {
    let app = Router::new()
        .route("/sfn/{sfn_name}", post(handle))
        .with_state(Arc::new(Mutex::new(connector)));

    let listener = TcpListener::bind((host.to_owned(), port)).await?;

    info!("start http server: {}:{}", host, port);
    axum::serve(listener, app).await?;

    Ok(())
}
