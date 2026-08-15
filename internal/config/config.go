package config

import "os"

// Direction is the translation direction the UI is currently in.
type Direction int

const (
	ZH2EN Direction = iota
	EN2ZH
)

// Toggle returns the opposite direction.
func (d Direction) Toggle() Direction {
	if d == ZH2EN {
		return EN2ZH
	}
	return ZH2EN
}

// SystemPrompt is the instruction sent to the model as the `system` field.
func (d Direction) SystemPrompt() string {
	if d == ZH2EN {
		return "Translate the following Chinese text to English. Output only the translation, no explanations or extra text."
	}
	return "Translate the following English text to Traditional Chinese. Output only the translation, no explanations or extra text."
}

// Label is the short indicator shown in the status bar.
func (d Direction) Label() string {
	if d == ZH2EN {
		return "ZH→EN"
	}
	return "EN→ZH"
}

// InputTitle / OutputTitle are the pane border titles.
func (d Direction) InputTitle() string {
	if d == ZH2EN {
		return " 中文輸入 "
	}
	return " English Input "
}

func (d Direction) OutputTitle() string {
	if d == ZH2EN {
		return " English Translation "
	}
	return " 中文翻譯 "
}

func (d Direction) Placeholder() string {
	if d == ZH2EN {
		return "在這裡輸入中文... (Ctrl+T 翻譯)"
	}
	return "Type English here... (Ctrl+T to translate)"
}

// Config mirrors src/config.ts.
type Config struct {
	BaseURL string
	Model   string
}

func Load() Config {
	return Config{
		BaseURL: envOr("OLLAMA_URL", "http://localhost:11434"),
		Model:   envOr("OLLAMA_MODEL", "qwen2.5:3b"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
