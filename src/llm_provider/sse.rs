/// Splits an SSE byte stream into `data:` payload strings.
///
/// The reader buffers raw bytes because upstream chunk boundaries can split a
/// multi-byte UTF-8 character, so only complete lines are decoded.
#[derive(Debug, Default)]
pub(crate) struct SseDataReader {
    buffer: Vec<u8>,
}

impl SseDataReader {
    /// Feeds a chunk and returns the payloads of all complete lines it holds.
    pub(crate) fn push(&mut self, chunk: &[u8]) -> Vec<String> {
        self.buffer.extend_from_slice(chunk);
        let mut payloads = Vec::new();
        while let Some(position) = self.buffer.iter().position(|byte| *byte == b'\n') {
            let line = self.buffer.drain(..=position).collect::<Vec<u8>>();
            if let Some(payload) = parse_sse_data_line(&line) {
                payloads.push(payload);
            }
        }
        payloads
    }
}

/// Extracts the payload of an SSE `data:` line.
fn parse_sse_data_line(line: &[u8]) -> Option<String> {
    let text = String::from_utf8_lossy(line);
    let trimmed = text.trim();
    let payload = trimmed.strip_prefix("data:")?;
    Some(payload.strip_prefix(' ').unwrap_or(payload).to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn sse_reader_keeps_multibyte_characters_split_across_chunks() {
        let mut reader = SseDataReader::default();
        let payload = "data: {\"text\":\"你好\"}\n";

        let mut payloads = reader.push(&payload.as_bytes()[..12]);
        payloads.extend(reader.push(&payload.as_bytes()[12..]));

        assert_eq!(payloads, vec!["{\"text\":\"你好\"}".to_string()]);
    }

    #[test]
    fn sse_reader_keeps_multibyte_characters_fed_one_byte_at_a_time() {
        let mut reader = SseDataReader::default();
        let stream = "data: {\"text\":\"中文回答\"}\n\ndata: {\"text\":\"🙂👍\"}\n\n";

        let payloads = stream
            .as_bytes()
            .iter()
            .flat_map(|byte| reader.push(std::slice::from_ref(byte)))
            .collect::<Vec<_>>();

        assert_eq!(
            payloads,
            vec![
                "{\"text\":\"中文回答\"}".to_string(),
                "{\"text\":\"🙂👍\"}".to_string(),
            ]
        );
    }

    #[test]
    fn sse_reader_ignores_non_data_lines() {
        let mut reader = SseDataReader::default();

        let payloads = reader.push(b"event: message\ndata: {\"a\":1}\n\n");

        assert_eq!(payloads, vec!["{\"a\":1}".to_string()]);
    }
}
