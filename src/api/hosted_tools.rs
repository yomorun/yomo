//! Keeps upstream models doing model work only.
//!
//! Tools that the upstream platform executes itself (web search, URL fetch,
//! code execution, retrieval, remote MCP, ...) are removed before a request is
//! forwarded. They are billed per call or per container outside token usage,
//! so the cost cannot be attributed to a caller, and whether they exist at all
//! differs per vendor and model. Tools the client executes (functions, custom
//! tools, local shell, computer use, ...) are only model output and are kept.
//!
//! Each endpoint keeps an allowlist of client-executed tools, so tool types an
//! upstream adds later are removed until they are reviewed here. Hosted tools
//! are removed rather than rejected: agent clients declare them by default
//! (Codex sends `web_search` with every request), and an upstream without them
//! drops them silently too.

use axum::body::Bytes;
use serde_json::{Map, Value};

use crate::serve_config::EndpointKind;

/// `/responses` tool types executed by the client. `namespace`, `tool_search`
/// and `shell` are checked further in [`is_client_responses_tool`].
const RESPONSES_CLIENT_TOOL_TYPES: [&str; 6] = [
    "function",
    "custom",
    "local_shell",
    "apply_patch",
    "computer",
    "computer_use_preview",
];

/// `/messages` tool type prefixes executed by the client (Anthropic-defined
/// client tools such as `bash_20250124`); tools without a type are custom tools.
const MESSAGES_CLIENT_TOOL_TYPE_PREFIXES: [&str; 4] =
    ["bash_", "text_editor_", "computer_", "memory_"];

/// `/messages` top-level fields that make the upstream call MCP servers or
/// run a code execution container.
const MESSAGES_HOSTED_FIELDS: [&str; 2] = ["mcp_servers", "container"];

/// generateContent `Tool` fields executed by the client, normalized by
/// [`normalize_field_name`].
const GENERATE_CONTENT_CLIENT_TOOL_FIELDS: [&str; 2] = ["functiondeclarations", "computeruse"];

/// Removes upstream-executed tools from a request object, recording what was removed.
type StripFn = fn(&mut Map<String, Value>, &mut Vec<String>) -> Result<(), String>;

/// Removes upstream-executed tools from requests to `endpoint` and returns the
/// body to forward together with the names of what was removed.
///
/// The body is always re-serialized for the checked endpoints, regardless of
/// the request content type, so upstream parses exactly the JSON checked here
/// (e.g. a duplicated `tools` key cannot slip past the check). Bodies of other
/// endpoints are returned unchanged.
pub(crate) fn strip_hosted_tools(
    endpoint: EndpointKind,
    body: Bytes,
) -> Result<(Bytes, Vec<String>), String> {
    let strip: StripFn = match endpoint {
        EndpointKind::Responses => strip_responses,
        EndpointKind::Messages => strip_messages,
        EndpointKind::ModelsGenerateContent => strip_generate_content,
        _ => return Ok((body, Vec::new())),
    };
    let mut payload: Value =
        serde_json::from_slice(&body).map_err(|err| format!("invalid json body: {err}"))?;
    let Some(request) = payload.as_object_mut() else {
        return Err("request body must be a json object".to_string());
    };
    let mut removed = Vec::new();
    strip(request, &mut removed)?;
    let body = serde_json::to_vec(&payload)
        .map(Bytes::from)
        .map_err(|err| format!("invalid json body: {err}"))?;
    Ok((body, removed))
}

fn strip_responses(
    request: &mut Map<String, Value>,
    removed: &mut Vec<String>,
) -> Result<(), String> {
    strip_tools(request, &["tool_choice"], |tools| {
        retain_responses_tools(tools, removed);
        Ok(())
    })
}

fn retain_responses_tools(tools: &mut Vec<Value>, removed: &mut Vec<String>) {
    tools.retain_mut(|tool| {
        let tool_type = tool
            .get("type")
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_string();
        let keep = if tool_type == "namespace" {
            // A namespace groups other tools; keep its client tools only and
            // drop the namespace when nothing is left in it.
            match tool.get_mut("tools").and_then(Value::as_array_mut) {
                Some(inner) if !inner.is_empty() => {
                    retain_responses_tools(inner, removed);
                    !inner.is_empty()
                }
                _ => true,
            }
        } else {
            is_client_responses_tool(&tool_type, tool)
        };
        if !keep && tool_type != "namespace" {
            removed.push(tool_label(tool_type));
        }
        keep
    });
}

fn is_client_responses_tool(tool_type: &str, tool: &Value) -> bool {
    match tool_type {
        // Hosted tool search loads deferred tools on OpenAI's side.
        "tool_search" => tool.get("execution").and_then(Value::as_str) == Some("client"),
        // Hosted shell runs in an OpenAI container.
        "shell" => match tool.get("environment") {
            None | Some(Value::Null) => true,
            Some(environment) => environment.get("type").and_then(Value::as_str) == Some("local"),
        },
        _ => RESPONSES_CLIENT_TOOL_TYPES.contains(&tool_type),
    }
}

fn strip_messages(
    request: &mut Map<String, Value>,
    removed: &mut Vec<String>,
) -> Result<(), String> {
    for field in MESSAGES_HOSTED_FIELDS {
        if request.remove(field).is_some() {
            removed.push(field.to_string());
        }
    }
    strip_tools(request, &["tool_choice"], |tools| {
        tools.retain(|tool| {
            let tool_type = match tool.get("type") {
                None | Some(Value::Null) => return true,
                Some(tool_type) => tool_type.as_str().unwrap_or_default(),
            };
            let keep = tool_type == "custom"
                || MESSAGES_CLIENT_TOOL_TYPE_PREFIXES
                    .iter()
                    .any(|prefix| tool_type.starts_with(prefix));
            if !keep {
                removed.push(tool_label(tool_type.to_string()));
            }
            keep
        });
        Ok(())
    })
}

fn strip_generate_content(
    request: &mut Map<String, Value>,
    removed: &mut Vec<String>,
) -> Result<(), String> {
    strip_tools(request, &["toolConfig", "tool_config"], |tools| {
        if tools.iter().any(|tool| !tool.is_object()) {
            return Err("each entry in `tools` must be an object".to_string());
        }
        tools.retain_mut(|tool| {
            let Some(fields) = tool.as_object_mut().filter(|fields| !fields.is_empty()) else {
                return true;
            };
            fields.retain(|field, _| {
                let keep = GENERATE_CONTENT_CLIENT_TOOL_FIELDS
                    .contains(&normalize_field_name(field).as_str());
                if !keep {
                    removed.push(field.clone());
                }
                keep
            });
            !fields.is_empty()
        });
        Ok(())
    })
}

/// Filters the `tools` array of `request` with `filter`. When that leaves a
/// non-empty array empty, `tools` is removed together with
/// `tool_choice_fields`, so the request reads as one sent without tools.
fn strip_tools(
    request: &mut Map<String, Value>,
    tool_choice_fields: &[&str],
    filter: impl FnOnce(&mut Vec<Value>) -> Result<(), String>,
) -> Result<(), String> {
    let tools = match request.get_mut("tools") {
        None | Some(Value::Null) => return Ok(()),
        Some(Value::Array(tools)) if tools.is_empty() => return Ok(()),
        Some(Value::Array(tools)) => tools,
        Some(_) => return Err("`tools` must be an array".to_string()),
    };
    filter(tools)?;
    if tools.is_empty() {
        request.remove("tools");
        for field in tool_choice_fields {
            request.remove(*field);
        }
    }
    Ok(())
}

fn tool_label(tool_type: String) -> String {
    if tool_type.is_empty() {
        "<untyped>".to_string()
    } else {
        tool_type
    }
}

/// Vertex accepts both the camelCase and snake_case spelling of proto fields.
fn normalize_field_name(field: &str) -> String {
    field
        .chars()
        .filter(|ch| *ch != '_')
        .map(|ch| ch.to_ascii_lowercase())
        .collect()
}

#[cfg(test)]
mod tests {
    use super::strip_hosted_tools;
    use crate::serve_config::EndpointKind;
    use axum::body::Bytes;
    use serde_json::{Value, json};

    fn strip(endpoint: EndpointKind, request: Value) -> Result<(Value, Vec<String>), String> {
        let body =
            Bytes::from(serde_json::to_vec(&request).expect("request json should serialize"));
        strip_hosted_tools(endpoint, body).map(|(body, removed)| {
            let body = serde_json::from_slice(&body).expect("stripped json should parse");
            (body, removed)
        })
    }

    /// Verifies /responses keeps client-executed tools and removes everything OpenAI executes.
    #[test]
    fn responses_keeps_only_client_executed_tools() {
        let request = json!({
            "model": "gpt-5",
            "input": "hi",
            "tools": [
                {"type": "function", "name": "lookup", "parameters": {"type": "object"}},
                {"type": "custom", "name": "apply_patch"},
                {"type": "local_shell"},
                {"type": "shell", "environment": {"type": "local"}},
                {"type": "shell"},
                {"type": "apply_patch"},
                {"type": "computer_use_preview", "display_width": 1024, "display_height": 768},
                {"type": "tool_search", "execution": "client"},
                {"type": "web_search", "external_web_access": false},
                {"type": "web_search_preview_2025_03_11"},
                {"type": "file_search", "vector_store_ids": ["vs_1"]},
                {"type": "code_interpreter", "container": {"type": "auto"}},
                {"type": "image_generation"},
                {"type": "mcp", "server_label": "docs", "server_url": "https://example.com/mcp"},
                {"type": "tool_search"},
                {"type": "shell", "environment": {"type": "container_auto"}},
                {"type": "some_future_tool"},
                {"name": "untyped"}
            ]
        });

        let (stripped, removed) =
            strip(EndpointKind::Responses, request).expect("request should be accepted");

        assert_eq!(
            stripped["tools"],
            json!([
                {"type": "function", "name": "lookup", "parameters": {"type": "object"}},
                {"type": "custom", "name": "apply_patch"},
                {"type": "local_shell"},
                {"type": "shell", "environment": {"type": "local"}},
                {"type": "shell"},
                {"type": "apply_patch"},
                {"type": "computer_use_preview", "display_width": 1024, "display_height": 768},
                {"type": "tool_search", "execution": "client"}
            ])
        );
        assert_eq!(
            removed,
            [
                "web_search",
                "web_search_preview_2025_03_11",
                "file_search",
                "code_interpreter",
                "image_generation",
                "mcp",
                "tool_search",
                "shell",
                "some_future_tool",
                "<untyped>"
            ]
        );
        assert_eq!(stripped["model"], "gpt-5");
    }

    /// Verifies /responses filters tools inside a namespace and drops a namespace left empty.
    #[test]
    fn responses_filters_namespace_tools() {
        let request = json!({
            "tools": [
                {"type": "namespace", "name": "mcp", "tools": [
                    {"type": "function", "name": "read"},
                    {"type": "web_search"}
                ]},
                {"type": "namespace", "name": "hosted", "tools": [{"type": "file_search"}]}
            ]
        });

        let (stripped, removed) =
            strip(EndpointKind::Responses, request).expect("request should be accepted");

        assert_eq!(
            stripped["tools"],
            json!([{"type": "namespace", "name": "mcp", "tools": [{"type": "function", "name": "read"}]}])
        );
        assert_eq!(removed, ["web_search", "file_search"]);
    }

    /// Verifies /messages keeps custom and Anthropic-defined client tools and removes server tools.
    #[test]
    fn messages_keeps_only_client_executed_tools() {
        let request = json!({
            "model": "claude-sonnet-4-5",
            "max_tokens": 1024,
            "messages": [{"role": "user", "content": "hi"}],
            "mcp_servers": [{"type": "url", "url": "https://example.com/mcp", "name": "docs"}],
            "container": {"skills": [{"type": "anthropic", "skill_id": "pptx"}]},
            "tools": [
                {"name": "lookup", "input_schema": {"type": "object"}},
                {"type": "custom", "name": "custom_lookup", "input_schema": {"type": "object"}},
                {"type": "bash_20250124", "name": "bash"},
                {"type": "text_editor_20250728", "name": "str_replace_based_edit_tool"},
                {"type": "computer_20250124", "name": "computer", "display_width_px": 1024, "display_height_px": 768},
                {"type": "memory_20250818", "name": "memory"},
                {"type": "web_search_20250305", "name": "web_search", "max_uses": 5},
                {"type": "web_fetch_20250910", "name": "web_fetch"},
                {"type": "code_execution_20250825", "name": "code_execution"},
                {"type": "tool_search_tool_regex_20251119", "name": "tool_search_tool_regex"},
                {"type": "mcp_toolset", "mcp_server_name": "docs"},
                {"type": "some_future_tool_20990101", "name": "future"}
            ]
        });

        let (stripped, removed) =
            strip(EndpointKind::Messages, request).expect("request should be accepted");

        assert_eq!(
            stripped["tools"],
            json!([
                {"name": "lookup", "input_schema": {"type": "object"}},
                {"type": "custom", "name": "custom_lookup", "input_schema": {"type": "object"}},
                {"type": "bash_20250124", "name": "bash"},
                {"type": "text_editor_20250728", "name": "str_replace_based_edit_tool"},
                {"type": "computer_20250124", "name": "computer", "display_width_px": 1024, "display_height_px": 768},
                {"type": "memory_20250818", "name": "memory"}
            ])
        );
        assert_eq!(stripped.get("mcp_servers"), None);
        assert_eq!(stripped.get("container"), None);
        assert_eq!(
            removed,
            [
                "mcp_servers",
                "container",
                "web_search_20250305",
                "web_fetch_20250910",
                "code_execution_20250825",
                "tool_search_tool_regex_20251119",
                "mcp_toolset",
                "some_future_tool_20990101"
            ]
        );
    }

    /// Verifies a request whose only tools are hosted reads as one sent without tools.
    #[test]
    fn emptied_tools_are_removed_with_tool_choice() {
        let (stripped, _) = strip(
            EndpointKind::Messages,
            json!({
                "messages": [],
                "tools": [{"type": "web_search_20250305", "name": "web_search"}],
                "tool_choice": {"type": "tool", "name": "web_search"}
            }),
        )
        .expect("request should be accepted");
        assert_eq!(stripped, json!({"messages": []}));

        let (stripped, _) = strip(
            EndpointKind::Responses,
            json!({"input": "hi", "tools": [{"type": "web_search"}], "tool_choice": "required"}),
        )
        .expect("request should be accepted");
        assert_eq!(stripped, json!({"input": "hi"}));

        let (stripped, _) = strip(
            EndpointKind::ModelsGenerateContent,
            json!({
                "contents": [],
                "tools": [{"googleSearch": {}}],
                "toolConfig": {"retrievalConfig": {"latLng": {"latitude": 1, "longitude": 2}}}
            }),
        )
        .expect("request should be accepted");
        assert_eq!(stripped, json!({"contents": []}));
    }

    /// Verifies tools sent empty by the client are forwarded as sent.
    #[test]
    fn empty_tools_from_client_are_kept() {
        let request = json!({"input": "hi", "tools": [], "tool_choice": "auto"});

        let (stripped, removed) =
            strip(EndpointKind::Responses, request.clone()).expect("request should be accepted");

        assert_eq!(stripped, request);
        assert!(removed.is_empty());
    }

    /// Verifies generateContent keeps function declarations and computer use in either spelling.
    #[test]
    fn generate_content_keeps_only_client_executed_tools() {
        let request = json!({
            "contents": [{"role": "user", "parts": [{"text": "hi"}]}],
            "tools": [
                {"functionDeclarations": [{"name": "lookup", "parameters": {"type": "object"}}]},
                {"function_declarations": []},
                {"computerUse": {"environment": "ENVIRONMENT_BROWSER"}},
                {"googleSearch": {}},
                {"google_search": {}},
                {"googleSearchRetrieval": {}},
                {"enterpriseWebSearch": {}},
                {"googleMaps": {}},
                {"retrieval": {"vertexAiSearch": {"datastore": "ds"}}},
                {"codeExecution": {}},
                {"url_context": {}},
                {"someFutureTool": {}}
            ]
        });

        let (stripped, removed) = strip(EndpointKind::ModelsGenerateContent, request)
            .expect("request should be accepted");

        assert_eq!(
            stripped["tools"],
            json!([
                {"functionDeclarations": [{"name": "lookup", "parameters": {"type": "object"}}]},
                {"function_declarations": []},
                {"computerUse": {"environment": "ENVIRONMENT_BROWSER"}}
            ])
        );
        assert_eq!(
            removed,
            [
                "googleSearch",
                "google_search",
                "googleSearchRetrieval",
                "enterpriseWebSearch",
                "googleMaps",
                "retrieval",
                "codeExecution",
                "url_context",
                "someFutureTool"
            ]
        );
    }

    /// Verifies a hosted tool sharing a `Tool` object with function declarations is removed alone.
    #[test]
    fn generate_content_strips_hosted_field_from_mixed_entry() {
        let request = json!({
            "contents": [],
            "tools": [{"functionDeclarations": [], "googleSearch": {}}]
        });

        let (stripped, removed) = strip(EndpointKind::ModelsGenerateContent, request)
            .expect("request should be accepted");

        assert_eq!(stripped["tools"], json!([{"functionDeclarations": []}]));
        assert_eq!(removed, ["googleSearch"]);
    }

    /// Verifies requests without tools are forwarded unchanged.
    #[test]
    fn requests_without_tools_are_unchanged() {
        for endpoint in [
            EndpointKind::Responses,
            EndpointKind::Messages,
            EndpointKind::ModelsGenerateContent,
        ] {
            let request = json!({"model": "m", "input": "hi"});
            assert_eq!(strip(endpoint, request.clone()), Ok((request, Vec::new())));

            let request = json!({"model": "m", "tools": null});
            assert_eq!(strip(endpoint, request.clone()), Ok((request, Vec::new())));
        }
    }

    /// Verifies malformed bodies are rejected instead of forwarded unchecked.
    #[test]
    fn malformed_bodies_are_rejected() {
        for endpoint in [
            EndpointKind::Responses,
            EndpointKind::Messages,
            EndpointKind::ModelsGenerateContent,
        ] {
            assert_eq!(
                strip(endpoint, json!({"tools": {"type": "web_search"}})),
                Err("`tools` must be an array".to_string())
            );
            assert_eq!(
                strip(endpoint, json!([])),
                Err("request body must be a json object".to_string())
            );
            assert!(strip_hosted_tools(endpoint, Bytes::from_static(b"not json")).is_err());
        }
        assert_eq!(
            strip(
                EndpointKind::ModelsGenerateContent,
                json!({"tools": ["googleSearch"]})
            ),
            Err("each entry in `tools` must be an object".to_string())
        );
    }

    /// Verifies a duplicated `tools` key forwards only the value that was checked.
    #[test]
    fn duplicated_tools_key_forwards_checked_value() {
        let body = Bytes::from_static(br#"{"tools":[],"tools":[{"type":"web_search"}]}"#);

        let (stripped, removed) =
            strip_hosted_tools(EndpointKind::Responses, body).expect("request should be accepted");

        let parsed: Value = serde_json::from_slice(&stripped).expect("stripped json should parse");
        assert_eq!(parsed, json!({}));
        assert_eq!(removed, ["web_search"]);
    }

    /// Verifies endpoints without upstream tools forward the body untouched, even if it is not JSON.
    #[test]
    fn other_endpoints_are_untouched() {
        let body = Bytes::from_static(b"not json");

        let (forwarded, removed) = strip_hosted_tools(EndpointKind::Embeddings, body.clone())
            .expect("other endpoints are not checked");

        assert_eq!(forwarded, body);
        assert!(removed.is_empty());
    }
}
