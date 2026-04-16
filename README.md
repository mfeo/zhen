# cn2en

A terminal UI tool for Chinese-to-English translation using local LLM (Ollama).

Split-pane layout: type Chinese on the left, see English translation streaming on the right.

Built with [OpenTUI](https://github.com/anomalyco/opentui) + Bun.

## Prerequisites

- [Bun](https://bun.sh/)
- [Ollama](https://ollama.ai/) running locally
- A model pulled: `ollama pull gemma4:e2b`

## Usage

```bash
bun install
bun run start
```

## Keybindings

| Key | Action |
|---|---|
| `Ctrl+T` | Translate |
| `Ctrl+Y` | Copy translation to clipboard |
| `Ctrl+Q` / `Esc` | Quit |

## Configuration

| Environment Variable | Default |
|---|---|
| `OLLAMA_URL` | `http://localhost:11434` |
| `OLLAMA_MODEL` | `gemma4:e2b` |

Example:

```bash
OLLAMA_MODEL=qwen2.5:7b bun run start
```
