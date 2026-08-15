package ui

import (
	"encoding/base64"

	tea "charm.land/bubbletea/v2"
)

// The TS version writes the OSC 52 escape sequence straight to process.stdout
// (src/services/clipboard.ts). Under Bubble Tea that would race the renderer,
// which owns stdout and repaints on its own schedule — the sequence can land
// mid-frame and be dropped or corrupt the screen.
//
// Bubble Tea v2 exposes tea.SetClipboard as a Cmd, so the write is sequenced
// with the frame. This is the concrete answer to the Phase 3 risk: OSC 52 still
// works, but it must go through the framework rather than around it. It also
// removes the need for a second native-clipboard dependency (clipboardy), since
// OSC 52 covers local terminals, SSH and tmux alike.
func copyToClipboard(text string) tea.Cmd {
	return tea.SetClipboard(text)
}

// osc52Sequence is the raw sequence tea.SetClipboard emits, kept separate so the
// encoding can be asserted in a test without driving a real terminal.
func osc52Sequence(text string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x07"
}
