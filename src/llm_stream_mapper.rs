use std::{pin::Pin, sync::Arc};

use axum::{body::Bytes, http::HeaderMap};
use futures_core::Stream;
use tracing::Span;

use crate::{
    llm_provider::{ProviderError, UnifiedEvent},
    openai_http_mapping::stream_openai_chunks,
};

pub type UnifiedEventStream =
    Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send>>;
pub type StreamBytes = Pin<Box<dyn Stream<Item = Result<Bytes, std::io::Error>> + Send>>;

pub trait StreamChunkMapper: Send + Sync {
    fn map_stream(
        &self,
        stream: UnifiedEventStream,
        trace_id: String,
        default_model: String,
        root_span: Span,
    ) -> StreamBytes;
}

pub trait StreamMapperSelector: Send + Sync {
    fn select(&self, headers: &HeaderMap) -> Arc<dyn StreamChunkMapper>;
}

#[derive(Default)]
pub struct OpenAiSseStreamMapper;

impl StreamChunkMapper for OpenAiSseStreamMapper {
    fn map_stream(
        &self,
        stream: UnifiedEventStream,
        trace_id: String,
        default_model: String,
        root_span: Span,
    ) -> StreamBytes {
        Box::pin(stream_openai_chunks(
            stream,
            trace_id,
            default_model,
            root_span,
        ))
    }
}

pub struct DefaultStreamMapperSelector {
    default_mapper: Arc<dyn StreamChunkMapper>,
}

impl Default for DefaultStreamMapperSelector {
    fn default() -> Self {
        Self {
            default_mapper: Arc::new(OpenAiSseStreamMapper),
        }
    }
}

impl StreamMapperSelector for DefaultStreamMapperSelector {
    fn select(&self, _headers: &HeaderMap) -> Arc<dyn StreamChunkMapper> {
        Arc::clone(&self.default_mapper)
    }
}
