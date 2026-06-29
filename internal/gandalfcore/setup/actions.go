package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/fsutil"
	"github.com/qyinm/gandalf/internal/gandalfcore/pathconfinement"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// ErrActionUnavailable marks setup actions that cannot currently be executed.
var ErrActionUnavailable = errors.New("setup action unavailable")

// CommandPlan describes an executable command-backed setup action.
type CommandPlan struct {
	Program string
	Args    []string
}

// MCPTogglePlan describes the built-in file mutation for enabling/disabling
// JSON-backed MCP servers.
type MCPTogglePlan struct {
	ServerName string
	ConfigPath string
}

// ActionPlan is a concrete setup action proposal for confirmation and execution.
type ActionPlan struct {
	ID                string
	Action            ActionKind
	Agent             types.AgentID
	ObjectKind        ObjectKind
	TargetName        string
	Operation         string
	ConfigTarget      string
	Command           *CommandPlan
	MCPToggle         *MCPTogglePlan
	Available         bool
	UnavailableReason string
}

// CommandRunner executes command-backed setup action plans.
type CommandRunner interface {
	Run(ctx context.Context, command CommandPlan) error
}

// ActionResult reports what an executed setup action changed.
type ActionResult struct {
	ExecutedCommand bool
}

// PlanItemAction builds an executable or unavailable plan for an inventory item action.
func PlanItemAction(item InventoryItem, action ActionKind) ActionPlan {
	plan := ActionPlan{
		ID:           strings.Join([]string{item.ID, string(action)}, ":"),
		Action:       action,
		Agent:        item.Agent,
		ObjectKind:   item.ObjectKind,
		TargetName:   item.Name,
		ConfigTarget: item.SourcePath,
	}

	if item.Scope == types.ScopeProject || item.Agent == types.AgentProject {
		return unavailablePlan(plan, "project-local setup is outside the active product scope")
	}
	if item.Scope != types.ScopeUser && item.Scope != types.ScopeManaged {
		return unavailablePlan(plan, "setup scope is not supported")
	}
	if action == ActionAdd {
		return unavailablePlan(plan, "add is started from the inventory, not an installed item")
	}
	if !inventoryActionAvailable(item, action) {
		return unavailablePlan(plan, unavailableReason(item, action))
	}

	switch action {
	case ActionEdit:
		plan.Operation = "edit global setup target"
	case ActionRemove:
		plan.Operation = "remove global setup object from target"
	case ActionToggle:
		if item.Disabled {
			plan.Operation = "enable MCP server in config"
		} else {
			plan.Operation = "disable MCP server in config"
		}
		plan.MCPToggle = &MCPTogglePlan{ServerName: item.Name, ConfigPath: item.SourcePath}
	default:
		return unavailablePlan(plan, "unknown setup action")
	}
	plan.Available = true
	return plan
}

type actionExecutionOptions struct {
	HomeDir string
}

// ActionExecutionOption configures ExecuteActionPlan.
type ActionExecutionOption func(*actionExecutionOptions)

// WithHomeDir supplies the home root required by built-in file mutations.
func WithHomeDir(homeDir string) ActionExecutionOption {
	return func(options *actionExecutionOptions) {
		options.HomeDir = homeDir
	}
}

// ExecuteActionPlan runs a concrete setup action plan.
func ExecuteActionPlan(ctx context.Context, plan ActionPlan, runner CommandRunner, opts ...ActionExecutionOption) (ActionResult, error) {
	if !plan.Available {
		return ActionResult{}, fmt.Errorf("%w: %s", ErrActionUnavailable, plan.UnavailableReason)
	}
	if plan.ConfigTarget == "" {
		return ActionResult{}, errors.New("setup action requires a global config target")
	}
	var options actionExecutionOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if plan.Action == ActionToggle {
		if plan.MCPToggle == nil {
			return ActionResult{}, errors.New("toggle action requires an MCP toggle plan")
		}
		if strings.TrimSpace(options.HomeDir) == "" {
			return ActionResult{}, errors.New("toggle action requires home directory")
		}
		if _, err := ExecuteMCPToggle(plan, options.HomeDir, plan.MCPToggle.ServerName, plan.MCPToggle.ConfigPath); err != nil {
			return ActionResult{}, err
		}
		return ActionResult{ExecutedCommand: false}, nil
	}
	if plan.Command == nil {
		return ActionResult{}, errors.New("setup action requires an executable command plan")
	}
	if strings.TrimSpace(plan.Command.Program) == "" {
		return ActionResult{}, errors.New("setup action command requires a program")
	}
	if runner == nil {
		return ActionResult{}, errors.New("setup action command requires a runner")
	}
	if err := runner.Run(ctx, *plan.Command); err != nil {
		return ActionResult{}, err
	}
	return ActionResult{ExecutedCommand: true}, nil
}

// ToggleResult reports the new state after an MCP enable/disable toggle.
type ToggleResult struct {
	ServerName string
	Disabled   bool
	ConfigPath string
}

// ExecuteMCPToggle flips the `disabled` flag for an MCP server in its JSON
// config. The write is confined to homeDir using the same guard restore uses.
// homeDir must be the absolute user home directory; serverName is the MCP
// server key; configPath is the source path from the inventory item
// (absolute or "~/"-relative).
func ExecuteMCPToggle(plan ActionPlan, homeDir, serverName, configPath string) (ToggleResult, error) {
	if plan.Action != ActionToggle || !plan.Available {
		return ToggleResult{}, fmt.Errorf("%w: %s", ErrActionUnavailable, plan.UnavailableReason)
	}
	if plan.ObjectKind != ObjectMCPServer {
		return ToggleResult{}, errors.New("toggle is only supported for MCP servers")
	}
	serverName = strings.TrimSpace(serverName)
	if serverName == "" {
		return ToggleResult{}, errors.New("toggle requires an MCP server name")
	}

	resolved, err := resolveConfinedMCPPath(configPath, homeDir)
	if err != nil {
		return ToggleResult{}, err
	}

	raw, err := os.ReadFile(resolved)
	if err != nil {
		return ToggleResult{}, fmt.Errorf("read MCP config: %w", err)
	}
	mode := os.FileMode(0o600)
	if info, err := os.Lstat(resolved); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return ToggleResult{}, fmt.Errorf("refusing to write through symlink destination: %s", resolved)
		}
		mode = info.Mode().Perm()
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		return ToggleResult{}, fmt.Errorf("parse MCP config: %w", err)
	}
	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		return ToggleResult{}, fmt.Errorf("MCP config has no mcpServers object")
	}
	entry, ok := servers[serverName].(map[string]any)
	if !ok {
		return ToggleResult{}, fmt.Errorf("MCP server %q not found in config", serverName)
	}

	nextDisabled := !mcpEntryDisabled(entry)
	if nextDisabled {
		entry["disabled"] = true
	} else {
		delete(entry, "disabled")
		delete(entry, "enabled")
	}
	servers[serverName] = entry

	serialized, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return ToggleResult{}, err
	}
	if err := fsutil.WriteTextAtomically(resolved, string(serialized)+"\n", mode); err != nil {
		return ToggleResult{}, fmt.Errorf("write MCP config: %w", err)
	}
	return ToggleResult{ServerName: serverName, Disabled: nextDisabled, ConfigPath: resolved}, nil
}

func mcpEntryDisabled(entry map[string]any) bool {
	if disabled, ok := entry["disabled"].(bool); ok {
		return disabled
	}
	if enabled, ok := entry["enabled"].(bool); ok {
		return !enabled
	}
	return false
}

func resolveConfinedMCPPath(configPath, homeDir string) (string, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return "", errors.New("toggle requires an MCP config path")
	}
	if strings.HasPrefix(configPath, "~/") {
		if strings.TrimSpace(homeDir) == "" {
			return "", errors.New("home directory is required to resolve MCP config path")
		}
		configPath = strings.TrimPrefix(configPath, "~/")
		configPath = filepath.Join(homeDir, filepath.FromSlash(configPath))
	}
	roots := pathconfinement.RootsFromPaths(&homeDir, nil)
	if roots == nil {
		return "", errors.New("home root is required for path confinement")
	}
	resolved, err := pathconfinement.ValidateConstrainedWritePath(configPath, roots)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func unavailablePlan(plan ActionPlan, reason string) ActionPlan {
	plan.Available = false
	plan.UnavailableReason = reason
	return plan
}

func inventoryActionAvailable(item InventoryItem, action ActionKind) bool {
	for _, availability := range item.Actions {
		if availability.Action == action {
			return availability.Available
		}
	}
	return false
}

func unavailableReason(item InventoryItem, action ActionKind) string {
	for _, availability := range item.Actions {
		if availability.Action == action && availability.Reason != "" {
			return availability.Reason
		}
	}
	return "setup action is not available"
}
