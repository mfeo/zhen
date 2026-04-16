export const CONFIG = {
  ollamaBaseUrl: process.env["OLLAMA_URL"] ?? "http://localhost:11434",
  model: process.env["OLLAMA_MODEL"] ?? "gemma4:e2b",
  systemPrompt:
    "Translate the following Chinese text to English. Output only the translation, no explanations or extra text.",
}
