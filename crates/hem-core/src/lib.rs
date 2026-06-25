//! Hem core engine — Rust port of `packages/core`.

pub mod errors;
pub mod policy;
pub mod types;

pub const ENGINE_ID: &str = "hem-core";

pub use errors::{format_snap_error, SnapError};
pub use policy::{
    capture_status_for_key, ignored_directory, is_secret_like_key, redact_structured_value,
    restore_policy_for, MAX_DIRECTORY_DEPTH, MAX_DIRECTORY_ENTRIES, MAX_FILE_BYTES,
};
pub use types::*;

#[cfg(test)]
mod tests {
    use super::ENGINE_ID;

    #[test]
    fn workspace_smoke_test() {
        assert_eq!(ENGINE_ID, "hem-core");
    }
}