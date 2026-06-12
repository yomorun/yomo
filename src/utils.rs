use anyhow::{Result, bail};

pub const MAX_LOG_BODY_BYTES: usize = 8 * 1024;
const LOG_HEAD_BYTES: usize = 2 * 1024;

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

#[cfg(test)]
mod tests {
    use super::{
        LOG_HEAD_BYTES, MAX_LOG_BODY_BYTES, sanitize_name, truncate_bytes_for_log, truncate_for_log,
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
}
