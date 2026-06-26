package scan

import (
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// ScannerContext carries runtime options for custom scanner plugins.
type ScannerContext struct {
	ProjectPath string
	HomeDir     string
	StoreDir    string
	Explain     bool
	Scope       *types.EvidenceScope
}

// ScannerPlugin discovers evidence targets and optionally performs custom scans.
type ScannerPlugin interface {
	AgentID() types.AgentID
	AgentName() string
	Description() string
	Targets(projectPath, homeDir string) []ScanTarget
	// Scan returns custom evidence when implemented; nil means use filesystem scan on Targets.
	Scan(context *ScannerContext) []types.DiscoveredItem
}
