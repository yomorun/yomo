use std::collections::HashMap;
use std::fmt;
use std::pin::Pin;
use std::sync::Arc;

use crate::tool_invoker::ToolInvoker;
use crate::types::{BodyFormat, RequestHeaders, ToolRequest};
use axum::http::StatusCode;
use futures_core::Stream;
use futures_util::{StreamExt, future::join_all};
use log::{debug, error, info};
use serde::Serialize;
use serde_json;
use serde_json::Value;
use tracing::{Instrument, Span, field, info_span};

use crate::llm_provider::openai_compatible::mapper::ensure_tool_call_id;
use crate::llm_provider::{
    FinishReason, Provider, ProviderError, ToolCall as ProviderToolCall, UnifiedEvent,
    UnifiedResponse, Usage,
};
use crate::openai_types::{ChatCompletionRequest, Content, Message, Role, ToolDefinition};
use crate::trace::record_flattened_json_attributes;
use crate::usage_handler::{NoopUsageHandler, UsageHandler};
use async_trait::async_trait;

#[derive(Clone)]
pub struct AgentLoopConfig<M> {
    pub max_calls: usize,
    pub usage_handler: Arc<dyn UsageHandler<M>>,
    pub request_hook: Arc<dyn RequestHook<M>>,
}

impl<M> Default for AgentLoopConfig<M>
where
    M: Send + Sync + 'static,
{
    fn default() -> Self {
        Self {
            max_calls: 14,
            usage_handler: Arc::new(NoopUsageHandler::default()),
            request_hook: Arc::new(NoopRequestHook::default()),
        }
    }
}

impl<M> AgentLoopConfig<M> {
    pub fn handle_usage(&self, raw_usage: Option<Value>, usage: Usage) -> Usage {
        let mut usage = usage;
        if usage.raw.is_none() {
            usage.raw = raw_usage;
        }
        usage
    }
}

#[async_trait]
pub trait RequestHook<M>: Send + Sync {
    async fn preprocess(
        &self,
        trace_id: &str,
        metadata: &M,
        request: &mut ChatCompletionRequest,
    ) -> anyhow::Result<()>;
}

#[derive(Clone, Default)]
pub struct NoopRequestHook;

#[async_trait]
impl<M> RequestHook<M> for NoopRequestHook
where
    M: Send + Sync + 'static,
{
    async fn preprocess(
        &self,
        _trace_id: &str,
        _metadata: &M,
        _request: &mut ChatCompletionRequest,
    ) -> anyhow::Result<()> {
        Ok(())
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
    label: Option<String>,
    config: AgentLoopConfig<M>,
) -> Result<AgentLoopResult, ProviderError>
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Clone + Serialize + Send + Sync + 'static,
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
            label,
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
            label,
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
    label: Option<String>,
    config: AgentLoopConfig<M>,
) -> Result<AgentLoopResult, ProviderError>
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Clone + Serialize + Send + Sync + 'static,
{
    let parent_span = Span::current();
    let mut call_count = 0usize;
    let mut total_usage = Usage {
        input_tokens: 0,
        output_tokens: 0,
        total_tokens: 0,
        cached_tokens: None,
        reasoning_tokens: None,
        raw: None,
    };
    loop {
        let llm_chat_span = info_span!(
            parent: &parent_span,
            "llm.chat",
            trace_id = %trace_id,
            round = (call_count + 1) as i64,
            streaming = false,
            model = %request.model,
            request_id = field::Empty,
            finish_reason = field::Empty,
            "tool_calls.server.count" = 0i64,
            "tool_calls.client.count" = 0i64,
        );

        let tool_maps = build_tool_maps(&request, server_tools.as_ref())?;
        request.tools = tool_maps.merged_tools.clone();
        if call_count > 0 {
            request.tool_choice = None;
        }

        log_round_request(call_count + 1, false, &request, &trace_id);

        let mut response = provider
            .complete(request.clone())
            .instrument(llm_chat_span.clone())
            .await?;
        response.usage = config.handle_usage(response.usage.raw.clone(), response.usage);
        llm_chat_span.record("request_id", field::display(&response.request_id));
        llm_chat_span.record(
            "finish_reason",
            field::display(finish_reason_to_str(&response.finish_reason)),
        );
        record_flattened_json_attributes(&llm_chat_span, "usage", &usage_to_value(&response.usage));
        let usage_handler = Arc::clone(&config.usage_handler);
        let model_id = response.model.clone();
        let request_id = response.request_id.clone();
        let usage_trace_id = trace_id.clone();
        let label = label.clone();
        let metadata_value = (*metadata).clone();
        let usage = usage_to_value(&response.usage);
        tokio::spawn(async move {
            usage_handler
                .on_usage(
                    "/chat/completions",
                    &model_id,
                    label.as_deref(),
                    &request_id,
                    &usage_trace_id,
                    StatusCode::OK.as_u16(),
                    metadata_value,
                    usage,
                )
                .await;
        });
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
        llm_chat_span.record("tool_calls.server.count", server_calls.len() as i64);
        llm_chat_span.record("tool_calls.client.count", client_calls.len() as i64);
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
        let tool_calls_span = info_span!(
            parent: &parent_span,
            "tool.calls",
            trace_id = %trace_id,
            round = call_count as i64,
            streaming = false,
            tool_count = server_calls.len() as i64,
        );
        let next_messages = async {
            let mut next_messages = Vec::new();
            next_messages.push(build_assistant_tool_call_message(
                &request_id,
                &server_calls,
                Some(response.output_text.clone()),
            ));
            next_messages.extend(
                invoke_server_tools::<M>(
                    &request_id,
                    &server_calls,
                    invoker.clone(),
                    metadata.clone(),
                    trace_id.clone(),
                    request.agent_context.clone(),
                )
                .instrument(tool_calls_span)
                .await?
                .0,
            );
            Ok::<Vec<Message>, ProviderError>(next_messages)
        }
        .instrument(llm_chat_span)
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
    label: Option<String>,
    config: AgentLoopConfig<M>,
) -> Result<AgentLoopResult, ProviderError>
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Clone + Serialize + Send + Sync + 'static,
{
    let parent_span = Span::current();
    let stream = async_stream::try_stream! {
        let mut call_count = 0usize;
        let mut total_usage = Usage {
            input_tokens: 0,
            output_tokens: 0,
            total_tokens: 0,
            cached_tokens: None,
            reasoning_tokens: None,
            raw: None,
        };
        let mut last_finish_reason: Option<String> = None;
        let model_id = request.model.clone();
        loop {
            let llm_chat_span = info_span!(
                parent: &parent_span,
                "llm.chat",
                trace_id = %trace_id,
                round = (call_count + 1) as i64,
                streaming = true,
                model = %request.model,
                request_id = field::Empty,
                finish_reason = field::Empty,
                "tool_calls.server.count" = 0i64,
                "tool_calls.client.count" = 0i64,
            );

            let tool_maps = build_tool_maps(&request, server_tools.as_ref())?;
            request.tools = tool_maps.merged_tools.clone();
            if call_count > 0 {
                request.tool_choice = None;
            }

            log_round_request(call_count + 1, true, &request, &trace_id);

            let mut provider_stream = provider
                .stream(request.clone())
                .instrument(llm_chat_span.clone())
                .await?;

            let usage_offset = total_usage.clone();

            let mut tool_calls: Vec<ProviderToolCall> = Vec::new();
            let mut tool_call_index: HashMap<String, usize> = HashMap::new();
            let mut usage = None;
            let mut finish_reason = None;
            let mut saw_tool_call = false;
            let mut emitted_client_tool = false;
            let mut assistant_reasoning_content = String::new();
            let mut request_id: Option<String> = None;

            while let Some(item) = provider_stream.next().await {
                let event = item?;
                match &event {
                    UnifiedEvent::MessageDelta { id, delta } => {
                        request_id = Some(id.clone());
                        assistant_reasoning_content.push_str(delta);
                    }
                    UnifiedEvent::ResponseCreated { id, .. }
                    | UnifiedEvent::ResponseInProgress { id, .. }
                    | UnifiedEvent::MessageStart { id, .. }
                    | UnifiedEvent::MessageStop { id, .. } => {
                        request_id = Some(id.clone());
                    }
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
                        usage = Some(config.handle_usage(chunk_usage.raw.clone(), chunk_usage.clone()));
                    }
                    UnifiedEvent::Completed { finish_reason: reason, usage: chunk_usage } => {
                        finish_reason = reason.clone();
                        last_finish_reason = reason.clone();
                        if let Some(chunk_usage) = chunk_usage {
                            usage = Some(config.handle_usage(chunk_usage.raw.clone(), chunk_usage.clone()));
                        }
                    }
                    _ => {}
                }

                if !saw_tool_call {
                    match event {
                        UnifiedEvent::Usage { usage: chunk_usage } => {
                            let processed_usage =
                                config.handle_usage(chunk_usage.raw.clone(), chunk_usage.clone());
                            let usage = add_usage_cloned(&usage_offset, &processed_usage);
                            yield UnifiedEvent::Usage { usage };
                        }
                        UnifiedEvent::Completed { finish_reason, usage: chunk_usage } => {
                            let usage = chunk_usage.map(|chunk_usage| {
                                let processed_usage = config.handle_usage(
                                    chunk_usage.raw.clone(),
                                    chunk_usage,
                                );
                                add_usage_cloned(&usage_offset, &processed_usage)
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
            if let Some(id) = request_id.as_deref() {
                llm_chat_span.record("request_id", field::display(id));
            }
            llm_chat_span.record(
                "finish_reason",
                field::display(finish_reason.as_deref().unwrap_or("")),
            );
            if let Some(current_usage) = &usage {
                record_flattened_json_attributes(
                    &llm_chat_span,
                    "usage",
                    &usage_to_value(current_usage),
                );
                let usage_handler = Arc::clone(&config.usage_handler);
                let model_id = model_id.clone();
                let request_id = request_id.clone().unwrap_or_default();
                let trace_id = trace_id.clone();
                let label = label.clone();
                let metadata = (*metadata).clone();
                let usage = usage_to_value(current_usage);
                tokio::spawn(async move {
                    usage_handler
                        .on_usage(
                            "/chat/completions",
                            &model_id,
                            label.as_deref(),
                            &request_id,
                            &trace_id,
                            StatusCode::OK.as_u16(),
                            metadata,
                            usage,
                        )
                        .await;
                });
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
            llm_chat_span.record("tool_calls.server.count", server_calls.len() as i64);
            llm_chat_span.record("tool_calls.client.count", client_calls.len() as i64);
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
            let mut tool_messages = Vec::new();
            tool_messages.push(build_assistant_tool_call_message(
                &request_id,
                &server_calls,
                Some(assistant_reasoning_content.clone()),
            ));
            let tool_calls_span = info_span!(
                parent: &parent_span,
                "tool.calls",
                trace_id = %trace_id,
                round = call_count as i64,
                streaming = true,
                tool_count = server_calls.len() as i64,
            );
            let (tool_messages_from_invoker, tool_events) = invoke_server_tools::<M>(
                &request_id,
                &server_calls,
                invoker.clone(),
                metadata.clone(),
                trace_id.clone(),
                request.agent_context.clone(),
            )
            .instrument(tool_calls_span)
            .await?;
            for event in tool_events {
                yield event;
            }
            tool_messages.extend(tool_messages_from_invoker);
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

async fn invoke_server_tools<M: Serialize>(
    request_id: &str,
    calls: &[ProviderToolCall],
    invoker: Arc<dyn ToolInvoker>,
    metadata: Arc<M>,
    trace_id: String,
    agent_context: Option<Value>,
) -> Result<(Vec<Message>, Vec<UnifiedEvent>), ProviderError>
where
    M: Send + Sync + 'static,
{
    let agent_context = agent_context.map(|value| value.to_string());
    let request_id = request_id.to_string();
    let parent_span = Span::current();
    let tasks = calls.iter().cloned().enumerate().map(|(index, call)| {
        let invoker = invoker.clone();
        let metadata = metadata.clone();
        let trace_id = trace_id.clone();
        let agent_context = agent_context.clone();
        let request_id = request_id.clone();
        let parent_span = parent_span.clone();
        tokio::task::spawn(async move {
            invoke_server_tool_call(
                call,
                index,
                &request_id,
                invoker,
                metadata,
                trace_id,
                agent_context,
                parent_span,
            )
            .await
        })
    });

    let results = join_all(tasks).await;
    let mut messages = Vec::with_capacity(results.len());
    let mut events = Vec::with_capacity(results.len() * 2);
    for result in results {
        let (call_event, call_result_event) = result
            .map_err(|err| ProviderError::Internal(format!("tool task join error: {err}")))??;
        messages.push(compose_server_tool_message(&call_result_event));
        events.push(call_event);
        events.push(call_result_event);
    }
    Ok((messages, events))
}

async fn invoke_server_tool_call<M: Serialize>(
    call: ProviderToolCall,
    index: usize,
    request_id: &str,
    invoker: Arc<dyn ToolInvoker>,
    metadata: Arc<M>,
    trace_id: String,
    agent_context: Option<String>,
    parent_span: Span,
) -> Result<(UnifiedEvent, UnifiedEvent), ProviderError>
where
    M: Send + Sync + 'static,
{
    let tool_call_id = call
        .id
        .clone()
        .unwrap_or_else(|| ensure_tool_call_id(request_id, index));
    let tool_call_span = info_span!(
        parent: &parent_span,
        "tool.call",
        otel.name = %call.name,
        tool_name = %call.name,
        tool_call_id = %tool_call_id,
        args_size = call.arguments.as_bytes().len() as i64,
    );

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

    let response = invoker
        .invoke(request_headers, request)
        .instrument(tool_call_span)
        .await;
    let result = response.result;
    let error = response.error_msg;

    let call_event = UnifiedEvent::ServerToolCall {
        tool_call_id: tool_call_id.clone(),
        name: call.name.clone(),
        arguments: call.arguments,
    };
    let call_result_event = UnifiedEvent::ServerToolCallResult {
        tool_call_id,
        name: call.name,
        result,
        error,
    };

    Ok((call_event, call_result_event))
}

fn compose_server_tool_message(call_result_event: &UnifiedEvent) -> Message {
    let (tool_call_id, result, error) = match call_result_event {
        UnifiedEvent::ServerToolCallResult {
            tool_call_id,
            result,
            error,
            ..
        } => (tool_call_id.clone(), result.clone(), error.clone()),
        _ => unreachable!("compose_server_tool_message expects ServerToolCallResult"),
    };

    let content = if let Some(error_msg) = error {
        error_msg
    } else {
        result.unwrap_or_default()
    };

    Message {
        role: Role::Tool,
        content: Content::Text(content),
        reasoning_content: None,
        tool_call_id: Some(tool_call_id),
        tool_calls: None,
    }
}

fn build_assistant_tool_call_message(
    request_id: &str,
    calls: &[ProviderToolCall],
    reasoning_content: Option<String>,
) -> Message {
    let reasoning_content = reasoning_content.and_then(|value| {
        if value.trim().is_empty() {
            None
        } else {
            Some(value)
        }
    });

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
        reasoning_content,
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
        raw: None,
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
        raw: None,
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
    if delta.raw.is_some() {
        total.raw = delta.raw.clone();
    }
}

fn usage_to_value(usage: &Usage) -> Value {
    usage
        .raw
        .clone()
        .or_else(|| serde_json::to_value(usage).ok())
        .unwrap_or(Value::Null)
}

fn finish_reason_to_str(reason: &FinishReason) -> &'static str {
    match reason {
        FinishReason::Stop => "stop",
        FinishReason::Length => "length",
        FinishReason::ToolCalls => "tool_calls",
        FinishReason::ContentFilter => "content_filter",
        FinishReason::Other => "other",
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
        raw: None,
    };
    let usage = usage.unwrap_or(&default_usage);
    info!(
        "llm.call; trace_id={} round={} model={} streaming={} tool_count={} prompt_tokens={} completion_tokens={}",
        trace_id, round, model, streaming, tool_count, usage.input_tokens, usage.output_tokens
    );
}

fn log_round_request(
    round: usize,
    streaming: bool,
    request: &ChatCompletionRequest,
    trace_id: &str,
) {
    let messages_json =
        serde_json::to_string(&request.messages).unwrap_or_else(|_| "[]".to_string());
    debug!(
        "llm.request; trace_id={} round={} model={} streaming={} tool_choice={:?} messages_count={} messages={}",
        trace_id,
        round,
        request.model,
        streaming,
        request.tool_choice,
        request.messages.len(),
        messages_json
    );
}
