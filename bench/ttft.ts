import { translateStream } from "../src/services/ollama.js"
import { warmupOllama } from "../src/services/ollama.js"

const SAMPLE = "今天天氣很好，我打算去公園散步，順便買一杯咖啡。"
warmupOllama()
await new Promise((r) => setTimeout(r, 3000))

for (let i = 0; i < 3; i++) {
  const t0 = performance.now()
  let ttft = -1
  let chars = 0
  for await (const c of translateStream(SAMPLE, "zh2en")) {
    if (ttft < 0) ttft = performance.now() - t0
    chars += c.length
  }
  console.log(`run${i + 1} ttft=${ttft.toFixed(0)}ms total=${(performance.now() - t0).toFixed(0)}ms chars=${chars}`)
}
