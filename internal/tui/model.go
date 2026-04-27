package tui

import (
	"github.com/catoncat/skill-toggle/internal/config"
	"github.com/catoncat/skill-toggle/internal/skills"
	"github.com/catoncat/skill-toggle/internal/update"
)

// panel identifies which side of the left column has focus.
type panel string

const (
	panelEnabled  panel = "enabled"
	panelDisabled panel = "disabled"
)

// mode is the dominant top-level mode of the TUI.
type mode string

const (
	modeNormal      mode = "normal"
	modeSearch      mode = "search"
	modePreviewFull mode = "preview"
	modeHelp        mode = "help"
	modeUpdate      mode = "update"
)

// confirmKind tracks an inline y/N prompt while still in normal mode.
type confirmKind string

const (
	confirmNone      confirmKind = ""
	confirmApply     confirmKind = "apply"
	confirmUpdate    confirmKind = "update"
	confirmUpdateAll confirmKind = "update-all"
	confirmQuit      confirmKind = "quit"
)

// Model holds the full TUI state. It is constructed by NewModel and consumed
// by tea.Program through Init/Update/View.
type Model struct {
	sources     []skills.Source
	sourceRoots map[string]string
	offRoot     string
	legacyOff   [][]string

	// All scanned skills (raw, unfiltered).
	allSkills []skills.Skill

	// Filtered + sorted partitions, refreshed when allSkills/query/sort change.
	enabledList  []skills.Skill
	disabledList []skills.Skill

	// Panel cursors. enabledIdx/Offset and disabledIdx/Offset advance
	// independently so toggling Tab doesn't reset position.
	active         panel
	enabledIdx     int
	enabledOffset  int
	disabledIdx    int
	disabledOffset int

	// Filter / sort.
	query    string
	sortMode string

	// Staged operations awaiting `A`.
	stagedOps []skills.Operation

	// Preview state (used by both right-pane preview and full-screen preview).
	previewSkill   *skills.Skill
	previewSkillMD string
	previewOffset  int

	// Top-level mode and an optional inline confirm prompt.
	mode           mode
	pendingConfirm confirmKind

	// showLinked controls whether skill rows whose canonical path duplicates
	// another row (because one source root is a symlink to another) are
	// shown. Off by default, toggled via "." (dotfile-style).
	showLinked bool

	// Update overlay state. When mode == modeUpdate the right-hand layout
	// is replaced by a streaming view of npx output.
	updateName         string
	updateLines        []string
	updateRunning      bool
	updateExit         *int  // non-nil once the process has exited
	updateErr          error // non-nil if Start itself failed
	updateScrollOffset int   // 0 anchors at bottom; increments scroll up
	updateCancel       func()
	updateLinesCh      <-chan update.Line
	updateResultCh     <-chan update.Result

	// Status feedback.
	message     string
	messageType string // "info" | "error"

	// Terminal dimensions.
	width  int
	height int
}

// NewModel constructs a Model wired to the live config. Callers may override
// individual fields after construction in tests; see TUI tests.
func NewModel() Model {
	return Model{
		sources:     config.Sources(),
		sourceRoots: config.SourceRootMap(),
		offRoot:     config.OffRoot(),
		legacyOff:   config.LegacyOffPerSource(),
		active:      panelEnabled,
		mode:        modeNormal,
		sortMode:    skills.SortByName,
	}
}

// activePanel returns the model's active panel as an enum value.
func (m Model) activePanel() panel { return m.active }

// currentList returns the slice the active panel is showing.
func (m Model) currentList() []skills.Skill {
	if m.active == panelDisabled {
		return m.disabledList
	}
	return m.enabledList
}

// currentIdx returns the cursor index within the active panel.
func (m Model) currentIdx() int {
	if m.active == panelDisabled {
		return m.disabledIdx
	}
	return m.enabledIdx
}

// currentSkill returns a pointer to the skill under the active cursor, or
// nil when the active panel is empty.
func (m Model) currentSkill() *skills.Skill {
	list := m.currentList()
	idx := m.currentIdx()
	if idx < 0 || idx >= len(list) {
		return nil
	}
	s := list[idx]
	return &s
}

// isStaged reports whether the given (Source, Name) skill has a pending op.
func (m Model) isStaged(s skills.Skill) bool {
	for _, op := range m.stagedOps {
		if op.Source == s.Source && op.SkillName == s.Name {
			return true
		}
	}
	return false
}

// stageCounts returns (enableCount, disableCount) of pending operations.
func (m Model) stageCounts() (enable int, disable int) {
	for _, op := range m.stagedOps {
		if op.Direction == "enable" {
			enable++
		} else {
			disable++
		}
	}
	return
}

// refreshLists rebuilds enabledList and disabledList from allSkills using
// the current query, sort mode and showLinked flag, then clamps cursors.
func (m *Model) refreshLists() {
	base := m.allSkills
	if !m.showLinked {
		filtered := make([]skills.Skill, 0, len(base))
		for _, s := range base {
			if s.IsDuplicate {
				continue
			}
			filtered = append(filtered, s)
		}
		base = filtered
	}
	all := skills.FilterSkills(base, m.query, "all", m.sortMode)
	m.enabledList = m.enabledList[:0]
	m.disabledList = m.disabledList[:0]
	for _, s := range all {
		if s.Status == "enabled" {
			m.enabledList = append(m.enabledList, s)
		} else {
			m.disabledList = append(m.disabledList, s)
		}
	}
	m.clampSelection()
}

// clampSelection keeps the cursor and scroll offset in range for both panels.
func (m *Model) clampSelection() {
	enabledHeight, disabledHeight := m.panelBodyHeights()
	clampOne := func(idx, offset, size, height int) (int, int) {
		if size == 0 {
			return 0, 0
		}
		if idx < 0 {
			idx = 0
		}
		if idx >= size {
			idx = size - 1
		}
		if height < 1 {
			height = 1
		}
		if idx < offset {
			offset = idx
		}
		if idx >= offset+height {
			offset = idx - height + 1
		}
		if offset < 0 {
			offset = 0
		}
		return idx, offset
	}
	m.enabledIdx, m.enabledOffset = clampOne(m.enabledIdx, m.enabledOffset, len(m.enabledList), enabledHeight)
	m.disabledIdx, m.disabledOffset = clampOne(m.disabledIdx, m.disabledOffset, len(m.disabledList), disabledHeight)
}

// panelBodyHeights returns the renderable rows for each left-side panel,
// based on terminal height and the current mode.
func (m Model) panelBodyHeights() (enabled int, disabled int) {
	// Reserved chrome:
	//   1 line — bottom key strip
	//   1 line — search prompt when search mode is active
	chromeRows := 1
	if m.mode == modeSearch {
		chromeRows++
	}
	available := m.height - chromeRows
	if available < 4 {
		return 1, 1
	}
	enabledOuter := available / 2
	disabledOuter := available - enabledOuter
	// Each panel uses 2 rows of border + 1 row of title -> 3 rows of chrome.
	enabledBody := enabledOuter - 3
	disabledBody := disabledOuter - 3
	if enabledBody < 1 {
		enabledBody = 1
	}
	if disabledBody < 1 {
		disabledBody = 1
	}
	return enabledBody, disabledBody
}
