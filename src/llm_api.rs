use std::{path::PathBuf, sync::Arc};

use anyhow::{Result, anyhow, bail};
use axum::{Json, extract::State, http::StatusCode, response::IntoResponse, routing::post};
use log::info;
use serde_json::{Value, json};
use tokio::{fs, io::AsyncWriteExt, net::TcpListener};

use crate::{
    connector::{Connector, MemoryConnector},
    io::{receive_bytes, receive_frame, send_bytes, send_frame},
    types::{BodyFormat, RequestHeaders, ResponseHeaders},
};

/// Custom error with HTTP status code
pub struct CustomError {
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

#[derive(Clone)]
struct LlmApiState {
    connector: Arc<MemoryConnector>,
    schema_dir: PathBuf,
    base_url: String,
    api_key: String,
    client: reqwest::Client,
}

fn tool_name_from_path(path: &std::path::Path) -> String {
    path.file_stem()
        .and_then(|s| s.to_str())
        .unwrap_or("tool")
        .to_string()
}

fn extract_tools_from_schema(path: &std::path::Path, schema: Value) -> Option<Value> {
    let obj = schema.as_object()?;
    let fallback_name = tool_name_from_path(path);
    let name = obj
        .get("name")
        .and_then(|v| v.as_str())
        .unwrap_or(&fallback_name)
        .to_string();

    let description = obj
        .get("description")
        .and_then(|v| v.as_str())
        .unwrap_or_default();

    let parameters = obj.get("parameters").cloned().unwrap_or_else(|| json!({}));

    Some(json!({
        "type": "function",
        "function": {
            "name": name,
            "description": description,
            "parameters": parameters,
        }
    }))
}

async fn load_tools(schema_dir: &PathBuf) -> Result<Vec<Value>> {
    let mut tools = Vec::new();
    let mut entries = fs::read_dir(schema_dir).await?;
    while let Some(entry) = entries.next_entry().await? {
        let path = entry.path();
        if path.extension().and_then(|s| s.to_str()) != Some("json") {
            continue;
        }

        let text = fs::read_to_string(&path).await?;
        let schema: Value = serde_json::from_str(&text)?;
        if let Some(tool) = extract_tools_from_schema(&path, schema) {
            tools.push(tool);
        }
    }

    Ok(tools)
}

async fn call_llm(state: &LlmApiState, payload: &Value) -> Result<Value> {
    let url = format!(
        "{}/v1/chat/completions",
        state.base_url.trim_end_matches('/')
    );
    let res = state
        .client
        .post(url)
        .bearer_auth(&state.api_key)
        .json(payload)
        .send()
        .await?;

    let status = res.status();
    let text = res.text().await?;
    if !status.is_success() {
        bail!("llm api error [{}]: {}", status, text);
    }

    Ok(serde_json::from_str(&text)?)
}

fn response_message(response: &Value) -> Option<Value> {
    response
        .get("choices")
        .and_then(|v| v.as_array())
        .and_then(|arr| arr.first())
        .and_then(|v| v.get("message"))
        .cloned()
}

fn response_finish_reason(response: &Value) -> Option<String> {
    response
        .get("choices")
        .and_then(|v| v.as_array())
        .and_then(|arr| arr.first())
        .and_then(|v| v.get("finish_reason"))
        .and_then(|v| v.as_str())
        .map(ToOwned::to_owned)
}

async fn invoke_tool(connector: &MemoryConnector, tool_name: &str, args: Value) -> Result<Value> {
    let request_headers = RequestHeaders {
        name: tool_name.to_owned(),
        trace_id: "llm-api".to_string(),
        span_id: format!("tool-{}", tool_name),
        body_format: BodyFormat::Bytes,
        extension: String::new(),
    };

    let (mut reader, mut writer) = connector.open_new_stream().await?;

    send_frame(&mut writer, &request_headers).await?;
    let body = serde_json::to_vec(&json!({"args": args}))?;
    send_bytes(&mut writer, &body).await?;
    writer.shutdown().await?;

    let response_headers: ResponseHeaders = receive_frame(&mut reader)
        .await?
        .ok_or(anyhow!("Failed to receive response headers"))?;
    if response_headers.status_code != StatusCode::OK.as_u16() {
        bail!("tool invocation failed: {}", response_headers.error_msg);
    }

    let body = receive_bytes(&mut reader)
        .await?
        .ok_or(anyhow!("Failed to receive tool response"))?;
    let result: Value = serde_json::from_slice(&body)?;
    Ok(result)
}

async fn chat_completions_handler(
    State(state): State<Arc<LlmApiState>>,
    Json(req): Json<Value>,
) -> Result<Json<Value>, CustomError> {
    let mut payload = req.clone();
    let tools = load_tools(&state.schema_dir).await?;
    if !tools.is_empty() {
        payload["tools"] = Value::Array(tools.clone());
        payload["tool_choice"] = Value::String("auto".to_string());
    }

    let first = call_llm(&state, &payload).await?;
    if response_finish_reason(&first).as_deref() != Some("tool_calls") {
        return Ok(Json(first));
    }

    let assistant_msg = response_message(&first).ok_or(anyhow!("llm response has no message"))?;
    let tool_calls = assistant_msg
        .get("tool_calls")
        .and_then(|v| v.as_array())
        .ok_or(anyhow!("finish_reason=tool_calls but tool_calls missing"))?;

    let mut messages = req
        .get("messages")
        .and_then(|v| v.as_array())
        .cloned()
        .ok_or(anyhow!("request.messages is required"))?;
    messages.push(assistant_msg.clone());

    for tc in tool_calls {
        let tool_call_id = tc
            .get("id")
            .and_then(|v| v.as_str())
            .ok_or(anyhow!("tool_call.id missing"))?;
        let func = tc
            .get("function")
            .ok_or(anyhow!("tool_call.function missing"))?;
        let name = func
            .get("name")
            .and_then(|v| v.as_str())
            .ok_or(anyhow!("tool_call.function.name missing"))?;
        let args_str = func
            .get("arguments")
            .and_then(|v| v.as_str())
            .ok_or(anyhow!("tool_call.function.arguments missing"))?;

        let args: Value = serde_json::from_str(args_str)?;
        let tool_result = invoke_tool(&state.connector, name, args).await?;

        messages.push(json!({
            "role": "tool",
            "tool_call_id": tool_call_id,
            "content": tool_result.to_string(),
        }));
    }

    let mut second_payload = req.clone();
    second_payload["messages"] = Value::Array(messages);
    if !tools.is_empty() {
        second_payload["tools"] = Value::Array(tools);
        second_payload["tool_choice"] = Value::String("auto".to_string());
    }

    let second = call_llm(&state, &second_payload).await?;
    Ok(Json(second))
}

/// LLM API server: OpenAI-compatible /v1/chat/completions endpoint
pub async fn serve_llm_api(
    host: &str,
    port: u16,
    connector: MemoryConnector,
    schema_dir: PathBuf,
    base_url: String,
    api_key: String,
) -> Result<()> {
    let state = Arc::new(LlmApiState {
        connector: Arc::new(connector),
        schema_dir,
        base_url,
        api_key,
        client: reqwest::Client::new(),
    });

    let app = axum::Router::new()
        .route("/v1/chat/completions", post(chat_completions_handler))
        .with_state(state);

    let listener = TcpListener::bind((host.to_owned(), port)).await?;
    info!("start llm api server: {}:{}", host, port);
    axum::serve(listener, app).await?;
    Ok(())
}
