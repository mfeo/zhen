# zhen

A terminal UI for bidirectional Chinese ↔ English translation using a local LLM
(Ollama). Split-pane layout: type on the left, watch the translation stream in
on the right.

Written in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea).
Single static binary, no runtime to install.

## Prerequisites

- [Ollama](https://ollama.com) running locally
- A model pulled: `ollama pull qwen2.5:3b`
- Go 1.24+ (only to build)

## Install

```bash
make install        # into $(go env GOPATH)/bin
```

Or build in place:

```bash
make build && ./zhen
```

## Usage

```bash
zhen
```

| Key | Action |
|---|---|
| `Ctrl+T` | Translate |
| `Ctrl+L` | Toggle direction (zh→en / en→zh) |
| `Ctrl+Y` | Copy translation to clipboard |
| `Shift+↑` / `Shift+↓` | Scroll the translation one line |
| `Shift+PgUp` / `Shift+PgDn` | Scroll the translation one page |
| `Ctrl+Q` / `Esc` | Quit |

The clipboard uses OSC 52, so `Ctrl+Y` works over SSH and inside tmux, not just
in a local terminal.

A translation longer than the pane scrolls; the plain arrow and page keys move
the input cursor, so the shifted variants drive the translation pane. The mouse
wheel scrolls it too. The pane follows the stream while you are at the bottom
and stops following as soon as you scroll up, so reading earlier output does not
fight the incoming tokens.

### Selecting text with the mouse

Drag inside either pane to select, and release to copy — no extra keypress. The
selection is confined to the pane you started in, so it never picks up the frame
or whatever the other pane happens to show on the same rows, which is what the
terminal's own drag-select would do with two panes side by side.

What gets copied is what is on screen, so a selection that spans wrapped rows is
copied with those wrap points as line breaks. `Ctrl+Y` remains the way to copy
the whole translation as one unwrapped block.

The selection describes cells on the screen rather than a range in the text, so
it is dropped as soon as those cells are repainted: by typing, scrolling, a
resize, or the next translation token. Select once the translation has arrived.

Because the application reads the mouse, the terminal's own selection needs
`Shift` held down — use it if you want to copy across both panes at once.

## Configuration

| Environment Variable | Default |
|---|---|
| `OLLAMA_URL` | `http://localhost:11434` |
| `OLLAMA_MODEL` | `qwen2.5:3b` |

```bash
OLLAMA_MODEL=qwen2.5:7b zhen
```

The first translation after starting Ollama can take a minute while the model
loads into VRAM. zhen warms the model up on launch and pins it with
`keep_alive: 24h`, so subsequent translations start in well under a second.

## Development

```bash
make test     # go test ./...
make race     # go test -race
make check    # vet + race, what CI runs
```

Layout:

| Path | Contents |
|---|---|
| `main.go` | entrypoint, flags, Ollama health check |
| `internal/config` | direction, prompts, environment configuration |
| `internal/ollama` | streaming client for `/api/generate` |
| `internal/ui` | Bubble Tea model, pane rendering, clipboard |

The UI is covered end-to-end by
[`teatest`](https://github.com/charmbracelet/x/tree/main/exp/teatest), which
drives the real program and asserts on the bytes it writes to the terminal.

## History

zhen began as a TypeScript/Bun application rendered with OpenTUI. It was ported
to Go in August 2026; the reasoning, measurements and trade-offs are recorded in
[`docs/port-research.md`](docs/port-research.md). The TypeScript implementation
remains in git history.
