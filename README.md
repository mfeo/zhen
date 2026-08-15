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
| `Ctrl+Q` / `Esc` | Quit |

The clipboard uses OSC 52, so `Ctrl+Y` works over SSH and inside tmux, not just
in a local terminal.

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
