package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// Mouse selection is done in the application rather than left to the terminal.
// The two panes sit side by side, so the terminal's own drag-select takes the
// border and whatever is in the other pane along with the text on every row it
// crosses. Selecting inside one pane is something only the application can do.
//
// A selection is expressed in *screen* coordinates — a visible row of a pane
// and a terminal cell column — not as an offset into the text. That keeps the
// same code working for the viewport and for the textarea, neither of which
// exposes a mapping from a screen cell back into its content. The cost is that
// a selection is only meaningful for as long as the pane keeps painting the
// same thing, so scrolling, typing and incoming translation tokens drop it.

type paneID int

const (
	paneNone paneID = iota
	paneInput
	paneOutput
)

// cellPos is a point inside a pane's inner area: row counts visible rows from
// the top of the pane, col counts terminal cells from its left edge.
type cellPos struct{ row, col int }

func (p cellPos) before(q cellPos) bool {
	return p.row < q.row || (p.row == q.row && p.col < q.col)
}

type selection struct {
	pane     paneID
	anchor   cellPos // where the button went down
	head     cellPos // where the pointer is now
	dragging bool    // the button is still held
}

func (s selection) bounds() (from, to cellPos) {
	if s.head.before(s.anchor) {
		return s.head, s.anchor
	}
	return s.anchor, s.head
}

// rowRange is a resolved half-open cell range [from,to) on one visible row.
type rowRange struct{ row, from, to int }

// resolve maps a selection onto the rows it covers. rows are the pane's painted
// rows and may carry ANSI styling; every measurement here is on the stripped
// text, in cells.
//
// The ends snap outwards to whole runes, so dragging across a CJK ideograph
// takes both of its cells: a range that started or stopped inside a wide rune
// would make ansi.Cut drop it, silently shortening what gets copied.
func (s selection) resolve(rows []string) []rowRange {
	if s.pane == paneNone || len(rows) == 0 {
		return nil
	}
	from, to := s.bounds()
	if from == to {
		return nil
	}

	var out []rowRange
	for row := max(from.row, 0); row <= min(to.row, len(rows)-1); row++ {
		plain := ansi.Strip(rows[row])
		width := lipgloss.Width(plain)

		start, end := 0, width
		if row == from.row {
			start = snapStart(plain, from.col)
		}
		if row == to.row {
			end = min(snapEnd(plain, to.col), width)
		}
		if start >= end {
			continue
		}
		out = append(out, rowRange{row: row, from: start, to: end})
	}
	return out
}

// snapStart returns the first cell of the rune covering col, and snapEnd the
// cell just past it. Widths come from runewidth, matching how the panes are
// drawn — see the note at the top of pane.go.
func snapStart(plain string, col int) int {
	cell := 0
	for _, r := range plain {
		w := runewidth.RuneWidth(r)
		if col < cell+w {
			return cell
		}
		cell += w
	}
	return cell
}

func snapEnd(plain string, col int) int {
	cell := 0
	for _, r := range plain {
		w := runewidth.RuneWidth(r)
		if col < cell+w {
			return cell + w
		}
		cell += w
	}
	return cell
}

var selectStyle = lipgloss.NewStyle().Reverse(true)

// highlightRows paints the selection onto a pane's rows. lipgloss.StyleRanges
// indexes in cells and steps over escape sequences, so this works on rows that
// are already styled — the translation's colour, the textarea's placeholder.
func highlightRows(rows []string, ranges []rowRange) []string {
	if len(ranges) == 0 {
		return rows
	}
	out := make([]string, len(rows))
	copy(out, rows)
	for _, r := range ranges {
		out[r.row] = lipgloss.StyleRanges(out[r.row], lipgloss.NewRange(r.from, r.to, selectStyle))
	}
	return out
}

// selectedText is what a release copies to the clipboard: the visible rows,
// joined by newlines.
//
// Rows are what the pane painted, so a translation that wrapped is copied with
// the wrap points as line breaks, exactly as the terminal's own selection would
// do it. Ctrl+Y remains the way to get the whole translation unwrapped.
//
// Trailing spaces are trimmed. The textarea pads its rows out to the full pane
// width, so without this every row would come back with a tail of blanks.
func selectedText(rows []string, ranges []rowRange) string {
	if len(ranges) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		parts = append(parts, strings.TrimRight(ansi.Strip(ansi.Cut(rows[r.row], r.from, r.to)), " "))
	}
	text := strings.Join(parts, "\n")

	// A drag across the empty part of a pane selects real cells but no text, and
	// would otherwise copy a run of newlines and report a successful copy.
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return text
}
