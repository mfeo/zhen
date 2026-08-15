package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// OpenTUI gives every `box` a built-in `title` prop with `titleAlignment`.
// Lipgloss has no equivalent, so the rounded border with a centred title has to
// be drawn by hand. This is the spike's answer to "can we reproduce the frame?".
//
// Every width here is measured in terminal *cells* via runewidth, not in runes:
// a CJK ideograph occupies two cells, so rune counts would misalign the frame
// the moment Chinese text appears — which, for this app, is always.

var borderColor = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

// Pane draws a rounded box of exactly width x height cells, with title centred
// on the top border and body clipped/padded to fit inside.
func Pane(width, height int, title, body string) string {
	if width < 4 || height < 2 {
		return ""
	}
	inner := width - 2

	var sb strings.Builder
	sb.WriteString(borderColor.Render(topBorder(inner, title)))
	sb.WriteByte('\n')

	lines := strings.Split(body, "\n")
	for i := 0; i < height-2; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		sb.WriteString(borderColor.Render("│"))
		sb.WriteString(fitCells(line, inner))
		sb.WriteString(borderColor.Render("│"))
		sb.WriteByte('\n')
	}

	sb.WriteString(borderColor.Render("╰" + strings.Repeat("─", inner) + "╯"))
	return sb.String()
}

// topBorder builds "╭──<title>──╮", centring the title and falling back to a
// plain border when the title cannot fit.
func topBorder(inner int, title string) string {
	tw := runewidth.StringWidth(title)
	if title == "" || tw > inner-2 {
		return "╭" + strings.Repeat("─", inner) + "╮"
	}
	left := (inner - tw) / 2
	right := inner - tw - left
	return "╭" + strings.Repeat("─", left) + title + strings.Repeat("─", right) + "╮"
}

// fitCells pads or truncates s to exactly w terminal cells.
//
// Truncation has to be ANSI-aware. runewidth.Truncate counts the bytes of an
// escape sequence as visible characters, so a styled line gets cut in the middle
// of "\x1b[38;2;..." — the colour code is emitted half-written and the text
// after it silently disappears. ansi.Truncate measures printable cells only.
//
// It also never splits a wide rune in half; when the cut falls inside one the
// whole rune is dropped, leaving the line a cell short, so the freed cell is
// padded back. Missing that pad desynchronises a CJK frame by one column.
func fitCells(s string, w int) string {
	cw := lipgloss.Width(s)
	if cw == w {
		return s
	}
	if cw < w {
		return s + strings.Repeat(" ", w-cw)
	}
	out := ansi.Truncate(s, w, "")
	if short := w - lipgloss.Width(out); short > 0 {
		out += strings.Repeat(" ", short)
	}
	return out
}

// WrapCells hard-wraps text to w cells, breaking on spaces where possible and
// mid-string otherwise — Chinese has no spaces, so the fallback is the common
// path, unlike in an English-only TUI.
//
// A single rune wider than w cannot be made to fit and is placed on a line of
// its own, so a returned line exceeds w only when it holds exactly one rune.
func WrapCells(s string, w int) []string {
	if w <= 0 {
		return []string{s}
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		out = append(out, wrapParagraph(para, w)...)
	}
	return out
}

func wrapParagraph(s string, w int) []string {
	if s == "" {
		return []string{""}
	}
	var lines []string
	var line strings.Builder
	lineW := 0
	lastSpace, widthAtSpace := -1, 0

	flush := func() {
		lines = append(lines, line.String())
		line.Reset()
		lineW = 0
		lastSpace, widthAtSpace = -1, 0
	}

	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if lineW+rw > w {
			cur := line.String()
			if lastSpace >= 0 && lastSpace < len(cur) {
				head := strings.TrimRight(cur[:lastSpace], " ")
				tail := cur[lastSpace+1:]
				lines = append(lines, head)
				line.Reset()
				line.WriteString(tail)
				lineW = lineW - widthAtSpace - 1
				lastSpace, widthAtSpace = -1, 0
			} else {
				flush()
			}
		}
		if r == ' ' {
			lastSpace, widthAtSpace = line.Len(), lineW
		}
		line.WriteRune(r)
		lineW += rw
	}
	lines = append(lines, line.String())
	return lines
}
