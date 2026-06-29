package scan_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/policy"
	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	_ "github.com/qyinm/gandalf/internal/gandalfcore/scan/plugins"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

type sandbox struct {
	projectPath string
	homeDir     string
	storeDir    string
}

func makeSandbox(t *testing.T) sandbox {
	t.Helper()
	root := t.TempDir()
	projectPath := filepath.Join(root, "project")
	homeDir := filepath.Join(root, "home")
	storeDir := filepath.Join(homeDir, ".gandalf")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return sandbox{projectPath: projectPath, homeDir: homeDir, storeDir: storeDir}
}

func scanOptions(sb sandbox) *types.ScanOptions {
	return &types.ScanOptions{
		ProjectPath: sb.projectPath,
		HomeDir:     sb.homeDir,
		StoreDir:    sb.storeDir,
	}
}

func TestDefaultScanIgnoresProjectMCPAndReportsReadOnlyTrust(t *testing.T) {
	sb := makeSandbox(t)
	if err := os.WriteFile(
		filepath.Join(sb.projectPath, ".mcp.json"),
		[]byte(`{"mcpServers":{"github":{"command":"gh","args":["api"],"env":{"GITHUB_TOKEN":"secret"}}}}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	scanResult := scan.ScanProject(scanOptions(sb))

	if !scanResult.Trust.ReadOnly {
		t.Fatal("expected read-only trust")
	}
	if scanResult.Trust.Network != "disabled" {
		t.Fatalf("network = %q", scanResult.Trust.Network)
	}
	if len(scanResult.Trust.CommandsExecuted) != 0 {
		t.Fatalf("commands executed = %#v", scanResult.Trust.CommandsExecuted)
	}
	if scanResult.Trust.StoreWriteLocation != sb.storeDir {
		t.Fatalf("store location = %q", scanResult.Trust.StoreWriteLocation)
	}

	for i := range scanResult.Evidence {
		item := &scanResult.Evidence[i]
		if item.Kind == types.KindMcpServer && item.Name != nil && *item.Name == "github" &&
			item.SourcePath == ".mcp.json" && item.Scope == types.ScopeProject {
			t.Fatalf("default scan included project mcp evidence: %#v", item)
		}
	}

	serialized, err := json.Marshal(scanResult.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(serialized))
	if strings.Contains(lower, "secret") {
		t.Fatalf("serialized evidence leaked secret: %s", serialized)
	}
}

func TestScopeEnabledDefaultsToGlobalOnly(t *testing.T) {
	if scan.ScopeEnabled(types.ScopeProject, nil) {
		t.Fatal("nil scope should exclude project evidence")
	}
	if !scan.ScopeEnabled(types.ScopeUser, nil) {
		t.Fatal("nil scope should include user evidence")
	}
	if !scan.ScopeEnabled(types.ScopeManaged, nil) {
		t.Fatal("nil scope should include managed evidence")
	}

	project := types.ScopeProject
	if !scan.ScopeEnabled(types.ScopeProject, &project) {
		t.Fatal("explicit project scope should include project evidence")
	}
	if scan.ScopeEnabled(types.ScopeUser, &project) {
		t.Fatal("explicit project scope should exclude user evidence")
	}
}

func TestDiscoversCodexMCPServersFromConfigTOML(t *testing.T) {
	sb := makeSandbox(t)
	codexDir := filepath.Join(sb.homeDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := `[mcp_servers.context7] # docs server
command = "npx"
args = [
  "-y",
  "@upstash/context7-mcp",
]

[mcp_servers.node_repl]
command = "node"
enabled = false

[mcp_servers.node_repl.env]
OPENAI_API_KEY = "secret"
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	scanResult := scan.ScanProject(scanOptions(sb))
	var codexMCP []types.DiscoveredItem
	for _, item := range scanResult.Evidence {
		if item.Agent == types.AgentCodex && item.Kind == types.KindMcpServer {
			codexMCP = append(codexMCP, item)
		}
	}

	names := make([]string, 0, len(codexMCP))
	for _, item := range codexMCP {
		if item.Name != nil {
			names = append(names, *item.Name)
		}
	}
	sort.Strings(names)
	if len(names) != 2 || names[0] != "context7" || names[1] != "node_repl" {
		t.Fatalf("mcp names = %#v", names)
	}

	var context7, nodeRepl *types.DiscoveredItem
	for i := range codexMCP {
		switch {
		case codexMCP[i].Name != nil && *codexMCP[i].Name == "context7":
			context7 = &codexMCP[i]
		case codexMCP[i].Name != nil && *codexMCP[i].Name == "node_repl":
			nodeRepl = &codexMCP[i]
		}
	}
	if context7 == nil || nodeRepl == nil {
		t.Fatal("expected context7 and node_repl mcp servers")
	}

	var context7Value map[string]any
	if err := json.Unmarshal(context7.Value, &context7Value); err != nil {
		t.Fatal(err)
	}
	args, _ := context7Value["args"].([]any)
	if len(args) != 2 || args[0] != "-y" || args[1] != "@upstash/context7-mcp" {
		t.Fatalf("context7 args = %#v", context7Value["args"])
	}

	var nodeReplValue map[string]any
	if err := json.Unmarshal(nodeRepl.Value, &nodeReplValue); err != nil {
		t.Fatal(err)
	}
	if nodeReplValue["enabled"] != false {
		t.Fatalf("node_repl enabled = %#v", nodeReplValue["enabled"])
	}
	envKeys, _ := nodeReplValue["envKeys"].([]any)
	if len(envKeys) != 1 || envKeys[0] != "OPENAI_API_KEY" {
		t.Fatalf("node_repl envKeys = %#v", nodeReplValue["envKeys"])
	}

	serialized, err := json.Marshal(codexMCP)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), "secret") {
		t.Fatalf("serialized codex mcp leaked secret: %s", serialized)
	}
}

func TestDiscoversCodexSkillsFromUserAndPluginCache(t *testing.T) {
	sb := makeSandbox(t)
	userSkill := filepath.Join(sb.homeDir, ".codex/skills/review/SKILL.md")
	pluginSkill := filepath.Join(
		sb.homeDir,
		".codex/plugins/cache/openai-curated/build-web-apps/1.0.0/skills/react-best-practices/SKILL.md",
	)
	if err := os.MkdirAll(filepath.Dir(userSkill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(pluginSkill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userSkill, []byte("---\nname: review\ndescription: Review code\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pluginSkill, []byte("---\nname: react-best-practices\ndescription: React guidance\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	scanResult := scan.ScanProject(scanOptions(sb))
	foundReview := false
	foundReact := false
	for _, item := range scanResult.Evidence {
		if item.Agent != types.AgentCodex || item.Kind != types.KindSkill {
			continue
		}
		if item.Name != nil && *item.Name == "review" && item.SourcePath == "~/.codex/skills/review" {
			foundReview = true
		}
		if item.Name != nil && *item.Name == "react-best-practices" &&
			item.SourcePath == "~/.codex/plugins/cache/openai-curated/build-web-apps/1.0.0/skills/react-best-practices" {
			foundReact = true
		}
	}
	if !foundReview || !foundReact {
		t.Fatalf("skills missing: review=%v react=%v", foundReview, foundReact)
	}
}

func TestDiscoversCodexHooksFromHookFiles(t *testing.T) {
	sb := makeSandbox(t)
	projectHooks := filepath.Join(sb.projectPath, ".codex/hooks.json")
	userHooks := filepath.Join(sb.homeDir, ".codex/hooks.json")
	if err := os.MkdirAll(filepath.Dir(projectHooks), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(userHooks), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectHooks, []byte(`{"hooks":{"PreToolUse":[{"matcher":"Write","hooks":[{"type":"command","command":"project-hook","timeout":5}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userHooks, []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"user-hook"}]}],"Stop":[{"hooks":[{"type":"command","command":"stop-hook"}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	scanResult := scan.ScanProject(scanOptions(sb))
	var codexHooks []types.DiscoveredItem
	for _, item := range scanResult.Evidence {
		if item.Agent == types.AgentCodex && item.Kind == types.KindHook {
			codexHooks = append(codexHooks, item)
		}
	}

	names := make([]string, 0, len(codexHooks))
	for _, item := range codexHooks {
		if item.Name != nil {
			names = append(names, *item.Name)
		}
	}
	if len(names) != 2 {
		t.Fatalf("hook names = %#v", names)
	}

	foundProject := false
	foundSession := false
	for _, item := range codexHooks {
		if item.SourcePath == ".codex/hooks.json" && item.Name != nil && *item.Name == "PreToolUse.Write" {
			foundProject = true
		}
		if item.SourcePath == "~/.codex/hooks.json" && item.Name != nil && *item.Name == "SessionStart.*" {
			foundSession = true
		}
	}
	if foundProject || !foundSession {
		t.Fatalf("hooks missing: project=%v session=%v", foundProject, foundSession)
	}
}

func TestFiltersCodexUserGlobalEvidenceWhenAgentAndScopeSpecified(t *testing.T) {
	sb := makeSandbox(t)
	if err := os.MkdirAll(filepath.Join(sb.projectPath, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sb.homeDir, ".codex/skills/review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sb.projectPath, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sb.projectPath, "AGENTS.md"), []byte("project instructions"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sb.projectPath, ".codex/hooks.json"), []byte(`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"project-hook"}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sb.projectPath, ".claude/settings.json"), []byte(`{"permissions":{"allow":["Bash(echo hi)"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sb.homeDir, ".codex/config.toml"), []byte("model = \"gpt-5\"\n[mcp_servers.docs]\ncommand = \"docs-mcp\"\n\n[[hooks.Stop]]\n[[hooks.Stop.hooks]]\ntype = \"command\"\ncommand = \"notify\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sb.homeDir, ".codex/skills/review/SKILL.md"), []byte("---\nname: review\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	agent := types.AgentCodex
	scope := types.ScopeUser
	scanResult := scan.ScanProject(&types.ScanOptions{
		ProjectPath: sb.projectPath,
		HomeDir:     sb.homeDir,
		StoreDir:    sb.storeDir,
		Agent:       &agent,
		Scope:       &scope,
	})

	if len(scanResult.Evidence) == 0 {
		t.Fatal("expected evidence")
	}
	for _, item := range scanResult.Evidence {
		if item.Agent != types.AgentCodex {
			t.Fatalf("unexpected agent %q", item.Agent)
		}
		if item.Scope != types.ScopeUser {
			t.Fatalf("unexpected scope %q", item.Scope)
		}
		if !strings.HasPrefix(item.SourcePath, "~/.codex/") {
			t.Fatalf("unexpected source path %q", item.SourcePath)
		}
	}

	hasConfig := false
	hasMCP := false
	hasHook := false
	hasSkill := false
	for _, item := range scanResult.Evidence {
		switch {
		case item.Kind == types.KindAgentConfig && item.SourcePath == "~/.codex/config.toml":
			hasConfig = true
		case item.Kind == types.KindMcpServer && item.Name != nil && *item.Name == "docs":
			hasMCP = true
		case item.Kind == types.KindHook && item.Name != nil && *item.Name == "Stop.*":
			hasHook = true
		case item.Kind == types.KindSkill && item.Name != nil && *item.Name == "review":
			hasSkill = true
		}
	}
	if !hasConfig || !hasMCP || !hasHook || !hasSkill {
		t.Fatalf("missing evidence: config=%v mcp=%v hook=%v skill=%v", hasConfig, hasMCP, hasHook, hasSkill)
	}
}

func TestDefaultScanIgnoresMalformedProjectJSON(t *testing.T) {
	sb := makeSandbox(t)
	if err := os.MkdirAll(filepath.Join(sb.projectPath, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sb.projectPath, ".claude/settings.json"), []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	scanResult := scan.ScanProject(scanOptions(sb))
	found := false
	for _, item := range scanResult.Evidence {
		if item.SourcePath == ".claude/settings.json" &&
			item.Parser == types.ParserJSON &&
			item.CaptureStatus == types.CaptureParseFailed {
			found = true
			break
		}
	}
	if found {
		t.Fatal("default scan included malformed project json evidence")
	}
}

func TestDefaultScanIgnoresProjectSymlinkEvidence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test requires unix")
	}

	sb := makeSandbox(t)
	if err := os.WriteFile(filepath.Join(sb.projectPath, "CLAUDE.md"), []byte("project instructions"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(sb.projectPath, "CLAUDE.md"), filepath.Join(sb.projectPath, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}

	scanResult := scan.ScanProject(scanOptions(sb))
	foundSymlink := false
	for _, item := range scanResult.Evidence {
		if item.Kind == types.KindSymlink && item.SourcePath == "AGENTS.md" &&
			item.CaptureStatus == types.CaptureOmitted {
			var metadata map[string]any
			if err := json.Unmarshal(item.Metadata, &metadata); err != nil {
				t.Fatal(err)
			}
			if metadata["reason"] == "symlink_not_followed" {
				foundSymlink = true
			}
		}
		if item.SourcePath == "AGENTS.md" && item.Kind == types.KindAgentInstruction {
			t.Fatal("symlink target was followed")
		}
	}
	if foundSymlink {
		t.Fatal("default scan included project symlink evidence")
	}
}

func TestDefaultScanIgnoresProjectDotenv(t *testing.T) {
	sb := makeSandbox(t)
	if err := os.WriteFile(filepath.Join(sb.projectPath, ".env"), []byte("OPENAI_API_KEY=sk-real-secret\nGANDALF_MODE=local\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	scanResult := scan.ScanProject(scanOptions(sb))
	var envEvidence []types.DiscoveredItem
	for _, item := range scanResult.Evidence {
		if item.Kind == types.KindEnvKey {
			envEvidence = append(envEvidence, item)
		}
	}

	if len(envEvidence) != 0 {
		t.Fatalf("default scan included project env evidence: %#v", envEvidence)
	}

	serialized, err := json.Marshal(envEvidence)
	if err != nil {
		t.Fatal(err)
	}
	body := string(serialized)
	if strings.Contains(body, "sk-real-secret") || strings.Contains(body, "local") {
		t.Fatalf("env evidence leaked values: %s", body)
	}
}

func TestDefaultScanIgnoresOversizedProjectFiles(t *testing.T) {
	sb := makeSandbox(t)
	large := strings.Repeat("a", int(policy.MaxFileBytes)+1)
	if err := os.WriteFile(filepath.Join(sb.projectPath, "AGENTS.md"), []byte(large), 0o644); err != nil {
		t.Fatal(err)
	}

	scanResult := scan.ScanProject(scanOptions(sb))
	found := false
	for _, item := range scanResult.Evidence {
		if item.SourcePath == "AGENTS.md" && item.CaptureStatus == types.CaptureUnsupported {
			var metadata map[string]any
			if err := json.Unmarshal(item.Metadata, &metadata); err != nil {
				t.Fatal(err)
			}
			if metadata["reason"] == "file_too_large" {
				found = true
			}
		}
	}
	if found {
		t.Fatal("default scan included oversized project file evidence")
	}
}

func seedVerificationFixture(sb sandbox) error {
	if err := os.WriteFile(filepath.Join(sb.projectPath, ".mcp.json"), []byte(`{"mcpServers":{"claude-mcp":{"command":"claude"}}}`), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(sb.homeDir, ".codex"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(sb.homeDir, ".codex/config.toml"), []byte(`[mcp_servers.context7]
command = "npx"

[mcp_servers.node_repl]
command = "node"

[mcp_servers.node_repl.env]
OPENAI_API_KEY = "secret"
`), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(sb.homeDir, ".claude"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(sb.homeDir, ".claude/settings.json"), []byte(`{"permissions":{"allow":["Bash(echo hi)"]}}`), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(sb.projectPath, ".cursor"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(sb.projectPath, ".cursor/mcp.json"), []byte(`{"mcpServers":{"cursor-mcp":{"command":"cursor"}}}`), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(sb.homeDir, ".cursor"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(sb.homeDir, ".cursor/mcp.json"), []byte(`{"mcpServers":{"cursor-user-mcp":{"command":"cursor"}}}`), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(sb.homeDir, ".config/opencode"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(sb.homeDir, ".config/opencode/opencode.json"), []byte(`{"mcp":{"opencode-mcp":{"type":"local","command":["opencode"]}}}`), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(sb.projectPath, ".pi"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(sb.projectPath, ".pi/settings.json"), []byte(`{"skills":[]}`), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(sb.homeDir, ".pi/agent"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(sb.homeDir, ".pi/agent/settings.json"), []byte(`{"skills":[]}`), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(sb.projectPath, ".env"), []byte("GANDALF_MODE=local\n"), 0o644)
}

func TestDefaultScanDiscoversOnlyCurrentSupportedAgents(t *testing.T) {
	sb := makeSandbox(t)
	if err := seedVerificationFixture(sb); err != nil {
		t.Fatal(err)
	}

	scanResult := scan.ScanProject(scanOptions(sb))
	agents := make(map[types.AgentID]struct{})
	for _, item := range scanResult.Evidence {
		agents[item.Agent] = struct{}{}
	}

	want := []types.AgentID{
		types.AgentClaudeCode,
		types.AgentCodex,
	}
	for _, agent := range want {
		if _, ok := agents[agent]; !ok {
			t.Fatalf("missing agent %q in %#v", agent, agents)
		}
	}
	for agent := range agents {
		if agent != types.AgentClaudeCode && agent != types.AgentCodex {
			t.Fatalf("default scan included unsupported current agent %q in %#v", agent, agents)
		}
	}
}
