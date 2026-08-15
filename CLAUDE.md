# CLAUDE.md

zhen is a Go terminal application. It was previously TypeScript on Bun; that
implementation was removed in August 2026 and lives only in git history. Do not
add Bun, npm or TypeScript tooling back.

## Build and test

Use the Makefile rather than raw `go` commands, so version stamping and flags
stay consistent.

```bash
make build    # go build with -ldflags version stamping
make test     # go test ./...
make race     # go test -race -count=1 ./...
make check    # vet + race; run this before committing
make fmt      # gofmt -w .
```

Every `.go` file must pass `gofmt`. Run `gofmt -l .` (empty output means clean).

## Architecture

| Path | Contents |
|---|---|
| `main.go` | entrypoint; flags, Ollama health check, program start. Logic lives in `run()` so it is testable without `os.Exit`. |
| `internal/config` | `Direction` (zh2en / en2zh) with its prompts and pane titles; environment configuration |
| `internal/ollama` | streaming client for Ollama's `/api/generate` NDJSON stream |
| `internal/ui` | Bubble Tea model, hand-drawn panes, clipboard |

**Bubble Tea v2** — the module path is `charm.land/bubbletea/v2`, not
`github.com/charmbracelet/…`. The same applies to `bubbles` and `lipgloss`.
`teatest` is the exception and still lives at
`github.com/charmbracelet/x/exp/teatest/v2`.

## Things that will bite you

**Widths are terminal cells, never runes or bytes.** A CJK ideograph occupies two
cells. `internal/ui/pane.go` draws the frame by hand because lipgloss has no
bordered-box-with-title primitive, and every line must come out exactly N cells
wide or the whole frame drifts. Three separate bugs in this area are documented
in `docs/port-research.md`; the regression tests are in `pane_test.go`.

**Truncating styled text needs `ansi.Truncate`, not `runewidth.Truncate`.** The
latter counts the bytes of an escape sequence as visible characters and will cut
through `\x1b[38;2;…`, silently swallowing the text after it.

**`teatest`'s `tm.Output()` is a `bytes.Buffer` that drains as you read**, and
returns `io.EOF` whenever it is momentarily empty. Bubble Tea repaints only
changed lines, so a second `teatest.WaitFor` loses everything the first
consumed. `internal/ui/app_test.go` records output continuously into its own
buffer; follow that pattern for new UI tests.

**Stream generations.** `model.gen` invalidates in-flight translations. Any new
path that starts or cancels a request must bump it, or a stale response will
interleave into the output pane.

## Testing requirements

- Every behavior change, including bug fixes, must include a test that fails
  without the change.
- Cover the happy path, boundary values, invalid input, error paths, and the
  absence of unintended state changes.
- Test observable behavior, not implementation details.
- Tests must be deterministic and order-independent: no wall-clock dependence,
  no shared mutable state, no real network. Use `httptest` for Ollama.
- Do not delete tests or weaken assertions to make code pass. If a change makes
  an assertion genuinely obsolete, stop and report what it asserted and why the
  new behavior is correct.
- Run `make check` before completing, and report the command and its output.

## Git workflow

Commit immediately after each verified change. Messages in English, following
`<type>(<scope>): <subject>` — `feat`, `fix`, `docs`, `style`, `refactor`,
`perf`, `test`, `build`, `ci`, `chore`.

Do not include "Generated with Claude" or "Co-Authored-By: Claude" trailers.

## Language conventions

- CLI responses to the user: **Traditional Chinese**
- Code, comments, documentation and commit messages: **English**
- New files use LF line endings
