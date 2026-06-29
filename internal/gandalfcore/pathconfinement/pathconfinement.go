package pathconfinement

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Roots struct {
	HomeDir     string
	ProjectPath string
}

var blockedHomePrefixes = []string{
	".ssh", ".aws", ".gnupg", ".config", ".local", ".npm", ".docker", ".kube",
	".credentials", ".heroku", ".netrc", ".env", ".gitconfig", ".git-credentials",
	".npmrc", ".bash_profile", ".bashrc", ".zshrc", ".profile", ".pgpass", ".gem",
}

func RootsFromPaths(homeDir, projectPath *string) *Roots {
	switch {
	case homeDir == nil && projectPath == nil:
		return nil
	case homeDir != nil && projectPath != nil:
		return &Roots{HomeDir: filepath.Clean(*homeDir), ProjectPath: filepath.Clean(*projectPath)}
	case homeDir != nil:
		cleanHome := filepath.Clean(*homeDir)
		return &Roots{HomeDir: cleanHome, ProjectPath: cleanHome}
	default:
		cleanProject := filepath.Clean(*projectPath)
		return &Roots{HomeDir: cleanProject, ProjectPath: cleanProject}
	}
}

func PathHasTraversal(path string) bool {
	clean := filepath.Clean(path)
	parts := strings.Split(clean, string(os.PathSeparator))
	for _, part := range parts {
		if part == ".." {
			return true
		}
	}
	return strings.Contains(path, "..")
}

func IsStrictlyUnder(resolved, root string) bool {
	if resolved == root {
		return true
	}
	prefix := root + string(os.PathSeparator)
	return strings.HasPrefix(resolved, prefix)
}

func ValidateConstrainedWritePath(dest string, roots *Roots) (string, error) {
	if dest == "" {
		return "", fmt.Errorf("empty destination path")
	}
	if PathHasTraversal(dest) {
		return "", fmt.Errorf(`path traversal detected: "%s" contains ".."`, dest)
	}
	if !filepath.IsAbs(dest) {
		return "", fmt.Errorf("destination must be absolute for confinement check: %s", dest)
	}

	resolved := filepath.Clean(dest)
	if !IsStrictlyUnder(resolved, roots.HomeDir) && !IsStrictlyUnder(resolved, roots.ProjectPath) {
		return "", fmt.Errorf(`path "%s" resolves outside home and project directories`, resolved)
	}

	if IsStrictlyUnder(resolved, roots.HomeDir) {
		rel, err := filepath.Rel(roots.HomeDir, resolved)
		if err == nil {
			for _, prefix := range blockedHomePrefixes {
				if rel == prefix || strings.HasPrefix(rel, prefix+string(os.PathSeparator)) ||
					strings.Contains(rel, string(os.PathSeparator)+prefix+string(os.PathSeparator)) {
					return "", fmt.Errorf(`blocked content path prefix: "%s"`, rel)
				}
			}
		}
	}

	return resolved, nil
}

func ValidateHomeRelativeImportSegment(segment string) error {
	if segment == "" {
		return fmt.Errorf("empty home-relative path segment")
	}
	if strings.Contains(segment, "..") {
		return fmt.Errorf(`path traversal detected: "%s" contains ".."`, segment)
	}
	if filepath.IsAbs(segment) {
		return fmt.Errorf(`path traversal detected: "%s" is absolute`, segment)
	}
	return nil
}
