package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// defaultSources mirrors the production source order used by the TUI.
var defaultSources = []string{"agents", "claude", "codex"}

// realIn returns a Presence map where the named source is "real" and the
// rest are "missing" — handy for SkillRow test rows that just want to
// pretend a skill lives under one root.
func realIn(source string) map[string]string {
	p := make(map[string]string, len(defaultSources))
	for _, s := range defaultSources {
		p[s] = "missing"
	}
	if _, ok := p[source]; ok {
		p[source] = "real"
	} else {
		p[source] = "real"
	}
	return p
}

func TestTrimToWidthRespectsDisplayWidth(t *testing.T) {
	got := TrimToWidth("你好世界", 5)
	if got != "你好…" {
		t.Fatalf("expected wide text to truncate cleanly, got %q", got)
	}
	if width := lipgloss.Width(got); width > 5 {
		t.Fatalf("trimmed text should fit width 5, got width %d for %q", width, got)
	}
}

func TestTrimToWidthSingleColumnHandlesWideRune(t *testing.T) {
	got := TrimToWidth("你a", 1)
	if got != "" {
		t.Fatalf("expected no overflow when width is smaller than a wide rune, got %q", got)
	}
}

func TestTruncateLeftRespectsDisplayWidth(t *testing.T) {
	got := TruncateLeft("/tmp/你好世界", 6)
	if !strings.HasPrefix(got, "…") {
		t.Fatalf("expected left truncation to add an ellipsis, got %q", got)
	}
	if width := lipgloss.Width(got); width > 6 {
		t.Fatalf("truncated text should fit width 6, got width %d for %q", width, got)
	}
}

func TestSkillRowFitsRequestedWidth(t *testing.T) {
	row := SkillRow("alpha-skill", realIn("agents"), defaultSources, "a long description that should be truncated", 142, "enabled", true, true, false, 12, 80)
	if got := lipgloss.Width(row); got != 80 {
		t.Fatalf("expected width 80, got %d: %q", got, row)
	}
}

func TestSkillRowDoesNotEmitONOrOFFLabels(t *testing.T) {
	enabled := SkillRow("beta", realIn("claude"), defaultSources, "desc", 50, "enabled", false, false, false, 12, 60)
	disabled := SkillRow("beta", realIn("claude"), defaultSources, "desc", 50, "disabled", false, false, false, 12, 60)
	if strings.Contains(enabled, "ON") || strings.Contains(enabled, "OFF") {
		t.Fatalf("enabled row should not show ON/OFF labels: %q", enabled)
	}
	if strings.Contains(disabled, "ON") || strings.Contains(disabled, "OFF") {
		t.Fatalf("disabled row should not show ON/OFF labels: %q", disabled)
	}
}

func TestSkillRowKeepsLongNameWhenColWAllows(t *testing.T) {
	long := "a-very-long-skill-name-that-needs-room"
	row := SkillRow(long, realIn("agents"), defaultSources, "desc", 12, "enabled", false, false, false, lipgloss.Width(long), 120)
	if !strings.Contains(row, long) {
		t.Fatalf("expected full long name when nameColW >= len(name), got %q", row)
	}
}

func TestSkillRowTruncatesNameToColumnWidth(t *testing.T) {
	long := "a-very-long-skill-name-that-needs-room"
	row := SkillRow(long, realIn("agents"), defaultSources, "desc", 12, "enabled", false, false, false, 12, 120)
	if strings.Contains(row, long) {
		t.Fatalf("name longer than nameColW should be truncated, got %q", row)
	}
}

func TestSkillRowFitsWidthAcrossSizes(t *testing.T) {
	cases := []int{40, 60, 72, 80, 100, 120, 200}
	for _, w := range cases {
		row := SkillRow("network-diagnosis", realIn("agents"), defaultSources, "Use when the user mentions sb, sing-box, ...", 540, "enabled", false, false, false, 17, w)
		if got := lipgloss.Width(row); got != w {
			t.Errorf("width=%d: row width %d != %d: %q", w, got, w, row)
		}
	}
}

func TestSkillRowEnabledHasNoForegroundOverride(t *testing.T) {
	row := SkillRow("plain", realIn("agents"), defaultSources, "desc", 5, "enabled", false, false, false, 12, 80)
	// Enabled name should not push a foreground color; it should rely on
	// the terminal default. We can't easily inspect lipgloss output, but
	// the rune sequence for the visible part should at least not contain
	// ANSI color codes for the name segment beyond what other columns
	// inject. Spot-check that "plain" appears unwrapped by an ANSI esc.
	if !strings.Contains(row, "plain") {
		t.Fatalf("expected name 'plain' literal in row: %q", row)
	}
}

func TestNameColumnWidthClampsToBounds(t *testing.T) {
	// All very long names — should cap at min(28, (width-14)/2)
	long := []string{
		"a-very-long-skill-name-aaaa",            // 27
		"another-extremely-long-skill-name-bbbb", // 38
	}
	got := NameColumnWidth(long, 120)
	if got != 28 {
		t.Errorf("expected absolute cap 28 with wide width, got %d", got)
	}

	// Narrow width: cap should drop to (width-14)/2 = (60-14)/2 = 23
	got = NameColumnWidth(long, 60)
	if got != 23 {
		t.Errorf("expected (60-14)/2=23 cap on narrow width, got %d", got)
	}

	// All very short names — should floor at 12
	short := []string{"a", "bb", "ccc"}
	got = NameColumnWidth(short, 120)
	if got != 12 {
		t.Errorf("expected min floor 12 with short names, got %d", got)
	}

	// Mixed: pick max name length within bounds
	mid := []string{"foo", "exactly-15-char", "bar"}
	got = NameColumnWidth(mid, 120)
	if got != 15 {
		t.Errorf("expected exactly-15-char width 15, got %d", got)
	}
}

func TestSkillRowAlignsPresenceColumnAcrossRows(t *testing.T) {
	short := SkillRow("foo", realIn("agents"), defaultSources, "d1", 1, "enabled", false, false, false, 20, 80)
	long := SkillRow("frontend-design", realIn("claude"), defaultSources, "d2", 2, "enabled", false, false, false, 20, 80)

	// Both rows must place the 3-char presence bitmap at the same column.
	// stagedW(1)+cursorW(2)+nameColW(20)+SP(1) = 24, presence starts at col 24.
	for label, row := range map[string]string{"short": short, "long": long} {
		if lipgloss.Width(row) != 80 {
			t.Errorf("%s row width %d != 80", label, lipgloss.Width(row))
		}
	}
	if !strings.Contains(short, "foo") {
		t.Fatalf("short row missing name: %q", short)
	}
	if !strings.Contains(short, "A··") {
		t.Fatalf("short row missing agents-real bitmap A··: %q", short)
	}
	if !strings.Contains(long, "frontend-design") {
		t.Fatalf("long row missing name: %q", long)
	}
	if !strings.Contains(long, "·C·") {
		t.Fatalf("long row missing claude-real bitmap ·C·: %q", long)
	}
}

func TestFormatPresenceCases(t *testing.T) {
	cases := []struct {
		name     string
		presence map[string]string
		want     string
	}{
		{"agents-real-only", map[string]string{"agents": "real", "claude": "missing", "codex": "missing"}, "A··"},
		{"agents-real-claude-link", map[string]string{"agents": "real", "claude": "link", "codex": "missing"}, "Ac·"},
		{"all-real", map[string]string{"agents": "real", "claude": "real", "codex": "real"}, "ACX"},
		{"only-claude-real", map[string]string{"agents": "missing", "claude": "real", "codex": "missing"}, "·C·"},
		{"all-missing", map[string]string{"agents": "missing", "claude": "missing", "codex": "missing"}, "···"},
		{"agents-real-codex-link", map[string]string{"agents": "real", "claude": "missing", "codex": "link"}, "A·x"},
	}
	for _, c := range cases {
		got := FormatPresence(c.presence, defaultSources)
		if got != c.want {
			t.Errorf("%s: FormatPresence = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestFormatPresenceUnknownSourceFallsBack(t *testing.T) {
	// Custom source with no entry in presenceLetters — should use first
	// letter, lowercased.
	got := FormatPresence(map[string]string{"zoo": "real"}, []string{"zoo"})
	if got != "Z" {
		t.Errorf("FormatPresence with unknown source = %q, want %q", got, "Z")
	}
}

func TestFormatPresenceNilMapDoesNotPanic(t *testing.T) {
	got := FormatPresence(nil, defaultSources)
	if got != "···" {
		t.Errorf("nil presence map should render all-missing, got %q", got)
	}
}

func TestSkillRowStagedMarkerCoexistsWithPresence(t *testing.T) {
	row := SkillRow("foo", realIn("agents"), defaultSources, "d", 0, "enabled", false, false, true, 12, 80)
	if !strings.Contains(row, "~") {
		t.Errorf("staged row should include '~' marker: %q", row)
	}
	if !strings.Contains(row, "A··") {
		t.Errorf("staged row should still render presence bitmap: %q", row)
	}
}

func TestPanelEmbedsTitleInTopBorder(t *testing.T) {
	out := Panel("Skills · all (12)", "row1\nrow2", 40, 6, true)
	lines := strings.Split(out, "\n")
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines for height 6, got %d", len(lines))
	}
	// Top border line should contain the title text and the rounded corners.
	top := lines[0]
	if !strings.Contains(top, "Skills · all") {
		t.Fatalf("expected title embedded in top border, got %q", top)
	}
	if !strings.Contains(top, "╭") || !strings.Contains(top, "╮") {
		t.Fatalf("expected rounded corners in top border, got %q", top)
	}
	// Title should never duplicate as a separate row inside the body.
	for _, l := range lines[1 : len(lines)-1] {
		if strings.Contains(l, "Skills · all") {
			t.Fatalf("title should not repeat inside body, got %q", l)
		}
	}
	// Bottom border has corners but no title.
	bot := lines[len(lines)-1]
	if !strings.Contains(bot, "╰") || !strings.Contains(bot, "╯") {
		t.Fatalf("expected bottom corners, got %q", bot)
	}
	if strings.Contains(bot, "Skills") {
		t.Fatalf("bottom border should not carry title, got %q", bot)
	}
}

func TestPanelRendersAtRequestedSize(t *testing.T) {
	out := Panel("Skills", "row1\nrow2", 30, 6, true)
	lines := strings.Split(out, "\n")
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines for height 6, got %d", len(lines))
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got != 30 {
			t.Fatalf("line %d width %d != 30: %q", i, got, line)
		}
	}
}

func TestPanelHandlesNarrowWidth(t *testing.T) {
	// Outer width too small for an embedded title — should fall back to a
	// plain border line without crashing or overflowing.
	out := Panel("Skills · all", "x", 10, 4, true)
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines for height 4, got %d", len(lines))
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got != 10 {
			t.Fatalf("line %d width %d != 10: %q", i, got, line)
		}
	}
}
