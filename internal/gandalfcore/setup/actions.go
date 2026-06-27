package setup

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

var ErrActionUnavailable = errors.New("setup action unavailable")

type CommandPlan struct {
	Program string
	Args    []string
}

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

type CommandRunner interface {
	Run(ctx context.Context, command CommandPlan) error
}

type ActionResult struct {
	ExecutedCommand bool
	OperationOnly   bool
}

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

func ExecuteActionPlan(ctx context.Context, plan ActionPlan, runner CommandRunner) (ActionResult, error) {
	if !plan.Available {
		return ActionResult{}, fmt.Errorf("%w: %s", ErrActionUnavailable, plan.UnavailableReason)
	}
	if plan.ConfigTarget == "" {
		return ActionResult{}, errors.New("setup action requires a global config target")
	}
	if plan.Command == nil {
		return ActionResult{OperationOnly: true}, nil
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
