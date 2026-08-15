package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ---------- messages ----------

// streamEvent is what the translating goroutine pushes onto its channel. One
// channel carries tokens, terminal errors and completion, so the UI only has to
// wait on a single source.
type streamEvent struct {
	gen   int
	chunk string
	err   error
	done  bool
}

type streamMsg streamEvent
type clearStatusMsg struct{ token int }

// ---------- model ----------

type model struct {
	cfg    Config
	client *Client

	input  textarea.Model
	output viewport.Model

	direction   Direction
	outputText  string
	translating bool
	status      string
	statusStyle lipgloss.Style

	width, height int

	// gen invalidates in-flight streams: a toggle or a re-translate bumps it, so
	// late events from the previous request are dropped instead of interleaving
	// into the new output. This replaces the TS version's AbortController-plus-
	// ref juggling with a single integer compared inside Update.
	gen         int
	statusToken int
	cancel      context.CancelFunc
	events      chan streamEvent
	started     time.Time
	sawFirst    bool
}

func initialModel(cfg Config) model {
	ta := textarea.New()
	ta.Placeholder = ZH2EN.Placeholder()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.Focus()

	vp := viewport.New()

	return model{
		cfg:       cfg,
		client:    NewClient(cfg),
		input:     ta,
		output:    vp,
		direction: ZH2EN,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

// ---------- update ----------

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		m.refreshOutput()
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+q", "esc":
			m.cancelStream()
			return m, tea.Quit
		case "ctrl+t":
			return m.startTranslate()
		case "ctrl+l":
			return m.toggleDirection()
		case "ctrl+y":
			if m.outputText == "" {
				return m.withStatus("Nothing to copy", warnStyle)
			}
			m2, statusCmd := m.withStatus("✓ Copied!", okStyle)
			return m2, tea.Batch(copyToClipboard(m.outputText), statusCmd)
		}

	case streamMsg:
		return m.applyStream(streamEvent(msg))

	case clearStatusMsg:
		if msg.token == m.statusToken {
			m.status = ""
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) startTranslate() (tea.Model, tea.Cmd) {
	src := strings.TrimSpace(m.input.Value())
	if src == "" {
		return m, nil
	}
	m.cancelStream()

	m.gen++
	gen := m.gen
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.translating = true
	m.outputText = ""
	m.sawFirst = false
	m.started = time.Now()
	m.refreshOutput()

	// Buffered so a burst of tokens never blocks the HTTP read loop while the
	// UI is mid-repaint.
	events := make(chan streamEvent, 64)
	m.events = events
	dir := m.direction
	client := m.client

	go func() {
		err := client.Translate(ctx, src, dir, func(c string) {
			select {
			case events <- streamEvent{gen: gen, chunk: c}:
			case <-ctx.Done():
			}
		})
		select {
		case events <- streamEvent{gen: gen, err: err, done: true}:
		case <-ctx.Done():
		}
	}()

	return m, waitForEvent(events)
}

// waitForEvent blocks in Bubble Tea's Cmd goroutine until the next token. Each
// received event re-issues this Cmd, so the read loop is driven by the runtime.
func waitForEvent(ch chan streamEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return streamMsg{done: true}
		}
		return streamMsg(ev)
	}
}

func (m model) applyStream(ev streamEvent) (tea.Model, tea.Cmd) {
	if ev.gen != m.gen {
		return m, nil // stale event from a cancelled or superseded request
	}

	if ev.done {
		m.translating = false
		// The pane shows "Translating..." only while translating and empty. A
		// stream that ends without producing anything (an error, or a model that
		// returned nothing) must repaint, or the placeholder is stuck forever.
		m.refreshOutput()
		if ev.err != nil && !isCancelled(ev.err) {
			return m.withStatus("Error: "+ev.err.Error(), errStyle)
		}
		return m, nil
	}

	var cmds []tea.Cmd
	if !m.sawFirst {
		m.sawFirst = true
		ttft := time.Since(m.started).Milliseconds()
		mi, cmd := m.withStatus(fmt.Sprintf("TTFT %dms", ttft), warnStyle)
		m = mi.(model)
		cmds = append(cmds, cmd)
	}

	m.outputText += ev.chunk
	m.refreshOutput()
	cmds = append(cmds, waitForEvent(m.events))
	return m, tea.Batch(cmds...)
}

func isCancelled(err error) bool {
	return err == context.Canceled || strings.Contains(err.Error(), "context canceled")
}

func (m model) toggleDirection() (tea.Model, tea.Cmd) {
	m.cancelStream()
	m.gen++
	m.direction = m.direction.Toggle()
	m.translating = false
	m.outputText = ""
	m.status = ""
	m.input.Reset()
	m.input.Placeholder = m.direction.Placeholder()
	m.refreshOutput()
	return m, nil
}

func (m *model) cancelStream() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

func (m model) withStatus(s string, style lipgloss.Style) (tea.Model, tea.Cmd) {
	m.status = s
	m.statusStyle = style
	m.statusToken++
	token := m.statusToken
	return m, tea.Tick(2500*time.Millisecond, func(time.Time) tea.Msg {
		return clearStatusMsg{token: token}
	})
}

// ---------- view ----------

var (
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#44ff88"))
	warnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaa44"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))
	hintStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	plainStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff"))
)

func (m *model) layout() {
	paneW := m.width / 2
	contentH := m.height - 1

	m.input.SetWidth(paneW - 2)
	m.input.SetHeight(contentH - 2)

	m.output.SetWidth(m.width - paneW - 2)
	m.output.SetHeight(contentH - 2)
}

func (m *model) refreshOutput() {
	text, style := m.outputText, plainStyle
	if text == "" {
		if m.translating {
			text = "Translating..."
		} else {
			text = "Translation will appear here"
			style = dimStyle
		}
	}
	wrapped := WrapCells(text, m.output.Width())
	m.output.SetContent(style.Render(strings.Join(wrapped, "\n")))
	m.output.GotoBottom() // equivalent of OpenTUI's stickyScroll
}

func (m model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	if m.width == 0 {
		return v
	}

	paneW := m.width / 2
	contentH := m.height - 1

	left := Pane(paneW, contentH, m.direction.InputTitle(), m.input.View())
	right := Pane(m.width-paneW, contentH, m.direction.OutputTitle(), m.output.View())

	v.SetContent(lipgloss.JoinHorizontal(lipgloss.Top, left, right) + "\n" + m.statusBar())

	// Offset the textarea's own cursor by the left pane's border so it lands on
	// the right cell of the composed frame.
	if c := m.input.Cursor(); c != nil {
		c.Position.X += 1
		c.Position.Y += 1
		v.Cursor = c
	}
	return v
}

func (m model) statusBar() string {
	hints := fmt.Sprintf(" ^T: 翻譯  ^L: 切換方向  ^Y: 複製  ^Q: 退出  [%s]", m.direction.Label())
	bar := hintStyle.Render(hints)
	switch {
	case m.translating:
		bar += warnStyle.Render(" [翻譯中...]")
	case m.status != "":
		bar += m.statusStyle.Render(" " + m.status)
	}
	return fitCells(bar, m.width)
}

// ---------- entrypoint ----------

func main() {
	cfg := LoadConfig()

	// Benchmark hook, mirroring bench/startup.ts on the Bun side: build the model
	// and render one full frame, then report how long the process took to get
	// there. Kept in-process because Go's `main` package cannot be imported.
	if os.Getenv("ZHEN_BENCH_STARTUP") != "" {
		m := initialModel(cfg)
		m.width, m.height = 80, 24
		m.layout()
		m.refreshOutput()
		_ = m.View()
		fmt.Printf("loaded_ms=%.1f\n", float64(time.Since(processStart).Microseconds())/1000)
		return
	}

	client := NewClient(cfg)

	if !client.Health(context.Background()) {
		fmt.Fprintf(os.Stderr, "Cannot connect to Ollama at %s.\n", cfg.BaseURL)
		fmt.Fprintln(os.Stderr, "Please start it with: ollama serve")
		fmt.Fprintf(os.Stderr, "And ensure the model is pulled: ollama pull %s\n", cfg.Model)
		os.Exit(1)
	}
	client.Warmup()

	if _, err := tea.NewProgram(initialModel(cfg)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// processStart is stamped during package initialisation, as close to process
// start as Go allows, so the benchmark hook measures runtime init too.
var processStart = time.Now()
