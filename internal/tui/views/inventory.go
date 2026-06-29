package views

import (
	"fmt"
	"strings"
)

// SetupActionConfirmation contains the confirmation text for a pending setup action.
type SetupActionConfirmation struct {
	Action       string
	AgentLabel   string
	ObjectKind   string
	TargetName   string
	Operation    string
	ConfigTarget string
	Command      string
}

// SetupMarketplaceReview contains the preview/result for a non-mutating
// marketplace-originated Review Action.
type SetupMarketplaceReview struct {
	Title          string
	Status         string
	AgentLabel     string
	SourceLabel    string
	SourcePath     string
	TargetName     string
	Operation      string
	ExpectedEffect string
	Instructions   string
	Pending        bool
}

func renderSetupActionConfirmation(model SetupActionConfirmation) []string {
	return []string{
		titleStyle.Render("Confirm setup action"),
		fmt.Sprintf("%s %s: %s", model.Action, model.ObjectKind, model.TargetName),
		labelStyle.Render("agent: " + model.AgentLabel),
		labelStyle.Render("operation: " + model.Operation),
		labelStyle.Render("target: " + model.ConfigTarget),
		labelStyle.Render("command: " + model.Command),
		mutedStyle.Render("Enter confirm · esc cancel"),
	}
}

func renderSetupMarketplaceReview(model SetupMarketplaceReview, width int) []string {
	lines := []string{
		titleStyle.Render(model.Title),
		labelStyle.Render("status: " + model.Status),
		labelStyle.Render("agent: " + model.AgentLabel),
		labelStyle.Render("source: " + model.SourceLabel),
		labelStyle.Render("target: " + model.TargetName),
		labelStyle.Render("operation: " + model.Operation),
		labelStyle.Render("effect: " + model.ExpectedEffect),
	}
	if model.SourcePath != "" {
		lines = append(lines, labelStyle.Render("source path: "+model.SourcePath))
	}
	lines = append(lines, "", labelStyle.Render("instructions"))
	for _, line := range strings.Split(model.Instructions, "\n") {
		for _, wrapped := range wrapText(line, max(16, width-4)) {
			lines = append(lines, mutedStyle.Render("  "+wrapped))
		}
	}
	if model.Pending {
		lines = append(lines, mutedStyle.Render("Enter complete review · esc cancel"))
	} else {
		lines = append(lines, mutedStyle.Render("No files changed. No commands executed."))
	}
	return lines
}
