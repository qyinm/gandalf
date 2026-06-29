package views

import "fmt"

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
