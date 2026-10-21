//! MiniMax family: profiles per upstream and per-model request rules.

use crate::models::{Ctx, Profile, generic_profile};
use crate::provider::{ProviderError, UnifiedRequest};
use crate::upstreams::Upstream;

mod minimax_m2_7;
mod minimax_m3;

const MODELS: &[&str] = &["minimax-m2.7", "minimax-m3"];

pub(super) fn profile(id: &str, upstream: Upstream) -> Option<Profile> {
    if !MODELS.contains(&id) {
        return None;
    }
    match upstream {
        Upstream::Tokenhub => Some(Profile::chat()),
        other => generic_profile(other),
    }
}

/// MiniMax request adaptation; only models with quirks have their own rules.
pub(super) fn adapt_request(
    id: &str,
    ctx: &Ctx,
    request: &mut UnifiedRequest,
) -> Result<(), ProviderError> {
    match id {
        "minimax-m2.7" => minimax_m2_7::adapt_request(ctx, request),
        "minimax-m3" => minimax_m3::adapt_request(ctx, request),
        _ => Ok(()),
    }
}
