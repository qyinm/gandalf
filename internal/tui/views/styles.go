package views

import "github.com/charmbracelet/lipgloss"

// Palette is seeded from DESIGN.md tokens, adapted to a terminal color space.
// The brand accent is reserved for focus/active emphasis, matching the design
// rule that brand appears as a stamp rather than a system.
const (
	colorBrand      = lipgloss.Color("203") // #FF5F6A brand accent
	colorForeground = lipgloss.Color("252") // primary text
	colorMuted      = lipgloss.Color("244") // secondary text / labels
	colorFaint      = lipgloss.Color("240") // captions / disabled hints
	colorSelectedBg = lipgloss.Color("236") // selected row background
	colorClean      = lipgloss.Color("78")  // green: clean / enabled
	colorChanged    = lipgloss.Color("214") // amber: drift / changed
	colorRemoved    = lipgloss.Color("203") // red-ish: removed / unavailable
	colorBorder     = lipgloss.Color("240") // pane borders
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorBrand)
	labelStyle  = lipgloss.NewStyle().Foreground(colorMuted)
	activeStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	warnStyle   = lipgloss.NewStyle().Foreground(colorChanged)
	mutedStyle  = lipgloss.NewStyle().Foreground(colorFaint)

	// Semantic styles for the control workspace.
	focusStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorBrand)
	selectedRow  = lipgloss.NewStyle().Foreground(colorForeground).Background(colorSelectedBg)
	cleanStyle   = lipgloss.NewStyle().Foreground(colorClean)
	changedStyle = lipgloss.NewStyle().Foreground(colorChanged)
	removedStyle = lipgloss.NewStyle().Foreground(colorRemoved)
	paneBorder   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorBorder)
)

// driftStyle returns the style for a drift/status word.
func driftStyle(status string) lipgloss.Style {
	switch status {
	case "clean", "enabled", "on", "ready":
		return cleanStyle
	case "changed", "drift", "disabled", "off":
		return changedStyle
	case "removed", "missing", "unavailable":
		return removedStyle
	default:
		return labelStyle
	}
}
