package setup

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// MarketplaceReviewPlan is a non-mutating Review Action that starts from an
// agent-native marketplace/source entry.
type MarketplaceReviewPlan struct {
	ID                string
	Action            MarketplaceActionKind
	Agent             string
	SourceID          string
	SourceLabel       string
	SourceKind        MarketplaceSourceKind
	SourcePath        string
	EntryID           string
	EntryName         string
	EntryKind         string
	EntryPath         string
	Operation         string
	ExpectedEffect    string
	Instructions      string
	NonMutating       bool
	Available         bool
	UnavailableReason string
}

// MarketplaceReviewResult reports a completed non-mutating marketplace review.
type MarketplaceReviewResult struct {
	Plan          MarketplaceReviewPlan
	Instructions  string
	NonMutating   bool
	ChangedFiles  bool
	ExecutedTools bool
}

// PlanMarketplaceEntryAction builds an available or unavailable marketplace
// review plan for a source entry.
func PlanMarketplaceEntryAction(source MarketplaceSource, entry MarketplaceEntry, action MarketplaceActionKind) MarketplaceReviewPlan {
	plan := MarketplaceReviewPlan{
		ID:             strings.Join([]string{entry.ID, string(action)}, ":"),
		Action:         action,
		Agent:          entry.Agent.String(),
		SourceID:       source.ID,
		SourceLabel:    displaySafeText(firstNonEmpty(source.Label, sourceLabelFromPath(source.Path), entry.SourceID)),
		SourceKind:     source.Kind,
		SourcePath:     displaySafeText(firstNonEmpty(source.Path, entry.SourcePath)),
		EntryID:        entry.ID,
		EntryName:      displaySafeText(entry.Name),
		EntryKind:      entry.Kind.String(),
		EntryPath:      displaySafeText(entry.SourcePath),
		Operation:      "review marketplace setup guidance",
		ExpectedEffect: "non-mutating setup guidance only; no files, commands, hooks, plugins, or network calls",
		NonMutating:    true,
	}
	if plan.SourceKind == "" {
		plan.SourceKind = entry.SourceKind
	}
	if action != MarketplaceActionReview {
		return unavailableMarketplaceReview(plan, marketplaceActionUnavailableReason(entry, action))
	}
	if !marketplaceEntryReviewAvailable(entry) {
		return unavailableMarketplaceReview(plan, marketplaceEntryReviewUnavailableReason(entry))
	}
	plan.Instructions = marketplaceReviewInstructions(source, entry)
	plan.Available = true
	return plan
}

// ExecuteMarketplaceReviewPlan completes a non-mutating marketplace review
// after revalidating the source entry against fresh marketplace data.
func ExecuteMarketplaceReviewPlan(plan MarketplaceReviewPlan, freshSources []MarketplaceSource) (MarketplaceReviewResult, error) {
	if !plan.Available {
		return MarketplaceReviewResult{}, fmt.Errorf("%w: %s", ErrActionUnavailable, plan.UnavailableReason)
	}
	if plan.Action != MarketplaceActionReview {
		return MarketplaceReviewResult{}, fmt.Errorf("%w: marketplace action %q is not reviewable", ErrActionUnavailable, plan.Action)
	}
	source, entry, ok := findMarketplaceEntry(freshSources, plan.SourceID, plan.EntryID)
	if !ok {
		return MarketplaceReviewResult{}, errors.New("stale marketplace review: source entry no longer exists")
	}
	next := PlanMarketplaceEntryAction(source, entry, MarketplaceActionReview)
	if !next.Available {
		return MarketplaceReviewResult{}, fmt.Errorf("%w: %s", ErrActionUnavailable, next.UnavailableReason)
	}
	return MarketplaceReviewResult{
		Plan:          next,
		Instructions:  next.Instructions,
		NonMutating:   true,
		ChangedFiles:  false,
		ExecutedTools: false,
	}, nil
}

func findMarketplaceEntry(sources []MarketplaceSource, sourceID, entryID string) (MarketplaceSource, MarketplaceEntry, bool) {
	for _, source := range sources {
		if source.ID != sourceID {
			continue
		}
		for _, entry := range source.Entries {
			if entry.ID == entryID {
				return source, entry, true
			}
		}
	}
	return MarketplaceSource{}, MarketplaceEntry{}, false
}

func marketplaceEntryReviewAvailable(entry MarketplaceEntry) bool {
	if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.SourceID) == "" {
		return false
	}
	if strings.TrimSpace(entry.Name) == "" {
		return false
	}
	return strings.TrimSpace(entry.SourcePath) != "" ||
		strings.TrimSpace(entry.Description) != "" ||
		strings.TrimSpace(entry.Author) != "" ||
		strings.TrimSpace(entry.Category) != "" ||
		strings.TrimSpace(entry.Version) != "" ||
		len(entry.Provides) > 0
}

func marketplaceEntryReviewUnavailableReason(entry MarketplaceEntry) string {
	if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.SourceID) == "" {
		return "marketplace review requires a stable source entry"
	}
	if strings.TrimSpace(entry.Name) == "" {
		return "marketplace review requires an entry name"
	}
	return "marketplace review requires source metadata"
}

func marketplaceActionUnavailableReason(entry MarketplaceEntry, action MarketplaceActionKind) string {
	for _, availability := range entry.Actions {
		if availability.Action == action && strings.TrimSpace(availability.Reason) != "" {
			return availability.Reason
		}
	}
	return "agent-native marketplace action provider is not implemented yet"
}

func unavailableMarketplaceReview(plan MarketplaceReviewPlan, reason string) MarketplaceReviewPlan {
	plan.Available = false
	plan.UnavailableReason = reason
	return plan
}

func marketplaceReviewInstructions(source MarketplaceSource, entry MarketplaceEntry) string {
	lines := []string{
		"Review this agent-native source entry before changing setup.",
		"Source: " + firstNonEmpty(source.Label, sourceLabelFromPath(source.Path), entry.SourceID),
		"Entry: " + entry.Name,
		"Effect: non-mutating guidance only; Gandalf will not install, update, uninstall, add sources, remove sources, run commands, or write files.",
	}
	if strings.TrimSpace(entry.Description) != "" {
		lines = append(lines, "Description: "+entry.Description)
	}
	if strings.TrimSpace(entry.Author) != "" {
		lines = append(lines, "Author: "+entry.Author)
	}
	if strings.TrimSpace(entry.Version) != "" {
		lines = append(lines, "Version: "+entry.Version)
	}
	if len(entry.Provides) > 0 {
		lines = append(lines, "Provides: "+strings.Join(entry.Provides, ", "))
	}
	if strings.TrimSpace(entry.SourcePath) != "" {
		lines = append(lines, "Source path: "+entry.SourcePath)
	}
	return displaySafeText(strings.Join(lines, "\n"))
}

func displaySafeText(value string) string {
	value = stripTerminalEscapes(value)
	var builder strings.Builder
	for _, r := range value {
		if r == '\n' || r == '\t' {
			builder.WriteRune(r)
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return strings.TrimSpace(builder.String())
}

func stripTerminalEscapes(value string) string {
	var builder strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != 0x1b {
			builder.WriteByte(value[i])
			continue
		}
		i++
		if i >= len(value) {
			break
		}
		switch value[i] {
		case '[':
			for i+1 < len(value) {
				i++
				if value[i] >= 0x40 && value[i] <= 0x7e {
					break
				}
			}
		case ']':
			for i+1 < len(value) {
				i++
				if value[i] == 0x07 {
					break
				}
				if value[i] == 0x1b && i+1 < len(value) && value[i+1] == '\\' {
					i++
					break
				}
			}
		default:
			// Single-character escape sequence.
		}
	}
	return builder.String()
}
