package boost

import (
	"fmt"
	"sort"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
)

// ResolveSkills resolves the complete list of unique skill names for a profile,
// including all skills from inherited profiles (via 'includes').
func ResolveSkills(m *manifest.Manifest, profileName string) ([]string, error) {
	if m == nil {
		return nil, fmt.Errorf("manifest is nil")
	}

	profile, exists := m.Profiles[profileName]
	if !exists {
		return nil, fmt.Errorf("profile '%s' is not defined in manifest", profileName)
	}

	visited := make(map[string]bool)
	skillsSet := make(map[string]struct{})
	var orderedSkills []string

	var resolveHelper func(pName string) error
	resolveHelper = func(pName string) error {
		if visited[pName] {
			return nil
		}
		visited[pName] = true

		p, ok := m.Profiles[pName]
		if !ok {
			return fmt.Errorf("referenced profile '%s' does not exist", pName)
		}

		// First resolve included profiles
		for _, inc := range p.Includes {
			if err := resolveHelper(inc); err != nil {
				return err
			}
		}

		// Then collect own skills
		for _, sk := range p.Skills {
			if _, already := skillsSet[sk]; !already {
				skillsSet[sk] = struct{}{}
				orderedSkills = append(orderedSkills, sk)
			}
		}

		return nil
	}

	if err := resolveHelper(profileName); err != nil {
		return nil, err
	}

	// We can sort for deterministic results if desired, or keep insertion order.
	_ = profile
	sort.Strings(orderedSkills)
	return orderedSkills, nil
}
