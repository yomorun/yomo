use std::path::Path;

use anyhow::{Result, bail};
use serde::Serialize;
use serde_json::Value;
use tokio::fs;

pub const MAX_LOG_BODY_BYTES: usize = 8 * 1024;
const LOG_HEAD_BYTES: usize = 2 * 1024;
const MAX_LOG_JSON_STRING_CHARS: usize = 256;

pub fn sanitize_name(name: &str) -> Result<String> {
    let sanitized = name
        .chars()
        .map(|ch| {
            if ch.is_ascii_alphanumeric() || ch == '-' || ch == '_' {
                ch
            } else {
                '_'
            }
        })
        .collect::<String>();

    if sanitized.is_empty() {
        bail!("name is empty");
    }

    Ok(sanitized)
}

pub fn truncate_for_log(value: &str) -> String {
    if value.len() <= MAX_LOG_BODY_BYTES {
        return value.to_string();
    }

    let mut head_end = LOG_HEAD_BYTES;
    while !value.is_char_boundary(head_end) {
        head_end -= 1;
    }

    let tail_budget = MAX_LOG_BODY_BYTES - head_end;
    let mut tail_start = value.len().saturating_sub(tail_budget);
    while tail_start < value.len() && !value.is_char_boundary(tail_start) {
        tail_start += 1;
    }

    let head = &value[..head_end];
    let tail = &value[tail_start..];
    let truncated_bytes = value
        .len()
        .saturating_sub(head.len().saturating_add(tail.len()));

    format!(
        "{}...[truncated {} bytes]...{}",
        head, truncated_bytes, tail
    )
}

pub fn truncate_bytes_for_log(bytes: &[u8]) -> String {
    let decoded = String::from_utf8_lossy(bytes);
    truncate_for_log(decoded.as_ref())
}

/// Render a JSON body for logging, keeping every field but shortening long string values
/// (message text, base64 payloads) so request parameters stay visible.
pub fn json_body_for_log(bytes: &[u8]) -> String {
    match serde_json::from_slice::<Value>(bytes) {
        Ok(mut value) => {
            shorten_json_strings(&mut value);
            value.to_string()
        }
        Err(_) => truncate_bytes_for_log(bytes),
    }
}

/// Serialize a request for logging with the same string shortening as [`json_body_for_log`].
pub fn json_value_for_log<T: Serialize>(value: &T) -> String {
    match serde_json::to_value(value) {
        Ok(mut value) => {
            shorten_json_strings(&mut value);
            value.to_string()
        }
        Err(err) => format!("<unserializable: {err}>"),
    }
}

fn shorten_json_strings(value: &mut Value) {
    match value {
        Value::String(text) => {
            let total_chars = text.chars().count();
            if total_chars > MAX_LOG_JSON_STRING_CHARS {
                let head = text
                    .chars()
                    .take(MAX_LOG_JSON_STRING_CHARS)
                    .collect::<String>();
                *text = format!(
                    "{}...[{} more chars]",
                    head,
                    total_chars - MAX_LOG_JSON_STRING_CHARS
                );
            }
        }
        Value::Array(items) => items.iter_mut().for_each(shorten_json_strings),
        Value::Object(fields) => fields.values_mut().for_each(shorten_json_strings),
        _ => {}
    }
}

pub async fn copy_if_provided(src: &Path, dst: &Path, default_content: Option<&str>) -> Result<()> {
    if src.exists() {
        fs::copy(src, dst).await?;
    } else if let Some(default_content) = default_content {
        fs::write(dst, default_content).await?;
    }

    Ok(())
}

pub async fn copy_dir_recursive(src: &Path, dst: &Path) -> Result<()> {
    fs::create_dir_all(dst).await?;

    let mut stack = vec![(src.to_path_buf(), dst.to_path_buf())];
    while let Some((current_src, current_dst)) = stack.pop() {
        fs::create_dir_all(&current_dst).await?;
        let mut entries = fs::read_dir(&current_src).await?;
        while let Some(entry) = entries.next_entry().await? {
            let entry_type = entry.file_type().await?;
            let src_path = entry.path();
            let dst_path = current_dst.join(entry.file_name());
            if entry_type.is_dir() {
                stack.push((src_path, dst_path));
            } else if entry_type.is_file() {
                fs::copy(src_path, dst_path).await?;
            }
        }
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use serde_json::{Value, json};

    use super::{
        LOG_HEAD_BYTES, MAX_LOG_BODY_BYTES, MAX_LOG_JSON_STRING_CHARS, json_body_for_log,
        json_value_for_log, sanitize_name, truncate_bytes_for_log, truncate_for_log,
    };

    #[test]
    fn sanitize_name_replaces_invalid_characters() {
        let value = sanitize_name("my/name with*space").expect("sanitized");
        assert_eq!(value, "my_name_with_space");
    }

    #[test]
    fn truncate_for_log_keeps_short_value() {
        let value = "ok";
        assert_eq!(truncate_for_log(value), value);
    }

    #[test]
    fn truncate_for_log_truncates_long_value() {
        let value = "a".repeat(MAX_LOG_BODY_BYTES + 10);
        let truncated = truncate_for_log(&value);

        assert!(truncated.starts_with(&"a".repeat(LOG_HEAD_BYTES)));
        assert!(truncated.contains("...[truncated 10 bytes]..."));
        assert!(truncated.ends_with(&"a".repeat(MAX_LOG_BODY_BYTES - LOG_HEAD_BYTES)));
    }

    #[test]
    fn truncate_for_log_respects_utf8_boundary() {
        let value = "a".repeat(MAX_LOG_BODY_BYTES - 1) + "中";
        let truncated = truncate_for_log(&value);

        assert!(truncated.contains("...[truncated 2 bytes]..."));
        assert!(truncated.ends_with("中"));
    }

    #[test]
    fn truncate_bytes_for_log_handles_non_utf8() {
        let bytes = [0xff, 0xfe, b'a'];
        let truncated = truncate_bytes_for_log(&bytes);

        assert_eq!(truncated, "��a");
    }

    #[test]
    fn json_body_for_log_keeps_fields_and_shortens_long_strings() {
        let long_text = "中".repeat(MAX_LOG_JSON_STRING_CHARS + 5);
        let body = json!({
            "model": "claude-haiku-5-5",
            "max_tokens": 32000,
            "messages": [{"role": "user", "content": long_text}],
        });

        let logged = json_body_for_log(body.to_string().as_bytes());
        let parsed: Value = serde_json::from_str(&logged).expect("valid json");

        assert_eq!(parsed["model"], "claude-haiku-5-5");
        assert_eq!(parsed["max_tokens"], 32000);
        assert_eq!(
            parsed["messages"][0]["content"],
            format!(
                "{}...[5 more chars]",
                "中".repeat(MAX_LOG_JSON_STRING_CHARS)
            )
        );
    }

    #[test]
    fn json_body_for_log_falls_back_for_invalid_json() {
        assert_eq!(json_body_for_log(b"not json"), "not json");
    }

    #[test]
    fn json_value_for_log_shortens_serialized_strings() {
        let request = json!({
            "max_tokens": 4096,
            "system": "a".repeat(MAX_LOG_JSON_STRING_CHARS + 1),
        });

        let logged = json_value_for_log(&request);
        let parsed: Value = serde_json::from_str(&logged).expect("valid json");

        assert_eq!(parsed["max_tokens"], 4096);
        assert!(
            parsed["system"]
                .as_str()
                .expect("string")
                .ends_with("...[1 more chars]")
        );
    }
}
