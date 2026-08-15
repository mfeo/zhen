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
