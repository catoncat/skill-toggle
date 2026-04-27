package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/catoncat/skill-toggle/internal/skills"
	"github.com/catoncat/skill-toggle/internal/update"

	tea "github.com/charmbracelet/bubbletea"
)

// --- async messages ---

type skillsScannedMsg struct {
	skills []skills.Skill
	err    error
}

type updateLineMsg struct {
	line update.Line
}

type updateDoneMsg struct {
	result update.Result
}

func scanSkillsCmd(m Model) tea.Cmd {
	sources := m.sources
	off := m.offRoot
	legacy := m.legacyOff
	return func() tea.Msg {
		all, err := skills.Scan(sources, off, legacy...)
		if err != nil {
			return skillsScannedMsg{err: err}
		}
		return skillsScannedMsg{skills: all}
	}
}

// waitUpdateCmd blocks on the next event from either of the streaming
// channels and turns it into a tea.Msg. Re-issued by Update after every
// updateLineMsg so streaming continues until the process exits.
func waitUpdateCmd(lines <-chan update.Line, result <-chan update.Result) tea.Cmd {
	return func() tea.Msg {
		select {
		case line, ok := <-lines:
			if !ok {
				res, ok := <-result
				if !ok {
					return updateDoneMsg{result: update.Result{}}
				}
				return updateDoneMsg{result: res}
			}
			return updateLineMsg{line: line}
		case res, ok := <-result:
			if !ok {
				return updateDoneMsg{result: update.Result{}}
			}
			return updateDoneMsg{result: res}
		}
	}
}

// Init kicks off the initial scan.
func (m Model) Init() tea.Cmd {
	return scanSkillsCmd(m)
}

// Update is the central tea.Model update entry point.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampSelection()
		return m, nil

	case skillsScannedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("scan error: %v", msg.err)
			m.messageType = "error"
			return m, nil
		}
		m.allSkills = msg.skills
		m.refreshLists()
		// Keep staged ops only for skills still present.
		m.pruneStagedOps()
		return m, nil

	case updateLineMsg:
		m.appendUpdateLine(msg.line)
		// Keep listening — re-issue the wait cmd after every line.
		return m, waitUpdateCmd(m.updateLinesCh, m.updateResultCh)

	case updateDoneMsg:
		m.updateRunning = false
		m.updateLinesCh = nil
		m.updateResultCh = nil
		m.updateCancel = nil
		if msg.result.Err != nil {
			m.updateErr = msg.result.Err
		} else {
			ec := msg.result.ExitCode
			m.updateExit = &ec
		}
		// Always rescan after an update — files may have shifted.
		return m, scanSkillsCmd(m)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// appendUpdateLine stores a streamed line and keeps the buffer bounded so
// a runaway process doesn't grow memory unbounded.
func (m *Model) appendUpdateLine(l update.Line) {
	prefix := ""
	if l.IsErr {
		prefix = "[err] "
	}
	m.updateLines = append(m.updateLines, prefix+l.Text)
	const maxLines = 4096
	if len(m.updateLines) > maxLines {
		m.updateLines = m.updateLines[len(m.updateLines)-maxLines:]
	}
}

func (m *Model) pruneStagedOps() {
	if len(m.stagedOps) == 0 {
		return
	}
	exists := make(map[string]bool)
	for _, s := range m.allSkills {
		exists[s.Source+"/"+s.Name] = true
	}
	out := m.stagedOps[:0]
	for _, op := range m.stagedOps {
		if exists[op.Source+"/"+op.SkillName] {
			out = append(out, op)
		}
	}
	m.stagedOps = out
}

// --- key dispatch ---

func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Type == tea.KeyCtrlC {
		if m.updateCancel != nil {
			m.updateCancel()
		}
		return m, tea.Quit
	}
	if m.pendingConfirm != confirmNone {
		return m.handleConfirmKey(key)
	}
	switch m.mode {
	case modeSearch:
		return m.handleSearchKey(key)
	case modePreviewFull:
		return m.handlePreviewKey(key)
	case modeHelp:
		return m.dismissHelp()
	case modeUpdate:
		return m.handleUpdateKey(key)
	default:
		return m.handleNormalKey(key)
	}
}

// --- normal mode ---

func (m Model) handleNormalKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyTab:
		return m.swapPanel(true), nil
	case tea.KeyShiftTab:
		return m.swapPanel(false), nil
	case tea.KeyUp:
		return m.moveCursor(-1), nil
	case tea.KeyDown:
		return m.moveCursor(1), nil
	case tea.KeyPgUp:
		return m.moveCursorBy(-m.activePageSize()), nil
	case tea.KeyPgDown:
		return m.moveCursorBy(m.activePageSize()), nil
	case tea.KeySpace:
		m.stageCurrent()
		return m, nil
	case tea.KeyEnter:
		return m.openPreviewFull()
	case tea.KeyCtrlD:
		return m.moveCursorBy(m.activePageSize() / 2), nil
	case tea.KeyCtrlU:
		return m.moveCursorBy(-m.activePageSize() / 2), nil
	case tea.KeyEsc:
		// Esc clears the active filter first, then status messages, so the
		// user has a fast path back from any "stuck" state without having
		// to delete the search query character by character.
		if m.query != "" {
			m.query = ""
			m.refreshLists()
			m.message = "filter cleared"
			m.messageType = "info"
			return m, nil
		}
		m.message = ""
		return m, nil
	}

	switch key.String() {
	case "q":
		if len(m.stagedOps) > 0 {
			m.pendingConfirm = confirmQuit
			return m, nil
		}
		return m, tea.Quit
	case "j":
		return m.moveCursor(1), nil
	case "k":
		return m.moveCursor(-1), nil
	case "g":
		return m.cursorToEdge(true), nil
	case "G":
		return m.cursorToEdge(false), nil
	case "H":
		m.active = panelEnabled
		m.clampSelection()
		return m, nil
	case "L":
		m.active = panelDisabled
		m.clampSelection()
		return m, nil
	case "/":
		m.mode = modeSearch
		return m, nil
	case "p":
		return m.openPreviewFull()
	case "?":
		m.mode = modeHelp
		return m, nil
	case "s":
		m.cycleSort()
		return m, nil
	case "r":
		m.message = "rescanning…"
		m.messageType = "info"
		return m, scanSkillsCmd(m)
	case "u":
		s := m.currentSkill()
		if s == nil {
			return m, nil
		}
		if s.Status != "enabled" {
			m.message = fmt.Sprintf("cannot update disabled skill: %s/%s", s.Source, s.Name)
			m.messageType = "error"
			return m, nil
		}
		m.pendingConfirm = confirmUpdate
		return m, nil
	case "U":
		m.pendingConfirm = confirmUpdateAll
		return m, nil
	case "A":
		if len(m.stagedOps) == 0 {
			m.message = "no staged operations"
			m.messageType = "info"
			return m, nil
		}
		m.pendingConfirm = confirmApply
		return m, nil
	case "C":
		if len(m.stagedOps) == 0 {
			return m, nil
		}
		m.stagedOps = nil
		m.message = "cleared staged operations"
		m.messageType = "info"
		return m, nil
	case ".":
		m.showLinked = !m.showLinked
		m.refreshLists()
		if m.showLinked {
			m.message = "showing symlinked duplicates"
		} else {
			m.message = "hiding symlinked duplicates"
		}
		m.messageType = "info"
		return m, nil
	}
	return m, nil
}

// --- search mode ---

func (m Model) handleSearchKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEnter:
		m.mode = modeNormal
		return m, nil
	case tea.KeyEsc:
		m.mode = modeNormal
		m.query = ""
		m.refreshLists()
		return m, nil
	case tea.KeyBackspace:
		if len(m.query) > 0 {
			m.query = m.query[:len(m.query)-1]
			m.refreshLists()
		}
		return m, nil
	case tea.KeyRunes:
		m.query += string(key.Runes)
		m.refreshLists()
		return m, nil
	case tea.KeySpace:
		m.query += " "
		m.refreshLists()
		return m, nil
	}
	return m, nil
}

// --- preview-full mode ---

func (m Model) handlePreviewKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	previewHeight := m.previewBodyHeight()
	totalLines := m.previewLineCount()
	maxOffset := totalLines - previewHeight
	if maxOffset < 0 {
		maxOffset = 0
	}

	switch key.Type {
	case tea.KeyEsc:
		return m.closePreviewFull(), nil
	case tea.KeyDown:
		if m.previewOffset < maxOffset {
			m.previewOffset++
		}
		return m, nil
	case tea.KeyUp:
		if m.previewOffset > 0 {
			m.previewOffset--
		}
		return m, nil
	case tea.KeyPgDown, tea.KeyCtrlD:
		m.previewOffset += previewHeight
		if m.previewOffset > maxOffset {
			m.previewOffset = maxOffset
		}
		return m, nil
	case tea.KeyPgUp, tea.KeyCtrlU:
		m.previewOffset -= previewHeight
		if m.previewOffset < 0 {
			m.previewOffset = 0
		}
		return m, nil
	}
	switch key.String() {
	case "q", "p":
		return m.closePreviewFull(), nil
	case "j":
		if m.previewOffset < maxOffset {
			m.previewOffset++
		}
		return m, nil
	case "k":
		if m.previewOffset > 0 {
			m.previewOffset--
		}
		return m, nil
	case "g":
		m.previewOffset = 0
		return m, nil
	case "G":
		m.previewOffset = maxOffset
		return m, nil
	}
	return m, nil
}

func (m Model) openPreviewFull() (tea.Model, tea.Cmd) {
	skill := m.currentSkill()
	if skill == nil {
		return m, nil
	}
	mdPath := filepath.Join(skill.Path, "SKILL.md")
	data, err := os.ReadFile(mdPath)
	if err != nil {
		m.previewSkillMD = fmt.Sprintf("(error reading SKILL.md: %v)", err)
	} else {
		m.previewSkillMD = string(data)
	}
	m.previewSkill = skill
	m.previewOffset = 0
	m.mode = modePreviewFull
	return m, nil
}

func (m Model) closePreviewFull() Model {
	m.mode = modeNormal
	m.previewOffset = 0
	return m
}

func (m Model) dismissHelp() (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	return m, nil
}

// --- confirm dispatch ---

func (m Model) handleConfirmKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Type == tea.KeyEsc {
		m.pendingConfirm = confirmNone
		return m, nil
	}
	if key.String() != "y" && key.String() != "Y" {
		m.pendingConfirm = confirmNone
		return m, nil
	}

	pending := m.pendingConfirm
	m.pendingConfirm = confirmNone
	switch pending {
	case confirmApply:
		return m.applyAllStaged()
	case confirmUpdate:
		s := m.currentSkill()
		if s == nil {
			return m, nil
		}
		return m.startUpdate(s.Name)
	case confirmUpdateAll:
		return m.startUpdate("")
	case confirmQuit:
		return m, tea.Quit
	}
	return m, nil
}

// startUpdate kicks off `npx skills update` for one or all skills, switches
// the TUI into modeUpdate, and primes the streaming command. The user sees
// live stdout/stderr in a dedicated overlay until the process exits or
// they press Esc / q.
func (m Model) startUpdate(name string) (tea.Model, tea.Cmd) {
	m.mode = modeUpdate
	m.updateName = name
	m.updateLines = nil
	m.updateRunning = true
	m.updateExit = nil
	m.updateErr = nil
	m.updateScrollOffset = 0

	lines, result, cancel, err := update.Start(name)
	if err != nil {
		m.updateRunning = false
		m.updateErr = err
		return m, nil
	}
	m.updateLinesCh = lines
	m.updateResultCh = result
	m.updateCancel = cancel
	return m, waitUpdateCmd(lines, result)
}

// handleUpdateKey services keypresses while the update overlay is active.
func (m Model) handleUpdateKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		return m.closeUpdateOverlay(), nil
	case tea.KeyUp:
		m.updateScrollOffset++
		return m, nil
	case tea.KeyDown:
		if m.updateScrollOffset > 0 {
			m.updateScrollOffset--
		}
		return m, nil
	case tea.KeyPgUp, tea.KeyCtrlU:
		m.updateScrollOffset += 10
		return m, nil
	case tea.KeyPgDown, tea.KeyCtrlD:
		m.updateScrollOffset -= 10
		if m.updateScrollOffset < 0 {
			m.updateScrollOffset = 0
		}
		return m, nil
	}
	switch key.String() {
	case "q":
		return m.closeUpdateOverlay(), nil
	case "k":
		m.updateScrollOffset++
		return m, nil
	case "j":
		if m.updateScrollOffset > 0 {
			m.updateScrollOffset--
		}
		return m, nil
	case "g":
		m.updateScrollOffset = len(m.updateLines)
		return m, nil
	case "G":
		m.updateScrollOffset = 0
		return m, nil
	}
	return m, nil
}

// closeUpdateOverlay cancels the streaming process if it's still alive
// and resets the overlay state, returning the model in normal mode.
func (m Model) closeUpdateOverlay() Model {
	if m.updateCancel != nil {
		m.updateCancel()
	}
	exit := m.updateExit
	hadErr := m.updateErr
	m.mode = modeNormal
	m.updateLines = nil
	m.updateScrollOffset = 0
	m.updateExit = nil
	m.updateErr = nil
	switch {
	case hadErr != nil:
		m.message = fmt.Sprintf("update failed: %v", hadErr)
		m.messageType = "error"
	case exit != nil && *exit == 0:
		m.message = "update finished"
		m.messageType = "info"
	case exit != nil:
		m.message = fmt.Sprintf("update exit %d", *exit)
		m.messageType = "error"
	default:
		// closed while still running
		m.message = "update cancelled"
		m.messageType = "info"
	}
	return m
}

// --- staging actions ---

func (m *Model) stageCurrent() {
	skill := m.currentSkill()
	if skill == nil {
		return
	}
	for i, op := range m.stagedOps {
		if op.Source == skill.Source && op.SkillName == skill.Name {
			m.stagedOps = append(m.stagedOps[:i], m.stagedOps[i+1:]...)
			m.message = fmt.Sprintf("unstaged %s/%s", skill.Source, skill.Name)
			m.messageType = "info"
			return
		}
	}
	if skill.Protected {
		m.message = fmt.Sprintf("%s is protected and cannot be toggled", skill.Name)
		m.messageType = "error"
		return
	}
	op := skills.PlanOperation(*skill, m.sourceRoots[skill.Source], m.offRoot)
	m.stagedOps = append(m.stagedOps, op)
	m.message = fmt.Sprintf("staged %s %s/%s", op.Direction, skill.Source, skill.Name)
	m.messageType = "info"
}

func (m Model) applyAllStaged() (tea.Model, tea.Cmd) {
	pending := append([]skills.Operation(nil), m.stagedOps...)
	var applied []skills.Operation
	for i, op := range pending {
		if err := skills.ApplyOperation(op); err != nil {
			m.stagedOps = pending[i:]
			m.message = fmt.Sprintf("failed: %s/%s — %v", op.Source, op.SkillName, err)
			m.messageType = "error"
			return m, scanSkillsCmd(m)
		}
		applied = append(applied, op)
	}
	m.stagedOps = nil

	enabled, disabled := 0, 0
	for _, op := range applied {
		if op.Direction == "enable" {
			enabled++
		} else {
			disabled++
		}
	}
	var parts []string
	if enabled > 0 {
		parts = append(parts, fmt.Sprintf("enabled %d", enabled))
	}
	if disabled > 0 {
		parts = append(parts, fmt.Sprintf("disabled %d", disabled))
	}
	m.message = fmt.Sprintf("applied: %s", strings.Join(parts, ", "))
	m.messageType = "info"
	return m, scanSkillsCmd(m)
}

// --- panel/cursor mutation helpers ---

func (m Model) swapPanel(forward bool) Model {
	if m.active == panelEnabled {
		m.active = panelDisabled
	} else {
		m.active = panelEnabled
	}
	_ = forward
	m.clampSelection()
	return m
}

func (m Model) moveCursor(delta int) Model {
	return m.moveCursorBy(delta)
}

func (m Model) moveCursorBy(delta int) Model {
	if delta == 0 {
		return m
	}
	idx := m.currentIdx() + delta
	listSize := len(m.currentList())
	if listSize == 0 {
		return m
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= listSize {
		idx = listSize - 1
	}
	if m.active == panelDisabled {
		m.disabledIdx = idx
	} else {
		m.enabledIdx = idx
	}
	m.clampSelection()
	return m
}

func (m Model) cursorToEdge(top bool) Model {
	listSize := len(m.currentList())
	if listSize == 0 {
		return m
	}
	target := 0
	if !top {
		target = listSize - 1
	}
	if m.active == panelDisabled {
		m.disabledIdx = target
	} else {
		m.enabledIdx = target
	}
	m.clampSelection()
	return m
}

func (m Model) activePageSize() int {
	enabled, disabled := m.panelBodyHeights()
	if m.active == panelDisabled {
		return disabled
	}
	return enabled
}

func (m *Model) cycleSort() {
	switch m.sortMode {
	case skills.SortByName:
		m.sortMode = skills.SortByDescSizeDesc
	case skills.SortByDescSizeDesc:
		m.sortMode = skills.SortByDescSizeAsc
	default:
		m.sortMode = skills.SortByName
	}
	m.refreshLists()
}
