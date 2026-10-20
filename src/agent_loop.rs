use std::collections::HashMap;
use std::fmt;
use std::pin::Pin;
use std::sync::Arc;

use crate::tool_invoker::ToolInvoker;
use crate::types::{BodyFormat, RequestHeaders, ToolRequest};
use futures_core::Stream;
use futures_util::{StreamExt, future::join_all};
use log::{error, info};
use opentelemetry::trace::TraceContextExt;
use opentelemetry_sdk::trace::{IdGenerator, RandomIdGenerator};
use serde::Serialize;
use serde_json;
use serde_json::{Value, json};
use tracing::{Instrument, Span, field, info_span};
use tracing_opentelemetry::OpenTelemetrySpanExt;

use crate::openai_types::{Content, FunctionDefinition, Message, Role, ToolDefinition, Usage};
use crate::provider::{
    FinishReason, Provider, ProviderError, ToolCall as ProviderToolCall, UnifiedEvent,
    UnifiedResponse, adapt_bound_request, diagnostic_call_id, log_unified,
};
use crate::provider::{
    UnifiedContentDelta, UnifiedEventKind, UnifiedInputItem, UnifiedInputItemKind,
    UnifiedOutputItemKind, UnifiedRequest, UnifiedToolOutput, UnifiedUsageUpdate, apply_event,
};
use crate::trace::record_usage_attributes;
use crate::usage_handler::{EndpointUsage, NoopUsageHandler, UsageHandler, aggregate_to_openai};
use crate::utils::ensure_tool_call_id;
use async_trait::async_trait;

#[derive(Clone)]
pub struct AgentLoopConfig<M> {
    pub max_calls: usize,
    pub max_total_tokens: Option<i64>,
    pub usage_handler: Arc<dyn UsageHandler<M>>,
    pub request_hook: Arc<dyn RequestHook<M>>,
}

impl<M> Default for AgentLoopConfig<M>
where
    M: Send + Sync + 'static,
{
    fn default() -> Self {
        Self {
            max_calls: 24,
            max_total_tokens: Some(327_680),
            usage_handler: Arc::new(NoopUsageHandler::default()),
            request_hook: Arc::new(NoopRequestHook::default()),
        }
    }
}

#[async_trait]
pub trait RequestHook<M>: Send + Sync {
    async fn preprocess(
        &self,
        trace_id: &str,
        metadata: &M,
        request: &mut UnifiedRequest,
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
        _request: &mut UnifiedRequest,
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

struct StreamLoopState {
    call_count: usize,
    round_usages: Vec<Value>,
    last_finish_reason: Option<String>,
}

impl StreamLoopState {
    fn new() -> Self {
        Self {
            call_count: 0,
            round_usages: Vec::new(),
            last_finish_reason: None,
        }
    }
}

struct StreamRoundState {
    tool_calls: Vec<ProviderToolCall>,
    tool_call_index: HashMap<String, usize>,
    openai_usage_payload: Option<Value>,
    raw_usage: Option<EndpointUsage>,
    finish_reason: Option<String>,
    saw_tool_call: bool,
    emitted_client_tool: bool,
    assistant_output_content: String,
    assistant_reasoning_content: String,
    request_id: Option<String>,
    response: UnifiedResponse,
    pending_tool_events: Vec<UnifiedEvent>,
}

impl StreamRoundState {
    fn new() -> Self {
        Self {
            tool_calls: Vec::new(),
            tool_call_index: HashMap::new(),
            openai_usage_payload: None,
            raw_usage: None,
            finish_reason: None,
            saw_tool_call: false,
            emitted_client_tool: false,
            assistant_output_content: String::new(),
            assistant_reasoning_content: String::new(),
            request_id: None,
            response: UnifiedResponse::new(String::new(), String::new()),
            pending_tool_events: Vec::new(),
        }
    }
}

pub async fn run_agent_loop<A, M>(
    provider: Arc<dyn Provider<M>>,
    request: UnifiedRequest,
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
    if request.controls.stream.unwrap_or(false) {
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
    provider: Arc<dyn Provider<M>>,
    mut request: UnifiedRequest,
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
    let mut round_usages: Vec<Value> = Vec::new();
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
        set_request_tools(&mut request, tool_maps.merged_tools.as_deref())?;
        if call_count > 0 {
            request.tool_choice = None;
        }

        let call_id = diagnostic_call_id(&llm_chat_span);
        log_unified(
            "unified.request",
            &call_id,
            &trace_id,
            call_count + 1,
            provider.model_id(),
            &request,
        );

        if let Some(binding) = provider.binding() {
            adapt_bound_request(&mut request, binding)?;
        }
        let mut response = provider
            .complete(request.clone(), &metadata)
            .instrument(llm_chat_span.clone())
            .await?;
        log_unified(
            "unified.response",
            &call_id,
            &trace_id,
            call_count + 1,
            provider.model_id(),
            &response,
        );
        llm_chat_span.record("request_id", field::display(&response.id));
        llm_chat_span.record(
            "finish_reason",
            field::display(finish_reason_to_str(&response_finish_reason(&response))),
        );
        let raw_usage = response
            .usage
            .clone()
            .unwrap_or_else(crate::openai_http_mapping::empty_usage);
        record_usage_attributes(&llm_chat_span, "usage", &raw_usage);
        let usage_handler = Arc::clone(&config.usage_handler);
        let model_id = response.model.clone();
        let request_id = response.id.clone();
        let usage_trace_id = trace_id.clone();
        let label = label.clone();
        let metadata_value = (*metadata).clone();
        let modified_usage = usage_handler
            .on_usage(
                &model_id,
                label.as_deref(),
                &request_id,
                &usage_trace_id,
                metadata_value,
                raw_usage,
            )
            .instrument(llm_chat_span.clone())
            .await
            .into_payload("/chat/completions");
        response.usage = Some(
            EndpointUsage::from_endpoint_payload("/chat/completions", modified_usage.clone())
                .expect("agent_loop expected chat/completions usage payload"),
        );
        round_usages.push(modified_usage);
        call_count += 1;

        let mut tool_calls = response_tool_calls(&response);
        ensure_provider_call_ids(&response.id, &mut tool_calls);
        log_llm_call(
            call_count,
            &request.model,
            false,
            tool_calls.len(),
            response.usage.as_ref().map(usage_to_value).as_ref(),
            &trace_id,
        );
        let (server_calls, client_calls) = split_tool_calls(&tool_calls, &tool_maps.source_map);
        llm_chat_span.record("tool_calls.server.count", server_calls.len() as i64);
        llm_chat_span.record("tool_calls.client.count", client_calls.len() as i64);

        let reached_max_calls = call_count >= config.max_calls;
        let reached_max_total_tokens =
            is_total_token_limit_reached(config.max_total_tokens, &round_usages);
        if reached_max_calls || reached_max_total_tokens {
            retain_response_tool_calls(&mut response, &client_calls);
            response.usage = Some(
                EndpointUsage::from_endpoint_payload(
                    "/chat/completions",
                    aggregate_usages_to_value("/chat/completions", &round_usages),
                )
                .expect("agent_loop expected chat/completions usage payload"),
            );
            return Ok(AgentLoopResult::NonStream(response));
        }

        if tool_calls.is_empty() {
            response.usage = Some(
                EndpointUsage::from_endpoint_payload(
                    "/chat/completions",
                    aggregate_usages_to_value("/chat/completions", &round_usages),
                )
                .expect("agent_loop expected chat/completions usage payload"),
            );
            return Ok(AgentLoopResult::NonStream(response));
        }

        if server_calls.is_empty() {
            retain_response_tool_calls(&mut response, &client_calls);
            response.usage = Some(
                EndpointUsage::from_endpoint_payload(
                    "/chat/completions",
                    aggregate_usages_to_value("/chat/completions", &round_usages),
                )
                .expect("agent_loop expected chat/completions usage payload"),
            );
            return Ok(AgentLoopResult::NonStream(response));
        }
        if !client_calls.is_empty() {
            retain_response_tool_calls(&mut response, &client_calls);
            response.usage = Some(
                EndpointUsage::from_endpoint_payload(
                    "/chat/completions",
                    aggregate_usages_to_value("/chat/completions", &round_usages),
                )
                .expect("agent_loop expected chat/completions usage payload"),
            );
            return Ok(AgentLoopResult::NonStream(response));
        }

        let request_id = response.id.clone();
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
            next_messages.extend(response_input_items(&response));
            next_messages.extend(
                invoke_server_tools::<M>(
                    &request_id,
                    &server_calls,
                    invoker.clone(),
                    metadata.clone(),
                    trace_id.clone(),
                    request.execution_context.clone(),
                )
                .instrument(tool_calls_span)
                .await?
                .0
                .into_iter()
                .map(tool_message_to_input),
            );
            Ok::<Vec<UnifiedInputItem>, ProviderError>(next_messages)
        }
        .instrument(llm_chat_span)
        .await?;
        request.input.extend(next_messages);
    }
}

async fn run_agent_loop_stream<A, M>(
    provider: Arc<dyn Provider<M>>,
    mut request: UnifiedRequest,
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
        let mut loop_state = StreamLoopState::new();
        let model_id = request.model.clone();
        loop {
            let llm_chat_span = info_span!(
                parent: &parent_span,
                "llm.chat",
                trace_id = %trace_id,
                round = (loop_state.call_count + 1) as i64,
                streaming = true,
                model = %request.model,
                request_id = field::Empty,
                finish_reason = field::Empty,
                "tool_calls.server.count" = 0i64,
                "tool_calls.client.count" = 0i64,
            );

            let tool_maps = build_tool_maps(&request, server_tools.as_ref())?;
            set_request_tools(&mut request, tool_maps.merged_tools.as_deref())?;
            if loop_state.call_count > 0 {
                request.tool_choice = None;
            }

            let call_id = diagnostic_call_id(&llm_chat_span);
            log_unified("unified.request", &call_id, &trace_id, loop_state.call_count + 1, provider.model_id(), &request);

            if let Some(binding) = provider.binding() {
                adapt_bound_request(&mut request, binding)?;
            }
            let mut provider_stream = provider
                .stream(request.clone(), &metadata)
                .instrument(llm_chat_span.clone())
                .await?;

            let mut round_state = StreamRoundState::new();

            while let Some(item) = provider_stream.next().await {
                let event = item?;
                log_unified("unified.event", &call_id, &trace_id, loop_state.call_count + 1, provider.model_id(), &event);
                apply_event(&mut round_state.response, &event).map_err(ProviderError::internal)?;
                if !round_state.response.id.is_empty() { round_state.request_id = Some(round_state.response.id.clone()); }
                match &event.kind {
                    UnifiedEventKind::ContentPartDelta { delta: UnifiedContentDelta::Text(delta) } => {
                        round_state.assistant_output_content.push_str(delta);
                    }
                    UnifiedEventKind::ContentPartDelta { delta: UnifiedContentDelta::Thinking(delta) }
                    | UnifiedEventKind::ReasoningSummaryTextDelta { delta } => {
                        round_state.assistant_reasoning_content.push_str(delta);
                    }
                    UnifiedEventKind::ResponseCreated { response }
                    | UnifiedEventKind::ResponseInProgress { response: Some(response) } => {
                        round_state.request_id = Some(response.id.clone());
                    }
                    UnifiedEventKind::OutputItemAdded { item } | UnifiedEventKind::OutputItemDone { item }
                        if matches!(item.kind, UnifiedOutputItemKind::FunctionCall { .. }) => {
                        round_state.saw_tool_call = true;
                        round_state.pending_tool_events.push(event.clone());
                        let UnifiedOutputItemKind::FunctionCall { call_id, name, arguments, .. } = &item.kind else { unreachable!() };
                        let id = &item.identity.internal_id;
                        let index = *round_state.tool_call_index.entry(id.clone()).or_insert_with(|| {
                            let current = round_state.tool_calls.len();
                            round_state.tool_calls.push(ProviderToolCall {
                                id: Some(call_id.clone()),
                                name: name.clone(),
                                description: String::new(),
                                arguments: String::new(),
                            });
                            current
                        });
                        let resolved_name = round_state
                            .tool_calls
                            .get(index)
                            .map(|call| resolve_tool_call_name(&call.name, name))
                            .unwrap_or_else(|| name.clone());
                        if let Some(call) = round_state.tool_calls.get_mut(index) {
                            call.name = resolved_name.clone();
                            call.id = Some(call_id.clone());
                            call.arguments = arguments.clone();
                        }
                        if resolved_name.trim().is_empty() {
                            continue;
                        }
                        let is_server = matches!(tool_maps.source_map.get(&resolved_name), Some(ToolSource::Server));
                        if is_server {
                        } else {
                            round_state.emitted_client_tool = true;
                            let mut retained = Vec::new();
                            for pending in round_state.pending_tool_events.drain(..) {
                                if pending.target.as_ref().and_then(|target| target.item_id.as_ref()) == Some(id) { yield pending; }
                                else { retained.push(pending); }
                            }
                            round_state.pending_tool_events = retained;
                        }
                    }
                    UnifiedEventKind::ItemDelta { .. } | UnifiedEventKind::FunctionArgumentsDone { .. } => {
                        round_state.saw_tool_call = true;
                        round_state.pending_tool_events.push(event.clone());
                        let Some(target) = &event.target else { continue; };
                        let Some(id) = &target.item_id else { continue; };
                        let Some(item) = round_state.response.candidates.iter().flat_map(|candidate| &candidate.output).find(|item| &item.identity.internal_id == id) else { continue; };
                        let UnifiedOutputItemKind::FunctionCall { call_id, name, arguments, .. } = &item.kind else { continue; };
                        let index = *round_state.tool_call_index.entry(id.clone()).or_insert_with(|| {
                            let current = round_state.tool_calls.len();
                            round_state.tool_calls.push(ProviderToolCall {
                                id: Some(call_id.clone()),
                                name: name.clone(),
                                description: String::new(),
                                arguments: String::new(),
                            });
                            current
                        });
                        let resolved_name = round_state
                            .tool_calls
                            .get(index)
                            .map(|call| resolve_tool_call_name(&call.name, name))
                            .unwrap_or_else(|| name.clone());
                        let is_server = matches!(
                            tool_maps
                                .source_map
                                .get(&resolved_name)
                                .copied()
                                .unwrap_or(ToolSource::Client),
                            ToolSource::Server
                        );
                        if let Some(call) = round_state.tool_calls.get_mut(index) {
                            call.name = resolved_name.clone();
                            call.id = Some(call_id.clone());
                            call.arguments = arguments.clone();
                        }
                        if resolved_name.trim().is_empty() {
                            continue;
                        }
                        if is_server {
                        } else {
                            round_state.emitted_client_tool = true;
                            let mut retained = Vec::new();
                            for pending in round_state.pending_tool_events.drain(..) {
                                if pending.target.as_ref().and_then(|target| target.item_id.as_ref()) == Some(id) { yield pending; }
                                else { retained.push(pending); }
                            }
                            round_state.pending_tool_events = retained;
                        }
                    }
                    UnifiedEventKind::CandidateDone { ending } => {
                        let reason = Some(ending.to_chat().to_string());
                        round_state.finish_reason = reason.clone();
                        loop_state.last_finish_reason = reason;
                    }
                    UnifiedEventKind::Completed { .. } => {
                        let reason = response_finish_reason(&round_state.response);
                        round_state.finish_reason = Some(finish_reason_to_str(&reason).into());
                        loop_state.last_finish_reason = round_state.finish_reason.clone();
                    }
                    UnifiedEventKind::Incomplete { .. } => {
                        round_state.finish_reason = Some("length".into());
                        loop_state.last_finish_reason = round_state.finish_reason.clone();
                    }
                    _ => {}
                }

                let usage_event = matches!(&event.kind, UnifiedEventKind::Usage { .. });
                if let Some(chunk_usage) = event_usage(&round_state.response, &event) {
                    let changed = round_state.raw_usage.as_ref().is_none_or(|old| old.raw_payload() != chunk_usage.raw_payload());
                    round_state.raw_usage = Some(chunk_usage.clone());
                    let processed_usage = chunk_usage.into_payload("/chat/completions");
                    round_state.openai_usage_payload = Some(processed_usage.clone());
                    let mut usage_with_history = loop_state.round_usages.clone();
                    usage_with_history.push(processed_usage);
                    let usage_with_offset =
                        aggregate_usages_to_value("/chat/completions", &usage_with_history);
                    if changed { yield UnifiedEvent::new(UnifiedEventKind::Usage {
                        usage: EndpointUsage::from_endpoint_payload(
                            "/chat/completions",
                            usage_with_offset,
                        )
                        .expect("agent_loop expected chat/completions usage payload"),
                        update: UnifiedUsageUpdate::Snapshot,
                        source_usage: None,
                    }); }
                }
                if usage_event { continue; }

                if !round_state.saw_tool_call {
                    yield event;
                }
            }

            log_unified("unified.response", &call_id, &trace_id, loop_state.call_count + 1, provider.model_id(), &round_state.response);
            loop_state.call_count += 1;
            if let Some(id) = round_state.request_id.as_deref() {
                llm_chat_span.record("request_id", id);
            }
            llm_chat_span.record(
                "finish_reason",
                round_state.finish_reason.as_deref().unwrap_or(""),
            );
            if let Some(raw_usage) = &round_state.raw_usage {
                record_usage_attributes(&llm_chat_span, "usage", raw_usage);
                let modified_usage = config
                    .usage_handler
                    .on_usage(
                        &model_id,
                        label.as_deref(),
                        round_state.request_id.as_deref().unwrap_or(""),
                        &trace_id,
                        (*metadata).clone(),
                        raw_usage.clone(),
                    )
                    .instrument(llm_chat_span.clone())
                    .await
                    .into_payload("/chat/completions");
                loop_state.round_usages.push(modified_usage);
            }
            log_llm_call(
                loop_state.call_count,
                &request.model,
                true,
                round_state.tool_calls.len(),
                round_state.openai_usage_payload.as_ref(),
                &trace_id,
            );
            let (server_calls, client_calls) =
                split_tool_calls(&round_state.tool_calls, &tool_maps.source_map);
            llm_chat_span.record("tool_calls.server.count", server_calls.len() as i64);
            llm_chat_span.record("tool_calls.client.count", client_calls.len() as i64);
            let reached_max_calls = loop_state.call_count >= config.max_calls;
            let reached_max_total_tokens =
                is_total_token_limit_reached(config.max_total_tokens, &loop_state.round_usages);
            if reached_max_calls || reached_max_total_tokens {
                if !client_calls.is_empty() && !round_state.emitted_client_tool {
                    let events = build_client_tool_events(&round_state.finish_reason, &client_calls);
                    for event in events {
                        yield event;
                    }
                } else if let Some(completed) = build_completed_event(&round_state.finish_reason) {
                    yield completed;
                }
                break;
            }

            if server_calls.is_empty() && client_calls.is_empty() {
                break;
            }
            if server_calls.is_empty() {
                if !round_state.emitted_client_tool {
                    let events = build_client_tool_events(&round_state.finish_reason, &client_calls);
                    for event in events {
                        yield event;
                    }
                } else if let Some(completed) =
                    build_completed_event(&round_state.finish_reason)
                {
                    yield completed;
                }
                break;
            }
            if !client_calls.is_empty() {
                if !round_state.emitted_client_tool {
                    let events = build_client_tool_events(&round_state.finish_reason, &client_calls);
                    for event in events {
                        yield event;
                    }
                } else if let Some(completed) =
                    build_completed_event(&round_state.finish_reason)
                {
                    yield completed;
                }
                break;
            }

            let request_id = resolve_round_request_id(round_state.request_id.as_deref(), &trace_id);
            let mut tool_messages = Vec::new();
            tool_messages.extend(response_input_items(&round_state.response));
            let tool_calls_span = info_span!(
                parent: &parent_span,
                "tool.calls",
                trace_id = %trace_id,
                round = loop_state.call_count as i64,
                streaming = true,
                tool_count = server_calls.len() as i64,
            );
            let (tool_messages_from_invoker, tool_events) = invoke_server_tools::<M>(
                &request_id,
                &server_calls,
                invoker.clone(),
                metadata.clone(),
                trace_id.clone(),
                request.execution_context.clone(),
            )
            .instrument(tool_calls_span)
            .await?;
            for event in tool_events {
                yield event;
            }
            tool_messages.extend(tool_messages_from_invoker.into_iter().map(tool_message_to_input));
            request.input.extend(tool_messages);
        }
        let total_usage = aggregate_to_openai("/chat/completions", &loop_state.round_usages);
        info!(
            "http.request.end; status_code=200 model_id={} finish_reason={} prompt_tokens={} completion_tokens={} trace_id={}",
            model_id,
            loop_state.last_finish_reason.as_deref().unwrap_or(""),
            total_usage.prompt_tokens,
            total_usage.completion_tokens,
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
    request: &UnifiedRequest,
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

    let request_tools = request_function_tools(request);
    if let Some(request_tools) = &request_tools {
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

fn request_function_tools(request: &UnifiedRequest) -> Option<Vec<ToolDefinition>> {
    request.tools.as_ref().map(|tools| {
        tools
            .iter()
            .filter_map(|tool| {
                let crate::provider::UnifiedToolKind::Function {
                    name,
                    description,
                    parameters,
                    strict,
                    ..
                } = &tool.kind
                else {
                    return None;
                };
                Some(ToolDefinition {
                    r#type: "function".into(),
                    function: FunctionDefinition {
                        name: name.clone(),
                        description: description.clone(),
                        parameters: parameters.clone(),
                        strict: *strict,
                    },
                    extra: Default::default(),
                })
            })
            .collect()
    })
}

fn set_request_tools(
    request: &mut UnifiedRequest,
    tools: Option<&[ToolDefinition]>,
) -> Result<(), ProviderError> {
    let mut merged = tools
        .unwrap_or(&[])
        .iter()
        .map(|tool| crate::provider::UnifiedTool {
            cache_control: None,
            source_extensions: None,
            kind: crate::provider::UnifiedToolKind::Function {
                name: tool.function.name.clone(),
                description: tool.function.description.clone(),
                parameters: tool.function.parameters.clone(),
                strict: tool.function.strict,
                allowed_callers: None,
            },
        })
        .collect::<Vec<_>>();
    if let Some(existing) = &request.tools {
        for tool in existing {
            match &tool.kind {
                crate::provider::UnifiedToolKind::Function { name, .. } => {
                    if let Some(next) = merged.iter_mut().find(|next| matches!(&next.kind, crate::provider::UnifiedToolKind::Function { name: next_name, .. } if name == next_name)) {
                        next.cache_control = tool.cache_control.clone(); next.source_extensions = tool.source_extensions.clone();
                        if let (crate::provider::UnifiedToolKind::Function { allowed_callers, .. }, crate::provider::UnifiedToolKind::Function { allowed_callers: original, .. }) = (&mut next.kind, &tool.kind) { *allowed_callers = original.clone(); }
                    }
                }
                _ => merged.push(tool.clone()),
            }
        }
    }
    request.tools = (!merged.is_empty()).then_some(merged);
    Ok(())
}

fn response_tool_calls(response: &UnifiedResponse) -> Vec<ProviderToolCall> {
    response
        .candidates
        .first()
        .into_iter()
        .flat_map(|candidate| &candidate.output)
        .filter_map(|item| {
            let UnifiedOutputItemKind::FunctionCall {
                call_id,
                name,
                arguments,
                ..
            } = &item.kind
            else {
                return None;
            };
            Some(ProviderToolCall {
                id: (!call_id.is_empty()).then_some(call_id.clone()),
                name: name.clone(),
                arguments: arguments.clone(),
                description: String::new(),
            })
        })
        .collect()
}

fn response_finish_reason(response: &UnifiedResponse) -> FinishReason {
    let reason = response
        .candidates
        .first()
        .and_then(|candidate| candidate.ending.as_ref())
        .map(|ending| ending.to_chat());
    match reason {
        Some("stop") => FinishReason::Stop,
        Some("length") => FinishReason::Length,
        Some("tool_calls") => FinishReason::ToolCalls,
        Some("content_filter") => FinishReason::ContentFilter,
        _ => FinishReason::Other,
    }
}

fn event_usage(response: &UnifiedResponse, event: &UnifiedEvent) -> Option<EndpointUsage> {
    match &event.kind {
        UnifiedEventKind::Usage { .. }
        | UnifiedEventKind::Completed { response: Some(_) }
        | UnifiedEventKind::Incomplete {
            response: Some(_), ..
        }
        | UnifiedEventKind::Failed {
            response: Some(_), ..
        }
        | UnifiedEventKind::Cancelled {
            response: Some(_), ..
        } => response.usage.clone(),
        _ => None,
    }
}

fn retain_response_tool_calls(response: &mut UnifiedResponse, calls: &[ProviderToolCall]) {
    for candidate in &mut response.candidates {
        candidate.output.retain(|item| match &item.kind {
            UnifiedOutputItemKind::FunctionCall { call_id, .. } => {
                calls.iter().any(|call| call.id.as_ref() == Some(call_id))
            }
            _ => true,
        });
    }
}

fn response_input_items(response: &UnifiedResponse) -> Vec<UnifiedInputItem> {
    response
        .candidates
        .first()
        .into_iter()
        .flat_map(|candidate| &candidate.output)
        .map(crate::provider::UnifiedOutputItem::as_input)
        .collect()
}

fn tool_message_to_input(message: Message) -> UnifiedInputItem {
    let output = match message.content {
        Content::Text(text) => UnifiedToolOutput::Text(text),
        Content::Parts(parts) => UnifiedToolOutput::Json(json!(parts)),
    };
    UnifiedInputItem::new(UnifiedInputItemKind::FunctionCallOutput {
        call_id: message.tool_call_id.unwrap_or_default(),
        output,
        is_error: None,
        cache_control: None,
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
        function: FunctionDefinition {
            name: name.to_string(),
            description,
            strict,
            parameters,
        },
        extra: Default::default(),
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

fn resolve_tool_call_name(existing: &str, incoming: &str) -> String {
    if incoming.trim().is_empty() {
        existing.to_string()
    } else {
        incoming.to_string()
    }
}

fn resolve_round_request_id(request_id: Option<&str>, trace_id: &str) -> String {
    request_id
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_string)
        .unwrap_or_else(|| trace_id.to_string())
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
            .map_err(|err| ProviderError::internal(format!("tool task join error: {err}")))??;
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
        arguments = field::Empty,
        status = field::Empty,
        result_size = field::Empty,
        result = field::Empty,
    );
    tool_call_span.record("arguments", field::display(&call.arguments));

    let span_id = {
        let span_context = tool_call_span.context().span().span_context().clone();
        if span_context.is_valid() {
            span_context.span_id().to_string()
        } else {
            RandomIdGenerator::default().new_span_id().to_string()
        }
    };

    let request = ToolRequest {
        args: call.arguments.clone(),
        agent_context,
    };
    let request_headers = RequestHeaders {
        name: call.name.clone(),
        trace_id,
        span_id,
        body_format: BodyFormat::Bytes,
        extension: serde_json::to_string(metadata.as_ref()).unwrap_or_default(),
    };

    let response = invoker
        .invoke(request_headers, request)
        .instrument(tool_call_span.clone())
        .await;
    let mut result = None;
    let error = response.error_msg;
    if let Some(error_msg) = error.as_ref() {
        tool_call_span.record("status", field::display("error"));
        tool_call_span.record("result_size", error_msg.as_bytes().len() as i64);
        tool_call_span.record("result", error_msg);
    } else {
        let result_text = match &response.result {
            serde_json::Value::Null => String::new(),
            serde_json::Value::String(s) => s.clone(),
            other => other.to_string(),
        };
        tool_call_span.record("status", field::display("ok"));
        tool_call_span.record("result_size", result_text.as_bytes().len() as i64);
        tool_call_span.record("result", &result_text);
        result = Some(result_text);
    }

    let call_event = UnifiedEvent::new(UnifiedEventKind::RuntimeToolStarted {
        call_id: tool_call_id.clone(),
        name: call.name.clone(),
        arguments: call.arguments,
    });
    let is_error = error.is_some();
    let call_result_event = UnifiedEvent::new(UnifiedEventKind::RuntimeToolFinished {
        call_id: tool_call_id,
        name: call.name,
        output: UnifiedToolOutput::Text(error.or(result).unwrap_or_default()),
        is_error,
    });

    Ok((call_event, call_result_event))
}

fn compose_server_tool_message(call_result_event: &UnifiedEvent) -> Message {
    let (tool_call_id, content) = match &call_result_event.kind {
        UnifiedEventKind::RuntimeToolFinished {
            call_id,
            output: UnifiedToolOutput::Text(content),
            ..
        } => (call_id.clone(), content.clone()),
        _ => unreachable!("compose_server_tool_message expects ServerToolCallResult"),
    };

    Message {
        role: Role::Tool,
        content: Content::Text(content),
        reasoning_content: None,
        reasoning_blocks: None,
        cache_control: None,
        tool_call_id: Some(tool_call_id),
        tool_calls: None,
    }
}

#[cfg(test)]
fn compose_assistant_tool_call_content(
    content: Option<String>,
    reasoning_content: Option<String>,
) -> String {
    let mut sections = Vec::new();
    if let Some(reasoning) = reasoning_content {
        sections.push(format!("<think>{reasoning}</think>"));
    }
    if let Some(content) = content {
        sections.push(content);
    }
    if sections.is_empty() {
        "Tool call".to_string()
    } else {
        sections.join("\n")
    }
}

fn build_client_tool_events(
    finish_reason: &Option<String>,
    calls: &[ProviderToolCall],
) -> Vec<UnifiedEvent> {
    let finish_reason = finish_reason
        .clone()
        .or_else(|| Some("tool_calls".to_string()));

    let mut events = Vec::new();
    for call in calls {
        let id = call.id.clone().unwrap_or_default();
        let item = crate::provider::UnifiedOutputItem::new(
            id.clone(),
            UnifiedOutputItemKind::FunctionCall {
                call_id: id.clone(),
                name: call.name.clone(),
                arguments: call.arguments.clone(),
                caller: None,
            },
        );
        events.push(
            UnifiedEvent::new(UnifiedEventKind::OutputItemDone { item }).located(
                0,
                id,
                events.len() as u32,
                None,
            ),
        );
    }
    if let Some(event) = build_completed_event(&finish_reason) {
        events.push(event);
    }
    events
}

fn build_completed_event(finish_reason: &Option<String>) -> Option<UnifiedEvent> {
    let finish_reason = finish_reason
        .clone()
        .or_else(|| Some("tool_calls".to_string()));
    let mut response = UnifiedResponse::new(String::new(), String::new());
    response.candidates.push(crate::provider::UnifiedCandidate {
        index: 0,
        output: vec![],
        ending: finish_reason
            .as_deref()
            .map(crate::provider::UnifiedEnding::from_source),
        logprobs: None,
        source_extensions: None,
    });
    Some(UnifiedEvent::new(UnifiedEventKind::Completed {
        response: Some(response),
    }))
}

fn usage_to_value(usage: &EndpointUsage) -> Value {
    usage.clone().into_payload("/chat/completions")
}

fn aggregate_usages_to_value(endpoint: &str, usages: &[Value]) -> Value {
    serde_json::to_value(aggregate_to_openai(endpoint, usages)).unwrap_or(Value::Null)
}

fn is_total_token_limit_reached(max_total_tokens: Option<i64>, usages: &[Value]) -> bool {
    let Some(max_total_tokens) = max_total_tokens else {
        return false;
    };
    let usage = aggregate_to_openai("/chat/completions", usages);
    usage.total_tokens >= max_total_tokens
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

fn log_llm_call(
    round: usize,
    model: &str,
    streaming: bool,
    tool_count: usize,
    usage: Option<&Value>,
    trace_id: &str,
) {
    let usage = usage
        .map(map_usage_to_log)
        .map(|value| (value.prompt_tokens, value.completion_tokens))
        .unwrap_or((0, 0));
    info!(
        "llm.call; trace_id={} round={} model={} streaming={} tool_count={} prompt_tokens={} completion_tokens={}",
        trace_id, round, model, streaming, tool_count, usage.0, usage.1
    );
}

fn map_usage_to_log(usage: &Value) -> Usage {
    aggregate_to_openai("/chat/completions", std::slice::from_ref(usage))
}

#[cfg(test)]
mod tests {
    use serde_json::json;
    use std::collections::HashMap;
    use std::pin::Pin;
    use std::sync::{Arc, Mutex};

    use async_trait::async_trait;
    use futures_core::Stream;
    use futures_util::{StreamExt, stream};

    use super::response_tool_calls;
    use super::{
        AgentLoopConfig, AgentLoopResult, compose_assistant_tool_call_content,
        resolve_round_request_id, resolve_tool_call_name, run_agent_loop,
    };
    use crate::openai_types::{
        ChatCompletionRequest, Content, Message, Role, Usage as OpenAIUsage,
    };
    use crate::provider::{Provider, ProviderError, UnifiedEvent, UnifiedResponse};
    use crate::provider::{
        UnifiedContentDelta, UnifiedEventKind, UnifiedItemDelta, UnifiedOutputItem,
        UnifiedOutputItemKind, UnifiedRequest, UnifiedUsageUpdate,
    };
    use crate::tool_invoker::ToolInvoker;
    use crate::types::{RequestHeaders, ToolRequest, ToolResponse};
    use crate::usage_handler::{EndpointUsage, UsageHandler};
    use crate::usage_types::GenerateContentUsage;

    #[test]
    fn terminal_snapshots_supply_usage_without_suppressing_empty_completion() {
        let mut response = UnifiedResponse::new("r".into(), "m".into());
        response.usage = Some(usage(10, 5, 15));
        let event = UnifiedEvent::new(UnifiedEventKind::Completed {
            response: Some(response.clone()),
        });
        assert_eq!(
            super::event_usage(&response, &event)
                .expect("usage")
                .raw_payload(),
            response.usage.as_ref().expect("usage").raw_payload()
        );
        response.usage = None;
        assert!(super::event_usage(&response, &event).is_none());
    }

    #[test]
    fn responses_terminal_usage_replaces_snapshot_instead_of_adding_it() {
        let mut state = crate::dialects::openai_responses::mapper::StreamMapState::default();
        let events = crate::dialects::openai_responses::mapper::stream_event_from_value(&json!({"type":"response.completed","response":{"id":"r","model":"m","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}), &mut state).expect("events");
        let mut response = UnifiedResponse::new("r".into(), "m".into());
        super::apply_event(&mut response, &usage_event(usage(10, 2, 12))).expect("snapshot");
        let mut final_usage = None;
        for event in events {
            super::apply_event(&mut response, &event).expect("terminal");
            if let Some(usage) = super::event_usage(&response, &event) {
                final_usage = Some(usage);
            }
        }
        let usage = final_usage
            .expect("terminal usage")
            .to_openai_usage()
            .expect("billing usage");
        assert_eq!(usage.prompt_tokens, 10);
        assert_eq!(usage.completion_tokens, 5);
        assert_eq!(usage.total_tokens, 15);
    }

    #[test]
    fn tool_round_preserves_callers_and_selects_one_candidate() {
        let mut request = crate::api::request_from_value(&json!({"model":"m","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"lookup","input_schema":{},"allowed_callers":["direct"]}]}), crate::provider::UnifiedSourceProtocol::Messages).expect("request");
        let maps = super::build_tool_maps(&request, &HashMap::new()).expect("maps");
        super::set_request_tools(&mut request, maps.merged_tools.as_deref()).expect("tools");
        let native = crate::api::request_to_value(
            &request,
            crate::provider::UnifiedSourceProtocol::Messages,
        )
        .expect("native");
        assert_eq!(native["tools"][0]["allowed_callers"], json!(["direct"]));
        let mut response = UnifiedResponse::new("r".into(), "m".into());
        for index in 0..2 {
            super::apply_event(
                &mut response,
                &call_done(&format!("call-{index}"), "lookup", "{}").located(
                    index,
                    format!("call-{index}"),
                    0,
                    None,
                ),
            )
            .expect("call");
        }
        assert_eq!(response_tool_calls(&response).len(), 1);
        assert_eq!(
            response_tool_calls(&response)[0].id.as_deref(),
            Some("call-0")
        );
        let event = super::build_completed_event(&Some("length".into())).expect("completed");
        assert!(
            matches!(event.kind, UnifiedEventKind::Completed { response: Some(response) } if super::response_finish_reason(&response) == crate::provider::FinishReason::Length)
        );
    }

    fn start(id: &str) -> UnifiedEvent {
        UnifiedEvent::new(UnifiedEventKind::ResponseCreated {
            response: UnifiedResponse::new(id.into(), "mock-model".into()),
        })
    }
    fn usage_event(usage: EndpointUsage) -> UnifiedEvent {
        UnifiedEvent::new(UnifiedEventKind::Usage {
            usage,
            update: UnifiedUsageUpdate::Snapshot,
            source_usage: None,
        })
    }
    fn completed(reason: Option<&str>) -> UnifiedEvent {
        let mut response = UnifiedResponse::new(String::new(), String::new());
        response.candidates.push(crate::provider::UnifiedCandidate {
            index: 0,
            output: vec![],
            ending: reason.map(crate::provider::UnifiedEnding::from_source),
            logprobs: None,
            source_extensions: None,
        });
        UnifiedEvent::new(UnifiedEventKind::Completed {
            response: Some(response),
        })
    }
    fn call_done(id: &str, name: &str, arguments: &str) -> UnifiedEvent {
        let item = UnifiedOutputItem::new(
            id.into(),
            UnifiedOutputItemKind::FunctionCall {
                call_id: id.into(),
                name: name.into(),
                arguments: arguments.into(),
                caller: None,
            },
        );
        UnifiedEvent::new(UnifiedEventKind::OutputItemDone { item }).located(0, id.into(), 0, None)
    }
    fn call_delta(id: &str, arguments: &str) -> UnifiedEvent {
        UnifiedEvent::new(UnifiedEventKind::ItemDelta {
            delta: UnifiedItemDelta::FunctionArguments(arguments.into()),
        })
        .located(0, id.into(), 0, None)
    }
    fn text_event(id: &str, text: &str) -> UnifiedEvent {
        let mut event = UnifiedEvent::new(UnifiedEventKind::ContentPartDelta {
            delta: UnifiedContentDelta::Text(text.into()),
        })
        .located(
            0,
            format!("{id}-message"),
            0,
            Some(crate::provider::UnifiedPartLocation::Content(0)),
        );
        event.response_id = Some(id.into());
        event
    }

    #[derive(Clone)]
    struct StaticStreamProvider {
        events: Arc<Vec<UnifiedEvent>>,
    }

    #[derive(Clone)]
    struct StaticCompleteProvider {
        response: UnifiedResponse,
    }

    #[derive(Clone)]
    struct SequencedStreamProvider {
        rounds: Arc<Vec<Vec<UnifiedEvent>>>,
        next_round: Arc<Mutex<usize>>,
    }

    #[derive(Clone)]
    struct SequencedCompleteProvider {
        rounds: Arc<Vec<UnifiedResponse>>,
        next_round: Arc<Mutex<usize>>,
    }

    #[async_trait]
    impl Provider<()> for StaticStreamProvider {
        fn model_id(&self) -> &str {
            "mock-model"
        }

        async fn complete(
            &self,
            _request: UnifiedRequest,
            _metadata: &(),
        ) -> Result<UnifiedResponse, ProviderError> {
            Err(ProviderError::internal("unused in stream test"))
        }

        async fn stream<'a>(
            &'a self,
            _request: UnifiedRequest,
            _metadata: &(),
        ) -> Result<
            Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
            ProviderError,
        > {
            let events = self
                .events
                .iter()
                .cloned()
                .map(Ok::<UnifiedEvent, ProviderError>)
                .collect::<Vec<_>>();
            Ok(Box::pin(stream::iter(events)))
        }
    }

    #[async_trait]
    impl Provider<()> for StaticCompleteProvider {
        fn model_id(&self) -> &str {
            "mock-model"
        }

        async fn complete(
            &self,
            _request: UnifiedRequest,
            _metadata: &(),
        ) -> Result<UnifiedResponse, ProviderError> {
            Ok(self.response.clone())
        }

        async fn stream<'a>(
            &'a self,
            _request: UnifiedRequest,
            _metadata: &(),
        ) -> Result<
            Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
            ProviderError,
        > {
            Err(ProviderError::internal("unused in non-stream test"))
        }
    }

    #[async_trait]
    impl Provider<()> for SequencedStreamProvider {
        fn model_id(&self) -> &str {
            "mock-model"
        }

        async fn complete(
            &self,
            _request: UnifiedRequest,
            _metadata: &(),
        ) -> Result<UnifiedResponse, ProviderError> {
            Err(ProviderError::internal("unused in stream test"))
        }

        async fn stream<'a>(
            &'a self,
            _request: UnifiedRequest,
            _metadata: &(),
        ) -> Result<
            Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
            ProviderError,
        > {
            let mut guard = self.next_round.lock().expect("next round lock");
            let index = *guard;
            *guard += 1;
            let events = self
                .rounds
                .get(index)
                .cloned()
                .unwrap_or_default()
                .into_iter()
                .map(Ok::<UnifiedEvent, ProviderError>)
                .collect::<Vec<_>>();
            Ok(Box::pin(stream::iter(events)))
        }
    }

    #[async_trait]
    impl Provider<()> for SequencedCompleteProvider {
        fn model_id(&self) -> &str {
            "mock-model"
        }

        async fn complete(
            &self,
            _request: UnifiedRequest,
            _metadata: &(),
        ) -> Result<UnifiedResponse, ProviderError> {
            let mut guard = self.next_round.lock().expect("next round lock");
            let index = *guard;
            *guard += 1;
            self.rounds
                .get(index)
                .cloned()
                .or_else(|| self.rounds.last().cloned())
                .ok_or_else(|| ProviderError::internal("no complete rounds configured"))
        }

        async fn stream<'a>(
            &'a self,
            _request: UnifiedRequest,
            _metadata: &(),
        ) -> Result<
            Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
            ProviderError,
        > {
            Err(ProviderError::internal("unused in non-stream test"))
        }
    }

    #[derive(Default)]
    struct NoopToolInvoker;

    #[async_trait]
    impl ToolInvoker for NoopToolInvoker {
        async fn invoke(&self, _headers: RequestHeaders, _request: ToolRequest) -> ToolResponse {
            ToolResponse::default()
        }
    }

    #[derive(Clone, Default)]
    struct RecordingToolInvoker {
        args: Arc<Mutex<Vec<String>>>,
    }

    impl RecordingToolInvoker {
        fn captured(&self) -> Vec<String> {
            self.args.lock().expect("tool calls lock").clone()
        }
    }

    #[async_trait]
    impl ToolInvoker for RecordingToolInvoker {
        async fn invoke(&self, _headers: RequestHeaders, request: ToolRequest) -> ToolResponse {
            self.args
                .lock()
                .expect("tool calls lock")
                .push(request.args);
            ToolResponse {
                result: serde_json::json!({"status": "ok"}),
                error_msg: None,
            }
        }
    }

    #[derive(Clone, Default)]
    struct RecordingUsageHandler {
        calls: Arc<Mutex<Vec<EndpointUsage>>>,
    }

    impl RecordingUsageHandler {
        fn captured(&self) -> Vec<EndpointUsage> {
            self.calls.lock().expect("usage calls lock").clone()
        }
    }

    #[async_trait]
    impl UsageHandler<()> for RecordingUsageHandler {
        async fn on_usage(
            &self,
            _model_id: &str,
            _label: Option<&str>,
            _request_id: &str,
            _trace_id: &str,
            _metadata: (),
            usage: EndpointUsage,
        ) -> EndpointUsage {
            self.calls
                .lock()
                .expect("usage calls lock")
                .push(usage.clone());
            usage
        }
    }

    fn stream_request() -> UnifiedRequest {
        let request = ChatCompletionRequest {
            model: "mock-model".to_string(),
            messages: vec![Message {
                role: Role::User,
                content: Content::Text("hello".to_string()),
                reasoning_content: None,
                reasoning_blocks: None,
                cache_control: None,
                tool_call_id: None,
                tool_calls: None,
            }],
            n: None,
            temperature: None,
            top_p: None,
            presence_penalty: None,
            frequency_penalty: None,
            logprobs: None,
            top_logprobs: None,
            modalities: None,
            audio: None,
            max_completion_tokens: None,
            stop: None,
            response_format: None,
            thinking: None,
            reasoning_effort: None,
            chat_template_kwargs: None,
            prediction: None,
            verbosity: None,
            tools: None,
            tool_choice: None,
            allowed_tools: None,
            parallel_tool_calls: None,
            service_tier: None,
            seed: None,
            stream: Some(true),
            stream_options: None,
            metadata: None,
            agent_context: None,
            extra: Default::default(),
        };
        crate::dialects::openai_chat_completions::mapper::request_from_chat(&request)
            .expect("request")
    }

    fn usage(prompt_tokens: i64, completion_tokens: i64, total_tokens: i64) -> EndpointUsage {
        EndpointUsage::ChatCompletions(OpenAIUsage {
            prompt_tokens,
            completion_tokens,
            total_tokens,
            prompt_tokens_details: None,
            completion_tokens_details: None,
        })
    }

    fn generate_content_usage(
        prompt_token_count: i64,
        candidates_token_count: i64,
        total_token_count: i64,
    ) -> EndpointUsage {
        EndpointUsage::GenerateContent(GenerateContentUsage {
            prompt_token_count: Some(prompt_token_count),
            candidates_token_count: Some(candidates_token_count),
            cached_content_token_count: None,
            tool_use_prompt_token_count: None,
            thoughts_token_count: None,
            total_token_count: Some(total_token_count),
            cache_tokens_details: None,
            prompt_tokens_details: None,
            candidates_tokens_details: None,
            tool_use_prompt_tokens_details: None,
            traffic_type: None,
        })
    }

    #[test]
    fn compose_assistant_tool_call_content_wraps_reasoning_in_think_tag() {
        let content = compose_assistant_tool_call_content(
            Some("visible".to_string()),
            Some("hidden reasoning".to_string()),
        );

        assert_eq!(content, "<think>hidden reasoning</think>\nvisible");
    }

    #[test]
    fn compose_assistant_tool_call_content_falls_back_to_tool_call_text() {
        let content = compose_assistant_tool_call_content(None, None);
        assert_eq!(content, "Tool call");
    }

    #[test]
    fn resolve_tool_call_name_keeps_existing_when_incoming_is_empty() {
        assert_eq!(
            resolve_tool_call_name("get_weather", ""),
            "get_weather".to_string()
        );
        assert_eq!(
            resolve_tool_call_name("get_weather", "   "),
            "get_weather".to_string()
        );
        assert_eq!(
            resolve_tool_call_name("get_weather", "query_weather"),
            "query_weather".to_string()
        );
    }

    #[test]
    fn resolve_round_request_id_prefers_non_empty_request_id() {
        assert_eq!(
            resolve_round_request_id(Some(" req-123 "), "trace-1"),
            "req-123".to_string()
        );
    }

    #[test]
    fn resolve_round_request_id_falls_back_to_trace_id() {
        assert_eq!(
            resolve_round_request_id(None, "trace-1"),
            "trace-1".to_string()
        );
        assert_eq!(
            resolve_round_request_id(Some("   "), "trace-2"),
            "trace-2".to_string()
        );
    }

    #[tokio::test]
    async fn run_agent_loop_stream_calls_on_usage_once_per_round_with_tool_events() {
        let usage_handler = RecordingUsageHandler::default();
        let provider = StaticStreamProvider {
            events: Arc::new(vec![
                start("req-1"),
                usage_event(usage(11, 3, 14)),
                call_done("tool-1", "client_tool", "{}"),
                usage_event(usage(22, 4, 26)),
                completed(Some("tool_calls")),
            ]),
        };
        let config = AgentLoopConfig {
            max_calls: 1,
            usage_handler: Arc::new(usage_handler.clone()),
            ..AgentLoopConfig::default()
        };

        let result = run_agent_loop::<(), ()>(
            Arc::new(provider),
            stream_request(),
            HashMap::new(),
            Arc::new(NoopToolInvoker),
            (),
            "trace-1".to_string(),
            None,
            config,
        )
        .await
        .expect("stream run should succeed");

        let AgentLoopResult::Stream { mut events } = result else {
            panic!("expected stream result");
        };
        while let Some(event) = events.next().await {
            event.expect("stream event should be ok");
        }

        let calls = usage_handler.captured();
        assert_eq!(calls.len(), 1);
        assert_eq!(
            calls[0].clone().into_payload("/chat/completions"),
            serde_json::json!({
                "prompt_tokens": 22,
                "completion_tokens": 4,
                "total_tokens": 26
            })
        );
    }

    #[tokio::test]
    async fn run_agent_loop_stream_emits_usage_after_tool_call() {
        let usage_handler = RecordingUsageHandler::default();
        let provider = StaticStreamProvider {
            events: Arc::new(vec![
                start("req-1"),
                call_done("tool-1", "client_tool", "{}"),
                usage_event(usage(22, 4, 26)),
                completed(Some("tool_calls")),
            ]),
        };
        let config = AgentLoopConfig {
            max_calls: 1,
            usage_handler: Arc::new(usage_handler.clone()),
            ..AgentLoopConfig::default()
        };

        let result = run_agent_loop::<(), ()>(
            Arc::new(provider),
            stream_request(),
            HashMap::new(),
            Arc::new(NoopToolInvoker),
            (),
            "trace-1".to_string(),
            None,
            config,
        )
        .await
        .expect("stream run should succeed");

        let AgentLoopResult::Stream { mut events } = result else {
            panic!("expected stream result");
        };
        let mut output_usages = Vec::new();
        while let Some(event) = events.next().await {
            if let UnifiedEventKind::Usage { usage, .. } =
                event.expect("stream event should be ok").kind
            {
                output_usages.push(usage.into_payload("/chat/completions"));
            }
        }

        assert_eq!(output_usages.len(), 1);
        assert_eq!(output_usages[0]["prompt_tokens"], 22);
        assert_eq!(output_usages[0]["completion_tokens"], 4);
        assert_eq!(output_usages[0]["total_tokens"], 26);
        let calls = usage_handler.captured();
        assert_eq!(calls.len(), 1);
        assert_eq!(
            calls[0].clone().into_payload("/chat/completions"),
            serde_json::json!({
                "prompt_tokens": 22,
                "completion_tokens": 4,
                "total_tokens": 26
            })
        );
    }

    #[tokio::test]
    async fn run_agent_loop_nonstream_passes_raw_generate_content_usage_to_handler() {
        let usage_handler = RecordingUsageHandler::default();
        let provider = StaticCompleteProvider {
            response: {
                let mut response = UnifiedResponse::new("req-1".into(), "gemini-2.5-flash".into());
                response.usage = Some(generate_content_usage(11, 7, 18));
                response
            },
        };
        let config = AgentLoopConfig {
            max_calls: 1,
            usage_handler: Arc::new(usage_handler.clone()),
            ..AgentLoopConfig::default()
        };

        let mut request = stream_request();
        request.controls.stream = Some(false);
        let result = run_agent_loop::<(), ()>(
            Arc::new(provider),
            request,
            HashMap::new(),
            Arc::new(NoopToolInvoker),
            (),
            "trace-1".to_string(),
            None,
            config,
        )
        .await
        .expect("non-stream run should succeed");

        let AgentLoopResult::NonStream(response) = result else {
            panic!("expected non-stream result");
        };
        assert_eq!(
            response
                .usage
                .expect("usage")
                .into_payload("/chat/completions")["prompt_tokens"],
            11
        );
        let calls = usage_handler.captured();
        assert_eq!(calls.len(), 1);
        let EndpointUsage::GenerateContent(usage) = &calls[0] else {
            panic!("expected generate content usage");
        };
        assert_eq!(usage.prompt_token_count, Some(11));
        assert_eq!(usage.candidates_token_count, Some(7));
        assert_eq!(usage.total_token_count, Some(18));
    }

    #[tokio::test]
    async fn run_agent_loop_stream_passes_raw_generate_content_usage_to_handler() {
        let usage_handler = RecordingUsageHandler::default();
        let provider = StaticStreamProvider {
            events: Arc::new(vec![
                start("req-1"),
                usage_event(generate_content_usage(13, 5, 18)),
                completed(Some("stop")),
            ]),
        };
        let config = AgentLoopConfig {
            max_calls: 1,
            usage_handler: Arc::new(usage_handler.clone()),
            ..AgentLoopConfig::default()
        };

        let result = run_agent_loop::<(), ()>(
            Arc::new(provider),
            stream_request(),
            HashMap::new(),
            Arc::new(NoopToolInvoker),
            (),
            "trace-1".to_string(),
            None,
            config,
        )
        .await
        .expect("stream run should succeed");

        let AgentLoopResult::Stream { mut events } = result else {
            panic!("expected stream result");
        };
        let mut output_usages = Vec::new();
        while let Some(event) = events.next().await {
            if let UnifiedEventKind::Usage { usage, .. } =
                event.expect("stream event should be ok").kind
            {
                output_usages.push(usage.into_payload("/chat/completions"));
            }
        }

        assert_eq!(output_usages.len(), 1);
        assert_eq!(output_usages[0]["prompt_tokens"], 13);
        let calls = usage_handler.captured();
        assert_eq!(calls.len(), 1);
        let EndpointUsage::GenerateContent(usage) = &calls[0] else {
            panic!("expected generate content usage");
        };
        assert_eq!(usage.prompt_token_count, Some(13));
        assert_eq!(usage.candidates_token_count, Some(5));
        assert_eq!(usage.total_token_count, Some(18));
    }

    #[tokio::test]
    async fn run_agent_loop_stream_resolves_empty_tool_delta_name_to_server_tool() {
        let provider = SequencedStreamProvider {
            rounds: Arc::new(vec![
                vec![
                    start("req-1"),
                    {
                        let event = call_done("call_weather", "", "");
                        let UnifiedEventKind::OutputItemDone { item } = event.kind else {
                            unreachable!()
                        };
                        UnifiedEvent {
                            kind: UnifiedEventKind::OutputItemAdded { item },
                            ..event
                        }
                    },
                    call_delta("call_weather", "{\"location\""),
                    call_done("call_weather", "get_weather", "{\"location\":\"beijing\"}"),
                    completed(Some("tool_calls")),
                ],
                vec![
                    start("req-2"),
                    text_event("req-2", "晴天"),
                    completed(Some("stop")),
                ],
            ]),
            next_round: Arc::new(Mutex::new(0)),
        };
        let invoker = RecordingToolInvoker::default();
        let mut server_tools = HashMap::new();
        server_tools.insert(
            "get_weather".to_string(),
            serde_json::json!({
                "description": "Query weather",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "location": {"type": "string"}
                    },
                    "required": ["location"]
                }
            })
            .to_string(),
        );

        let result = run_agent_loop::<(), ()>(
            Arc::new(provider),
            stream_request(),
            server_tools,
            Arc::new(invoker.clone()),
            (),
            "trace-1".to_string(),
            None,
            AgentLoopConfig::default(),
        )
        .await
        .expect("stream run should succeed");

        let AgentLoopResult::Stream { mut events } = result else {
            panic!("expected stream result");
        };
        let mut seen_empty_client_tool_event = false;
        while let Some(event) = events.next().await {
            match event.expect("stream event should be ok").kind {
                UnifiedEventKind::OutputItemAdded { item }
                | UnifiedEventKind::OutputItemDone { item } => {
                    let UnifiedOutputItemKind::FunctionCall { name, .. } = item.kind else {
                        continue;
                    };
                    if name.trim().is_empty() {
                        seen_empty_client_tool_event = true;
                    }
                }
                _ => {}
            }
        }

        assert!(!seen_empty_client_tool_event);
        assert_eq!(
            invoker.captured(),
            vec!["{\"location\":\"beijing\"}".to_string()]
        );
    }

    #[tokio::test]
    async fn run_agent_loop_nonstream_stops_when_total_tokens_reach_limit() {
        let provider = SequencedCompleteProvider {
            rounds: Arc::new(vec![{
                let mut response = UnifiedResponse::new("req-1".into(), "mock-model".into());
                super::apply_event(
                    &mut response,
                    &call_done("call-1", "get_weather", "{\"location\":\"beijing\"}"),
                )
                .expect("call");
                response.usage = Some(usage(12, 8, 20));
                response
            }]),
            next_round: Arc::new(Mutex::new(0)),
        };
        let invoker = RecordingToolInvoker::default();
        let mut server_tools = HashMap::new();
        server_tools.insert(
            "get_weather".to_string(),
            serde_json::json!({
                "description": "Query weather",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "location": {"type": "string"}
                    },
                    "required": ["location"]
                }
            })
            .to_string(),
        );

        let mut request = stream_request();
        request.controls.stream = Some(false);
        let result = run_agent_loop::<(), ()>(
            Arc::new(provider.clone()),
            request,
            server_tools,
            Arc::new(invoker.clone()),
            (),
            "trace-1".to_string(),
            None,
            AgentLoopConfig {
                max_total_tokens: Some(10),
                ..AgentLoopConfig::default()
            },
        )
        .await
        .expect("non-stream run should succeed");

        let AgentLoopResult::NonStream(response) = result else {
            panic!("expected non-stream result");
        };
        assert!(response_tool_calls(&response).is_empty());
        assert!(invoker.captured().is_empty());
        assert_eq!(*provider.next_round.lock().expect("next round lock"), 1);
    }

    #[tokio::test]
    async fn run_agent_loop_stream_stops_when_total_tokens_reach_limit() {
        let provider = SequencedStreamProvider {
            rounds: Arc::new(vec![
                vec![
                    start("req-1"),
                    call_done("call-1", "get_weather", "{\"location\":\"beijing\"}"),
                    usage_event(usage(12, 8, 20)),
                    completed(Some("tool_calls")),
                ],
                vec![start("req-2")],
            ]),
            next_round: Arc::new(Mutex::new(0)),
        };
        let invoker = RecordingToolInvoker::default();
        let mut server_tools = HashMap::new();
        server_tools.insert(
            "get_weather".to_string(),
            serde_json::json!({
                "description": "Query weather",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "location": {"type": "string"}
                    },
                    "required": ["location"]
                }
            })
            .to_string(),
        );

        let result = run_agent_loop::<(), ()>(
            Arc::new(provider.clone()),
            stream_request(),
            server_tools,
            Arc::new(invoker.clone()),
            (),
            "trace-1".to_string(),
            None,
            AgentLoopConfig {
                max_total_tokens: Some(10),
                ..AgentLoopConfig::default()
            },
        )
        .await
        .expect("stream run should succeed");

        let AgentLoopResult::Stream { mut events } = result else {
            panic!("expected stream result");
        };
        let mut seen_server_tool_event = false;
        while let Some(event) = events.next().await {
            match event.expect("stream event should be ok").kind {
                UnifiedEventKind::OutputItemAdded { item }
                | UnifiedEventKind::OutputItemDone { item } => {
                    let UnifiedOutputItemKind::FunctionCall { name, .. } = item.kind else {
                        continue;
                    };
                    if name == "get_weather" {
                        seen_server_tool_event = true;
                    }
                }
                _ => {}
            }
        }

        assert!(!seen_server_tool_event);
        assert!(invoker.captured().is_empty());
        assert_eq!(*provider.next_round.lock().expect("next round lock"), 1);
    }
}
