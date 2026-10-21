//! `passthrough` dialect: non-chat endpoints (bge/tts/whisper/images/jev/
//! generateContent) and the escape hatch.
//!
//! Raw forwarding plus request model override,
//! `apply_response_model_mode_to_json` (JSON and SSE frames) and usage extraction.
//! Usage is delivered via the `usage_sink` callback for the handler to wire into `on_usage`.
//!
//! passthrough never appears on a chat endpoint (guaranteed by registry),
//! so `build_client` always errors for this kind; at request time forwarding goes
//! through `PassthroughForwarder`.

use std::pin::Pin;
use std::sync::Arc;

use axum::body::Bytes;

// The bytes crate is available via axum/reqwest; referenced explicitly where needed.
use axum::http::{HeaderMap, StatusCode};
use futures_core::Stream;
use serde_json::Value;
use url::Url;

use crate::dialects::credential::StaticHeaderCredential;
use crate::dialects::credential::shared_http_client;
use crate::serve_config::{EndpointKind, PassthroughParams, ResponseModelMode};

/// Forwarded response body: full bytes or a stream of byte frames.
pub enum ForwardBody {
    Full(Bytes),
    Stream(Pin<Box<dyn Stream<Item = Result<Bytes, std::io::Error>> + Send>>),
}

/// Forward result: status, headers, body and extracted usage (for the handler's on_usage).
pub struct ForwardResponse {
    pub status: StatusCode,
    pub headers: HeaderMap,
    pub body: ForwardBody,
    /// Extracted directly for non-streaming; the usage_sink fires mid-stream otherwise.
    pub usage: Option<crate::usage_handler::EndpointUsage>,
}

/// Forward error.
#[derive(Debug)]
pub enum PassthroughError {
    InvalidPath(String),
    Http(reqwest::Error),
    Internal(String),
}

impl std::fmt::Display for PassthroughError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            PassthroughError::InvalidPath(path) => write!(f, "invalid passthrough path: {path}"),
            PassthroughError::Http(err) => write!(f, "passthrough http error: {err}"),
            PassthroughError::Internal(message) => write!(f, "passthrough error: {message}"),
        }
    }
}

impl std::error::Error for PassthroughError {}

/// Usage sink (invoked mid-stream for streaming responses).
pub type UsageSink = Arc<dyn Fn(crate::usage_handler::EndpointUsage) + Send + Sync>;

/// Forward authentication: static header or GCP SA OAuth.
#[derive(Clone)]
pub enum ForwardAuth {
    Static(Option<StaticHeaderCredential>),
    /// GCP generateContent passthrough (SA OAuth).
    GoogleVertex {
        project_id: String,
        location: String,
        token_cache: Arc<crate::dialects::credential::GcpTokenCache>,
    },
}

/// Passthrough forwarder.
#[derive(Clone)]
pub struct PassthroughForwarder {
    /// Static upstream base (None in GoogleVertex mode; URLs are assembled).
    base_url: Option<Url>,
    auth: ForwardAuth,
    /// Upstream model name (overrides the request body `model` field, matching rewrite_model_in_json_body).
    model: String,
}

const FORWARD_HEADER_BLACKLIST: [&str; 12] = [
    "host",
    "content-length",
    "transfer-encoding",
    "connection",
    "keep-alive",
    "te",
    "trailer",
    "upgrade",
    "proxy-authenticate",
    "proxy-authorization",
    "authorization",
    "cookie",
];

impl PassthroughForwarder {
    pub fn new(params: &PassthroughParams) -> Result<Self, String> {
        match (&params.base_url, &params.google_vertex) {
            (Some(base_url), None) => {
                let auth =
                    ForwardAuth::Static(params.auth.as_ref().map(|auth| StaticHeaderCredential {
                        scheme: auth.scheme,
                        api_key: auth.api_key.clone(),
                    }));
                Ok(Self {
                    base_url: Some(base_url.clone()),
                    auth,
                    model: params.model.clone(),
                })
            }
            (None, Some(google)) => {
                let fetcher = crate::dialects::google_vertex::YupOauth2TokenFetcher::new(
                    google.credentials_file.clone(),
                );
                Ok(Self {
                    base_url: None,
                    auth: ForwardAuth::GoogleVertex {
                        project_id: google.project_id.clone(),
                        location: google.location.clone(),
                        token_cache: Arc::new(crate::dialects::credential::GcpTokenCache::new(
                            Arc::new(fetcher),
                        )),
                    },
                    model: params.model.clone(),
                })
            }
            (Some(_), Some(_)) => Err(
                "passthrough params must set exactly one of base_url / google_vertex".to_string(),
            ),
            (None, None) => Err("passthrough params require base_url or google_vertex".to_string()),
        }
    }

    pub fn base_url(&self) -> Option<&Url> {
        self.base_url.as_ref()
    }

    pub fn model(&self) -> &str {
        &self.model
    }

    /// Resolve the upstream URL. Static mode joins base + path; GoogleVertex
    /// mode assembles the generateContent URL (stream flag from the body).
    fn resolve_url(&self, path: &str, body: &Bytes) -> Result<String, PassthroughError> {
        match &self.auth {
            ForwardAuth::Static(_) => {
                let base = self.base_url.as_ref().ok_or_else(|| {
                    PassthroughError::Internal("static forwarder missing base_url".to_string())
                })?;
                url_with_base(base, path).map_err(PassthroughError::InvalidPath)
            }
            ForwardAuth::GoogleVertex {
                project_id,
                location,
                ..
            } => {
                let stream = serde_json::from_slice::<Value>(body)
                    .ok()
                    .and_then(|value| value.get("stream").and_then(Value::as_bool))
                    .unwrap_or(false);
                Ok(crate::dialects::google_vertex::generate_content_url(
                    location,
                    project_id,
                    &self.model,
                    stream,
                ))
            }
        }
    }

    /// Attach auth to the request.
    async fn apply_auth(
        &self,
        builder: reqwest::RequestBuilder,
    ) -> Result<reqwest::RequestBuilder, PassthroughError> {
        match &self.auth {
            ForwardAuth::Static(auth) => Ok(match auth {
                Some(auth) => auth.apply(builder),
                None => builder,
            }),
            ForwardAuth::GoogleVertex { token_cache, .. } => {
                let token = token_cache
                    .token()
                    .await
                    .map_err(|err| PassthroughError::Internal(err.to_string()))?;
                Ok(builder.bearer_auth(token))
            }
        }
    }

    /// Forward: raw forwarding + model override +
    /// response_model application (JSON / SSE frames) + usage extraction.
    pub async fn forward(
        &self,
        endpoint: EndpointKind,
        path: &str,
        request_headers: &HeaderMap,
        body: Bytes,
        response_model_mode: &ResponseModelMode,
        routed_model: &str,
        usage_sink: Option<UsageSink>,
    ) -> Result<ForwardResponse, PassthroughError> {
        // Request body model override (equivalent to rewrite_model_in_json_body).
        let body = rewrite_request_model(&body, &self.model).unwrap_or(body);
        let url = self.resolve_url(path, &body)?;

        let builder = shared_http_client()
            .post(url)
            .headers(filter_forward_headers(request_headers));
        let builder = self.apply_auth(builder).await?;
        let response = builder
            .body(body)
            .send()
            .await
            .map_err(PassthroughError::Http)?;

        let status = response.status();
        let headers = response.headers().clone();
        let content_type = headers
            .get("content-type")
            .and_then(|value| value.to_str().ok())
            .unwrap_or_default()
            .to_string();

        if content_type.contains("text/event-stream") {
            let endpoint_path = endpoint.as_path().to_string();
            let stream = apply_mode_to_sse_stream(
                response.bytes_stream(),
                response_model_mode.clone(),
                routed_model.to_string(),
                usage_sink,
                endpoint_path,
            );
            Ok(ForwardResponse {
                status,
                headers,
                body: ForwardBody::Stream(Box::pin(stream)),
                usage: None,
            })
        } else {
            let bytes = response.bytes().await.map_err(PassthroughError::Http)?;
            let usage = extract_usage_bytes(&bytes).and_then(|payload| {
                crate::usage_handler::EndpointUsage::from_endpoint_payload(
                    endpoint.as_path(),
                    payload,
                )
                .ok()
            });
            let bytes = apply_mode_to_json_bytes(&bytes, response_model_mode, routed_model);
            Ok(ForwardResponse {
                status,
                headers,
                body: ForwardBody::Full(bytes),
                usage,
            })
        }
    }
}

fn filter_forward_headers(headers: &HeaderMap) -> HeaderMap {
    let mut filtered = HeaderMap::new();
    for (name, value) in headers {
        if FORWARD_HEADER_BLACKLIST
            .iter()
            .any(|blocked| name.as_str().eq_ignore_ascii_case(blocked))
        {
            continue;
        }
        filtered.append(name.clone(), value.clone());
    }
    filtered
}

/// URL assembly (pure function): strip trailing slash from base; path must start with `/`.
pub fn url_with_base(base: &Url, path: &str) -> Result<String, String> {
    if !path.starts_with('/') {
        return Err(format!("passthrough path must start with '/': {path}"));
    }
    Ok(format!("{}{path}", base.as_str().trim_end_matches('/')))
}

/// serve list validation: every path must start with `/` and not be the root.
/// Unknown paths / chat-endpoint hijacking are handled by the registry.
pub fn validate_serve_paths(serve: &[String]) -> Result<(), String> {
    for path in serve {
        if !path.starts_with('/') || path.trim() == "/" {
            return Err(format!("invalid passthrough serve path: {path}"));
        }
    }
    Ok(())
}

/// Parse a `/models/:generateContent` endpoint.
pub fn parse_generate_content_model(endpoint: &str) -> Option<String> {
    endpoint
        .strip_prefix("/models/")
        .and_then(|value| value.strip_suffix(":generateContent"))
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_string)
}

/// build_client always errors for passthrough: dialects serve chat endpoints,
/// passthrough serves its serve list through the forwarder.
pub const PASSTHROUGH_NOT_A_CHAT_KIND: &str =
    "passthrough does not serve chat endpoints (use PassthroughForwarder)";

// ---------------------------------------------------------------------------
// Pure functions for request and response body rewriting.
// ---------------------------------------------------------------------------

/// Request body model override: insert/replace the `model` field in a JSON body.
/// Returns None for non-JSON bodies or failed replacement (forward as-is).
pub fn rewrite_request_model(body: &Bytes, model: &str) -> Option<Bytes> {
    let mut payload: Value = serde_json::from_slice(body).ok()?;
    let obj = payload.as_object_mut()?;
    obj.insert("model".to_string(), Value::String(model.to_string()));
    serde_json::to_vec(&payload).ok().map(Bytes::from)
}

/// Non-streaming response: apply response_model rewrite (JSON).
pub fn apply_mode_to_json_bytes(
    body: &Bytes,
    mode: &ResponseModelMode,
    routed_model: &str,
) -> Bytes {
    let Ok(mut payload) = serde_json::from_slice::<Value>(body) else {
        return body.clone();
    };
    if apply_response_model_mode_to_json(&mut payload, mode, routed_model) {
        serde_json::to_vec(&payload)
            .map(Bytes::from)
            .unwrap_or_else(|_| body.clone())
    } else {
        body.clone()
    }
}

/// Apply the response_model rewrite to the model / response.model /
/// message.model fields.
pub fn apply_response_model_mode_to_json(
    payload_json: &mut Value,
    mode: &ResponseModelMode,
    _routed_model: &str,
) -> bool {
    let replacement = match mode {
        ResponseModelMode::Upstream => return false,
        ResponseModelMode::Fixed(model) => model.clone(),
    };

    let mut changed = false;
    let Some(obj) = payload_json.as_object_mut() else {
        return false;
    };

    if let Some(model) = obj.get_mut("model") {
        *model = Value::String(replacement.clone());
        changed = true;
    }

    if let Some(response) = obj.get_mut("response").and_then(Value::as_object_mut) {
        if response.contains_key("model") {
            response.insert("model".to_string(), Value::String(replacement.clone()));
            changed = true;
        }
    }

    // Anthropic /messages streaming frames nest the model under "message"
    // (e.g. message_start: {"type":"message_start","message":{"model":...}}).
    if let Some(message) = obj.get_mut("message").and_then(Value::as_object_mut) {
        if message.contains_key("model") {
            message.insert("model".to_string(), Value::String(replacement));
            changed = true;
        }
    }

    changed
}

/// Parse an SSE data section: merge multi-line data; [DONE] yields None.
pub fn parse_sse_data_json(frame: &str) -> Option<Value> {
    let mut payload = String::new();
    for line in frame.lines() {
        if let Some(data) = line.strip_prefix("data:") {
            if !payload.is_empty() {
                payload.push('\n');
            }
            payload.push_str(data.trim_start());
        }
    }
    let payload = payload.trim();
    if payload.is_empty() || payload == "[DONE]" {
        return None;
    }
    serde_json::from_str(payload).ok()
}

/// Rebuild an SSE frame, keeping non-data lines and replacing the data section.
pub fn rebuild_sse_frame_with_data(frame: &str, json_payload: &str) -> String {
    let mut lines = Vec::new();
    for line in frame.lines() {
        if !line.starts_with("data:") {
            lines.push(line.to_string());
        }
    }
    lines.push(format!("data: {json_payload}"));
    format!("{}\n\n", lines.join("\n"))
}

/// Extract the usage object from a response JSON
/// (usage / response.usage / message.usage / usageMetadata / response.usageMetadata).
pub fn extract_usage_value(payload: &Value) -> Option<Value> {
    let non_null = |value: Option<&Value>| -> Option<Value> {
        value.filter(|usage| !usage.is_null()).cloned()
    };
    non_null(payload.get("usage"))
        .or_else(|| non_null(payload.get("response").and_then(|r| r.get("usage"))))
        .or_else(|| non_null(payload.get("message").and_then(|m| m.get("usage"))))
        .or_else(|| non_null(payload.get("usageMetadata")))
        .or_else(|| non_null(payload.get("response").and_then(|r| r.get("usageMetadata"))))
}

fn extract_usage_bytes(body: &Bytes) -> Option<Value> {
    let payload: Value = serde_json::from_slice(body).ok()?;
    extract_usage_value(&payload)
}

/// SSE stream: cross-chunk frame buffering + per-frame response_model rewrite + usage extraction callback.
fn apply_mode_to_sse_stream(
    stream: impl Stream<Item = Result<axum::body::Bytes, reqwest::Error>> + Send + 'static,
    mode: ResponseModelMode,
    routed_model: String,
    usage_sink: Option<UsageSink>,
    endpoint_path: String,
) -> Pin<Box<dyn Stream<Item = Result<Bytes, std::io::Error>> + Send>> {
    use futures_util::StreamExt;
    Box::pin(async_stream::try_stream! {
        futures_util::pin_mut!(stream);
        let mut buffer = String::new();
        while let Some(chunk) = stream.next().await {
            let chunk = chunk.map_err(|err| std::io::Error::other(err.to_string()))?;
            buffer.push_str(&String::from_utf8_lossy(&chunk));

            while let Some(pos) = buffer.find("\n\n") {
                let frame = buffer[..pos + 2].to_string();
                if let Some(mut payload) = parse_sse_data_json(&frame) {
                    let changed =
                        apply_response_model_mode_to_json(&mut payload, &mode, &routed_model);
                    if let Some(sink) = &usage_sink {
                        if let Some(usage) = extract_usage_value(&payload) {
                            if let Ok(usage) = crate::usage_handler::EndpointUsage::from_endpoint_payload(
                                &endpoint_path,
                                usage,
                            ) {
                                sink(usage);
                            }
                        }
                    }
                    if changed {
                        let json_payload = payload.to_string();
                        yield Bytes::from(rebuild_sse_frame_with_data(&frame, &json_payload));
                        buffer.drain(..pos + 2);
                        continue;
                    }
                }
                // forward the frame unchanged
                yield Bytes::from(frame.to_string());
                buffer.drain(..pos + 2);
            }
        }
        // forward remaining non-frame data unchanged
        if !buffer.is_empty() {
            yield Bytes::from(buffer);
        }
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::serve_config::AuthScheme;
    use axum::http::{HeaderMap, HeaderValue};
    use secrecy::SecretString;

    fn params_with_auth() -> PassthroughParams {
        PassthroughParams {
            base_url: Some(Url::parse("http://115.190.156.86:8001/v1").expect("url")),
            auth: Some(crate::serve_config::AuthConfig {
                scheme: AuthScheme::Bearer,
                api_key: SecretString::from("sk-passthrough"),
            }),
            google_vertex: None,
            model: "BAAI/bge-m3".to_string(),
        }
    }

    #[test]
    fn forwarder_rejects_invalid_param_combinations() {
        let base_only = PassthroughParams {
            base_url: Some(Url::parse("http://127.0.0.1:8001/v1").expect("url")),
            auth: None,
            google_vertex: None,
            model: "m".to_string(),
        };
        assert!(PassthroughForwarder::new(&base_only).is_ok());

        let both = PassthroughParams {
            base_url: Some(Url::parse("http://127.0.0.1:8001/v1").expect("url")),
            auth: None,
            google_vertex: Some(crate::serve_config::GoogleVertexPassthroughAuth {
                project_id: "pc".to_string(),
                location: "global".to_string(),
                credentials_file: "/tmp/sa.json".into(),
            }),
            model: "m".to_string(),
        };
        assert!(PassthroughForwarder::new(&both).is_err());

        let neither = PassthroughParams {
            base_url: None,
            auth: None,
            google_vertex: None,
            model: "m".to_string(),
        };
        assert!(PassthroughForwarder::new(&neither).is_err());
    }

    #[tokio::test]
    async fn apply_auth_adds_bearer_header() {
        let forwarder = PassthroughForwarder::new(&params_with_auth()).expect("forwarder");
        let request = forwarder
            .apply_auth(reqwest::Client::new().post("http://example.com/embeddings"))
            .await
            .expect("apply auth")
            .build()
            .expect("build request");
        assert_eq!(
            request.headers().get("authorization").unwrap(),
            "Bearer sk-passthrough"
        );
    }

    #[tokio::test]
    async fn no_auth_leaves_request_untouched() {
        let params = PassthroughParams {
            base_url: Some(Url::parse("http://127.0.0.1:8001/v1").expect("url")),
            auth: None,
            google_vertex: None,
            model: "bge".to_string(),
        };
        let forwarder = PassthroughForwarder::new(&params).expect("forwarder");
        let request = forwarder
            .apply_auth(reqwest::Client::new().post("http://example.com/embeddings"))
            .await
            .expect("apply auth")
            .build()
            .expect("build request");
        assert!(request.headers().get("authorization").is_none());
    }

    #[test]
    fn filter_forward_headers_keeps_business_headers_and_user_agent() {
        let mut headers = HeaderMap::new();
        headers.insert("x-client-id", HeaderValue::from_static("abc"));
        headers.insert("user-agent", HeaderValue::from_static("byoc-client/1.0"));
        headers.insert("authorization", HeaderValue::from_static("Bearer upstream"));
        headers.insert("cookie", HeaderValue::from_static("a=b"));
        headers.insert("connection", HeaderValue::from_static("keep-alive"));

        let filtered = filter_forward_headers(&headers);
        assert_eq!(
            filtered.get("x-client-id"),
            Some(&HeaderValue::from_static("abc"))
        );
        assert_eq!(
            filtered.get("user-agent"),
            Some(&HeaderValue::from_static("byoc-client/1.0"))
        );
        assert!(filtered.get("authorization").is_none());
        assert!(filtered.get("cookie").is_none());
        assert!(filtered.get("connection").is_none());
    }

    #[test]
    fn filter_forward_headers_keeps_non_blacklisted_headers() {
        let mut headers = HeaderMap::new();
        headers.insert("x-tenant", HeaderValue::from_static("t-1"));
        headers.insert("accept", HeaderValue::from_static("application/json"));
        headers.insert("cookie", HeaderValue::from_static("a=b"));

        let filtered = filter_forward_headers(&headers);
        assert_eq!(
            filtered.get("x-tenant"),
            Some(&HeaderValue::from_static("t-1"))
        );
        assert_eq!(
            filtered.get("accept"),
            Some(&HeaderValue::from_static("application/json"))
        );
        assert!(filtered.get("cookie").is_none());
    }

    #[test]
    fn validate_serve_paths_rejects_invalid() {
        assert!(validate_serve_paths(&["/embeddings".to_string()]).is_ok());
        assert!(validate_serve_paths(&["embeddings".to_string()]).is_err());
        assert!(validate_serve_paths(&["/".to_string()]).is_err());
    }

    #[test]
    fn parse_generate_content_model_extracts_model_name() {
        assert_eq!(
            parse_generate_content_model("/models/gemini-3-pro-image:generateContent"),
            Some("gemini-3-pro-image".to_string())
        );
        assert_eq!(
            parse_generate_content_model("/models/:generateContent"),
            None
        );
        assert_eq!(parse_generate_content_model("/embeddings"), None);
    }

    #[test]
    fn rewrite_request_model_inserts_and_replaces() {
        let body = Bytes::from(r#"{"input": [1,2]}"#);
        let rewritten = rewrite_request_model(&body, "BAAI/bge-m3").expect("rewrite");
        let value: Value = serde_json::from_slice(&rewritten).expect("json");
        assert_eq!(value["model"], "BAAI/bge-m3");
        assert_eq!(value["input"], serde_json::json!([1, 2]));

        let body = Bytes::from(r#"{"model": "old", "input": "x"}"#);
        let rewritten = rewrite_request_model(&body, "new-model").expect("rewrite");
        let value: Value = serde_json::from_slice(&rewritten).expect("json");
        assert_eq!(value["model"], "new-model");

        assert!(rewrite_request_model(&Bytes::from("not-json"), "m").is_none());
    }

    #[test]
    fn apply_response_model_mode_updates_model_fields() {
        let mut payload = serde_json::json!({
            "model": "upstream",
            "response": {"model": "upstream"},
            "message": {"model": "upstream", "role": "assistant"}
        });
        assert!(apply_response_model_mode_to_json(
            &mut payload,
            &ResponseModelMode::Fixed("fixed-model".to_string()),
            "routed"
        ));
        assert_eq!(payload["model"], "fixed-model");
        assert_eq!(payload["response"]["model"], "fixed-model");
        assert_eq!(payload["message"]["model"], "fixed-model");

        // upstream mode: no rewrite
        let mut payload = serde_json::json!({"model": "upstream"});
        assert!(!apply_response_model_mode_to_json(
            &mut payload,
            &ResponseModelMode::Upstream,
            "routed"
        ));
        assert_eq!(payload["model"], "upstream");
    }

    #[test]
    fn sse_frame_parse_and_rebuild_roundtrip() {
        let frame = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"model\":\"up\"}}\n\n";
        let payload = parse_sse_data_json(frame).expect("payload");
        assert_eq!(payload["message"]["model"], "up");
        let rebuilt = rebuild_sse_frame_with_data(frame, &payload.to_string());
        assert!(rebuilt.starts_with("event: message_start\n"));
        assert!(rebuilt.ends_with("\n\n"));
        assert!(parse_sse_data_json("data: [DONE]\n\n").is_none());
    }

    #[test]
    fn extract_usage_checks_three_nesting_shapes() {
        assert_eq!(
            extract_usage_value(&serde_json::json!({"usage": {"total_tokens": 1}})),
            Some(serde_json::json!({"total_tokens": 1}))
        );
        assert_eq!(
            extract_usage_value(&serde_json::json!({"response": {"usage": {"total_tokens": 2}}})),
            Some(serde_json::json!({"total_tokens": 2}))
        );
        assert_eq!(
            extract_usage_value(&serde_json::json!({"message": {"usage": {"total_tokens": 3}}})),
            Some(serde_json::json!({"total_tokens": 3}))
        );
        assert_eq!(
            extract_usage_value(&serde_json::json!({"usage": null})),
            None
        );
        assert_eq!(
            extract_usage_value(&serde_json::json!({"usageMetadata": {"total_token_count": 4}})),
            Some(serde_json::json!({"total_token_count": 4}))
        );
        assert_eq!(
            extract_usage_value(
                &serde_json::json!({"response": {"usageMetadata": {"total_token_count": 5}}})
            ),
            Some(serde_json::json!({"total_token_count": 5}))
        );
    }

    #[tokio::test]
    async fn sse_stream_applies_mode_and_extracts_usage() {
        use futures_util::StreamExt;
        // Two frames: one carrying model (rewritten) + usage, one passed through.
        let wire = concat!(
            "data: {\"model\":\"up\",\"usage\":{\"total_tokens\":7}}\n\n",
            "data: {\"plain\":true}\n\n",
        );
        let stream = futures_util::stream::once(async move {
            Ok::<axum::body::Bytes, reqwest::Error>(axum::body::Bytes::from(wire))
        });
        let seen = Arc::new(std::sync::Mutex::new(Vec::new()));
        let seen_sink = seen.clone();
        let sink: UsageSink = Arc::new(move |usage| {
            seen_sink.lock().expect("lock").push(usage);
        });

        let mut stream = apply_mode_to_sse_stream(
            stream,
            ResponseModelMode::Fixed("fixed-model".to_string()),
            "routed".to_string(),
            Some(sink),
            "/embeddings".to_string(),
        );
        let mut output = String::new();
        while let Some(chunk) = stream.next().await {
            output.push_str(&String::from_utf8_lossy(&chunk.expect("chunk")));
        }

        assert!(output.contains("fixed-model"));
        assert!(output.contains("\"plain\":true"));
        let usages = seen.lock().expect("lock");
        assert_eq!(usages.len(), 1);
        assert!(matches!(
            usages[0],
            crate::usage_handler::EndpointUsage::Embeddings(_)
        ));
    }
}
