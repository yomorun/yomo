//! YoMo core library.
//!
//! This crate provides the transport, routing, and bridge abstractions used to
//! connect tools and the zipper runtime.

/// Request forwarding bridge implementations.
pub mod bridge;
/// YoMo client implementation.
pub mod client;
/// Connector abstractions for opening downstream streams.
pub mod connector;
/// Framed IO helpers.
pub mod io;
/// LLM-facing HTTP APIs.
pub mod llm_api;
/// Routing traits and implementations.
pub mod router;
/// Serverless runtime and handlers.
pub mod serverless;
/// TLS configuration helpers.
pub mod tls;
/// Tool-facing HTTP APIs.
pub mod tool_api;
/// Shared protocol types.
pub mod types;
/// Utility helpers.
pub mod utils;
/// Zipper coordinator implementation.
pub mod zipper;
