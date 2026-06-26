pub mod commands;

use hem_core::{scan_project, ScanOptions, ScanResult};

/// Run the CLI with the given argument list. Returns a process exit code.
pub fn run<I, S>(args: I) -> i32
where
    I: IntoIterator<Item = S>,
    S: AsRef<str>,
{
    commands::run(args)
}

/// Execute a project scan using the same logic as `hem scan`.
pub fn run_scan(options: &ScanOptions) -> ScanResult {
    scan_project(options)
}