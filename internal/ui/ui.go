package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Color palette — lazygit-inspired: muted greys, a single soft accent for the
// active panel, semantic green/yellow/red for status & staging signals only.
var (
	Text         = lipgloss.AdaptiveColor{Light: "#1F2430", Dark: "#D7DAE0"}
	Muted        = lipgloss.AdaptiveColor{Light: "#7A8190", Dark: "#7A8190"}
	Subtle       = lipgloss.AdaptiveColor{Light: "#A0A6B0", Dark: "#5A6068"}
	Border       = lipgloss.AdaptiveColor{Light: "#C2C7D0", Dark: "#3A3F4A"}
	BorderActive = lipgloss.AdaptiveColor{Light: "#3D74B0", Dark: "#7AAFE5"}
	Accent       = lipgloss.AdaptiveColor{Light: "#3D74B0", Dark: "#7AAFE5"}
	Success      = lipgloss.AdaptiveColor{Light: "#2C8A4E", Dark: "#5FB875"}
	Warning      = lipgloss.AdaptiveColor{Light: "#A06000", Dark: "#D4A75A"}
	Danger       = lipgloss.AdaptiveColor{Light: "#B43836", Dark: "#E37A75"}
	SelectionBg  = lipgloss.AdaptiveColor{Light: "#D8E3F2", Dark: "#2A3340"}
)

// Panel renders a bordered region. width and height count the OUTER size
// (border-inclusive). Pass active=true to highlight the border.
func Panel(title string, body string, width, height int, active bool) string {
	border := Border
	if active {
		border = BorderActive
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1)

	// lipgloss style.Width sets padding+content, so we subtract just the
	// border to land on the requested outer width after rendering.
	frameW := width - style.GetHorizontalBorderSize()
	if frameW < 1 {
		frameW = 1
	}
	frameH := height - style.GetVerticalBorderSize()
	if frameH < 1 {
		frameH = 1
	}

	contentWidth := frameW - style.GetHorizontalPadding()
	if contentWidth < 1 {
		contentWidth = 1
	}
	contentHeight := frameH - style.GetVerticalPadding()
	if contentHeight < 1 {
		contentHeight = 1
	}

	titleStyled := PanelTitle(title, active, contentWidth)
	bodyHeight := contentHeight - 1
	if bodyHeight < 0 {
		bodyHeight = 0
	}
	bodyLines := strings.Split(body, "\n")
	if len(bodyLines) > bodyHeight {
		bodyLines = bodyLines[:bodyHeight]
	}
	for len(bodyLines) < bodyHeight {
		bodyLines = append(bodyLines, "")
	}
	for i, line := range bodyLines {
		bodyLines[i] = PadRight(TrimToWidth(line, contentWidth), contentWidth)
	}
	content := titleStyled
	if len(bodyLines) > 0 {
		content += "\n" + strings.Join(bodyLines, "\n")
	}
	return style.Width(frameW).Height(frameH).Render(content)
}

// PanelTitle renders a panel title line. When active, the title gets the
// accent color and a small marker so the user can find it without reading
// border colors only.
func PanelTitle(title string, active bool, width int) string {
	prefix := "  "
	color := Muted
	if active {
		prefix = "● "
		color = Accent
	}
	titleStyle := lipgloss.NewStyle().Foreground(color).Bold(active)
	rendered := titleStyle.Render(prefix + title)
	pad := width - lipgloss.Width(rendered)
	if pad > 0 {
		rendered += strings.Repeat(" ", pad)
	} else if pad < 0 {
		rendered = TrimToWidth(rendered, width)
	}
	return rendered
}

// SkillRow formats a single row in the Enabled/Disabled list.
//
// width       = total cell width
// selected    = cursor sits on this row in the panel
// activePanel = whether the panel containing this row currently has focus
// staged      = the row is staged for toggling
//
// Layout (visible columns):
//
//	<staged><cursor><name>SP<source>SP<chars>[SP<description>]
//	  1       2      N    1   7     1   5     1   M
//
// Status (ON/OFF) is conveyed by panel placement plus name color: enabled
// rows leave the foreground unset (terminal default — adapts to dark/light
// themes without relying on Lip Gloss adaptive color), disabled rows use
// Muted. The column algorithm gives name the room it needs first, then
// hands the rest to description; description is dropped when there's less
// than 6 visible columns left so a 1-2 char trailing description doesn't
// look like garbage. Total visible width is held to exactly `width` so the
// outer Panel never has to re-trim a styled string (which would risk
// cutting an ANSI escape sequence in half and producing replacement glyphs
// like `◇` / `�`).
func SkillRow(name, source, description string, descChars int, status string, selected, activePanel, staged bool, width int) string {
	if width < 10 {
		return TrimToWidth(name, width)
	}

	cursor := "  "
	if selected && activePanel {
		cursor = "▌ "
	} else if selected {
		cursor = "› "
	}

	stagedMarker := " "
	if staged {
		stagedMarker = lipgloss.NewStyle().Foreground(Warning).Bold(true).Render("~")
	}

	const (
		stagedW    = 1
		cursorW    = 2
		sourceW    = 7
		charsW     = 5
		nameMin    = 4
		descMin    = 6
		fixedNoDesc   = stagedW + cursorW + 1 + sourceW + 1 + charsW // 17
		fixedWithDesc = fixedNoDesc + 1                              // 18 (desc spacer)
	)

	nameWant := lipgloss.Width(name)

	var nameW, descW int
	switch {
	case width-fixedWithDesc-nameWant >= descMin:
		nameW = nameWant
		descW = width - fixedWithDesc - nameW
	case width-fixedNoDesc >= nameWant:
		nameW = nameWant
		descW = 0
	case width-fixedNoDesc >= nameMin:
		nameW = width - fixedNoDesc
		descW = 0
	default:
		nameW = nameMin
		descW = 0
	}

	nameStyle := lipgloss.NewStyle()
	descStyle := lipgloss.NewStyle().Foreground(Muted)
	if status != "enabled" {
		nameStyle = lipgloss.NewStyle().Foreground(Muted)
		descStyle = lipgloss.NewStyle().Foreground(Subtle)
	}
	sourceStyle := lipgloss.NewStyle().Foreground(Muted)
	charsStyle := lipgloss.NewStyle().Foreground(Subtle)

	parts := []string{
		stagedMarker,
		cursor,
		PadRight(nameStyle.Render(TrimToWidth(name, nameW)), nameW),
		" ",
		PadRight(sourceStyle.Render(TrimToWidth(source, sourceW)), sourceW),
		" ",
		PadRight(charsStyle.Render(FormatDescChars(descChars)), charsW),
	}
	if descW > 0 {
		parts = append(parts, " ", descStyle.Render(TrimToWidth(description, descW)))
	}
	row := strings.Join(parts, "")
	rowWidth := lipgloss.Width(row)
	if rowWidth < width {
		row += strings.Repeat(" ", width-rowWidth)
	}

	if selected && activePanel {
		row = lipgloss.NewStyle().Background(SelectionBg).Render(row)
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
	rendered := prefix + lipgloss.NewStyle().Foreground(Text).Render(query) + cursor
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
func HelpOverlayBox(body string, width int) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Accent).
		Padding(1, 2).
		Foreground(Text)
	innerWidth := width - style.GetHorizontalFrameSize()
	if innerWidth < 20 {
		innerWidth = 20
	}
	return style.Width(innerWidth).Render(body)
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
