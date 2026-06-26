use std::path::{Component, Path, PathBuf};

/// Roots that writable restore/bundle paths must stay within.
#[derive(Debug, Clone)]
pub struct ConfinementRoots {
    pub home_dir: PathBuf,
    pub project_path: PathBuf,
}

const BLOCKED_HOME_PREFIXES: &[&str] = &[
    ".ssh",
    ".aws",
    ".gnupg",
    ".config",
    ".local",
    ".npm",
    ".docker",
    ".kube",
    ".credentials",
    ".heroku",
    ".netrc",
    ".env",
    ".gitconfig",
    ".git-credentials",
    ".npmrc",
    ".bash_profile",
    ".bashrc",
    ".zshrc",
    ".profile",
    ".pgpass",
    ".gem",
];

pub fn path_has_traversal(path: &Path) -> bool {
    path.components()
        .any(|component| matches!(component, Component::ParentDir))
}

pub fn is_strictly_under(resolved: &Path, root: &Path) -> bool {
    let root_str = root.to_string_lossy();
    let resolved_str = resolved.to_string_lossy();
    resolved_str == root_str
        || resolved_str.starts_with(&format!(
            "{root_str}{}",
            std::path::MAIN_SEPARATOR
        ))
}

/// Reject traversal, out-of-root absolute paths, and blocked home prefixes.
pub fn validate_constrained_write_path(
    dest: &Path,
    roots: &ConfinementRoots,
) -> Result<PathBuf, String> {
    if dest.as_os_str().is_empty() {
        return Err("Empty destination path".to_string());
    }
    if path_has_traversal(dest) {
        return Err(format!(
            "Path traversal detected: \"{}\" contains \"..\"",
            dest.display()
        ));
    }
    if !dest.is_absolute() {
        return Err(format!(
            "Destination must be absolute for confinement check: {}",
            dest.display()
        ));
    }

    let resolved = dest.to_path_buf();
    if !is_strictly_under(&resolved, &roots.home_dir)
        && !is_strictly_under(&resolved, &roots.project_path)
    {
        return Err(format!(
            "Path \"{}\" resolves outside home and project directories",
            resolved.display()
        ));
    }

    if is_strictly_under(&resolved, &roots.home_dir) {
        if let Ok(relative) = resolved.strip_prefix(&roots.home_dir) {
            let rel = relative.to_string_lossy();
            for prefix in BLOCKED_HOME_PREFIXES {
                if rel.starts_with(prefix)
                    || rel.starts_with(&format!("{prefix}{}", std::path::MAIN_SEPARATOR))
                    || rel.contains(&format!(
                        "{}{prefix}{}",
                        std::path::MAIN_SEPARATOR,
                        std::path::MAIN_SEPARATOR
                    ))
                {
                    return Err(format!("Blocked content path prefix: \"{rel}\""));
                }
            }
        }
    }

    Ok(resolved)
}

/// Validate a home-relative import path segment before joining to home_dir.
pub fn validate_home_relative_import_segment(segment: &str) -> Result<(), String> {
    if segment.is_empty() {
        return Err("Empty home-relative path segment".to_string());
    }
    if segment.contains("..") {
        return Err(format!(
            "Path traversal detected: \"{segment}\" contains \"..\""
        ));
    }
    if Path::new(segment).is_absolute() {
        return Err(format!(
            "Path traversal detected: \"{segment}\" is absolute"
        ));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rejects_traversal_in_dest() {
        let roots = ConfinementRoots {
            home_dir: PathBuf::from("/home/user"),
            project_path: PathBuf::from("/home/user/project"),
        };
        let err = validate_constrained_write_path(
            Path::new("/home/user/../../etc/passwd"),
            &roots,
        )
        .expect_err("traversal");
        assert!(err.contains("traversal"));
    }

    #[test]
    fn allows_paths_under_home() {
        let roots = ConfinementRoots {
            home_dir: PathBuf::from("/home/user"),
            project_path: PathBuf::from("/home/user/project"),
        };
        let ok = validate_constrained_write_path(
            Path::new("/home/user/.codex/config.toml"),
            &roots,
        )
        .expect("allowed");
        assert_eq!(ok, PathBuf::from("/home/user/.codex/config.toml"));
    }
}