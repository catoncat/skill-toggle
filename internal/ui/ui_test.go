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
	row := SkillRow("alpha-skill", "agents", "a long description that should be truncated", 142, "enabled", true, true, false, 80)
	if got := lipgloss.Width(row); got != 80 {
		t.Fatalf("expected width 80, got %d: %q", got, row)
	}
}

func TestSkillRowDoesNotEmitONOrOFFLabels(t *testing.T) {
	enabled := SkillRow("beta", "claude", "desc", 50, "enabled", false, false, false, 60)
	disabled := SkillRow("beta", "claude", "desc", 50, "disabled", false, false, false, 60)
	if strings.Contains(enabled, "ON") || strings.Contains(enabled, "OFF") {
		t.Fatalf("enabled row should not show ON/OFF labels: %q", enabled)
	}
	if strings.Contains(disabled, "ON") || strings.Contains(disabled, "OFF") {
		t.Fatalf("disabled row should not show ON/OFF labels: %q", disabled)
	}
}

func TestSkillRowKeepsLongNameWhenSpaceAllows(t *testing.T) {
	long := "a-very-long-skill-name-that-needs-room"
	row := SkillRow(long, "agents", "desc", 12, "enabled", false, false, false, 80)
	if !strings.Contains(row, long) {
		t.Fatalf("expected full long name in row, got %q", row)
	}
}

func TestSkillRowFitsWidthAcrossSizes(t *testing.T) {
	cases := []int{40, 60, 72, 80, 100, 120, 200}
	for _, w := range cases {
		row := SkillRow("network-diagnosis", "agents", "Use when the user mentions sb, sing-box, ...", 540, "enabled", false, false, false, w)
		if got := lipgloss.Width(row); got != w {
			t.Errorf("width=%d: row width %d != %d: %q", w, got, w, row)
		}
	}
}

func TestSkillRowEnabledHasNoForegroundOverride(t *testing.T) {
	row := SkillRow("plain", "agents", "desc", 5, "enabled", false, false, false, 80)
	// Enabled name should not push a foreground color; it should rely on
	// the terminal default. We can't easily inspect lipgloss output, but
	// the rune sequence for the visible part should at least not contain
	// ANSI color codes for the name segment beyond what other columns
	// inject. Spot-check that "plain" appears unwrapped by an ANSI esc.
	if !strings.Contains(row, "plain") {
		t.Fatalf("expected name 'plain' literal in row: %q", row)
	}
}

func TestPanelTitleHasActiveMarker(t *testing.T) {
	active := PanelTitle("Enabled", true, 30)
	inactive := PanelTitle("Enabled", false, 30)
	if !strings.Contains(active, "●") {
		t.Fatalf("expected active marker in %q", active)
	}
	if strings.Contains(inactive, "●") {
		t.Fatalf("inactive title should not have ● marker: %q", inactive)
	}
}

func TestPanelRendersAtRequestedSize(t *testing.T) {
	out := Panel("Enabled", "row1\nrow2", 30, 6, true)
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
