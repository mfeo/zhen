// Measures time to have the full TUI dependency graph loaded and a renderer ready.
import { createCliRenderer } from "@opentui/core"
import { createRoot } from "@opentui/react"
import { App } from "../src/app.js"
void createCliRenderer
void createRoot
void App
console.log(`loaded_ms=${performance.now().toFixed(1)} rss_mb=${(process.memoryUsage().rss / 1048576).toFixed(1)}`)
