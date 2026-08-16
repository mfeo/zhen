package ui

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	"zhen/internal/config"
)

// These drive the real Bubble Tea program against a fake Ollama and assert both
// on what reaches the terminal and on the final model state. This is the
// capability OpenTUI + React has no equivalent for today, and a major reason the
// port is attractive.

func fakeOllama(t *testing.T, chunks ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			io.WriteString(w, `{"models":[]}`)
			return
		}
		flusher := w.(http.Flusher)
		for _, c := range chunks {
			io.WriteString(w, `{"response":`+quote(c)+`}`+"\n")
			flusher.Flush()
		}
		io.WriteString(w, `{"done":true}`+"\n")
		flusher.Flush()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		if r == '"' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

// recorder accumulates everything the program ever wrote.
//
// Two teatest sharp edges make this necessary. First, tm.Output() is a
// *bytes.Buffer, so reading drains it — and Bubble Tea repaints only the lines
// that changed, so calling teatest.WaitFor twice in a row silently loses every
// frame the first call already consumed. Second, that buffer reports io.EOF
// whenever it happens to be empty, so a plain io.Copy returns instantly with
// nothing. The drain loop below treats EOF as "nothing yet" and keeps polling.
type recorder struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (r *recorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(p)
}

func (r *recorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

func newTUI(t *testing.T, url string) (*teatest.TestModel, *recorder) {
	t.Helper()
	m := newModel(config.Config{BaseURL: url, Model: "test-model"})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 20))
	rec := &recorder{}

	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		out := tm.Output()
		buf := make([]byte, 8192)
		for {
			select {
			case <-stop:
				return
			default:
			}
			n, err := out.Read(buf)
			if n > 0 {
				rec.Write(buf[:n])
				continue
			}
			if err != nil && err != io.EOF {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	return tm, rec
}

func ctrl(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
}

// waitForPainted polls the accumulated output until every wanted substring has
// been painted at some point.
func waitForPainted(t *testing.T, rec *recorder, want ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		out := rec.String()
		missing := ""
		for _, w := range want {
			if !strings.Contains(out, w) {
				missing = w
				break
			}
		}
		if missing == "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("never painted %q\n--- output ---\n%s", missing, out)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func quit(t *testing.T, tm *teatest.TestModel) model {
	t.Helper()
	tm.Send(ctrl('q'))
	return tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(model)
}

func TestInitialFrameShowsZH2ENLayout(t *testing.T) {
	tm, rec := newTUI(t, fakeOllama(t).URL)
	waitForPainted(t, rec,
		"ZH→EN",
		"中文輸入",
		"English Translation",
		"Translation will appear here",
		"^T: 翻譯",
	)
	quit(t, tm)
}

func TestCtrlLTogglesDirectionAndClearsPanes(t *testing.T) {
	tm, rec := newTUI(t, fakeOllama(t, "Hello").URL)
	waitForPainted(t, rec, "ZH→EN")

	tm.Type("你好")
	tm.Send(ctrl('t'))
	waitForPainted(t, rec, "Hello")

	tm.Send(ctrl('l'))
	waitForPainted(t, rec, "EN→ZH", "English Input", "中文翻譯")

	final := quit(t, tm)
	if final.direction != config.EN2ZH {
		t.Errorf("direction = %v, want config.EN2ZH", final.direction.Label())
	}
	// The previous translation must be gone, not left showing a result for the
	// opposite direction.
	if final.outputText != "" {
		t.Errorf("output not cleared on toggle: %q", final.outputText)
	}
	if final.input.Value() != "" {
		t.Errorf("input not cleared on toggle: %q", final.input.Value())
	}
	if final.input.Placeholder != config.EN2ZH.Placeholder() {
		t.Errorf("placeholder = %q, want the en2zh one", final.input.Placeholder)
	}
}

func TestTranslateStreamsIntoOutputPane(t *testing.T) {
	tm, rec := newTUI(t, fakeOllama(t, "The ", "weather ", "is ", "nice").URL)
	waitForPainted(t, rec, "ZH→EN")

	tm.Type("今天天氣很好")
	tm.Send(ctrl('t'))

	// TTFT is surfaced in the status bar as soon as the first token lands.
	waitForPainted(t, rec, "TTFT")
	waitForPainted(t, rec, "The weather is nice")

	final := quit(t, tm)
	if final.outputText != "The weather is nice" {
		t.Errorf("outputText = %q", final.outputText)
	}
	if final.translating {
		t.Error("still marked as translating after the stream finished")
	}
}

func TestCtrlTOnEmptyInputDoesNothing(t *testing.T) {
	tm, rec := newTUI(t, fakeOllama(t, "SHOULD NOT APPEAR").URL)
	waitForPainted(t, rec, "Translation will appear here")

	tm.Send(ctrl('t'))
	time.Sleep(200 * time.Millisecond)

	final := quit(t, tm)
	if strings.Contains(rec.String(), "SHOULD NOT APPEAR") || final.outputText != "" {
		t.Error("an empty input must not trigger a request")
	}
	if final.gen != 0 {
		t.Errorf("gen advanced to %d without a request", final.gen)
	}
}

// Whitespace-only input is the boundary case of the same rule.
func TestCtrlTOnWhitespaceInputDoesNothing(t *testing.T) {
	tm, rec := newTUI(t, fakeOllama(t, "SHOULD NOT APPEAR").URL)
	waitForPainted(t, rec, "Translation will appear here")

	tm.Type("   ")
	tm.Send(ctrl('t'))
	time.Sleep(200 * time.Millisecond)

	final := quit(t, tm)
	if final.gen != 0 || final.outputText != "" {
		t.Errorf("whitespace input triggered a request (gen=%d, out=%q)", final.gen, final.outputText)
	}
}

// End to end: a drag in the real program must put the selected text on the
// system clipboard, which over a terminal means an OSC 52 sequence on stdout.
// No translation is involved, so nothing here depends on timing.
func TestDragCopiesThroughOSC52(t *testing.T) {
	tm, rec := newTUI(t, fakeOllama(t).URL)
	waitForPainted(t, rec, "Translation will appear here")

	tm.Send(press(outputX0, firstRow))
	tm.Send(moveTo(outputX0+10, firstRow))
	tm.Send(release(outputX0+10, firstRow))

	waitForPainted(t, rec, osc52Sequence("Translation"))
	quit(t, tm)
}

func TestCopyWithNothingToCopyShowsStatus(t *testing.T) {
	tm, rec := newTUI(t, fakeOllama(t).URL)
	waitForPainted(t, rec, "Translation will appear here")

	tm.Send(ctrl('y'))
	waitForPainted(t, rec, "Nothing to copy")
	quit(t, tm)
}

func TestServerErrorIsShownInStatusBar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			io.WriteString(w, `{"models":[]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":"model not found"}`)
	}))
	defer srv.Close()

	tm, rec := newTUI(t, srv.URL)
	waitForPainted(t, rec, "ZH→EN")
	tm.Type("你好")
	tm.Send(ctrl('t'))
	waitForPainted(t, rec, "Error:")

	final := quit(t, tm)
	if final.translating {
		t.Error("still marked as translating after an error")
	}
}

// --- pure model tests, no terminal involved ---

// A late chunk from a superseded request must be dropped, or two translations
// interleave in the output pane. The generation counter is what prevents it.
func TestStaleStreamEventsAreDropped(t *testing.T) {
	m := testModel()
	m.gen = 5
	m.outputText = "current"

	got, _ := m.applyStream(streamEvent{gen: 4, chunk: "STALE"})
	if got.(model).outputText != "current" {
		t.Fatalf("stale chunk leaked into output: %q", got.(model).outputText)
	}

	got, _ = m.applyStream(streamEvent{gen: 5, chunk: " fresh"})
	if got.(model).outputText != "current fresh" {
		t.Fatalf("current-generation chunk was dropped: %q", got.(model).outputText)
	}
}

// A stale terminal event must not clear the translating flag of the request that
// superseded it, or the status bar lies about work still in flight.
func TestStaleDoneEventDoesNotStopCurrentRequest(t *testing.T) {
	m := testModel()
	m.gen = 5
	m.translating = true

	got, _ := m.applyStream(streamEvent{gen: 4, done: true})
	if !got.(model).translating {
		t.Fatal("a stale done event cleared the current request's translating flag")
	}
}

// A cancellation is a normal outcome of Ctrl+L or a re-translate, so it must not
// be reported to the user as an error.
func TestCancelledStreamShowsNoError(t *testing.T) {
	m := testModel()
	m.translating = true

	got, _ := m.applyStream(streamEvent{gen: m.gen, done: true, err: context.Canceled})
	after := got.(model)
	if after.status != "" {
		t.Fatalf("cancellation surfaced as status %q", after.status)
	}
	if after.translating {
		t.Error("translating flag not cleared")
	}
}

func TestRealErrorIsSurfaced(t *testing.T) {
	m := testModel()
	m.translating = true

	got, _ := m.applyStream(streamEvent{gen: m.gen, done: true, err: io.ErrUnexpectedEOF})
	after := got.(model)
	if !strings.Contains(after.status, "unexpected EOF") {
		t.Fatalf("error not surfaced, status = %q", after.status)
	}
}

// A stale clearStatusMsg must not wipe a newer status message.
func TestStaleStatusClearIsIgnored(t *testing.T) {
	m := testModel()
	mi, _ := m.withStatus("newer", okStyle)
	m = mi.(model)

	got, _ := m.Update(clearStatusMsg{token: m.statusToken - 1})
	if got.(model).status != "newer" {
		t.Fatalf("stale clear wiped the status: %q", got.(model).status)
	}

	got, _ = got.(model).Update(clearStatusMsg{token: m.statusToken})
	if got.(model).status != "" {
		t.Fatalf("matching clear did not wipe the status: %q", got.(model).status)
	}
}

func testModel() model {
	m := newModel(config.Config{BaseURL: "http://unused", Model: "m"})
	m.width, m.height = 80, 20
	m.layout()
	return m
}

// A stream that ends without producing any text must not leave the output pane
// stuck on the "Translating..." placeholder.
func TestFailedStreamRestoresPlaceholder(t *testing.T) {
	m := testModel()
	m.translating = true
	m.refreshOutput()
	if !strings.Contains(m.output.GetContent(), "Translating...") {
		t.Fatalf("precondition failed, pane shows %q", m.output.GetContent())
	}

	got, _ := m.applyStream(streamEvent{gen: m.gen, done: true, err: io.ErrUnexpectedEOF})
	after := got.(model)
	if strings.Contains(after.output.GetContent(), "Translating...") {
		t.Errorf("pane still stuck on the placeholder: %q", after.output.GetContent())
	}
	if !strings.Contains(after.output.GetContent(), "Translation will appear here") {
		t.Errorf("pane did not return to its idle text: %q", after.output.GetContent())
	}
}

// --- terminal size handling ---

// Resizing must re-lay-out both widgets, not just the frame drawn around them.
func TestResizeRelaysOutWidgets(t *testing.T) {
	m := testModel()

	got, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	wide := got.(model)
	if wide.output.Width() != 120-60-2 {
		t.Errorf("output width = %d, want %d", wide.output.Width(), 120-60-2)
	}
	if wide.input.Width() != 60-2 {
		t.Errorf("input width = %d, want %d", wide.input.Width(), 60-2)
	}
	if wide.output.Height() != 40-1-2 {
		t.Errorf("output height = %d, want %d", wide.output.Height(), 40-1-2)
	}

	got, _ = wide.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	narrow := got.(model)
	if narrow.output.Width() != 40-20-2 {
		t.Errorf("output width after shrink = %d, want %d", narrow.output.Width(), 40-20-2)
	}
}

// Existing output must survive a resize, re-wrapped to the new width rather
// than dropped.
func TestResizeRewrapsExistingOutput(t *testing.T) {
	m := testModel()
	m.outputText = "The weather is nice today and I plan to walk in the park"

	got, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	after := got.(model)
	if after.outputText != m.outputText {
		t.Fatalf("output text lost on resize: %q", after.outputText)
	}
	for _, line := range WrapCells(after.outputText, after.output.Width()) {
		if lipgloss.Width(line) > after.output.Width() {
			t.Errorf("line %q exceeds the new pane width %d", line, after.output.Width())
		}
	}
}

// A terminal too small for the frame must say so rather than render blank. The
// message wraps to the available width, so the assertion is on the text with
// line breaks removed.
func TestTinyTerminalShowsAMessage(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{
		{Width: 10, Height: 20},
		{Width: 80, Height: 3},
		{Width: 23, Height: 4},
	} {
		m := testModel()
		got, _ := m.Update(size)
		view := got.(model).View().Content

		// Re-join across the wrap points and the padding fitCells added.
		flat := strings.Join(strings.Fields(stripANSI(view)), " ")
		if !strings.Contains(flat, "Terminal too small") {
			t.Errorf("%dx%d rendered %q, want a size warning", size.Width, size.Height, view)
		}
		for i, line := range strings.Split(view, "\n") {
			if w := lipgloss.Width(line); w != size.Width {
				t.Errorf("%dx%d line %d is %d cells, want %d", size.Width, size.Height, i, w, size.Width)
			}
		}
	}
}

// The degenerate case: too narrow to fit even one word, and only one row. It
// must not panic and must not overflow the single row it has.
func TestOneByOneTerminalDoesNotPanic(t *testing.T) {
	m := testModel()
	got, _ := m.Update(tea.WindowSizeMsg{Width: 1, Height: 1})
	view := got.(model).View().Content
	if n := len(strings.Split(view, "\n")); n != 1 {
		t.Errorf("rendered %d rows into a 1-row terminal", n)
	}
}

// stripANSI removes SGR escape sequences so assertions can look at plain text.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// At the minimum size the real frame must render, and stay exactly that wide.
func TestMinimumSizeRendersTheFrame(t *testing.T) {
	m := testModel()
	got, _ := m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})
	view := got.(model).View().Content
	if strings.Contains(view, "Terminal too small") {
		t.Fatalf("%dx%d should render the frame, got %q", minWidth, minHeight, view)
	}
	for i, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w != minWidth {
			t.Errorf("line %d is %d cells, want %d", i, w, minWidth)
		}
	}
}

// --- output pane scrolling ---

// scrollMarker sits at the very start of the scrollable fixture's output.
const scrollMarker = "起點ALPHA。"

// scrollableModel returns a model whose output is twice as tall as the pane, so
// roughly half of it is off-screen at any time.
func scrollableModel(t *testing.T) model {
	t.Helper()
	m := testModel()
	// The marker makes the first line identifiable; the body repeats, so without
	// it "is the top visible?" cannot be told apart from any other line.
	m.outputText = scrollMarker + strings.Repeat("這是一段很長的翻譯結果，用來測試捲動。", 40)
	m.refreshOutput()
	if m.output.TotalLineCount() <= m.output.Height() {
		t.Fatalf("fixture does not overflow: %d lines in a %d-row pane",
			m.output.TotalLineCount(), m.output.Height())
	}
	return m
}

func shift(c rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: c, Mod: tea.ModShift}
}

func send(t *testing.T, m model, msgs ...tea.Msg) model {
	t.Helper()
	for _, msg := range msgs {
		got, _ := m.Update(msg)
		m = got.(model)
	}
	return m
}

// The whole point: text scrolled past the top of the pane must be reachable
// again. Before the fix the viewport never received an Update and this was
// impossible.
func TestShiftArrowsScrollTheOutputPane(t *testing.T) {
	m := scrollableModel(t)
	bottom := m.output.YOffset()
	if bottom == 0 {
		t.Fatal("overflowing output should start scrolled to the bottom")
	}

	up := send(t, m, shift(tea.KeyUp))
	if up.output.YOffset() != bottom-1 {
		t.Errorf("shift+up gave offset %d, want %d", up.output.YOffset(), bottom-1)
	}

	down := send(t, up, shift(tea.KeyDown))
	if down.output.YOffset() != bottom {
		t.Errorf("shift+down gave offset %d, want %d", down.output.YOffset(), bottom)
	}
}

// Scrolling all the way up must actually reveal the first line of the
// translation, not merely move a counter.
func TestScrollingUpRevealsTheStartOfTheTranslation(t *testing.T) {
	m := scrollableModel(t)
	if strings.Contains(stripANSI(m.output.View()), scrollMarker) {
		t.Fatal("fixture already shows its first line; nothing is off-screen")
	}
	for i := m.output.YOffset(); i > 0; i-- {
		m = send(t, m, shift(tea.KeyUp))
	}
	if !strings.Contains(stripANSI(m.output.View()), scrollMarker) {
		t.Errorf("first line %q still not visible after scrolling to the top:\n%s",
			scrollMarker, stripANSI(m.output.View()))
	}
}

func TestShiftPageKeysScrollTheOutputPane(t *testing.T) {
	m := scrollableModel(t)
	bottom := m.output.YOffset()

	up := send(t, m, shift(tea.KeyPgUp))
	if up.output.YOffset() >= bottom {
		t.Errorf("shift+pgup gave offset %d, want less than %d", up.output.YOffset(), bottom)
	}

	down := send(t, up, shift(tea.KeyPgDown))
	if down.output.YOffset() != bottom {
		t.Errorf("shift+pgdown gave offset %d, want back at %d", down.output.YOffset(), bottom)
	}
}

func TestMouseWheelScrollsTheOutputPane(t *testing.T) {
	m := scrollableModel(t)
	bottom := m.output.YOffset()

	up := send(t, m, tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if up.output.YOffset() != bottom-mouseWheelLines {
		t.Errorf("wheel up gave offset %d, want %d", up.output.YOffset(), bottom-mouseWheelLines)
	}

	down := send(t, up, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if down.output.YOffset() != bottom {
		t.Errorf("wheel down gave offset %d, want %d", down.output.YOffset(), bottom)
	}
}

// The scroll keys belong to the output pane; the focused textarea must not see
// them as cursor movement or, worse, as text.
func TestScrollKeysDoNotDisturbTheInput(t *testing.T) {
	m := scrollableModel(t)
	m.input.SetValue("你好\n世界")
	before := m.input.Value()
	line, col := m.input.Line(), m.input.LineInfo().ColumnOffset

	m = send(t, m,
		shift(tea.KeyUp), shift(tea.KeyDown),
		shift(tea.KeyPgUp), shift(tea.KeyPgDown),
		tea.MouseWheelMsg{Button: tea.MouseWheelUp},
	)

	if m.input.Value() != before {
		t.Errorf("input text changed to %q, want %q", m.input.Value(), before)
	}
	if m.input.Line() != line || m.input.LineInfo().ColumnOffset != col {
		t.Errorf("input cursor moved to (%d,%d), want (%d,%d)",
			m.input.Line(), m.input.LineInfo().ColumnOffset, line, col)
	}
}

// Sticky scroll must be conditional: while the user is reading further up, an
// arriving token must not yank the view back to the bottom.
func TestStreamingDoesNotStealAManualScrollPosition(t *testing.T) {
	m := scrollableModel(t)
	m.gen, m.translating = 1, true
	m = send(t, m, shift(tea.KeyPgUp))
	parked := m.output.YOffset()

	m = send(t, m, streamMsg{gen: 1, chunk: "更多的譯文內容持續串流進來。"})
	if m.output.YOffset() != parked {
		t.Errorf("offset moved to %d during streaming, want it parked at %d",
			m.output.YOffset(), parked)
	}
	if !strings.HasSuffix(m.outputText, "更多的譯文內容持續串流進來。") {
		t.Error("streamed chunk was dropped")
	}
}

// ...but the default remains stickiness: a reader sitting at the bottom keeps
// following the stream.
func TestStreamingFollowsTheBottomByDefault(t *testing.T) {
	m := scrollableModel(t)
	m.gen, m.translating = 1, true

	m = send(t, m, streamMsg{gen: 1, chunk: strings.Repeat("新的內容。", 20)})
	if !m.output.AtBottom() {
		t.Errorf("offset %d is not at the bottom of %d lines",
			m.output.YOffset(), m.output.TotalLineCount())
	}
}

// Boundaries: scrolling cannot run off either end.
func TestScrollingIsClampedAtBothEnds(t *testing.T) {
	m := scrollableModel(t)

	for i := 0; i < m.output.TotalLineCount()+10; i++ {
		m = send(t, m, shift(tea.KeyUp))
	}
	if m.output.YOffset() != 0 {
		t.Errorf("offset %d after over-scrolling up, want 0", m.output.YOffset())
	}
	if !m.output.AtTop() {
		t.Error("AtTop() is false after scrolling to the top")
	}

	for i := 0; i < m.output.TotalLineCount()+10; i++ {
		m = send(t, m, shift(tea.KeyDown))
	}
	if !m.output.AtBottom() {
		t.Errorf("offset %d after over-scrolling down, want the bottom", m.output.YOffset())
	}
}

// Output that fits in the pane has nothing to scroll, and scroll keys must not
// push the visible text off-screen.
func TestScrollKeysAreInertWhenOutputFits(t *testing.T) {
	m := testModel()
	m.outputText = "Short."
	m.refreshOutput()

	m = send(t, m, shift(tea.KeyUp), shift(tea.KeyPgUp), tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m.output.YOffset() != 0 {
		t.Errorf("offset %d, want 0 for content that fits", m.output.YOffset())
	}
	if !strings.Contains(stripANSI(m.output.View()), "Short.") {
		t.Error("visible text disappeared")
	}
}

// A parked scroll position belongs to the old translation: starting a new one,
// or switching direction, must return to the top of the fresh content.
func TestNewTranslationResetsTheScrollPosition(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"ctrl+t", ctrl('t')},
		{"ctrl+l", ctrl('l')},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := scrollableModel(t)
			m.input.SetValue("你好")
			m = send(t, m, shift(tea.KeyPgUp))
			if m.output.YOffset() == 0 {
				t.Skip("pane is too tall for this fixture to scroll")
			}

			m = send(t, m, tc.key)
			m.cancelStream()
			if m.output.YOffset() != 0 {
				t.Errorf("offset %d after %s, want 0", m.output.YOffset(), tc.name)
			}
		})
	}
}

// --- cursor placement ---

// The terminal's real cursor must be positioned inside the input pane. An IME
// anchors its candidate window to it, so a cursor left unset parks the popup in
// the bottom-right corner of the screen instead of beside the caret.
func TestCursorIsPlacedInsideTheInputPane(t *testing.T) {
	m := testModel()

	c := m.View().Cursor
	if c == nil {
		t.Fatal("no cursor reported; the terminal cursor would stay where the renderer left it")
	}
	// (1,1) is the first cell inside the left pane's border.
	if c.Position.X != 1 || c.Position.Y != 1 {
		t.Errorf("empty input put the cursor at (%d,%d), want (1,1)", c.Position.X, c.Position.Y)
	}
}

// Cursor columns are terminal cells: a CJK ideograph is two of them, so a rune
// count would leave the caret — and the IME popup — half a pane to the left.
func TestCursorAdvancesByCellsNotRunes(t *testing.T) {
	for _, tc := range []struct {
		value string
		wantX int
	}{
		{"ab", 1 + 2},
		{"你好世", 1 + 6},
		{"你a好", 1 + 5},
	} {
		m := testModel()
		m.input.SetValue(tc.value)
		c := m.View().Cursor
		if c == nil {
			t.Fatalf("%q: no cursor reported", tc.value)
		}
		if c.Position.X != tc.wantX {
			t.Errorf("%q put the cursor at column %d, want %d", tc.value, c.Position.X, tc.wantX)
		}
	}
}

func TestCursorFollowsTheInputDownTheLines(t *testing.T) {
	m := testModel()
	m.input.SetValue("你好\n世界ab")

	c := m.View().Cursor
	if c == nil {
		t.Fatal("no cursor reported")
	}
	if c.Position.Y != 2 {
		t.Errorf("cursor row = %d, want 2 (border + one line)", c.Position.Y)
	}
	if c.Position.X != 1+6 {
		t.Errorf("cursor column = %d, want %d", c.Position.X, 1+6)
	}
}

// typeInput drives text through Update the way a real keyboard does, rather
// than SetValue: only the update loop repositions the textarea's own viewport.
func typeInput(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		if r == '\n' {
			m = send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
			continue
		}
		m = send(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// Input taller than the pane scrolls inside the textarea; the cursor must stay
// within the frame rather than being reported below it.
func TestCursorStaysInsideTheFrameWhenInputOverflows(t *testing.T) {
	m := typeInput(t, testModel(), strings.Repeat("很長的一行輸入內容。\n", 40))

	c := m.View().Cursor
	if c == nil {
		t.Fatal("no cursor reported")
	}
	paneW, contentH := m.width/2, m.height-1
	if c.Position.Y < 1 || c.Position.Y > contentH-2 {
		t.Errorf("cursor row %d is outside the pane's rows 1..%d", c.Position.Y, contentH-2)
	}
	if c.Position.X < 1 || c.Position.X > paneW-2 {
		t.Errorf("cursor column %d is outside the pane's columns 1..%d", c.Position.X, paneW-2)
	}
}

// No frame is drawn below the minimum size and before the first resize, so
// there is nowhere to put a cursor and none must be reported.
func TestNoCursorWhenTheFrameIsNotDrawn(t *testing.T) {
	if c := newModel(config.Config{}).View().Cursor; c != nil {
		t.Errorf("cursor reported at (%d,%d) before the first resize", c.Position.X, c.Position.Y)
	}

	m := testModel()
	got, _ := m.Update(tea.WindowSizeMsg{Width: 10, Height: 20})
	if c := got.(model).View().Cursor; c != nil {
		t.Errorf("cursor reported at (%d,%d) on a too-small terminal", c.Position.X, c.Position.Y)
	}
}

// --- mouse selection ---

// press, moveTo and release are the three messages a drag is made of. The
// coordinates are absolute terminal cells, as the terminal reports them.
func press(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func moveTo(x, y int) tea.MouseMotionMsg {
	return tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func release(x, y int) tea.MouseReleaseMsg {
	return tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// In an 80x20 terminal the left pane is 40 wide, so the output pane's first
// text cell is at x=41 and both panes' first text row is y=1.
const (
	inputX0  = 1
	outputX0 = 41
	firstRow = 1
)

func TestDraggingInTheOutputPaneSelectsAndCopies(t *testing.T) {
	m := testModel()
	m.outputText = "你好世界abc"
	m.refreshOutput()

	m = send(t, m, press(outputX0, firstRow), moveTo(outputX0+3, firstRow))
	if got := m.selectionText(); got != "你好" {
		t.Errorf("mid-drag selection = %q, want 你好", got)
	}

	got, cmd := m.Update(release(outputX0+3, firstRow))
	after := got.(model)
	if after.selectionText() != "你好" {
		t.Errorf("selection after release = %q, want it to stay", after.selectionText())
	}
	if after.sel.dragging {
		t.Error("still dragging after the button came up")
	}
	if cmd == nil {
		t.Fatal("release returned no command, so nothing was copied")
	}
	if after.status != "✓ Copied!" {
		t.Errorf("status = %q, want the copy confirmation", after.status)
	}
}

func TestDraggingInTheInputPaneSelectsAndCopies(t *testing.T) {
	m := typeInput(t, testModel(), "hello 世界")

	m = send(t, m, press(inputX0, firstRow), moveTo(inputX0+8, firstRow), release(inputX0+8, firstRow))
	if got := m.selectionText(); got != "hello 世界" {
		t.Errorf("selection = %q, want the whole line", got)
	}
	if m.status != "✓ Copied!" {
		t.Errorf("status = %q, want the copy confirmation", m.status)
	}
}

// The selected text must be the pane's own text, never the border or whatever
// the other pane happens to have on the same rows. That is the entire reason
// this is done in the application instead of left to the terminal.
func TestSelectionNeverPicksUpTheOtherPaneOrTheBorder(t *testing.T) {
	m := testModel()
	m.input.SetValue("這是輸入區的文字")
	m.outputText = "這是輸出區的文字"
	m.refreshOutput()

	// Drag right across the output pane, from its first cell well past its edge.
	m = send(t, m, press(outputX0, firstRow), moveTo(200, firstRow), release(200, firstRow))

	got := m.selectionText()
	if got != "這是輸出區的文字" {
		t.Errorf("selected %q, want only the output pane's text", got)
	}
	if strings.ContainsAny(got, "│╭╮╰╯") {
		t.Errorf("selection %q contains frame characters", got)
	}
}

// A drag that leaves the pane extends the selection to its edge rather than
// being ignored or escaping into the neighbouring pane.
func TestDragOutsideThePaneIsClamped(t *testing.T) {
	m := testModel()
	m.outputText = "第一行的文字\n第二行的文字"
	m.refreshOutput()

	m = send(t, m, press(outputX0, firstRow), moveTo(-30, 400), release(-30, 400))
	if m.sel.head.row < 0 || m.sel.head.col < 0 {
		t.Errorf("head clamped to %+v, want it inside the pane", m.sel.head)
	}
	if got := m.selectionText(); !strings.Contains(got, "第一行的文字") {
		t.Errorf("selected %q, want it to reach the first row's text", got)
	}
}

// Clicking on a border, on the status bar or outside any pane starts nothing.
func TestPressOutsideAPaneStartsNoSelection(t *testing.T) {
	m := testModel()
	m.outputText = "some output"
	m.refreshOutput()

	for _, p := range []struct {
		name string
		x, y int
	}{
		{"top border", 5, 0},
		{"left frame edge", 0, 3},
		{"divider between panes", 40, 3},
		{"status bar", 5, 19},
	} {
		got := send(t, m, press(p.x, p.y), moveTo(p.x+5, p.y))
		if got.sel.pane != paneNone {
			t.Errorf("%s: started a selection in pane %v", p.name, got.sel.pane)
		}
		if got.selectionText() != "" {
			t.Errorf("%s: selected %q", p.name, got.selectionText())
		}
	}
}

// A click with no drag clears any previous selection and copies nothing.
func TestClickWithoutDraggingClearsTheSelection(t *testing.T) {
	m := testModel()
	m.outputText = "你好世界"
	m.refreshOutput()
	m = send(t, m, press(outputX0, firstRow), moveTo(outputX0+3, firstRow), release(outputX0+3, firstRow))
	if m.selectionText() == "" {
		t.Fatal("setup: nothing selected")
	}

	m.status = "" // the setup copy left one behind; a plain click must not set a new one

	got, cmd := m.Update(press(outputX0+2, firstRow))
	m = got.(model)
	if m.selectionText() != "" {
		t.Errorf("selection %q survived a plain click", m.selectionText())
	}

	got, cmd = m.Update(release(outputX0+2, firstRow))
	if cmd != nil {
		t.Error("a click that selected nothing still issued a command")
	}
	if got.(model).status == "✓ Copied!" {
		t.Error("a click that selected nothing reported a copy")
	}
}

// Only the left button selects; the right button must not start a drag.
func TestRightButtonDoesNotSelect(t *testing.T) {
	m := testModel()
	m.outputText = "你好世界"
	m.refreshOutput()

	m = send(t, m,
		tea.MouseClickMsg{X: outputX0, Y: firstRow, Button: tea.MouseRight},
		moveTo(outputX0+4, firstRow),
	)
	if m.sel.pane != paneNone {
		t.Errorf("right button started a selection in pane %v", m.sel.pane)
	}
}

// A release with no drag in progress is a no-op, not a copy of a stale range.
func TestReleaseWithoutADragIsANoOp(t *testing.T) {
	m := testModel()
	m.outputText = "你好世界"
	m.refreshOutput()

	got, cmd := m.Update(release(outputX0+4, firstRow))
	if cmd != nil {
		t.Error("a stray release issued a command")
	}
	if got.(model).status != "" {
		t.Errorf("a stray release set the status to %q", got.(model).status)
	}
}

// The selection lives in screen coordinates, so anything that repaints those
// cells has to drop it rather than leave a highlight over unrelated text.
func TestSelectionIsClearedByAnythingThatRepaints(t *testing.T) {
	selected := func(t *testing.T) model {
		t.Helper()
		m := testModel()
		m.outputText = strings.Repeat("你好世界，這是一段中文。", 30)
		m.refreshOutput()
		m = send(t, m, press(outputX0, firstRow), moveTo(outputX0+6, firstRow), release(outputX0+6, firstRow))
		if m.selectionText() == "" {
			t.Fatal("setup: nothing selected")
		}
		return m
	}

	for _, tc := range []struct {
		name string
		msg  tea.Msg
	}{
		{"a keystroke", tea.KeyPressMsg{Code: 'a', Text: "a"}},
		{"scrolling with the keyboard", shift(tea.KeyUp)},
		{"scrolling with the wheel", tea.MouseWheelMsg{Button: tea.MouseWheelUp}},
		{"a resize", tea.WindowSizeMsg{Width: 100, Height: 30}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := send(t, selected(t), tc.msg)
			if m.sel.pane != paneNone || m.selectionText() != "" {
				t.Errorf("selection %q survived %s", m.selectionText(), tc.name)
			}
		})
	}

	t.Run("an incoming translation token", func(t *testing.T) {
		m := selected(t)
		m.gen, m.translating = 1, true
		m = send(t, m, streamMsg{gen: 1, chunk: "更多"})
		if m.sel.pane != paneNone || m.selectionText() != "" {
			t.Errorf("selection %q survived a streamed token", m.selectionText())
		}
	})
}

// The frame is drawn by hand: a highlighted row must still be exactly as wide
// as every other row, or the whole box drifts.
func TestSelectionDoesNotDisturbTheFrame(t *testing.T) {
	m := testModel()
	m.input.SetValue("輸入區的中文字")
	m.outputText = strings.Repeat("輸出區的中文字。", 10)
	m.refreshOutput()

	for _, tc := range []struct {
		name           string
		x0, y0, x1, y1 int
	}{
		{"output pane", outputX0, firstRow, outputX0 + 9, firstRow + 2},
		{"input pane", inputX0, firstRow, inputX0 + 5, firstRow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sel := send(t, m, press(tc.x0, tc.y0), moveTo(tc.x1, tc.y1))
			for i, line := range strings.Split(sel.View().Content, "\n") {
				if w := lipgloss.Width(line); w != sel.width {
					t.Errorf("line %d is %d cells wide, want %d", i, w, sel.width)
				}
			}
			if !strings.Contains(sel.View().Content, "\x1b[7m") {
				t.Error("no highlight painted for the selection")
			}
		})
	}
}

// Selecting in one pane must not highlight the other.
func TestOnlyTheSelectedPaneIsHighlighted(t *testing.T) {
	m := testModel()
	m.input.SetValue("輸入區的中文字")
	m.outputText = "輸出區的中文字"
	m.refreshOutput()

	sel := send(t, m, press(outputX0, firstRow), moveTo(outputX0+6, firstRow))
	if strings.Contains(sel.paneBody(paneInput), "\x1b[7m") {
		t.Error("the input pane was highlighted by an output-pane selection")
	}
	if !strings.Contains(sel.paneBody(paneOutput), "\x1b[7m") {
		t.Error("the output pane was not highlighted")
	}
}

// Below the minimum size there is no frame to select in.
func TestNoSelectionOnATooSmallTerminal(t *testing.T) {
	m := testModel()
	got, _ := m.Update(tea.WindowSizeMsg{Width: 10, Height: 4})
	small := send(t, got.(model), press(2, 1), moveTo(6, 1), release(6, 1))

	if small.sel.pane != paneNone || small.selectionText() != "" {
		t.Errorf("selected %q on a too-small terminal", small.selectionText())
	}
}
