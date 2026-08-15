package ollama

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"zhen/internal/config"
)

// --- decodeStream: the NDJSON parser that replaces the manual line buffer in
// src/services/ollama.ts ---

func collect(t *testing.T, body string) ([]string, error) {
	t.Helper()
	var got []string
	err := decodeStream(strings.NewReader(body), func(s string) { got = append(got, s) })
	return got, err
}

func TestDecodeStreamHappyPath(t *testing.T) {
	got, err := collect(t, `{"response":"Hello"}
{"response":" world"}
{"response":"","done":true}
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(got, "") != "Hello world" {
		t.Fatalf("got %q, want %q", strings.Join(got, ""), "Hello world")
	}
}

// A chunk arriving split mid-object is the failure mode the TS version needed a
// manual buffer plus a SyntaxError-swallowing catch for. json.Decoder must
// handle it with no help.
func TestDecodeStreamSplitAcrossReads(t *testing.T) {
	full := `{"response":"今"}` + "\n" + `{"response":"天"}` + "\n" + `{"response":"","done":true}` + "\n"
	for cut := 1; cut < len(full); cut++ {
		r := io.MultiReader(strings.NewReader(full[:cut]), strings.NewReader(full[cut:]))
		var got strings.Builder
		if err := decodeStream(r, func(s string) { got.WriteString(s) }); err != nil {
			t.Fatalf("cut at %d: unexpected error: %v", cut, err)
		}
		if got.String() != "今天" {
			t.Fatalf("cut at %d: got %q, want %q", cut, got.String(), "今天")
		}
	}
}

// An in-band error object must surface as an error, not be emitted as text.
func TestDecodeStreamInBandError(t *testing.T) {
	got, err := collect(t, `{"response":"partial"}
{"error":"model not found"}
`)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("error %q does not mention the cause", err)
	}
	if strings.Join(got, "") != "partial" {
		t.Fatalf("chunks before the error should still be delivered, got %q", got)
	}
}

// done:true ends the stream even if the server keeps writing.
func TestDecodeStreamStopsAtDone(t *testing.T) {
	got, err := collect(t, `{"response":"a"}
{"response":"b","done":true}
{"response":"SHOULD NOT APPEAR"}
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(got, "") != "ab" {
		t.Fatalf("got %q, want %q", strings.Join(got, ""), "ab")
	}
}

func TestDecodeStreamMalformedJSON(t *testing.T) {
	if _, err := collect(t, `{"response":"a"}`+"\n"+`{not json}`+"\n"); err == nil {
		t.Fatal("expected an error for malformed JSON, got nil")
	}
}

// A truncated stream (connection dropped mid-object) is an error, not a silent
// success — otherwise a half-finished translation looks complete.
func TestDecodeStreamTruncated(t *testing.T) {
	if _, err := collect(t, `{"response":"a"}`+"\n"+`{"respo`); err == nil {
		t.Fatal("expected an error for a truncated stream, got nil")
	}
}

// Empty body: server closed with nothing to say. Not an error, no chunks.
func TestDecodeStreamEmpty(t *testing.T) {
	got, err := collect(t, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no chunks, got %v", got)
	}
}

// --- Translate: HTTP behaviour against a fake Ollama ---

func newClientFor(url string) *Client {
	return NewClient(config.Config{BaseURL: url, Model: "test-model"})
}

func TestTranslateSendsExpectedRequest(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		io.WriteString(w, `{"response":"ok","done":true}`+"\n")
	}))
	defer srv.Close()

	var out strings.Builder
	if err := newClientFor(srv.URL).Translate(context.Background(), "你好", config.ZH2EN, func(s string) {
		out.WriteString(s)
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/api/generate" {
		t.Fatalf("path = %q, want /api/generate", gotPath)
	}
	for _, want := range []string{
		`"model":"test-model"`,
		`"prompt":"你好"`,
		`"stream":true`,
		`"think":false`,
		`"keep_alive":"24h"`,
		`"temperature":0`,
		"Translate the following Chinese text to English",
	} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("request body missing %s\nbody: %s", want, gotBody)
		}
	}
	if out.String() != "ok" {
		t.Fatalf("output = %q, want %q", out.String(), "ok")
	}
}

// en2zh must send the Traditional Chinese prompt, not the zh2en one.
func TestTranslateUsesDirectionPrompt(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		io.WriteString(w, `{"done":true}`+"\n")
	}))
	defer srv.Close()

	if err := newClientFor(srv.URL).Translate(context.Background(), "hi", config.EN2ZH, func(string) {}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotBody, "Traditional Chinese") {
		t.Fatalf("en2zh request did not use the Traditional Chinese prompt: %s", gotBody)
	}
}

func TestTranslateHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":"model 'test-model' not found"}`)
	}))
	defer srv.Close()

	var emitted int
	err := newClientFor(srv.URL).Translate(context.Background(), "hi", config.ZH2EN, func(string) { emitted++ })
	if err == nil {
		t.Fatal("expected an error for a 404 response")
	}
	if !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error %q should carry status and body", err)
	}
	if emitted != 0 {
		t.Fatalf("no chunks should be emitted on an HTTP error, got %d", emitted)
	}
}

// Cancelling must actually close the connection so Ollama stops generating —
// the whole point of wiring ctx through instead of just ignoring late chunks.
func TestTranslateCancelStopsServer(t *testing.T) {
	var mu sync.Mutex
	serverSawDisconnect := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		for i := 0; i < 1000; i++ {
			select {
			case <-r.Context().Done():
				mu.Lock()
				serverSawDisconnect = true
				mu.Unlock()
				return
			default:
			}
			io.WriteString(w, `{"response":"x"}`+"\n")
			flusher.Flush()
			time.Sleep(2 * time.Millisecond)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var seen int
	errCh := make(chan error, 1)
	go func() {
		errCh <- newClientFor(srv.URL).Translate(ctx, "hi", config.ZH2EN, func(string) {
			seen++
			if seen == 3 {
				cancel()
			}
		})
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Translate did not return after cancellation")
	}

	// Give the server goroutine a moment to observe the closed connection.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		saw := serverSawDisconnect
		mu.Unlock()
		if saw {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server never observed the client disconnect; generation would keep running")
}

func TestHealth(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("health hit %q, want /api/tags", r.URL.Path)
		}
		io.WriteString(w, `{"models":[]}`)
	}))
	defer ok.Close()

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()

	if !newClientFor(ok.URL).Health(context.Background()) {
		t.Error("healthy server reported unhealthy")
	}
	if newClientFor(bad.URL).Health(context.Background()) {
		t.Error("500 server reported healthy")
	}
	if newClientFor("http://127.0.0.1:1").Health(context.Background()) {
		t.Error("unreachable server reported healthy")
	}
}
