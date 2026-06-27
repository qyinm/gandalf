package setup

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// ErrActionUnavailable marks setup actions that cannot currently be executed.
var ErrActionUnavailable = errors.New("setup action unavailable")

// CommandPlan describes an executable command-backed setup action.
type CommandPlan struct {
	Program string
	Args    []string
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
	default:
		return unavailablePlan(plan, "unknown setup action")
	}
	plan.Available = true
	return plan
}

// ExecuteActionPlan runs a concrete setup action plan.
func ExecuteActionPlan(ctx context.Context, plan ActionPlan, runner CommandRunner) (ActionResult, error) {
	if !plan.Available {
		return ActionResult{}, fmt.Errorf("%w: %s", ErrActionUnavailable, plan.UnavailableReason)
	}
	if plan.ConfigTarget == "" {
		return ActionResult{}, errors.New("setup action requires a global config target")
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
