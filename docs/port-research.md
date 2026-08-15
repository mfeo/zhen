# Porting zhen to Rust or Go — research findings

**Status:** **Decided and shipped.** The Go port replaced the TypeScript
implementation in August 2026 and is now the only implementation; the paths this
document refers to as `spikes/go-tui/` are now `main.go` and `internal/`. The
TypeScript sources remain in git history at commit `55f653b` and earlier.

Rust was investigated on paper only — no Rust toolchain was installed on this
machine and installing one was declined, so every Rust claim below is unverified
and marked as such.

**Recommendation was: port to Go with Bubble Tea v2.** Reasoning in
[Verdict](#verdict).

---

## 0. What zhen was before the port

A terminal UI (TUI) for bidirectional Chinese ↔ English translation against a
local Ollama server. 383 lines of TypeScript across 8 files, running on Bun,
rendered with **OpenTUI** — a terminal renderer driven by a React reconciler, so
the UI is written as React components with Flexbox layout.

| Piece | Implementation |
|---|---|
| Layout | Two side-by-side bordered panes + a 1-row status bar |
| Widgets | `box` (rounded border, centred title), `textarea`, `scrollbox` (auto-scroll), `text` |
| Backend | Ollama `/api/generate`, streamed as NDJSON; `/api/tags` health check |
| Cancellation | `AbortController` |
| Clipboard | OSC 52 escape sequence written to stdout, plus `clipboardy` |
| Tests | none |

> **NDJSON** (Newline-Delimited JSON) — a stream of JSON objects, one per line.
> **OSC 52** — a terminal escape sequence that sets the system clipboard; works
> through SSH and tmux, where a native clipboard API cannot reach.
> **TTFT** (Time To First Token) — request sent → first character received.

---

## 1. Baseline (measured)

Full method and raw numbers in [`bench/baseline.md`](../bench/baseline.md).

| | Bun + OpenTUI |
|---|---|
| Startup (module graph loaded) | ~236 ms |
| Resident memory | ~92 MB |
| Single-file executable | 109 MB |
| TTFT, warm | ~670 ms |
| Translation total, warm | ~907 ms |
| First run after boot (model loads into VRAM) | ~75 s |

### The finding that frames everything else

**Latency is not a client problem.** The 670 ms TTFT and the 75 s cold model load
happen inside Ollama. No client rewrite touches them. The client's entire share
of the budget is ~236 ms of startup plus NDJSON parsing, which is noise next to
670 ms.

So a port cannot be justified on translation speed. It has to be justified on
startup, memory, distribution, and maintainability — which is where the measured
gaps turn out to be large.

---

## 2. Go — investigated by building it

The spike was a working port: same layout, same four keybindings, same streaming
behaviour, same environment variables. ~700 lines including 47 tests. It has
since been promoted to the project proper — see the README for the current
layout.

```
make test    # go test ./...
make check   # vet + race
```

### Stack

| Concern | Package | Notes |
|---|---|---|
| TUI runtime | `charm.land/bubbletea/v2` | Elm architecture: Model / Update / View |
| Widgets | `charm.land/bubbles/v2` | `textarea`, `viewport` both built in |
| Styling | `charm.land/lipgloss/v2` | ANSI-aware width and styling |
| Cell widths | `github.com/mattn/go-runewidth`, `github.com/charmbracelet/x/ansi` | |
| TUI testing | `github.com/charmbracelet/x/exp/teatest/v2` | drives the program, asserts on real terminal output |

**Version trap, worth knowing before starting.** Bubble Tea v2 has moved its
module path from `github.com/charmbracelet/…` to `charm.land/…`. `go get
github.com/charmbracelet/bubbletea/v2` fails outright with a module-path
mismatch. v1 is still the default result of a naive `go get` and does **not**
have `tea.SetClipboard`, which the OSC 52 support depends on (see below). Target
v2 and the `charm.land` paths from the start. `teatest` is the exception — it
still lives at `github.com/charmbracelet/x/exp/teatest/v2`.

### What mapped cleanly

**The architecture.** React's unidirectional data flow maps almost one-to-one
onto Elm's Model/Update/View. Every `useState` became a struct field; every
handler became a `case` in `Update`.

**Streaming.** Go's standard library is a better fit than `fetch` here.
`json.Decoder` consumes one JSON value per call and buffers partial reads itself,
so the TS version's manual line buffer, `split("\n")`, leftover-tail handling and
`SyntaxError`-swallowing `catch` all collapse into:

```go
dec := json.NewDecoder(r)
for {
    var chunk generateChunk
    if err := dec.Decode(&chunk); err != nil { ... }
    ...
}
```

A test that splits the stream at **every possible byte offset** confirms it
(`TestDecodeStreamSplitAcrossReads`).

**Cancellation.** `context.WithCancel` threaded through `http.NewRequestWithContext`
actually closes the TCP connection, so Ollama stops generating.
`TestTranslateCancelStopsServer` asserts the server observes the disconnect —
not merely that the client stopped listening. This matters: it frees the GPU.

**Render coalescing became unnecessary.** The TS version buffers tokens and
flushes on a 33 ms timer to avoid over-rendering (`src/app.tsx`). Bubble Tea's
renderer already repaints on its own schedule and only redraws changed lines, so
the spike appends each token directly and the timer disappears.

**Stale-response handling got simpler.** `AbortController` + a `useRef` became a
single integer generation counter compared inside `Update`. Two tests cover it.

### What cost real work

**No built-in pane titles.** OpenTUI gives every `box` a `title` prop with
`titleAlignment`. Lipgloss has no equivalent, so `pane.go` draws the rounded
border and centres the title by hand — ~80 lines including a CJK-safe wrapper.
This is the single largest piece of code the port has to add.

**No Flexbox.** The layout here is simple enough (`width/2`) that
`lipgloss.JoinHorizontal` suffices. A more complex layout would need `taffy`-like
help that lipgloss does not provide.

**Three real bugs the tests caught**, all width-related, all invisible to an
English-only test suite:

1. `runewidth.Truncate` drops a full-width rune whole when the cut falls inside
   it, leaving the line one cell short — the frame drifts a column.
2. `runewidth.Truncate` counts the **bytes of an ANSI escape sequence** as
   visible characters. Truncating a styled status bar cut through
   `\x1b[38;2;…`, emitting a half-written colour code and silently swallowing
   the text after it. This is why an error message never appeared on screen. The
   fix is `ansi.Truncate`, which measures printable cells.
3. A stream ending with no output left the pane stuck on "Translating…" forever.

None of these are Go's fault — they are the cost of hand-drawing a frame, and
they are exactly what a spike is for. All three now have regression tests.

### Testing — the clearest win

47 tests at the end of the spike (55 after the resize and small-terminal work
that followed), all passing under `-race`:

- **NDJSON parser**: happy path, split at every byte offset, in-band error,
  early `done`, malformed JSON, truncated stream, empty body
- **HTTP layer** against `httptest`: request shape, per-direction prompts, non-200
  handling, cancellation propagation, health check
- **Cell widths**: every frame line is exactly N cells for ASCII, CJK and mixed
  content; wrapping never exceeds width; wrapping loses no characters
- **The TUI itself** via `teatest`: initial frame, Ctrl+L toggle, streaming into
  the output pane, empty and whitespace-only input, error display, quit
- **Model logic** with no terminal: stale events, cancellation vs real errors,
  status-message expiry

The TypeScript version had **zero** tests, and OpenTUI has no published
equivalent of `teatest`. This is the largest single difference between the two
stacks.

**Two `teatest` sharp edges**, both of which cost debugging time and are
documented in `tui_test.go`:

- `tm.Output()` is a `bytes.Buffer`, so reading *drains* it. Bubble Tea repaints
  only changed lines, so a second `teatest.WaitFor` silently loses every frame
  the first one consumed. Record output continuously into your own buffer.
- That buffer returns `io.EOF` whenever it is momentarily empty, so a plain
  `io.Copy` returns instantly with nothing. The drain loop must treat EOF as
  "nothing yet".

### Clipboard

The TS version writes the OSC 52 sequence straight to `process.stdout`. Under
Bubble Tea that races the renderer, which owns stdout — the sequence can land
mid-frame and be dropped. Bubble Tea v2 exposes `tea.SetClipboard` as a command,
so the write is sequenced with the frame. This also removes the `clipboardy`
dependency: OSC 52 alone covers local terminals, SSH and tmux.

### Measured results

| | Bun + OpenTUI | Go + Bubble Tea | |
|---|---|---|---|
| CPU per startup (20 runs) | 319 ms | **77 ms** | 4× less |
| Resident memory | 92 MB | **11.6 MB** | 8× less |
| Executable | 109 MB | **8.0 MB** stripped (11.4 MB unstripped) | 13× smaller |
| Cross-compile | Bun target per platform | `GOOS`/`GOARCH`, no toolchain per target | |

**Wall-clock startup could not be measured reliably on this machine.** Repeated
identical runs varied between 0.31 s and 3.28 s under WSL2 while CPU time stayed
flat at ~70 ms, so process wall-clock here is dominated by scheduling noise. CPU
time (`user + sys`) is the honest metric and is reported above. Anyone
reproducing this on native Linux or macOS should re-measure wall clock.

---

## 3. Rust — paper research only, unverified

No Rust toolchain on this machine; nothing below was compiled or run. Treat it
as a survey to be checked, not as findings.

### Expected stack

| Concern | Crate | Concern for this port |
|---|---|---|
| TUI runtime | `ratatui` | Immediate-mode: the whole frame is redrawn each tick, so there is no retained widget tree. Further from React than Elm is. |
| Multi-line input | `tui-textarea` | Not part of ratatui; a separate crate |
| Layout | ratatui `Layout` constraints | Constraint-based, not Flexbox. `taffy` would be needed for real Flexbox. |
| HTTP + streaming | `reqwest` (stream feature) | Mature |
| NDJSON | `tokio_util::codec::LinesCodec` + `serde_json` | Straightforward |
| Cancellation | drop the future, or `CancellationToken` | Straightforward |
| Cell widths | `unicode-width` | Same class of problem as Go; the same three bugs would need the same care |
| Clipboard | `arboard` | On Linux/X11 the clipboard is owned by the process — content may not survive exit. Needs verification on WSL2. |
| TUI testing | ratatui `TestBackend` | Asserts on a render buffer. Narrower than `teatest`, which drives the real event loop. |

### Expected trade-off

Rust should beat Go on binary size and memory, and both are already far ahead of
Bun. But the ~670 ms TTFT is server-side, so those wins do not reach the user as
speed. Against that, Rust has to supply a textarea from a separate crate, has no
Flexbox, and has a narrower TUI-testing story — while the pane-title and
cell-width work is the same either way.

**Rust looks like more implementation cost for gains this particular application
cannot spend.** This is a paper judgement; a spike would be needed to confirm it,
and the honest way to get one is to install the toolchain and build the same
frame the Go spike builds.

---

## 4. Verdict

**Port to Go with Bubble Tea v2.**

| Criterion | Weight | Go (measured) | Rust (unverified) |
|---|---|---|---|
| Widget coverage (textarea, scroll) | High | Both built into `bubbles` | textarea is a third-party crate |
| CJK width correctness | High | Achieved; 3 bugs found and fixed, regression-tested | Same work required |
| Streaming + cancellation | Medium | Simpler than the TS original | Comparable |
| Testability | Medium | `teatest` drives the real program; 47 tests | `TestBackend` only asserts buffers |
| Startup / memory / size | Medium | 4× / 8× / 13× better than Bun | Likely better still |
| Architecture fit with existing React code | High | Near one-to-one | Immediate-mode, further away |

The decisive factors are testability and widget coverage, not performance. Both
Go and Rust bury the performance question relative to Bun, and the user-visible
latency belongs to Ollama regardless.

### What was still owed after the spike, and where it stands

Closed during the promotion to the project proper:

- **Terminal resize** — `layout()` now clamps every widget dimension to at least
  one cell, and tests assert both widgets are re-laid-out and that existing
  output is re-wrapped rather than dropped.
- **Terminals too small to draw the frame** — previously rendered blank, because
  `Pane` returns an empty string below its minimum. Now shows a wrapped warning
  sized to whatever space exists, down to a 1x1 terminal.

Still open, and none of it is testable without a human at a real terminal:

1. **CJK input method (IME) behaviour is untested.** `teatest` sends synthetic
   key events; it cannot exercise a real IME. This is the largest remaining
   unknown for a Chinese-input tool.
2. **OSC 52 through tmux and SSH** — `tea.SetClipboard` is the right mechanism,
   but the end-to-end path is unverified.
3. **Input pane scrolling** for text longer than the pane; the textarea supports
   it, the composed frame has not been checked interactively.
4. **`arboard`-style native clipboard fallback** if OSC 52 proves insufficient on
   any target terminal.

### If Rust is wanted anyway

Install `rustup`, then build the same thing the Go spike builds: the two-pane
frame with a centred title and CJK-correct widths, plus the streaming client.
That single spike answers the open questions — it is roughly the `pane.go` +
`ollama.go` pair, ~250 lines. The three width bugs documented above are the
specific things to test for.
