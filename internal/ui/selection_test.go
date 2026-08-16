package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// styled is a row that carries colour, like the ones both panes actually paint.
func styled(s string) string {
	return plainStyle.Render(s)
}

func drag(p paneID, r0, c0, r1, c1 int) selection {
	return selection{
		pane:   p,
		anchor: cellPos{row: r0, col: c0},
		head:   cellPos{row: r1, col: c1},
	}
}

func TestResolveSelectsACellRangeOnOneRow(t *testing.T) {
	rows := []string{"hello world"}

	got := drag(paneOutput, 0, 0, 0, 4).resolve(rows)
	want := []rowRange{{row: 0, from: 0, to: 5}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("resolve = %+v, want %+v", got, want)
	}
	if text := selectedText(rows, got); text != "hello" {
		t.Errorf("selected %q, want %q", text, "hello")
	}
}

// Dragging backwards selects the same range as dragging forwards.
func TestResolveIsDirectionAgnostic(t *testing.T) {
	rows := []string{"hello world"}

	forward := selectedText(rows, drag(paneOutput, 0, 6, 0, 9).resolve(rows))
	backward := selectedText(rows, drag(paneOutput, 0, 9, 0, 6).resolve(rows))
	if forward != "worl" || backward != forward {
		t.Errorf("forward = %q, backward = %q, want both %q", forward, backward, "worl")
	}
}

// A CJK ideograph is two cells wide. Stopping the drag on either of them must
// take the whole character: half of one cannot be copied, and ansi.Cut would
// drop it entirely rather than cut it.
func TestResolveSnapsToWholeWideRunes(t *testing.T) {
	rows := []string{"你好世界"}

	for _, tc := range []struct {
		name         string
		c0, c1       int
		want         string
		wantFrom, to int
	}{
		{"both ends on first cells", 0, 2, "你好", 0, 4},
		{"end on the second cell of 好", 0, 3, "你好", 0, 4},
		{"start on the second cell of 你", 1, 2, "你好", 0, 4},
		{"both ends inside one character", 4, 5, "世", 4, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ranges := drag(paneOutput, 0, tc.c0, 0, tc.c1).resolve(rows)
			if len(ranges) != 1 {
				t.Fatalf("resolve = %+v, want one range", ranges)
			}
			if ranges[0].from != tc.wantFrom || ranges[0].to != tc.to {
				t.Errorf("range = [%d,%d), want [%d,%d)",
					ranges[0].from, ranges[0].to, tc.wantFrom, tc.to)
			}
			if text := selectedText(rows, ranges); text != tc.want {
				t.Errorf("selected %q, want %q", text, tc.want)
			}
		})
	}
}

// Rows between the first and the last are taken whole, and the pane's padding
// is not part of the text.
func TestResolveSpansWholeIntermediateRows(t *testing.T) {
	rows := []string{"first row   ", "middle row  ", "last row    "}

	ranges := drag(paneOutput, 0, 6, 2, 3).resolve(rows)
	if len(ranges) != 3 {
		t.Fatalf("resolve = %+v, want three ranges", ranges)
	}
	want := "row\nmiddle row\nlast"
	if text := selectedText(rows, ranges); text != want {
		t.Errorf("selected %q, want %q", text, want)
	}
}

// A selection that runs past the bottom of the painted rows stops there instead
// of indexing out of range.
func TestResolveClampsToTheRowsThatExist(t *testing.T) {
	rows := []string{"only row"}

	ranges := drag(paneOutput, 0, 0, 9, 5).resolve(rows)
	if len(ranges) != 1 || ranges[0].row != 0 {
		t.Fatalf("resolve = %+v, want a single range on row 0", ranges)
	}
	if text := selectedText(rows, ranges); text != "only row" {
		t.Errorf("selected %q, want the whole row", text)
	}
}

func TestResolveOfAnEmptyOrAbsentSelection(t *testing.T) {
	rows := []string{"hello"}

	if got := (selection{}).resolve(rows); got != nil {
		t.Errorf("no pane resolved to %+v, want nil", got)
	}
	if got := drag(paneOutput, 0, 2, 0, 2).resolve(rows); got != nil {
		t.Errorf("a click without a drag resolved to %+v, want nil", got)
	}
	if got := drag(paneOutput, 0, 0, 0, 3).resolve(nil); got != nil {
		t.Errorf("resolve against no rows = %+v, want nil", got)
	}
}

// Selecting blank space copies nothing rather than a run of newlines.
func TestSelectingEmptySpaceCopiesNothing(t *testing.T) {
	rows := []string{"      ", "      ", "      "}

	if text := selectedText(rows, drag(paneOutput, 0, 0, 2, 5).resolve(rows)); text != "" {
		t.Errorf("selected %q, want the empty string", text)
	}
}

// The panes are drawn by hand and every row must stay exactly as wide as the
// frame, so highlighting may not change a row's cell width — nor its text.
func TestHighlightPreservesRowWidthAndText(t *testing.T) {
	for _, row := range []string{
		styled("你好世界，這是一段中文。"),
		styled("plain ascii text"),
		"unstyled 混合 text",
	} {
		rows := []string{row}
		ranges := drag(paneOutput, 0, 2, 0, 9).resolve(rows)
		got := highlightRows(rows, ranges)[0]

		if lipgloss.Width(got) != lipgloss.Width(row) {
			t.Errorf("row %q: width %d after highlighting, want %d",
				row, lipgloss.Width(got), lipgloss.Width(row))
		}
		if stripANSI(got) != stripANSI(row) {
			t.Errorf("row %q: text became %q", stripANSI(row), stripANSI(got))
		}
	}
}

// The highlight has to actually be emitted, and only over the selected cells.
func TestHighlightCoversOnlyTheSelection(t *testing.T) {
	rows := []string{"你好世界"}
	got := highlightRows(rows, drag(paneOutput, 0, 0, 0, 2).resolve(rows))[0]

	if !strings.Contains(got, "\x1b[7m") {
		t.Fatalf("no reverse-video sequence in %q", got)
	}
	before, after, found := strings.Cut(got, "\x1b[7m")
	if !found || before != "" {
		t.Errorf("highlight starts at %q, want the beginning of the row", before)
	}
	if !strings.HasPrefix(stripANSI(after), "你好") {
		t.Errorf("highlighted %q, want it to start with 你好", stripANSI(after))
	}
}

func TestHighlightWithoutASelectionIsAPassthrough(t *testing.T) {
	rows := []string{styled("hello"), styled("world")}
	got := highlightRows(rows, nil)
	if len(got) != 2 || got[0] != rows[0] || got[1] != rows[1] {
		t.Errorf("rows changed: %q", got)
	}
}

// highlightRows must not scribble on the caller's slice: the rows come straight
// from the widgets' own views.
func TestHighlightDoesNotMutateTheRowsGivenToIt(t *testing.T) {
	rows := []string{"hello world"}
	original := rows[0]

	highlightRows(rows, drag(paneOutput, 0, 0, 0, 4).resolve(rows))
	if rows[0] != original {
		t.Errorf("input row became %q, want %q", rows[0], original)
	}
}
