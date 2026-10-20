//! Responses protocol adapters share the same unified contract as the engine.
use crate::api::responses::{error_from_value, output_item_from_value, parse_content};
use crate::api::{request_to_value, response_from_value};
use crate::openai_responses_types::{ResponsesRequest, ResponsesResponse, ResponsesStreamEvent};
use crate::provider::*;
use secrecy::{ExposeSecret, SecretString};
use serde_json::{Value, json};
pub(super) fn apply_optional_bearer_auth(
    builder: reqwest::RequestBuilder,
    key: Option<&SecretString>,
) -> reqwest::RequestBuilder {
    match key
        .map(ExposeSecret::expose_secret)
        .filter(|key| !key.trim().is_empty())
    {
        Some(key) => builder.bearer_auth(key),
        None => builder,
    }
}
pub(super) fn upstream_error(status: reqwest::StatusCode, body: &[u8]) -> ProviderError {
    if let Some(error) = crate::upstreams::azure_openai::parse_compatible_error(status, body) {
        return ProviderError::Public { status, error };
    }
    ProviderError::internal_with_upstream_status(status, String::from_utf8_lossy(body).into_owned())
}

#[cfg(test)]
mod tests {
    use super::*;
    use reqwest::header;

    #[test]
    fn ir_to_native_preserves_reasoning_encryption_and_tool_loop() {
        let raw = json!({"model":"codex","input":[{"type":"message","role":"system","content":"be terse"},{"type":"message","role":"user","content":"run shell"},{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"thinking"}],"encrypted_content":"enc-blob"},{"type":"function_call","call_id":"call_1","name":"shell","arguments":"{\"cmd\":\"ls\"}"},{"type":"function_call_output","call_id":"call_1","output":"{\"out\":\"ok\"}"}],"tools":[{"type":"function","name":"shell","parameters":{"type":"object"}}],"reasoning":{"effort":"high"},"max_output_tokens":1024,"stream":true});
        let request = crate::api::request_from_value(&raw, UnifiedSourceProtocol::Responses)
            .expect("request");
        let native = ir_to_responses_request(&request, "gpt-5.3-codex").expect("native");
        let value = serde_json::to_value(native).expect("value");
        assert_eq!(value["input"].as_array().expect("items").len(), 5);
        assert_eq!(value["input"][2]["encrypted_content"], "enc-blob");
        assert_eq!(value["input"][3]["call_id"], value["input"][4]["call_id"]);
        assert_eq!(value["reasoning"]["effort"], "high");
        assert_eq!(value["max_output_tokens"], 1024);
        assert_eq!(value["stream"], true);
    }

    #[test]
    fn native_response_maps_reasoning_blocks_and_tool_calls() {
        let response:ResponsesResponse=serde_json::from_value(json!({"id":"resp_1","created_at":1,"status":"completed","model":"codex","output":[{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"thinking"}],"encrypted_content":"enc-blob"},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"shell","arguments":"{}"},{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"done","annotations":[]}]}],"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}})).expect("response");
        let unified = responses_to_unified(response).expect("map");
        assert_eq!(unified.id, "resp_1");
        assert!(
            matches!(&unified.candidates[0].output[0].kind,UnifiedOutputItemKind::Reasoning {encrypted_content:Some(data),..}if data=="enc-blob")
        );
        assert!(
            matches!(&unified.candidates[0].output[1].kind,UnifiedOutputItemKind::FunctionCall {call_id,name,..}if call_id=="call_1"&&name=="shell")
        );
        assert!(matches!(
            unified.usage,
            Some(crate::usage_handler::EndpointUsage::Responses(_))
        ));
    }

    #[test]
    fn stream_events_map_to_unified_sequence() {
        let mut state = StreamMapState::default();
        let created=stream_event_from_value(&json!({"type":"response.created","response":{"id":"r","created_at":1,"status":"in_progress","model":"codex"},"sequence_number":0}),&mut state).expect("created");
        assert!(matches!(
            created[0].kind,
            UnifiedEventKind::ResponseCreated { .. }
        ));
        let added=stream_event_from_value(&json!({"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc","call_id":"call","name":"shell","arguments":""},"sequence_number":1}),&mut state).expect("added");
        assert!(matches!(
            added[0].kind,
            UnifiedEventKind::OutputItemAdded { .. }
        ));
        let delta=stream_event_from_value(&json!({"type":"response.function_call_arguments.delta","item_id":"fc","output_index":0,"delta":"{","sequence_number":2}),&mut state).expect("delta");
        assert!(
            matches!(&delta[0].kind,UnifiedEventKind::ItemDelta {delta:UnifiedItemDelta::FunctionArguments(args)}if args=="{")
        );
        let failed=stream_event_from_value(&json!({"type":"response.failed","response":{"id":"r","created_at":1,"status":"failed","model":"codex","error":{"code":"server_error","message":"boom"}},"sequence_number":3}),&mut state).expect("failed");
        assert!(
            matches!(&failed[0].kind,UnifiedEventKind::Failed {error,..}if error.code.as_deref()==Some("server_error"))
        );
    }

    #[test]
    fn optional_bearer_auth_skips_header_for_empty_key() {
        let request = apply_optional_bearer_auth(
            reqwest::Client::new().post("https://example.com"),
            Some(&SecretString::from(" ")),
        )
        .build()
        .expect("request");
        assert!(request.headers().get(header::AUTHORIZATION).is_none());
    }
    #[test]
    fn optional_bearer_auth_sets_header_for_non_empty_key() {
        let request = apply_optional_bearer_auth(
            reqwest::Client::new().post("https://example.com"),
            Some(&SecretString::from("key")),
        )
        .build()
        .expect("request");
        assert_eq!(
            request
                .headers()
                .get(header::AUTHORIZATION)
                .expect("authorization"),
            "Bearer key"
        );
    }
    #[test]
    fn optional_bearer_auth_skips_header_for_missing_auth() {
        let request =
            apply_optional_bearer_auth(reqwest::Client::new().post("https://example.com"), None)
                .build()
                .expect("request");
        assert!(request.headers().get(header::AUTHORIZATION).is_none());
    }
    #[test]
    fn responses_encoding_preserves_exact_effort_and_explicit_thinking() {
        let mut request = UnifiedRequest::default();
        request.options.reasoning_effort = Some("medium".into());
        request.options.thinking = Some(json!({"type":"disabled"}));
        let body = request_to_value(&request, UnifiedSourceProtocol::Responses).expect("encoding");
        assert_eq!(body["reasoning"]["effort"], "medium");
        assert_eq!(body["thinking"]["type"], "disabled");
    }
    #[test]
    fn stream_summary_and_function_arguments_target_different_locations() {
        let mut state = StreamMapState {
            response_id: "r".into(),
        };
        let summary = stream_event_from_value(&json!({"type":"response.reasoning_summary_text.delta","item_id":"rs","output_index":0,"summary_index":1,"delta":"plan","sequence_number":5}), &mut state).expect("summary");
        let arguments = stream_event_from_value(&json!({"type":"response.function_call_arguments.delta","item_id":"fc","output_index":1,"delta":"{","sequence_number":6}), &mut state).expect("arguments");
        assert_eq!(
            summary[0].target.as_ref().expect("target").part,
            Some(UnifiedPartLocation::ReasoningSummary(1))
        );
        assert_eq!(arguments[0].target.as_ref().expect("target").part, None);
        assert_eq!(arguments[0].sequence_number, Some(6));
    }
}
pub fn ir_to_responses_request(
    request: &UnifiedRequest,
    model: &str,
) -> Result<ResponsesRequest, ProviderError> {
    let mut body = request_to_value(request, UnifiedSourceProtocol::Responses)
        .map_err(ProviderError::internal)?;
    body["model"] = json!(model);
    serde_json::from_value(body).map_err(|e| ProviderError::internal(e.to_string()))
}
pub fn responses_to_unified(response: ResponsesResponse) -> Result<UnifiedResponse, ProviderError> {
    response_from_value(
        &serde_json::to_value(response).map_err(|e| ProviderError::internal(e.to_string()))?,
        UnifiedSourceProtocol::Responses,
    )
    .map_err(ProviderError::internal)
}

#[derive(Default)]
pub struct StreamMapState {
    pub response_id: String,
}

pub fn map_stream_event(
    event: ResponsesStreamEvent,
    state: &mut StreamMapState,
) -> Vec<UnifiedEvent> {
    let Ok(value) = serde_json::to_value(event) else {
        return vec![];
    };
    stream_event_from_value(&value, state).unwrap_or_else(|message| {
        vec![UnifiedEvent::new(UnifiedEventKind::Failed {
            error: UnifiedResponseError {
                code: None,
                error_type: None,
                param: None,
                message,
                source_extensions: None,
            },
            response: None,
        })]
    })
}

pub fn stream_event_from_value(
    value: &Value,
    state: &mut StreamMapState,
) -> Result<Vec<UnifiedEvent>, String> {
    let protocol = UnifiedSourceProtocol::Responses;
    let kind = value["type"].as_str().unwrap_or("");
    let item_id = value["item_id"]
        .as_str()
        .or_else(|| value["item"]["id"].as_str())
        .unwrap_or("")
        .to_owned();
    let response = || response_from_value(&value["response"], protocol);
    let output_index = value["output_index"].as_u64().unwrap_or(0) as u32;
    let mut target = UnifiedEventTarget {
        candidate_index: 0,
        item_id: (!item_id.is_empty()).then_some(item_id.clone()),
        output_index: Some(output_index),
        part: None,
    };
    if let Some(index) = value["summary_index"].as_u64() {
        target.part = Some(UnifiedPartLocation::ReasoningSummary(index as u32));
    } else if let Some(index) = value["content_index"].as_u64() {
        target.part = Some(UnifiedPartLocation::Content(index as u32));
    }
    let text = |key: &str| value[key].as_str().unwrap_or("").to_owned();
    let payload = match kind {
        "response.created" => {
            let response = response()?;
            state.response_id = response.id.clone();
            UnifiedEventKind::ResponseCreated { response }
        }
        "response.queued" => UnifiedEventKind::ResponseQueued {
            response: Some(response()?),
        },
        "response.in_progress" => UnifiedEventKind::ResponseInProgress {
            response: Some(response()?),
        },
        "response.completed" => UnifiedEventKind::Completed {
            response: Some(response()?),
        },
        "response.incomplete" => UnifiedEventKind::Incomplete {
            reason: value["response"]["incomplete_details"]["reason"]
                .as_str()
                .map(str::to_owned),
            response: Some(response()?),
        },
        "response.failed" => UnifiedEventKind::Failed {
            error: error_from_value(&value["response"]["error"]),
            response: Some(response()?),
        },
        "response.output_item.added" => UnifiedEventKind::OutputItemAdded {
            item: output_item_from_value(&value["item"], item_id.clone())?,
        },
        "response.output_item.done" => UnifiedEventKind::OutputItemDone {
            item: output_item_from_value(&value["item"], item_id.clone())?,
        },
        "response.content_part.added" => UnifiedEventKind::ContentPartAdded {
            part: parse_content(&json!([value["part"]]))?
                .into_iter()
                .next()
                .ok_or("missing part")?,
        },
        "response.content_part.done" => UnifiedEventKind::ContentPartDone {
            part: parse_content(&json!([value["part"]]))?
                .into_iter()
                .next()
                .ok_or("missing part")?,
        },
        "response.output_text.delta" => UnifiedEventKind::ContentPartDelta {
            delta: if let Some(probs) = value["logprobs"].as_array() {
                UnifiedContentDelta::TextWithLogprobs {
                    text: text("delta"),
                    logprobs: probs.clone(),
                }
            } else {
                UnifiedContentDelta::Text(text("delta"))
            },
        },
        "response.output_text.done" => UnifiedEventKind::ContentValueDone {
            value: UnifiedContentDelta::Text(text("text")),
        },
        "response.refusal.delta" => UnifiedEventKind::ContentPartDelta {
            delta: UnifiedContentDelta::Refusal(text("delta")),
        },
        "response.refusal.done" => UnifiedEventKind::ContentValueDone {
            value: UnifiedContentDelta::Refusal(text("refusal")),
        },
        "response.output_text.annotation.added" => UnifiedEventKind::ContentPartDelta {
            delta: UnifiedContentDelta::AnnotationAdded {
                index: value["annotation_index"].as_u64().unwrap_or(0) as u32,
                annotation: value["annotation"].clone(),
            },
        },
        "response.function_call_arguments.delta" => UnifiedEventKind::ItemDelta {
            delta: UnifiedItemDelta::FunctionArguments(text("delta")),
        },
        "response.function_call_arguments.done" => UnifiedEventKind::FunctionArgumentsDone {
            arguments: text("arguments"),
        },
        "response.reasoning_summary_part.added" => UnifiedEventKind::ReasoningSummaryPartAdded {
            part: value["part"].clone(),
        },
        "response.reasoning_summary_part.done" => UnifiedEventKind::ReasoningSummaryPartDone {
            part: value["part"].clone(),
            status: value["status"].as_str().map(str::to_owned),
        },
        "response.reasoning_summary_text.delta" => UnifiedEventKind::ReasoningSummaryTextDelta {
            delta: text("delta"),
        },
        "response.reasoning_summary_text.done" => {
            UnifiedEventKind::ReasoningSummaryTextDone { text: text("text") }
        }
        "response.reasoning_text.delta" => UnifiedEventKind::ContentPartDelta {
            delta: UnifiedContentDelta::Thinking(text("delta")),
        },
        "response.reasoning_text.done" => UnifiedEventKind::ContentValueDone {
            value: UnifiedContentDelta::Thinking(text("text")),
        },
        "error" | "response.error" => UnifiedEventKind::Failed {
            error: error_from_value(value),
            response: None,
        },
        _ => UnifiedEventKind::Other {
            event_type: kind.into(),
            payload: value.clone(),
        },
    };
    let mut event = UnifiedEvent::new(payload);
    event.response_id = Some(state.response_id.clone());
    event.sequence_number = value["sequence_number"].as_i64();
    if !item_id.is_empty() {
        event.target = Some(target);
    }
    event.source_extensions = source_extensions(
        value,
        &[
            "type",
            "sequence_number",
            "response",
            "item",
            "item_id",
            "output_index",
            "content_index",
            "summary_index",
            "part",
            "delta",
            "text",
            "refusal",
            "arguments",
            "annotation_index",
            "annotation",
            "status",
            "logprobs",
        ],
        protocol,
    );
    Ok(vec![event])
}
