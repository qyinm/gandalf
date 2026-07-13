package setup

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

type recordingCommandRunner struct {
	commands []CommandPlan
	err      error
}

func (runner *recordingCommandRunner) Run(_ context.Context, command CommandPlan) error {
	runner.commands = append(runner.commands, command)
	return runner.err
}

func TestClaudeMarketplaceInstallFullLoop(t *testing.T) {
	source, entry := claudeMarketplaceInstallFixture()
	plan := PlanMarketplaceInstall(source, entry)
	if !plan.Available {
		t.Fatalf("plan unavailable: %#v", plan)
	}
	wantInstall := CommandPlan{Program: "claude", Args: []string{"plugin", "install", "codex@openai-codex", "--scope", "user"}}
	wantRollback := CommandPlan{Program: "claude", Args: []string{"plugin", "uninstall", "codex@openai-codex", "--scope", "user", "--keep-data"}}
	if !reflect.DeepEqual(*plan.Command, wantInstall) || !reflect.DeepEqual(*plan.Rollback, wantRollback) {
		t.Fatalf("commands = %#v / %#v", plan.Command, plan.Rollback)
	}

	runner := &recordingCommandRunner{}
	if _, err := ExecuteMarketplaceInstallPlan(context.Background(), plan, []MarketplaceSource{source}, runner); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runner.commands, []CommandPlan{wantInstall}) {
		t.Fatalf("install commands = %#v", runner.commands)
	}

	installed := source
	installed.Entries = append([]MarketplaceEntry(nil), source.Entries...)
	installed.Entries[0].Installed = true
	installed.Entries[0].Status = "installed"
	installed.Entries[0].Actions = marketplaceEntryActions(installed.Entries[0])
	if err := VerifyMarketplaceInstallPlan(plan, []MarketplaceSource{installed}); err != nil {
		t.Fatal(err)
	}
	if _, err := RollbackMarketplaceInstallPlan(context.Background(), plan, []MarketplaceSource{installed}, runner); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runner.commands, []CommandPlan{wantInstall, wantRollback}) {
		t.Fatalf("full-loop commands = %#v", runner.commands)
	}
	if err := VerifyMarketplaceInstallRollback(plan, []MarketplaceSource{source}); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeMarketplaceInstallRejectsStalePreview(t *testing.T) {
	source, entry := claudeMarketplaceInstallFixture()
	plan := PlanMarketplaceInstall(source, entry)
	changed := source
	changed.Entries = append([]MarketplaceEntry(nil), source.Entries...)
	changed.Entries[0].Version = "2.0.0"
	runner := &recordingCommandRunner{}
	if _, err := ExecuteMarketplaceInstallPlan(context.Background(), plan, []MarketplaceSource{changed}, runner); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("stale error = %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("stale plan executed commands: %#v", runner.commands)
	}
}

func TestClaudeMarketplaceInstallSurfacesUnavailableReasons(t *testing.T) {
	source, entry := claudeMarketplaceInstallFixture()
	entry.Installed = true
	if plan := PlanMarketplaceInstall(source, entry); plan.Available || !strings.Contains(plan.UnavailableReason, "already installed") {
		t.Fatalf("installed plan = %#v", plan)
	}
	entry.Installed = false
	entry.Agent = types.AgentCodex
	if plan := PlanMarketplaceInstall(source, entry); plan.Available || !strings.Contains(plan.UnavailableReason, "Claude Code") {
		t.Fatalf("unsupported provider plan = %#v", plan)
	}
	entry.Agent = types.AgentClaudeCode
	source.Label = "bad marketplace"
	if plan := PlanMarketplaceInstall(source, entry); plan.Available || !strings.Contains(plan.UnavailableReason, "letters") {
		t.Fatalf("unsafe selector plan = %#v", plan)
	}
}

func TestClaudeMarketplaceInstallPropagatesCommandFailure(t *testing.T) {
	source, entry := claudeMarketplaceInstallFixture()
	plan := PlanMarketplaceInstall(source, entry)
	runner := &recordingCommandRunner{err: errors.New("provider failed")}
	if _, err := ExecuteMarketplaceInstallPlan(context.Background(), plan, []MarketplaceSource{source}, runner); err == nil || !strings.Contains(err.Error(), "provider failed") {
		t.Fatalf("execution error = %v", err)
	}
}

func claudeMarketplaceInstallFixture() (MarketplaceSource, MarketplaceEntry) {
	entry := MarketplaceEntry{
		ID: "entry", SourceID: "source", SourceKind: MarketplaceSourceMarketplace,
		Agent: types.AgentClaudeCode, Scope: types.ScopeUser, Name: "codex", Kind: types.KindSkill,
		SourcePath: "~/.claude/plugins/marketplaces/openai-codex/codex/skills/codex", Version: "1.0.5",
	}
	entry.Actions = marketplaceEntryActions(entry)
	source := MarketplaceSource{
		ID: "source", Label: "openai-codex", Kind: MarketplaceSourceMarketplace,
		Agent: types.AgentClaudeCode, Scope: types.ScopeUser, Path: "~/.claude/plugins/marketplaces/openai-codex",
		Entries: []MarketplaceEntry{entry},
	}
	return source, entry
}
