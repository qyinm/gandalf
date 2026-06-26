package policy_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/policy"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestRestorePolicyFor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind types.EvidenceKind
		want types.RestorePolicy
	}{
		{types.KindAgentConfig, types.RestoreFullContent},
		{types.KindMcpServer, types.RestoreStructuredFields},
		{types.KindEnvKey, types.RestoreKeyInventory},
		{types.KindSymlink, types.RestoreNotSupported},
	}
	for _, tc := range cases {
		if got := policy.RestorePolicyFor(tc.kind); got != tc.want {
			t.Fatalf("RestorePolicyFor(%s) = %s, want %s", tc.kind, got, tc.want)
		}
	}
}

func TestSecretLikeKeys(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"OPENAI_API_KEY", "github_token", "client-secret"} {
		if !policy.IsSecretLikeKey(key) {
			t.Fatalf("expected secret-like key %q", key)
		}
	}
	if policy.IsSecretLikeKey("MODEL_NAME") {
		t.Fatal("MODEL_NAME should not be secret-like")
	}
	if policy.CaptureStatusForKey("OPENAI_API_KEY") != "redacted" {
		t.Fatal("expected redacted")
	}
	if policy.CaptureStatusForKey("MODEL_NAME") != "omitted" {
		t.Fatal("expected omitted")
	}
}

func TestRedactStructuredValue(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`{"command":"npx","api_key":"secret-value","env":{"GITHUB_TOKEN":"abc","MODEL":"gpt-5"}}`)
	redacted, err := policy.RedactStructuredValue(input)
	if err != nil {
		t.Fatalf("redact: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(redacted, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["api_key"] != "[redacted]" {
		t.Fatalf("api_key = %v", obj["api_key"])
	}
	if obj["command"] != "npx" {
		t.Fatalf("command = %v", obj["command"])
	}
	envKeys, ok := obj["envKeys"].([]any)
	if !ok || len(envKeys) != 2 {
		t.Fatalf("envKeys = %v", obj["envKeys"])
	}
	if _, hasEnv := obj["env"]; hasEnv {
		t.Fatal("env should be removed")
	}
}

func TestIgnoredDirectory(t *testing.T) {
	t.Parallel()
	if !policy.IgnoredDirectory("node_modules") || !policy.IgnoredDirectory(".git") {
		t.Fatal("expected ignored directories")
	}
	if policy.IgnoredDirectory("src") {
		t.Fatal("src should not be ignored")
	}
}

func fixturesDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "fixtures", "evidence")
}

func TestEvidenceFixtureRoundTrip(t *testing.T) {
	t.Parallel()
	for _, file := range []string{"user-permission-bash.json", "project-permission-bash.json"} {
		raw, err := os.ReadFile(filepath.Join(fixturesDir(t), file))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		var item types.DiscoveredItem
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Fatalf("deserialize: %v", err)
		}
		serialized, err := json.Marshal(item)
		if err != nil {
			t.Fatalf("serialize: %v", err)
		}
		var roundTrip types.DiscoveredItem
		if err := json.Unmarshal(serialized, &roundTrip); err != nil {
			t.Fatalf("round trip: %v", err)
		}
		if roundTrip.ID != item.ID || roundTrip.Agent != item.Agent || roundTrip.Scope != item.Scope {
			t.Fatalf("round trip mismatch for %s", file)
		}
	}
}

func TestEvidenceFixtureScopes(t *testing.T) {
	t.Parallel()
	dir := fixturesDir(t)
	var user, project types.DiscoveredItem
	if err := json.Unmarshal(mustRead(t, filepath.Join(dir, "user-permission-bash.json")), &user); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mustRead(t, filepath.Join(dir, "project-permission-bash.json")), &project); err != nil {
		t.Fatal(err)
	}
	if user.Scope != types.ScopeUser || user.SourcePath != "~/.claude/settings.json" {
		t.Fatalf("user fixture: %+v", user)
	}
	if project.Scope != types.ScopeProject || project.SourcePath != ".claude/settings.json" {
		t.Fatalf("project fixture: %+v", project)
	}
}

func TestInvalidAgentIDDeserializesToUnknown(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{
		"id":"unknown-agent",
		"agent":"not-a-real-agent",
		"kind":"permission",
		"sourcePath":".claude/settings.json",
		"scope":"project",
		"precedence":40,
		"parser":"json",
		"sensitivity":"command_config",
		"contentPolicy":"structured_safe_fields_only",
		"restorePolicy":"structured_fields_only",
		"captureStatus":"captured",
		"confidence":"high"
	}`)
	var item types.DiscoveredItem
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	if item.Agent != types.AgentUnknown {
		t.Fatalf("agent = %s", item.Agent)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
