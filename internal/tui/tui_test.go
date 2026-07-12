package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/catoncat/skill-toggle/internal/freshness"
	"github.com/catoncat/skill-toggle/internal/lockfile"
	"github.com/catoncat/skill-toggle/internal/skills"
	"github.com/catoncat/skill-toggle/internal/update"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func newTestModel(width, height int, all []skills.Skill) Model {
	m := Model{
		sources: []skills.Source{
			{Name: "agents", Root: "/tmp/agents/skills"},
			{Name: "claude", Root: "/tmp/claude/skills"},
			{Name: "codex", Root: "/tmp/codex/skills"},
		},
		sourceRoots: map[string]string{
			"agents": "/tmp/agents/skills",
			"claude": "/tmp/claude/skills",
			"codex":  "/tmp/codex/skills",
		},
		offRoot:      "/tmp/off",
		mode:         modeNormal,
		sortMode:     skills.SortByName,
		statusFilter: filterAll,
		allSkills:    all,
		width:        width,
		height:       height,
	}
	m.refreshLists()
	return m
}

func makeSkill(source, name, status, desc string) skills.Skill {
	path := "/tmp/" + source + "/skills/" + name
	if status == "disabled" {
		path = "/tmp/off/" + source + "/" + name
	}
	return skills.Skill{
		Name:             name,
		Source:           source,
		DisplayName:      name,
		Description:      desc,
		DescriptionChars: len(desc),
		Status:           status,
		Path:             path,
	}
}

func TestNewModelSeedsLayoutDefaults(t *testing.T) {
	m := NewModel()
	if m.mode != modeNormal {
		t.Errorf("expected normal mode, got %s", m.mode)
	}
	if m.statusFilter != filterAll {
		t.Errorf("expected statusFilter=all, got %s", m.statusFilter)
	}
	if m.sortMode != skills.SortByName {
		t.Errorf("expected sort=name, got %s", m.sortMode)
	}
	if len(m.sources) != 3 {
		t.Errorf("expected 3 sources, got %d", len(m.sources))
	}
}

func TestRefreshListsRespectsStatusFilter(t *testing.T) {
	all := []skills.Skill{
		makeSkill("agents", "alpha", "enabled", "first"),
		makeSkill("claude", "beta", "disabled", "second"),
		makeSkill("agents", "gamma", "enabled", "third"),
	}
	m := newTestModel(120, 32, all)
	if len(m.visibleList) != 3 {
		t.Errorf("filterAll: expected 3 visible, got %d", len(m.visibleList))
	}

	m.statusFilter = filterEnabled
	m.refreshLists()
	if len(m.visibleList) != 2 {
		t.Errorf("filterEnabled: expected 2 visible, got %d", len(m.visibleList))
	}

	m.statusFilter = filterDisabled
	m.refreshLists()
	if len(m.visibleList) != 1 || m.visibleList[0].Name != "beta" {
		t.Errorf("filterDisabled: expected [beta], got %#v", m.visibleList)
	}
}

func TestStageCurrentTogglesAndUnstages(t *testing.T) {
	m := newTestModel(120, 32, []skills.Skill{
		makeSkill("agents", "demo", "enabled", "desc"),
	})
	m.stageCurrent()
	if len(m.stagedOps) != 1 {
		t.Fatalf("expected 1 staged op, got %d", len(m.stagedOps))
	}
	if m.stagedOps[0].Direction != "disable" {
		t.Errorf("expected disable, got %s", m.stagedOps[0].Direction)
	}
	// Frontmatter-based: TargetPath should be empty (no physical move).
	if m.stagedOps[0].TargetPath != "" {
		t.Errorf("unexpected target %s, expected empty (frontmatter-based)", m.stagedOps[0].TargetPath)
	}
	m.stageCurrent()
	if len(m.stagedOps) != 0 {
		t.Fatalf("expected unstage, got %d ops", len(m.stagedOps))
	}
}

func TestSetFilterPreservesCursorWhenSkillStillVisible(t *testing.T) {
	m := newTestModel(120, 32, []skills.Skill{
		makeSkill("agents", "alpha", "enabled", "a"),
		makeSkill("claude", "beta", "disabled", "b"),
		makeSkill("agents", "gamma", "enabled", "c"),
	})
	// Locate gamma in the visible list (default sort puts enabled first then
	// by name, so order is alpha, gamma, beta).
	gammaIdx := -1
	for i, s := range m.visibleList {
		if s.Name == "gamma" {
			gammaIdx = i
			break
		}
	}
	if gammaIdx < 0 {
		t.Fatalf("setup: gamma missing from visible list")
	}
	m.idx = gammaIdx
	m = m.setFilter(filterEnabled)
	if got := m.currentSkill(); got == nil || got.Name != "gamma" {
		t.Fatalf("expected cursor preserved on gamma after filter, got %#v", got)
	}
}

func TestMoveCursorRespectsBounds(t *testing.T) {
	m := newTestModel(120, 32, []skills.Skill{
		makeSkill("agents", "a", "enabled", "x"),
		makeSkill("agents", "b", "enabled", "x"),
		makeSkill("agents", "c", "enabled", "x"),
	})
	m = m.moveCursor(1)
	if m.idx != 1 {
		t.Errorf("expected idx=1, got %d", m.idx)
	}
	m = m.moveCursor(99)
	if m.idx != 2 {
		t.Errorf("expected clamp at 2, got %d", m.idx)
	}
	m = m.moveCursor(-99)
	if m.idx != 0 {
		t.Errorf("expected clamp at 0, got %d", m.idx)
	}
}

func TestCycleSortRotatesModes(t *testing.T) {
	m := newTestModel(120, 32, nil)
	m.cycleSort()
	if m.sortMode != skills.SortByDescSizeDesc {
		t.Errorf("expected size-desc, got %s", m.sortMode)
	}
	m.cycleSort()
	if m.sortMode != skills.SortByDescSizeAsc {
		t.Errorf("expected size-asc, got %s", m.sortMode)
	}
	m.cycleSort()
	if m.sortMode != skills.SortByName {
		t.Errorf("expected name (cycled), got %s", m.sortMode)
	}
}

func TestViewDoesNotOverflowWidth(t *testing.T) {
	m := newTestModel(120, 32, []skills.Skill{
		makeSkill("agents", "alpha-skill-with-a-very-long-name", "enabled", "A long description meant to exercise truncation."),
		makeSkill("claude", "beta-skill", "disabled", "Another long description for the disabled side."),
	})
	view := m.View()
	for i, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > m.width {
			t.Fatalf("line %d width %d > %d: %q", i, got, m.width, line)
		}
	}
}

func TestViewFitsRequestedHeight(t *testing.T) {
	all := []skills.Skill{}
	for i := 0; i < 25; i++ {
		status := "enabled"
		if i%3 == 0 {
			status = "disabled"
		}
		all = append(all, makeSkill("agents", "name-"+string(rune('a'+i)), status, "row"))
	}
	m := newTestModel(80, 24, all)
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) > m.height {
		t.Fatalf("view has %d lines, > %d", len(lines), m.height)
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got > m.width {
			t.Fatalf("line %d width %d > %d", i, got, m.width)
		}
	}
}

func TestSearchModeFiltersVisibleList(t *testing.T) {
	m := newTestModel(120, 32, []skills.Skill{
		makeSkill("agents", "cloudflare", "enabled", "cloudflare global"),
		makeSkill("claude", "session-wrap", "enabled", "session wrap"),
		makeSkill("claude", "ctf-web", "disabled", "ctf web"),
	})
	m.mode = modeSearch
	m.query = "cloud"
	m.refreshLists()
	if len(m.visibleList) != 1 || m.visibleList[0].Name != "cloudflare" {
		t.Fatalf("expected [cloudflare], got %#v", m.visibleList)
	}
}

func TestNarrowScreenHidesPreviewPane(t *testing.T) {
	m := newTestModel(100, 24, []skills.Skill{
		makeSkill("agents", "alpha", "enabled", "alpha description"),
	})
	view := m.View()
	if strings.Contains(view, "Preview") {
		t.Fatal("narrow screen should hide preview panel — full preview is via 'p'")
	}
}

func TestWideScreenShowsPreviewPane(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		makeSkill("agents", "alpha", "enabled", "alpha description"),
	})
	view := m.View()
	if !strings.Contains(view, "Preview") {
		t.Fatalf("wide screen should render preview panel: %q", view)
	}
}

func TestSearchQueryEchoesInPanelTitle(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		makeSkill("agents", "cloudflare", "enabled", "cf"),
	})
	m.query = "cloud"
	m.refreshLists()
	view := m.View()
	if !strings.Contains(view, "/cloud") {
		t.Fatalf("expected /cloud in panel title, got: %q", view)
	}
}

func TestEscClearsQueryInNormalMode(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		makeSkill("agents", "cloudflare", "enabled", "cf"),
	})
	m.query = "cloud"
	m.refreshLists()
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyEsc})
	nm := next.(Model)
	if nm.query != "" {
		t.Fatalf("expected query cleared after esc, got %q", nm.query)
	}
}

func TestSearchResetsToFirstPageAndEscRestoresScroll(t *testing.T) {
	// Enough rows that paging past the first screen is possible.
	all := make([]skills.Skill, 0, 40)
	for i := 0; i < 40; i++ {
		name := fmt.Sprintf("skill-%02d", i)
		all = append(all, makeSkill("agents", name, "enabled", "desc"))
	}
	// Sprinkle a match near the top so a search still has results after
	// the user has scrolled far down the unfiltered list.
	all = append(all, makeSkill("agents", "needle-item", "enabled", "special"))

	m := newTestModel(80, 20, all)
	body := m.listBodyHeight()
	if body < 2 {
		t.Fatalf("list body too small for paging test: %d", body)
	}

	// Page down a few times so idx/offset leave the top.
	m = m.moveCursorBy(body * 2)
	if m.idx == 0 || m.offset == 0 {
		t.Fatalf("expected cursor off first page before search, got idx=%d offset=%d", m.idx, m.offset)
	}
	savedIdx, savedOffset := m.idx, m.offset

	// Enter search mode — snapshot must capture the paged position.
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = next.(Model)
	if m.mode != modeSearch {
		t.Fatalf("expected search mode, got %s", m.mode)
	}
	if !m.preSearchSaved || m.preSearchIdx != savedIdx || m.preSearchOffset != savedOffset {
		t.Fatalf("pre-search snapshot wrong: saved=%v idx=%d offset=%d want idx=%d offset=%d",
			m.preSearchSaved, m.preSearchIdx, m.preSearchOffset, savedIdx, savedOffset)
	}

	// Typing a query must jump to the first page of matches.
	next, _ = m.handleSearchKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("needle")})
	m = next.(Model)
	if m.query != "needle" {
		t.Fatalf("expected query=needle, got %q", m.query)
	}
	if m.idx != 0 || m.offset != 0 {
		t.Fatalf("search should reset to first page, got idx=%d offset=%d", m.idx, m.offset)
	}
	if len(m.visibleList) != 1 || m.visibleList[0].Name != "needle-item" {
		t.Fatalf("expected single needle match, got %#v", m.visibleList)
	}

	// Esc cancels search and restores the pre-search scroll position.
	next, _ = m.handleSearchKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.mode != modeNormal {
		t.Fatalf("esc should leave search mode, got %s", m.mode)
	}
	if m.query != "" {
		t.Fatalf("esc should clear query, got %q", m.query)
	}
	if m.preSearchSaved {
		t.Fatal("snapshot should be cleared after restore")
	}
	if m.idx != savedIdx || m.offset != savedOffset {
		t.Fatalf("esc should restore scroll, got idx=%d offset=%d want idx=%d offset=%d",
			m.idx, m.offset, savedIdx, savedOffset)
	}
}

func TestSearchEnterKeepsQueryEscInNormalRestores(t *testing.T) {
	all := make([]skills.Skill, 0, 30)
	for i := 0; i < 30; i++ {
		all = append(all, makeSkill("agents", fmt.Sprintf("item-%02d", i), "enabled", "d"))
	}
	all = append(all, makeSkill("agents", "target-skill", "enabled", "match me"))

	m := newTestModel(80, 20, all)
	m = m.moveCursorBy(m.listBodyHeight() * 2)
	savedIdx, savedOffset := m.idx, m.offset

	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = next.(Model)
	next, _ = m.handleSearchKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("target")})
	m = next.(Model)
	// Confirm search with enter — keep query, keep match-list position.
	next, _ = m.handleSearchKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.mode != modeNormal || m.query != "target" {
		t.Fatalf("enter should keep query in normal mode, mode=%s query=%q", m.mode, m.query)
	}
	if m.idx != 0 {
		t.Fatalf("match list should still be on first page, idx=%d", m.idx)
	}

	// Later esc in normal mode clears search and restores the original place.
	next, _ = m.handleNormalKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.query != "" {
		t.Fatalf("normal esc should clear query, got %q", m.query)
	}
	if m.idx != savedIdx || m.offset != savedOffset {
		t.Fatalf("normal esc should restore pre-search scroll, got idx=%d offset=%d want %d/%d",
			m.idx, m.offset, savedIdx, savedOffset)
	}
}

func TestUpdateKeyRefusesUnmanagedSkill(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		{
			Name: "handcrafted", Source: "agents",
			DisplayName: "handcrafted", Description: "no lock entry",
			DescriptionChars: 14, Status: "enabled",
			Path:    "/tmp/agents/skills/handcrafted",
			Managed: false,
		},
	})
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmNone {
		t.Fatalf("u should not enter confirm for unmanaged skill, got %v", nm.pendingConfirm)
	}
	if !strings.Contains(nm.message, "manual update only") {
		t.Fatalf("expected manual-update message, got %q", nm.message)
	}
}

func TestUpdateKeyEntersConfirmForManagedSkill(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		{
			Name: "managed", Source: "agents",
			DisplayName: "managed", Description: "via skills add",
			DescriptionChars: 14, Status: "enabled",
			Path:       "/tmp/agents/skills/managed",
			Managed:    true,
			LockSource: "vercel-labs/agent-skills",
		},
	})
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmUpdate {
		t.Fatalf("u should enter confirmUpdate for managed skill, got %v", nm.pendingConfirm)
	}
}

func TestLinkedDuplicatesHiddenByDefault(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		{
			Name: "shared", Source: "agents", DisplayName: "shared",
			Description: "primary", DescriptionChars: 7, Status: "enabled",
			Path:        "/tmp/agents/skills/shared",
			IsDuplicate: false,
		},
		{
			Name: "shared", Source: "claude", DisplayName: "shared",
			Description: "duplicate", DescriptionChars: 9, Status: "enabled",
			Path:        "/tmp/claude/skills/shared",
			IsDuplicate: true,
		},
	})
	if len(m.visibleList) != 1 || m.visibleList[0].Source != "agents" {
		t.Fatalf("expected only the agents-side row to surface, got %#v", m.visibleList)
	}
}

func TestFilterKeysSetStatusAxis(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		makeSkill("agents", "alpha", "enabled", "a"),
		makeSkill("claude", "beta", "disabled", "b"),
	})

	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	nm := next.(Model)
	if nm.statusFilter != filterEnabled {
		t.Fatalf("expected statusFilter=enabled after 'e', got %s", nm.statusFilter)
	}
	if len(nm.visibleList) != 1 || nm.visibleList[0].Name != "alpha" {
		t.Fatalf("expected only alpha visible, got %#v", nm.visibleList)
	}

	next, _ = nm.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	nm = next.(Model)
	if nm.statusFilter != filterDisabled {
		t.Fatalf("expected statusFilter=disabled after 'd', got %s", nm.statusFilter)
	}
	if len(nm.visibleList) != 1 || nm.visibleList[0].Name != "beta" {
		t.Fatalf("expected only beta visible, got %#v", nm.visibleList)
	}

	next, _ = nm.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	nm = next.(Model)
	if nm.statusFilter != filterAll {
		t.Fatalf("expected statusFilter=all after 'a', got %s", nm.statusFilter)
	}
	if len(nm.visibleList) != 2 {
		t.Fatalf("expected 2 visible after a, got %d", len(nm.visibleList))
	}
}

func TestTabOpensPreview(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		makeSkill("agents", "alpha", "enabled", "a"),
	})
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyTab})
	if next.(Model).mode != modePreviewFull {
		t.Fatalf("expected preview mode after Tab, got %s", next.(Model).mode)
	}
	// Tab again exits preview.
	next2, _ := next.(Model).handlePreviewKey(tea.KeyMsg{Type: tea.KeyTab})
	if next2.(Model).mode != modeNormal {
		t.Fatalf("expected normal mode after Tab in preview, got %s", next2.(Model).mode)
	}
}

func TestSortKeyEmitsMessage(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		makeSkill("agents", "alpha", "enabled", "a"),
	})
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	nm := next.(Model)
	if !strings.HasPrefix(nm.message, "sort:") {
		t.Fatalf("expected sort message, got %q", nm.message)
	}
}

func TestStatusSegmentSilentWhenIdle(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		makeSkill("agents", "alpha", "enabled", "a"),
	})
	if got := m.renderStatusSegment(); got != "" {
		t.Fatalf("expected empty status when idle, got %q", got)
	}
}

func TestSearchModeKeepsViewHeight(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		makeSkill("agents", "alpha", "enabled", "a"),
	})
	normal := strings.Count(m.View(), "\n")
	m.mode = modeSearch
	m.query = "alpha"
	if got := strings.Count(m.View(), "\n"); got != normal {
		t.Fatalf("expected View() height to stay %d in search mode, got %d", normal, got)
	}
}

func TestHelpScreenFitsHeight(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
	}{
		{"compact 120x24", 120, 24},
		{"typical 140x32", 140, 32},
		{"narrow tall 70x40", 70, 40},
		{"ultra-wide 250x32", 250, 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(tc.width, tc.height, []skills.Skill{
				makeSkill("agents", "alpha", "enabled", "a"),
			})
			m.mode = modeHelp
			view := m.View()
			lines := strings.Split(view, "\n")
			if len(lines) > tc.height {
				t.Fatalf("help view rendered %d lines but height=%d", len(lines), tc.height)
			}
			if !strings.Contains(view, "press any key to dismiss") {
				t.Fatalf("help footer was truncated for %dx%d:\n%s", tc.width, tc.height, view)
			}
		})
	}
}

func TestListTitleEchoesActiveFilter(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		makeSkill("agents", "alpha", "enabled", "a"),
		makeSkill("claude", "beta", "disabled", "b"),
	})
	if got := m.View(); !strings.Contains(got, "Skills · all") {
		t.Fatalf("expected default title to show 'Skills · all', got %q", got)
	}
	m.statusFilter = filterEnabled
	m.refreshLists()
	if got := m.View(); !strings.Contains(got, "Skills · enabled") {
		t.Fatalf("expected title to show 'Skills · enabled', got %q", got)
	}
}

func TestLinkedDuplicatesShownAfterDot(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		{
			Name: "shared", Source: "agents", DisplayName: "shared",
			Description: "primary", DescriptionChars: 7, Status: "enabled",
			Path: "/tmp/agents/skills/shared",
		},
		{
			Name: "shared", Source: "claude", DisplayName: "shared",
			Description: "duplicate", DescriptionChars: 9, Status: "enabled",
			Path:        "/tmp/claude/skills/shared",
			IsDuplicate: true,
		},
	})
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	nm := next.(Model)
	if !nm.showLinked {
		t.Fatal("expected showLinked=true after pressing .")
	}
	if len(nm.visibleList) != 2 {
		t.Fatalf("expected both rows after toggle, got %d", len(nm.visibleList))
	}
}

// --- update overlay scroll regression tests ---

func updateTestModel(height, lineCount int) Model {
	m := newTestModel(120, height, nil)
	m.mode = modeUpdate
	for i := 0; i < lineCount; i++ {
		m.appendUpdateLine(update.Line{Text: fmt.Sprintf("line-%d", i)})
	}
	return m
}

func TestUpdateScrollGGoesToTopNotPastIt(t *testing.T) {
	// 200 lines in a window where inner = h - 4 = 28.
	m := updateTestModel(32, 200)
	next, _ := m.handleUpdateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	nm := next.(Model)
	expected := nm.maxUpdateScroll()
	if nm.updateScrollOffset != expected {
		t.Fatalf("g should land on maxUpdateScroll=%d, got %d", expected, nm.updateScrollOffset)
	}
	// View should still produce non-empty content (was empty in the bug).
	visible := nm.updateVisibleLines(nm.updateInnerHeight(), 80)
	if !strings.Contains(strings.Join(visible, "\n"), "line-0") {
		t.Fatalf("g should reveal the oldest line, got: %q", visible)
	}
}

func TestUpdateScrollKDoesNotEscapePastTop(t *testing.T) {
	m := updateTestModel(32, 50)
	max := m.maxUpdateScroll()
	// Press k 1000 times; offset must clamp to maxUpdateScroll.
	for i := 0; i < 1000; i++ {
		next, _ := m.handleUpdateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = next.(Model)
	}
	if m.updateScrollOffset != max {
		t.Fatalf("k spam should clamp at %d, got %d", max, m.updateScrollOffset)
	}
}

func TestUpdateScrollFollowsBottomByDefault(t *testing.T) {
	m := updateTestModel(32, 5)
	if m.updateScrollOffset != 0 {
		t.Fatalf("expected offset=0 by default, got %d", m.updateScrollOffset)
	}
	// Append more — still following bottom.
	m.appendUpdateLine(update.Line{Text: "fresh"})
	if m.updateScrollOffset != 0 {
		t.Fatalf("offset must stay 0 while following, got %d", m.updateScrollOffset)
	}
}

func TestUpdateScrollAnchorsOnAppend(t *testing.T) {
	m := updateTestModel(32, 100)
	// Scroll up 5 lines.
	for i := 0; i < 5; i++ {
		next, _ := m.handleUpdateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = next.(Model)
	}
	if m.updateScrollOffset != 5 {
		t.Fatalf("setup expected offset=5, got %d", m.updateScrollOffset)
	}
	// What line is at the top of the visible window?
	inner := m.updateInnerHeight()
	end := len(m.updateLines) - m.updateScrollOffset
	topLine := m.updateLines[end-inner]

	// Append 3 lines — view should remain anchored on the same absolute line.
	for i := 0; i < 3; i++ {
		m.appendUpdateLine(update.Line{Text: fmt.Sprintf("new-%d", i)})
	}
	if m.updateScrollOffset != 8 {
		t.Fatalf("offset should track new lines while scrolled, got %d", m.updateScrollOffset)
	}
	end = len(m.updateLines) - m.updateScrollOffset
	if got := m.updateLines[end-inner]; got != topLine {
		t.Fatalf("top of window drifted: was %q, now %q", topLine, got)
	}
}

func TestUpdateScrollGCapitalSnapsToBottom(t *testing.T) {
	m := updateTestModel(32, 200)
	m.updateScrollOffset = 50
	next, _ := m.handleUpdateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if next.(Model).updateScrollOffset != 0 {
		t.Fatalf("G should snap to 0, got %d", next.(Model).updateScrollOffset)
	}
}

func TestFreshnessKeyRefusesUnmanaged(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		{
			Name: "handcrafted", Source: "agents", DisplayName: "handcrafted",
			Description: "no lock entry", DescriptionChars: 14, Status: "enabled",
			Path:    "/tmp/agents/skills/handcrafted",
			Managed: false,
		},
	})
	m.freshnessChecker = freshness.NewChecker()
	m.freshnessCache = map[string]freshness.Result{}
	m.freshnessInflight = map[string]bool{}
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("F")})
	nm := next.(Model)
	if !strings.Contains(nm.message, "not managed") {
		t.Fatalf("expected unmanaged message, got %q", nm.message)
	}
}

func TestFreshnessKeyHitsCacheBeforeNetwork(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		{
			Name: "managed", Source: "agents", DisplayName: "managed",
			Description: "via skills add", DescriptionChars: 14, Status: "enabled",
			Path:       "/tmp/agents/skills/managed",
			Managed:    true,
			LockSource: "vercel-labs/agent-skills",
			LockEntry: &lockfile.Entry{
				SourceURL:       "https://github.com/vercel-labs/agent-skills.git",
				SkillPath:       "skills/managed/SKILL.md",
				SkillFolderHash: "abc12345",
			},
		},
	})
	m.freshnessChecker = freshness.NewChecker()
	m.freshnessCache = map[string]freshness.Result{
		"agents/managed": {
			LocalSHA:  "abc12345",
			RemoteSHA: "abc12345",
			UpToDate:  true,
			CheckedAt: time.Now(),
		},
	}
	m.freshnessInflight = map[string]bool{}
	next, cmd := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("F")})
	nm := next.(Model)
	if cmd != nil {
		t.Fatal("cache hit should not issue a tea.Cmd (no network call)")
	}
	if !strings.Contains(nm.message, "up to date") {
		t.Fatalf("expected cached up-to-date message, got %q", nm.message)
	}
}

func TestFormatFreshnessStatus(t *testing.T) {
	cases := []struct {
		name        string
		result      freshness.Result
		err         error
		wantSubstr  string
		wantMsgKind string
	}{
		{
			name:        "up-to-date",
			result:      freshness.Result{LocalSHA: "abc12345", RemoteSHA: "abc12345", UpToDate: true},
			wantSubstr:  "up to date",
			wantMsgKind: "info",
		},
		{
			name:        "out-of-date",
			result:      freshness.Result{LocalSHA: "abc12345", RemoteSHA: "def67890", UpToDate: false},
			wantSubstr:  "new version available",
			wantMsgKind: "info",
		},
		{
			name:        "rate-limited",
			err:         freshness.ErrRateLimited,
			wantSubstr:  "rate limited",
			wantMsgKind: "error",
		},
		{
			name:        "unsupported-source",
			err:         freshness.ErrUnsupported,
			wantSubstr:  "only github.com",
			wantMsgKind: "error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, kind := formatFreshnessStatus("agents/foo", tc.result, tc.err)
			if !strings.Contains(msg, tc.wantSubstr) {
				t.Errorf("expected %q in message, got %q", tc.wantSubstr, msg)
			}
			if kind != tc.wantMsgKind {
				t.Errorf("expected kind %s, got %s", tc.wantMsgKind, kind)
			}
		})
	}
}

// --- L key: per-skill link / unlink ---

// withPresence is a tiny helper for L-key tests: build a Skill row with
// the given Presence map without re-typing all the Skill fields.
func withPresence(source, name, status string, presence map[string]string) skills.Skill {
	s := makeSkill(source, name, status, "desc")
	s.Presence = presence
	return s
}

func TestLinkKeyNoCandidatesShowsMessage(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		withPresence("agents", "all-real", "enabled", map[string]string{
			"agents": skills.PresenceReal,
			"claude": skills.PresenceReal,
			"codex":  skills.PresenceReal,
		}),
	})
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmNone {
		t.Fatalf("expected no confirm when 0 candidates, got %v", nm.pendingConfirm)
	}
	if !strings.Contains(nm.message, "no link target") {
		t.Fatalf("expected no-link-target message, got %q", nm.message)
	}
}

func TestLinkKeyOneCandidateEntersSingleConfirm(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		withPresence("agents", "single", "enabled", map[string]string{
			"agents": skills.PresenceReal,
			"claude": skills.PresenceMissing,
			"codex":  skills.PresenceReal,
		}),
	})
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmLinkSingle {
		t.Fatalf("expected confirmLinkSingle, got %v", nm.pendingConfirm)
	}
	if len(nm.pendingLinkOps) != 1 {
		t.Fatalf("expected 1 pending op, got %d", len(nm.pendingLinkOps))
	}
	op := nm.pendingLinkOps[0]
	if op.TargetSource != "claude" {
		t.Errorf("expected target=claude, got %s", op.TargetSource)
	}
	if op.Action != "link" {
		t.Errorf("expected action=link, got %s", op.Action)
	}
}

func TestLinkKeyTwoCandidatesEntersChoiceConfirm(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		withPresence("agents", "two", "enabled", map[string]string{
			"agents": skills.PresenceReal,
			"claude": skills.PresenceMissing,
			"codex":  skills.PresenceMissing,
		}),
	})
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmLinkChoice {
		t.Fatalf("expected confirmLinkChoice, got %v", nm.pendingConfirm)
	}
	if len(nm.pendingLinkOps) != 2 {
		t.Fatalf("expected 2 pending ops, got %d", len(nm.pendingLinkOps))
	}
}

func TestLinkKeyMixesLinkAndUnlinkCandidates(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		withPresence("agents", "mixed", "enabled", map[string]string{
			"agents": skills.PresenceReal,
			"claude": skills.PresenceLink,
			"codex":  skills.PresenceMissing,
		}),
	})
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmLinkChoice {
		t.Fatalf("expected confirmLinkChoice, got %v", nm.pendingConfirm)
	}
	haveLink, haveUnlink := false, false
	for _, op := range nm.pendingLinkOps {
		if op.Action == "link" {
			haveLink = true
		}
		if op.Action == "unlink" {
			haveUnlink = true
		}
	}
	if !haveLink || !haveUnlink {
		t.Errorf("expected both link & unlink actions, got %#v", nm.pendingLinkOps)
	}
}

func TestLinkKeyRefusesProtected(t *testing.T) {
	s := withPresence("agents", ".system", "enabled", map[string]string{
		"agents": skills.PresenceReal,
		"claude": skills.PresenceMissing,
		"codex":  skills.PresenceMissing,
	})
	s.Protected = true
	m := newTestModel(140, 32, []skills.Skill{s})
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmNone {
		t.Fatalf("expected no confirm for protected, got %v", nm.pendingConfirm)
	}
	if !strings.Contains(nm.message, "protected") {
		t.Fatalf("expected protected message, got %q", nm.message)
	}
}

func TestLinkKeyRefusesDisabledRow(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		withPresence("agents", "off-skill", "disabled", map[string]string{
			"agents": skills.PresenceMissing,
			"claude": skills.PresenceMissing,
			"codex":  skills.PresenceMissing,
		}),
	})
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmNone {
		t.Fatalf("expected no confirm for disabled row, got %v", nm.pendingConfirm)
	}
	if !strings.Contains(nm.message, "enabled") {
		t.Fatalf("expected disabled-row message, got %q", nm.message)
	}
}

func TestLinkChoiceEscClears(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		withPresence("agents", "two", "enabled", map[string]string{
			"agents": skills.PresenceReal,
			"claude": skills.PresenceMissing,
			"codex":  skills.PresenceMissing,
		}),
	})
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	nm := next.(Model)
	next2, _ := nm.handleConfirmKey(tea.KeyMsg{Type: tea.KeyEsc})
	nm2 := next2.(Model)
	if nm2.pendingConfirm != confirmNone {
		t.Errorf("esc should clear confirm, got %v", nm2.pendingConfirm)
	}
	if len(nm2.pendingLinkOps) != 0 {
		t.Errorf("esc should clear pendingLinkOps, got %d", len(nm2.pendingLinkOps))
	}
}

func TestLinkChoiceNonDigitClears(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		withPresence("agents", "two", "enabled", map[string]string{
			"agents": skills.PresenceReal,
			"claude": skills.PresenceMissing,
			"codex":  skills.PresenceMissing,
		}),
	})
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	nm := next.(Model)
	next2, _ := nm.handleConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	nm2 := next2.(Model)
	if nm2.pendingConfirm != confirmNone {
		t.Errorf("non-digit should cancel choice, got %v", nm2.pendingConfirm)
	}
	if len(nm2.pendingLinkOps) != 0 {
		t.Errorf("non-digit should clear pendingLinkOps, got %d", len(nm2.pendingLinkOps))
	}
}

func TestLinkSingleConfirmAppliesToFilesystem(t *testing.T) {
	dir := t.TempDir()
	agentsRoot := filepath.Join(dir, "agents", "skills")
	claudeRoot := filepath.Join(dir, "claude", "skills")
	if err := os.MkdirAll(filepath.Join(agentsRoot, "foo"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsRoot, "foo", "SKILL.md"),
		[]byte("---\nname: foo\ndescription: test\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m := newTestModel(140, 32, nil)
	m.sources = []skills.Source{
		{Name: "agents", Root: agentsRoot},
		{Name: "claude", Root: claudeRoot},
	}
	m.allSkills = []skills.Skill{{
		Name: "foo", Source: "agents", Status: "enabled",
		Path: filepath.Join(agentsRoot, "foo"),
		Presence: map[string]string{
			"agents": skills.PresenceReal,
			"claude": skills.PresenceMissing,
		},
	}}
	m.refreshLists()

	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmLinkSingle {
		t.Fatalf("expected confirmLinkSingle, got %v", nm.pendingConfirm)
	}

	next2, _ := nm.handleConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm2 := next2.(Model)
	if nm2.pendingConfirm != confirmNone {
		t.Errorf("expected confirm cleared after y, got %v", nm2.pendingConfirm)
	}
	if !strings.Contains(nm2.message, "linked") {
		t.Errorf("expected linked message, got %q", nm2.message)
	}

	fi, err := os.Lstat(filepath.Join(claudeRoot, "foo"))
	if err != nil {
		t.Fatalf("expected symlink at claude/foo: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected symlink, got mode %v", fi.Mode())
	}
}

func TestLinkChoiceDigitDispatch(t *testing.T) {
	dir := t.TempDir()
	agentsRoot := filepath.Join(dir, "agents", "skills")
	claudeRoot := filepath.Join(dir, "claude", "skills")
	codexRoot := filepath.Join(dir, "codex", "skills")
	if err := os.MkdirAll(filepath.Join(agentsRoot, "foo"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsRoot, "foo", "SKILL.md"),
		[]byte("---\nname: foo\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m := newTestModel(140, 32, nil)
	m.sources = []skills.Source{
		{Name: "agents", Root: agentsRoot},
		{Name: "claude", Root: claudeRoot},
		{Name: "codex", Root: codexRoot},
	}
	m.allSkills = []skills.Skill{{
		Name: "foo", Source: "agents", Status: "enabled",
		Path: filepath.Join(agentsRoot, "foo"),
		Presence: map[string]string{
			"agents": skills.PresenceReal,
			"claude": skills.PresenceMissing,
			"codex":  skills.PresenceMissing,
		},
	}}
	m.refreshLists()

	// L → confirmLinkChoice (claude is candidate 1, codex is 2 — order
	// follows m.sources).
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmLinkChoice {
		t.Fatalf("expected confirmLinkChoice, got %v", nm.pendingConfirm)
	}

	// Press "2" → should ln into codex, leaving claude untouched.
	next2, _ := nm.handleConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	nm2 := next2.(Model)
	if nm2.pendingConfirm != confirmNone {
		t.Errorf("expected confirm cleared, got %v", nm2.pendingConfirm)
	}

	if _, err := os.Lstat(filepath.Join(codexRoot, "foo")); err != nil {
		t.Fatalf("expected symlink at codex/foo: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(claudeRoot, "foo")); !os.IsNotExist(err) {
		t.Errorf("claude/foo should not have been touched, lstat err=%v", err)
	}
}

func TestPresenceBitmapAppearsInRow(t *testing.T) {
	m := newTestModel(140, 32, []skills.Skill{
		withPresence("agents", "alpha", "enabled", map[string]string{
			"agents": skills.PresenceReal,
			"claude": skills.PresenceLink,
			"codex":  skills.PresenceMissing,
		}),
	})
	view := m.View()
	if !strings.Contains(view, "Ac·") {
		t.Fatalf("expected presence bitmap Ac· in rendered view, got: %q", view)
	}
}

// linkChoiceFixture stages a 2-candidate confirmLinkChoice (claude + codex)
// against a real temp filesystem so y/letter/digit dispatch tests can
// assert the resulting symlink lands at the expected root.
func linkChoiceFixture(t *testing.T) (Model, string, string) {
	t.Helper()
	dir := t.TempDir()
	agentsRoot := filepath.Join(dir, "agents", "skills")
	claudeRoot := filepath.Join(dir, "claude", "skills")
	codexRoot := filepath.Join(dir, "codex", "skills")
	if err := os.MkdirAll(filepath.Join(agentsRoot, "foo"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsRoot, "foo", "SKILL.md"),
		[]byte("---\nname: foo\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m := newTestModel(140, 32, nil)
	m.sources = []skills.Source{
		{Name: "agents", Root: agentsRoot},
		{Name: "claude", Root: claudeRoot},
		{Name: "codex", Root: codexRoot},
	}
	m.allSkills = []skills.Skill{{
		Name: "foo", Source: "agents", Status: "enabled",
		Path: filepath.Join(agentsRoot, "foo"),
		Presence: map[string]string{
			"agents": skills.PresenceReal,
			"claude": skills.PresenceMissing,
			"codex":  skills.PresenceMissing,
		},
	}}
	m.refreshLists()

	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmLinkChoice {
		t.Fatalf("setup: expected confirmLinkChoice, got %v", nm.pendingConfirm)
	}
	return nm, claudeRoot, codexRoot
}

func TestLinkChoiceLetterDispatchClaude(t *testing.T) {
	m, claudeRoot, codexRoot := linkChoiceFixture(t)
	next, _ := m.handleConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmNone {
		t.Errorf("expected confirm cleared after 'c', got %v", nm.pendingConfirm)
	}
	if _, err := os.Lstat(filepath.Join(claudeRoot, "foo")); err != nil {
		t.Fatalf("expected symlink at claude/foo: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(codexRoot, "foo")); !os.IsNotExist(err) {
		t.Errorf("'c' should not touch codex, lstat err=%v", err)
	}
}

func TestLinkChoiceLetterDispatchCodex(t *testing.T) {
	m, claudeRoot, codexRoot := linkChoiceFixture(t)
	next, _ := m.handleConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmNone {
		t.Errorf("expected confirm cleared after 'x', got %v", nm.pendingConfirm)
	}
	if _, err := os.Lstat(filepath.Join(codexRoot, "foo")); err != nil {
		t.Fatalf("expected symlink at codex/foo: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(claudeRoot, "foo")); !os.IsNotExist(err) {
		t.Errorf("'x' should not touch claude, lstat err=%v", err)
	}
}

func TestLinkChoiceLetterIsCaseInsensitive(t *testing.T) {
	m, claudeRoot, _ := linkChoiceFixture(t)
	next, _ := m.handleConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("C")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmNone {
		t.Errorf("expected confirm cleared after 'C', got %v", nm.pendingConfirm)
	}
	if _, err := os.Lstat(filepath.Join(claudeRoot, "foo")); err != nil {
		t.Fatalf("'C' should dispatch like 'c', lstat err=%v", err)
	}
}

func TestLinkChoiceUnknownLetterCancels(t *testing.T) {
	m, claudeRoot, codexRoot := linkChoiceFixture(t)
	next, _ := m.handleConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmNone {
		t.Errorf("unknown letter should cancel, got %v", nm.pendingConfirm)
	}
	if len(nm.pendingLinkOps) != 0 {
		t.Errorf("expected pendingLinkOps cleared, got %d", len(nm.pendingLinkOps))
	}
	// Neither root should have been touched.
	if _, err := os.Lstat(filepath.Join(claudeRoot, "foo")); !os.IsNotExist(err) {
		t.Errorf("unknown letter should not link claude, lstat err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(codexRoot, "foo")); !os.IsNotExist(err) {
		t.Errorf("unknown letter should not link codex, lstat err=%v", err)
	}
}

func TestLinkChoiceFooterUsesSemanticLetters(t *testing.T) {
	m, _, _ := linkChoiceFixture(t)
	strip := m.renderLinkChoiceStrip(140)
	// Each candidate's hint should advertise its semantic letter, not "1"/"2".
	if !strings.Contains(strip, "ln claude") || !strings.Contains(strip, "ln codex") {
		t.Fatalf("footer should describe candidates by name: %q", strip)
	}
	// Hint keys are in lipgloss-rendered form; spot-check that the c/x
	// glyphs both appear *and* that the digit fallbacks are NOT advertised.
	if !strings.Contains(strip, "c") || !strings.Contains(strip, "x") {
		t.Fatalf("footer should expose c/x letter shortcuts: %q", strip)
	}
}

// --- t key: instant toggle ---

// liveSkillFixture stages a single live skill on disk so toggle tests can
// observe the file move without mocking ApplyOperation.
func liveSkillFixture(t *testing.T) (Model, string, string) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "agents", "skills")
	off := filepath.Join(dir, "off")
	if err := os.MkdirAll(filepath.Join(root, "demo"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo", "SKILL.md"),
		[]byte("---\nname: demo\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m := newTestModel(140, 32, nil)
	m.sources = []skills.Source{{Name: "agents", Root: root}}
	m.sourceRoots = map[string]string{"agents": root}
	m.offRoot = off
	m.allSkills = []skills.Skill{{
		Name: "demo", Source: "agents", Status: "enabled",
		Path: filepath.Join(root, "demo"),
	}}
	m.refreshLists()
	return m, root, off
}

func TestTKeyTogglesCursorImmediately(t *testing.T) {
	m, root, off := liveSkillFixture(t)
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmNone {
		t.Errorf("t should not enter any confirm flow, got %v", nm.pendingConfirm)
	}
	if !strings.Contains(nm.message, "disabled") {
		t.Errorf("expected 'disabled' message, got %q", nm.message)
	}
	// Frontmatter-based: skill stays in live root.
	if _, err := os.Stat(filepath.Join(root, "demo", "SKILL.md")); err != nil {
		t.Errorf("live source should remain in root: %v", err)
	}
	// Should have frontmatter flag.
	data, err := os.ReadFile(filepath.Join(root, "demo", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "disable-model-invocation: true") {
		t.Error("expected disable-model-invocation: true in SKILL.md")
	}
	// Should have agents/openai.yaml.
	if _, err := os.Stat(filepath.Join(root, "demo", "agents", "openai.yaml")); err != nil {
		t.Errorf("expected agents/openai.yaml: %v", err)
	}
	// Should NOT be in off pool.
	if _, err := os.Stat(filepath.Join(off, "agents", "demo")); !os.IsNotExist(err) {
		t.Error("skill should not be moved to off pool")
	}
}

func TestTKeyAppliesMarkedSetWhenPresent(t *testing.T) {
	m, root, off := liveSkillFixture(t)
	// space marks the demo row, then t commits. With marks present,
	// t must apply the marked set, not double-toggle the cursor row.
	m.stageCurrent()
	if len(m.stagedOps) != 1 {
		t.Fatalf("setup: expected 1 staged op, got %d", len(m.stagedOps))
	}
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmNone {
		t.Errorf("t should not confirm, got %v", nm.pendingConfirm)
	}
	if len(nm.stagedOps) != 0 {
		t.Errorf("staged list should be empty after apply, got %d", len(nm.stagedOps))
	}
	// Frontmatter-based: skill stays in live root.
	if _, err := os.Stat(filepath.Join(root, "demo", "SKILL.md")); err != nil {
		t.Errorf("live source should remain in root: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "demo", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "disable-model-invocation: true") {
		t.Error("expected disable-model-invocation: true in SKILL.md")
	}
	if _, err := os.Stat(filepath.Join(off, "agents", "demo")); !os.IsNotExist(err) {
		t.Error("skill should not be moved to off pool")
	}
}

func TestAKeyIsUnbound(t *testing.T) {
	m, root, off := liveSkillFixture(t)
	m.stageCurrent()
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	nm := next.(Model)
	if nm.pendingConfirm != confirmNone {
		t.Errorf("A should not enter a confirm flow, got %v", nm.pendingConfirm)
	}
	if len(nm.stagedOps) != 1 {
		t.Fatalf("A should leave marked operations untouched, got %d", len(nm.stagedOps))
	}
	if _, err := os.Stat(filepath.Join(root, "demo", "SKILL.md")); err != nil {
		t.Errorf("live source should stay in place: %v", err)
	}
	if _, err := os.Stat(filepath.Join(off, "agents", "demo")); !os.IsNotExist(err) {
		t.Errorf("off pool should stay empty, got err=%v", err)
	}
}

func TestTKeyRefusesProtected(t *testing.T) {
	m, _, _ := liveSkillFixture(t)
	m.allSkills[0].Protected = true
	m.refreshLists()
	next, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	nm := next.(Model)
	if !strings.Contains(nm.message, "protected") {
		t.Errorf("expected protected refusal, got %q", nm.message)
	}
}
