use std::pin::Pin;

use anyhow::anyhow;
use async_trait::async_trait;
use axum::body::Bytes;
use axum::http::{HeaderMap, Method, StatusCode};
use futures_core::Stream;
use futures_util::StreamExt;
use futures_util::stream;
use reqwest::multipart::{Form, Part};
use serde_json::Value;

pub struct ProviderRequest {
    pub method: Method,
    pub endpoint_path: String,
    pub headers: HeaderMap,
    pub body: Bytes,
    pub is_stream: bool,
    pub content_type: Option<String>,
}

pub enum ProviderBody {
    Full(Bytes),
    Stream(Pin<Box<dyn Stream<Item = Result<Bytes, std::io::Error>> + Send>>),
}

pub struct ProviderResponse {
    pub status: StatusCode,
    pub headers: HeaderMap,
    pub body: ProviderBody,
}

#[async_trait]
pub trait ModelApiProvider: Send + Sync {
    fn model_id(&self) -> &str;

    async fn execute(&self, req: ProviderRequest) -> Result<ProviderResponse, anyhow::Error>;

    fn extract_request_id_from_full(&self, body_json: &Value) -> Option<String> {
        body_json
            .get("id")
            .and_then(Value::as_str)
            .map(str::to_string)
            .or_else(|| {
                body_json
                    .get("response")
                    .and_then(|response| response.get("id"))
                    .and_then(Value::as_str)
                    .map(str::to_string)
            })
    }

    fn extract_request_id_from_stream_event(&self, event_json: &Value) -> Option<String> {
        self.extract_request_id_from_full(event_json)
    }

    fn extract_usage_from_full(&self, body_json: &Value) -> Option<Value> {
        body_json
            .get("usage")
            .cloned()
            .or_else(|| body_json.get("usageMetadata").cloned())
            .or_else(|| {
                body_json
                    .get("response")
                    .and_then(|response| response.get("usage"))
                    .cloned()
            })
            .or_else(|| {
                body_json
                    .get("response")
                    .and_then(|response| response.get("usageMetadata"))
                    .cloned()
            })
    }

    fn extract_usage_from_stream_event(&self, event_json: &Value) -> Option<Value> {
        self.extract_usage_from_full(event_json)
    }

    fn inject_usage_into_full(&self, body_json: &mut Value, usage: Value) -> bool {
        inject_usage_value(body_json, usage)
    }

    fn inject_usage_into_stream_event(&self, event_json: &mut Value, usage: Value) -> bool {
        self.inject_usage_into_full(event_json, usage)
    }
}

fn inject_usage_value(value: &mut Value, usage: Value) -> bool {
    let Some(obj) = value.as_object_mut() else {
        return false;
    };

    if obj.contains_key("usage") {
        obj.insert("usage".to_string(), usage);
        return true;
    }
    if obj.contains_key("usageMetadata") {
        obj.insert("usageMetadata".to_string(), usage);
        return true;
    }

    if let Some(response) = obj.get_mut("response").and_then(Value::as_object_mut) {
        if response.contains_key("usage") {
            response.insert("usage".to_string(), usage);
            return true;
        }
        if response.contains_key("usageMetadata") {
            response.insert("usageMetadata".to_string(), usage);
            return true;
        }
    }

    false
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
    model_override: Option<&str>,
    req: ProviderRequest,
) -> Result<ProviderResponse, anyhow::Error> {
    let url = format!("{}{}", base_url.trim_end_matches('/'), req.endpoint_path);
    let mut headers = filter_request_headers(req.headers);
    headers.extend(auth_headers.drain());

    let mut request_body = req.body;
    let mut multipart_form: Option<Form> = None;
    if let Some(model) = model_override {
        if let Some(content_type) = req.content_type.as_deref() {
            if content_type.starts_with("application/json") {
                request_body = rewrite_json_model(&request_body, model)?;
            } else if content_type.starts_with("multipart/form-data") {
                multipart_form =
                    Some(rewrite_multipart_model(content_type, &request_body, model).await?);
                headers.remove(axum::http::header::CONTENT_TYPE);
            }
        }
    }

    let mut builder = client.request(req.method, url).headers(headers);
    if let Some(form) = multipart_form {
        builder = builder.multipart(form);
    } else if !request_body.is_empty() {
        builder = builder.body(request_body);
    }

    let response = builder.send().await.map_err(|err| anyhow!(err))?;

    let status = response.status();
    let mut resp_headers = filter_response_headers(response.headers());
    let is_stream = req.is_stream;

    if is_stream {
        resp_headers.remove(axum::http::header::CONTENT_LENGTH);
        let stream = response.bytes_stream().map(|chunk| match chunk {
            Ok(bytes) => Ok(bytes),
            Err(err) => Err(std::io::Error::new(std::io::ErrorKind::Other, err)),
        });
        let body: Pin<Box<dyn Stream<Item = Result<Bytes, std::io::Error>> + Send>> =
            Box::pin(stream);
        Ok(ProviderResponse {
            status,
            headers: resp_headers,
            body: ProviderBody::Stream(body),
        })
    } else {
        let bytes = response.bytes().await.map_err(|err| anyhow!(err))?;
        Ok(ProviderResponse {
            status,
            headers: resp_headers,
            body: ProviderBody::Full(bytes),
        })
    }
}

pub(crate) fn rewrite_json_model(body: &Bytes, model: &str) -> Result<Bytes, anyhow::Error> {
    let mut json: Value = serde_json::from_slice(body)?;
    if !json.is_object() {
        return Ok(body.clone());
    }
    json["model"] = Value::String(model.to_string());
    let rewritten = serde_json::to_vec(&json)?;
    Ok(Bytes::from(rewritten))
}

pub(crate) fn parse_stream_flag(body: &Bytes) -> bool {
    serde_json::from_slice::<Value>(body)
        .ok()
        .and_then(|value| value.get("stream").and_then(Value::as_bool))
        .unwrap_or(false)
}

pub(crate) fn rewrite_messages_body(
    body: &Bytes,
    anthropic_version: &str,
    default_max_tokens: u64,
) -> Result<Bytes, anyhow::Error> {
    let mut value: Value = serde_json::from_slice(body)?;
    if !value.is_object() {
        return Ok(body.clone());
    }

    {
        let obj = value
            .as_object_mut()
            .expect("checked object with Value::is_object");
        obj.remove("model");
        obj.remove("stream");
    }

    strip_cache_control_scope(&mut value);

    {
        let obj = value
            .as_object_mut()
            .expect("checked object with Value::is_object");
        obj.insert(
            "anthropic_version".to_string(),
            Value::String(anthropic_version.to_string()),
        );
        if !obj.contains_key("max_tokens") {
            obj.insert(
                "max_tokens".to_string(),
                Value::Number(default_max_tokens.into()),
            );
        }
    }

    Ok(Bytes::from(serde_json::to_vec(&value)?))
}

pub(crate) async fn rewrite_multipart_model(
    content_type: &str,
    body: &Bytes,
    model: &str,
) -> Result<Form, anyhow::Error> {
    let boundary = parse_multipart_boundary(content_type)
        .ok_or_else(|| anyhow!("multipart boundary is missing"))?;
    let stream = stream::once(async move { Ok::<Bytes, multer::Error>(body.clone()) });
    let mut multipart = multer::Multipart::new(stream, boundary);
    let mut form = Form::new();

    while let Some(field) = multipart.next_field().await? {
        let name = field.name().unwrap_or("").to_string();
        if name == "model" {
            continue;
        }

        let filename = field.file_name().map(|value| value.to_string());
        let mime = field.content_type().map(|value| value.to_string());
        let bytes = field.bytes().await?;

        let mut part = Part::bytes(bytes.to_vec());
        if let Some(filename) = filename {
            part = part.file_name(filename);
        }
        if let Some(mime) = mime {
            part = part.mime_str(&mime)?;
        }
        form = form.part(name, part);
    }

    Ok(form.text("model", model.to_string()))
}

pub(crate) fn filter_request_headers(headers: HeaderMap) -> HeaderMap {
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

pub(crate) fn filter_response_headers(headers: &HeaderMap) -> HeaderMap {
    let mut filtered = HeaderMap::new();
    for (key, value) in headers.iter() {
        if is_hop_header(key.as_str()) {
            continue;
        }
        filtered.insert(key.clone(), value.clone());
    }
    filtered
}

fn strip_cache_control_scope(value: &mut Value) {
    match value {
        Value::Object(map) => {
            if let Some(cache_control) = map.get_mut("cache_control") {
                if let Some(cache_control_obj) = cache_control.as_object_mut() {
                    cache_control_obj.remove("scope");
                    if let Some(ephemeral) = cache_control_obj.get_mut("ephemeral") {
                        if let Some(ephemeral_obj) = ephemeral.as_object_mut() {
                            ephemeral_obj.remove("scope");
                        }
                    }
                }
            }
            for child in map.values_mut() {
                strip_cache_control_scope(child);
            }
        }
        Value::Array(items) => {
            for item in items.iter_mut() {
                strip_cache_control_scope(item);
            }
        }
        _ => {}
    }
}

fn parse_multipart_boundary(content_type: &str) -> Option<String> {
    content_type.split(';').find_map(|part| {
        let part = part.trim();
        part.strip_prefix("boundary=")
            .map(|value| value.trim_matches('"').to_string())
    })
}

fn is_hop_header(header: &str) -> bool {
    HOP_HEADERS
        .iter()
        .any(|item| item.eq_ignore_ascii_case(header))
}
