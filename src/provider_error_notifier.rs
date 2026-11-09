use serde::{Deserialize, Serialize};

/// Unified provider error payload passed to notifiers.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct ProviderErrorEvent<M> {
    pub model: Option<String>,
    pub metadata: M,
    pub http_status: Option<u16>,
    pub error: String,
    pub endpoint: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub origin: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub upstream_class: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub upstream_code: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub upstream_request_id: Option<String>,
}

/// Handles provider errors from both LLM and model API registries.
pub trait ProviderErrorNotifier<M>: Send + Sync {
    /// Receives provider error events for side-effect notifications.
    fn notify_provider_error(&self, event: ProviderErrorEvent<M>);
}

/// Default no-op provider error notifier.
#[derive(Debug, Default)]
pub struct NoopProviderErrorNotifier;

impl<M> ProviderErrorNotifier<M> for NoopProviderErrorNotifier {
    fn notify_provider_error(&self, _event: ProviderErrorEvent<M>) {}
}
