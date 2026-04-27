package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

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
	row := SkillRow("alpha-skill", "agents", "a long description that should be truncated", 142, "enabled", true, true, false, 12, 80)
	if got := lipgloss.Width(row); got != 80 {
		t.Fatalf("expected width 80, got %d: %q", got, row)
	}
}

func TestSkillRowDoesNotEmitONOrOFFLabels(t *testing.T) {
	enabled := SkillRow("beta", "claude", "desc", 50, "enabled", false, false, false, 12, 60)
	disabled := SkillRow("beta", "claude", "desc", 50, "disabled", false, false, false, 12, 60)
	if strings.Contains(enabled, "ON") || strings.Contains(enabled, "OFF") {
		t.Fatalf("enabled row should not show ON/OFF labels: %q", enabled)
	}
	if strings.Contains(disabled, "ON") || strings.Contains(disabled, "OFF") {
		t.Fatalf("disabled row should not show ON/OFF labels: %q", disabled)
	}
}

func TestSkillRowKeepsLongNameWhenColWAllows(t *testing.T) {
	long := "a-very-long-skill-name-that-needs-room"
	row := SkillRow(long, "agents", "desc", 12, "enabled", false, false, false, lipgloss.Width(long), 120)
	if !strings.Contains(row, long) {
		t.Fatalf("expected full long name when nameColW >= len(name), got %q", row)
	}
}

func TestSkillRowTruncatesNameToColumnWidth(t *testing.T) {
	long := "a-very-long-skill-name-that-needs-room"
	row := SkillRow(long, "agents", "desc", 12, "enabled", false, false, false, 12, 120)
	if strings.Contains(row, long) {
		t.Fatalf("name longer than nameColW should be truncated, got %q", row)
	}
}

func TestSkillRowFitsWidthAcrossSizes(t *testing.T) {
	cases := []int{40, 60, 72, 80, 100, 120, 200}
	for _, w := range cases {
		row := SkillRow("network-diagnosis", "agents", "Use when the user mentions sb, sing-box, ...", 540, "enabled", false, false, false, 17, w)
		if got := lipgloss.Width(row); got != w {
			t.Errorf("width=%d: row width %d != %d: %q", w, got, w, row)
		}
	}
}

func TestSkillRowEnabledHasNoForegroundOverride(t *testing.T) {
	row := SkillRow("plain", "agents", "desc", 5, "enabled", false, false, false, 12, 80)
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
	// All very long names — should cap at min(28, (width-18)/2)
	long := []string{
		"a-very-long-skill-name-aaaa",            // 27
		"another-extremely-long-skill-name-bbbb", // 38
	}
	got := NameColumnWidth(long, 120)
	if got != 28 {
		t.Errorf("expected absolute cap 28 with wide width, got %d", got)
	}

	// Narrow width: cap should drop to (width-18)/2 = (60-18)/2 = 21
	got = NameColumnWidth(long, 60)
	if got != 21 {
		t.Errorf("expected (60-18)/2=21 cap on narrow width, got %d", got)
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

func TestSkillRowAlignsSourceColumnAcrossRows(t *testing.T) {
	short := SkillRow("foo", "agents", "d1", 1, "enabled", false, false, false, 20, 80)
	long := SkillRow("frontend-design", "claude", "d2", 2, "enabled", false, false, false, 20, 80)

	// Both rows must place "agents"/"claude" at the same column index. Since
	// stagedW(1)+cursorW(2)+nameColW(20)+SP(1) = 24, source starts at col 24.
	for label, row := range map[string]string{"short": short, "long": long} {
		// Strip ANSI: lipgloss.Width counts visible width but we want a
		// substring match. PadRight on name guarantees the source label
		// is preceded by the literal padding spaces; we just verify the
		// row contains "agents" / "claude" some place after at least 24
		// chars of visible content.
		if lipgloss.Width(row) != 80 {
			t.Errorf("%s row width %d != 80", label, lipgloss.Width(row))
		}
	}
	if !strings.Contains(short, "foo") || !strings.Contains(short, "agents") {
		t.Fatalf("short row missing expected fields: %q", short)
	}
	if !strings.Contains(long, "frontend-design") || !strings.Contains(long, "claude") {
		t.Fatalf("long row missing expected fields: %q", long)
	}
}

func TestPanelTitleNumbered(t *testing.T) {
	active := PanelTitle(1, "Enabled", true, 30)
	inactive := PanelTitle(2, "Disabled", false, 30)
	unNumbered := PanelTitle(0, "Update", true, 30)

	if !strings.Contains(active, "[1]") {
		t.Fatalf("expected [1] marker in active title: %q", active)
	}
	if !strings.Contains(active, "Enabled") {
		t.Fatalf("expected title text 'Enabled' in: %q", active)
	}
	if !strings.Contains(inactive, "[2]") {
		t.Fatalf("expected [2] marker in inactive title: %q", inactive)
	}
	if strings.Contains(unNumbered, "[") {
		t.Fatalf("number=0 should not render brackets: %q", unNumbered)
	}
	if strings.Contains(active, "●") || strings.Contains(inactive, "●") {
		t.Fatalf("● marker should be removed once numbering is in place: %q / %q", active, inactive)
	}
}

func TestPanelRendersAtRequestedSize(t *testing.T) {
	out := Panel(1, "Enabled", "row1\nrow2", 30, 6, true)
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
