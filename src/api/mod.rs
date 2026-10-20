//! Client APIs. Each API owns its native/unified conversions.

pub mod chat_completions;
pub mod hosted_tools;
pub mod messages;
pub mod responses;

use crate::provider::{
    UnifiedContentPart, UnifiedOutputItem, UnifiedRequest, UnifiedResponse, UnifiedSourceProtocol,
};
use crate::serve_config::EndpointKind;
use serde_json::Value;

/// Dispatch request parsing to the originating protocol.
pub fn request_from_value(
    value: &Value,
    protocol: UnifiedSourceProtocol,
) -> Result<UnifiedRequest, String> {
    match protocol {
        UnifiedSourceProtocol::ChatCompletions => chat_completions::request_from_value(value),
        UnifiedSourceProtocol::Responses => responses::request_from_value(value),
        UnifiedSourceProtocol::Messages => messages::request_from_value(value),
    }
}

/// Dispatch native request serialization to the selected upstream protocol.
pub fn request_to_value(
    request: &UnifiedRequest,
    protocol: UnifiedSourceProtocol,
) -> Result<Value, String> {
    match protocol {
        UnifiedSourceProtocol::ChatCompletions => chat_completions::request_to_value(request),
        UnifiedSourceProtocol::Responses => responses::request_to_value(request),
        UnifiedSourceProtocol::Messages => messages::request_to_value(request),
    }
}

/// Dispatch complete native response parsing.
pub fn response_from_value(
    value: &Value,
    protocol: UnifiedSourceProtocol,
) -> Result<UnifiedResponse, String> {
    match protocol {
        UnifiedSourceProtocol::ChatCompletions => chat_completions::response_from_value(value),
        UnifiedSourceProtocol::Responses => responses::response_from_value(value),
        UnifiedSourceProtocol::Messages => messages::response_from_value(value),
    }
}

/// Dispatch content conversion; native block schemas belong to their protocols.
pub fn parse_content(
    value: &Value,
    protocol: UnifiedSourceProtocol,
) -> Result<Vec<UnifiedContentPart>, String> {
    match protocol {
        UnifiedSourceProtocol::ChatCompletions => chat_completions::parse_content(value),
        UnifiedSourceProtocol::Responses => responses::parse_content(value),
        UnifiedSourceProtocol::Messages => messages::parse_content(value),
    }
}

pub fn content_value(
    part: &UnifiedContentPart,
    protocol: UnifiedSourceProtocol,
    output: bool,
) -> Result<Value, String> {
    match protocol {
        UnifiedSourceProtocol::ChatCompletions => chat_completions::content_value(part),
        UnifiedSourceProtocol::Responses => responses::content_value(part, output),
        UnifiedSourceProtocol::Messages => messages::content_value(part),
    }
}

pub fn output_item_value(
    item: &UnifiedOutputItem,
    protocol: UnifiedSourceProtocol,
) -> Result<Value, String> {
    match protocol {
        UnifiedSourceProtocol::Responses => responses::output_item_value(item),
        UnifiedSourceProtocol::Messages => messages::output_item_value(item),
        UnifiedSourceProtocol::ChatCompletions => {
            Err("Chat output items must be serialized as a complete choice".into())
        }
    }
}

/// Client API selected by the request endpoint.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Api {
    ChatCompletions,
    Responses,
    Messages,
}

impl Api {
    /// Select a conversation API; other endpoints use their own handlers.
    pub fn from_endpoint(kind: EndpointKind) -> Option<Self> {
        match kind {
            EndpointKind::ChatCompletions => Some(Self::ChatCompletions),
            EndpointKind::Responses => Some(Self::Responses),
            EndpointKind::Messages => Some(Self::Messages),
            _ => None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::{Api, EndpointKind};

    #[test]
    fn conversation_api_selection_excludes_forwarding_endpoints() {
        for (endpoint, expected) in [
            (EndpointKind::ChatCompletions, Api::ChatCompletions),
            (EndpointKind::Responses, Api::Responses),
            (EndpointKind::Messages, Api::Messages),
        ] {
            assert_eq!(Api::from_endpoint(endpoint), Some(expected));
        }
        for endpoint in [
            EndpointKind::SystemOne,
            EndpointKind::Embeddings,
            EndpointKind::Rerank,
            EndpointKind::AudioSpeech,
            EndpointKind::AudioTranscriptions,
            EndpointKind::ImagesGenerations,
            EndpointKind::ImagesEdits,
            EndpointKind::ModelsGenerateContent,
        ] {
            assert_eq!(Api::from_endpoint(endpoint), None);
        }
    }
}
