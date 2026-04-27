package tui

import (
	"github.com/catoncat/skill-toggle/internal/config"
	"github.com/catoncat/skill-toggle/internal/skills"
	"github.com/catoncat/skill-toggle/internal/update"
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

// Status filter values. The filter is the user-visible "axis" that the
// list is sliced along (a/e/d keys), orthogonal to the search query.
const (
	filterAll      = "all"
	filterEnabled  = "enabled"
	filterDisabled = "disabled"
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

	// Single visible list — refreshed when allSkills/query/filter/sort change.
	visibleList []skills.Skill

	// Cursor + viewport offset for the single list.
	idx    int
	offset int

	// Filter / sort.
	query        string
	statusFilter string
	sortMode     string

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
		sources:      config.Sources(),
		sourceRoots:  config.SourceRootMap(),
		offRoot:      config.OffRoot(),
		legacyOff:    config.LegacyOffPerSource(),
		mode:         modeNormal,
		sortMode:     skills.SortByName,
		statusFilter: filterAll,
	}
}

// currentList returns the visible list slice.
func (m Model) currentList() []skills.Skill { return m.visibleList }

// currentIdx returns the cursor index in the visible list.
func (m Model) currentIdx() int { return m.idx }

// currentSkill returns a pointer to the skill under the cursor, or nil when
// the visible list is empty.
func (m Model) currentSkill() *skills.Skill {
	if m.idx < 0 || m.idx >= len(m.visibleList) {
		return nil
	}
	s := m.visibleList[m.idx]
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

// refreshLists rebuilds visibleList from allSkills using the current query,
// filter, sort mode and showLinked flag, then clamps the cursor.
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
	m.visibleList = skills.FilterSkills(base, m.query, m.statusFilter, m.sortMode)
	m.clampSelection()
}

// clampSelection keeps the cursor and scroll offset in range.
func (m *Model) clampSelection() {
	size := len(m.visibleList)
	if size == 0 {
		m.idx = 0
		m.offset = 0
		return
	}
	if m.idx < 0 {
		m.idx = 0
	}
	if m.idx >= size {
		m.idx = size - 1
	}
	height := m.listBodyHeight()
	if height < 1 {
		height = 1
	}
	if m.idx < m.offset {
		m.offset = m.idx
	}
	if m.idx >= m.offset+height {
		m.offset = m.idx - height + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// listBodyHeight returns the renderable rows for the single skill panel,
// based on terminal height and the current mode.
func (m Model) listBodyHeight() int {
	// Reserved chrome:
	//   1 line — bottom key strip / search prompt
	chromeRows := 1
	if m.mode == modeSearch {
		chromeRows++
	}
	available := m.height - chromeRows
	if available < 4 {
		return 1
	}
	// Panel uses 2 rows of border + 1 row of title -> 3 rows of chrome.
	body := available - 3
	if body < 1 {
		return 1
	}
	return body
}
