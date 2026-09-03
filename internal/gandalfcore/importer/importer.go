package importer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/fsutil"
	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
)

// RunImport performs the full import pipeline: detection, parsing, reconciling,
// templatizing secrets, validating, and writing out gandalf.toml (unless dry-run).
func RunImport(opts ImportOptions) (*ImportResult, error) {
	if opts.ProjectPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get current working directory: %w", err)
		}
		opts.ProjectPath = cwd
	}

	cleanProjectPath := filepath.Clean(opts.ProjectPath)
	opts.ProjectPath = cleanProjectPath

	if opts.HomeDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			opts.HomeDir = home
		}
	}

	if opts.OutputFile == "" {
		opts.OutputFile = "gandalf.toml"
	}

	// Security: Validate OutputFile does not escape the project boundary
	cleanOutput := filepath.Clean(opts.OutputFile)
	var targetPath string
	if filepath.IsAbs(cleanOutput) {
		rel, err := filepath.Rel(cleanProjectPath, cleanOutput)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("security violation: output file '%s' escapes project root '%s'", opts.OutputFile, cleanProjectPath)
		}
		targetPath = cleanOutput
	} else {
		if strings.HasPrefix(cleanOutput, "..") {
			return nil, fmt.Errorf("security violation: output file '%s' escapes project root '%s'", opts.OutputFile, cleanProjectPath)
		}
		targetPath = filepath.Join(cleanProjectPath, cleanOutput)
	}

	var candidates []DetectedCandidate

	if opts.FromPath != "" {
		cleanFrom := filepath.Clean(opts.FromPath)
		if !filepath.IsAbs(cleanFrom) {
			cleanFrom = filepath.Join(opts.ProjectPath, cleanFrom)
		}
		if !fileExists(cleanFrom) && !dirExists(cleanFrom) {
			return nil, fmt.Errorf("specified --from target does not exist: %s", opts.FromPath)
		}

		kind := "mcp_json"
		if strings.HasSuffix(cleanFrom, ".toml") {
			kind = "codex_toml"
		} else if strings.HasSuffix(cleanFrom, ".claude.json") {
			kind = "claude_json"
		} else if dirExists(cleanFrom) {
			kind = "skills_dir"
		}

		candidates = append(candidates, DetectedCandidate{
			Agent: "custom",
			Scope: "project",
			Path:  cleanFrom,
			Kind:  kind,
		})
	} else {
		candidates = DetectCandidates(opts)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no agent configurations found to import (checked .cursor/mcp.json, .mcp.json, ~/.cursor/mcp.json, etc.)")
	}

	result, err := ReconcileSources(opts, candidates)
	if err != nil {
		return nil, fmt.Errorf("reconcile sources: %w", err)
	}

	// Validate the generated manifest
	validationErrors := manifest.Validate(result.Manifest, opts.ProjectPath)
	if len(validationErrors) > 0 {
		var errMsgs []string
		for _, ve := range validationErrors {
			errMsgs = append(errMsgs, ve.Error())
		}
		return nil, fmt.Errorf("generated manifest failed validation: %s", strings.Join(errMsgs, "; "))
	}

	// If not dry-run, verify file overwrite permissions, mirror skills, and write manifest
	if !opts.DryRun {
		if fileExists(targetPath) && !opts.Force {
			return nil, fmt.Errorf("manifest '%s' already exists (use --force to overwrite)", opts.OutputFile)
		}

		// First, safely mirror skills. If mirroring fails, abort before writing manifest.
		for _, cand := range candidates {
			if cand.Kind == "skills_dir" {
				entries, err := os.ReadDir(cand.Path)
				if err != nil {
					return nil, fmt.Errorf("read skills directory '%s': %w", cand.Path, err)
				}
				for _, entry := range entries {
					if entry.IsDir() {
						skillName := entry.Name()
						srcSkillDir := filepath.Join(cand.Path, skillName)
						destSkillDir := filepath.Join(opts.ProjectPath, ".gandalf", "skills", skillName)
						if srcSkillDir != destSkillDir {
							if err := copyDirectory(srcSkillDir, destSkillDir); err != nil {
								return nil, fmt.Errorf("mirror skill '%s': %w", skillName, err)
							}
							result.MirroredSkills = append(result.MirroredSkills, skillName)
						}
					}
				}
			}
		}

		// Finally, atomically write the manifest
		if err := fsutil.WriteTextAtomically(targetPath, result.FormattedTOML, 0644); err != nil {
			return nil, fmt.Errorf("write %s: %w", opts.OutputFile, err)
		}
	}

	return result, nil
}

func copyDirectory(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	for _, entry := range entries {
		// Security: Ignore symlinks to prevent path traversal / link-following exploits
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}

		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDirectory(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	// Security: Check for symlink before opening
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
