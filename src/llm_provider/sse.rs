use std::time::Duration;

use futures_core::Stream;
use futures_util::StreamExt;
use tokio::time::timeout;

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

/// Upper bound for reading the rest of a stream body after its `[DONE]` event.
pub(crate) const BODY_DRAIN_TIMEOUT: Duration = Duration::from_secs(5);

/// Reads `body` to its end, discarding the data, for at most `limit`.
///
/// An HTTP/1.1 connection goes back to the pool only once its response body has
/// been read to the end. Upstreams send the terminating chunk after `[DONE]`,
/// often in a later packet, so dropping the body at `[DONE]` closes the
/// connection and costs the next request a new TCP and TLS handshake. Callers
/// spawn this so the wait stays out of their own stream. Returns `true` when
/// the body reached its end.
pub(crate) async fn drain_body<S, B, E>(mut body: S, limit: Duration) -> bool
where
    S: Stream<Item = Result<B, E>> + Unpin,
{
    let drained = timeout(limit, async {
        while let Some(chunk) = body.next().await {
            if chunk.is_err() {
                return false;
            }
        }
        true
    })
    .await;
    drained.unwrap_or(false)
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

    #[tokio::test]
    async fn drain_body_reads_finite_body_to_end() {
        let body = futures_util::stream::iter(vec![Ok::<_, ()>(b"a".to_vec()), Ok(b"b".to_vec())]);
        assert!(drain_body(body, Duration::from_secs(1)).await);
    }

    #[tokio::test]
    async fn drain_body_stops_at_body_error() {
        let body = futures_util::stream::iter(vec![Ok(b"a".to_vec()), Err(())]);
        assert!(!drain_body(body, Duration::from_secs(1)).await);
    }

    #[tokio::test]
    async fn drain_body_gives_up_after_limit() {
        let body = futures_util::stream::pending::<Result<Vec<u8>, ()>>();
        assert!(!drain_body(body, Duration::from_millis(10)).await);
    }
}
