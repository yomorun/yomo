//! YoMo core library.
//!
//! This crate provides the transport, routing, and bridge abstractions used to
//! connect tools and the zipper runtime.

/// Handshake authentication abstractions.
pub mod auth;
/// Request forwarding bridge implementations.
pub mod bridge;
/// YoMo client implementation.
pub mod client;
/// Connector abstractions for opening downstream streams.
pub mod connector;
/// Framed IO helpers.
pub mod io;
/// LLM-facing HTTP routers.
pub mod llm_router;
/// LLM provider abstractions.
pub mod llm_provider;
/// Manage user-defined metadata extension.
pub mod metadata_mgr;
/// Routing traits and implementations.
pub mod router;
/// Serverless runtime and handlers.
pub mod serverless;
/// TLS configuration helpers.
pub mod tls;
/// Tool-facing HTTP APIs.
pub mod tool_api;
/// Tool manager trait and implementation.
pub mod tool_mgr;
/// Shared protocol types.
pub mod types;
/// OpenAI request/response types.
pub mod openai_types;
/// Utility helpers.
pub mod utils;
/// Zipper coordinator implementation.
pub mod zipper;
/// Agent loop implementation.
pub mod agent_loop;
/// Tool invoker implementation.
pub mod tool_invoker;
/// OpenAI request/response mapping to Events.
pub mod openai_http_mapping;

/// OpenTelemetry tracing setup.
pub mod trace;
/// Server configuration used by the CLI.
pub mod serve_config;
/// Model API router.
pub mod model_api_router;
/// Model API providers.
pub mod model_api_provider;
