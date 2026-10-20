//! MiniMax family: profiles per upstream.

use crate::models::{Profile, generic_profile};
use crate::upstreams::Upstream;

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
