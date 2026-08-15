package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"-version"}, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(out.String(), "zhen ") {
		t.Errorf("stdout = %q, want a version line", out.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr should be empty, got %q", errOut.String())
	}
}

func TestUnknownFlagIsAnError(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"-nope"}, &out, &errOut); err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty on a usage error, got %q", out.String())
	}
}

// With no reachable Ollama the program must fail with an actionable message
// rather than starting a UI that can never translate anything.
func TestUnreachableOllamaFailsWithGuidance(t *testing.T) {
	t.Setenv("OLLAMA_URL", "http://127.0.0.1:1")
	t.Setenv("OLLAMA_MODEL", "some-model")

	var out, errOut bytes.Buffer
	err := run(nil, &out, &errOut)
	if err == nil {
		t.Fatal("expected an error when Ollama is unreachable")
	}
	for _, want := range []string{"http://127.0.0.1:1", "ollama serve", "some-model"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message missing %q:\n%s", want, err)
		}
	}
}
