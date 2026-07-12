package ui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Color palette — uses ANSI palette indices (0-15) instead of hard-coded hex
// values so the terminal theme (Solarized, Gruvbox, Tokyo Night, …) drives the
// actual hue. The TUI inherits whatever the user already configured for their
// shell, which is what lazygit does too. The trade-off is that we can't tune
// individual shades, but consistency with the user's environment matters more.
//
// Indices used:
//
//	1  red        2 green        3 yellow       4 blue
//	5  magenta    6 cyan         7 white        8 bright black (gray)
var (
	// Border line for every panel — quiet gray. Inactive == active by design;
	// activation is conveyed through the selection cursor + bold title, not
	// through a high-contrast frame color.
	Border = lipgloss.Color("8")

	// Title text inside the top border. Active panels render bold + cyan to
	// pop without drowning the frame; inactive panels drop to Muted.
	TitleColor = lipgloss.Color("6")

	// Accent — used by key hints, search prompt, markdown headings.
	Accent = lipgloss.Color("4")

	// Muted secondary text (key-hint labels, inactive titles, chrome).
	// Not used for enabled/disabled skill status — that distinction is
	// conveyed with Faint so it stays relative to the terminal default.
	Muted = lipgloss.Color("8")

	// Subtle — reserved for chrome that should match Muted under ANSI-16.
	Subtle = lipgloss.Color("8")

	// Semantic statuses.
	Success = lipgloss.Color("2")
	Warning = lipgloss.Color("3")
	Danger  = lipgloss.Color("1")
)

// Panel renders a bordered region with the title embedded in the top border
// (lazygit-style: `╭─ Title ────────╮`). width and height count the OUTER
// size (border-inclusive). Pass active=true to make the title bold + accent;
// the border itself stays the same color either way so the user's eye is
// drawn to row selection rather than the frame.
func Panel(title string, body string, width, height int, active bool) string {
	if width < 2 {
		width = 2
	}
	if height < 2 {
		height = 2
	}

	innerWidth := width - 2
	if innerWidth < 1 {
		innerWidth = 1
	}
	innerHeight := height - 2
	if innerHeight < 1 {
		innerHeight = 1
	}

	// Account for the 1-cell horizontal padding we apply to body content.
	contentWidth := innerWidth - 2
	if contentWidth < 1 {
		contentWidth = 1
	}

	borderStyle := lipgloss.NewStyle().Foreground(Border)
	side := borderStyle.Render("│")

	top := renderTopBorder(title, width, active)
	bottom := borderStyle.Render("╰" + strings.Repeat("─", innerWidth) + "╯")

	bodyLines := strings.Split(body, "\n")
	if len(bodyLines) > innerHeight {
		bodyLines = bodyLines[:innerHeight]
	}
	for len(bodyLines) < innerHeight {
		bodyLines = append(bodyLines, "")
	}

	rows := make([]string, 0, innerHeight+2)
	rows = append(rows, top)
	for _, line := range bodyLines {
		// Pad the body line to the inner content width so the right border
		// lands at the same column on every row.
		line = PadRight(TrimToWidth(line, contentWidth), contentWidth)
		rows = append(rows, side+" "+line+" "+side)
	}
	rows = append(rows, bottom)
	return strings.Join(rows, "\n")
}

// renderTopBorder builds the top border line with the title embedded:
//
//	╭─ Title ─────────────╮
//
// When the title is too long, it is truncated with an ellipsis. When the
// outer width is too narrow to fit the title at all, we fall back to a plain
// horizontal line.
func renderTopBorder(title string, outerWidth int, active bool) string {
	innerWidth := outerWidth - 2
	if innerWidth < 1 {
		innerWidth = 1
	}
	borderStyle := lipgloss.NewStyle().Foreground(Border)

	titleStyle := lipgloss.NewStyle().Foreground(TitleColor).Bold(true)
	if !active {
		titleStyle = lipgloss.NewStyle().Foreground(Muted)
	}

	// Layout: ╭─ SP <title> SP ─{n}─ ╮
	// Reserved cells inside innerWidth: 1 (─) + 1 (SP) + 1 (SP) + 1 (─ minimum on right)
	const reserved = 4
	titleBudget := innerWidth - reserved
	if titleBudget < 1 || title == "" {
		// No room for an embedded title — fall back to a plain top line.
		return borderStyle.Render("╭" + strings.Repeat("─", innerWidth) + "╮")
	}
	t := TrimToWidth(title, titleBudget)
	tw := lipgloss.Width(t)

	// Right-side dash fill = innerWidth - 1 (left dash) - 2 (spaces) - tw
	rightDashes := innerWidth - 3 - tw
	if rightDashes < 1 {
		rightDashes = 1
	}

	leftSeg := borderStyle.Render("╭─ ")
	titleSeg := titleStyle.Render(t)
	rightSeg := borderStyle.Render(" " + strings.Repeat("─", rightDashes) + "╮")
	return leftSeg + titleSeg + rightSeg
}

// NameColumnWidth picks a panel-wide name column width using the standard
// two-pass approach: scan all rows for max(name width), clamp into
// [nameMin, cap]. The cap is min(28, half the budget left after the
// fixed columns) so a runaway long skill name can't starve description.
//
// Pass the result to every SkillRow rendered for that panel so presence /
// chars / description columns line up across rows.
func NameColumnWidth(names []string, width int) int {
	const (
		// Mirror SkillRow's fixedWithDesc — see the constant block there.
		fixedWithDesc = 14
		nameMin       = 12
		nameMaxAbs    = 28
	)
	budget := (width - fixedWithDesc) / 2
	if budget < 1 {
		budget = 1
	}
	cap := nameMaxAbs
	if cap > budget {
		cap = budget
	}
	if cap < nameMin {
		cap = nameMin
	}

	maxName := 0
	for _, n := range names {
		if w := lipgloss.Width(n); w > maxName {
			maxName = w
		}
	}
	if maxName < nameMin {
		maxName = nameMin
	}
	if maxName > cap {
		maxName = cap
	}
	return maxName
}

// presenceLetters maps the built-in source names to their bitmap letters.
// agents/claude/codex are the only sources skill-toggle ships with; if a
// caller passes an unknown name we fall back to its first letter (lower
// case → masquerades as a link, never as real). The map is ASCII-only so
// the column renders identically across terminals and themes.
//
// codex starts with 'c' just like claude, so it gets 'x' to keep the
// bitmap unambiguous. The same letters double as semantic shortcut keys
// in the L (link) confirm menu, so any change here propagates there.
var presenceLetters = map[string]rune{
	"agents": 'a',
	"claude": 'c',
	"codex":  'x',
}

// LetterForSource returns the lowercase glyph used to represent the given
// source — both in the presence bitmap (where uppercase = real and
// lowercase = link) and as the semantic shortcut key in the L confirm
// menu. Unknown sources fall back to their first lowercase letter, or
// '?' for an empty string.
func LetterForSource(source string) rune {
	if r, ok := presenceLetters[source]; ok {
		return r
	}
	runes := []rune(source)
	if len(runes) == 0 {
		return '?'
	}
	return unicode.ToLower(runes[0])
}

// FormatPresence renders the 3-character a/c/x bitmap for a skill's
// per-source visibility. The order matches sources; each glyph is the
// uppercase letter for "real", lowercase for "link", or '·' for missing.
// Unknown sources fall back to the first letter of the source name.
//
// Width is exactly len(sources) ASCII columns so callers can budget
// against it just like a fixed-width source string.
func FormatPresence(presence map[string]string, sources []string) string {
	if len(sources) == 0 {
		return ""
	}
	out := make([]rune, 0, len(sources))
	for _, src := range sources {
		letter, ok := presenceLetters[src]
		if !ok {
			r := []rune(src)
			if len(r) == 0 {
				letter = '?'
			} else {
				letter = r[0]
				if letter >= 'A' && letter <= 'Z' {
					letter = letter + ('a' - 'A')
				}
			}
		}
		switch presence[src] {
		case "real":
			out = append(out, unicode.ToUpper(letter))
		case "link":
			out = append(out, unicode.ToLower(letter))
		default:
			out = append(out, '·')
		}
	}
	return string(out)
}

// SkillRow formats a single row in the skill list.
//
// width       = total cell width
// presence    = per-source visibility map (key: source name, value: "real"
//
//	/ "link" / "missing"); rendered into a fixed-width
//	a/c/x bitmap via FormatPresence
//
// sources     = source name order matching the bitmap columns (e.g.
//
//	["agents", "claude", "codex"])
//
// nameColW    = pre-computed column width for the name field (caller is
//
//	expected to compute this once per render with the panel's
//	max name width — see NameColumnWidth — so all rows in the
//	panel share the same presence/chars/desc column starts)
//
// selected    = cursor sits on this row in the panel
// activePanel = whether the panel containing this row currently has focus
// staged      = the row is staged for toggling
//
// Layout (visible columns):
//
//	<staged><cursor><name>SP<presence>SP<chars>[SP<description>]
//	  1       2     nameColW 1   N     1   5     1   M
//
// Presence column width equals len(sources) (3 for the built-in trio).
//
// Status (ON/OFF) is conveyed by name weight / dimming, not by a hardcoded
// palette color:
//
//   - enabled  → terminal default foreground (no Lip Gloss color set)
//   - disabled → Faint (SGR 2). Faint is a relative attribute, so it stays
//     dimmer than the default on both light and dark themes. Hard-coding
//     ANSI index 8 ("bright black") used to look inverted on some dark
//     themes where color 8 is brighter than the default fg.
//
// Selection uses Reverse on a color-free row. Nested Foreground styles emit
// \x1b[0m resets that cancel reverse mid-row and make enabled/disabled look
// swapped under the cursor; building the selected row plain avoids that.
func SkillRow(name string, presence map[string]string, sources []string, description string, descChars int, status string, selected, activePanel, staged bool, nameColW, width int) string {
	if width < 10 {
		return TrimToWidth(name, width)
	}

	cursor := "  "
	if selected && activePanel {
		cursor = "▌ "
	} else if selected {
		cursor = "› "
	}

	presenceW := len(sources)
	if presenceW < 1 {
		presenceW = 1
	}
	const (
		stagedW   = 1
		cursorW   = 2
		charsW    = 5
		nameFloor = 4
		descMin   = 6
	)
	fixedNoDesc := stagedW + cursorW + 1 + presenceW + 1 + charsW
	fixedWithDesc := fixedNoDesc + 1

	// Cap nameColW so the row can never exceed the cell width: nameW must
	// fit within (width - fixedNoDesc) at minimum.
	nameW := nameColW
	if maxNameW := width - fixedNoDesc; nameW > maxNameW {
		nameW = maxNameW
	}
	if nameW < nameFloor {
		nameW = nameFloor
	}

	descW := width - fixedWithDesc - nameW
	if descW < descMin {
		descW = 0
	}

	presenceText := FormatPresence(presence, sources)
	nameText := PadRight(TrimToWidth(name, nameW), nameW)
	presencePlain := PadRight(TrimToWidth(presenceText, presenceW), presenceW)
	charsPlain := PadRight(FormatDescChars(descChars), charsW)
	var descPlain string
	if descW > 0 {
		descPlain = TrimToWidth(description, descW)
	}

	// Selected + focused: plain text + reverse. No per-cell Foreground —
	// nested SGR (including a colored staged marker) would emit \x1b[0m
	// mid-row and cancel reverse, which is what made enabled/disabled
	// look inverted under the cursor in dark themes.
	if selected && activePanel {
		stagedMarker := " "
		if staged {
			stagedMarker = "~"
		}
		parts := []string{stagedMarker, cursor, nameText, " ", presencePlain, " ", charsPlain}
		if descW > 0 {
			parts = append(parts, " ", descPlain)
		}
		row := strings.Join(parts, "")
		rowWidth := lipgloss.Width(row)
		if rowWidth < width {
			row += strings.Repeat(" ", width-rowWidth)
		}
		return lipgloss.NewStyle().Reverse(true).Render(row)
	}

	stagedMarker := " "
	if staged {
		stagedMarker = lipgloss.NewStyle().Foreground(Warning).Bold(true).Render("~")
	}

	disabled := status != "enabled"
	nameStyle := lipgloss.NewStyle()
	descStyle := lipgloss.NewStyle().Faint(true)
	if disabled {
		// Whole disabled name is faint so it is clearly secondary to
		// enabled peers, independent of the terminal's ANSI color-8 mapping.
		nameStyle = lipgloss.NewStyle().Faint(true)
	}
	presenceStyle := lipgloss.NewStyle().Faint(true)
	charsStyle := lipgloss.NewStyle().Faint(true)

	parts := []string{
		stagedMarker,
		cursor,
		nameStyle.Render(nameText),
		" ",
		presenceStyle.Render(presencePlain),
		" ",
		charsStyle.Render(charsPlain),
	}
	if descW > 0 {
		parts = append(parts, " ", descStyle.Render(descPlain))
	}
	row := strings.Join(parts, "")
	rowWidth := lipgloss.Width(row)
	if rowWidth < width {
		row += strings.Repeat(" ", width-rowWidth)
	}
	return row
}

// PreviewMetadataLine formats one metadata row in the right pane. The label
// is dimmed; the value uses the terminal's default foreground so it adapts
// to dark/light themes without relying on Lip Gloss adaptive color.
func PreviewMetadataLine(label, value string, width int) string {
	labelStyled := lipgloss.NewStyle().Foreground(Muted).Render(label)
	valueWidth := width - lipgloss.Width(labelStyled) - 1
	if valueWidth < 1 {
		valueWidth = 1
	}
	return labelStyled + " " + TrimToWidth(value, valueWidth)
}

// PreviewBodyLine renders one line of SKILL.md body, applying a tiny accent
// to markdown headings so the wall of text isn't completely flat. Body
// paragraphs use the terminal default foreground (no Lip Gloss color set)
// so dark/light themes both render with sensible contrast.
func PreviewBodyLine(line string, width int) string {
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, "#") {
		return lipgloss.NewStyle().Foreground(Accent).Bold(true).Render(TrimToWidth(line, width))
	}
	if strings.HasPrefix(trimmed, "> ") {
		return lipgloss.NewStyle().Foreground(Muted).Italic(true).Render(TrimToWidth(line, width))
	}
	return TrimToWidth(line, width)
}

// KeyHint renders a single "[key] label" pair for the bottom strip.
func KeyHint(key, label string) string {
	keyStyle := lipgloss.NewStyle().Foreground(Accent).Bold(true)
	textStyle := lipgloss.NewStyle().Foreground(Muted)
	return keyStyle.Render(key) + " " + textStyle.Render(label)
}

// SearchPrompt formats the inline search input shown above the active panel.
func SearchPrompt(query string, width int) string {
	prefix := lipgloss.NewStyle().Foreground(Accent).Bold(true).Render("/ ")
	cursor := lipgloss.NewStyle().Foreground(Accent).Render("▍")
	hint := lipgloss.NewStyle().Foreground(Subtle).Italic(true).Render(" matches name · source · description")
	rendered := prefix + query + cursor
	used := lipgloss.Width(rendered) + lipgloss.Width(hint)
	if used <= width {
		return rendered + strings.Repeat(" ", width-used) + hint
	}
	return TrimToWidth(rendered, width)
}

// StatusMessage renders an info or error blob suitable for the bottom strip.
func StatusMessage(text string, isError bool) string {
	if isError {
		return lipgloss.NewStyle().Foreground(Danger).Bold(true).Render(text)
	}
	return lipgloss.NewStyle().Foreground(Success).Render(text)
}

// MutedText renders dim secondary text.
func MutedText(text string) string {
	return lipgloss.NewStyle().Foreground(Muted).Render(text)
}

// HelpOverlayBox builds a centered help block — bordered, body left-aligned.
// `width` is the outer width (border-inclusive). lipgloss.Style.Width sets
// padding+content, so we subtract just the border to land on the requested
// outer width. Subtracting the full frame leaves the box 4 columns short of
// what callers asked for, which on narrow terminals triggers ugly mid-word
// wrap inside the help box ("bottom of\npanel").
func HelpOverlayBox(body string, width int) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Border).
		Padding(1, 2)
	frameW := width - style.GetHorizontalBorderSize()
	min := style.GetHorizontalPadding() + 20
	if frameW < min {
		frameW = min
	}
	return style.Width(frameW).Render(body)
}

// ConfirmPrompt is shown inline at the bottom for y/N decisions.
func ConfirmPrompt(question string) string {
	q := lipgloss.NewStyle().Foreground(Warning).Bold(true).Render(question)
	hint := lipgloss.NewStyle().Foreground(Muted).Render("  (y/N)")
	return q + hint
}

// FormatDescChars compacts character counts (1.4k for >=1000).
func FormatDescChars(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

// PadRight right-pads s to display width w.
func PadRight(s string, w int) string {
	width := lipgloss.Width(s)
	if width >= w {
		return s
	}
	return s + strings.Repeat(" ", w-width)
}

// TrimToWidth trims s to fit visual width w, appending an ellipsis if cut.
func TrimToWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	if width <= 1 {
		return trimRunesToWidth(text, width)
	}
	return trimRunesToWidth(text, width-1) + "…"
}

// TruncateLeft trims with an ellipsis prefix, preserving the rightmost
// visible characters (handy for paths).
func TruncateLeft(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	runes := []rune(s)
	target := width - lipgloss.Width("…")
	if target <= 0 {
		return "…"
	}
	var tail []rune
	currentWidth := 0
	for i := len(runes) - 1; i >= 0; i-- {
		r := runes[i]
		rw := runewidth.RuneWidth(r)
		if currentWidth+rw > target {
			break
		}
		tail = append(tail, r)
		currentWidth += rw
	}
	for i, j := 0, len(tail)-1; i < j; i, j = i+1, j-1 {
		tail[i], tail[j] = tail[j], tail[i]
	}
	return "…" + string(tail)
}

func trimRunesToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	currentWidth := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if currentWidth+rw > width {
			break
		}
		b.WriteRune(r)
		currentWidth += rw
	}
	return b.String()
}
