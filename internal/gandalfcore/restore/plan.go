package restore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/qyinm/gandalf/internal/gandalfcore/diff"
	"github.com/qyinm/gandalf/internal/gandalfcore/graph"
	"github.com/qyinm/gandalf/internal/gandalfcore/pathconfinement"
	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

var virtualSourcePathPattern = regexp.MustCompile(`^[a-z_]+:`)

type ParseDryRunError struct {
	Message string
}

type ParseDryRunResult struct {
	Items  []types.RestoreItem
	Errors []ParseDryRunError
}

// ParseDryRunOutput parses planner JSON into executable restore items.
func ParseDryRunOutput(input string) ParseDryRunResult {
	var errors []ParseDryRunError

	var cleanedLines []string
	for _, line := range strings.Split(input, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "//") ||
			strings.HasPrefix(trimmed, "---") {
			continue
		}
		cleanedLines = append(cleanedLines, line)
	}
	cleaned := strings.TrimSpace(strings.Join(cleanedLines, "\n"))
	if cleaned == "" {
		return ParseDryRunResult{Items: nil, Errors: errors}
	}

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		errors = append(errors, ParseDryRunError{
			Message: fmt.Sprintf("Failed to parse dry-run output as JSON: %v", err),
		})
		return ParseDryRunResult{Items: nil, Errors: errors}
	}

	var targetProject, targetHome *string
	if raw, ok := parsed["targetProject"]; ok {
		var value string
		if json.Unmarshal(raw, &value) == nil {
			targetProject = &value
		}
	}
	if raw, ok := parsed["targetHome"]; ok {
		var value string
		if json.Unmarshal(raw, &value) == nil {
			targetHome = &value
		}
	}

	rawItems, ok := parsed["items"]
	if !ok {
		errors = append(errors, ParseDryRunError{
			Message: `Dry-run plan is missing required "items" array`,
		})
		return ParseDryRunResult{Items: nil, Errors: errors}
	}

	var planItems []json.RawMessage
	if err := json.Unmarshal(rawItems, &planItems); err != nil {
		errors = append(errors, ParseDryRunError{
			Message: `Dry-run plan is missing required "items" array`,
		})
		return ParseDryRunResult{Items: nil, Errors: errors}
	}

	var executionOrder []string
	if raw, ok := parsed["executionOrder"]; ok {
		_ = json.Unmarshal(raw, &executionOrder)
	}

	var unsupportedItems []types.UnsupportedPlanItem
	if raw, ok := parsed["unsupportedItems"]; ok {
		_ = json.Unmarshal(raw, &unsupportedItems)
	}

	var planItemsList []types.RestorePlanItem
	for _, rawItem := range planItems {
		var planItem types.RestorePlanItem
		if err := json.Unmarshal(rawItem, &planItem); err != nil {
			errors = append(errors, ParseDryRunError{
				Message: "Skipping item: invalid restore plan item structure",
			})
			continue
		}
		planItemsList = append(planItemsList, planItem)
	}

	return buildRestoreItems(targetProject, targetHome, planItemsList, executionOrder, unsupportedItems, errors)
}

// RestoreItemsFromPlan converts a structured restore plan into executable items.
func RestoreItemsFromPlan(plan *types.RestorePlan) ParseDryRunResult {
	return buildRestoreItems(
		&plan.TargetProject,
		&plan.TargetHome,
		plan.Items,
		plan.ExecutionOrder,
		plan.UnsupportedItems,
		nil,
	)
}

func buildRestoreItems(
	targetProject, targetHome *string,
	planItems []types.RestorePlanItem,
	executionOrder []string,
	unsupportedItems []types.UnsupportedPlanItem,
	errors []ParseDryRunError,
) ParseDryRunResult {
	if errors == nil {
		errors = []ParseDryRunError{}
	}

	orderLookup := make(map[string]uint32)
	for index, itemID := range executionOrder {
		orderLookup[itemID] = uint32(index + 1)
	}

	seenIDs := make(map[string]struct{})
	itemsItemIDs := make(map[string]struct{})
	nextAppendOrder := uint32(len(executionOrder) + 1)
	var result []types.RestoreItem

	for _, planItem := range planItems {
		if _, dup := seenIDs[planItem.ItemID]; dup {
			errors = append(errors, ParseDryRunError{
				Message: fmt.Sprintf("Duplicate itemId %q skipped", planItem.ItemID),
			})
			continue
		}
		seenIDs[planItem.ItemID] = struct{}{}
		itemsItemIDs[planItem.ItemID] = struct{}{}

		order, ok := orderLookup[planItem.ItemID]
		if !ok {
			order = nextAppendOrder
			nextAppendOrder++
		}

		canRollback := canRollbackAction(planItem.Action)
		dest := resolvePlanDestination(&planItem, targetProject, targetHome)
		if roots := pathconfinement.RootsFromPaths(targetHome, targetProject); roots != nil {
			if _, err := pathconfinement.ValidateConstrainedWritePath(dest, roots); err != nil {
				errors = append(errors, ParseDryRunError{
					Message: fmt.Sprintf("Skipping item %q: %v", planItem.ItemID, err),
				})
				continue
			}
		}

		action := planItem.Action
		status := types.RestoreItemStatusPending
		if planItem.Action == types.RestoreActionUnsupported {
			status = types.RestoreItemStatusUnsupported
		}

		result = append(result, types.RestoreItem{
			ItemID:         planItem.ItemID,
			Path:           planItem.SourcePath,
			ItemType:       planItem.Kind.String(),
			Source:         planItem.SourcePath,
			Dest:           dest,
			Action:         &action,
			Status:         status,
			ExecutionOrder: order,
			TargetContent:  targetContentForPlanItem(&planItem),
			CanRollback:    canRollback,
			Metadata:       restoreItemMetadata(&planItem),
		})
	}

	for _, unsupported := range unsupportedItems {
		if _, inItems := itemsItemIDs[unsupported.ItemID]; inItems {
			continue
		}
		if _, dup := seenIDs[unsupported.ItemID]; dup {
			errors = append(errors, ParseDryRunError{
				Message: fmt.Sprintf("Duplicate itemId %q skipped", unsupported.ItemID),
			})
			continue
		}
		seenIDs[unsupported.ItemID] = struct{}{}
		unsupportedAction := types.RestoreActionUnsupported
		reason := unsupported.Reason
		result = append(result, types.RestoreItem{
			ItemID:         unsupported.ItemID,
			Path:           unsupported.SourcePath,
			ItemType:       unsupported.Kind.String(),
			Source:         unsupported.SourcePath,
			Dest:           unsupported.SourcePath,
			Action:         &unsupportedAction,
			Status:         types.RestoreItemStatusUnsupported,
			SkipReason:     &reason,
			ExecutionOrder: nextAppendOrder,
			CanRollback:    false,
		})
		nextAppendOrder++
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ExecutionOrder < result[j].ExecutionOrder
	})
	return ParseDryRunResult{Items: result, Errors: errors}
}

// BuildRestorePlan diffs a snapshot against the current scan and produces a restore plan.
func BuildRestorePlan(options *types.RestoreOptions) (*types.RestorePlan, error) {
	snapshot, err := store.ReadSnapshot(options.StoreDir, options.SourceSnapshot, options.Agent)
	if err != nil {
		return nil, err
	}

	scanResult := scan.ScanProject(&types.ScanOptions{
		ProjectPath: options.ProjectPath,
		HomeDir:     options.HomeDir,
		StoreDir:    options.StoreDir,
		Agent:       options.Agent,
		Scope:       options.Scope,
	})
	currentGraph := graph.BuildGraph(scanResult.Evidence)

	snapshotContent, err := snapshotContentByEvidenceID(&snapshot, options)
	if err != nil {
		return nil, err
	}

	graphDiff := diff.DiffGraphs(snapshot.Graph, currentGraph)
	var items []types.RestorePlanItem
	var unsupportedItems []types.UnsupportedPlanItem
	var executionOrder []string
	var riskCounts types.RiskSummary

	for _, change := range graphDiff.SemanticChanges {
		sourcePath := change.Details.SourcePath
		currentState := findMatchingEvidence(&change, scanResult.Evidence, sourcePath)
		targetState := withSnapshotContent(
			findMatchingEvidence(&change, snapshot.Evidence, sourcePath),
			snapshotContent,
		)

		itemID := fmt.Sprintf("%s:%s:%s", change.EntityKind, change.EntityName, shortRandomID())
		restorePath := restorePathFromContent(targetState)
		if restorePath == "" {
			restorePath = restorePathFromContent(currentState)
		}
		if restorePath == "" {
			restorePath = restorePathForEvidenceFile(targetState)
		}
		if restorePath == "" {
			restorePath = restorePathForEvidenceFile(currentState)
		}
		if restorePath == "" {
			restorePath = sourcePathForRestoreItem(
				change.EntityKind,
				change.EntityName,
				currentState,
				targetState,
				sourcePath,
			)
		}

		action := restoreActionForChange(change.Code.String(), currentState, targetState)
		riskLevel := change.Severity

		if action == types.RestoreActionUnsupported || action == types.RestoreActionSkip {
			unsupportedItems = append(unsupportedItems, types.UnsupportedPlanItem{
				ItemID:     itemID,
				Agent:      agentForRestoreItem(currentState, targetState),
				Kind:       change.EntityKind,
				SourcePath: restorePath,
				Reason: unsupportedReasonFor(
					change.Code.String(),
					change.EntityKind,
					change.EntityName,
					currentState,
					targetState,
				),
			})
			continue
		}

		incrementRisk(&riskCounts, riskLevel)
		needsConfirmation := riskLevel == types.SeverityHigh || riskLevel == types.SeverityCritical
		confirmationPrompt := ""
		if needsConfirmation {
			confirmationPrompt = fmt.Sprintf(
				"Restore %s %q with risk %s. Continue?",
				change.EntityKind,
				change.EntityName,
				riskLevel,
			)
		}

		items = append(items, types.RestorePlanItem{
			ItemID:       itemID,
			Agent:        agentForRestoreItem(currentState, targetState),
			Kind:         change.EntityKind,
			SourcePath:   restorePath,
			DependsOn:    nil,
			Action:       action,
			CurrentState: currentState,
			TargetState:  targetState,
			Diff: types.ItemDiff{
				Changes:   change.Details.ChangedFields,
				Additions: nil,
				Removals:  nil,
			},
			RiskLevel:           riskLevel,
			RiskReason:          fmt.Sprintf("Restore %s for %s: %s", action, change.EntityKind, change.EntityName),
			NeedsConfirmation:   needsConfirmation,
			ConfirmationPrompt:  confirmationPrompt,
			RollbackInstruction: rollbackInstructionFor(action, change.EntityKind, change.EntityName),
		})
		executionOrder = append(executionOrder, itemID)
	}
	unsupportedItems = omitUnsupportedMCPChangesCoveredByAgentConfig(unsupportedItems, items)

	return &types.RestorePlan{
		PlanID:         "plan-" + longerRandomID(),
		SourceSnapshot: options.SourceSnapshot,
		TargetProject:  options.ProjectPath,
		TargetHome:     options.HomeDir,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		ItemCount:      uint32(len(items)),
		RiskSummary:    riskCounts,
		Items:          items,
		RollbackPlan: types.RollbackPlan{
			Steps: buildRollbackSteps(items),
		},
		ExecutionOrder:   executionOrder,
		UnsupportedItems: unsupportedItems,
		PlanMetadata: types.RestorePlanMetadata{
			PlannerVersion: "0.2.0",
			GeneratedBy:    "gandalf restore",
		},
	}, nil
}

func omitUnsupportedMCPChangesCoveredByAgentConfig(
	unsupported []types.UnsupportedPlanItem,
	items []types.RestorePlanItem,
) []types.UnsupportedPlanItem {
	coveredConfigPaths := make(map[string]struct{})
	for i := range items {
		item := &items[i]
		if item.Kind != types.KindAgentConfig {
			continue
		}
		if item.Action != types.RestoreActionDelete && len(targetContentForPlanItem(item)) == 0 {
			continue
		}
		coveredConfigPaths[restoreCoverageKey(item.Agent, item.SourcePath)] = struct{}{}
	}

	filtered := make([]types.UnsupportedPlanItem, 0, len(unsupported))
	for _, item := range unsupported {
		if item.Kind == types.KindMcpServer {
			if _, covered := coveredConfigPaths[restoreCoverageKey(item.Agent, item.SourcePath)]; covered {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func restoreCoverageKey(agent types.AgentID, sourcePath string) string {
	return agent.String() + "\x00" + filepath.Clean(strings.TrimSpace(sourcePath))
}

func snapshotContentByEvidenceID(
	snapshot *types.Snapshot,
	options *types.RestoreOptions,
) (map[string]types.SnapshotContentEntry, error) {
	content := make(map[string]types.SnapshotContentEntry)
	if snapshot.Content == nil {
		return content, nil
	}
	for _, entry := range snapshot.Content {
		if entry.CaptureStatus != "captured" {
			continue
		}
		text, err := store.ReadSnapshotContent(
			options.StoreDir,
			options.SourceSnapshot,
			entry,
			options.Agent,
		)
		if err != nil {
			return nil, err
		}
		withContent := entry
		withContent.Content = &text
		content[withContent.EvidenceID] = withContent
	}
	return content, nil
}

func targetContentForPlanItem(planItem *types.RestorePlanItem) json.RawMessage {
	if planItem.TargetState == nil || len(planItem.TargetState.Value) == 0 {
		return nil
	}
	switch planItem.Kind {
	case types.KindMcpServer, types.KindPermission, types.KindEnvKey:
		return planItem.TargetState.Value
	default:
		var value string
		if json.Unmarshal(planItem.TargetState.Value, &value) == nil {
			return planItem.TargetState.Value
		}
		return nil
	}
}

func withSnapshotContent(
	item *types.DiscoveredItem,
	content map[string]types.SnapshotContentEntry,
) *types.DiscoveredItem {
	if item == nil {
		return nil
	}
	entry, ok := content[item.ID]
	if !ok {
		return item
	}

	cloned := *item
	metadata := map[string]any{}
	if len(item.Metadata) > 0 {
		_ = json.Unmarshal(item.Metadata, &metadata)
	}
	metadata["contentCaptureStatus"] = "captured"
	metadata["contentRestorePath"] = entry.RestorePath
	metadata["contentChecksum"] = entry.Checksum
	metaBytes, _ := json.Marshal(metadata)
	cloned.Metadata = metaBytes
	if entry.Content != nil {
		value, _ := json.Marshal(*entry.Content)
		cloned.Value = value
	}
	return &cloned
}

func restorePathFromContent(item *types.DiscoveredItem) string {
	if item == nil || len(item.Metadata) == 0 {
		return ""
	}
	var metadata map[string]any
	if json.Unmarshal(item.Metadata, &metadata) != nil {
		return ""
	}
	restorePath, _ := metadata["contentRestorePath"].(string)
	if restorePath == "" {
		return ""
	}
	return restorePath
}

func restorePathForEvidenceFile(item *types.DiscoveredItem) string {
	if item == nil {
		return ""
	}
	if item.Kind == types.KindSkill {
		entrypoint := "SKILL.md"
		if len(item.Metadata) > 0 {
			var metadata map[string]any
			if json.Unmarshal(item.Metadata, &metadata) == nil {
				if ep, ok := metadata["entrypoint"].(string); ok && ep != "" {
					entrypoint = ep
				}
			}
		}
		return item.SourcePath + "/" + entrypoint
	}
	if strings.HasPrefix(item.SourcePath, "~/") ||
		strings.HasPrefix(item.SourcePath, ".") ||
		filepath.IsAbs(item.SourcePath) {
		return item.SourcePath
	}
	return ""
}

func agentForRestoreItem(currentState, targetState *types.DiscoveredItem) types.AgentID {
	if targetState != nil {
		return targetState.Agent
	}
	if currentState != nil {
		return currentState.Agent
	}
	return types.AgentUnknown
}

func unsupportedReasonFor(
	code string,
	kind types.EvidenceKind,
	name string,
	currentState, targetState *types.DiscoveredItem,
) string {
	if currentState == nil && targetState == nil {
		return fmt.Sprintf("Cannot map %s for %s %s to captured evidence", code, kind, name)
	}
	if targetState != nil && len(targetState.Metadata) > 0 {
		var metadata map[string]any
		if json.Unmarshal(targetState.Metadata, &metadata) == nil {
			if status, _ := metadata["contentCaptureStatus"].(string); status == "omitted" {
				reason, _ := metadata["contentCaptureReason"].(string)
				if reason == "" {
					reason = "policy"
				}
				return fmt.Sprintf("Snapshot content for %s %s was omitted: %s", kind, name, reason)
			}
		}
	}
	if kind == types.KindEnvKey {
		return fmt.Sprintf(
			"Environment key values are key-inventory-only; %s cannot be restored without a user-supplied value",
			code,
		)
	}
	if code == "UNSUPPORTED_STATE_CHANGED" {
		return fmt.Sprintf("Unsupported state change: %s %s", kind, name)
	}
	return fmt.Sprintf("No supported restore action for %s on %s %s", code, kind, name)
}

func canRollbackAction(action types.RestoreAction) bool {
	return action == types.RestoreActionCreate ||
		action == types.RestoreActionUpdate ||
		action == types.RestoreActionDelete
}

func buildRollbackSteps(items []types.RestorePlanItem) []types.RollbackStep {
	var steps []types.RollbackStep
	for _, item := range items {
		if !canRollbackAction(item.Action) {
			continue
		}
		steps = append(steps, types.RollbackStep{
			ItemID:      item.ItemID,
			Action:      rollbackActionFor(item.Action),
			Instruction: item.RollbackInstruction,
		})
	}
	return steps
}

func rollbackActionFor(action types.RestoreAction) string {
	switch action {
	case types.RestoreActionCreate:
		return "delete"
	case types.RestoreActionDelete:
		return "create"
	default:
		return "revert"
	}
}

func restoreActionForChange(
	code string,
	currentState, targetState *types.DiscoveredItem,
) types.RestoreAction {
	switch code {
	case "AGENT_CONFIG_ADDED", "SKILL_ADDED", "HOOK_ADDED":
		if currentState != nil {
			return types.RestoreActionDelete
		}
		return types.RestoreActionUnsupported
	case "MCP_ADDED":
		if currentState != nil && isJSONMCPState(currentState) {
			return types.RestoreActionDelete
		}
		return types.RestoreActionUnsupported
	case "AGENT_CONFIG_REMOVED", "SKILL_REMOVED", "HOOK_REMOVED":
		if targetState != nil {
			return types.RestoreActionCreate
		}
		return types.RestoreActionUnsupported
	case "MCP_REMOVED":
		if targetState != nil && isJSONMCPState(targetState) {
			return types.RestoreActionCreate
		}
		return types.RestoreActionUnsupported
	case "AGENT_CONFIG_CHANGED", "HOOK_CHANGED", "PERMISSION_CHANGED",
		"INSTRUCTION_CHANGED", "SKILL_EXECUTABLE_APPEARED":
		if targetState != nil {
			return types.RestoreActionUpdate
		}
		return types.RestoreActionUnsupported
	case "MCP_CHANGED":
		if targetState != nil && isJSONMCPState(targetState) {
			return types.RestoreActionUpdate
		}
		return types.RestoreActionUnsupported
	case "ENV_KEY_ADDED":
		if currentState != nil {
			return types.RestoreActionDelete
		}
		return types.RestoreActionUnsupported
	case "ENV_KEY_REMOVED", "UNSUPPORTED_STATE_CHANGED":
		return types.RestoreActionUnsupported
	default:
		return types.RestoreActionUnsupported
	}
}

func isJSONMCPState(item *types.DiscoveredItem) bool {
	return strings.HasSuffix(item.SourcePath, ".mcp.json") ||
		strings.HasSuffix(item.SourcePath, "/mcp.json")
}

func rollbackInstructionFor(action types.RestoreAction, kind types.EvidenceKind, name string) string {
	switch action {
	case types.RestoreActionDelete:
		return fmt.Sprintf("Recreate deleted %s: %s", kind, name)
	case types.RestoreActionCreate:
		return fmt.Sprintf("Remove created %s: %s", kind, name)
	default:
		return fmt.Sprintf("Reverse %s for %s: %s", action, kind, name)
	}
}

func sourcePathForRestoreItem(
	kind types.EvidenceKind,
	name string,
	currentState, targetState *types.DiscoveredItem,
	diffSourcePath *string,
) string {
	if targetState != nil {
		return targetState.SourcePath
	}
	if currentState != nil {
		return currentState.SourcePath
	}
	if diffSourcePath != nil {
		return *diffSourcePath
	}
	return resolveSourcePathByKind(kind, name)
}

func isVirtualSourcePath(sourcePath string) bool {
	return virtualSourcePathPattern.MatchString(sourcePath)
}

func resolvePlanDestination(
	planItem *types.RestorePlanItem,
	targetProject, targetHome *string,
) string {
	if targetHome != nil {
		if planItem.SourcePath == "~" {
			return *targetHome
		}
		if rest, ok := strings.CutPrefix(planItem.SourcePath, "~/"); ok {
			return filepath.Join(*targetHome, rest)
		}
	}
	if targetProject == nil ||
		filepath.IsAbs(planItem.SourcePath) ||
		isVirtualSourcePath(planItem.SourcePath) {
		return planItem.SourcePath
	}
	return filepath.Join(*targetProject, planItem.SourcePath)
}

func restoreItemMetadata(planItem *types.RestorePlanItem) json.RawMessage {
	metadata := make(map[string]any)
	if restorePath := restorePathFromContent(planItem.TargetState); restorePath != "" {
		metadata["restorePath"] = restorePath
	} else if restorePath := restorePathFromContent(planItem.CurrentState); restorePath != "" {
		metadata["restorePath"] = restorePath
	}

	if planItem.Kind == types.KindMcpServer {
		var serverName *string
		if planItem.TargetState != nil {
			serverName = planItem.TargetState.Name
		} else if planItem.CurrentState != nil {
			serverName = planItem.CurrentState.Name
		}
		if serverName != nil {
			metadata["serverName"] = *serverName
		}
		metadata["sourcePath"] = planItem.SourcePath
		if strings.HasSuffix(planItem.SourcePath, ".mcp.json") ||
			strings.HasSuffix(planItem.SourcePath, "/mcp.json") {
			metadata["mcpPath"] = planItem.SourcePath
		}
	}

	if planItem.Kind == types.KindPermission {
		if name := permissionNameFromState(planItem.TargetState); name != "" {
			metadata["permissionName"] = name
		} else if name := permissionNameFromState(planItem.CurrentState); name != "" {
			metadata["permissionName"] = name
		}
	}

	if len(metadata) == 0 {
		return nil
	}
	raw, _ := json.Marshal(metadata)
	return raw
}

func permissionNameFromState(state *types.DiscoveredItem) string {
	if state == nil {
		return ""
	}
	if len(state.Metadata) > 0 {
		var metadata map[string]any
		if json.Unmarshal(state.Metadata, &metadata) == nil {
			if key, ok := metadata["permissionKey"].(string); ok && key != "" {
				return key
			}
		}
	}
	return permissionNameFromEvidenceID(state.ID)
}

func permissionNameFromEvidenceID(id string) string {
	const marker = ".perm-"
	index := strings.LastIndex(id, marker)
	if index < 0 {
		return ""
	}
	suffix := id[index+len(marker):]
	name, _, _ := strings.Cut(suffix, ":")
	if name == "" {
		return ""
	}
	return name
}

func resolveSourcePathByKind(kind types.EvidenceKind, name string) string {
	switch kind {
	case types.KindMcpServer:
		return fmt.Sprintf(".mcp.json (%s)", name)
	case types.KindEnvKey:
		return fmt.Sprintf("env:%s", name)
	case types.KindAgentConfig:
		return fmt.Sprintf("config:%s", name)
	case types.KindAgentInstruction:
		return fmt.Sprintf("instruction:%s", name)
	case types.KindSkill:
		return fmt.Sprintf("skill:%s", name)
	case types.KindPermission:
		return fmt.Sprintf("permission:%s", name)
	case types.KindHook:
		return fmt.Sprintf("hook:%s", name)
	default:
		return fmt.Sprintf("unknown:%s", name)
	}
}

func findMatchingEvidence(
	change *diff.SemanticChange,
	evidence []types.DiscoveredItem,
	sourcePath *string,
) *types.DiscoveredItem {
	if sourcePath != nil {
		for i := range evidence {
			item := &evidence[i]
			if item.Kind == change.EntityKind &&
				item.Name != nil && *item.Name == change.EntityName &&
				item.SourcePath == *sourcePath {
				return item
			}
		}
		for i := range evidence {
			item := &evidence[i]
			if item.Kind == change.EntityKind && item.SourcePath == *sourcePath {
				return item
			}
		}
	}
	for i := range evidence {
		item := &evidence[i]
		if item.Kind == change.EntityKind &&
			item.Name != nil && *item.Name == change.EntityName {
			return item
		}
	}
	for i := range evidence {
		item := &evidence[i]
		if item.Kind == change.EntityKind {
			return item
		}
	}
	return nil
}

func incrementRisk(summary *types.RiskSummary, severity types.Severity) {
	switch severity {
	case types.SeverityNone:
		summary.None++
	case types.SeverityLow:
		summary.Low++
	case types.SeverityMedium:
		summary.Medium++
	case types.SeverityHigh:
		summary.High++
	case types.SeverityCritical:
		summary.Critical++
	}
}

func shortRandomID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func longerRandomID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
