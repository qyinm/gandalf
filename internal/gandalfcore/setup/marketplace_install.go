package setup

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

var claudePluginSelectorPart = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// MarketplaceInstallContext pins the source identity used to preview and
// revalidate a Claude Code marketplace install.
type MarketplaceInstallContext struct {
	SourceID    string
	SourceLabel string
	SourceKind  MarketplaceSourceKind
	SourcePath  string
	EntryID     string
	EntryPath   string
	Selector    string
	Version     string
}

// PlanMarketplaceInstall previews a provider-backed Claude Code plugin install
// and its exact compensating rollback command.
func PlanMarketplaceInstall(source MarketplaceSource, entry MarketplaceEntry) ActionPlan {
	plan := ActionPlan{
		ID:           strings.Join([]string{entry.ID, string(ActionAdd)}, ":"),
		Action:       ActionAdd,
		Agent:        entry.Agent,
		ObjectKind:   ObjectPlugin,
		TargetName:   entry.Name,
		Operation:    "install Claude Code plugin from configured marketplace",
		ConfigTarget: "~/.claude/plugins/installed_plugins.json",
	}
	if reason := marketplaceInstallUnavailableReason(entry); reason != "" {
		return unavailablePlan(plan, reason)
	}
	if source.ID != entry.SourceID || source.Agent != entry.Agent || source.Kind != MarketplaceSourceMarketplace || source.Scope != types.ScopeUser {
		return unavailablePlan(plan, "install requires the matching user-scope Claude Code marketplace source")
	}
	name := strings.TrimSpace(entry.Name)
	marketplace := strings.TrimSpace(source.Label)
	if !claudePluginSelectorPart.MatchString(name) || !claudePluginSelectorPart.MatchString(marketplace) {
		return unavailablePlan(plan, "plugin and marketplace names must contain only letters, numbers, dot, underscore, or hyphen")
	}
	selector := name + "@" + marketplace
	plan.Command = &CommandPlan{Program: "claude", Args: []string{"plugin", "install", selector, "--scope", "user"}}
	plan.Rollback = &CommandPlan{Program: "claude", Args: []string{"plugin", "uninstall", selector, "--scope", "user", "--keep-data"}}
	plan.MarketplaceInstall = &MarketplaceInstallContext{
		SourceID: source.ID, SourceLabel: marketplace, SourceKind: source.Kind, SourcePath: source.Path,
		EntryID: entry.ID, EntryPath: entry.SourcePath, Selector: selector, Version: entry.Version,
	}
	plan.Available = true
	return plan
}

// ExecuteMarketplaceInstallPlan re-plans from fresh scan data before executing.
func ExecuteMarketplaceInstallPlan(ctx context.Context, plan ActionPlan, fresh []MarketplaceSource, runner CommandRunner) (ActionResult, error) {
	source, entry, err := freshMarketplaceInstallEntry(plan, fresh)
	if err != nil {
		return ActionResult{}, err
	}
	next := PlanMarketplaceInstall(source, entry)
	if !sameMarketplaceInstallPlan(plan, next) {
		return ActionResult{}, errors.New("stale marketplace install: preview no longer matches current source data")
	}
	return ExecuteActionPlan(ctx, next, runner)
}

// VerifyMarketplaceInstallPlan confirms a rescan observes the plugin installed.
func VerifyMarketplaceInstallPlan(plan ActionPlan, fresh []MarketplaceSource) error {
	_, entry, err := freshMarketplaceInstallEntry(plan, fresh)
	if err != nil {
		return err
	}
	if !entry.Installed {
		return errors.New("marketplace install verification failed: plugin is still reported as available")
	}
	return nil
}

// RollbackMarketplaceInstallPlan removes only the plugin pinned by a successful
// install preview, using the previewed compensating command.
func RollbackMarketplaceInstallPlan(ctx context.Context, plan ActionPlan, fresh []MarketplaceSource, runner CommandRunner) (ActionResult, error) {
	_, _, err := freshMarketplaceInstallEntry(plan, fresh)
	if err != nil {
		return ActionResult{}, err
	}
	if plan.Rollback == nil || runner == nil {
		return ActionResult{}, errors.New("marketplace install rollback requires a command and runner")
	}
	if err := runner.Run(ctx, *plan.Rollback); err != nil {
		return ActionResult{}, err
	}
	return ActionResult{ExecutedCommand: true}, nil
}

// VerifyMarketplaceInstallRollback confirms a rescan no longer reports the plugin installed.
func VerifyMarketplaceInstallRollback(plan ActionPlan, fresh []MarketplaceSource) error {
	_, entry, err := freshMarketplaceInstallEntry(plan, fresh)
	if err != nil {
		return err
	}
	if entry.Installed {
		return errors.New("marketplace install rollback verification failed: plugin is still installed")
	}
	return nil
}

func freshMarketplaceInstallEntry(plan ActionPlan, fresh []MarketplaceSource) (MarketplaceSource, MarketplaceEntry, error) {
	if !plan.Available || plan.MarketplaceInstall == nil {
		return MarketplaceSource{}, MarketplaceEntry{}, fmt.Errorf("%w: marketplace install context is missing", ErrActionUnavailable)
	}
	source, entry, ok := findMarketplaceEntry(fresh, plan.MarketplaceInstall.SourceID, plan.MarketplaceInstall.EntryID)
	if !ok {
		return MarketplaceSource{}, MarketplaceEntry{}, errors.New("stale marketplace install: source entry no longer exists")
	}
	context := plan.MarketplaceInstall
	if source.Label != context.SourceLabel || source.Kind != context.SourceKind || source.Path != context.SourcePath ||
		entry.SourcePath != context.EntryPath || entry.Version != context.Version || entry.Name+"@"+source.Label != context.Selector {
		return MarketplaceSource{}, MarketplaceEntry{}, errors.New("stale marketplace install: source entry changed")
	}
	return source, entry, nil
}

func sameMarketplaceInstallPlan(left, right ActionPlan) bool {
	if !right.Available || left.MarketplaceInstall == nil || right.MarketplaceInstall == nil || left.Command == nil || right.Command == nil || left.Rollback == nil || right.Rollback == nil {
		return false
	}
	return *left.MarketplaceInstall == *right.MarketplaceInstall && sameCommand(*left.Command, *right.Command) && sameCommand(*left.Rollback, *right.Rollback)
}

func sameCommand(left, right CommandPlan) bool {
	if left.Program != right.Program || len(left.Args) != len(right.Args) {
		return false
	}
	for index := range left.Args {
		if left.Args[index] != right.Args[index] {
			return false
		}
	}
	return true
}
