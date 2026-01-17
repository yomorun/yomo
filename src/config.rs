use serde::Deserialize;

use crate::zipper::config::ZipperConfig;

#[derive(Debug, Clone, Deserialize, Default)]
pub struct ServeConfig<T> {
    #[serde(default)]
    pub zipper: ZipperConfig<T>,
}
