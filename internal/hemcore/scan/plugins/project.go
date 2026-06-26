package plugins

import (
	"github.com/qyinm/hem/internal/hemcore/scan"
	"github.com/qyinm/hem/internal/hemcore/types"
)

// ProjectScanner discovers project-level environment variable inventory.
type ProjectScanner struct{}

func (ProjectScanner) AgentID() types.AgentID   { return types.AgentProject }
func (ProjectScanner) AgentName() string        { return "Project" }
func (ProjectScanner) Description() string {
	return "Project-level environment variable inventory"
}
func (ProjectScanner) Scan(*scan.ScannerContext) []types.DiscoveredItem { return nil }
func (ProjectScanner) Targets(projectPath, _ string) []scan.ScanTarget {
	return []scan.ScanTarget{
		scan.ProjectTarget(
			projectPath,
			".env",
			types.AgentProject,
			types.KindEnvKey,
			types.ParserDotenv,
			scan.ScanTargetOverrides{
				Sensitivity:   stringPtr("env_key_inventory"),
				ContentPolicy: stringPtr("key_inventory_only"),
			},
		),
	}
}