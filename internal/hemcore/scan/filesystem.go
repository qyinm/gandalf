package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qyinm/hem/internal/hemcore/parsers"
	"github.com/qyinm/hem/internal/hemcore/policy"
	"github.com/qyinm/hem/internal/hemcore/types"
)

// ScanTargets scans multiple targets and returns combined evidence.
func ScanTargets(targets []ScanTarget) []types.DiscoveredItem {
	var evidence []types.DiscoveredItem
	for _, target := range targets {
		evidence = append(evidence, ScanOneTarget(target)...)
	}
	return evidence
}

// ScanOneTarget scans a single filesystem target.
func ScanOneTarget(target ScanTarget) []types.DiscoveredItem {
	metadata, err := os.Lstat(target.AbsolutePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []types.DiscoveredItem{
			baseItem(target, types.CaptureUnsupported, map[string]any{"error": err.Error()}, nil),
		}
	}

	if metadata.Mode()&os.ModeSymlink != 0 {
		return []types.DiscoveredItem{{
			ID:            itemID(target, "symlink"),
			Agent:         target.Agent,
			Kind:          types.KindSymlink,
			SourcePath:    target.SourcePath,
			Scope:         target.Scope,
			Precedence:    target.Precedence,
			Parser:        types.ParserFilesystem,
			Sensitivity:   target.Sensitivity,
			ContentPolicy: "metadata_only",
			RestorePolicy: policy.RestorePolicyFor(types.KindSymlink),
			CaptureStatus: types.CaptureOmitted,
			Confidence:    types.ConfidenceHigh,
			Metadata:      marshalRaw(map[string]any{"reason": "symlink_not_followed"}),
		}}
	}

	if target.Directory {
		if !metadata.IsDir() {
			return nil
		}
		return scanDirectory(target)
	}

	if !metadata.Mode().IsRegular() {
		return nil
	}

	if metadata.Size() > policy.MaxFileBytes {
		return []types.DiscoveredItem{
			baseItem(target, types.CaptureUnsupported, map[string]any{
				"reason":    "file_too_large",
				"sizeBytes": metadata.Size(),
			}, nil),
		}
	}

	if target.MetadataOnly {
		return []types.DiscoveredItem{
			baseItem(target, types.CaptureCaptured, map[string]any{
				"present":   true,
				"sizeBytes": metadata.Size(),
			}, nil),
		}
	}

	text, err := os.ReadFile(target.AbsolutePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []types.DiscoveredItem{
			baseItem(target, types.CaptureUnsupported, map[string]any{"error": err.Error()}, nil),
		}
	}

	if target.Parser == types.ParserDotenv {
		var items []types.DiscoveredItem
		for _, entry := range parsers.ParseDotenvKeys(string(text)) {
			captureStatus := types.CaptureOmitted
			switch entry.CaptureStatus {
			case "redacted":
				captureStatus = types.CaptureRedacted
			case "omitted":
				captureStatus = types.CaptureOmitted
			}
			item := baseItem(target, captureStatus, map[string]any{"secretLike": entry.SecretLike}, nil)
			item.ID = itemID(target, entry.Key)
			item.Name = stringPtr(entry.Key)
			items = append(items, item)
		}
		return items
	}

	parsed := parseTarget(target, string(text))
	if parsed.Err != nil {
		return []types.DiscoveredItem{
			baseItem(target, types.CaptureParseFailed, map[string]any{"error": parsed.Err.Error}, nil),
		}
	}

	if target.Parser == types.ParserJSON && !target.MetadataOnly {
		return emitJSONEvidence(target, parsed.Ok.Value)
	}

	return []types.DiscoveredItem{
		baseItem(target, types.CaptureCaptured, nil, parsed.Ok.Value),
	}
}

func scanDirectory(target ScanTarget) []types.DiscoveredItem {
	if target.Kind == types.KindSkill {
		return ScanSkillDirectory(target)
	}

	captureStatus := types.CaptureCaptured
	if target.Kind == types.KindUnsupported {
		captureStatus = types.CaptureUnsupported
	}

	evidence := []types.DiscoveredItem{
		baseItem(target, captureStatus, map[string]any{"present": true}, nil),
	}
	scanDirectoryEntries(target, target.AbsolutePath, target.SourcePath, &evidence, 0)
	return evidence
}

// ScanSkillDirectory scans a skill root directory for skill subdirectories.
func ScanSkillDirectory(target ScanTarget) []types.DiscoveredItem {
	entries, err := os.ReadDir(target.AbsolutePath)
	if err != nil {
		return nil
	}

	var evidence []types.DiscoveredItem
	limit := policy.MaxDirectoryEntries
	if len(entries) > limit {
		entries = entries[:limit]
	}

	for _, entry := range entries {
		entryName := entry.Name()
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.IsDir() && policy.IgnoredDirectory(entryName) {
			continue
		}

		absolutePath := filepath.Join(target.AbsolutePath, entryName)
		sourcePath := target.SourcePath + "/" + entryName
		childTarget := target
		childTarget.AbsolutePath = absolutePath
		childTarget.SourcePath = sourcePath

		childMeta, err := os.Lstat(absolutePath)
		if err != nil {
			continue
		}

		if childMeta.Mode()&os.ModeSymlink != 0 {
			item := baseItem(childTarget, types.CaptureOmitted, map[string]any{
				"reason":    "symlink_not_followed",
				"skillName": entryName,
			}, nil)
			item.Name = stringPtr(entryName)
			evidence = append(evidence, item)
			continue
		}

		if !childMeta.IsDir() {
			continue
		}

		itemMetadata := map[string]any{
			"present":   true,
			"skillName": entryName,
		}
		skillMD := filepath.Join(absolutePath, "SKILL.md")
		entrypointMeta, err := os.Lstat(skillMD)
		if err == nil {
			entrypointStatus := "captured"
			if entrypointMeta.Mode()&os.ModeSymlink != 0 {
				entrypointStatus = "symlink_not_followed"
			}
			itemMetadata["entrypoint"] = "SKILL.md"
			itemMetadata["entrypointStatus"] = entrypointStatus
			if entrypointMeta.Mode().IsRegular() {
				itemMetadata["entrypointSizeBytes"] = entrypointMeta.Size()
			}
		} else if !os.IsNotExist(err) {
			itemMetadata["entrypointStatus"] = "unreadable"
			itemMetadata["entrypointError"] = err.Error()
		}

		item := baseItem(childTarget, types.CaptureCaptured, itemMetadata, nil)
		item.Name = stringPtr(entryName)
		evidence = append(evidence, item)
	}

	return evidence
}

func scanDirectoryEntries(
	target ScanTarget,
	absoluteDir string,
	sourceDir string,
	evidence *[]types.DiscoveredItem,
	depth uint32,
) {
	if depth >= policy.MaxDirectoryDepth {
		return
	}

	entries, err := os.ReadDir(absoluteDir)
	if err != nil {
		return
	}
	limit := policy.MaxDirectoryEntries
	if len(entries) > limit {
		entries = entries[:limit]
	}

	for _, entry := range entries {
		entryName := entry.Name()
		absolutePath := filepath.Join(absoluteDir, entryName)
		sourcePath := sourceDir + "/" + entryName
		childTarget := target
		childTarget.AbsolutePath = absolutePath
		childTarget.SourcePath = sourcePath

		childMeta, err := os.Lstat(absolutePath)
		if err != nil {
			continue
		}

		if childMeta.Mode()&os.ModeSymlink != 0 {
			*evidence = append(*evidence, types.DiscoveredItem{
				ID:            itemID(childTarget, "symlink"),
				Agent:         childTarget.Agent,
				Kind:          types.KindSymlink,
				SourcePath:    childTarget.SourcePath,
				Scope:         childTarget.Scope,
				Precedence:    childTarget.Precedence,
				Parser:        types.ParserFilesystem,
				Sensitivity:   childTarget.Sensitivity,
				ContentPolicy: childTarget.ContentPolicy,
				RestorePolicy: policy.RestorePolicyFor(types.KindSymlink),
				CaptureStatus: types.CaptureOmitted,
				Confidence:    types.ConfidenceHigh,
				Metadata:      marshalRaw(map[string]any{"reason": "symlink_not_followed"}),
			})
			continue
		}

		childCapture := types.CaptureUnsupported
		if target.Kind == types.KindSkill {
			childCapture = types.CaptureCaptured
		}

		if childMeta.IsDir() {
			*evidence = append(*evidence, baseItem(childTarget, childCapture, map[string]any{"present": true}, nil))
			scanDirectoryEntries(target, absolutePath, sourcePath, evidence, depth+1)
		} else if childMeta.Mode().IsRegular() {
			*evidence = append(*evidence, baseItem(childTarget, childCapture, map[string]any{
				"sizeBytes": childMeta.Size(),
			}, nil))
		}
	}
}

func parseTarget(target ScanTarget, text string) parsers.ParseResult {
	switch target.Parser {
	case types.ParserJSON:
		return parsers.ParseJSON(text)
	case types.ParserToml:
		return parsers.ParseTOMLKeyValues(text)
	case types.ParserMarkdown:
		return parsers.ParseMarkdown(text)
	default:
		return parsers.ParseResult{Ok: &parsers.ParseSuccess{Value: map[string]any{"present": true}}}
	}
}

func emitJSONEvidence(target ScanTarget, value any) []types.DiscoveredItem {
	if strings.HasSuffix(target.SourcePath, ".mcp.json") || strings.HasSuffix(target.SourcePath, "/mcp.json") {
		if servers := mcpServers(value); len(servers) > 0 {
			var evidence []types.DiscoveredItem
			for name, serverValue := range servers {
				serverTarget := target
				serverTarget.Kind = types.KindMcpServer
				serverTarget.Sensitivity = "command_config"
				serverTarget.ContentPolicy = "structured_safe_fields_only"
				item := baseItem(serverTarget, types.CaptureCaptured, nil, serverValue)
				item.ID = itemID(target, "mcp-"+name)
				item.Kind = types.KindMcpServer
				item.Name = stringPtr(name)
				evidence = append(evidence, item)
			}
			return evidence
		}
	}

	var evidence []types.DiscoveredItem
	if strings.HasSuffix(target.SourcePath, "/settings.json") || target.SourcePath == "settings.json" {
		if record, ok := AsRecord(value); ok {
			if perms, ok := AsRecord(record["permissions"]); ok {
				for permName, permRule := range perms {
					permTarget := target
					permTarget.Kind = types.KindPermission
					permTarget.Sensitivity = "command_config"
					permTarget.ContentPolicy = "structured_safe_fields_only"
					item := baseItem(permTarget, types.CaptureCaptured, map[string]any{
						"permissionKey": permName,
					}, map[string]any{"rule": permRule})
					item.ID = itemID(target, "perm-"+permName)
					item.Kind = types.KindPermission
					item.Name = stringPtr(ValueToJSString(permRule))
					evidence = append(evidence, item)
				}
			}

			if hooks, ok := AsRecord(record["hooks"]); ok {
				for eventName, eventHooksValue := range hooks {
					eventHooks, ok := eventHooksValue.([]any)
					if !ok {
						continue
					}
					for i, eventHookValue := range eventHooks {
						entry, ok := AsRecord(eventHookValue)
						if !ok {
							continue
						}
						matcher := "*"
						if m, ok := entry["matcher"].(string); ok {
							matcher = m
						}
						nestedHooks, ok := entry["hooks"].([]any)
						if !ok {
							continue
						}
						for _, hookValue := range nestedHooks {
							hookRecord, ok := AsRecord(hookValue)
							if !ok {
								continue
							}
							command, ok := hookRecord["command"].(string)
							if !ok {
								continue
							}
							hookTarget := target
							hookTarget.Kind = types.KindHook
							hookTarget.Sensitivity = "command_config"
							hookTarget.ContentPolicy = "structured_safe_fields_only"
							item := baseItem(hookTarget, types.CaptureCaptured, map[string]any{
								"executable": true,
								"eventName":  eventName,
								"matcher":    matcher,
								"command":    command,
							}, hookRecord)
							item.ID = itemID(target, fmt.Sprintf("hook-%s-%d", eventName, i))
							item.Kind = types.KindHook
							item.Name = stringPtr(eventName + "." + matcher)
							evidence = append(evidence, item)
						}
					}
				}
			}
		}
	}

	evidence = append(evidence, baseItem(target, types.CaptureCaptured, nil, value))
	return evidence
}

func mcpServers(value any) map[string]any {
	record, ok := AsRecord(value)
	if !ok {
		return nil
	}
	serversValue, ok := record["mcpServers"]
	if !ok {
		return nil
	}
	servers, ok := AsRecord(serversValue)
	if !ok {
		return nil
	}
	return servers
}

func baseItem(
	target ScanTarget,
	captureStatus types.CaptureStatus,
	metadata any,
	value any,
) types.DiscoveredItem {
	return types.DiscoveredItem{
		ID:            itemID(target, string(target.Kind)),
		Agent:         target.Agent,
		Kind:          target.Kind,
		SourcePath:    target.SourcePath,
		Scope:         target.Scope,
		Precedence:    target.Precedence,
		Parser:        target.Parser,
		Sensitivity:   target.Sensitivity,
		ContentPolicy: target.ContentPolicy,
		RestorePolicy: policy.RestorePolicyFor(target.Kind),
		CaptureStatus: captureStatus,
		Confidence:    types.ConfidenceHigh,
		Value:         marshalRaw(value),
		Metadata:      marshalRaw(metadata),
	}
}

func itemID(target ScanTarget, suffix string) string {
	return ScannerItemID(target.Scope, target.Agent, target.SourcePath, suffix)
}

func resolvePath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			return path
		}
		return abs
	}
	return resolved
}

