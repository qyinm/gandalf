package agents_test

import (
	"reflect"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/agents"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestCurrentSupportedIDsAreCodexAndClaudeCode(t *testing.T) {
	t.Parallel()

	want := []types.AgentID{types.AgentClaudeCode, types.AgentCodex}
	if got := agents.CurrentSupportedIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("CurrentSupportedIDs() = %#v, want %#v", got, want)
	}
	if got := agents.CurrentSupportedNames(); !reflect.DeepEqual(got, []string{"claude-code", "codex"}) {
		t.Fatalf("CurrentSupportedNames() = %#v", got)
	}
}

func TestIsCurrentSupported(t *testing.T) {
	t.Parallel()

	for _, id := range []types.AgentID{types.AgentClaudeCode, types.AgentCodex} {
		if !agents.IsCurrentSupported(id) {
			t.Fatalf("%s should be current supported", id)
		}
	}
	for _, id := range []types.AgentID{
		types.AgentCursor,
		types.AgentOpencode,
		types.AgentPiAgent,
		types.AgentProject,
		types.AgentUnknown,
	} {
		if agents.IsCurrentSupported(id) {
			t.Fatalf("%s should not be current supported", id)
		}
	}
}

func TestSupportsContentBackedUserSnapshot(t *testing.T) {
	t.Parallel()

	for _, id := range []types.AgentID{types.AgentClaudeCode, types.AgentCodex} {
		if !agents.SupportsContentBackedUserSnapshot(id, types.ScopeUser) {
			t.Fatalf("%s user scope should support content-backed snapshots", id)
		}
		if agents.SupportsContentBackedUserSnapshot(id, types.ScopeProject) {
			t.Fatalf("%s project scope should not support content-backed snapshots", id)
		}
	}
	if agents.SupportsContentBackedUserSnapshot(types.AgentCursor, types.ScopeUser) {
		t.Fatal("cursor should not support content-backed snapshots in current product set")
	}
}
