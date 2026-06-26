package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

const maxSkillDepth = 8

var agentSkillNameValidPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func dedupeSkillsByName(evidence []types.DiscoveredItem) []types.DiscoveredItem {
	result := make([]types.DiscoveredItem, 0, len(evidence))
	indexes := make(map[string]int)
	for _, item := range evidence {
		if item.Kind != types.KindSkill || item.Name == nil {
			result = append(result, item)
			continue
		}
		name := *item.Name
		if existingIndex, ok := indexes[name]; ok {
			existing := result[existingIndex]
			if item.Precedence > existing.Precedence {
				duplicateSources := append([]string{existing.SourcePath}, scan.MetadataStringArray(item.Metadata, "duplicateSources")...)
				result[existingIndex] = mergeDuplicateSources(item, duplicateSources)
			} else {
				duplicateSources := append(scan.MetadataStringArray(existing.Metadata, "duplicateSources"), item.SourcePath)
				result[existingIndex] = mergeDuplicateSources(existing, duplicateSources)
			}
			continue
		}
		indexes[name] = len(result)
		result = append(result, item)
	}
	return result
}

func mergeDuplicateSources(item types.DiscoveredItem, sources []string) types.DiscoveredItem {
	metadata := map[string]any{}
	if len(item.Metadata) > 0 {
		_ = json.Unmarshal(item.Metadata, &metadata)
	}
	metadata["duplicateSources"] = sources
	item.Metadata, _ = json.Marshal(metadata)
	return item
}

func validAgentSkillName(name string) bool {
	return agentSkillNameValidPattern.MatchString(name)
}

func skillEntrypointStatus(root, skillFile string) string {
	rel, err := filepath.Rel(root, skillFile)
	if err != nil {
		return "captured"
	}
	cursor := root
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		cursor = filepath.Join(cursor, part)
		info, err := os.Lstat(cursor)
		if err != nil {
			return "captured"
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if part == "SKILL.md" {
				return "symlink_followed"
			}
			return "symlink_directory_followed"
		}
	}
	return "captured"
}

func readAgentSkillFrontmatter(filePath string) (name, description *string, sizeBytes uint64) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, nil, 0
	}
	text, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, uint64(info.Size())
	}
	matches := skillFrontmatterPattern.FindStringSubmatch(string(text))
	if len(matches) < 2 {
		return nil, nil, uint64(info.Size())
	}
	fieldRe := regexp.MustCompile(`^(name|description):\s*(.*)$`)
	for _, line := range strings.Split(matches[1], "\n") {
		if caps := fieldRe.FindStringSubmatch(strings.TrimSpace(line)); len(caps) == 3 {
			value := scan.UnquoteYAMLScalar(caps[2])
			switch caps[1] {
			case "name":
				name = &value
			case "description":
				description = &value
			}
		}
	}
	return name, description, uint64(info.Size())
}
