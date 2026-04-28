package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/catoncat/skill-toggle/internal/skills"
	"github.com/catoncat/skill-toggle/internal/ui"

	"github.com/charmbracelet/lipgloss"
)

// updateElapsed returns the wall-clock time since startUpdate fired (zero
// when the overlay has never been opened).
func (m Model) updateElapsed() time.Duration {
	if m.updateStartedAt.IsZero() {
		return 0
	}
	return time.Since(m.updateStartedAt)
}

// formatElapsed prints a duration like "12s" or "1m23s" — short enough to
// fit on the footer next to other status segments.
func formatElapsed(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	d = d.Truncate(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	mins := int(d / time.Minute)
	secs := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%dm%02ds", mins, secs)
}

// previewBreakpoint is the minimum width at which the right-hand preview
// panel is shown. Below this, the layout collapses to a single column and
// the preview is reachable with `p` / `Enter` / `Tab` (full-screen).
const previewBreakpoint = 120

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

	left := m.renderListPanel(m.leftWidth(), m.leftColumnHeight())
	var body string
	if m.showPreviewPane() {
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, m.renderRightColumn())
	} else {
		body = left
	}

	view := lipgloss.JoinVertical(lipgloss.Left, body, m.renderBottomStrip())
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

// --- left column: single Skills panel ---

func (m Model) leftWidth() int {
	if !m.showPreviewPane() {
		return m.width
	}
	min := 48
	// Preview is auxiliary (skim metadata + first body lines) — give the
	// list ~65% so long descriptions breathe.
	target := m.width * 65 / 100
	if target < min {
		target = min
	}
	// Floor for the right pane so metadata lines (paths, descriptions)
	// stay legible.
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
	h := m.height - chrome
	if h < 4 {
		return 4
	}
	return h
}

func (m Model) renderListPanel(width, height int) string {
	title := m.listTitle()
	rowWidth := width - 4
	nameColW := ui.NameColumnWidth(skillNames(m.visibleList), rowWidth)
	body := m.renderSkillRows(m.visibleList, m.idx, m.offset, rowWidth, nameColW, true, height-3)
	return ui.Panel(title, body, width, height, true)
}

// skillNames adapts a Skill slice into a name slice so the ui package
// doesn't need to import skills.
func skillNames(list []skills.Skill) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = s.Name
	}
	return out
}

// listTitle builds "Skills · <filter> (n)" or, when a search query is
// active, "Skills · <filter> (matched / total)  /<query>" so the user
// always sees both the filter axis and the search axis at a glance.
func (m Model) listTitle() string {
	label := "Skills"
	switch m.statusFilter {
	case filterEnabled:
		label = "Skills · enabled"
	case filterDisabled:
		label = "Skills · disabled"
	default:
		label = "Skills · all"
	}
	shown := len(m.visibleList)
	if m.query == "" {
		return fmt.Sprintf("%s (%d)", label, shown)
	}
	return fmt.Sprintf("%s (%d / %d)  /%s", label, shown, m.countByFilter(), m.query)
}

// countByFilter returns how many skills the current statusFilter would
// surface if the search query were cleared (i.e. the denominator shown in
// the title's "matched / total" form).
func (m Model) countByFilter() int {
	count := 0
	for _, s := range m.allSkills {
		switch m.statusFilter {
		case filterEnabled:
			if s.Status != "enabled" {
				continue
			}
		case filterDisabled:
			if s.Status != "disabled" {
				continue
			}
		}
		count++
	}
	return count
}

func (m Model) renderSkillRows(list []skills.Skill, idx, offset, rowWidth, nameColW int, active bool, rows int) string {
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
	sourceNames := m.sourceNames()
	end := offset + rows
	if end > len(list) {
		end = len(list)
	}
	out := make([]string, 0, rows)
	for i := offset; i < end; i++ {
		s := list[i]
		row := ui.SkillRow(
			s.Name, s.Presence, sourceNames, s.Description, s.DescriptionChars,
			s.Status,
			i == idx, active, m.isStaged(s),
			nameColW, rowWidth,
		)
		out = append(out, row)
	}
	for len(out) < rows {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}

// sourceNames extracts the names from the model's source list, in the
// same order they were configured. The presence column relies on this
// order to keep the a/c/x bitmap stable across re-renders.
func (m Model) sourceNames() []string {
	out := make([]string, len(m.sources))
	for i, s := range m.sources {
		out[i] = s.Name
	}
	return out
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
	if m.mode == modeSearch {
		return ui.SearchPrompt(m.query, width)
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
		parts = append(parts, fmt.Sprintf("marked enable %d", enabled))
	}
	if disabled > 0 {
		parts = append(parts, fmt.Sprintf("marked disable %d", disabled))
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
	return out
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
	if m.pendingConfirm == confirmLinkChoice {
		return m.renderLinkChoiceStrip(width)
	}

	question := ""
	switch m.pendingConfirm {
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
		question = fmt.Sprintf("Quit with %d enable / %d disable marked?", enabled, disabled)
	case confirmLinkSingle:
		if len(m.pendingLinkOps) == 0 {
			question = "Link?"
		} else {
			op := m.pendingLinkOps[0]
			verb := "Link"
			if op.Action == "unlink" {
				verb = "Unlink"
			}
			question = fmt.Sprintf("%s %s/%s?", verb, op.TargetSource, op.SkillName)
		}
	}
	prompt := ui.ConfirmPrompt(question)
	hint := ui.KeyHint("y", "yes") + "  " + ui.KeyHint("esc", "no")
	gap := width - lipgloss.Width(prompt) - lipgloss.Width(hint)
	if gap < 1 {
		gap = 1
	}
	return prompt + strings.Repeat(" ", gap) + hint
}

// renderLinkChoiceStrip composes the choice footer for the
// confirmLinkChoice flow: one "(c) ln target" or "(c) unlink target"
// hint per pending op (using the same a/c/x letters as the presence
// bitmap), plus an esc-to-cancel marker. Hints are trimmed from the
// right when the strip would overflow the terminal width so the cursor
// never wraps mid-row. Numeric 1/2/3 fallbacks are wired up in the
// dispatcher but not advertised in the strip — keeping the footer one
// shortcut per candidate avoids visual clutter, and users reaching for
// the number row will discover them on first try.
func (m Model) renderLinkChoiceStrip(width int) string {
	if len(m.pendingLinkOps) == 0 {
		return ui.ConfirmPrompt("Link?")
	}
	prompt := ui.ConfirmPrompt(fmt.Sprintf("Link / unlink %s?", m.pendingLinkOps[0].SkillName))
	hints := make([]string, 0, len(m.pendingLinkOps)+1)
	for _, op := range m.pendingLinkOps {
		verb := "ln"
		if op.Action == "unlink" {
			verb = "unlink"
		}
		key := string(ui.LetterForSource(op.TargetSource))
		hints = append(hints, ui.KeyHint(key, fmt.Sprintf("%s %s", verb, op.TargetSource)))
	}
	hints = append(hints, ui.KeyHint("esc", "cancel"))
	hintsLine := strings.Join(hints, "  ")

	gap := width - lipgloss.Width(prompt) - lipgloss.Width(hintsLine)
	if gap < 1 {
		// Drop hints from the right until the row fits.
		for len(hints) > 0 && lipgloss.Width(prompt)+2+lipgloss.Width(hintsLine) > width {
			hints = hints[:len(hints)-1]
			hintsLine = strings.Join(hints, "  ")
		}
		gap = width - lipgloss.Width(prompt) - lipgloss.Width(hintsLine)
		if gap < 1 {
			gap = 1
		}
	}
	return prompt + strings.Repeat(" ", gap) + hintsLine
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
		status = ui.MutedText(m.runningStatusText())
	case m.updateExit != nil && *m.updateExit == 0:
		status = ui.StatusMessage("done · exit 0 · "+formatElapsed(m.updateElapsed()), false)
	case m.updateExit != nil:
		status = ui.StatusMessage(fmt.Sprintf("failed · exit %d · %s", *m.updateExit, formatElapsed(m.updateElapsed())), true)
	default:
		status = ui.MutedText("starting…")
	}
	// Surface the scroll position so users don't think the stream froze
	// when they've scrolled up — `G` snaps back to the live tail.
	if m.updateScrollOffset > 0 {
		marker := ui.MutedText(fmt.Sprintf("[scrolled · %d ↑ · G to follow]", m.updateScrollOffset))
		if status == "" {
			status = marker
		} else {
			status = status + "  " + marker
		}
	}
	hint := ui.KeyHint("j/k", "scroll") + "  " + ui.KeyHint("g/G", "top/bot") + "  " + ui.KeyHint("esc/q", "close")
	gap := m.width - lipgloss.Width(status) - lipgloss.Width(hint)
	if gap < 1 {
		gap = 1
	}
	return status + strings.Repeat(" ", gap) + hint
}

// runningStatusText assembles the live footer for an in-flight update:
//
//	running 12s · checking 3/78 · waiting on github api 8s
//
// The waiting hint kicks in after ~5s of silence so users can tell a slow
// API call apart from a hung process.
func (m Model) runningStatusText() string {
	parts := []string{"running " + formatElapsed(m.updateElapsed())}
	if m.updatePhase != "" {
		parts = append(parts, m.updatePhase)
	}
	if !m.updateLastLineAt.IsZero() {
		idle := time.Since(m.updateLastLineAt)
		if idle > 5*time.Second {
			parts = append(parts, fmt.Sprintf("waiting on github api %s", formatElapsed(idle)))
		}
	}
	parts = append(parts, "esc to cancel")
	return strings.Join(parts, " · ")
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

// renderHelpBox builds the help overlay. Single-column would not fit a
// 24-row terminal (the four groups + headers run ~30 lines), so blocks are
// greedy-packed into the smallest column count whose tallest column still
// fits the terminal. The box outer width also snaps to the laid-out content
// width — a 250-col terminal no longer gets a 167-col help box.
func (m Model) renderHelpBox() string {
	entries := helpEntries()
	groupOrder := []string{}
	groupRows := map[string][]string{}
	for _, e := range entries {
		if _, ok := groupRows[e.Group]; !ok {
			groupOrder = append(groupOrder, e.Group)
		}
		groupRows[e.Group] = append(groupRows[e.Group], fmt.Sprintf("  %-15s %s", e.Key, e.Label))
	}

	titleLine := lipgloss.NewStyle().Foreground(ui.Accent).Bold(true).Render("skill-toggle keys")
	footerLine := ui.MutedText("press any key to dismiss")

	blocks := make([][]string, 0, len(groupOrder))
	blockW := 0
	for _, g := range groupOrder {
		b := []string{lipgloss.NewStyle().Bold(true).Render(g)}
		b = append(b, groupRows[g]...)
		blocks = append(blocks, b)
		for _, l := range b {
			if w := lipgloss.Width(l); w > blockW {
				blockW = w
			}
		}
	}

	const (
		gutter       = 4 // spacing between columns
		boxBorderW   = 2 // rounded border (left + right)
		boxPaddingW  = 4 // HelpOverlayBox padding (1, 2)
		boxBorderH   = 2
		boxPaddingH  = 2
		extraHChrome = 4 // title + spacer + spacer + footer
	)

	availH := m.height - boxBorderH - boxPaddingH - extraHChrome
	if availH < 4 {
		availH = 4
	}

	maxBoxOuterW := m.width
	if maxBoxOuterW < blockW+boxBorderW+boxPaddingW {
		maxBoxOuterW = blockW + boxBorderW + boxPaddingW
	}
	maxInnerW := maxBoxOuterW - boxBorderW - boxPaddingW
	if maxInnerW < blockW {
		maxInnerW = blockW
	}

	maxCols := (maxInnerW + gutter) / (blockW + gutter)
	if maxCols < 1 {
		maxCols = 1
	}
	if maxCols > len(blocks) {
		maxCols = len(blocks)
	}

	columns := packHelpBlocksIntoColumns(blocks, availH, maxCols)

	rendered := make([]string, len(columns))
	for i, c := range columns {
		rendered[i] = strings.Join(c, "\n")
	}
	var body string
	if len(rendered) == 1 {
		body = rendered[0]
	} else {
		cells := make([]string, 0, 2*len(rendered)-1)
		for i, r := range rendered {
			if i > 0 {
				cells = append(cells, strings.Repeat(" ", gutter))
			}
			cells = append(cells, r)
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, cells...)
	}

	bodyW := lipgloss.Width(body)
	contentW := bodyW
	if w := lipgloss.Width(titleLine); w > contentW {
		contentW = w
	}
	if w := lipgloss.Width(footerLine); w > contentW {
		contentW = w
	}

	boxOuterW := contentW + boxBorderW + boxPaddingW
	if boxOuterW > maxBoxOuterW {
		boxOuterW = maxBoxOuterW
	}

	full := titleLine + "\n\n" + body + "\n\n" + footerLine
	return ui.HelpOverlayBox(full, boxOuterW)
}

// packHelpBlocksIntoColumns greedily distributes blocks into N columns,
// trying N=1..maxCols, returning the smallest N where the tallest column is
// ≤ availH. If even maxCols overflows, packs into maxCols and lets the box
// be tall (caller may still truncate, but at least the layout is balanced).
func packHelpBlocksIntoColumns(blocks [][]string, availH, maxCols int) [][]string {
	for n := 1; n <= maxCols; n++ {
		if cols, ok := tryPackHelpColumns(blocks, n, availH); ok {
			return cols
		}
	}
	cols, _ := tryPackHelpColumns(blocks, maxCols, 1<<30)
	return cols
}

func tryPackHelpColumns(blocks [][]string, n, maxColH int) ([][]string, bool) {
	if n < 1 {
		n = 1
	}
	cols := make([][]string, 0, n)
	cur := []string{}
	curH := 0
	for _, b := range blocks {
		addH := len(b)
		if curH > 0 {
			addH++ // spacer line between blocks in the same column
		}
		if curH > 0 && curH+addH > maxColH {
			cols = append(cols, cur)
			if len(cols) >= n {
				return nil, false
			}
			cur = []string{}
			curH = 0
			addH = len(b)
		}
		if curH > 0 {
			cur = append(cur, "")
		}
		cur = append(cur, b...)
		curH = len(cur)
	}
	if len(cur) > 0 {
		if len(cols) >= n {
			return nil, false
		}
		cols = append(cols, cur)
	}
	return cols, true
}

// previewBodyHeight returns how many body rows the right panel can render
// when normal layout is in effect (used by full preview key handling).
func (m Model) previewBodyHeight() int {
	chrome := 1
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
