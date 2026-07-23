#[derive(Clone, Debug)]
pub struct SelectionResult {
    pub model_id: String,
}

pub use crate::llm_provider::selection::SelectionError;

pub trait SelectionStrategy<M>: Send + Sync {
    fn select(
        &self,
        endpoint: &str,
        model_id: Option<&str>,
        metadata: &M,
    ) -> Result<SelectionResult, SelectionError>;
}
