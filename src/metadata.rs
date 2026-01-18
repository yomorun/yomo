pub trait Metadata: Sync + Send {
    fn trace_id(&self) -> &str;

    fn req_id(&self) -> &str;
}
