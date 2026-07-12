package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/catoncat/skill-toggle/internal/freshness"
	"github.com/catoncat/skill-toggle/internal/lockfile"
	"github.com/catoncat/skill-toggle/internal/skills"
	"github.com/catoncat/skill-toggle/internal/ui"
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

type freshnessCheckedMsg struct {
	key    string
	result freshness.Result
	err    error
}

func freshnessCheckCmd(checker *freshness.Checker, key string, entry *lockfile.Entry) tea.Cmd {
	return func() tea.Msg {
		res, err := checker.Check(entry)
		return freshnessCheckedMsg{key: key, result: res, err: err}
	}
}

type updateDoneMsg struct {
	result update.Result
}

// updateTickMsg fires once a second while modeUpdate is active so the
// elapsed clock in the footer keeps moving even when npx is waiting on
// the GitHub API and not producing new lines.
type updateTickMsg struct{}

func updateTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return updateTickMsg{} })
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

	case updateTickMsg:
		// Re-arm only while the overlay is still streaming; otherwise the
		// ticker dies naturally so we don't burn CPU after the user
		// dismisses the overlay.
		if m.mode == modeUpdate && m.updateRunning {
			return m, updateTickCmd()
		}
		return m, nil

	case freshnessCheckedMsg:
		if m.freshnessInflight != nil {
			delete(m.freshnessInflight, msg.key)
		}
		if msg.err == nil && m.freshnessCache != nil {
			m.freshnessCache[msg.key] = msg.result
		}
		m.message, m.messageType = formatFreshnessStatus(msg.key, msg.result, msg.err)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// appendUpdateLine stores a streamed line, keeps the buffer bounded so a
// runaway process doesn't grow memory unbounded, and parses the line for
// progress signals (phase + last-line timestamp) that the footer reads.
//
// When updateName is set (single-skill mode), it also rewrites the one
// vercel-labs/skills line that reads "✓ All global skills are up to date"
// — that wording is misleading when the user only updated one skill.
func (m *Model) appendUpdateLine(l update.Line) {
	text := l.Text
	if m.updateName != "" && !l.IsErr {
		text = rewriteSingleUpdateLine(text, m.updateName)
	}
	prefix := ""
	if l.IsErr {
		prefix = "[err] "
	}
	m.updateLines = append(m.updateLines, prefix+text)
	const maxLines = 4096
	if len(m.updateLines) > maxLines {
		m.updateLines = m.updateLines[len(m.updateLines)-maxLines:]
	}
	// Anchor the user to the absolute line they're currently reading: when
	// they've scrolled up, increment offset so the view stays put as new
	// lines arrive. Offset 0 keeps following the bottom (live tail).
	if m.updateScrollOffset > 0 {
		m.updateScrollOffset++
		m.clampUpdateScroll()
	}
	m.updateLastLineAt = time.Now()
	if phase := parseUpdatePhase(text); phase != "" {
		m.updatePhase = phase
	}
}

// updateInnerHeight returns the renderable rows of the modeUpdate body
// panel, mirroring renderUpdateScreen's dimension math so scroll bounds
// match what the user actually sees.
func (m Model) updateInnerHeight() int {
	panelHeight := m.height - 1
	if panelHeight < 4 {
		panelHeight = 4
	}
	inner := panelHeight - 3 // border (2) + title row
	if inner < 1 {
		inner = 1
	}
	return inner
}

// maxUpdateScroll is the largest valid updateScrollOffset — it puts the
// oldest line at the top of the visible window. When the buffer fits in
// one screen, no scrolling is possible and this returns 0.
func (m Model) maxUpdateScroll() int {
	n := len(m.updateLines) - m.updateInnerHeight()
	if n < 0 {
		return 0
	}
	return n
}

func (m *Model) clampUpdateScroll() {
	if max := m.maxUpdateScroll(); m.updateScrollOffset > max {
		m.updateScrollOffset = max
	}
	if m.updateScrollOffset < 0 {
		m.updateScrollOffset = 0
	}
}

// rewriteSingleUpdateLine swaps misleading "All global skills" phrasing
// for the actual targeted skill name so single-update users don't think
// the tool ran a global sweep.
func rewriteSingleUpdateLine(line, name string) string {
	if strings.Contains(line, "All global skills are up to date") {
		return "✓ " + name + " is already up to date"
	}
	return line
}

// checkingProgressRe extracts X/Y from "Checking global skill 3/78: name".
var checkingProgressRe = regexp.MustCompile(`Checking\s+global\s+skill\s+(\d+)/(\d+)`)

// updatingNameRe extracts <name> from "Updating <name>..." (vercel-labs
// emits this when a skill actually has a newer version to install).
var updatingNameRe = regexp.MustCompile(`^\s*Updating\s+(\S+)\.\.\.`)

// parseUpdatePhase extracts a short footer-friendly status from one line of
// vercel-labs/skills output. Returns "" when the line doesn't carry phase
// info — the caller keeps the previous phase rather than blanking it.
func parseUpdatePhase(line string) string {
	if m := checkingProgressRe.FindStringSubmatch(line); m != nil {
		return "checking " + m[1] + "/" + m[2]
	}
	if m := updatingNameRe.FindStringSubmatch(line); m != nil {
		return "updating " + m[1]
	}
	if strings.HasPrefix(strings.TrimSpace(line), "Found ") && strings.Contains(line, "update(s)") {
		return strings.TrimSpace(line)
	}
	return ""
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
	case tea.KeyTab, tea.KeyShiftTab:
		return m.openPreviewFull()
	case tea.KeyUp:
		return m.moveCursor(-1), nil
	case tea.KeyDown:
		return m.moveCursor(1), nil
	case tea.KeyPgUp:
		return m.moveCursorBy(-m.listPageSize()), nil
	case tea.KeyPgDown:
		return m.moveCursorBy(m.listPageSize()), nil
	case tea.KeySpace:
		m.stageCurrent()
		return m, nil
	case tea.KeyEnter:
		return m.openPreviewFull()
	case tea.KeyCtrlD:
		return m.moveCursorBy(m.listPageSize() / 2), nil
	case tea.KeyCtrlU:
		return m.moveCursorBy(-m.listPageSize() / 2), nil
	case tea.KeyEsc:
		// Esc clears the active search first (restoring the pre-search
		// scroll position), then status messages, so the user has a fast
		// path back from any "stuck" state without having to delete the
		// search query character by character.
		if m.query != "" || m.preSearchSaved {
			m.clearSearch(true /* restore */)
			m.message = "search cleared"
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
	case "a":
		return m.setFilter(filterAll), nil
	case "e":
		return m.setFilter(filterEnabled), nil
	case "d":
		return m.setFilter(filterDisabled), nil
	case "/":
		m.mode = modeSearch
		// Snapshot the current scroll position once, before any query is
		// typed, so cancelling search can put the list back exactly where
		// it was (including after paging). Subsequent `/` while a query is
		// already active keeps the original snapshot.
		m.savePreSearchPosition()
		return m, nil
	case "p":
		return m.openPreviewFull()
	case "?":
		m.mode = modeHelp
		return m, nil
	case "s":
		m.cycleSort()
		m.message = "sort: " + shortSortLabel(m.sortMode)
		m.messageType = "info"
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
		if !s.Managed {
			m.message = fmt.Sprintf("%s/%s not installed via `npx skills add` — manual update only", s.Source, s.Name)
			m.messageType = "error"
			return m, nil
		}
		m.pendingConfirm = confirmUpdate
		return m, nil
	case "U":
		m.pendingConfirm = confirmUpdateAll
		return m, nil
	case "F":
		return m.startFreshnessCheck()
	case "t":
		if len(m.stagedOps) > 0 {
			return m.applyAllStaged()
		}
		return m.toggleCurrent()
	case "C":
		if len(m.stagedOps) == 0 {
			return m, nil
		}
		m.stagedOps = nil
		m.message = "cleared marks"
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
	case "L":
		return m.handleLinkKey()
	}
	return m, nil
}

// handleLinkKey is fired by `L` in normal mode. It computes per-source
// link/unlink candidates against the cursor row's source and routes to
// the appropriate confirm flow: zero candidates surfaces a status
// message, one candidate uses confirmLinkSingle (y/N), and two-or-more
// use confirmLinkChoice (numbered keys 1..N).
func (m Model) handleLinkKey() (tea.Model, tea.Cmd) {
	skill := m.currentSkill()
	if skill == nil {
		return m, nil
	}
	if skill.Protected {
		m.message = fmt.Sprintf("%s is protected — link refused", skill.Name)
		m.messageType = "error"
		return m, nil
	}
	if skill.Status != "enabled" {
		m.message = fmt.Sprintf("link only applies to enabled skills (row is %s)", skill.Status)
		m.messageType = "error"
		return m, nil
	}

	candidates := m.planLinkCandidates(*skill)
	switch len(candidates) {
	case 0:
		m.message = "no link target — all sources covered"
		m.messageType = "info"
		return m, nil
	case 1:
		m.pendingLinkOps = candidates
		m.pendingConfirm = confirmLinkSingle
		return m, nil
	default:
		m.pendingLinkOps = candidates
		m.pendingConfirm = confirmLinkChoice
		return m, nil
	}
}

// planLinkCandidates builds the list of link/unlink ops that `L` exposes
// for a skill row. Sibling sources whose presence is "missing" become
// link candidates (create a symlink under that root); "link" siblings
// become unlink candidates (remove the existing symlink); "real"
// siblings are skipped — we never overwrite an independent real dir.
// Order follows m.sources so candidate numbering stays stable.
func (m Model) planLinkCandidates(skill skills.Skill) []skills.LinkOp {
	out := make([]skills.LinkOp, 0, len(m.sources))
	for _, src := range m.sources {
		if src.Name == skill.Source {
			continue
		}
		switch skill.Presence[src.Name] {
		case skills.PresenceMissing:
			out = append(out, skills.PlanLink(skill, src.Name, src.Root))
		case skills.PresenceLink:
			out = append(out, skills.PlanUnlink(skill, src.Name, src.Root))
		}
	}
	return out
}

// applyPendingLink dispatches one of the staged link/unlink ops, writes
// a status message, and triggers a rescan so Presence maps catch up.
func (m Model) applyPendingLink(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.pendingLinkOps) {
		m.pendingLinkOps = nil
		return m, nil
	}
	op := m.pendingLinkOps[idx]
	m.pendingLinkOps = nil
	if err := skills.ApplyLinkOp(op); err != nil {
		m.message = fmt.Sprintf("link failed: %v", err)
		m.messageType = "error"
		return m, scanSkillsCmd(m)
	}
	switch op.Action {
	case "link":
		m.message = fmt.Sprintf("linked %s/%s", op.TargetSource, op.SkillName)
	case "unlink":
		m.message = fmt.Sprintf("unlinked %s/%s", op.TargetSource, op.SkillName)
	}
	m.messageType = "info"
	return m, scanSkillsCmd(m)
}

// --- search mode ---

// savePreSearchPosition records idx/offset the first time a search session
// begins. Later keystrokes (or re-entering `/` with an existing query) leave
// the snapshot alone so cancel always returns to the original place.
func (m *Model) savePreSearchPosition() {
	if m.preSearchSaved {
		return
	}
	m.preSearchIdx = m.idx
	m.preSearchOffset = m.offset
	m.preSearchSaved = true
}

// clearSearch drops the query. When restore is true (esc / cancel), the
// pre-search cursor and viewport are put back; when false the caller has
// already decided the new position.
func (m *Model) clearSearch(restore bool) {
	m.query = ""
	if restore && m.preSearchSaved {
		m.idx = m.preSearchIdx
		m.offset = m.preSearchOffset
	}
	m.preSearchSaved = false
	m.refreshLists()
}

// setSearchQuery updates the live query. Entering a non-empty query always
// jumps the viewport to the first page of matches; clearing back to "" while
// still in search mode restores the snapshot without discarding it (so a
// later cancel / retype still knows the original place).
func (m *Model) setSearchQuery(q string) {
	wasEmpty := m.query == ""
	m.query = q
	switch {
	case q == "":
		if m.preSearchSaved {
			m.idx = m.preSearchIdx
			m.offset = m.preSearchOffset
		}
	default:
		if wasEmpty {
			m.savePreSearchPosition()
		}
		// New / refined search always starts at the top of the match list.
		m.idx = 0
		m.offset = 0
	}
	m.refreshLists()
}

func (m Model) handleSearchKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEnter:
		// Keep the query and the match-list position; snapshot stays so a
		// later esc in normal mode can still restore the pre-search place.
		m.mode = modeNormal
		return m, nil
	case tea.KeyEsc:
		m.mode = modeNormal
		m.clearSearch(true /* restore */)
		return m, nil
	case tea.KeyBackspace:
		if len(m.query) > 0 {
			m.setSearchQuery(m.query[:len(m.query)-1])
		}
		return m, nil
	case tea.KeyRunes:
		m.setSearchQuery(m.query + string(key.Runes))
		return m, nil
	case tea.KeySpace:
		m.setSearchQuery(m.query + " ")
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
	case tea.KeyEsc, tea.KeyTab, tea.KeyShiftTab:
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
	pending := m.pendingConfirm

	if key.Type == tea.KeyEsc {
		m.pendingConfirm = confirmNone
		m.pendingLinkOps = nil
		return m, nil
	}

	// Choice menu for link / unlink: semantic letters (c=claude, x=codex,
	// a=agents — matches the presence bitmap) take priority, with 1/2/3
	// kept as a positional fallback for users who reach for the number row.
	// Anything that doesn't match a candidate cancels the prompt, in keeping
	// with the "esc to bail" feel of the confirm flow.
	if pending == confirmLinkChoice {
		s := key.String()
		if len(s) == 1 {
			ch := rune(s[0])
			lower := unicode.ToLower(ch)
			for i, op := range m.pendingLinkOps {
				if ui.LetterForSource(op.TargetSource) == lower {
					m.pendingConfirm = confirmNone
					return m.applyPendingLink(i)
				}
			}
			if ch >= '1' && ch <= '9' {
				idx := int(ch - '1')
				if idx < len(m.pendingLinkOps) {
					m.pendingConfirm = confirmNone
					return m.applyPendingLink(idx)
				}
			}
		}
		m.pendingConfirm = confirmNone
		m.pendingLinkOps = nil
		return m, nil
	}

	if key.String() != "y" && key.String() != "Y" {
		m.pendingConfirm = confirmNone
		m.pendingLinkOps = nil
		return m, nil
	}

	m.pendingConfirm = confirmNone
	switch pending {
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
	case confirmLinkSingle:
		return m.applyPendingLink(0)
	}
	return m, nil
}

// startFreshnessCheck runs the F-key flow: validate the current row is a
// managed skill with a lockfile entry, hit the in-memory TTL cache, and
// fall back to a non-blocking GitHub Contents API request whose result
// arrives later as freshnessCheckedMsg. The status message line carries
// every outcome (cache hit, in-flight dedup, success, every error mode)
// so we don't need a dedicated overlay for what is one HTTP call.
func (m Model) startFreshnessCheck() (tea.Model, tea.Cmd) {
	s := m.currentSkill()
	if s == nil {
		return m, nil
	}
	if !s.Managed {
		m.message = fmt.Sprintf("%s/%s not managed by `npx skills add` — nothing to check", s.Source, s.Name)
		m.messageType = "error"
		return m, nil
	}
	if s.LockEntry == nil {
		m.message = fmt.Sprintf("%s/%s: missing lockfile entry", s.Source, s.Name)
		m.messageType = "error"
		return m, nil
	}

	key := s.Source + "/" + s.Name
	if cached, ok := m.freshnessCache[key]; ok && time.Since(cached.CheckedAt) < freshnessTTL {
		m.message, m.messageType = formatFreshnessStatus(key, cached, nil)
		return m, nil
	}
	if m.freshnessInflight[key] {
		m.message = fmt.Sprintf("%s: check already in progress", key)
		m.messageType = "info"
		return m, nil
	}
	if m.freshnessChecker == nil || m.freshnessInflight == nil {
		// Defensive: tests can construct a bare Model without going through
		// NewModel, in which case the freshness machinery is unwired.
		m.message = "freshness check unavailable"
		m.messageType = "error"
		return m, nil
	}

	m.freshnessInflight[key] = true
	m.message = fmt.Sprintf("checking %s upstream sha…", key)
	m.messageType = "info"
	return m, freshnessCheckCmd(m.freshnessChecker, key, s.LockEntry)
}

// formatFreshnessStatus turns a freshness.Result + error pair into the
// (message, messageType) pair the bottom strip wants. Used both for fresh
// API responses and cached hits so the wording stays identical.
func formatFreshnessStatus(key string, r freshness.Result, err error) (string, string) {
	if err != nil {
		switch err {
		case freshness.ErrRateLimited:
			return fmt.Sprintf("%s: github rate limited — set GH_TOKEN or GITHUB_TOKEN to lift the 60/h anonymous cap", key), "error"
		case freshness.ErrNotFound:
			return fmt.Sprintf("%s: upstream folder not found (lockfile path may be stale)", key), "error"
		case freshness.ErrUnsupported:
			return fmt.Sprintf("%s: source url not supported (only github.com)", key), "error"
		case freshness.ErrNoLockEntry:
			return fmt.Sprintf("%s: missing lockfile entry", key), "error"
		}
		return fmt.Sprintf("%s: check failed: %v", key, err), "error"
	}
	if r.UpToDate {
		return fmt.Sprintf("%s: up to date (sha %s)", key, shortSHA(r.RemoteSHA)), "info"
	}
	return fmt.Sprintf("%s: new version available — press u to update (local %s → remote %s)",
		key, shortSHA(r.LocalSHA), shortSHA(r.RemoteSHA)), "info"
}

func shortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// startUpdate kicks off `npx skills update` for one or all skills, switches
// the TUI into modeUpdate, and primes the streaming command. The user sees
// live stdout/stderr in a dedicated overlay until the process exits or
// they press Esc / q.
func (m Model) startUpdate(name string) (tea.Model, tea.Cmd) {
	m.mode = modeUpdate
	m.updateName = name
	m.updateLines = nil
	if name != "" {
		// Banner makes the scope obvious before vercel-labs/skills prints
		// "All global skills..." which sounds like a sweep otherwise.
		m.updateLines = append(m.updateLines, "→ scope: single skill ("+name+")")
	}
	m.updateRunning = true
	m.updateExit = nil
	m.updateErr = nil
	m.updateScrollOffset = 0
	m.updatePhase = ""
	now := time.Now()
	m.updateStartedAt = now
	m.updateLastLineAt = now

	lines, result, cancel, err := update.Start(name)
	if err != nil {
		m.updateRunning = false
		m.updateErr = err
		return m, nil
	}
	m.updateLinesCh = lines
	m.updateResultCh = result
	m.updateCancel = cancel
	// Batch the streaming wait with a 1s ticker so the elapsed clock and
	// "no output for Ns" hint stay live even when npx is blocked on a
	// GitHub API call (which is most of the time during the checking phase).
	return m, tea.Batch(waitUpdateCmd(lines, result), updateTickCmd())
}

// handleUpdateKey services keypresses while the update overlay is active.
//
// Scroll model: updateScrollOffset is the number of lines we've scrolled
// up from the bottom (the live tail). 0 follows the bottom; values above
// 0 freeze the view on a specific absolute line. Bounds are kept tight in
// clampUpdateScroll so the user can't accidentally scroll past either
// edge into an empty panel.
func (m Model) handleUpdateKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	page := m.updateInnerHeight()
	switch key.Type {
	case tea.KeyEsc:
		return m.closeUpdateOverlay(), nil
	case tea.KeyUp:
		m.updateScrollOffset++
		m.clampUpdateScroll()
		return m, nil
	case tea.KeyDown:
		m.updateScrollOffset--
		m.clampUpdateScroll()
		return m, nil
	case tea.KeyPgUp, tea.KeyCtrlU:
		m.updateScrollOffset += page
		m.clampUpdateScroll()
		return m, nil
	case tea.KeyPgDown, tea.KeyCtrlD:
		m.updateScrollOffset -= page
		m.clampUpdateScroll()
		return m, nil
	case tea.KeyHome:
		m.updateScrollOffset = m.maxUpdateScroll()
		return m, nil
	case tea.KeyEnd:
		m.updateScrollOffset = 0
		return m, nil
	}
	switch key.String() {
	case "q":
		return m.closeUpdateOverlay(), nil
	case "k":
		m.updateScrollOffset++
		m.clampUpdateScroll()
		return m, nil
	case "j":
		m.updateScrollOffset--
		m.clampUpdateScroll()
		return m, nil
	case "g":
		m.updateScrollOffset = m.maxUpdateScroll()
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
			m.message = fmt.Sprintf("unmarked %s/%s", skill.Source, skill.Name)
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
	m.message = fmt.Sprintf("marked %s %s/%s", op.Direction, skill.Source, skill.Name)
	m.messageType = "info"
}

// toggleCurrent flips the cursor row's skill immediately, with no
// staged-list detour. Used by `t` when nothing is selected — the user's
// mental model is "one action, applied to either the cursor row or the
// space-marked rows", so single-row toggle never needs the queue.
func (m Model) toggleCurrent() (tea.Model, tea.Cmd) {
	skill := m.currentSkill()
	if skill == nil {
		return m, nil
	}
	if skill.Protected {
		m.message = fmt.Sprintf("%s is protected and cannot be toggled", skill.Name)
		m.messageType = "error"
		return m, nil
	}
	op := skills.PlanOperation(*skill, m.sourceRoots[skill.Source], m.offRoot)
	if err := skills.ApplyOperation(op); err != nil {
		m.message = fmt.Sprintf("failed: %s/%s — %v", op.Source, op.SkillName, err)
		m.messageType = "error"
		return m, scanSkillsCmd(m)
	}
	if op.Direction == "enable" {
		m.message = fmt.Sprintf("enabled %s/%s", op.Source, op.SkillName)
	} else {
		m.message = fmt.Sprintf("disabled %s/%s", op.Source, op.SkillName)
	}
	m.messageType = "info"
	return m, scanSkillsCmd(m)
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
	m.message = fmt.Sprintf("toggled: %s", strings.Join(parts, ", "))
	m.messageType = "info"
	return m, scanSkillsCmd(m)
}

// --- cursor mutation helpers ---

func (m Model) moveCursor(delta int) Model {
	return m.moveCursorBy(delta)
}

func (m Model) moveCursorBy(delta int) Model {
	if delta == 0 {
		return m
	}
	listSize := len(m.visibleList)
	if listSize == 0 {
		return m
	}
	idx := m.idx + delta
	if idx < 0 {
		idx = 0
	}
	if idx >= listSize {
		idx = listSize - 1
	}
	m.idx = idx
	m.clampSelection()
	return m
}

func (m Model) cursorToEdge(top bool) Model {
	listSize := len(m.visibleList)
	if listSize == 0 {
		return m
	}
	if top {
		m.idx = 0
	} else {
		m.idx = listSize - 1
	}
	m.clampSelection()
	return m
}

func (m Model) listPageSize() int { return m.listBodyHeight() }

// setFilter switches the status-filter axis, preserving the cursor's
// underlying skill when possible so the view doesn't jump unexpectedly.
func (m Model) setFilter(f string) Model {
	if m.statusFilter == f {
		return m
	}
	prev := m.currentSkill()
	m.statusFilter = f
	m.refreshLists()
	if prev != nil {
		for i, s := range m.visibleList {
			if s.Source == prev.Source && s.Name == prev.Name {
				m.idx = i
				m.clampSelection()
				break
			}
		}
	}
	return m
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
