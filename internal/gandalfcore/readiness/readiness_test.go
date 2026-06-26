package readiness_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/readiness"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func mcpItem(id string, value map[string]any) types.DiscoveredItem {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	name := id
	return types.DiscoveredItem{
		ID: id, Agent: types.AgentClaudeCode, Kind: types.KindMcpServer,
		SourcePath: ".mcp.json", Scope: types.ScopeProject, Precedence: 40,
		Parser: types.ParserJSON, Sensitivity: "command_config",
		ContentPolicy: "structured_safe_fields_only", RestorePolicy: types.RestoreStructuredFields,
		CaptureStatus: types.CaptureCaptured, Confidence: types.ConfidenceHigh,
		Name: &name, Value: raw,
	}
}

func TestClassifiesMcpCommandStatesWithoutExecutingShellStrings(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	markerPath := filepath.Join(root, "shell-marker")
	maliciousCommand := `missing" ; touch "` + markerPath + `" ; "`

	report := readiness.BuildReadinessReport([]types.DiscoveredItem{
		mcpItem("mcp-remote", map[string]any{"url": "https://mcp.example.test"}),
		mcpItem("mcp-local", map[string]any{"command": "/Users/source/.local/bin/private-mcp"}),
		mcpItem("mcp-malicious", map[string]any{"command": maliciousCommand}),
	}, &types.ReadinessOptions{SourceHomeDir: stringPtr("/Users/source")})

	if report.Summary[types.ReadinessUnverified] != 1 {
		t.Fatalf("unverified summary = %d", report.Summary[types.ReadinessUnverified])
	}
	if categoryFor(report, "mcp-remote") != types.ReadinessUnverified {
		t.Fatalf("remote category = %s", categoryFor(report, "mcp-remote"))
	}
	if categoryFor(report, "mcp-local") != types.ReadinessNeedsManualAction {
		t.Fatalf("local category = %s", categoryFor(report, "mcp-local"))
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatal("shell marker file should not exist")
	}
}

func categoryFor(report types.ReadinessReport, evidenceID string) types.ReadinessCategory {
	for _, item := range report.Items {
		if item.EvidenceID != nil && *item.EvidenceID == evidenceID {
			return item.Category
		}
	}
	return ""
}

func stringPtr(value string) *string { return &value }
