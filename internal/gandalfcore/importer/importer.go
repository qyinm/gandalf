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

	// Security: Ensure parent directory components are not symlinks escaping the project
	if err := verifyDestinationPathConfinement(cleanProjectPath, filepath.Dir(targetPath)); err != nil {
		return nil, fmt.Errorf("security violation: output parent directory: %w", err)
	}

	// Security: If target file itself already exists, ensure it is not a symlink
	if fi, err := os.Lstat(targetPath); err == nil && (fi.Mode()&os.ModeSymlink != 0) {
		return nil, fmt.Errorf("security violation: output file '%s' is a symlink", targetPath)
	}

	// Security: Check realpath of parent directory if it exists to prevent symlink traversal
	realProject := cleanProjectPath
	if rp, err := filepath.EvalSymlinks(cleanProjectPath); err == nil {
		realProject = rp
	}
	if realParent, err := filepath.EvalSymlinks(filepath.Dir(targetPath)); err == nil {
		relParent, err := filepath.Rel(realProject, realParent)
		if err != nil || strings.HasPrefix(relParent, "..") {
			return nil, fmt.Errorf("security violation: output directory escapes project root: %s", realParent)
		}
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

		if dirExists(cleanFrom) {
			foundAny := false
			// Check for nested mcp.json
			if fileExists(filepath.Join(cleanFrom, "mcp.json")) {
				candidates = append(candidates, DetectedCandidate{
					Agent: "custom",
					Scope: "project",
					Path:  filepath.Join(cleanFrom, "mcp.json"),
					Kind:  "mcp_json",
				})
				foundAny = true
			}
			// Check for nested config.toml
			if fileExists(filepath.Join(cleanFrom, "config.toml")) {
				candidates = append(candidates, DetectedCandidate{
					Agent: "custom",
					Scope: "project",
					Path:  filepath.Join(cleanFrom, "config.toml"),
					Kind:  "codex_toml",
				})
				foundAny = true
			}
			// Check for nested skills directory
			if dirExists(filepath.Join(cleanFrom, "skills")) {
				candidates = append(candidates, DetectedCandidate{
					Agent: "custom",
					Scope: "project",
					Path:  filepath.Join(cleanFrom, "skills"),
					Kind:  "skills_dir",
				})
				foundAny = true
			}
			// Fallback: treat directory itself as a skills container
			if !foundAny {
				candidates = append(candidates, DetectedCandidate{
					Agent: "custom",
					Scope: "project",
					Path:  cleanFrom,
					Kind:  "skills_dir",
				})
			}
		} else {
			kind := "mcp_json"
			if strings.HasSuffix(cleanFrom, ".toml") {
				kind = "codex_toml"
			} else if strings.HasSuffix(cleanFrom, ".claude.json") {
				kind = "claude_json"
			}
			candidates = append(candidates, DetectedCandidate{
				Agent: "custom",
				Scope: "project",
				Path:  cleanFrom,
				Kind:  kind,
			})
		}
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

		// Ensure parent directory for target output exists
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return nil, fmt.Errorf("create output directory '%s': %w", filepath.Dir(targetPath), err)
		}

		var newlyCreatedSkills []string
		backedUpSkills := make(map[string]string) // destSkillDir -> tempBackupDir
		success := false
		defer func() {
			if !success {
				// Rollback newly created skills
				for _, skillDir := range newlyCreatedSkills {
					_ = os.RemoveAll(skillDir)
				}
				// Restore original skills from backup
				for destDir, bakDir := range backedUpSkills {
					_ = os.RemoveAll(destDir)
					_ = os.Rename(bakDir, destDir)
				}
			} else {
				// Clean up temporary backups on success
				for _, bakDir := range backedUpSkills {
					_ = os.RemoveAll(bakDir)
				}
			}
		}()

		// First, safely mirror skills. Project skills take precedence over global skills.
		// If mirroring fails, abort before writing manifest.
		mirroredScope := make(map[string]string)

		mirrorCandidateSkills := func(cand DetectedCandidate) error {
			if cand.Kind != "skills_dir" {
				return nil
			}
			entries, err := os.ReadDir(cand.Path)
			if err != nil {
				return fmt.Errorf("read skills directory '%s': %w", cand.Path, err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					skillName := entry.Name()
					srcSkillDir := filepath.Join(cand.Path, skillName)

					// Filter out unrecognized non-skill folders (must contain SKILL.md)
					if !fileExists(filepath.Join(srcSkillDir, "SKILL.md")) {
						continue
					}

					// Project skills override global skills
					if mirroredScope[skillName] == "project" {
						continue
					}

					destSkillDir := filepath.Join(opts.ProjectPath, ".gandalf", "skills", skillName)
					if srcSkillDir != destSkillDir {
						// Security: Ensure .gandalf and .gandalf/skills parents are not symlinks
						if err := verifyDestinationPathConfinement(opts.ProjectPath, destSkillDir); err != nil {
							return err
						}

						// Security: If destination itself is a symlink, reject immediately
						if dfi, err := os.Lstat(destSkillDir); err == nil && (dfi.Mode()&os.ModeSymlink != 0) {
							return fmt.Errorf("security violation: destination skill '%s' is a symlink", destSkillDir)
						}

						// Check if destination skill exists and requires --force
						destExisted := dirExists(destSkillDir)
						if destExisted && !opts.Force {
							return fmt.Errorf("team skill '%s' already exists (use --force to overwrite)", skillName)
						}

						if destExisted {
							// For atomic overwrite without retaining obsolete files, move existing to temporary backup
							bakDir := destSkillDir + fmt.Sprintf(".bak.%d", len(backedUpSkills)+1)
							if err := os.Rename(destSkillDir, bakDir); err != nil {
								return fmt.Errorf("backup existing skill '%s': %w", skillName, err)
							}
							backedUpSkills[destSkillDir] = bakDir
						}

						if err := copyDirectory(srcSkillDir, destSkillDir); err != nil {
							return fmt.Errorf("mirror skill '%s': %w", skillName, err)
						}

						if !destExisted {
							newlyCreatedSkills = append(newlyCreatedSkills, destSkillDir)
						}
						mirroredScope[skillName] = cand.Scope
						if !sliceContains(result.MirroredSkills, skillName) {
							result.MirroredSkills = append(result.MirroredSkills, skillName)
						}
					}
				}
			}
			return nil
		}

		// 1. Mirror project skills first
		for _, cand := range candidates {
			if cand.Scope == "project" {
				if err := mirrorCandidateSkills(cand); err != nil {
					return nil, err
				}
			}
		}

		// 2. Mirror global skills second (skipping any duplicate skillNames)
		for _, cand := range candidates {
			if cand.Scope != "project" {
				if err := mirrorCandidateSkills(cand); err != nil {
					return nil, err
				}
			}
		}

		// Finally, atomically write the manifest
		if err := fsutil.WriteTextAtomically(targetPath, result.FormattedTOML, 0644); err != nil {
			return nil, fmt.Errorf("write %s: %w", opts.OutputFile, err)
		}
		success = true
	}

	return result, nil
}

func copyDirectory(src, dst string) error {
	// Security: If destination root is a symlink, do not follow it
	if fi, err := os.Lstat(dst); err == nil && (fi.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("security violation: destination directory '%s' is a symlink", dst)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	for _, entry := range entries {
		// Security: Ignore source symlinks to prevent path traversal / link-following exploits
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}

		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		// Security: If destination child already exists as a symlink, reject
		if fi, err := os.Lstat(dstPath); err == nil && (fi.Mode()&os.ModeSymlink != 0) {
			return fmt.Errorf("security violation: destination file '%s' is a symlink", dstPath)
		}

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
	// Security: Check for source symlink before opening
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil
	}

	// Security: Check for destination symlink before creating/writing
	if dfi, err := os.Lstat(dst); err == nil && (dfi.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("security violation: destination file '%s' is a symlink", dst)
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

// verifyDestinationPathConfinement checks that destDir and all its parent directories
// under projectPath are regular directories and never symlinks.
func verifyDestinationPathConfinement(projectRoot, destPath string) error {
	cleanProj := filepath.Clean(projectRoot)
	cleanDest := filepath.Clean(destPath)

	rel, err := filepath.Rel(cleanProj, cleanDest)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("security violation: path '%s' escapes project root '%s'", destPath, projectRoot)
	}

	parts := strings.Split(rel, string(filepath.Separator))
	curr := cleanProj
	for _, part := range parts {
		curr = filepath.Join(curr, part)
		fi, err := os.Lstat(curr)
		if err != nil {
			if os.IsNotExist(err) {
				// Path does not exist yet; safe to create as regular directory
				break
			}
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("security violation: destination path component '%s' is a symlink", curr)
		}
	}
	return nil
}

func sliceContains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
