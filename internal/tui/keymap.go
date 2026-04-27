package tui

// helpEntry is one row in the help overlay.
type helpEntry struct {
	Group string
	Key   string
	Label string
}

func helpEntries() []helpEntry {
	return []helpEntry{
		{"Navigation", "j / ↓", "cursor down"},
		{"Navigation", "k / ↑", "cursor up"},
		{"Navigation", "g", "top of panel"},
		{"Navigation", "G", "bottom of panel"},
		{"Navigation", "ctrl+d", "half page down"},
		{"Navigation", "ctrl+u", "half page up"},
		{"Navigation", "tab / shift+tab", "switch panel"},
		{"Navigation", "H", "focus Enabled panel"},
		{"Navigation", "L", "focus Disabled panel"},

		{"Skill", "space", "stage / unstage"},
		{"Skill", "A", "apply staged ops"},
		{"Skill", "C", "clear all staged ops"},
		{"Skill", "p / enter", "full-screen preview"},
		{"Skill", "u", "update current enabled skill"},
		{"Skill", "U", "update all global skills"},
		{"Skill", "r", "rescan filesystem"},

		{"View", "/", "search (clears with esc)"},
		{"View", "esc", "clear search filter"},
		{"View", ".", "toggle symlink markers"},
		{"View", "s", "cycle sort"},
		{"View", "?", "toggle help"},

		{"Quit", "q", "quit (confirm if staged)"},
		{"Quit", "ctrl+c", "hard quit"},
	}
}

// keyStripHints returns the inline key reminders shown at the bottom of the
// screen for the given mode.
func keyStripHints(m Model) []hintPair {
	switch {
	case m.mode == modeSearch:
		return []hintPair{
			{"type", "filter"},
			{"enter", "done"},
			{"esc", "clear"},
		}
	case m.mode == modePreviewFull:
		return []hintPair{
			{"j/k", "scroll"},
			{"g/G", "top/bot"},
			{"esc/p/q", "back"},
		}
	case m.pendingConfirm != confirmNone:
		return []hintPair{
			{"y", "confirm"},
			{"esc", "cancel"},
		}
	default:
		return []hintPair{
			{"tab", "switch"},
			{"j/k", "move"},
			{"space", "stage"},
			{"A", "apply"},
			{"p", "preview"},
			{"/", "search"},
			{"u/U", "update"},
			{"?", "help"},
			{"q", "quit"},
		}
	}
}

type hintPair struct {
	Key, Label string
}
