#[derive(Clone, Debug)]
pub struct SelectionResult {
    pub model_id: String,
}

#[derive(Debug)]
pub enum SelectionError {
    ModelNotSupported,
}

pub trait SelectionStrategy<M>: Send + Sync {
    fn select(
        &self,
        endpoint: &str,
        model_id: Option<&str>,
        metadata: &M,
    ) -> Result<SelectionResult, SelectionError>;
}
