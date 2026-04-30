export type Direction = "zh2en" | "en2zh"

export const CONFIG = {
  ollamaBaseUrl: process.env["OLLAMA_URL"] ?? "http://localhost:11434",
  model: process.env["OLLAMA_MODEL"] ?? "gemma4:e2b",
  systemPrompts: {
    zh2en: "Translate the following Chinese text to English. Output only the translation, no explanations or extra text.",
    en2zh: "Translate the following English text to Traditional Chinese. Output only the translation, no explanations or extra text.",
  } satisfies Record<Direction, string>,
}
