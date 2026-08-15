package config

import "testing"

func TestDirectionToggleIsInvolutive(t *testing.T) {
	for _, d := range []Direction{ZH2EN, EN2ZH} {
		if d.Toggle().Toggle() != d {
			t.Errorf("toggling %v twice did not return to itself", d.Label())
		}
	}
	if ZH2EN.Toggle() != EN2ZH {
		t.Error("ZH2EN should toggle to EN2ZH")
	}
}

func TestDirectionPromptsMatchDirection(t *testing.T) {
	if p := ZH2EN.SystemPrompt(); !contains(p, "Chinese text to English") {
		t.Errorf("zh2en prompt is wrong: %q", p)
	}
	if p := EN2ZH.SystemPrompt(); !contains(p, "English text to Traditional Chinese") {
		t.Errorf("en2zh prompt is wrong: %q", p)
	}
	if ZH2EN.SystemPrompt() == EN2ZH.SystemPrompt() {
		t.Error("both directions share a prompt")
	}
}

// The languages of the two pane titles must swap with the direction: whichever
// side is Chinese in zh2en has to be the English side in en2zh. The wording
// differs (輸入 vs 翻譯), so the invariant is about which language labels which
// pane, not about the exact strings.
func TestDirectionTitleLanguagesSwap(t *testing.T) {
	if !contains(ZH2EN.InputTitle(), "中文") {
		t.Errorf("zh2en input title should be Chinese-labelled, got %q", ZH2EN.InputTitle())
	}
	if !contains(ZH2EN.OutputTitle(), "English") {
		t.Errorf("zh2en output title should be English-labelled, got %q", ZH2EN.OutputTitle())
	}
	if !contains(EN2ZH.InputTitle(), "English") {
		t.Errorf("en2zh input title should be English-labelled, got %q", EN2ZH.InputTitle())
	}
	if !contains(EN2ZH.OutputTitle(), "中文") {
		t.Errorf("en2zh output title should be Chinese-labelled, got %q", EN2ZH.OutputTitle())
	}
}

// The placeholder must follow the direction too, or the user is prompted in the
// language they are meant to be reading, not writing.
func TestDirectionPlaceholdersFollowDirection(t *testing.T) {
	if !contains(ZH2EN.Placeholder(), "輸入中文") {
		t.Errorf("zh2en placeholder = %q", ZH2EN.Placeholder())
	}
	if !contains(EN2ZH.Placeholder(), "English") {
		t.Errorf("en2zh placeholder = %q", EN2ZH.Placeholder())
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("OLLAMA_URL", "")
	t.Setenv("OLLAMA_MODEL", "")
	cfg := Load()
	if cfg.BaseURL != "http://localhost:11434" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.Model != "qwen2.5:3b" {
		t.Errorf("Model = %q", cfg.Model)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("OLLAMA_URL", "http://box:9999")
	t.Setenv("OLLAMA_MODEL", "qwen2.5:7b")
	cfg := Load()
	if cfg.BaseURL != "http://box:9999" || cfg.Model != "qwen2.5:7b" {
		t.Errorf("env not honoured: %+v", cfg)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
