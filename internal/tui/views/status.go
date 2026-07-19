package views

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// StatusLevel is the semantic severity of the frame status line.
type StatusLevel string

const (
	StatusInfo    StatusLevel = "info"
	StatusSuccess StatusLevel = "success"
	StatusWarn    StatusLevel = "warn"
	StatusError   StatusLevel = "error"
)

// StatusLine is the single frame-level feedback channel rendered on every screen.
type StatusLine struct {
	Level StatusLevel
	Text  string
}

// RenderStatusLine renders the unified status region. Empty text renders nothing.
func RenderStatusLine(status StatusLine, width int) string {
	if strings.TrimSpace(status.Text) == "" {
		return ""
	}
	var style lipgloss.Style
	var marker string
	switch status.Level {
	case StatusError:
		style, marker = removedStyle, "✗ "
	case StatusWarn:
		style, marker = changedStyle, "▲ "
	case StatusSuccess:
		style, marker = cleanStyle, "✓ "
	default:
		style, marker = labelStyle, ""
	}
	return style.Render(truncate(marker+status.Text, width))
}

// RenderMuted applies the shared secondary-text token to a simple fallback.
func RenderMuted(text string) string {
	return mutedStyle.Render(text)
}

// RenderLoading renders the framed pre-boot loading state.
func RenderLoading(message string, width, height int) string {
	header := RenderHeader(HeaderView{Title: "Gandalf"}, width)
	body := mutedStyle.Render(truncate(message, width))
	return RenderFrame(header, body, "", width, height)
}

// RenderBootError renders the framed boot failure state.
func RenderBootError(title, detail string, width, height int) string {
	header := RenderHeader(HeaderView{Title: "Gandalf"}, width)
	lines := []string{
		removedStyle.Render(truncate("✗ "+title, width)),
		mutedStyle.Render(truncate(detail, width)),
		"",
		mutedStyle.Render("r rescan · q quit"),
	}
	return RenderFrame(header, strings.Join(lines, "\n"), "", width, height)
}
