package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Color palette using Lip Gloss adaptive colors for light/dark terminal support.
var (
	Accent  = lipgloss.AdaptiveColor{Light: "#6C47FF", Dark: "#8B6BFF"}
	Success = lipgloss.AdaptiveColor{Light: "#008000", Dark: "#00D700"}
	Dimmed  = lipgloss.AdaptiveColor{Light: "#909090", Dark: "#626262"}
	Warning = lipgloss.AdaptiveColor{Light: "#CC8800", Dark: "#FFD700"}
	Danger  = lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#FF4444"}
)

var (
	// Base styles used internally to compose the exported functions.
	tabActive    = lipgloss.NewStyle().Padding(0, 2).Foreground(Accent).Bold(true).Underline(true)
	tabInactive  = lipgloss.NewStyle().Padding(0, 2).Foreground(Dimmed)
	tabBar       = lipgloss.NewStyle().MarginBottom(1)
	statusBar    = lipgloss.NewStyle().Padding(0, 1).Foreground(Dimmed).Width(60)
	stats        = lipgloss.NewStyle().Foreground(Dimmed)
	enabledBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(Success).
			Padding(0, 1).
			Bold(true)
	disabledBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(Dimmed).
			Padding(0, 1).
			Bold(true)
	previewTitle   = lipgloss.NewStyle().Bold(true).Foreground(Accent).MarginBottom(1)
	previewContent = lipgloss.NewStyle().Padding(0, 2)
	title          = lipgloss.NewStyle().Bold(true).Foreground(Accent).Padding(0, 1)
	footer         = lipgloss.NewStyle().Foreground(Dimmed).Padding(0, 1).MarginTop(1)
	errStyle       = lipgloss.NewStyle().Foreground(Danger).Bold(true)
	infoStyle      = lipgloss.NewStyle().Foreground(Dimmed)
	searchInput    = lipgloss.NewStyle().Padding(0, 1).Width(30)
	commandPalette = lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.RoundedBorder()).Width(50)
	confirmDialog  = lipgloss.NewStyle().Padding(1, 2).Border(lipgloss.DoubleBorder()).Width(40)
)

// TabStyle returns a style for a profile tab. Active tabs are underlined and
// accented; inactive tabs are dimmed.
func TabStyle(active bool) lipgloss.Style {
	if active {
		return tabActive
	}
	return tabInactive
}

// TabBarStyle returns the style wrapping the tab row.
func TabBarStyle() lipgloss.Style { return tabBar }

// StatusBarStyle returns the style for the status bar.
func StatusBarStyle() lipgloss.Style { return statusBar }

// StatsStyle returns the style for inline statistics.
func StatsStyle() lipgloss.Style { return stats }

// SkillRowStyle returns a style for a single skill row. When selected the row
// is highlighted with the accent color.
func SkillRowStyle(selected bool, status string) lipgloss.Style {
	s := lipgloss.NewStyle().Padding(0, 1)
	if selected {
		s = s.Background(Accent).Foreground(lipgloss.Color("#000000"))
	}
	return s
}

// EnabledBadge returns the "enabled" badge style.
func EnabledBadge() lipgloss.Style { return enabledBadge }

// DisabledBadge returns the "disabled" badge style.
func DisabledBadge() lipgloss.Style { return disabledBadge }

// PreviewTitleStyle returns the style for the preview pane title.
func PreviewTitleStyle() lipgloss.Style { return previewTitle }

// PreviewContentStyle returns the style for the preview pane content.
func PreviewContentStyle() lipgloss.Style { return previewContent }

// TitleStyle returns the style for the main application title.
func TitleStyle() lipgloss.Style { return title }

// FooterStyle returns the style for the footer.
func FooterStyle() lipgloss.Style { return footer }

// ErrorStyle returns the style for error messages.
func ErrorStyle() lipgloss.Style { return errStyle }

// InfoStyle returns the style for informational messages.
func InfoStyle() lipgloss.Style { return infoStyle }

// SearchInputStyle returns the style for the search/filter input.
func SearchInputStyle() lipgloss.Style { return searchInput }

// CommandPaletteStyle returns the style for the command palette overlay.
func CommandPaletteStyle() lipgloss.Style { return commandPalette }

// ConfirmDialogStyle returns the style for confirmation dialogs.
func ConfirmDialogStyle() lipgloss.Style { return confirmDialog }

// TrimToWidth truncates text with an ellipsis if it exceeds the given width.
//   - If width <= 0, returns "".
//   - If len(text) <= width, returns text unchanged.
//   - If width <= 1, returns text[:width] (no room for ellipsis).
//   - Otherwise returns text[:width-1] + "…".
func TrimToWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(text) <= width {
		return text
	}
	if width <= 1 {
		return text[:width]
	}
	return text[:width-1] + "…"
}

// FormatDescChars formats a description character count with a human-readable
// unit suffix, e.g. 12400 -> "12.4k", 500 -> "500", 0 -> "0".
func FormatDescChars(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

// PadRight pads s with spaces to the given width. If s is already longer than
// width, it is returned unchanged.
func PadRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// TruncateLeft truncates a string from the left if it exceeds the given width,
// prefixing the result with "…". e.g. TruncateLeft("hello", 4) -> "…llo".
func TruncateLeft(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(s) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	return "…" + s[len(s)-width+1:]
}
