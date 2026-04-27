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
		{"Navigation", "g", "top of list"},
		{"Navigation", "G", "bottom of list"},
		{"Navigation", "ctrl+d", "half page down"},
		{"Navigation", "ctrl+u", "half page up"},

		{"Filter", "a", "show all"},
		{"Filter", "e", "show enabled only"},
		{"Filter", "d", "show disabled only"},
		{"Filter", "/", "search"},
		{"Filter", "esc", "clear search"},
		{"Filter", ".", "toggle symlinks"},
		{"Filter", "s", "cycle sort"},

		{"Skill", "space", "stage / unstage"},
		{"Skill", "A", "apply staged ops"},
		{"Skill", "C", "clear staged"},
		{"Skill", "p / enter / tab", "open preview"},
		{"Skill", "u", "update skill"},
		{"Skill", "U", "update all"},
		{"Skill", "r", "rescan disk"},

		{"View", "?", "toggle help"},
		{"View", "q", "quit"},
		{"View", "ctrl+c", "hard quit"},
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
			{"esc/p/tab", "back"},
		}
	case m.pendingConfirm != confirmNone:
		return []hintPair{
			{"y", "confirm"},
			{"esc", "cancel"},
		}
	default:
		return []hintPair{
			{"a/e/d", "filter"},
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
