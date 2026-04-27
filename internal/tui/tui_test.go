package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/catoncat/skill-toggle/internal/skills"
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
	expectedTarget := filepath.Join("/tmp/off", "agents", "demo")
	if m.stagedOps[0].TargetPath != expectedTarget {
		t.Errorf("unexpected target %s", m.stagedOps[0].TargetPath)
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
			Path:        "/tmp/agents/skills/shared",
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
