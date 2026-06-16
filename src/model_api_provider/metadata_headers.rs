use axum::http::HeaderMap;

pub trait MetadataHeaderMapper<M>: Send + Sync {
    fn headers_for(&self, metadata: &M) -> HeaderMap;
}

pub struct NoopMetadataHeaderMapper;

impl<M> MetadataHeaderMapper<M> for NoopMetadataHeaderMapper {
    fn headers_for(&self, _metadata: &M) -> HeaderMap {
        HeaderMap::new()
    }
}
