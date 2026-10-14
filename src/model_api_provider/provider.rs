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
pub trait ModelApiProvider<M>: Send + Sync {
    fn model_id(&self) -> &str;

    async fn execute(
        &self,
        req: ProviderRequest,
        metadata: &M,
    ) -> Result<ProviderResponse, anyhow::Error>;

    fn extract_request_id(&self, payload_json: &Value) -> Option<String> {
        payload_json
            .get("id")
            .and_then(Value::as_str)
            .map(str::to_string)
            .or_else(|| {
                payload_json
                    .get("response")
                    .and_then(|response| response.get("id"))
                    .and_then(Value::as_str)
                    .map(str::to_string)
            })
    }

    fn extract_usage(&self, payload_json: &Value) -> Option<Value> {
        payload_json
            .get("usage")
            .cloned()
            .or_else(|| payload_json.get("usageMetadata").cloned())
            .or_else(|| {
                payload_json
                    .get("response")
                    .and_then(|response| response.get("usage"))
                    .cloned()
            })
            .or_else(|| {
                payload_json
                    .get("response")
                    .and_then(|response| response.get("usageMetadata"))
                    .cloned()
            })
    }

    fn inject_usage(&self, payload_json: &mut Value, usage: Value) -> bool {
        inject_usage_value(payload_json, usage)
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
    let is_stream_response = should_stream_response(req.is_stream, status);

    if is_stream_response {
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

pub(crate) fn should_stream_response(is_stream_request: bool, status: StatusCode) -> bool {
    is_stream_request && status.is_success()
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
    strip_bedrock_unsupported_beta_fields(&mut value);

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

// follow: https://code.claude.com/docs/en/errors#extra-inputs-are-not-permitted
fn strip_bedrock_unsupported_beta_fields(value: &mut Value) {
    let Some(obj) = value.as_object_mut() else {
        return;
    };

    obj.remove("context_management");
    obj.remove("effort");

    let Some(tools) = obj.get_mut("tools").and_then(Value::as_array_mut) else {
        return;
    };

    for tool in tools {
        let Some(tool_obj) = tool.as_object_mut() else {
            continue;
        };
        let Some(custom_obj) = tool_obj.get_mut("custom").and_then(Value::as_object_mut) else {
            continue;
        };
        custom_obj.remove("input_examples");
    }
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

#[cfg(test)]
mod tests {
    use super::{rewrite_messages_body, should_stream_response};
    use axum::body::Bytes;
    use axum::http::StatusCode;
    use serde_json::{Value, json};

    #[test]
    fn should_stream_response_requires_stream_request() {
        assert!(!should_stream_response(false, StatusCode::OK));
    }

    #[test]
    fn should_stream_response_requires_success_status() {
        assert!(!should_stream_response(true, StatusCode::BAD_REQUEST));
        assert!(!should_stream_response(
            true,
            StatusCode::INTERNAL_SERVER_ERROR
        ));
    }

    #[test]
    fn should_stream_response_allows_successful_stream_request() {
        assert!(should_stream_response(true, StatusCode::OK));
    }

    #[test]
    fn rewrite_messages_body_strips_beta_fields_for_bedrock_compatibility() {
        let request = json!({
            "model": "claude-sonnet-4",
            "stream": true,
            "messages": [{"role": "user", "content": "hello"}],
            "context_management": {"enabled": true},
            "effort": "medium",
            "tools": [
                {
                    "name": "search",
                    "custom": {
                        "input_schema": {"type": "object"},
                        "input_examples": [{"query": "rust"}]
                    }
                }
            ]
        });

        let rewritten = rewrite_messages_body(
            &Bytes::from(serde_json::to_vec(&request).expect("request json should serialize")),
            "bedrock-2023-05-31",
            4096,
        )
        .expect("rewrite should succeed");
        let parsed: Value =
            serde_json::from_slice(&rewritten).expect("rewritten json should parse");

        assert!(parsed.get("context_management").is_none());
        assert!(parsed.get("effort").is_none());
        assert!(parsed.get("model").is_none());
        assert!(parsed.get("stream").is_none());
        assert_eq!(
            parsed.get("anthropic_version").and_then(Value::as_str),
            Some("bedrock-2023-05-31")
        );
        assert_eq!(parsed.get("max_tokens"), Some(&json!(4096)));
        assert!(parsed["tools"][0]["custom"].get("input_examples").is_none());
        assert_eq!(
            parsed["tools"][0]["custom"].get("input_schema"),
            Some(&json!({"type": "object"}))
        );
    }

    #[test]
    fn rewrite_messages_body_keeps_existing_max_tokens() {
        let request = json!({
            "messages": [{"role": "user", "content": "hello"}],
            "max_tokens": 123
        });

        let rewritten = rewrite_messages_body(
            &Bytes::from(serde_json::to_vec(&request).expect("request json should serialize")),
            "bedrock-2023-05-31",
            4096,
        )
        .expect("rewrite should succeed");
        let parsed: Value =
            serde_json::from_slice(&rewritten).expect("rewritten json should parse");

        assert_eq!(parsed.get("max_tokens"), Some(&json!(123)));
    }
}
