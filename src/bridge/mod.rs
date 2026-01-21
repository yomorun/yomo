pub mod http;

use anyhow::Result;
use tokio::io::{ReadHalf, SimplexStream, WriteHalf};

use crate::metadata::Metadata;

#[async_trait::async_trait]
pub trait Bridge: Send + Sync {
    async fn forward(
        &self,
        sfn_name: &str,
        metadata: &Box<dyn Metadata>,
        reader: ReadHalf<SimplexStream>,
        writer: WriteHalf<SimplexStream>,
    ) -> Result<bool>;
}
