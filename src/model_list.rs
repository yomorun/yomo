use std::sync::Arc;

use axum::extract::State;
use axum::response::IntoResponse;
use serde::Serialize;

use crate::provider_registry;

const FIXED_CREATED_AT: i64 = 1_715_367_049;
const FIXED_OWNED_BY: &str = "system";

#[derive(Clone)]
pub struct ModelListHandlerState {
    pub provider_registry: Arc<provider_registry::ProviderRegistry<()>>,
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
    models.extend(state.provider_registry.model_list());

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
    provider_registry: provider_registry::ProviderRegistry<()>,
) -> anyhow::Result<axum::Router> {
    let state = ModelListHandlerState {
        provider_registry: Arc::new(provider_registry),
    };
    let app = axum::Router::new()
        .route("/models", axum::routing::get(handle_list_models))
        .with_state(state);
    Ok(app)
}
