//! Provider dialects, with one file per kind.
//!
//! `build_client` dispatch: serde-validated `ProviderKindParams` -> concrete client instance.
//! Startup fails fast when credentials are missing or invalid.
//! Red line: no code outside a dialect touches native bytes.

pub mod bedrock_converse;
pub mod credential;
pub mod google_vertex;
pub mod openai_chat_completions;
pub mod openai_responses;
pub mod passthrough;

pub use credential::{Credential, GcpTokenCache, TokenFetcher};

use crate::dialects::bedrock_converse::BedrockConverseProvider;
use crate::dialects::google_vertex::GoogleVertexProvider;
use crate::dialects::openai_chat_completions::OpenAiChatCompletionsProvider;
use crate::dialects::openai_responses::OpenAiResponsesProvider;
use crate::provider::Provider;
use crate::serve_config::{ProviderConfig, ProviderKind, ProviderKindParams};
use secrecy::ExposeSecret;
use std::sync::Arc;

/// Dispatch dialect client construction.
///
/// Returns `Arc<dyn Provider<M>>`; params are already serde-validated, so this only
/// performs fail-fast validation and assembly.
pub fn build_client<M: Send + Sync + 'static>(
    _kind: ProviderKind,
    params: &ProviderKindParams,
) -> Result<Arc<dyn Provider<M>>, String> {
    // fail-fast: only kinds requiring mandatory credentials are rejected here.
    if let Some(message) = validate_credentials(params) {
        return Err(message);
    }

    match params {
        ProviderKindParams::OpenAiChatCompletions(p) => {
            Ok(Arc::new(OpenAiChatCompletionsProvider::new(p)))
        }
        ProviderKindParams::OpenAiResponses(p) => Ok(Arc::new(OpenAiResponsesProvider::new(p))),
        ProviderKindParams::BedrockConverse(p) => Ok(Arc::new(BedrockConverseProvider::new(p))),
        ProviderKindParams::GoogleVertex(p) => Ok(Arc::new(GoogleVertexProvider::new(p))),
        ProviderKindParams::Passthrough(_) => {
            Err(crate::dialects::passthrough::PASSTHROUGH_NOT_A_CHAT_KIND.to_string())
        }
    }
}

/// Build an explicitly resolved integration without URL-based vendor inference.
pub fn build_bound_client<M: Send + Sync + 'static>(
    entry: &ProviderConfig,
) -> Result<Arc<dyn Provider<M>>, String> {
    if let Some(message) = validate_credentials(&entry.params) {
        return Err(message);
    }
    match (&entry.params, &entry.binding) {
        (ProviderKindParams::OpenAiChatCompletions(params), Some(binding)) => Ok(Arc::new(
            OpenAiChatCompletionsProvider::new_bound(params, binding.clone()),
        )),
        (ProviderKindParams::OpenAiResponses(params), Some(binding)) => Ok(Arc::new(
            OpenAiResponsesProvider::new_bound(params, binding.clone()),
        )),
        _ => build_client(entry.kind, &entry.params),
    }
}

/// Common credential fail-fast validation.
/// Kind-specific checks (credentials file existence, region validity) live in each dialect's build_client.
fn validate_credentials(params: &ProviderKindParams) -> Option<String> {
    let empty_key = |label: &str, key: &secrecy::SecretString| {
        key.expose_secret()
            .trim()
            .is_empty()
            .then(|| format!("credential validation failed: {} api_key is empty", label))
    };

    match params {
        ProviderKindParams::OpenAiChatCompletions(_) => None,
        ProviderKindParams::OpenAiResponses(_) => None,
        ProviderKindParams::BedrockConverse(p) => p
            .aws_bearer_token
            .expose_secret()
            .trim()
            .is_empty()
            .then(|| "credential validation failed: bedrock aws_bearer_token is empty".to_string()),
        ProviderKindParams::GoogleVertex(p) => {
            // The credentials file must exist before startup.
            if !p.credentials_file.is_file() {
                return Some(format!(
                    "credential validation failed: google-vertex credentials file not found: {}",
                    p.credentials_file.display()
                ));
            }
            None
        }
        ProviderKindParams::Passthrough(p) => p
            .auth
            .as_ref()
            .and_then(|auth| empty_key("passthrough", &auth.api_key)),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::serve_config::{
        AuthConfig, AuthScheme, BedrockParams, GoogleVertexParams, OpenAiParams, PassthroughParams,
    };
    use secrecy::SecretString;
    use std::path::PathBuf;

    fn openai_params() -> OpenAiParams {
        OpenAiParams {
            base_url: "https://x.example.com/v1".parse().expect("url"),
            auth: Some(AuthConfig {
                scheme: AuthScheme::Bearer,
                api_key: SecretString::from("sk-test"),
            }),
            model: "m".to_string(),
            response_model: None,
        }
    }

    #[test]
    fn build_client_allows_missing_auth_for_openai_chat_completions() {
        let mut params = openai_params();
        params.auth = None;
        let provider = build_client::<()>(
            ProviderKind::OpenAiChatCompletions,
            &ProviderKindParams::OpenAiChatCompletions(params),
        )
        .expect("openai-chat-completions should allow missing auth");
        assert_eq!(provider.model_id(), "m");
    }

    #[test]
    fn build_client_allows_missing_auth_for_openai_responses() {
        let mut params = openai_params();
        params.auth = None;
        let provider = build_client::<()>(
            ProviderKind::OpenAiResponses,
            &ProviderKindParams::OpenAiResponses(params),
        )
        .expect("openai-responses should allow missing auth");
        assert_eq!(provider.model_id(), "m");
    }

    #[test]
    fn build_client_rejects_missing_google_credentials_file() {
        let params = GoogleVertexParams {
            model: "gemini".to_string(),
            project_id: "pc".to_string(),
            location: "global".to_string(),
            credentials_file: PathBuf::from("/nonexistent/sa.json"),
        };
        let err = match build_client::<()>(
            ProviderKind::GoogleVertex,
            &ProviderKindParams::GoogleVertex(params),
        ) {
            Ok(_) => panic!("missing credentials file must be rejected"),
            Err(err) => err,
        };
        assert!(err.contains("credentials file not found"));
    }

    #[test]
    fn build_client_rejects_empty_bedrock_bearer_token() {
        let params = BedrockParams {
            model: "global.anthropic.claude-opus-5".to_string(),
            aws_region: "ap-northeast-3".to_string(),
            aws_bearer_token: SecretString::from(""),
        };
        let err = match build_client::<()>(
            ProviderKind::BedrockConverse,
            &ProviderKindParams::BedrockConverse(params),
        ) {
            Ok(_) => panic!("empty bearer token must be rejected"),
            Err(err) => err,
        };
        assert!(err.contains("aws_bearer_token is empty"));
    }

    #[test]
    fn passthrough_is_never_a_chat_kind() {
        let params = PassthroughParams {
            base_url: Some("http://127.0.0.1:8001/v1".parse().expect("url")),
            auth: None,
            google_vertex: None,
            model: "bge".to_string(),
        };
        // passthrough never serves chat endpoints.
        let err = match build_client::<()>(
            ProviderKind::Passthrough,
            &ProviderKindParams::Passthrough(params),
        ) {
            Ok(_) => panic!("passthrough must not build a chat provider"),
            Err(err) => err,
        };
        assert!(err.contains("does not serve chat endpoints"));
    }
}
