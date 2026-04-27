// Package tui implements the Bubble Tea TUI for skill-toggle.
//
// File layout:
//
//	tui.go     Run entry point, package doc, small file helpers
//	model.go   Model type, NewModel, panel/mode/confirm enums, layout helpers
//	update.go  Init / Update, async messages (scan + npx skills update), key handlers
//	view.go    View, panel rendering, bottom strip, help overlay
//	keymap.go  help table & bottom-strip hint sets
package tui

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// Run launches the TUI against the live config (sources, off-root, legacy
// off-roots) resolved by internal/config. Tests construct Model directly via
// NewModel and skip Run().
func Run() error {
	m := NewModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

// readFileSafe wraps os.ReadFile so the view layer can stringify content
// without dragging os into view.go.
func readFileSafe(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
