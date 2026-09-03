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

	if opts.HomeDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			opts.HomeDir = home
		}
	}

	if opts.OutputFile == "" {
		opts.OutputFile = "gandalf.toml"
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

	targetPath := filepath.Join(opts.ProjectPath, opts.OutputFile)

	// If not dry-run, write manifest and mirror skills
	if !opts.DryRun {
		if fileExists(targetPath) && !opts.Force {
			return nil, fmt.Errorf("manifest '%s' already exists (use --force to overwrite)", opts.OutputFile)
		}

		if err := fsutil.WriteTextAtomically(targetPath, result.FormattedTOML, 0644); err != nil {
			return nil, fmt.Errorf("write %s: %w", opts.OutputFile, err)
		}

		// Mirror discovered skills to .gandalf/skills/<name>
		for _, cand := range candidates {
			if cand.Kind == "skills_dir" {
				entries, err := os.ReadDir(cand.Path)
				if err == nil {
					for _, entry := range entries {
						if entry.IsDir() {
							skillName := entry.Name()
							srcSkillDir := filepath.Join(cand.Path, skillName)
							destSkillDir := filepath.Join(opts.ProjectPath, ".gandalf", "skills", skillName)
							if srcSkillDir != destSkillDir {
								if err := copyDirectory(srcSkillDir, destSkillDir); err == nil {
									result.MirroredSkills = append(result.MirroredSkills, skillName)
								}
							}
						}
					}
				}
			}
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
