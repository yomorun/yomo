//! Kimi family: profiles per upstream and per-model request rules.

use crate::models::{Ctx, Profile, generic_profile};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::Upstream;

mod kimi_k2_6;
mod kimi_k3;

const MODELS: &[&str] = &["kimi-k2.6", "kimi-k2.8-preview", "kimi-k3"];

pub(super) fn profile(id: &str, upstream: Upstream) -> Option<Profile> {
    if !MODELS.contains(&id) {
        return None;
    }
    match upstream {
        Upstream::Tokenhub => Some(Profile::chat()),
        other => generic_profile(other),
    }
}

/// Kimi request adaptation; only models with quirks have their own rules.
pub(super) fn adapt_request(
    id: &str,
    ctx: &Ctx,
    request: &mut UnifiedRequest,
) -> Result<(), ProviderError> {
    match id {
        "kimi-k2.6" => kimi_k2_6::adapt_request(ctx, request),
        "kimi-k3" | "kimi-k2.8-preview" => kimi_k3::adapt_request(ctx, request),
        _ => Ok(()),
    }
}
