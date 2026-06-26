package restore

import (
	"fmt"

	"github.com/qyinm/hem/internal/hemcore/types"
)

// ApplyHandler applies a single restore item of a specific kind.
type ApplyHandler func(item *types.RestoreItem) error

// UndoHandler rolls back a single restore item of a specific kind.
type UndoHandler func(item *types.RestoreItem) error

// ApplyHandlerRegistry maps evidence kinds to apply handlers.
type ApplyHandlerRegistry struct {
	Handlers map[string]ApplyHandler
}

// UndoHandlerRegistry maps evidence kinds to undo handlers.
type UndoHandlerRegistry struct {
	Handlers map[string]UndoHandler
}

var (
	defaultApplyRegistry = ApplyHandlerRegistry{
		Handlers: map[string]ApplyHandler{
			"agent_config":       ApplyAgentConfig,
			"agent_instruction":  ApplyAgentInstruction,
			"hook":               ApplyHook,
			"skill":              ApplySkill,
			"mcp_server":         ApplyMCPServer,
			"permission":         ApplyPermission,
			"env_key":            ApplyEnvKey,
			"env":                ApplyEnv,
		},
	}
	defaultUndoRegistry = UndoHandlerRegistry{
		Handlers: map[string]UndoHandler{
			"agent_config":      RestorePreviousContentUndoHandler,
			"agent_instruction": RestorePreviousContentUndoHandler,
			"mcp_server":        RestorePreviousContentUndoHandler,
			"permission":        RestorePreviousContentUndoHandler,
			"hook":              RestorePreviousContentUndoHandler,
			"skill":             RestorePreviousContentUndoHandler,
			"env_key":           RestorePreviousContentUndoHandler,
			"env":               RestorePreviousContentUndoHandler,
			"symlink":           RestorePreviousContentUndoHandler,
			"unsupported":       NoopUndoHandler,
		},
	}
)

// DefaultApplyHandlerRegistry returns the standard apply handler registry.
func DefaultApplyHandlerRegistry() ApplyHandlerRegistry {
	return defaultApplyRegistry
}

// DispatchDefaultApply routes an item to its registered apply handler.
func DispatchDefaultApply(item *types.RestoreItem) error {
	handler, ok := defaultApplyRegistry.Handlers[item.ItemType]
	if !ok {
		message := fmt.Sprintf("No apply handler for type %q", item.ItemType)
		item.SkipReason = &message
		return fmt.Errorf("%s", message)
	}
	return handler(item)
}

// CreateDefaultApplyExecutor returns the default restore executor.
func CreateDefaultApplyExecutor() RestoreExecutor {
	return DispatchDefaultApply
}

// DefaultUndoHandlerRegistry returns the standard undo handler registry.
func DefaultUndoHandlerRegistry() UndoHandlerRegistry {
	return defaultUndoRegistry
}

// DispatchDefaultUndo routes an item to its registered undo handler.
func DispatchDefaultUndo(item *types.RestoreItem) error {
	if !item.CanRollback || item.ItemType == "unsupported" {
		return nil
	}
	handler, ok := defaultUndoRegistry.Handlers[item.ItemType]
	if !ok {
		return NoopUndoHandler(item)
	}
	return handler(item)
}

// CreateDefaultUndoExecutor returns the default undo executor.
func CreateDefaultUndoExecutor() UndoExecutor {
	return DispatchDefaultUndo
}