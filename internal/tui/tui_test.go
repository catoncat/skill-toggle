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
		offRoot:   "/tmp/off",
		active:    panelEnabled,
		mode:      modeNormal,
		sortMode:  skills.SortByName,
		allSkills: all,
		width:     width,
		height:    height,
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
	if m.active != panelEnabled {
		t.Errorf("expected enabled panel active, got %s", m.active)
	}
	if m.sortMode != skills.SortByName {
		t.Errorf("expected sort=name, got %s", m.sortMode)
	}
	if len(m.sources) != 3 {
		t.Errorf("expected 3 sources, got %d", len(m.sources))
	}
}

func TestRefreshListsPartitionsByStatus(t *testing.T) {
	m := newTestModel(120, 32, []skills.Skill{
		makeSkill("agents", "alpha", "enabled", "first"),
		makeSkill("claude", "beta", "disabled", "second"),
		makeSkill("agents", "gamma", "enabled", "third"),
	})
	if len(m.enabledList) != 2 {
		t.Errorf("expected 2 enabled, got %d", len(m.enabledList))
	}
	if len(m.disabledList) != 1 {
		t.Errorf("expected 1 disabled, got %d", len(m.disabledList))
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

func TestSwapPanelMovesActive(t *testing.T) {
	m := newTestModel(120, 32, []skills.Skill{
		makeSkill("agents", "alpha", "enabled", "a"),
		makeSkill("claude", "beta", "disabled", "b"),
	})
	m2 := m.swapPanel(true)
	if m2.active != panelDisabled {
		t.Fatalf("expected disabled active after swap, got %s", m2.active)
	}
}

func TestMoveCursorRespectsBounds(t *testing.T) {
	m := newTestModel(120, 32, []skills.Skill{
		makeSkill("agents", "a", "enabled", "x"),
		makeSkill("agents", "b", "enabled", "x"),
		makeSkill("agents", "c", "enabled", "x"),
	})
	m = m.moveCursor(1)
	if m.enabledIdx != 1 {
		t.Errorf("expected idx=1, got %d", m.enabledIdx)
	}
	m = m.moveCursor(99)
	if m.enabledIdx != 2 {
		t.Errorf("expected clamp at 2, got %d", m.enabledIdx)
	}
	m = m.moveCursor(-99)
	if m.enabledIdx != 0 {
		t.Errorf("expected clamp at 0, got %d", m.enabledIdx)
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

func TestSearchModeFiltersBothPanels(t *testing.T) {
	m := newTestModel(120, 32, []skills.Skill{
		makeSkill("agents", "cloudflare", "enabled", "cloudflare global"),
		makeSkill("claude", "session-wrap", "enabled", "session wrap"),
		makeSkill("claude", "ctf-web", "disabled", "ctf web"),
	})
	m.mode = modeSearch
	m.query = "cloud"
	m.refreshLists()
	if len(m.enabledList) != 1 || m.enabledList[0].Name != "cloudflare" {
		t.Fatalf("expected enabled filtered to cloudflare, got %#v", m.enabledList)
	}
	if len(m.disabledList) != 0 {
		t.Fatalf("expected disabled empty, got %#v", m.disabledList)
	}
}

func TestNarrowScreenHidesPreviewPane(t *testing.T) {
	m := newTestModel(80, 24, []skills.Skill{
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
	if len(m.enabledList) != 1 || m.enabledList[0].Source != "agents" {
		t.Fatalf("expected only the agents-side row to surface, got %#v", m.enabledList)
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
	if len(nm.enabledList) != 2 {
		t.Fatalf("expected both rows after toggle, got %d", len(nm.enabledList))
	}
}
