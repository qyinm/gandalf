package scan

import (
	"path/filepath"

	"github.com/qyinm/hem/internal/hemcore/types"
)

// ScanTarget describes a filesystem location to scan for evidence.
type ScanTarget struct {
	AbsolutePath string
	SourcePath   string
	Scope        types.EvidenceScope
	Agent        types.AgentID
	Kind         types.EvidenceKind
	Parser       types.EvidenceParser
	Precedence   uint32
	Sensitivity  string
	ContentPolicy string
	Directory    bool
	MetadataOnly bool
}

// ScanTargetOverrides optionally overrides ScanTarget defaults.
type ScanTargetOverrides struct {
	Sensitivity   *string
	ContentPolicy *string
	Directory     *bool
	MetadataOnly  *bool
	Precedence    *uint32
}

// ProjectTarget builds a project-scoped scan target.
func ProjectTarget(
	projectPath string,
	relativePath string,
	agent types.AgentID,
	kind types.EvidenceKind,
	parser types.EvidenceParser,
	overrides ScanTargetOverrides,
) ScanTarget {
	return makeTarget(
		projectPath,
		relativePath,
		types.ScopeProject,
		40,
		agent,
		kind,
		parser,
		overrides,
	)
}

// HomeTarget builds a user-scoped scan target under the home directory.
func HomeTarget(
	homeDir string,
	relativePath string,
	agent types.AgentID,
	kind types.EvidenceKind,
	parser types.EvidenceParser,
	overrides ScanTargetOverrides,
) ScanTarget {
	return makeTarget(
		homeDir,
		relativePath,
		types.ScopeUser,
		10,
		agent,
		kind,
		parser,
		overrides,
	)
}

func makeTarget(
	root string,
	relativePath string,
	scope types.EvidenceScope,
	precedence uint32,
	agent types.AgentID,
	kind types.EvidenceKind,
	parser types.EvidenceParser,
	overrides ScanTargetOverrides,
) ScanTarget {
	sourcePath := relativePath
	if scope == types.ScopeUser {
		sourcePath = "~/" + relativePath
	}

	sensitivity := "metadata"
	if overrides.Sensitivity != nil {
		sensitivity = *overrides.Sensitivity
	}

	contentPolicy := "metadata_only"
	if overrides.ContentPolicy != nil {
		contentPolicy = *overrides.ContentPolicy
	}

	directory := false
	if overrides.Directory != nil {
		directory = *overrides.Directory
	}

	metadataOnly := false
	if overrides.MetadataOnly != nil {
		metadataOnly = *overrides.MetadataOnly
	}

	if overrides.Precedence != nil {
		precedence = *overrides.Precedence
	}

	return ScanTarget{
		AbsolutePath:  filepath.Join(root, relativePath),
		SourcePath:    sourcePath,
		Scope:         scope,
		Agent:         agent,
		Kind:          kind,
		Parser:        parser,
		Precedence:    precedence,
		Sensitivity:   sensitivity,
		ContentPolicy: contentPolicy,
		Directory:     directory,
		MetadataOnly:  metadataOnly,
	}
}