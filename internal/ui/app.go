package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"zhen/internal/config"
	"zhen/internal/ollama"
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
	cfg    config.Config
	client *ollama.Client

	input  textarea.Model
	output viewport.Model

	direction   config.Direction
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

// New builds the root Bubble Tea model for the translator UI.
func New(cfg config.Config) tea.Model {
	return newModel(cfg)
}

func newModel(cfg config.Config) model {
	ta := textarea.New()
	ta.Placeholder = config.ZH2EN.Placeholder()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.Focus()

	vp := viewport.New()

	return model{
		cfg:       cfg,
		client:    ollama.NewClient(cfg),
		input:     ta,
		output:    vp,
		direction: config.ZH2EN,
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

		// The textarea has focus and owns up/down and pgup/pgdown for its own
		// cursor, so the output viewport gets the shifted variants. Without these
		// the viewport never receives an Update at all and everything scrolled
		// past the top of the pane is unreachable.
		case "shift+up":
			m.output.ScrollUp(1)
			return m, nil
		case "shift+down":
			m.output.ScrollDown(1)
			return m, nil
		case "shift+pgup":
			m.output.PageUp()
			return m, nil
		case "shift+pgdown":
			m.output.PageDown()
			return m, nil
		}

	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.output.ScrollUp(mouseWheelLines)
		case tea.MouseWheelDown:
			m.output.ScrollDown(mouseWheelLines)
		}
		return m, nil

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

// minWidth/minHeight are the smallest terminal that can hold two bordered panes
// with a usable column of text inside each, plus the status bar.
const (
	minWidth  = 24
	minHeight = 5
)

// mouseWheelLines is how far one wheel notch scrolls the output pane.
const mouseWheelLines = 3

func (m model) tooSmall() bool {
	return m.width < minWidth || m.height < minHeight
}

// layout resizes the two widgets to the current terminal. It clamps to at least
// one cell: a resize below the minimum still reaches the widgets, and both panic
// or misbehave on a negative dimension.
func (m *model) layout() {
	paneW := m.width / 2
	contentH := m.height - 1

	m.input.SetWidth(max(1, paneW-2))
	m.input.SetHeight(max(1, contentH-2))

	m.output.SetWidth(max(1, m.width-paneW-2))
	m.output.SetHeight(max(1, contentH-2))
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
	// Sticky scroll, but only while the user is already at the bottom. An
	// unconditional GotoBottom would yank the view back down on every streamed
	// token, making it impossible to read earlier output while a translation is
	// still arriving. Measured before SetContent, i.e. against the old content.
	stick := m.output.AtBottom()

	wrapped := WrapCells(text, m.output.Width())
	m.output.SetContent(style.Render(strings.Join(wrapped, "\n")))
	if stick {
		m.output.GotoBottom()
	}
}

func (m model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	// Mouse reporting is a property of the view in Bubble Tea v2, not a program
	// option. It is on so the wheel scrolls the output pane; the cost is the
	// terminal's native drag-to-select, which then needs Shift held down.
	v.MouseMode = tea.MouseModeCellMotion
	if m.width == 0 {
		return v
	}

	// Below the minimum the frame cannot be drawn at all — Pane would return an
	// empty string and the user would face a blank screen with no explanation.
	if m.tooSmall() {
		v.SetContent(m.tooSmallView())
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

// tooSmallView wraps the warning across as many rows as the terminal has rather
// than truncating it to one line — at 10 columns a single clipped line reads
// "Terminal t", which tells the user nothing.
func (m model) tooSmallView() string {
	msg := fmt.Sprintf("Terminal too small (need %d x %d)", minWidth, minHeight)

	lines := WrapCells(msg, m.width)
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	for i, line := range lines {
		lines[i] = fitCells(warnStyle.Render(line), m.width)
	}
	return strings.Join(lines, "\n")
}

func (m model) statusBar() string {
	hints := fmt.Sprintf(" ^T: 翻譯  ^L: 切換方向  ^Y: 複製  ⇧↑↓: 捲動  ^Q: 退出  [%s]", m.direction.Label())
	bar := hintStyle.Render(hints)
	switch {
	case m.translating:
		bar += warnStyle.Render(" [翻譯中...]")
	case m.status != "":
		bar += m.statusStyle.Render(" " + m.status)
	}
	return fitCells(bar, m.width)
}
