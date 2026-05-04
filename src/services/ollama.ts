import { CONFIG } from "../config.js"
import type { Direction } from "../config.js"

interface OllamaGenerateChunk {
  model: string
  response: string
  done: boolean
  error?: string
}

export async function checkOllamaHealth(): Promise<boolean> {
  try {
    const res = await fetch(`${CONFIG.ollamaBaseUrl}/api/tags`, { signal: AbortSignal.timeout(3000) })
    return res.ok
  } catch {
    return false
  }
}

export function warmupOllama(): void {
  void fetch(`${CONFIG.ollamaBaseUrl}/api/generate`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ model: CONFIG.model, prompt: "", think: false, keep_alive: "24h" }),
  }).catch(() => {})
}

export async function* translateStream(
  sourceText: string,
  direction: Direction,
  signal?: AbortSignal,
): AsyncGenerator<string, void, unknown> {
  const res = await fetch(`${CONFIG.ollamaBaseUrl}/api/generate`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      model: CONFIG.model,
      system: CONFIG.systemPrompts[direction],
      prompt: sourceText,
      stream: true,
      think: false,
      keep_alive: "24h",
      options: { temperature: 0 },
    }),
    signal,
  })

  if (!res.ok) {
    const body = await res.text()
    throw new Error(`Ollama error ${res.status}: ${body}`)
  }

  if (!res.body) throw new Error("No response body from Ollama")

  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ""

  while (true) {
    const { done, value } = await reader.read()
    if (done) break

    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split("\n")
    buffer = lines.pop() ?? ""

    for (const line of lines) {
      if (!line.trim()) continue
      try {
        const chunk: OllamaGenerateChunk = JSON.parse(line)
        if (chunk.error) throw new Error(`Ollama: ${chunk.error}`)
        if (chunk.response) yield chunk.response
        if (chunk.done) return
      } catch (e) {
        if (e instanceof SyntaxError) continue
        throw e
      }
    }
  }
}
