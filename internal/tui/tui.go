package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/catoncat/skill-toggle/internal/config"
	"github.com/catoncat/skill-toggle/internal/skills"
	"github.com/catoncat/skill-toggle/internal/ui"
	"github.com/catoncat/skill-toggle/internal/update"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- async message types ---

type skillsScannedMsg struct {
	skills []skills.Skill
	err    error
}

type updateFinishedMsg struct {
	name     string
	output   string
	exitCode int
	err      error
}

// --- Model ---

type Model struct {
	config  *config.Config
	profile *config.Profile

	// profile tabs
	profiles      []config.Profile
	activeProfile int

	// skills state
	allSkills      []skills.Skill
	filteredSkills []skills.Skill
	selected       int
	offset         int

	// ui mode
	mode string // normal, search, preview, confirm_update, confirm_update_all, confirm_quit, command

	// search / filter / sort
	query        string
	statusFilter string // all, enabled, disabled
	sortMode     string

	// preview
	previewOffset  int
	previewSkillMD string // raw SKILL.md content
	previewSkill   *skills.Skill

	// staged operations (Space to stage, A to apply)
	stagedOps []skills.Operation

	// command palette
	commandInput string

	// messages
	message     string
	messageType string // info, error

	// terminal dimensions
	width  int
	height int
}

// NewModel creates a new model with the given config and resolved profile.
func NewModel(cfg *config.Config, resolved *config.Profile) Model {
	// Sort profiles deterministically for tab display
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	profiles := make([]config.Profile, len(names))
	activeIdx := 0
	for i, name := range names {
		p := cfg.Profiles[name]
		if name == resolved.Name {
			// Use resolved paths (may include env overrides)
			p.Root = resolved.Root
			p.OffRoot = resolved.OffRoot
			p.OffRoots = resolved.OffRoots
			activeIdx = i
		}
		if len(p.OffRoots) == 0 {
			p.OffRoots = []string{p.OffRoot}
		}
		profiles[i] = p
	}

	return Model{
		config:        cfg,
		profile:       resolved,
		profiles:      profiles,
		activeProfile: activeIdx,
		mode:          "normal",
		statusFilter:  "all",
		sortMode:      skills.SortByName,
	}
}

// Run is the entry point for the Bubble Tea TUI.
func Run(cfg *config.Config, profile *config.Profile) error {
	m := NewModel(cfg, profile)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

// Init returns the initial command: scan skills for the active profile.
func (m Model) Init() tea.Cmd {
	return scanSkillsCmd(
		m.currentProfile().Root,
		m.currentProfile().OffRoots,
	)
}

// --- tea.Cmd factories ---

func scanSkillsCmd(liveRoot string, offRoots []string) tea.Cmd {
	return func() tea.Msg {
		all, err := skills.Scan(liveRoot, offRoots...)
		if err != nil {
			return skillsScannedMsg{err: err}
		}
		return skillsScannedMsg{skills: all}
	}
}

func runUpdateCmd(name string) tea.Cmd {
	return func() tea.Msg {
		output, exitCode, err := update.RunSkillsUpdate(name)
		return updateFinishedMsg{
			name:     name,
			output:   output,
			exitCode: exitCode,
			err:      err,
		}
	}
}

// --- helpers ---

func (m Model) currentProfile() config.Profile {
	if m.activeProfile < 0 || m.activeProfile >= len(m.profiles) {
		return config.Profile{}
	}
	profile := m.profiles[m.activeProfile]
	if len(profile.OffRoots) == 0 {
		profile.OffRoots = []string{profile.OffRoot}
	}
	return profile
}

func (m Model) isStaged(skill skills.Skill) bool {
	for _, op := range m.stagedOps {
		if op.SkillName == skill.Name {
			return true
		}
	}
	return false
}

func (m *Model) clampSelection() {
	if len(m.filteredSkills) == 0 {
		m.selected = 0
		m.offset = 0
		return
	}
	if m.selected >= len(m.filteredSkills) {
		m.selected = len(m.filteredSkills) - 1
	}
	visible := m.skillListHeight()
	if visible <= 0 {
		visible = 1
	}
	if m.selected < m.offset {
		m.offset = m.selected
	}
	if m.selected >= m.offset+visible {
		m.offset = m.selected - visible + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *Model) refreshFilteredSkills() {
	m.filteredSkills = skills.FilterSkills(m.allSkills, m.query, m.statusFilter, m.sortMode)
	m.clampSelection()
}

func (m Model) skillListHeight() int {
	h := m.height - 3 // tabs (1) + stats (1) + status bar (1)
	if m.mode == "search" {
		h-- // search bar
	}
	if h < 0 {
		h = 0
	}
	return h
}

func (m Model) stagingCountByDirection() (enableCount, disableCount int) {
	for _, op := range m.stagedOps {
		if op.Direction == "enable" {
			enableCount++
		} else {
			disableCount++
		}
	}
	return
}

// --- Update ---

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampSelection()
		return m, nil

	case skillsScannedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Scan error: %v", msg.err)
			m.messageType = "error"
			return m, nil
		}
		m.allSkills = msg.skills
		m.refreshFilteredSkills()
		return m, nil

	case updateFinishedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Update failed: %v", msg.err)
			m.messageType = "error"
		} else if msg.exitCode != 0 {
			output := strings.TrimSpace(msg.output)
			m.message = fmt.Sprintf("Update failed (exit %d): %s", msg.exitCode, output)
			m.messageType = "error"
		} else {
			output := strings.TrimSpace(msg.output)
			if len(output) > 200 {
				output = output[:200] + "..."
			}
			m.message = output
			m.messageType = "info"
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	default:
		return m, nil
	}
}

func (m Model) handleKeyMsg(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+C always quits
	if key.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	switch m.mode {
	case "normal":
		return m.handleNormalModeKey(key)
	case "search":
		return m.handleSearchModeKey(key)
	case "preview":
		return m.handlePreviewModeKey(key)
	case "confirm_update":
		return m.handleConfirmUpdateKey(key)
	case "confirm_update_all":
		return m.handleConfirmUpdateAllKey(key)
	case "confirm_quit":
		return m.handleConfirmQuitKey(key)
	case "command":
		return m.handleCommandModeKey(key)
	default:
		return m, nil
	}
}

// --- normal mode key handling ---

func (m Model) handleNormalModeKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyTab:
		m.activeProfile = (m.activeProfile + 1) % len(m.profiles)
		m.stagedOps = nil
		m.message = ""
		m.selected = 0
		m.offset = 0
		m.previewSkill = nil
		m.previewSkillMD = ""
		return m, scanSkillsCmd(
			m.currentProfile().Root,
			m.currentProfile().OffRoots,
		)

	case tea.KeyShiftTab:
		m.activeProfile = (m.activeProfile - 1 + len(m.profiles)) % len(m.profiles)
		m.stagedOps = nil
		m.message = ""
		m.selected = 0
		m.offset = 0
		m.previewSkill = nil
		m.previewSkillMD = ""
		return m, scanSkillsCmd(
			m.currentProfile().Root,
			m.currentProfile().OffRoots,
		)

	case tea.KeyUp:
		if m.selected > 0 {
			m.selected--
			m.clampSelection()
		}
		return m, nil

	case tea.KeyDown:
		if m.selected < len(m.filteredSkills)-1 {
			m.selected++
			m.clampSelection()
		}
		return m, nil

	case tea.KeyPgUp:
		visible := m.skillListHeight()
		if visible < 1 {
			visible = 1
		}
		m.selected -= visible
		if m.selected < 0 {
			m.selected = 0
		}
		m.clampSelection()
		return m, nil

	case tea.KeyPgDown:
		visible := m.skillListHeight()
		if visible < 1 {
			visible = 1
		}
		m.selected += visible
		if m.selected >= len(m.filteredSkills) {
			m.selected = len(m.filteredSkills) - 1
		}
		m.clampSelection()
		return m, nil

	case tea.KeySpace:
		return m.handleStageToggle()

	case tea.KeyEnter:
		if len(m.filteredSkills) > 0 {
			return m.enterPreviewMode()
		}
		return m, nil

	case tea.KeyEscape:
		return m, nil
	}

	switch key.String() {
	case "q":
		if len(m.stagedOps) > 0 {
			m.mode = "confirm_quit"
		} else {
			return m, tea.Quit
		}
		return m, nil

	case "/":
		m.mode = "search"
		m.query = ""
		return m, nil

	case ":":
		m.mode = "command"
		m.commandInput = ""
		return m, nil

	case "s":
		switch m.sortMode {
		case skills.SortByName:
			m.sortMode = skills.SortByDescSizeDesc
		case skills.SortByDescSizeDesc:
			m.sortMode = skills.SortByDescSizeAsc
		case skills.SortByDescSizeAsc:
			m.sortMode = skills.SortByName
		default:
			m.sortMode = skills.SortByName
		}
		m.refreshFilteredSkills()
		return m, nil

	case "a":
		m.statusFilter = "all"
		m.refreshFilteredSkills()
		return m, nil

	case "e":
		m.statusFilter = "enabled"
		m.refreshFilteredSkills()
		return m, nil

	case "d":
		m.statusFilter = "disabled"
		m.refreshFilteredSkills()
		return m, nil

	case "r":
		m.message = ""
		return m, scanSkillsCmd(
			m.currentProfile().Root,
			m.currentProfile().OffRoots,
		)

	case "p":
		if len(m.filteredSkills) > 0 {
			return m.enterPreviewMode()
		}
		return m, nil

	case "u":
		if len(m.filteredSkills) > 0 {
			skill := m.filteredSkills[m.selected]
			if skill.Status == "enabled" {
				m.mode = "confirm_update"
			} else {
				m.message = fmt.Sprintf("Cannot update disabled skill: %s", skill.Name)
				m.messageType = "error"
			}
		}
		return m, nil

	case "U":
		m.mode = "confirm_update_all"
		return m, nil

	case "A":
		if len(m.stagedOps) == 0 {
			m.message = "No staged operations to apply"
			m.messageType = "info"
			return m, nil
		}
		return m.applyAllStaged()

	case "j":
		if m.selected < len(m.filteredSkills)-1 {
			m.selected++
			m.clampSelection()
		}
		return m, nil

	case "k":
		if m.selected > 0 {
			m.selected--
			m.clampSelection()
		}
		return m, nil
	}

	return m, nil
}

// --- stage toggle ---

func (m Model) handleStageToggle() (tea.Model, tea.Cmd) {
	if len(m.filteredSkills) == 0 {
		return m, nil
	}

	m.stageSkill(m.filteredSkills[m.selected])
	return m, nil
}

func (m *Model) stageSkill(skill skills.Skill) {
	// Check if already staged — toggle off
	for i, op := range m.stagedOps {
		if op.SkillName == skill.Name {
			m.stagedOps = append(m.stagedOps[:i], m.stagedOps[i+1:]...)
			m.message = fmt.Sprintf("unstaged %s", skill.Name)
			m.messageType = "info"
			return
		}
	}

	// Protected skills cannot be staged
	if skill.Protected {
		m.message = fmt.Sprintf("%s is protected and cannot be toggled", skill.Name)
		m.messageType = "error"
		return
	}

	// Determine direction and target
	var direction, targetRoot string
	if skill.Status == "enabled" {
		direction = "disable"
		targetRoot = m.currentProfile().OffRoot
	} else {
		direction = "enable"
		targetRoot = m.currentProfile().Root
	}

	m.stagedOps = append(m.stagedOps, skills.Operation{
		SkillName:  skill.Name,
		Direction:  direction,
		SourcePath: skill.Path,
		TargetPath: filepath.Join(targetRoot, skill.Name),
	})
	m.message = fmt.Sprintf("staged %s %s", direction, skill.Name)
	m.messageType = "info"
}

func (m Model) findSkill(name, status string) (skills.Skill, bool) {
	for _, skill := range m.allSkills {
		if skill.Status != status {
			continue
		}
		if skill.Name == name || skill.DisplayName == name {
			return skill, true
		}
	}
	return skills.Skill{}, false
}

// --- apply staged ---

func (m Model) applyAllStaged() (tea.Model, tea.Cmd) {
	// Clone the staged ops list so we can track what succeeded
	ops := make([]skills.Operation, len(m.stagedOps))
	copy(ops, m.stagedOps)

	var applied []skills.Operation
	for _, op := range ops {
		if err := skills.ApplyOperation(op); err != nil {
			// Keep unapplied ops in stagedOps
			m.stagedOps = ops[len(applied):]
			m.message = fmt.Sprintf("Failed to %s %s: %v", op.Direction, op.SkillName, err)
			m.messageType = "error"
			return m, scanSkillsCmd(
				m.currentProfile().Root,
				m.currentProfile().OffRoots,
			)
		}
		applied = append(applied, op)
	}

	// All succeeded
	m.stagedOps = nil
	enableCount, disableCount := 0, 0
	for _, op := range applied {
		if op.Direction == "enable" {
			enableCount++
		} else {
			disableCount++
		}
	}

	var parts []string
	if enableCount > 0 {
		parts = append(parts, fmt.Sprintf("enabled %d", enableCount))
	}
	if disableCount > 0 {
		parts = append(parts, fmt.Sprintf("disabled %d", disableCount))
	}
	m.message = fmt.Sprintf("Applied: %s", strings.Join(parts, ", "))
	m.messageType = "info"

	return m, scanSkillsCmd(
		m.currentProfile().Root,
		m.currentProfile().OffRoots,
	)
}

// --- enter preview mode ---

func (m Model) enterPreviewMode() (tea.Model, tea.Cmd) {
	skill := m.filteredSkills[m.selected]
	skillMDPath := filepath.Join(skill.Path, "SKILL.md")
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		m.previewSkillMD = fmt.Sprintf("(error reading SKILL.md: %v)", err)
	} else {
		m.previewSkillMD = string(data)
	}
	m.previewSkill = &skill
	m.previewOffset = 0
	m.mode = "preview"
	return m, nil
}

// --- search mode key handling ---

func (m Model) handleSearchModeKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEnter, tea.KeyEscape:
		m.mode = "normal"
		m.query = ""
		m.refreshFilteredSkills()
		return m, nil

	case tea.KeyBackspace:
		if len(m.query) > 0 {
			m.query = m.query[:len(m.query)-1]
			m.refreshFilteredSkills()
		}
		return m, nil

	case tea.KeyRunes:
		m.query += string(key.Runes)
		m.refreshFilteredSkills()
		return m, nil
	}
	return m, nil
}

// --- preview mode key handling ---

func (m Model) handlePreviewModeKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	lines := m.previewLines()
	totalLines := len(lines)
	previewHeight := m.height - 5

	switch key.Type {
	case tea.KeyUp:
		if m.previewOffset > 0 {
			m.previewOffset--
		}
		return m, nil

	case tea.KeyDown:
		maxOffset := totalLines - previewHeight
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.previewOffset < maxOffset {
			m.previewOffset++
		}
		return m, nil

	case tea.KeyPgUp:
		m.previewOffset -= previewHeight
		if m.previewOffset < 0 {
			m.previewOffset = 0
		}
		return m, nil

	case tea.KeyPgDown:
		maxOffset := totalLines - previewHeight
		if maxOffset < 0 {
			maxOffset = 0
		}
		m.previewOffset += previewHeight
		if m.previewOffset > maxOffset {
			m.previewOffset = maxOffset
		}
		return m, nil

	case tea.KeyEscape:
		m.mode = "normal"
		m.previewSkill = nil
		m.previewSkillMD = ""
		m.previewOffset = 0
		return m, nil
	}

	switch key.String() {
	case "q", "p":
		m.mode = "normal"
		m.previewSkill = nil
		m.previewSkillMD = ""
		m.previewOffset = 0
		return m, nil

	case "j":
		maxOffset := totalLines - previewHeight
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.previewOffset < maxOffset {
			m.previewOffset++
		}
		return m, nil

	case "k":
		if m.previewOffset > 0 {
			m.previewOffset--
		}
		return m, nil
	}

	return m, nil
}

func (m Model) previewLines() []string {
	if m.previewSkill == nil {
		return nil
	}

	skill := m.previewSkill
	var b strings.Builder

	b.WriteString(fmt.Sprintf("name: %s\n", skill.Name))
	b.WriteString(fmt.Sprintf("status: %s\n", skill.Status))
	b.WriteString(fmt.Sprintf("path: %s\n", skill.Path))
	b.WriteString(fmt.Sprintf("desc_chars: %d\n", skill.DescriptionChars))
	b.WriteString(fmt.Sprintf("symlink: %v\n\n", skill.IsSymlink))
	b.WriteString("--- SKILL.md ---\n")
	if m.previewSkillMD != "" {
		b.WriteString(m.previewSkillMD)
	}

	return strings.Split(b.String(), "\n")
}

// --- confirm update mode ---

func (m Model) handleConfirmUpdateKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Type == tea.KeyRunes && string(key.Runes) == "y" {
		m.mode = "normal"
		if m.selected < len(m.filteredSkills) {
			skill := m.filteredSkills[m.selected]
			m.message = fmt.Sprintf("Updating %s...", skill.Name)
			m.messageType = "info"
			return m, tea.Batch(
				runUpdateCmd(skill.Name),
				scanSkillsCmd(
					m.currentProfile().Root,
					m.currentProfile().OffRoots,
				),
			)
		}
		return m, nil
	}
	m.mode = "normal"
	return m, nil
}

// --- confirm update all mode ---

func (m Model) handleConfirmUpdateAllKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Type == tea.KeyRunes && string(key.Runes) == "y" {
		m.mode = "normal"
		m.message = "Updating all skills..."
		m.messageType = "info"
		return m, tea.Batch(
			runUpdateCmd(""),
			scanSkillsCmd(
				m.currentProfile().Root,
				m.currentProfile().OffRoots,
			),
		)
	}
	m.mode = "normal"
	return m, nil
}

// --- confirm quit mode ---

func (m Model) handleConfirmQuitKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Type == tea.KeyRunes && string(key.Runes) == "y" {
		return m, tea.Quit
	}
	m.mode = "normal"
	return m, nil
}

// --- command palette mode ---

func (m Model) handleCommandModeKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEscape:
		m.mode = "normal"
		m.commandInput = ""
		return m, nil

	case tea.KeyEnter:
		return m.executeCommand()

	case tea.KeyBackspace:
		if len(m.commandInput) > 0 {
			m.commandInput = m.commandInput[:len(m.commandInput)-1]
		}
		return m, nil

	case tea.KeyRunes:
		m.commandInput += string(key.Runes)
		return m, nil
	}
	return m, nil
}

func (m Model) executeCommand() (tea.Model, tea.Cmd) {
	cmd := strings.TrimSpace(m.commandInput)
	m.mode = "normal"
	m.commandInput = ""

	if cmd == "" {
		return m, nil
	}

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return m, nil
	}

	action := strings.ToLower(parts[0])

	switch action {
	case "disable":
		if len(parts) < 2 {
			m.message = "Usage: disable <skill-name>"
			m.messageType = "error"
			return m, nil
		}
		name := parts[1]
		skill, ok := m.findSkill(name, "enabled")
		if !ok {
			m.message = fmt.Sprintf("enabled skill not found: %s", name)
			m.messageType = "error"
			return m, nil
		}
		m.stageSkill(skill)
		return m, nil

	case "enable":
		if len(parts) < 2 {
			m.message = "Usage: enable <skill-name>"
			m.messageType = "error"
			return m, nil
		}
		name := parts[1]
		skill, ok := m.findSkill(name, "disabled")
		if !ok {
			m.message = fmt.Sprintf("disabled skill not found: %s", name)
			m.messageType = "error"
			return m, nil
		}
		m.stageSkill(skill)
		return m, nil

	case "profile":
		if len(parts) < 2 {
			m.message = "Usage: profile <profile-name>"
			m.messageType = "error"
			return m, nil
		}
		name := parts[1]
		for i, p := range m.profiles {
			if p.Name == name {
				m.activeProfile = i
				m.stagedOps = nil
				m.message = ""
				m.selected = 0
				m.offset = 0
				return m, scanSkillsCmd(
					m.currentProfile().Root,
					m.currentProfile().OffRoots,
				)
			}
		}
		m.message = fmt.Sprintf("Unknown profile: %s", name)
		m.messageType = "error"
		return m, nil

	case "quit", "q":
		if len(m.stagedOps) > 0 {
			m.mode = "confirm_quit"
			return m, nil
		}
		return m, tea.Quit

	default:
		m.message = fmt.Sprintf("Unknown command: %s", action)
		m.messageType = "error"
		return m, nil
	}
}

// --- View ---

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var parts []string

	// 1. Tabs bar
	parts = append(parts, m.renderTabs())

	// 2. Search bar (only in search mode)
	if m.mode == "search" {
		parts = append(parts, m.renderSearchBar())
	}

	// 3. Stats bar
	parts = append(parts, m.renderStats())

	// 4. Main content: skill list (+ preview pane in preview mode)
	parts = append(parts, m.renderContent())

	// 5. Footer / status bar
	parts = append(parts, m.renderFooter())

	view := lipgloss.JoinVertical(lipgloss.Top, parts...)

	// 6. Overlays
	switch m.mode {
	case "confirm_update":
		view = m.renderConfirmUpdateOverlay(view)
	case "confirm_update_all":
		view = m.renderConfirmUpdateAllOverlay(view)
	case "confirm_quit":
		view = m.renderConfirmQuitOverlay(view)
	case "command":
		view = m.renderCommandOverlay(view)
	}

	return view
}

// --- render: tabs ---

func (m Model) renderTabs() string {
	var b strings.Builder
	b.WriteString("Profiles:")
	for i, p := range m.profiles {
		style := ui.TabStyle(i == m.activeProfile)
		b.WriteString("  " + style.Render(p.Name))
	}
	return ui.TabBarStyle().Render(b.String())
}

// --- render: search bar ---

func (m Model) renderSearchBar() string {
	return ui.SearchInputStyle().Render(fmt.Sprintf("Search: %s_", m.query))
}

// --- render: stats ---

func (m Model) renderStats() string {
	enabledCount := 0
	disabledCount := 0
	totalDescChars := 0
	for _, s := range m.allSkills {
		if s.Status == "enabled" {
			enabledCount++
		} else {
			disabledCount++
		}
		totalDescChars += s.DescriptionChars
	}

	stagedPart := ""
	if len(m.stagedOps) > 0 {
		stagedPart = fmt.Sprintf("  Staged %d", len(m.stagedOps))
	}

	sortIndicator := ""
	switch m.sortMode {
	case skills.SortByDescSizeDesc:
		sortIndicator = " [desc size v]"
	case skills.SortByDescSizeAsc:
		sortIndicator = " [desc size ^]"
	}

	filterIndicator := ""
	switch m.statusFilter {
	case "enabled":
		filterIndicator = " [enabled]"
	case "disabled":
		filterIndicator = " [disabled]"
	}

	text := fmt.Sprintf("Enabled %d  Off %d  Desc chars %s%s%s%s",
		enabledCount, disabledCount, ui.FormatDescChars(totalDescChars),
		stagedPart, sortIndicator, filterIndicator)

	return ui.StatsStyle().Render(text)
}

// --- render: content area ---

func (m Model) renderContent() string {
	if m.mode == "preview" {
		return m.renderSplitContent()
	}
	return m.renderSkillList(m.width)
}

func (m Model) renderSplitContent() string {
	listWidth := m.width * 40 / 100
	previewWidth := m.width - listWidth
	if listWidth < 10 {
		listWidth = 10
	}
	if previewWidth < 20 {
		previewWidth = 20
	}

	listView := m.renderSkillList(listWidth)
	previewView := m.renderPreviewPane(previewWidth)

	return lipgloss.JoinHorizontal(lipgloss.Top, listView, previewView)
}

// --- render: skill list ---

func (m Model) renderSkillList(width int) string {
	if width < 10 {
		return ""
	}

	visible := m.skillListHeight()
	if visible <= 0 {
		return ""
	}

	// Column layout
	const badgeWidth = 5 // "[ON ]" or "[OFF]" + trailing space
	const nameWidth = 26
	const descCharsWidth = 6

	// If the terminal is narrow, shrink name column
	actualNameWidth := nameWidth
	metaWidth := badgeWidth + actualNameWidth + descCharsWidth
	if metaWidth > width {
		actualNameWidth = width - badgeWidth - descCharsWidth
		if actualNameWidth < 5 {
			actualNameWidth = 5
		}
	}
	descWidth := width - badgeWidth - actualNameWidth - descCharsWidth
	if descWidth < 0 {
		descWidth = 0
	}

	if len(m.filteredSkills) == 0 {
		msg := "No skills found"
		if m.query != "" {
			msg = "No skills match your search"
		}
		padding := (width - len(msg)) / 2
		if padding < 0 {
			padding = 0
		}
		return strings.Repeat("\n", visible/2) + strings.Repeat(" ", padding) + msg
	}

	start := m.offset
	end := start + visible
	if end > len(m.filteredSkills) {
		end = len(m.filteredSkills)
	}

	var out strings.Builder
	out.Grow((end - start) * (width + 1))
	for i := start; i < end; i++ {
		skill := m.filteredSkills[i]
		isSelected := i == m.selected

		var rowBuf strings.Builder
		rowBuf.Grow(width + 10)

		// Staged indicator
		staged := m.isStaged(skill)
		if staged {
			rowBuf.WriteString("~")
		} else {
			rowBuf.WriteString(" ")
		}

		// Badge
		if skill.Status == "enabled" {
			rowBuf.WriteString(ui.EnabledBadge().Render("ON") + " ")
		} else {
			rowBuf.WriteString(ui.DisabledBadge().Render("OFF") + " ")
		}

		// Name
		displayName := skill.Name
		if skill.IsSymlink && !staged {
			namePart := ui.TrimToWidth(displayName, actualNameWidth-1)
			rowBuf.WriteString(ui.PadRight(namePart, actualNameWidth-1) + "@")
		} else {
			rowBuf.WriteString(ui.PadRight(ui.TrimToWidth(displayName, actualNameWidth), actualNameWidth))
		}

		// Desc chars
		descChars := ui.FormatDescChars(skill.DescriptionChars)
		rowBuf.WriteString(ui.PadRight(descChars, descCharsWidth))

		// Description
		if descWidth > 0 {
			rowBuf.WriteString(ui.TrimToWidth(skill.Description, descWidth))
		}

		row := rowBuf.String()

		if isSelected {
			row = ui.SkillRowStyle(true, skill.Status).Render(ui.PadRight(row, width))
		}

		out.WriteString(row)
		if i < end-1 {
			out.WriteString("\n")
		}
	}

	// Pad bottom
	rendered := end - start
	result := out.String()
	if rendered < visible {
		result += strings.Repeat("\n", visible-rendered)
	}

	return result
}

// --- render: preview pane ---

func (m Model) renderPreviewPane(width int) string {
	if m.previewSkill == nil {
		return ""
	}

	previewHeight := m.height - 5
	if previewHeight < 5 {
		previewHeight = 5
	}

	lines := m.previewLines()
	totalLines := len(lines)

	maxOffset := totalLines - previewHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.previewOffset > maxOffset {
		m.previewOffset = maxOffset
	}

	end := m.previewOffset + previewHeight
	if end > totalLines {
		end = totalLines
	}

	var visibleLines []string
	for _, line := range lines[m.previewOffset:end] {
		visibleLines = append(visibleLines, ui.TrimToWidth(line, width))
	}
	for len(visibleLines) < previewHeight {
		visibleLines = append(visibleLines, "")
	}

	content := ui.PreviewContentStyle().Render(strings.Join(visibleLines, "\n"))
	title := ui.PreviewTitleStyle().Render(fmt.Sprintf("Preview: %s", m.previewSkill.Name))

	scrollInfo := ""
	if totalLines > previewHeight {
		pct := 0
		if totalLines > previewHeight {
			pct = (m.previewOffset * 100) / (totalLines - previewHeight)
		}
		scrollInfo = fmt.Sprintf("  [%d%%]", pct)
	}
	footer := fmt.Sprintf("Line %d/%d%s", m.previewOffset+1, totalLines, scrollInfo)

	result := lipgloss.JoinVertical(lipgloss.Top, title, content)
	result += "\n" + ui.FooterStyle().Render(footer)

	return result
}

// --- render: footer / status bar ---

func (m Model) renderFooter() string {
	left := m.mode

	hints := "j/k move  Space stage  A apply  p preview  s sort  / search  a/e/d filter  u update  U update-all  q quit"
	switch m.mode {
	case "preview":
		hints = "j/k scroll  q/p/Esc exit"
	case "search":
		hints = "type to search  Enter/Esc done"
	case "command":
		hints = "type command  Enter execute  Esc cancel"
	case "confirm_update", "confirm_update_all", "confirm_quit":
		hints = "y confirm  else cancel"
	}

	leftRendered := ui.TitleStyle().Render(left)

	var rightRendered string
	availableHintWidth := m.width - lipgloss.Width(leftRendered) - 2
	if m.message != "" {
		if m.messageType == "error" {
			rightRendered = ui.ErrorStyle().Render(ui.TrimToWidth(m.message, availableHintWidth))
		} else {
			rightRendered = ui.InfoStyle().Render(ui.TrimToWidth(m.message, availableHintWidth))
		}
	} else {
		rightRendered = ui.InfoStyle().Render(ui.TrimToWidth(hints, availableHintWidth))
	}

	middle := availableHintWidth - lipgloss.Width(rightRendered)
	if middle < 0 {
		middle = 0
		rightRendered = ""
	}

	return ui.FooterStyle().Render(
		leftRendered + strings.Repeat(" ", middle) + rightRendered,
	)
}

// --- overlay renderers ---

func (m Model) renderConfirmUpdateOverlay(view string) string {
	return m.renderOverlay(view, "Update this skill? (y/N)")
}

func (m Model) renderConfirmUpdateAllOverlay(view string) string {
	return m.renderOverlay(view, "Update ALL enabled skills? (y/N)")
}

func (m Model) renderConfirmQuitOverlay(view string) string {
	enableCount, disableCount := m.stagingCountByDirection()
	var msg string
	if enableCount > 0 || disableCount > 0 {
		var parts []string
		if enableCount > 0 {
			parts = append(parts, fmt.Sprintf("%d to enable", enableCount))
		}
		if disableCount > 0 {
			parts = append(parts, fmt.Sprintf("%d to disable", disableCount))
		}
		msg = fmt.Sprintf("Unapplied changes: %s. Quit? (y/N)", strings.Join(parts, ", "))
	} else {
		msg = "Quit? (y/N)"
	}
	return m.renderOverlay(view, msg)
}

func (m Model) renderCommandOverlay(view string) string {
	prompt := ":" + m.commandInput + "_"
	dialog := ui.CommandPaletteStyle().Render(prompt)
	return m.renderDialogOverlay(view, dialog)
}

func (m Model) renderOverlay(view, msg string) string {
	dialog := ui.ConfirmDialogStyle().Render(msg)
	return m.renderDialogOverlay(view, dialog)
}

func (m Model) renderDialogOverlay(view, dialog string) string {
	dialogWidth := lipgloss.Width(dialog)
	dialogHeight := strings.Count(dialog, "\n") + 1

	if dialogWidth > m.width {
		dialogWidth = m.width
	}

	x := (m.width - dialogWidth) / 2
	if x < 0 {
		x = 0
	}

	y := (m.height - dialogHeight) / 2
	if y < 0 {
		y = 0
	}

	lines := strings.Split(view, "\n")
	if len(lines) < y+dialogHeight {
		extra := make([]string, (y+dialogHeight)-len(lines))
		lines = append(lines, extra...)
	}

	dialogLines := strings.Split(dialog, "\n")
	for i, dl := range dialogLines {
		lineIdx := y + i
		if lineIdx >= len(lines) {
			break
		}
		line := lines[lineIdx]
		if x >= len(line) {
			// Line is shorter than x position; pad it
			line = line + strings.Repeat(" ", x-len(line))
		}
		// Replace region with dialog text
		maxDL := len(line) - x
		if len(dl) > maxDL {
			dl = dl[:maxDL]
		}
		before := line[:x]
		after := ""
		if x+len(dl) < len(line) {
			after = line[x+len(dl):]
		}
		lines[lineIdx] = before + dl + after
	}

	return strings.Join(lines, "\n")
}
