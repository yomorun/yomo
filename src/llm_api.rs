use std::sync::Arc;

use anyhow::{Result, anyhow, bail};
use axum::{Json, extract::State, http::StatusCode, response::IntoResponse, routing::post};
use log::{debug, error, info};
use serde_json::{Value, json};
use tokio::{io::AsyncWriteExt, net::TcpListener};

use crate::{
    connector::{Connector, MemoryConnector},
    io::{receive_frame, send_frame},
    metadata::Metadata,
    tool_mgr::ToolMgr,
    types::{BodyFormat, RequestHeaders, ResponseHeaders, ToolRequest, ToolResponse},
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
    tool_mgr: Arc<dyn ToolMgr>,
    base_url: String,
    api_key: String,
    client: reqwest::Client,
}

async fn load_tools(tool_mgr: &Arc<dyn ToolMgr>) -> Result<Vec<Value>> {
    Ok(tool_mgr
        .list_tools(&Metadata::default())
        .await?
        .into_iter()
        .filter_map(|(name, tool)| match serde_json::from_str::<Value>(&tool) {
            Ok(v) => {
                if let Some(description) = v.get("description") {
                    Some(json!({
                        "type": "function",
                        "function": {
                            "name": Value::String(name),
                            "description": description.as_str(),
                            "parameters": v.get("parameters").cloned().unwrap_or(json!({})),
                        }
                    }))
                } else {
                    None
                }
            }
            Err(e) => {
                error!("failed to parse tool {}: {}", name, e);
                None
            }
        })
        .collect())
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

async fn invoke_tool(
    connector: &MemoryConnector,
    tool_name: &str,
    tool_request: &ToolRequest,
) -> Result<ToolResponse> {
    debug!("invoke tool: {}", tool_name);

    let request_headers = RequestHeaders {
        name: tool_name.to_owned(),
        trace_id: "llm-api".to_string(),
        span_id: format!("tool-{}", tool_name),
        body_format: BodyFormat::Bytes,
        ..Default::default()
    };

    let (mut reader, mut writer) = connector.open_new_stream().await?;

    send_frame(&mut writer, &request_headers).await?;
    send_frame(&mut writer, &tool_request).await?;
    writer.shutdown().await?;

    let response_headers: ResponseHeaders = receive_frame(&mut reader)
        .await?
        .ok_or(anyhow!("Failed to receive response headers"))?;
    if response_headers.status_code != StatusCode::OK.as_u16() {
        bail!("tool invocation failed: {}", response_headers.error_msg);
    }

    let result = receive_frame(&mut reader)
        .await?
        .ok_or(anyhow!("Failed to receive tool response"))?;

    debug!("tool response received: {}", tool_name);

    Ok(result)
}

async fn chat_completions_handler(
    State(state): State<Arc<LlmApiState>>,
    Json(req): Json<Value>,
) -> Result<Json<Value>, CustomError> {
    let message_count = req
        .get("messages")
        .and_then(|v| v.as_array())
        .map_or(0, |messages| messages.len());
    info!(
        "chat_completions request received: messages={}",
        message_count
    );

    let mut payload = req.clone();
    let tools = load_tools(&state.tool_mgr).await?;
    if !tools.is_empty() {
        payload["tools"] = Value::Array(tools.clone());
        payload["tool_choice"] = Value::String("auto".to_string());
    }

    debug!("first llm request prepared");
    let first = call_llm(&state, &payload).await?;
    debug!("first llm response received");

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

    let agent_context = req.get("agent_context").and_then(|v| Some(v.to_string()));

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
        let args = func
            .get("arguments")
            .and_then(|v| v.as_str())
            .ok_or(anyhow!("tool_call.function.arguments missing"))?
            .to_string();

        let tool_response = invoke_tool(
            &state.connector,
            name,
            &ToolRequest {
                args,
                agent_context: agent_context.to_owned(),
            },
        )
        .await?;

        if let Some(result) = tool_response.result {
            messages.push(json!({
                "role": "tool",
                "tool_call_id": tool_call_id,
                "content": result,
            }));
        } else if let Some(error_msg) = tool_response.error_msg {
            messages.push(json!({
                "role": "tool",
                "tool_call_id": tool_call_id,
                "content": format!("tool_call error: {}", error_msg),
            }));
        }
    }

    let mut second_payload = req.clone();
    second_payload["messages"] = Value::Array(messages);
    if !tools.is_empty() {
        second_payload["tools"] = Value::Array(tools);
        second_payload["tool_choice"] = Value::String("auto".to_string());
    }

    debug!("second llm request prepared");
    let second = call_llm(&state, &second_payload).await?;
    debug!("second llm response received");

    Ok(Json(second))
}

/// LLM API server: OpenAI-compatible /v1/chat/completions endpoint
pub async fn serve_llm_api(
    host: &str,
    port: u16,
    connector: MemoryConnector,
    tool_mgr: Arc<dyn ToolMgr>,
    base_url: String,
    api_key: String,
) -> Result<()> {
    let state = Arc::new(LlmApiState {
        connector: Arc::new(connector),
        tool_mgr,
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
