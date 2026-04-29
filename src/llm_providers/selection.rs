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
        model_id: Option<&str>,
        metadata: &M,
    ) -> Result<SelectionResult, SelectionError>;
}

#[derive(Clone, Default)]
pub struct ByModel;

impl<M> SelectionStrategy<M> for ByModel {
    fn select(
        &self,
        model_id: Option<&str>,
        _metadata: &M,
    ) -> Result<SelectionResult, SelectionError> {
        match model_id {
            Some(model_id) if !model_id.trim().is_empty() => Ok(SelectionResult {
                model_id: model_id.to_string(),
            }),
            _ => Err(SelectionError::ModelNotSupported),
        }
    }
}
