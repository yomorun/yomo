use std::sync::Arc;

use axum::extract::State;
use axum::response::IntoResponse;
use serde::Serialize;

use crate::llm_provider::registry as llm_registry;
use crate::model_api_provider as model_api_registry;

const FIXED_CREATED_AT: i64 = 1_715_367_049;
const FIXED_OWNED_BY: &str = "system";

#[derive(Clone)]
pub struct ModelListHandlerState {
    pub llm_provider_registry: Option<Arc<llm_registry::ProviderRegistry<()>>>,
    pub model_api_provider_registry: Option<Arc<model_api_registry::ProviderRegistry<()>>>,
}

#[derive(Debug, Serialize)]
struct ModelListResponse {
    object: &'static str,
    data: Vec<ModelItem>,
}

#[derive(Debug, Serialize)]
struct ModelItem {
    id: String,
    object: &'static str,
    created: i64,
    owned_by: &'static str,
}

pub async fn handle_list_models(State(state): State<ModelListHandlerState>) -> impl IntoResponse {
    let mut models = Vec::new();
    if let Some(registry) = &state.llm_provider_registry {
        models.extend(registry.model_list());
    }
    if let Some(registry) = &state.model_api_provider_registry {
        models.extend(registry.model_list());
    }

    let mut unique = std::collections::HashMap::new();
    for model in models {
        unique.entry(model.to_ascii_lowercase()).or_insert(model);
    }

    let mut list: Vec<String> = unique.into_values().collect();
    list.sort_by_key(|model| model.to_ascii_lowercase());

    let data = list
        .into_iter()
        .map(|id| ModelItem {
            id,
            object: "model",
            created: FIXED_CREATED_AT,
            owned_by: FIXED_OWNED_BY,
        })
        .collect();
    axum::Json(ModelListResponse {
        object: "list",
        data,
    })
}

pub async fn build_model_list_api(
    llm_provider_registry: Option<llm_registry::ProviderRegistry<()>>,
    model_api_provider_registry: Option<model_api_registry::ProviderRegistry<()>>,
) -> anyhow::Result<axum::Router> {
    let state = ModelListHandlerState {
        llm_provider_registry: llm_provider_registry.map(Arc::new),
        model_api_provider_registry: model_api_provider_registry.map(Arc::new),
    };
    let app = axum::Router::new()
        .route("/models", axum::routing::get(handle_list_models))
        .with_state(state);
    Ok(app)
}
