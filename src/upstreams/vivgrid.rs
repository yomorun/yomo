//! Vivgrid error interpretation; account metadata remains owned by the application.

use super::UpstreamError;
use axum::http::{HeaderMap, StatusCode};

/// Interpret Vivgrid's OpenAI-style envelope without modifying authentication metadata.
pub fn parse_error(status: StatusCode, headers: &HeaderMap, body: &[u8]) -> UpstreamError {
    super::tokenhub::parse_error(status, headers, body)
}
