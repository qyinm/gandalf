package views

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// KeymapEntry is one key binding in the keymap overlay.
type KeymapEntry struct {
	Key  string
	Help string
}

// KeymapSection groups keymap entries under a title.
type KeymapSection struct {
	Title string
	Keys  []KeymapEntry
}

// RenderKeymapOverlay renders the full key reference centered over the body.
func RenderKeymapOverlay(background string, sections []KeymapSection, width, height int) string {
	keyWidth := 0
	lineCount := 0
	contentWidthNeeded := ansi.StringWidth("any key to close")
	for _, section := range sections {
		lineCount += len(section.Keys) + 2
		contentWidthNeeded = max(contentWidthNeeded, ansi.StringWidth(section.Title))
		for _, entry := range section.Keys {
			if w := ansi.StringWidth(entry.Key); w > keyWidth {
				keyWidth = w
			}
		}
	}
	for _, section := range sections {
		for _, entry := range section.Keys {
			entryWidth := 2 + keyWidth + 2 + ansi.StringWidth(entry.Help)
			contentWidthNeeded = max(contentWidthNeeded, entryWidth)
		}
	}

	overlayWidth := min(max(44, contentWidthNeeded+4), max(44, width-4))
	contentWidth := overlayWidth - 4
	lines := []string{titleStyle.Render(truncate("Keys", contentWidth)), ""}
	for _, section := range sections {
		lines = append(lines, labelStyle.Render(truncate(section.Title, contentWidth)))
		for _, entry := range section.Keys {
			key := entry.Key + strings.Repeat(" ", max(0, keyWidth-ansi.StringWidth(entry.Key)))
			lines = append(lines, truncate("  "+focusStyle.Render(key)+"  "+entry.Help, contentWidth))
		}
		lines = append(lines, "")
	}
	lines = append(lines, mutedStyle.Render("any key to close"))

	box := lipgloss.NewStyle().
		Width(overlayWidth).
		Background(colorOverlayBg).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))

	bgLines := strings.Split(background, "\n")
	for len(bgLines) < height {
		bgLines = append(bgLines, "")
	}
	boxLines := strings.Split(box, "\n")
	top := max(0, (height-len(boxLines))/2)
	left := max(0, (width-overlayWidth)/2)
	for i, boxLine := range boxLines {
		target := top + i
		if target < 0 || target >= len(bgLines) {
			continue
		}
		bgLines[target] = overlayLine(bgLines[target], boxLine, left, width)
	}
	return strings.Join(bgLines, "\n")
}
