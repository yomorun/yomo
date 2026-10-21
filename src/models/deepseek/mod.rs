//! DeepSeek family: profiles per upstream.

use crate::models::{Profile, generic_profile};
use crate::upstreams::Upstream;

const MODELS: &[&str] = &[
    "deepseek-v3.2",
    "deepseek-v4-pro",
    "deepseek-v4-pro-0813",
    "deepseek-v4-flash",
    "deepseek-v4.1-flash",
    "deepseek-v4-flash-vision-exp",
];

pub(super) fn profile(id: &str, upstream: Upstream) -> Option<Profile> {
    if !MODELS.contains(&id) {
        return None;
    }
    match upstream {
        Upstream::Tokenhub | Upstream::VolcengineArk => Some(Profile::chat()),
        other => generic_profile(other),
    }
}
