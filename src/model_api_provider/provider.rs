use std::pin::Pin;

use async_trait::async_trait;
use axum::body::Bytes;
use axum::http::{HeaderMap, Method, StatusCode};
use futures_core::Stream;
use futures_util::StreamExt;

pub struct ProxyRequest {
    pub method: Method,
    pub endpoint_path: String,
    pub headers: HeaderMap,
    pub body: Bytes,
    pub is_stream: bool,
    pub content_type: Option<String>,
}

pub enum ProxyBody {
    Full(Bytes),
    Stream(Pin<Box<dyn Stream<Item = Result<Bytes, std::io::Error>> + Send>>),
}

pub struct ProxyResponse {
    pub status: StatusCode,
    pub headers: HeaderMap,
    pub body: ProxyBody,
}

#[async_trait]
pub trait ModelApiProvider: Send + Sync {
    fn model_id(&self) -> &str;

    async fn proxy(&self, req: ProxyRequest) -> Result<ProxyResponse, anyhow::Error>;
}

#[derive(Clone)]
pub struct ProxyClient {
    client: reqwest::Client,
    base_url: String,
    auth_headers: HeaderMap,
    model_id: String,
}

impl ProxyClient {
    pub fn new(
        client: reqwest::Client,
        base_url: String,
        auth_headers: HeaderMap,
        model_id: String,
    ) -> Self {
        Self {
            client,
            base_url,
            auth_headers,
            model_id,
        }
    }
}

#[async_trait]
impl ModelApiProvider for ProxyClient {
    fn model_id(&self) -> &str {
        &self.model_id
    }

    async fn proxy(&self, req: ProxyRequest) -> Result<ProxyResponse, anyhow::Error> {
        proxy_request(
            &self.client,
            &self.base_url,
            self.auth_headers.clone(),
            req,
        )
        .await
    }
}

const HOP_HEADERS: [&str; 8] = [
    "connection",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailers",
    "transfer-encoding",
    "upgrade",
];

pub async fn proxy_request(
    client: &reqwest::Client,
    base_url: &str,
    mut auth_headers: HeaderMap,
    req: ProxyRequest,
) -> Result<ProxyResponse, anyhow::Error> {
    let url = format!("{}{}", base_url.trim_end_matches('/'), req.endpoint_path);
    let mut headers = filter_request_headers(req.headers);
    headers.extend(auth_headers.drain());

    let mut builder = client.request(req.method, url).headers(headers);
    if !req.body.is_empty() {
        builder = builder.body(req.body);
    }

    let response = builder.send().await.map_err(|err| anyhow::anyhow!(err))?;

    let status = response.status();
    let mut resp_headers = filter_response_headers(response.headers());
    let is_stream = req.is_stream;

    if is_stream {
        resp_headers.remove(axum::http::header::CONTENT_LENGTH);
        let stream = response
            .bytes_stream()
            .map(|chunk| match chunk {
                Ok(bytes) => Ok(bytes),
                Err(err) => Err(std::io::Error::new(std::io::ErrorKind::Other, err)),
            });
        let body: Pin<Box<dyn Stream<Item = Result<Bytes, std::io::Error>> + Send>> =
            Box::pin(stream);
        Ok(ProxyResponse {
            status,
            headers: resp_headers,
            body: ProxyBody::Stream(body),
        })
    } else {
        let bytes = response.bytes().await.map_err(|err| anyhow::anyhow!(err))?;
        Ok(ProxyResponse {
            status,
            headers: resp_headers,
            body: ProxyBody::Full(bytes),
        })
    }
}

fn filter_request_headers(headers: HeaderMap) -> HeaderMap {
    let mut filtered = HeaderMap::new();
    for (key, value) in headers.iter() {
        if key == axum::http::header::HOST {
            continue;
        }
        if key == axum::http::header::CONTENT_LENGTH {
            continue;
        }
        if is_hop_header(key.as_str()) {
            continue;
        }
        filtered.insert(key.clone(), value.clone());
    }
    filtered
}

fn filter_response_headers(headers: &HeaderMap) -> HeaderMap {
    let mut filtered = HeaderMap::new();
    for (key, value) in headers.iter() {
        if is_hop_header(key.as_str()) {
            continue;
        }
        filtered.insert(key.clone(), value.clone());
    }
    filtered
}

fn is_hop_header(header: &str) -> bool {
    HOP_HEADERS
        .iter()
        .any(|item| item.eq_ignore_ascii_case(header))
}
