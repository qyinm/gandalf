package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/qyinm/hem/internal/hemcore/diff"
	hemerrors "github.com/qyinm/hem/internal/hemcore/errors"
	"github.com/qyinm/hem/internal/hemcore/snapshot"
	"github.com/qyinm/hem/internal/hemcore/store"
	"github.com/qyinm/hem/internal/hemcore/types"
)

var validAgents = []string{
	"claude-code",
	"codex",
	"cursor",
	"opencode",
	"pi-agent",
	"project",
	"unknown",
}

var validScopes = []string{"user", "project", "managed", "unknown"}

// CommonFlags are shared CLI options across commands.
type CommonFlags struct {
	Project string
	Home    string
	Store   string
	Agent   string
	Scope   string
	JSON    bool
}

func (f *CommonFlags) bindFlags(cmdFlags interface {
	StringVarP(*string, string, string, string, string)
	StringVar(*string, string, string, string)
	BoolVar(*bool, string, bool, string)
}) {
	cmdFlags.StringVarP(&f.Project, "project", "p", ".", "Project directory to scan")
	cmdFlags.StringVar(&f.Home, "home", "", "Home directory (defaults to $HOME)")
	cmdFlags.StringVar(&f.Store, "store", "", "Hem store directory (defaults to ~/.hem or $HEM_STORE)")
	cmdFlags.StringVar(&f.Agent, "agent", "", "Filter by agent")
	cmdFlags.StringVar(&f.Scope, "scope", "", "Filter by evidence scope")
	cmdFlags.BoolVar(&f.JSON, "json", false, "Emit JSON output")
}

func resolveRuntime(flags *CommonFlags) (types.RuntimeOptions, *types.SnapError) {
	homeDir := flags.Home
	if homeDir == "" {
		if envHome := os.Getenv("HOME"); envHome != "" {
			homeDir = envHome
		} else if cwd, err := os.Getwd(); err == nil {
			homeDir = cwd
		} else {
			homeDir = "."
		}
	}

	storeDir := flags.Store
	if storeDir == "" {
		if envStore := os.Getenv("HEM_STORE"); envStore != "" {
			storeDir = envStore
		} else {
			storeDir = store.DefaultStoreDir(homeDir)
		}
	}

	projectPath := flags.Project
	if abs, err := filepath.Abs(projectPath); err == nil {
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			projectPath = resolved
		} else {
			projectPath = abs
		}
	}

	var agent *types.AgentID
	if flags.Agent != "" {
		valid := false
		for _, candidate := range validAgents {
			if candidate == flags.Agent {
				valid = true
				break
			}
		}
		if !valid {
			return types.RuntimeOptions{}, &types.SnapError{
				Code:    "HEM_INVALID_AGENT",
				Problem: fmt.Sprintf("Invalid agent: %q.", flags.Agent),
				Cause:   "An unsupported agent identifier was provided.",
				Fix:     fmt.Sprintf("Valid agents: %s", strings.Join(validAgents, ", ")),
			}
		}
		parsed := types.ParseAgentID(flags.Agent)
		agent = &parsed
	}

	var scope *types.EvidenceScope
	if flags.Scope != "" {
		valid := false
		for _, candidate := range validScopes {
			if candidate == flags.Scope {
				valid = true
				break
			}
		}
		if !valid {
			return types.RuntimeOptions{}, &types.SnapError{
				Code:    "HEM_INVALID_SCOPE",
				Problem: fmt.Sprintf("Invalid scope: %q.", flags.Scope),
				Cause:   "An unsupported evidence scope was provided.",
				Fix:     fmt.Sprintf("Valid scopes: %s", strings.Join(validScopes, ", ")),
			}
		}
		parsed, _ := types.ParseScope(flags.Scope)
		scope = &parsed
	}

	return types.RuntimeOptions{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
		StoreDir:    storeDir,
		Agent:       agent,
		Scope:       scope,
	}, nil
}

func snapshotByRef(reference string, runtime *types.RuntimeOptions) (types.Snapshot, *types.SnapError) {
	if reference == "current" {
		state, err := snapshot.CaptureCurrentState(runtime, "current")
		if err != nil {
			return types.Snapshot{}, &types.SnapError{
				Code:    "HEM_CURRENT_STATE_FAILED",
				Problem: "Failed to capture current state.",
				Cause:   err.Error(),
				Fix:     "Verify project and store paths are accessible.",
			}
		}
		return state.Snapshot, nil
	}

	snap, err := store.ReadSnapshot(runtime.StoreDir, reference, runtime.Agent)
	if err != nil {
		return types.Snapshot{}, &types.SnapError{
			Code:    "HEM_SNAPSHOT_NOT_FOUND",
			Problem: fmt.Sprintf("Snapshot %q not found.", reference),
			Cause:   err.Error(),
			Fix:     "Run `hem snapshot list` to see available snapshots.",
		}
	}
	return snap, nil
}

func writeJSON(w io.Writer, value any) int {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return writeError(w, &types.SnapError{
			Code:    "HEM_JSON_SERIALIZE_FAILED",
			Problem: "Failed to serialize JSON output.",
			Cause:   err.Error(),
			Fix:     "This is an internal error.",
		})
	}
	if _, err := fmt.Fprintf(w, "%s\n", data); err != nil {
		return 1
	}
	return 0
}

func writeStdout(w io.Writer, text string) int {
	if _, err := io.WriteString(w, text); err != nil {
		return 1
	}
	return 0
}

func writeError(w io.Writer, err *types.SnapError) int {
	_, _ = io.WriteString(w, hemerrors.FormatSnapError(*err))
	return 1
}

type exitError struct {
	code int
}

func (e exitError) Error() string {
	return fmt.Sprintf("exit %d", e.code)
}

func errExit(code int) error {
	return exitError{code: code}
}

// IsExitError reports whether err is a CLI exit code wrapper.
func IsExitError(err error) (int, bool) {
	var target exitError
	if errors.As(err, &target) {
		return target.code, true
	}
	return 0, false
}

func notImplementedError(feature string) *types.SnapError {
	return &types.SnapError{
		Code:    "HEM_NOT_IMPLEMENTED",
		Problem: fmt.Sprintf("%s is not implemented in the Go engine yet.", feature),
		Cause:   "This command requires engine modules planned for U8.",
		Fix:     "Use the Rust CLI for this feature until Go parity lands.",
	}
}

func displayAgent(agent types.AgentID) string {
	switch agent {
	case types.AgentClaudeCode:
		return "Claude Code"
	case types.AgentCodex:
		return "Codex"
	case types.AgentCursor:
		return "Cursor"
	case types.AgentProject:
		return "Project"
	default:
		return agent.String()
	}
}

func renderScanText(scan *types.ScanResult) string {
	lines := []string{
		"hem scan",
		"",
		fmt.Sprintf("Read-only: %s", boolYesNo(scan.Trust.ReadOnly)),
		fmt.Sprintf("Network: %s", scan.Trust.Network),
		fmt.Sprintf("Commands executed: %d", len(scan.Trust.CommandsExecuted)),
		fmt.Sprintf("Writes: %s/index only", scan.Trust.StoreWriteLocation),
		"",
		"Detected agents",
	}

	agentSet := make(map[types.AgentID]struct{})
	for _, item := range scan.Evidence {
		agentSet[item.Agent] = struct{}{}
	}
	agents := make([]types.AgentID, 0, len(agentSet))
	for agent := range agentSet {
		agents = append(agents, agent)
	}
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].String() < agents[j].String()
	})

	if len(agents) == 0 {
		lines = append(lines, "  none")
	} else {
		for _, agent := range agents {
			scopeSet := make(map[string]struct{})
			for _, item := range scan.Evidence {
				if item.Agent == agent {
					scopeSet[item.Scope.String()] = struct{}{}
				}
			}
			scopes := make([]string, 0, len(scopeSet))
			for scope := range scopeSet {
				scopes = append(scopes, scope)
			}
			sort.Strings(scopes)
			lines = append(lines, fmt.Sprintf("  %s  %s state found", displayAgent(agent), strings.Join(scopes, " + ")))
		}
	}

	lines = append(lines, "", "Blind spots")
	if len(scan.BlindSpots) == 0 {
		lines = append(lines, "  none")
	} else {
		for _, blindSpot := range scan.BlindSpots {
			lines = append(lines, "  "+blindSpot)
		}
	}

	lines = append(lines, "", "Next", "  hem snapshot create --name baseline --agent codex --scope user --project .")
	return strings.Join(lines, "\n") + "\n"
}

func renderScanExplainText(scan *types.ScanResult) string {
	output := strings.TrimRight(renderScanText(scan), "\n")
	output += "\n\nPaths considered\n"

	pathSet := make(map[string]struct{})
	for _, item := range scan.Evidence {
		pathSet[item.SourcePath] = struct{}{}
	}
	paths := make([]string, 0, len(pathSet))
	for path := range pathSet {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	if len(paths) == 0 {
		output += "  none found\n"
	} else {
		for _, path := range paths {
			output += "  " + path + "\n"
		}
	}
	return output
}

func renderDiffText(graphDiff *diff.GraphDiff) string {
	lines := []string{
		"hem diff",
		"",
		"Semantic changes",
	}
	if len(graphDiff.SemanticChanges) == 0 {
		lines = append(lines, "  none")
	} else {
		for _, change := range graphDiff.SemanticChanges {
			lines = append(lines, fmt.Sprintf(
				"  %s  %s: %s",
				severityLabel(change.Severity),
				change.Code,
				change.EntityName,
			))
		}
	}

	lines = append(lines, "", "Raw source changes")
	if len(graphDiff.RawSourceChanges) == 0 {
		lines = append(lines, "  none")
	} else {
		for _, change := range graphDiff.RawSourceChanges {
			lines = append(lines, fmt.Sprintf("  %s: %s", change.Status, change.SourcePath))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func formatRestorePlanPreview(plan *types.RestorePlan) string {
	lines := []string{
		"hem restore dry-run",
		"",
		fmt.Sprintf("Snapshot: %s", plan.SourceSnapshot),
		fmt.Sprintf("Target project: %s", plan.TargetProject),
		fmt.Sprintf("Target home: %s", plan.TargetHome),
		fmt.Sprintf("Writable changes: %d", len(plan.Items)),
		fmt.Sprintf("Unsupported items: %d", len(plan.UnsupportedItems)),
		fmt.Sprintf("Risk: %s", formatRiskSummary(&plan.RiskSummary)),
		"",
	}

	if len(plan.Items) == 0 && len(plan.UnsupportedItems) == 0 {
		lines = append(lines, "No restore actions needed.")
	}

	if len(plan.Items) > 0 {
		lines = append(lines, "Plan:")
		for index, item := range plan.Items {
			lines = append(lines, formatRestorePlanItem(&item, index+1))
		}
	}

	if len(plan.UnsupportedItems) > 0 {
		lines = append(lines, "", "Unsupported:")
		for index, item := range plan.UnsupportedItems {
			lines = append(lines, fmt.Sprintf(
				"%d. %s at %s\n   reason: %s",
				index+1,
				item.Kind,
				item.SourcePath,
				item.Reason,
			))
		}
	}

	lines = append(lines,
		"",
		"No files were changed.",
		"Use --apply --experimental to apply this plan.",
		"Use --json for the machine-readable restore plan.",
	)
	return strings.Join(lines, "\n") + "\n"
}

func formatRestorePlanItem(item *types.RestorePlanItem, index int) string {
	fields := []string{
		fmt.Sprintf(
			"%d. %s %s at %s",
			index,
			restoreActionLabel(item.Action),
			item.Kind,
			item.SourcePath,
		),
		fmt.Sprintf(
			"   risk: %s%s",
			severityLabel(item.RiskLevel),
			confirmationSuffix(item.NeedsConfirmation),
		),
		fmt.Sprintf("   rollback: %s", item.RollbackInstruction),
	}

	changedFields := append([]string{}, item.Diff.Changes...)
	for _, addition := range item.Diff.Additions {
		changedFields = append(changedFields, "+"+addition)
	}
	for _, removal := range item.Diff.Removals {
		changedFields = append(changedFields, "-"+removal)
	}
	if len(changedFields) > 0 {
		fields = append(fields, fmt.Sprintf("   fields: %s", strings.Join(changedFields, ", ")))
	}
	return strings.Join(fields, "\n")
}

func formatRollbackSummary(summary *types.RollbackSummary) string {
	lines := []string{
		"Rollback complete.",
		"",
		fmt.Sprintf("  Undone:  %d", summary.Undone),
		fmt.Sprintf("  Skipped: %d", summary.Skipped),
		fmt.Sprintf("  Failed:  %d", summary.Failed),
		fmt.Sprintf("  Total:   %d", summary.Total),
	}

	var failures []types.UndoResult
	var skipped []types.UndoResult
	for _, result := range summary.Results {
		switch result.Status {
		case types.UndoStatusFailed:
			failures = append(failures, result)
		case types.UndoStatusSkipped:
			skipped = append(skipped, result)
		}
	}

	if len(failures) > 0 {
		lines = append(lines, "", "Failures:")
		for _, failure := range failures {
			reason := "Unknown error"
			if failure.Reason != nil {
				reason = *failure.Reason
			}
			lines = append(lines, fmt.Sprintf("  [%s] %s", failure.ItemID, reason))
		}
	}

	if len(skipped) > 0 {
		lines = append(lines, "", "Skipped (non-reversible):")
		for _, entry := range skipped {
			reason := "No reason"
			if entry.Reason != nil {
				reason = *entry.Reason
			}
			lines = append(lines, fmt.Sprintf("  [%s] %s", entry.ItemID, reason))
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

func severityLabel(severity types.Severity) string {
	return string(severity)
}

func restoreActionLabel(action types.RestoreAction) string {
	return string(action)
}

func confirmationSuffix(needs bool) string {
	if needs {
		return " (confirmation required)"
	}
	return ""
}

func formatRiskSummary(riskSummary *types.RiskSummary) string {
	counts := []struct {
		label string
		count uint32
	}{
		{"critical", riskSummary.Critical},
		{"high", riskSummary.High},
		{"medium", riskSummary.Medium},
		{"low", riskSummary.Low},
		{"none", riskSummary.None},
	}
	var parts []string
	for _, entry := range counts {
		if entry.count > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", entry.label, entry.count))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func boolYesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func printRootHelp(w io.Writer) {
	help := []string{
		"hem",
		"",
		"Save, compare, and restore Codex user-global setup experiments.",
		"",
		"Diagnosis commands:",
		"  hem scan --project .",
		"  hem scan --project . --explain",
		"  hem snapshot create --name baseline --agent codex --scope user --project .",
		"  hem snapshot create --name baseline --metadata-only --project .",
		"  hem snapshot list",
		"  hem snapshot list --agent codex",
		"  hem snapshot show baseline --json",
		"  hem diff baseline current --agent codex --scope user --project .",
		"  hem diff baseline current --project .",
		"",
		"Restore commands:",
		"  hem restore --snapshot <name> --dry-run --agent codex --scope user --project .",
		"  hem restore --snapshot <name> --apply --experimental --agent codex --scope user --project .",
		"",
		"Extended commands:",
		"  hem doctor --project . [--json]",
		"  hem report [ref] --project . [--out path] [--json]",
		"  hem timeline list --project . [--json]",
		"  hem timeline undo <id> --dry-run --project . [--json]",
		"  hem bundle export --name <snapshot> --out file.hem --project .",
		"  hem bundle import file.hem --dry-run --project . [--json]",
		"  hem bundle verify file.hem",
		"",
		"Interactive workspace:",
		"  hem tui --project .",
	}
	_, _ = io.WriteString(w, strings.Join(help, "\n")+"\n")
}