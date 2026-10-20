//! GLM family: profiles per upstream and per-model request rules.

use crate::models::{Ctx, Profile, generic_profile};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::Upstream;

mod glm_5_3_flash;

const MODELS: &[&str] = &["glm-5.1", "glm-5.2", "glm-5.3", "glm-5.3-flash"];

/// GLM profile per upstream. Models without quirks share the plain Chat profile.
pub(super) fn profile(id: &str, upstream: Upstream) -> Option<Profile> {
    if !MODELS.contains(&id) {
        return None;
    }
    match (id, upstream) {
        ("glm-5.3-flash", Upstream::Tokenhub) => Some(glm_5_3_flash::tokenhub_profile()),
        (_, Upstream::Tokenhub | Upstream::VolcengineArk) => Some(Profile::chat()),
        (_, other) => generic_profile(other),
    }
}

/// GLM request adaptation; only models with quirks have their own rules.
pub(super) fn adapt_request(
    id: &str,
    ctx: &Ctx,
    request: &mut UnifiedRequest,
) -> Result<(), ProviderError> {
    match id {
        "glm-5.3-flash" => glm_5_3_flash::adapt_request(ctx, request),
        _ => Ok(()),
    }
}
