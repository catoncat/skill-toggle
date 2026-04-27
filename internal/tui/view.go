package tui

import (
	"fmt"
	"strings"

	"github.com/catoncat/skill-toggle/internal/skills"
	"github.com/catoncat/skill-toggle/internal/ui"

	"github.com/charmbracelet/lipgloss"
)

// previewBreakpoint is the minimum width at which the right-hand preview
// panel is shown. Below this, the layout collapses to a single column and
// the preview is reachable with `p` (full-screen).
const previewBreakpoint = 100

// View is the central tea.Model render entry point.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	switch m.mode {
	case modeHelp:
		return m.renderHelpScreen()
	case modePreviewFull:
		return m.renderFullPreview()
	case modeUpdate:
		return m.renderUpdateScreen()
	}

	left := m.renderLeftColumn()
	var body string
	if m.showPreviewPane() {
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, m.renderRightColumn())
	} else {
		body = left
	}

	parts := []string{}
	if m.mode == modeSearch {
		parts = append(parts, ui.SearchPrompt(m.query, m.width))
	}
	parts = append(parts, body, m.renderBottomStrip())

	view := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return m.padToHeight(view)
}

// showPreviewPane reports whether the right-hand preview should be drawn
// in the current layout. Below the breakpoint, only the left column is
// rendered so the rows have room to breathe.
func (m Model) showPreviewPane() bool {
	return m.width >= previewBreakpoint
}

// padToHeight makes sure the rendered output never exceeds the terminal
// height (Bubble Tea will draw extra lines as scroll).
func (m Model) padToHeight(view string) string {
	lines := strings.Split(view, "\n")
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	return strings.Join(lines, "\n")
}

// --- left column: Enabled + Disabled panels stacked vertically ---

func (m Model) leftWidth() int {
	if !m.showPreviewPane() {
		return m.width
	}
	min := 48
	target := m.width * 55 / 100
	if target < min {
		target = min
	}
	if target > m.width-30 {
		target = m.width - 30
	}
	if target < 1 {
		target = 1
	}
	return target
}

func (m Model) rightWidth() int {
	if !m.showPreviewPane() {
		return 0
	}
	r := m.width - m.leftWidth()
	if r < 1 {
		r = 1
	}
	return r
}

func (m Model) leftColumnHeight() int {
	chrome := 1
	if m.mode == modeSearch {
		chrome++
	}
	h := m.height - chrome
	if h < 4 {
		return 4
	}
	return h
}

func (m Model) renderLeftColumn() string {
	totalH := m.leftColumnHeight()
	enabledH := totalH / 2
	disabledH := totalH - enabledH
	w := m.leftWidth()

	enabled := m.renderEnabledPanel(w, enabledH)
	disabled := m.renderDisabledPanel(w, disabledH)
	return lipgloss.JoinVertical(lipgloss.Left, enabled, disabled)
}

func (m Model) renderEnabledPanel(width, height int) string {
	title := m.panelTitle("Enabled", len(m.enabledList), m.countByStatus("enabled"))
	body := m.renderSkillRows(m.enabledList, m.enabledIdx, m.enabledOffset, width-4, m.active == panelEnabled, height-3)
	return ui.Panel(title, body, width, height, m.active == panelEnabled)
}

func (m Model) renderDisabledPanel(width, height int) string {
	title := m.panelTitle("Disabled", len(m.disabledList), m.countByStatus("disabled"))
	body := m.renderSkillRows(m.disabledList, m.disabledIdx, m.disabledOffset, width-4, m.active == panelDisabled, height-3)
	return ui.Panel(title, body, width, height, m.active == panelDisabled)
}

// panelTitle builds "<label> (n)" or, when a search query is active,
// "<label> (matched / total) /<query>" so the user always sees the current
// filter without having to remember they had typed something.
func (m Model) panelTitle(label string, shown, total int) string {
	if m.query == "" {
		return fmt.Sprintf("%s (%d)", label, shown)
	}
	return fmt.Sprintf("%s (%d / %d)  /%s", label, shown, total, m.query)
}

func (m Model) countByStatus(status string) int {
	count := 0
	for _, s := range m.allSkills {
		if s.Status == status {
			count++
		}
	}
	return count
}

func (m Model) renderSkillRows(list []skills.Skill, idx, offset, rowWidth int, active bool, rows int) string {
	if rows < 1 {
		rows = 1
	}
	if rowWidth < 10 {
		rowWidth = 10
	}
	if len(list) == 0 {
		empty := "no skills"
		if m.query != "" {
			empty = "no matches"
		}
		filler := make([]string, rows)
		filler[0] = ui.MutedText(empty)
		return strings.Join(filler, "\n")
	}
	end := offset + rows
	if end > len(list) {
		end = len(list)
	}
	out := make([]string, 0, rows)
	for i := offset; i < end; i++ {
		s := list[i]
		row := ui.SkillRow(
			s.Name, s.Source, s.Description, s.DescriptionChars,
			s.Status,
			i == idx, active, m.isStaged(s),
			rowWidth,
		)
		out = append(out, row)
	}
	for len(out) < rows {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}

// --- right column: preview panel ---

func (m Model) renderRightColumn() string {
	w := m.rightWidth()
	h := m.leftColumnHeight()
	body, title := m.previewBody(w - 4)
	return ui.Panel(title, body, w, h, false)
}

// previewBody returns (body, title) for the preview panel rendered at the
// given inner width. The title is dynamic (based on the active skill).
func (m Model) previewBody(innerWidth int) (string, string) {
	skill := m.currentSkill()
	if skill == nil {
		return ui.MutedText("no skill selected"), "Preview"
	}
	title := fmt.Sprintf("Preview · %s/%s", skill.Source, skill.Name)
	lines := m.previewMetadataLines(*skill, innerWidth)
	lines = append(lines, "", ui.MutedText(strings.Repeat("─", innerWidth)))
	bodyLines := m.skillMDBodyLines(*skill, innerWidth)
	lines = append(lines, bodyLines...)
	return strings.Join(lines, "\n"), title
}

// previewMetadataLines builds the small metadata header for a skill.
func (m Model) previewMetadataLines(skill skills.Skill, innerWidth int) []string {
	symlink := "no"
	if skill.IsSymlink {
		symlink = "yes"
	}
	return []string{
		ui.PreviewMetadataLine("name   ", skill.DisplayName, innerWidth),
		ui.PreviewMetadataLine("source ", skill.Source, innerWidth),
		ui.PreviewMetadataLine("status ", skill.Status, innerWidth),
		ui.PreviewMetadataLine("path   ", ui.TruncateLeft(skill.Path, innerWidth-7), innerWidth),
		ui.PreviewMetadataLine("desc   ", fmt.Sprintf("%d chars", skill.DescriptionChars), innerWidth),
		ui.PreviewMetadataLine("symlink", symlink, innerWidth),
	}
}

// skillMDBodyLines returns the SKILL.md body, lazily loaded only when needed
// in normal mode (full preview mode keeps it in m.previewSkillMD).
func (m Model) skillMDBodyLines(skill skills.Skill, innerWidth int) []string {
	body := m.previewSkillMD
	if m.previewSkill == nil || m.previewSkill.Source != skill.Source || m.previewSkill.Name != skill.Name {
		body = m.lazyLoadSkillMD(skill)
	}
	if body == "" {
		return []string{ui.MutedText("(SKILL.md is empty)")}
	}
	src := strings.Split(body, "\n")
	out := make([]string, 0, len(src))
	for _, line := range src {
		out = append(out, ui.PreviewBodyLine(line, innerWidth))
	}
	return out
}

func (m Model) lazyLoadSkillMD(skill skills.Skill) string {
	// Cheap inline load — used for normal-mode right pane preview. Ignored
	// errors are surfaced via a placeholder line.
	if skill.Path == "" {
		return ""
	}
	mdPath := skill.Path + "/SKILL.md"
	data, err := readFileSafe(mdPath)
	if err != nil {
		return fmt.Sprintf("(error reading SKILL.md: %v)", err)
	}
	return data
}

// --- full-screen preview ---

func (m Model) renderFullPreview() string {
	chrome := 1
	bodyHeight := m.height - chrome
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	skill := m.previewSkill
	title := "Preview"
	if skill != nil {
		title = fmt.Sprintf("Preview · %s/%s", skill.Source, skill.Name)
	}
	innerWidth := m.width - 4
	if innerWidth < 1 {
		innerWidth = 1
	}

	var allLines []string
	if skill != nil {
		allLines = append(allLines, m.previewMetadataLines(*skill, innerWidth)...)
		allLines = append(allLines, "", ui.MutedText(strings.Repeat("─", innerWidth)))
	}
	bodyLines := strings.Split(m.previewSkillMD, "\n")
	for _, line := range bodyLines {
		allLines = append(allLines, ui.PreviewBodyLine(line, innerWidth))
	}

	previewBodyHeight := bodyHeight - 3 // border + title rows
	if previewBodyHeight < 1 {
		previewBodyHeight = 1
	}
	maxOffset := len(allLines) - previewBodyHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	offset := m.previewOffset
	if offset > maxOffset {
		offset = maxOffset
	}
	end := offset + previewBodyHeight
	if end > len(allLines) {
		end = len(allLines)
	}
	visible := allLines[offset:end]
	for len(visible) < previewBodyHeight {
		visible = append(visible, "")
	}

	body := strings.Join(visible, "\n")
	if len(allLines) > previewBodyHeight {
		body += "\n" + ui.MutedText(fmt.Sprintf("Line %d–%d / %d", offset+1, end, len(allLines)))
	}
	panel := ui.Panel(title, body, m.width, bodyHeight, true)
	return panel + "\n" + m.renderBottomStrip()
}

// --- bottom strip ---

func (m Model) renderBottomStrip() string {
	width := m.width
	if width < 1 {
		return ""
	}
	if m.pendingConfirm != confirmNone {
		return m.renderConfirmStrip(width)
	}
	hints := keyStripHints(m)
	rendered := make([]string, 0, len(hints))
	for _, h := range hints {
		rendered = append(rendered, ui.KeyHint(h.Key, h.Label))
	}
	hintsLine := strings.Join(rendered, "  ")

	left := m.renderStatusSegment()
	if left != "" && lipgloss.Width(left)+2+lipgloss.Width(hintsLine) > width {
		// Trim hints first, then status, until they fit.
		for len(rendered) > 0 && lipgloss.Width(left)+2+lipgloss.Width(hintsLine) > width {
			rendered = rendered[:len(rendered)-1]
			hintsLine = strings.Join(rendered, "  ")
		}
		if lipgloss.Width(left)+2+lipgloss.Width(hintsLine) > width {
			left = ui.TrimToWidth(left, width-2-lipgloss.Width(hintsLine))
		}
	} else if left == "" && lipgloss.Width(hintsLine) > width {
		hintsLine = ui.TrimToWidth(hintsLine, width)
	}
	if left == "" {
		return ui.PadRight(hintsLine, width)
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(hintsLine)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + hintsLine
}

func (m Model) renderStatusSegment() string {
	enabled, disabled := m.stageCounts()
	parts := []string{}
	if enabled > 0 {
		parts = append(parts, fmt.Sprintf("staged enable %d", enabled))
	}
	if disabled > 0 {
		parts = append(parts, fmt.Sprintf("staged disable %d", disabled))
	}
	staging := ""
	if len(parts) > 0 {
		staging = ui.StatusMessage(strings.Join(parts, " · "), false)
	}

	var out string
	if m.message != "" {
		text := m.message
		if m.messageType == "error" {
			out = ui.StatusMessage(text, true)
		} else {
			out = ui.StatusMessage(text, false)
		}
	}
	if staging != "" {
		if out != "" {
			out += "  " + staging
		} else {
			out = staging
		}
	}
	if out == "" {
		out = ui.MutedText(fmt.Sprintf("%s · sort=%s", m.activeSummary(), shortSortLabel(m.sortMode)))
	}
	return out
}

func (m Model) activeSummary() string {
	if m.active == panelDisabled {
		return "Disabled focus"
	}
	return "Enabled focus"
}

func shortSortLabel(s string) string {
	switch s {
	case skills.SortByDescSizeDesc:
		return "size↓"
	case skills.SortByDescSizeAsc:
		return "size↑"
	default:
		return "name"
	}
}

func (m Model) renderConfirmStrip(width int) string {
	question := ""
	switch m.pendingConfirm {
	case confirmApply:
		enabled, disabled := m.stageCounts()
		question = fmt.Sprintf("Apply %d enable / %d disable?", enabled, disabled)
	case confirmUpdate:
		s := m.currentSkill()
		if s == nil {
			question = "Update this skill?"
		} else {
			question = fmt.Sprintf("Update %s/%s?", s.Source, s.Name)
		}
	case confirmUpdateAll:
		question = "Update ALL global skills?"
	case confirmQuit:
		enabled, disabled := m.stageCounts()
		question = fmt.Sprintf("Quit with %d enable / %d disable staged?", enabled, disabled)
	}
	prompt := ui.ConfirmPrompt(question)
	hint := ui.KeyHint("y", "yes") + "  " + ui.KeyHint("esc", "no")
	gap := width - lipgloss.Width(prompt) - lipgloss.Width(hint)
	if gap < 1 {
		gap = 1
	}
	return prompt + strings.Repeat(" ", gap) + hint
}

// --- update screen (full-screen, modeUpdate) ---

func (m Model) renderUpdateScreen() string {
	title := "Updating all skills"
	if m.updateName != "" {
		title = "Updating " + m.updateName
	}
	panelHeight := m.height - 1
	if panelHeight < 4 {
		panelHeight = 4
	}
	innerHeight := panelHeight - 3 // border (2) + title row
	if innerHeight < 1 {
		innerHeight = 1
	}
	innerWidth := m.width - 4
	if innerWidth < 1 {
		innerWidth = 1
	}

	visible := m.updateVisibleLines(innerHeight, innerWidth)
	body := strings.Join(visible, "\n")
	panel := ui.Panel(title, body, m.width, panelHeight, true)

	footer := m.renderUpdateFooter()
	return panel + "\n" + footer
}

// updateVisibleLines returns the slice of streamed lines that should be
// rendered in the available innerHeight rows. Auto-anchors to the bottom
// (most-recent line) when updateScrollOffset == 0.
func (m Model) updateVisibleLines(innerHeight, innerWidth int) []string {
	pad := func(s string) string {
		s = ui.TrimToWidth(s, innerWidth)
		return s
	}

	if len(m.updateLines) == 0 {
		hint := "(no output yet)"
		if m.updateErr != nil {
			hint = "failed to start: " + m.updateErr.Error()
		}
		out := []string{ui.MutedText(hint)}
		for len(out) < innerHeight {
			out = append(out, "")
		}
		return out
	}

	totalLines := len(m.updateLines)
	end := totalLines - m.updateScrollOffset
	if end < 0 {
		end = 0
	}
	if end > totalLines {
		end = totalLines
	}
	start := end - innerHeight
	if start < 0 {
		start = 0
	}
	slice := m.updateLines[start:end]
	out := make([]string, 0, innerHeight)
	for _, line := range slice {
		out = append(out, pad(updateLineStyle(line)))
	}
	for len(out) < innerHeight {
		out = append(out, "")
	}
	return out
}

func updateLineStyle(line string) string {
	if strings.HasPrefix(line, "[err] ") {
		return ui.StatusMessage(line, true)
	}
	switch {
	case strings.Contains(line, "✓"), strings.Contains(line, "Found "), strings.Contains(line, "up-to-date"):
		return ui.StatusMessage(line, false)
	case strings.Contains(line, "✗"), strings.Contains(line, "failed"):
		return ui.StatusMessage(line, true)
	}
	return line
}

func (m Model) renderUpdateFooter() string {
	var status string
	switch {
	case m.updateErr != nil:
		status = ui.StatusMessage("failed: "+m.updateErr.Error(), true)
	case m.updateRunning:
		status = ui.MutedText("running…  esc to cancel & close")
	case m.updateExit != nil && *m.updateExit == 0:
		status = ui.StatusMessage("done · exit 0", false)
	case m.updateExit != nil:
		status = ui.StatusMessage(fmt.Sprintf("failed · exit %d", *m.updateExit), true)
	default:
		status = ui.MutedText("starting…")
	}
	hint := ui.KeyHint("j/k", "scroll") + "  " + ui.KeyHint("g/G", "top/bot") + "  " + ui.KeyHint("esc/q", "close")
	gap := m.width - lipgloss.Width(status) - lipgloss.Width(hint)
	if gap < 1 {
		gap = 1
	}
	return status + strings.Repeat(" ", gap) + hint
}

// --- help screen (full-screen, takes over layout in modeHelp) ---

func (m Model) renderHelpScreen() string {
	box := m.renderHelpBox()
	boxLines := strings.Split(box, "\n")
	boxH := len(boxLines)
	boxW := 0
	for _, l := range boxLines {
		if w := lipgloss.Width(l); w > boxW {
			boxW = w
		}
	}

	canvas := make([]string, m.height)
	blank := strings.Repeat(" ", m.width)
	for i := range canvas {
		canvas[i] = blank
	}

	y := (m.height - boxH) / 2
	if y < 0 {
		y = 0
	}
	x := (m.width - boxW) / 2
	if x < 0 {
		x = 0
	}

	for i, l := range boxLines {
		idx := y + i
		if idx >= len(canvas) {
			break
		}
		left := strings.Repeat(" ", x)
		line := left + l
		fillW := m.width - lipgloss.Width(line)
		if fillW > 0 {
			line += strings.Repeat(" ", fillW)
		}
		canvas[idx] = line
	}
	return strings.Join(canvas, "\n")
}

func (m Model) renderHelpBox() string {
	entries := helpEntries()
	groups := []string{}
	groupBody := map[string][]string{}
	for _, e := range entries {
		if _, ok := groupBody[e.Group]; !ok {
			groups = append(groups, e.Group)
		}
		groupBody[e.Group] = append(groupBody[e.Group], fmt.Sprintf("  %-18s %s", e.Key, e.Label))
	}
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(ui.Accent).Bold(true).Render("skill-toggle keys"))
	lines = append(lines, "")
	for _, g := range groups {
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render(g))
		lines = append(lines, groupBody[g]...)
		lines = append(lines, "")
	}
	lines = append(lines, ui.MutedText("press any key to dismiss"))
	width := m.width * 2 / 3
	if width < 50 {
		width = 50
	}
	if width > m.width-4 {
		width = m.width - 4
	}
	return ui.HelpOverlayBox(strings.Join(lines, "\n"), width)
}

// previewBodyHeight returns how many body rows the right panel can render
// when normal layout is in effect (used by full preview key handling).
func (m Model) previewBodyHeight() int {
	chrome := 1
	if m.mode == modeSearch {
		chrome++
	}
	avail := m.height - chrome - 3 // panel border + title rows
	if m.mode == modePreviewFull {
		avail = m.height - 1 - 3
	}
	if avail < 1 {
		avail = 1
	}
	return avail
}

// previewLineCount returns the total line count of the current preview
// content (used by full preview key handling for max offset).
func (m Model) previewLineCount() int {
	if m.previewSkill == nil {
		return 0
	}
	innerWidth := m.width - 4
	if innerWidth < 1 {
		innerWidth = 1
	}
	header := m.previewMetadataLines(*m.previewSkill, innerWidth)
	bodyLines := strings.Split(m.previewSkillMD, "\n")
	return len(header) + 2 + len(bodyLines)
}
