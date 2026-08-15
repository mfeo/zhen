# Baseline — Bun + OpenTUI (current implementation)

Measured on: Linux 6.18 (WSL2), x64, Bun 1.2.20, Ollama `gemma4:e2b`.

All numbers are **warm** unless stated. Reproduce with the scripts in this directory.

## Startup & memory

`bun run bench/startup.ts` — loads the full TUI dependency graph (`@opentui/core`,
`@opentui/react`, React 19, and `src/app.tsx`) and exits.

| Run | Module-graph loaded | RSS | Wall clock |
|---|---|---|---|
| 1 (first, cache cold) | 282 ms | 99.2 MB | 0.29 s |
| 2 | 234 ms | 92.3 MB | 0.24 s |
| 3 | 227 ms | 91.9 MB | 0.23 s |
| 4 | 235 ms | 91.5 MB | 0.24 s |
| 5 | 250 ms | 92.3 MB | 0.26 s |
| 6 | 238 ms | 91.3 MB | 0.24 s |

**Median: ~236 ms to loaded, ~92 MB RSS.**

Note this excludes the real app's blocking `checkOllamaHealth()` call in
`src/index.tsx`, which adds one HTTP round-trip (up to a 3 s timeout) before the
first frame is drawn.

## Distribution size

`bun build --compile` → **109 MB** single-file executable (bundles the Bun runtime
plus OpenTUI's native library).

## Translation latency

`bun run bench/ttft.ts` — fixed zh→en sample, `temperature: 0`, `think: false`,
`keep_alive: 24h`, streamed via `/api/generate`.

Sample: `今天天氣很好，我打算去公園散步，順便買一杯咖啡。` (92 output chars)

| Run | TTFT | Total |
|---|---|---|
| 1 (model cold-loading into VRAM) | 75 291 ms | 82 144 ms |
| 2 | 667 ms | 905 ms |
| 3 | 670 ms | 909 ms |

**Warm median: TTFT ~670 ms, total ~907 ms.**

### Why this dominates the comparison

The model load (75 s) and the warm inference (~900 ms) are both **server-side**,
inside Ollama. Nothing a client rewrite can change. The client's own share of the
latency budget is the ~236 ms startup plus NDJSON parsing overhead, which is
negligible against a 670 ms TTFT.

So a port should be judged on **startup time, memory, distribution size and
maintainability** — not on translation speed, where every implementation is
pinned to the same Ollama numbers.

> TTFT = Time To First Token. RSS = Resident Set Size, the physical memory a
> process holds. NDJSON = Newline-Delimited JSON, one JSON object per line, the
> format Ollama streams in.
