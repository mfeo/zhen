#!/usr/bin/env bun
import { createCliRenderer } from "@opentui/core"
import { createRoot } from "@opentui/react"
import { checkOllamaHealth } from "./services/ollama.js"
import { CONFIG } from "./config.js"
import { App } from "./app.js"

const healthy = await checkOllamaHealth()
if (!healthy) {
  console.error(`Cannot connect to Ollama at ${CONFIG.ollamaBaseUrl}.`)
  console.error("Please start it with: ollama serve")
  console.error(`And ensure the model is pulled: ollama pull ${CONFIG.model}`)
  process.exit(1)
}

const renderer = await createCliRenderer({
  exitOnCtrlC: false,
  targetFps: 30,
  useMouse: false,
})

createRoot(renderer).render(<App />)
