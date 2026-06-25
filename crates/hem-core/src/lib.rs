//! Hem core engine — Rust port of `packages/core`.

pub const ENGINE_ID: &str = "hem-core";

#[cfg(test)]
mod tests {
    use super::ENGINE_ID;

    #[test]
    fn workspace_smoke_test() {
        assert_eq!(ENGINE_ID, "hem-core");
    }
}