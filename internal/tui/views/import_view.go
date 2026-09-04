package views

import (
	"fmt"
	"strings"
)

// ImportItemView is one toggleable server or skill row in the import wizard.
type ImportItemView struct {
	Kind     string // "server" | "skill"
	Name     string
	Detail   string // e.g. server command or skill source
	Selected bool
	Cursor   bool
}

// ImportGroupView groups discovered items under one source agent.
type ImportGroupView struct {
	Agent string // display name, e.g. "Claude Code"
	Scope string // "project" | "global"
	Path  string
	Items []ImportItemView
}

// ImportSelectModel is the render input for the import selection screen.
type ImportSelectModel struct {
	Title       string
	ProjectPath string
	Groups      []ImportGroupView
	Width       int
	Height      int
}

// ImportPreviewModel is the render input for the manifest preview screen.
type ImportPreviewModel struct {
	OutputFile  string
	TOML        string
	Offset      int
	MaskedCount int
	Width       int
	Height      int
}

// ImportResultModel is the render input for done / cancelled / failed states.
type ImportResultModel struct {
	Title   string
	Badge   string // "In Sync" | "Drift" | "Discovered"
	Lines   []string
	Hint    string
	Width   int
	Height  int
}

// ImportBadge renders a high-contrast status badge like [In Sync].
func ImportBadge(label string) string {
	switch label {
	case "In Sync":
		return cleanStyle.Render("[" + label + "]")
	case "Drift":
		return changedStyle.Render("[" + label + "]")
	case "Discovered":
		return focusStyle.Render("[" + label + "]")
	default:
		return labelStyle.Render("[" + label + "]")
	}
}

// importHelpBar renders the keybinding help footer.
func importHelpBar(width int, entries ...string) string {
	return mutedStyle.Render(truncate(strings.Join(entries, "  ·  "), width))
}

// RenderImportSelect renders the grouped, toggleable source selection screen.
func RenderImportSelect(m ImportSelectModel) string {
	width := max(m.Width, 20)
	header := RenderHeader(HeaderView{
		Title: m.Title,
		Scope: m.ProjectPath,
		Chips: []HeaderChip{{
			AgentMarker: "import",
			State:       "clean",
			Detail:      "scan",
		}},
	}, width)

	var body []string
	for _, group := range m.Groups {
		scope := ""
		if group.Scope != "" {
			scope = " (" + group.Scope + ")"
		}
		head := labelStyle.Render(group.Agent+scope) + "  " + ImportBadge("Discovered") + "  " + mutedStyle.Render(group.Path)
		body = append(body, truncate(head, width))
		if len(group.Items) == 0 {
			body = append(body, truncate("    "+mutedStyle.Render("no importable items"), width))
		}
		for _, item := range group.Items {
			body = append(body, truncate(renderImportItem(item, width-2), width))
		}
		body = append(body, "")
	}
	if len(body) > 0 {
		body = body[:len(body)-1]
	}

	status := importHelpBar(width, "space toggle", "tab preview", "enter import", "q cancel")
	return RenderFrame(header, strings.Join(body, "\n"), status, width, m.Height)
}

func renderImportItem(item ImportItemView, width int) string {
	checkbox := "[ ]"
	if item.Selected {
		checkbox = cleanStyle.Render("[x]")
	}
	kind := mutedStyle.Render(item.Kind)
	line := fmt.Sprintf("  %s %s  %s", checkbox, item.Name, kind)
	if item.Detail != "" {
		line += mutedStyle.Render("  " + item.Detail)
	}
	if item.Cursor {
		plain := fmt.Sprintf("  %s %s  %s", "[ ]", item.Name, item.Kind)
		if item.Selected {
			plain = fmt.Sprintf("  %s %s  %s", "[x]", item.Name, item.Kind)
		}
		if item.Detail != "" {
			plain += "  " + item.Detail
		}
		return selectedRow.Render(truncate("❯"+plain[1:], width))
	}
	return line
}

// RenderImportPreview renders the prospective gandalf.toml with masked secrets
// inside a bordered, scrollable pane.
func RenderImportPreview(m ImportPreviewModel) string {
	width := max(m.Width, 20)
	header := titleStyle.Render("Import preview") + mutedStyle.Render("  "+m.OutputFile)
	if m.MaskedCount > 0 {
		header += cleanStyle.Render(fmt.Sprintf("  %d secret(s) masked as ${VAR}", m.MaskedCount))
	}
	header = truncate(header, width)

	innerWidth := max(width-6, 10) // border(2) + padding(2) + "+ "(2)
	lines := strings.Split(strings.TrimRight(m.TOML, "\n"), "\n")
	bodyHeight := max(m.Height-4, 1) // header + divider + status
	paneHeight := max(bodyHeight-2, 1)

	if m.Offset > len(lines)-1 {
		m.Offset = max(len(lines)-1, 0)
	}
	end := min(m.Offset+paneHeight, len(lines))

	rendered := make([]string, 0, paneHeight)
	for _, line := range lines[m.Offset:end] {
		rendered = append(rendered, truncate(cleanStyle.Render("+ ")+truncate(line, innerWidth), innerWidth+2))
	}
	for len(rendered) < paneHeight {
		rendered = append(rendered, "")
	}

	pane := paneBorder.Width(width - 4).Render(strings.Join(rendered, "\n"))

	scrollHint := fmt.Sprintf("lines %d-%d of %d", m.Offset+1, max(end, 1), len(lines))
	status := importHelpBar(width, "j/k scroll", "tab back", "enter import", "q cancel", scrollHint)
	return RenderFrame(header, pane, status, width, m.Height)
}

// RenderImportResult renders terminal states (done, cancelled, failed).
func RenderImportResult(m ImportResultModel) string {
	width := max(m.Width, 20)
	header := RenderHeader(HeaderView{Title: m.Title, Scope: ""}, width)

	var body []string
	if m.Badge != "" {
		body = append(body, ImportBadge(m.Badge), "")
	}
	for _, line := range m.Lines {
		body = append(body, truncate(line, width))
	}
	status := importHelpBar(width, m.Hint)
	return RenderFrame(header, strings.Join(body, "\n"), status, width, m.Height)
}

// RenderImportLoading renders the initial scan-in-progress screen.
func RenderImportLoading(title, projectPath string, width, height int) string {
	width = max(width, 20)
	header := RenderHeader(HeaderView{Title: title, Scope: projectPath}, width)
	body := mutedStyle.Render("Scanning Claude Code, Cursor, and Codex configurations…")
	return RenderFrame(header, body, importHelpBar(width, "q cancel"), width, height)
}

