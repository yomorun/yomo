use std::collections::HashMap;
use std::fmt;
use std::pin::Pin;
use std::sync::Arc;

use crate::tool_invoker::ToolInvoker;
use crate::types::{BodyFormat, RequestHeaders, ToolRequest};
use futures_core::Stream;
use futures_util::{StreamExt, future::join_all};
use log::{error, info};
use opentelemetry::KeyValue;
use opentelemetry::global;
use opentelemetry::trace::{Span as OtelSpan, Status, Tracer};
use serde::Serialize;
use serde_json::Value;
use tracing::Span;
use tracing_opentelemetry::OpenTelemetrySpanExt;

use crate::llm_provider::openai::mapper::ensure_tool_call_id;
use crate::llm_provider::{
    Provider, ProviderError, ToolCall as ProviderToolCall, UnifiedEvent, UnifiedResponse, Usage,
};
use crate::openai_types::{ChatCompletionRequest, Content, Message, Role, ToolDefinition};

pub struct AgentLoopConfig {
    pub max_calls: usize,
}

impl Default for AgentLoopConfig {
    fn default() -> Self {
        Self { max_calls: 14 }
    }
}

pub enum AgentLoopResult {
    NonStream(UnifiedResponse),
    Stream {
        events: Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send>>,
    },
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum ToolSource {
    Server,
    Client,
}

struct ToolMaps {
    merged_tools: Option<Vec<ToolDefinition>>,
    source_map: HashMap<String, ToolSource>,
}

pub async fn run_agent_loop<A, M>(
    provider: Arc<dyn Provider>,
    request: ChatCompletionRequest,
    server_tools: HashMap<String, String>,
    invoker: Arc<dyn ToolInvoker>,
    metadata: M,
    trace_id: String,
    config: AgentLoopConfig,
) -> Result<AgentLoopResult, ProviderError>
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Serialize + Send + Sync + 'static,
{
    let server_tools = Arc::new(server_tools);
    let metadata = Arc::new(metadata);
    if request.stream.unwrap_or(false) {
        run_agent_loop_stream::<A, M>(
            provider,
            request,
            Arc::clone(&server_tools),
            invoker,
            metadata,
            trace_id,
            config,
        )
        .await
    } else {
        run_agent_loop_nonstream::<A, M>(
            provider,
            request,
            Arc::clone(&server_tools),
            invoker,
            metadata,
            trace_id,
            config,
        )
        .await
    }
}

async fn run_agent_loop_nonstream<A, M>(
    provider: Arc<dyn Provider>,
    mut request: ChatCompletionRequest,
    server_tools: Arc<HashMap<String, String>>,
    invoker: Arc<dyn ToolInvoker>,
    metadata: Arc<M>,
    trace_id: String,
    config: AgentLoopConfig,
) -> Result<AgentLoopResult, ProviderError>
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Serialize + Send + Sync + 'static,
{
    let mut call_count = 0usize;
    let mut total_usage = Usage {
        input_tokens: 0,
        output_tokens: 0,
        total_tokens: 0,
        cached_tokens: None,
        reasoning_tokens: None,
    };
    loop {
        let tool_maps = build_tool_maps(&request, server_tools.as_ref())?;
        request.tools = tool_maps.merged_tools.clone();
        if call_count > 0 {
            request.tool_choice = None;
        }

        let mut response = provider
            .complete(request.clone())
            .await
            .map_err(|err| ProviderError::Internal(err.to_string()))?;
        add_usage(&mut total_usage, &response.usage);
        call_count += 1;

        if call_count >= config.max_calls {
            response.usage = total_usage;
            return Ok(AgentLoopResult::NonStream(response));
        }

        let mut tool_calls = response.tool_calls.take().unwrap_or_default();
        ensure_provider_call_ids(&response.request_id, &mut tool_calls);
        log_llm_call(
            call_count,
            &request.model,
            false,
            tool_calls.len(),
            Some(&response.usage),
            &trace_id,
        );
        if tool_calls.is_empty() {
            response.tool_calls = None;
            response.usage = total_usage;
            return Ok(AgentLoopResult::NonStream(response));
        }

        let (server_calls, client_calls) = split_tool_calls(&tool_calls, &tool_maps.source_map);
        if server_calls.is_empty() {
            response.tool_calls = Some(client_calls);
            response.usage = total_usage;
            return Ok(AgentLoopResult::NonStream(response));
        }
        if !client_calls.is_empty() {
            response.tool_calls = Some(client_calls);
            response.usage = total_usage;
            return Ok(AgentLoopResult::NonStream(response));
        }

        let request_id = response.request_id.clone();
        let next_messages = async {
            let mut next_messages = Vec::new();
            next_messages.push(build_assistant_tool_call_message(
                &request_id,
                &server_calls,
            ));
            next_messages.extend(
                build_tool_messages::<M>(
                    &request_id,
                    &server_calls,
                    invoker.clone(),
                    metadata.clone(),
                    trace_id.clone(),
                    request.agent_context.clone(),
                )
                .await?,
            );
            Ok::<Vec<Message>, ProviderError>(next_messages)
        }
        .await?;
        request.messages.extend(next_messages);
    }
}

async fn run_agent_loop_stream<A, M>(
    provider: Arc<dyn Provider>,
    mut request: ChatCompletionRequest,
    server_tools: Arc<HashMap<String, String>>,
    invoker: Arc<dyn ToolInvoker>,
    metadata: Arc<M>,
    trace_id: String,
    config: AgentLoopConfig,
) -> Result<AgentLoopResult, ProviderError>
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Serialize + Send + Sync + 'static,
{
    let stream = async_stream::try_stream! {
        let mut call_count = 0usize;
        let mut total_usage = Usage {
            input_tokens: 0,
            output_tokens: 0,
            total_tokens: 0,
            cached_tokens: None,
            reasoning_tokens: None,
        };
        let mut last_finish_reason: Option<String> = None;
        let model_id = request.model.clone();
        loop {
        let tool_maps = build_tool_maps(&request, server_tools.as_ref())?;
        request.tools = tool_maps.merged_tools.clone();
        if call_count > 0 {
            request.tool_choice = None;
            }

            let mut provider_stream = provider.stream(request.clone());

            let usage_offset = total_usage.clone();

            let mut tool_calls: Vec<ProviderToolCall> = Vec::new();
            let mut tool_call_index: HashMap<String, usize> = HashMap::new();
            let mut usage = None;
            let mut finish_reason = None;
            let mut saw_tool_call = false;
            let mut emitted_client_tool = false;

            while let Some(item) = provider_stream.next().await {
                let event = item?;
                match &event {
                    UnifiedEvent::ToolCallDelta { id, name, arguments_delta } => {
                        saw_tool_call = true;
                        let index = *tool_call_index.entry(id.clone()).or_insert_with(|| {
                            let current = tool_calls.len();
                            tool_calls.push(ProviderToolCall {
                                id: Some(id.clone()),
                                name: name.clone(),
                                description: String::new(),
                                arguments: String::new(),
                            });
                            current
                        });
                        let is_server = matches!(
                            tool_maps
                                .source_map
                                .get(name)
                                .copied()
                                .unwrap_or(ToolSource::Client),
                            ToolSource::Server
                        );
                        if is_server {
                            if let Some(call) = tool_calls.get_mut(index) {
                                call.name = name.clone();
                                call.arguments.push_str(arguments_delta);
                            }
                        } else {
                            if let Some(call) = tool_calls.get_mut(index) {
                                call.name = name.clone();
                                call.arguments.push_str(arguments_delta);
                            }
                            emitted_client_tool = true;
                            yield UnifiedEvent::ToolCallDelta {
                                id: id.clone(),
                                name: name.clone(),
                                arguments_delta: arguments_delta.clone(),
                            };
                        }
                    }
                    UnifiedEvent::ToolCallDone { id, name, arguments } => {
                        saw_tool_call = true;
                        let index = *tool_call_index.entry(id.clone()).or_insert_with(|| {
                            let current = tool_calls.len();
                            tool_calls.push(ProviderToolCall {
                                id: Some(id.clone()),
                                name: name.clone(),
                                description: String::new(),
                                arguments: String::new(),
                            });
                            current
                        });
                        let is_server = matches!(
                            tool_maps
                                .source_map
                                .get(name)
                                .copied()
                                .unwrap_or(ToolSource::Client),
                            ToolSource::Server
                        );
                        if is_server {
                            if let Some(call) = tool_calls.get_mut(index) {
                                call.name = name.clone();
                                call.arguments = arguments.clone();
                            }
                        } else {
                            if let Some(call) = tool_calls.get_mut(index) {
                                call.name = name.clone();
                                call.arguments = arguments.clone();
                            }
                            emitted_client_tool = true;
                            yield UnifiedEvent::ToolCallDone {
                                id: id.clone(),
                                name: name.clone(),
                                arguments: arguments.clone(),
                            };
                        }
                    }
                    UnifiedEvent::Usage { usage: chunk_usage } => {
                        usage = Some(chunk_usage.clone());
                    }
                    UnifiedEvent::Completed { finish_reason: reason, usage: chunk_usage } => {
                        finish_reason = reason.clone();
                        last_finish_reason = reason.clone();
                        if let Some(chunk_usage) = chunk_usage {
                            usage = Some(chunk_usage.clone());
                        }
                    }
                    _ => {}
                }

                if !saw_tool_call {
                    match event {
                        UnifiedEvent::Usage { usage: chunk_usage } => {
                            let usage = add_usage_cloned(&usage_offset, &chunk_usage);
                            yield UnifiedEvent::Usage { usage };
                        }
                        UnifiedEvent::Completed { finish_reason, usage: chunk_usage } => {
                            let usage = chunk_usage.map(|chunk_usage| {
                                add_usage_cloned(&usage_offset, &chunk_usage)
                            });
                            yield UnifiedEvent::Completed { finish_reason, usage };
                        }
                        _ => {
                            yield event;
                        }
                    }
                }
            }

            call_count += 1;
            if let Some(current_usage) = &usage {
                add_usage(&mut total_usage, current_usage);
            }
            log_llm_call(
                call_count,
                &request.model,
                true,
                tool_calls.len(),
                usage.as_ref(),
                &trace_id,
            );
            if call_count >= config.max_calls {
                if !tool_calls.is_empty() {
                    if !emitted_client_tool {
                        let events = build_client_tool_events(
                            &Some(total_usage.clone()),
                            &finish_reason,
                            &tool_calls,
                        );
                        for event in events {
                            yield event;
                        }
                    } else if let Some(completed) =
                        build_completed_event(&Some(total_usage.clone()), &finish_reason)
                    {
                        yield completed;
                    }
                }
                break;
            }

            if tool_calls.is_empty() {
                break;
            }

            let (server_calls, client_calls) = split_tool_calls(&tool_calls, &tool_maps.source_map);
            if server_calls.is_empty() {
                if !emitted_client_tool {
                    let events = build_client_tool_events(
                        &Some(total_usage.clone()),
                        &finish_reason,
                        &client_calls,
                    );
                    for event in events {
                        yield event;
                    }
                } else if let Some(completed) =
                    build_completed_event(&Some(total_usage.clone()), &finish_reason)
                {
                    yield completed;
                }
                break;
            }
            if !client_calls.is_empty() {
                if !emitted_client_tool {
                    let events = build_client_tool_events(
                        &Some(total_usage.clone()),
                        &finish_reason,
                        &client_calls,
                    );
                    for event in events {
                        yield event;
                    }
                } else if let Some(completed) =
                    build_completed_event(&Some(total_usage.clone()), &finish_reason)
                {
                    yield completed;
                }
                break;
            }

            let request_id = request.model.clone();
            let tool_messages = async {
                let mut tool_messages = Vec::new();
                tool_messages.push(build_assistant_tool_call_message(&request_id, &server_calls));
                tool_messages.extend(
                    build_tool_messages::<M>(
                        &request_id,
                        &server_calls,
                        invoker.clone(),
                        metadata.clone(),
                        trace_id.clone(),
                        request.agent_context.clone(),
                    )
                        .await?,
                );
                Ok::<Vec<Message>, ProviderError>(tool_messages)
            }
            .await?;
            request.messages.extend(tool_messages);
        }
        info!(
            "http.request.end; status_code=200 model_id={} finish_reason={} prompt_tokens={} completion_tokens={} trace_id={}",
            model_id,
            last_finish_reason.as_deref().unwrap_or(""),
            total_usage.input_tokens,
            total_usage.output_tokens,
            trace_id
        );
    };

    let boxed_stream: Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send>> =
        Box::pin(stream);
    Ok(AgentLoopResult::Stream {
        events: boxed_stream,
    })
}

fn build_tool_maps(
    request: &ChatCompletionRequest,
    server_tools: &HashMap<String, String>,
) -> Result<ToolMaps, ProviderError> {
    let server_tools = server_tools
        .into_iter()
        .filter_map(|(name, schema)| parse_tool_schema(&name, &schema))
        .collect::<Vec<_>>();
    let mut source_map = HashMap::new();
    let mut merged = Vec::new();
    let mut server_lookup = HashMap::new();
    let mut server_seen = HashMap::new();

    for tool in server_tools {
        source_map.insert(tool.function.name.clone(), ToolSource::Server);
        server_lookup.insert(tool.function.name.clone(), tool);
    }

    if let Some(request_tools) = &request.tools {
        for tool in request_tools {
            if let Some(server_tool) = server_lookup.get(&tool.function.name) {
                merged.push(server_tool.clone());
                server_seen.insert(tool.function.name.clone(), true);
                source_map.insert(tool.function.name.clone(), ToolSource::Server);
            } else {
                source_map.insert(tool.function.name.clone(), ToolSource::Client);
                merged.push(tool.clone());
            }
        }
    }

    for (name, tool) in server_lookup {
        if !server_seen.contains_key(&name) {
            merged.push(tool);
        }
    }

    Ok(ToolMaps {
        merged_tools: if merged.is_empty() {
            None
        } else {
            Some(merged)
        },
        source_map,
    })
}

fn parse_tool_schema(name: &str, schema: &str) -> Option<ToolDefinition> {
    let value: Value = match serde_json::from_str(schema) {
        Ok(value) => value,
        Err(err) => {
            error!("failed to parse tool schema {name}: {err}");
            return None;
        }
    };

    let description = value
        .get("description")
        .and_then(|value| value.as_str())
        .map(str::to_string);
    let strict = value.get("strict").and_then(|value| value.as_bool());
    let parameters = value
        .get("parameters")
        .cloned()
        .unwrap_or_else(|| serde_json::json!({}));

    Some(ToolDefinition {
        r#type: "function".to_string(),
        function: crate::openai_types::FunctionDefinition {
            name: name.to_string(),
            description,
            strict,
            parameters,
        },
    })
}

fn split_tool_calls(
    calls: &[ProviderToolCall],
    source_map: &HashMap<String, ToolSource>,
) -> (Vec<ProviderToolCall>, Vec<ProviderToolCall>) {
    let mut server = Vec::new();
    let mut client = Vec::new();
    for call in calls {
        match source_map
            .get(&call.name)
            .copied()
            .unwrap_or(ToolSource::Client)
        {
            ToolSource::Server => server.push(call.to_owned()),
            ToolSource::Client => client.push(call.to_owned()),
        }
    }
    (server, client)
}

fn ensure_provider_call_ids(request_id: &str, calls: &mut [ProviderToolCall]) {
    for (index, call) in calls.iter_mut().enumerate() {
        if call.id.is_none() {
            call.id = Some(ensure_tool_call_id(request_id, index));
        }
    }
}

async fn build_tool_messages<M: Serialize>(
    request_id: &str,
    calls: &[ProviderToolCall],
    invoker: Arc<dyn ToolInvoker>,
    metadata: Arc<M>,
    trace_id: String,
    agent_context: Option<Value>,
) -> Result<Vec<Message>, ProviderError>
where
    M: Send + Sync + 'static,
{
    let agent_context = agent_context.map(|value| value.to_string());
    let request_id = request_id.to_string();
    let parent_span = Span::current();
    let parent_cx = parent_span.context();
    let tasks = calls.iter().cloned().enumerate().map(|(index, call)| {
        let invoker = invoker.clone();
        let metadata = metadata.clone();
        let trace_id = trace_id.clone();
        let agent_context = agent_context.clone();
        let request_id = request_id.clone();
        let parent_span = parent_span.clone();
        let parent_cx = parent_cx.clone();
        tokio::task::spawn(async move {
            let _enter = parent_span.enter();
            let tracer = global::tracer("llm_router");
            let tool_name = call.name.clone();
            let mut span = tracer.start_with_context(tool_name.clone(), &parent_cx);
            span.set_attribute(KeyValue::new("tool_name", tool_name));
            span.set_attribute(KeyValue::new("arguments", call.arguments.clone()));
            let tool_call_id = call
                .id
                .clone()
                .unwrap_or_else(|| ensure_tool_call_id(&request_id, index));
            span.set_attribute(KeyValue::new(
                "args_size",
                call.arguments.as_bytes().len() as i64,
            ));
            let request = ToolRequest {
                args: call.arguments.clone(),
                agent_context,
            };
            let request_headers = RequestHeaders {
                name: call.name.clone(),
                trace_id,
                span_id: format!("tool-{}", call.name),
                body_format: BodyFormat::Bytes,
                extension: serde_json::to_string(metadata.as_ref()).unwrap_or_default(),
            };
            let response = invoker.invoke(request_headers, request).await;
            let content = if let Some(error_msg) = response.error_msg {
                span.set_attribute(KeyValue::new("status", "error"));
                span.set_attribute(KeyValue::new("error", error_msg.clone()));
                span.set_status(Status::error(error_msg.clone()));
                error_msg
            } else {
                span.set_attribute(KeyValue::new("status", "ok"));
                response.result.unwrap_or_default()
            };
            span.set_attribute(KeyValue::new(
                "result_size",
                content.as_bytes().len() as i64,
            ));
            span.set_attribute(KeyValue::new("result", content.clone()));
            span.end();
            Ok(Message {
                role: Role::Tool,
                content: Content::Text(content),
                tool_call_id: Some(tool_call_id),
                tool_calls: None,
            })
        })
    });

    let results = join_all(tasks).await;
    let mut messages = Vec::with_capacity(results.len());
    for result in results {
        let message = result
            .map_err(|err| ProviderError::Internal(format!("tool task join error: {err}")))??;
        messages.push(message);
    }
    Ok(messages)
}

fn build_assistant_tool_call_message(request_id: &str, calls: &[ProviderToolCall]) -> Message {
    let tool_calls = calls
        .iter()
        .enumerate()
        .map(|(index, call)| crate::openai_types::ToolCall {
            id: call
                .id
                .clone()
                .or_else(|| Some(ensure_tool_call_id(request_id, index))),
            r#type: Some("function".to_string()),
            function: crate::openai_types::ToolCallFunction {
                name: call.name.clone(),
                arguments: call.arguments.clone(),
                description: Some(call.description.clone()),
            },
        })
        .collect();

    Message {
        role: Role::Assistant,
        content: Content::Text("Tool call".to_string()),
        tool_call_id: None,
        tool_calls: Some(tool_calls),
    }
}

fn build_client_tool_events(
    usage: &Option<Usage>,
    finish_reason: &Option<String>,
    calls: &[ProviderToolCall],
) -> Vec<UnifiedEvent> {
    let usage = usage.clone().unwrap_or(Usage {
        input_tokens: 0,
        output_tokens: 0,
        total_tokens: 0,
        cached_tokens: None,
        reasoning_tokens: None,
    });
    let finish_reason = finish_reason
        .clone()
        .or_else(|| Some("tool_calls".to_string()));

    let mut events = Vec::new();
    for call in calls {
        events.push(UnifiedEvent::ToolCallDone {
            id: call.id.clone().unwrap_or_default(),
            name: call.name.clone(),
            arguments: call.arguments.clone(),
        });
    }
    events.push(UnifiedEvent::Completed {
        finish_reason,
        usage: Some(usage),
    });
    events
}

fn build_completed_event(
    usage: &Option<Usage>,
    finish_reason: &Option<String>,
) -> Option<UnifiedEvent> {
    let usage = usage.clone().unwrap_or(Usage {
        input_tokens: 0,
        output_tokens: 0,
        total_tokens: 0,
        cached_tokens: None,
        reasoning_tokens: None,
    });
    let finish_reason = finish_reason
        .clone()
        .or_else(|| Some("tool_calls".to_string()));
    Some(UnifiedEvent::Completed {
        finish_reason,
        usage: Some(usage),
    })
}

fn add_usage(total: &mut Usage, delta: &Usage) {
    total.input_tokens += delta.input_tokens;
    total.output_tokens += delta.output_tokens;
    total.total_tokens += delta.total_tokens;
    if total.cached_tokens.is_some() || delta.cached_tokens.is_some() {
        total.cached_tokens =
            Some(total.cached_tokens.unwrap_or(0) + delta.cached_tokens.unwrap_or(0));
    }
    if total.reasoning_tokens.is_some() || delta.reasoning_tokens.is_some() {
        total.reasoning_tokens =
            Some(total.reasoning_tokens.unwrap_or(0) + delta.reasoning_tokens.unwrap_or(0));
    }
}

fn add_usage_cloned(total: &Usage, delta: &Usage) -> Usage {
    let mut usage = total.clone();
    add_usage(&mut usage, delta);
    usage
}

fn log_llm_call(
    round: usize,
    model: &str,
    streaming: bool,
    tool_count: usize,
    usage: Option<&Usage>,
    trace_id: &str,
) {
    let default_usage = Usage {
        input_tokens: 0,
        output_tokens: 0,
        total_tokens: 0,
        cached_tokens: None,
        reasoning_tokens: None,
    };
    let usage = usage.unwrap_or(&default_usage);
    info!(
        "llm.call; trace_id={} round={} model={} streaming={} tool_count={} prompt_tokens={} completion_tokens={}",
        trace_id, round, model, streaming, tool_count, usage.input_tokens, usage.output_tokens
    );
}
