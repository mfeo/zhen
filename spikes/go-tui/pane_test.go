package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

// These are the tests that matter most for a Chinese/English tool: every frame
// line must be exactly `width` terminal cells, whatever mix of half-width and
// full-width runes it contains. A rune-count-based implementation passes the
// ASCII cases and fails every CJK one.

func TestPaneLinesAreExactCellWidth(t *testing.T) {
	cases := []struct {
		name  string
		title string
		body  string
	}{
		{"ascii", " English Translation ", "hello\nworld"},
		{"cjk title", " 中文翻譯 ", "abc"},
		{"cjk body", " 中文翻譯 ", "今天天氣很好\n我要去公園"},
		{"mixed", " 中文輸入 ", "Hello 世界 mixed 內容"},
		{"empty body", " 中文輸入 ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, w := range []int{20, 21, 40, 41} {
				out := Pane(w, 6, tc.title, tc.body)
				lines := strings.Split(out, "\n")
				if len(lines) != 6 {
					t.Fatalf("width %d: got %d lines, want 6", w, len(lines))
				}
				for i, line := range lines {
					if got := lipgloss.Width(line); got != w {
						t.Errorf("width %d, line %d: %d cells, want %d (%q)", w, i, got, w, line)
					}
				}
			}
		})
	}
}

// Body content wider than the pane must be clipped without splitting a
// full-width rune in half — a half-rune would desynchronise the whole frame.
func TestPaneClipsWideRunesCleanly(t *testing.T) {
	// Inner width 9 (odd) against 2-cell runes forces the awkward case.
	out := Pane(11, 3, "", "一二三四五六")
	body := strings.Split(out, "\n")[1]
	if got := lipgloss.Width(body); got != 11 {
		t.Fatalf("clipped line is %d cells, want 11: %q", got, body)
	}
	// Four 2-cell runes fill 8 of the 9 inner cells; the ninth must be padded
	// rather than left short, and no fifth rune may be half-drawn.
	if !strings.Contains(body, "一二三四 ") {
		t.Errorf("expected 4 runes plus a pad space, got %q", body)
	}
	if strings.Contains(body, "五") {
		t.Errorf("a rune that does not fit was drawn anyway: %q", body)
	}
}

func TestPaneTitleIsCentred(t *testing.T) {
	top := strings.Split(Pane(21, 3, " 中文翻譯 ", ""), "\n")[0]
	if !strings.Contains(top, "中文翻譯") {
		t.Fatalf("title missing from top border: %q", top)
	}
	left := strings.Index(top, "中")
	// Count dashes on each side; they should differ by at most one.
	before := strings.Count(top[:left], "─")
	after := strings.Count(top[left:], "─")
	if diff := before - after; diff > 1 || diff < -1 {
		t.Errorf("title not centred: %d dashes before, %d after (%q)", before, after, top)
	}
}

// A title too wide for the pane must degrade to a plain border rather than
// overflow and break the width invariant.
func TestPaneOversizedTitleFallsBack(t *testing.T) {
	out := Pane(8, 3, " 一個非常長的標題 ", "")
	top := strings.Split(out, "\n")[0]
	if lipgloss.Width(top) != 8 {
		t.Fatalf("top border is %d cells, want 8: %q", lipgloss.Width(top), top)
	}
	if strings.Contains(top, "標題") {
		t.Errorf("oversized title should have been dropped: %q", top)
	}
}

func TestPaneRejectsDegenerateSizes(t *testing.T) {
	for _, tc := range []struct{ w, h int }{{0, 0}, {3, 5}, {10, 1}} {
		if out := Pane(tc.w, tc.h, "t", "body"); out != "" {
			t.Errorf("Pane(%d,%d) should return empty, got %q", tc.w, tc.h, out)
		}
	}
}

// --- WrapCells ---

func TestWrapCellsNeverExceedsWidth(t *testing.T) {
	inputs := []string{
		"今天天氣很好，我打算去公園散步，順便買一杯咖啡。",
		"The weather is nice today so I plan to walk in the park.",
		"Mixed 中英文 content that 需要 wrapping 正確處理",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	for _, in := range inputs {
		for _, w := range []int{1, 2, 3, 7, 20} {
			for _, line := range WrapCells(in, w) {
				got := runewidth.StringWidth(line)
				if got <= w {
					continue
				}
				// The one legal overflow: a single rune wider than the pane.
				if len([]rune(line)) == 1 {
					continue
				}
				t.Errorf("width %d: line %q is %d cells", w, line, got)
			}
		}
	}
}

// Wrapping must not lose or duplicate characters.
func TestWrapCellsPreservesContent(t *testing.T) {
	in := "Mixed 中英文 content 需要 wrapping"
	for _, w := range []int{2, 5, 11, 30} {
		joined := strings.Join(WrapCells(in, w), "")
		stripped := strings.ReplaceAll(in, " ", "")
		if strings.ReplaceAll(joined, " ", "") != stripped {
			t.Errorf("width %d: content changed\n got %q\nwant %q", w, joined, stripped)
		}
	}
}

func TestWrapCellsKeepsExplicitNewlines(t *testing.T) {
	got := WrapCells("one\ntwo", 20)
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("got %q, want [one two]", got)
	}
}

func TestWrapCellsBreaksOnSpaces(t *testing.T) {
	got := WrapCells("hello world", 7)
	if len(got) != 2 || got[0] != "hello" || got[1] != "world" {
		t.Fatalf("got %q, want [hello world]", got)
	}
}

// Chinese has no spaces, so the mid-string break is the common path.
func TestWrapCellsBreaksMidStringForCJK(t *testing.T) {
	got := WrapCells("今天天氣很好", 4)
	if len(got) != 3 {
		t.Fatalf("got %d lines %q, want 3", len(got), got)
	}
	if got[0] != "今天" {
		t.Errorf("first line = %q, want 今天", got[0])
	}
}

func TestWrapCellsNonPositiveWidth(t *testing.T) {
	if got := WrapCells("abc", 0); len(got) != 1 || got[0] != "abc" {
		t.Fatalf("got %q, want [abc]", got)
	}
}

// --- fitCells ---

func TestFitCells(t *testing.T) {
	cases := []struct {
		in   string
		w    int
		want int
	}{
		{"", 5, 5},
		{"abc", 5, 5},
		{"abcdefgh", 5, 5},
		{"中文", 5, 5},
		{"中文字串", 5, 5},
		{"中文", 4, 4},
	}
	for _, tc := range cases {
		if got := lipgloss.Width(fitCells(tc.in, tc.w)); got != tc.want {
			t.Errorf("fitCells(%q,%d) is %d cells, want %d", tc.in, tc.w, got, tc.want)
		}
	}
}

// A styled string must be truncated by printable cells, not by bytes: cutting
// inside an ANSI escape sequence emits a half-written colour code and swallows
// the text after it. This is exactly how a long error message vanished from the
// status bar instead of being clipped.
func TestFitCellsTruncatesStyledTextByCells(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("hints") +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Render(" Error: something went wrong")

	got := fitCells(styled, 20)
	if w := lipgloss.Width(got); w != 20 {
		t.Fatalf("width = %d, want 20", w)
	}
	if !strings.Contains(got, "Error:") {
		t.Errorf("visible text was swallowed by escape-sequence truncation: %q", got)
	}
}
