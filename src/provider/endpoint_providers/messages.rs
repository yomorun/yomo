use async_trait::async_trait;
use axum::http::{HeaderMap, HeaderValue, header};
use serde_json::Value;
use std::sync::Arc;

use crate::provider::{
    HttpProviderRequest, HttpProviderResponse, Provider, ProviderError,
    extract_messages_request_id_json, extract_messages_usage_json, inject_messages_usage_json,
    proxy_request,
};
use crate::serve_config::{ConfigError, ProviderConfig};

#[derive(Clone)]
pub struct MessagesClient {
    client: reqwest::Client,
    base_url: String,
    auth_headers: HeaderMap,
    upstream_model: String,
    anthropic_version: String,
}

impl MessagesClient {
    pub fn new(
        client: reqwest::Client,
        base_url: String,
        auth_headers: HeaderMap,
        upstream_model: String,
        anthropic_version: String,
    ) -> Self {
        Self {
            client,
            base_url,
            auth_headers,
            upstream_model,
            anthropic_version,
        }
    }
}

#[async_trait]
impl<M: Sync> Provider<M> for MessagesClient {
    async fn http(
        &self,
        mut req: HttpProviderRequest,
        _metadata: &M,
    ) -> Result<HttpProviderResponse, ProviderError> {
        req.endpoint_path = "/messages".to_string();
        req.headers.insert(
            "anthropic-version",
            self.anthropic_version
                .parse::<HeaderValue>()
                .map_err(|err| anyhow::anyhow!("invalid anthropic-version header: {err}"))?,
        );
        proxy_request(
            &self.client,
            &self.base_url,
            self.auth_headers.clone(),
            Some(self.upstream_model.as_str()),
            req,
        )
        .await
    }

    fn extract_request_id(&self, payload_json: &Value) -> Option<String> {
        extract_messages_request_id_json(payload_json)
    }

    fn extract_usage(&self, payload_json: &Value) -> Option<Value> {
        extract_messages_usage_json(payload_json)
    }

    fn inject_usage(&self, payload_json: &mut Value, usage: Value) -> bool {
        inject_messages_usage_json(payload_json, usage)
    }
}

#[cfg(test)]
mod tests {
    use super::build_client;
    use crate::provider::{
        extract_messages_request_id_json, extract_messages_usage_json, inject_messages_usage_json,
    };
    use crate::serve_config::ProviderConfig;
    use serde_json::json;
    use std::collections::HashMap;

    #[test]
    fn extract_request_id_json_prefers_top_level_id() {
        let payload = json!({"id": "msg_top", "message": {"id": "msg_nested"}});

        let request_id = extract_messages_request_id_json(&payload);

        assert_eq!(request_id.as_deref(), Some("msg_top"));
    }

    #[test]
    fn extract_request_id_json_supports_nested_message_id() {
        let payload = json!({"message": {"id": "msg_nested"}});

        let request_id = extract_messages_request_id_json(&payload);

        assert_eq!(request_id.as_deref(), Some("msg_nested"));
    }

    #[test]
    fn extract_usage_json_reads_nested_message_usage() {
        let payload = json!({"message": {"usage": {"input_tokens": 3}}});

        let usage = extract_messages_usage_json(&payload);

        assert_eq!(usage, Some(json!({"input_tokens": 3})));
    }

    #[test]
    fn extract_usage_json_ignores_null_usage() {
        let payload = json!({"usage": null});

        let usage = extract_messages_usage_json(&payload);

        assert_eq!(usage, None);
    }

    #[test]
    fn inject_usage_json_updates_nested_message_usage() {
        let mut payload = json!({"message": {"usage": {"input_tokens": 1}}});
        let new_usage = json!({"input_tokens": 55});

        let injected = inject_messages_usage_json(&mut payload, new_usage.clone());

        assert!(injected);
        assert_eq!(payload["message"]["usage"], new_usage);
    }

    #[test]
    fn build_client_accepts_x_api_key_auth_style() {
        let mut params = HashMap::new();
        params.insert("api_key".to_string(), "sk-ant-test".to_string());
        params.insert(
            "base_url".to_string(),
            "https://api.anthropic.com/v1".to_string(),
        );
        params.insert("model".to_string(), "claude-sonnet-4-20250514".to_string());

        let provider = ProviderConfig {
            provider_type: "messages".to_string(),
            model_id: "claude-sonnet-4".to_string(),
            label: None,
            params,
        };

        assert!(build_client::<()>(&provider).is_ok());
    }

    #[test]
    fn build_client_accepts_bearer_auth_style() {
        let mut params = HashMap::new();
        params.insert("api_key".to_string(), "sk-test".to_string());
        params.insert(
            "base_url".to_string(),
            "https://proxy.example.com/v1".to_string(),
        );
        params.insert("model".to_string(), "claude-sonnet-4-20250514".to_string());
        params.insert("auth_style".to_string(), "bearer".to_string());

        let provider = ProviderConfig {
            provider_type: "messages".to_string(),
            model_id: "claude-sonnet-4".to_string(),
            label: None,
            params,
        };

        assert!(build_client::<()>(&provider).is_ok());
    }

    #[test]
    fn build_client_rejects_unknown_auth_style() {
        let mut params = HashMap::new();
        params.insert("api_key".to_string(), "sk-test".to_string());
        params.insert(
            "base_url".to_string(),
            "https://proxy.example.com/v1".to_string(),
        );
        params.insert("model".to_string(), "claude-sonnet-4-20250514".to_string());
        params.insert("auth_style".to_string(), "unknown".to_string());

        let provider = ProviderConfig {
            provider_type: "messages".to_string(),
            model_id: "claude-sonnet-4".to_string(),
            label: None,
            params,
        };

        let err = build_client::<()>(&provider)
            .err()
            .expect("unknown auth_style must be rejected");

        assert_eq!(
            err.to_string(),
            "invalid provider: unknown auth_style: unknown"
        );
    }

    #[test]
    fn build_client_rejects_missing_api_key() {
        let mut params = HashMap::new();
        params.insert(
            "base_url".to_string(),
            "https://api.anthropic.com/v1".to_string(),
        );
        params.insert("model".to_string(), "claude-sonnet-4-20250514".to_string());

        let provider = ProviderConfig {
            provider_type: "messages".to_string(),
            model_id: "claude-sonnet-4".to_string(),
            label: None,
            params,
        };

        let err = build_client::<()>(&provider)
            .err()
            .expect("missing api_key must be rejected");

        assert_eq!(err.to_string(), "invalid provider: api_key is required");
    }

    #[test]
    fn build_client_rejects_missing_base_url() {
        let mut params = HashMap::new();
        params.insert("api_key".to_string(), "sk-ant-test".to_string());
        params.insert("model".to_string(), "claude-sonnet-4-20250514".to_string());

        let provider = ProviderConfig {
            provider_type: "messages".to_string(),
            model_id: "claude-sonnet-4".to_string(),
            label: None,
            params,
        };

        let err = build_client::<()>(&provider)
            .err()
            .expect("missing base_url must be rejected");

        assert_eq!(err.to_string(), "invalid provider: base_url is required");
    }

    #[test]
    fn build_client_rejects_missing_model() {
        let mut params = HashMap::new();
        params.insert("api_key".to_string(), "sk-ant-test".to_string());
        params.insert(
            "base_url".to_string(),
            "https://api.anthropic.com/v1".to_string(),
        );

        let provider = ProviderConfig {
            provider_type: "messages".to_string(),
            model_id: "claude-sonnet-4".to_string(),
            label: None,
            params,
        };

        let err = build_client::<()>(&provider)
            .err()
            .expect("missing model must be rejected");

        assert_eq!(err.to_string(), "invalid provider: model is required");
    }
}

pub fn build_client<M: Sync>(
    provider: &ProviderConfig,
) -> Result<Arc<dyn Provider<M>>, ConfigError> {
    let api_key = provider
        .params
        .get("api_key")
        .ok_or_else(|| ConfigError::InvalidProvider("api_key is required".to_string()))?;
    let base_url = provider
        .params
        .get("base_url")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("base_url is required".to_string()))?;
    let upstream_model = provider
        .params
        .get("model")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("model is required".to_string()))?;
    let anthropic_version = provider
        .params
        .get("anthropic_version")
        .cloned()
        .unwrap_or_else(|| "2023-06-01".to_string());
    let auth_style = provider
        .params
        .get("auth_style")
        .cloned()
        .unwrap_or_else(|| "x-api-key".to_string());

    let mut headers = HeaderMap::new();
    match auth_style.as_str() {
        "x-api-key" => {
            headers.insert(
                "x-api-key",
                api_key
                    .parse::<HeaderValue>()
                    .map_err(|err| ConfigError::InvalidProvider(err.to_string()))?,
            );
        }
        "bearer" => {
            let auth_value = format!("Bearer {}", api_key);
            headers.insert(
                header::AUTHORIZATION,
                auth_value
                    .parse::<HeaderValue>()
                    .map_err(|err| ConfigError::InvalidProvider(err.to_string()))?,
            );
        }
        other => {
            return Err(ConfigError::InvalidProvider(format!(
                "unknown auth_style: {}",
                other
            )));
        }
    }

    Ok(Arc::new(MessagesClient::new(
        reqwest::Client::new(),
        base_url,
        headers,
        upstream_model,
        anthropic_version,
    )))
}
