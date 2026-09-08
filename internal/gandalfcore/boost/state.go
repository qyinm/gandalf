package boost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/qyinm/gandalf/internal/gandalfcore/fsutil"
)

// StateFilePath returns the path to the boost state cache file.
func StateFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".gandalf", ".boost.json")
}

// LoadState reads the boost state from .gandalf/.boost.json.
// If the file does not exist, an empty state is returned without error.
func LoadState(projectRoot string) (*BoostState, error) {
	filePath := StateFilePath(projectRoot)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &BoostState{
				ActiveProfile:  "",
				Scope:          ScopeProject,
				ProjectRoot:    projectRoot,
				InjectedSkills: nil,
			}, nil
		}
		return nil, err
	}

	var state BoostState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.Scope == "" {
		state.Scope = ScopeProject
	}
	state.ProjectRoot = projectRoot
	return &state, nil
}

// SaveState writes the boost state to .gandalf/.boost.json atomically.
func SaveState(projectRoot string, state *BoostState) error {
	if state == nil {
		return nil
	}
	state.ProjectRoot = projectRoot
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	filePath := StateFilePath(projectRoot)
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return fsutil.WriteTextAtomically(filePath, string(data), 0644)
}

// RemoveState removes the .gandalf/.boost.json file.
func RemoveState(projectRoot string) error {
	filePath := StateFilePath(projectRoot)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
