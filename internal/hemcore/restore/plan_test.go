package restore_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qyinm/hem/internal/hemcore/restore"
	"github.com/qyinm/hem/internal/hemcore/snapshot"
	"github.com/qyinm/hem/internal/hemcore/store"
	"github.com/qyinm/hem/internal/hemcore/types"
	_ "github.com/qyinm/hem/internal/hemcore/scan/plugins"
)

func makeRestoreSandbox(t *testing.T) (projectPath, homeDir, storeDir string) {
	t.Helper()
	root := t.TempDir()
	projectPath = filepath.Join(root, "project")
	homeDir = filepath.Join(root, "home")
	storeDir = filepath.Join(root, "store")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return projectPath, homeDir, storeDir
}

func TestRestoresCodexConfigByteForByteThroughTargetHome(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, storeDir := makeRestoreSandbox(t)
	configPath := filepath.Join(homeDir, ".codex", "config.toml")
	original := "model = \"gpt-5\"\napproval_policy = \"on-request\"\n"
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	agent := types.AgentCodex
	scope := types.ScopeUser
	state, err := snapshot.CaptureCurrentState(&types.RuntimeOptions{
		ProjectPath:    projectPath,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		Agent:          &agent,
		Scope:          &scope,
		CaptureContent: true,
	}, "baseline")
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if err := store.WriteSnapshot(storeDir, store.StoreSnapshotFrom(state.Snapshot), &agent); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := restore.BuildRestorePlan(&types.RestoreOptions{
		SourceSnapshot: "baseline",
		ProjectPath:    projectPath,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		DryRun:         true,
		Agent:          &agent,
		Scope:          &scope,
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	var configItem *types.RestorePlanItem
	for i := range plan.Items {
		if plan.Items[i].Kind == types.KindAgentConfig {
			configItem = &plan.Items[i]
			break
		}
	}
	if configItem == nil {
		t.Fatal("config item missing")
	}
	if configItem.Action != types.RestoreActionUpdate {
		t.Fatalf("action = %s", configItem.Action)
	}
	if configItem.Agent != types.AgentCodex {
		t.Fatalf("agent = %s", configItem.Agent)
	}
	if configItem.SourcePath != "~/.codex/config.toml" {
		t.Fatalf("source path = %q", configItem.SourcePath)
	}
	if plan.TargetHome != homeDir {
		t.Fatalf("target home = %q", plan.TargetHome)
	}

	planJSON, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	parsed := restore.ParseDryRunOutput(string(planJSON))
	if len(parsed.Errors) != 0 {
		t.Fatalf("parse errors: %#v", parsed.Errors)
	}
	var executable *types.RestoreItem
	for i := range parsed.Items {
		if parsed.Items[i].ItemType == "agent_config" {
			executable = &parsed.Items[i]
			break
		}
	}
	if executable == nil {
		t.Fatal("executable item missing")
	}
	if executable.Dest != configPath {
		t.Fatalf("dest = %q", executable.Dest)
	}
	var targetText string
	if err := json.Unmarshal(executable.TargetContent, &targetText); err != nil {
		t.Fatalf("target content: %v", err)
	}
	if targetText != original {
		t.Fatalf("target content = %q", targetText)
	}

	items := parsed.Items
	summary := restore.ApplyRestoreItems(items, restore.CreateDefaultApplyExecutor(), &types.ApplyOptions{
		FailFast:    true,
		HomeDir:     &homeDir,
		ProjectPath: &projectPath,
	})
	if summary.Failed != 0 {
		t.Fatalf("apply failed: %#v", summary.Failures)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("restored = %q", string(got))
	}
}

func TestDeletesCodexUserSkillAddedAfterBaseline(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, storeDir := makeRestoreSandbox(t)
	codexDir := filepath.Join(homeDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte("model = \"gpt-5\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	agent := types.AgentCodex
	scope := types.ScopeUser
	state, err := snapshot.CaptureCurrentState(&types.RuntimeOptions{
		ProjectPath:    projectPath,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		Agent:          &agent,
		Scope:          &scope,
		CaptureContent: true,
	}, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteSnapshot(storeDir, store.StoreSnapshotFrom(state.Snapshot), &agent); err != nil {
		t.Fatal(err)
	}

	skillFile := filepath.Join(homeDir, ".codex", "skills", "unsafe", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillFile, []byte("---\nname: unsafe\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := restore.BuildRestorePlan(&types.RestoreOptions{
		SourceSnapshot: "baseline",
		ProjectPath:    projectPath,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		DryRun:         true,
		Agent:          &agent,
		Scope:          &scope,
	})
	if err != nil {
		t.Fatal(err)
	}

	var skillItem *types.RestorePlanItem
	for i := range plan.Items {
		item := &plan.Items[i]
		if item.Kind == types.KindSkill && item.Action == types.RestoreActionDelete {
			skillItem = item
			break
		}
	}
	if skillItem == nil {
		t.Fatal("skill delete item missing")
	}
	if skillItem.Agent != types.AgentCodex {
		t.Fatalf("agent = %s", skillItem.Agent)
	}
	if skillItem.SourcePath != "~/.codex/skills/unsafe/SKILL.md" {
		t.Fatalf("source path = %q", skillItem.SourcePath)
	}

	planJSON, _ := json.Marshal(plan)
	items := restore.ParseDryRunOutput(string(planJSON)).Items
	summary := restore.ApplyRestoreItems(items, restore.CreateDefaultApplyExecutor(), &types.ApplyOptions{
		FailFast:    true,
		HomeDir:     &homeDir,
		ProjectPath: &projectPath,
	})
	if summary.Failed != 0 {
		t.Fatalf("apply failed: %#v", summary.Failures)
	}
	if _, err := os.Stat(skillFile); !os.IsNotExist(err) {
		t.Fatalf("skill file still exists: %v", err)
	}
}

func TestMarksCodexTOMLMCPChangesUnsupportedWhileConfigCarriesRestore(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, storeDir := makeRestoreSandbox(t)
	configPath := filepath.Join(homeDir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("model = \"gpt-5\"\n[mcp_servers.docs]\ncommand = \"docs-old\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	agent := types.AgentCodex
	scope := types.ScopeUser
	state, err := snapshot.CaptureCurrentState(&types.RuntimeOptions{
		ProjectPath:    projectPath,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		Agent:          &agent,
		Scope:          &scope,
		CaptureContent: true,
	}, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteSnapshot(storeDir, store.StoreSnapshotFrom(state.Snapshot), &agent); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("model = \"gpt-5\"\n[mcp_servers.docs]\ncommand = \"docs-new\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := restore.BuildRestorePlan(&types.RestoreOptions{
		SourceSnapshot: "baseline",
		ProjectPath:    projectPath,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		DryRun:         true,
		Agent:          &agent,
		Scope:          &scope,
	})
	if err != nil {
		t.Fatal(err)
	}

	hasConfigUpdate := false
	for _, item := range plan.Items {
		if item.Kind == types.KindAgentConfig && item.Action == types.RestoreActionUpdate {
			hasConfigUpdate = true
			break
		}
	}
	if !hasConfigUpdate {
		t.Fatal("expected agent_config update item")
	}

	foundUnsupportedMCP := false
	for _, item := range plan.UnsupportedItems {
		if item.Kind == types.KindMcpServer &&
			item.Agent == types.AgentCodex &&
			strings.Contains(item.Reason, "No supported restore action") {
			foundUnsupportedMCP = true
			break
		}
	}
	if !foundUnsupportedMCP {
		t.Fatalf("unsupported mcp items = %#v", plan.UnsupportedItems)
	}
}

func TestMetadataOnlySnapshotRefusesAgentConfigApplyWithoutContentBacking(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, storeDir := makeRestoreSandbox(t)
	configPath := filepath.Join(homeDir, ".codex", "config.toml")
	original := "model = \"gpt-5\"\napproval_policy = \"on-request\"\n"
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	agent := types.AgentCodex
	scope := types.ScopeUser
	state, err := snapshot.CaptureCurrentState(&types.RuntimeOptions{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
		StoreDir:    storeDir,
		Agent:       &agent,
		Scope:       &scope,
	}, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Snapshot.Content) != 0 {
		t.Fatal("expected metadata-only snapshot")
	}
	if err := store.WriteSnapshot(storeDir, store.StoreSnapshotFrom(state.Snapshot), &agent); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := restore.BuildRestorePlan(&types.RestoreOptions{
		SourceSnapshot: "baseline",
		ProjectPath:    projectPath,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		DryRun:         true,
		Agent:          &agent,
		Scope:          &scope,
	})
	if err != nil {
		t.Fatal(err)
	}

	var configPlanItem *types.RestorePlanItem
	for i := range plan.Items {
		if plan.Items[i].Kind == types.KindAgentConfig {
			configPlanItem = &plan.Items[i]
			break
		}
	}
	if configPlanItem == nil || configPlanItem.TargetState == nil || len(configPlanItem.TargetState.Value) == 0 {
		t.Fatal("expected target state value on config plan item")
	}

	planJSON, _ := json.Marshal(plan)
	parsed := restore.ParseDryRunOutput(string(planJSON))
	var executable *types.RestoreItem
	for i := range parsed.Items {
		if parsed.Items[i].ItemType == "agent_config" {
			executable = &parsed.Items[i]
			break
		}
	}
	if executable == nil {
		t.Fatal("executable item missing")
	}
	if len(executable.TargetContent) != 0 {
		t.Fatal("metadata-only snapshots must not populate string file content for apply")
	}

	items := parsed.Items
	summary := restore.ApplyRestoreItems(items, restore.CreateDefaultApplyExecutor(), &types.ApplyOptions{
		FailFast:    true,
		HomeDir:     &homeDir,
		ProjectPath: &projectPath,
	})
	if summary.Successful != 0 || summary.Failed != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if len(summary.Failures) == 0 || !strings.Contains(summary.Failures[0].Reason, "Missing target content") {
		t.Fatalf("unexpected failure: %#v", summary.Failures)
	}
	got, _ := os.ReadFile(configPath)
	if string(got) == original {
		t.Fatal("config should not have been restored")
	}
}

func TestParseDryRunSkipsDestinationsWithTraversal(t *testing.T) {
	t.Parallel()
	planJSON := `{
		"targetProject": "/tmp/project",
		"targetHome": "/tmp/home",
		"items": [{
			"itemId": "agent_config:test:abcd",
			"agent": "codex",
			"kind": "agent_config",
			"sourcePath": "~/../../etc/passwd",
			"dependsOn": [],
			"action": "update",
			"diff": { "changes": [], "additions": [], "removals": [] },
			"riskLevel": "low",
			"riskReason": "test",
			"needsConfirmation": false,
			"confirmationPrompt": "",
			"rollbackInstruction": "reverse",
			"targetState": {
				"id": "cfg-1",
				"agent": "codex",
				"kind": "agent_config",
				"sourcePath": "~/../../etc/passwd",
				"scope": "user",
				"precedence": 1,
				"parser": "toml",
				"sensitivity": "low",
				"contentPolicy": "content_backed",
				"restorePolicy": "full_content_supported",
				"captureStatus": "captured",
				"confidence": "high",
				"value": "model = \"x\"\n"
			}
		}],
		"executionOrder": ["agent_config:test:abcd"]
	}`
	parsed := restore.ParseDryRunOutput(planJSON)
	if len(parsed.Items) != 0 {
		t.Fatalf("items = %#v", parsed.Items)
	}
	found := false
	for _, errItem := range parsed.Errors {
		if strings.Contains(errItem.Message, "traversal") ||
			strings.Contains(errItem.Message, "outside home and project") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("errors = %#v", parsed.Errors)
	}
}

func writeProjectMCP(t *testing.T, projectPath, command string) {
	t.Helper()
	payload := map[string]any{
		"mcpServers": map[string]any{
			"github": map[string]any{"command": command},
		},
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRestorePlanPipelineAppliesMCPPermissionAndEnvWithConfinement(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, storeDir := makeRestoreSandbox(t)
	writeProjectMCP(t, projectPath, "gh-baseline")
	claudeDir := filepath.Join(homeDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(claudeDir, "settings.json"),
		[]byte("{\n  \"permissions\": {\n    \"bash\": { \"allow\": [] }\n  }\n}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	state, err := snapshot.CaptureCurrentState(&types.RuntimeOptions{
		ProjectPath:    projectPath,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		CaptureContent: false,
	}, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteSnapshot(storeDir, store.StoreSnapshotFrom(state.Snapshot), nil); err != nil {
		t.Fatal(err)
	}

	writeProjectMCP(t, projectPath, "gh-changed")
	if err := os.WriteFile(
		filepath.Join(claudeDir, "settings.json"),
		[]byte("{\n  \"permissions\": {\n    \"bash\": { \"allow\": [\"Bash(npm)\"] }\n  }\n}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, ".env"), []byte("NEW_KEY=added\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := restore.BuildRestorePlan(&types.RestoreOptions{
		SourceSnapshot: "baseline",
		ProjectPath:    projectPath,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		DryRun:         true,
	})
	if err != nil {
		t.Fatal(err)
	}

	var mcpItem, permissionItem, envItem *types.RestorePlanItem
	for i := range plan.Items {
		switch plan.Items[i].Kind {
		case types.KindMcpServer:
			mcpItem = &plan.Items[i]
		case types.KindPermission:
			permissionItem = &plan.Items[i]
		case types.KindEnvKey:
			envItem = &plan.Items[i]
		}
	}
	if mcpItem == nil || mcpItem.Action != types.RestoreActionUpdate {
		t.Fatalf("mcp item = %#v", mcpItem)
	}
	if permissionItem == nil || permissionItem.Action != types.RestoreActionUpdate {
		t.Fatalf("permission item = %#v", permissionItem)
	}
	if envItem == nil || envItem.Action != types.RestoreActionDelete {
		t.Fatalf("env item = %#v", envItem)
	}

	planJSON, _ := json.Marshal(plan)
	parsed := restore.ParseDryRunOutput(string(planJSON))
	if len(parsed.Errors) != 0 {
		t.Fatalf("parse errors: %#v", parsed.Errors)
	}

	structuredTypes := []string{"mcp_server", "permission", "env_key"}
	items := parsed.Items
	for _, itemType := range structuredTypes {
		found := false
		for _, item := range items {
			if item.ItemType == itemType {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %s in %#v", itemType, items)
		}
	}

	summary := restore.ApplyRestoreItems(items, restore.CreateDefaultApplyExecutor(), &types.ApplyOptions{
		FailFast:    false,
		HomeDir:     &homeDir,
		ProjectPath: &projectPath,
	})
	for _, itemType := range structuredTypes {
		var applied *types.RestoreItem
		for i := range items {
			if items[i].ItemType == itemType {
				applied = &items[i]
				break
			}
		}
		if applied == nil || applied.Status != types.RestoreItemStatusApplied {
			t.Fatalf("%s apply failed: status=%s err=%v", itemType, applied.Status, applied.ErrorMessage)
		}
	}
	for _, failure := range summary.Failures {
		var itemType string
		for _, item := range items {
			if item.ItemID == failure.ItemID {
				itemType = item.ItemType
				break
			}
		}
		if itemType != "agent_config" {
			t.Fatalf("non-agent_config failures: %#v", summary.Failures)
		}
	}

	mcpWritten, _ := os.ReadFile(filepath.Join(projectPath, ".mcp.json"))
	if !strings.Contains(string(mcpWritten), "gh-baseline") {
		t.Fatalf("mcp = %s", string(mcpWritten))
	}

	settingsWritten, _ := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	var settingsJSON map[string]any
	if err := json.Unmarshal(settingsWritten, &settingsJSON); err != nil {
		t.Fatal(err)
	}
	permissions := settingsJSON["permissions"].(map[string]any)
	bashRule := permissions["bash"].(map[string]any)
	allow, _ := bashRule["allow"].([]any)
	if len(allow) != 0 {
		t.Fatalf("bash allow = %#v", allow)
	}
	if _, hasRule := bashRule["rule"]; hasRule {
		t.Fatal("permission apply must unwrap rule wrapper")
	}

	envWritten, _ := os.ReadFile(filepath.Join(projectPath, ".env"))
	if strings.Contains(string(envWritten), "NEW_KEY=") {
		t.Fatalf("env = %s", string(envWritten))
	}
}