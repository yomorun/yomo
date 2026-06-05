//! YoMo core library.
//!
//! This crate provides the transport, routing, and bridge abstractions used to
//! connect tools and the zipper runtime.

/// Agent loop implementation.
pub mod agent_loop;
/// Handshake authentication abstractions.
pub mod auth;
/// Request forwarding bridge implementations.
pub mod bridge;
/// YoMo client implementation.
pub mod client;
/// Connector abstractions for opening downstream streams.
pub mod connector;
/// HTTP authentication middleware.
pub mod http_auth;
/// Framed IO helpers.
pub mod io;
/// LLM-facing HTTP APIs.
pub mod llm_api;
/// LLM provider abstractions.
pub mod llm_provider;
/// Stream mapper abstractions for LLM streaming output.
pub mod llm_stream_mapper;
/// Manage user-defined metadata extension.
pub mod metadata_mgr;
/// Model API HTTP APIs.
pub mod model_api;
/// Model API providers.
pub mod model_api_provider;
/// Models list HTTP API.
pub mod model_list;
/// OpenAI request/response mapping to Events.
pub mod openai_http_mapping;
/// OpenAI request/response types.
pub mod openai_types;
/// Provider error notifier abstractions.
pub mod provider_error_notifier;
/// Routing traits and implementations.
pub mod router;
/// Server configuration used by the CLI.
pub mod serve_config;
/// Serverless runtime and handlers.
pub mod serverless;
/// TLS configuration helpers.
pub mod tls;
/// Tool-facing HTTP APIs.
pub mod tool_api;
/// Tool invoker implementation.
pub mod tool_invoker;
/// Tool manager trait and implementation.
pub mod tool_mgr;
/// OpenTelemetry tracing setup.
pub mod trace;
/// Shared protocol types.
pub mod types;
/// Usage handler interfaces.
pub mod usage_handler;
/// Utility helpers.
pub mod utils;
/// Zipper coordinator implementation.
pub mod zipper;
