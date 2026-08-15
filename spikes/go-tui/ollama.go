package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to a local Ollama server.
type Client struct {
	cfg  Config
	http *http.Client
}

func NewClient(cfg Config) *Client {
	// No overall timeout: a translation stream is long-lived and is bounded by
	// the caller's context instead.
	return &Client{cfg: cfg, http: &http.Client{}}
}

type generateRequest struct {
	Model     string         `json:"model"`
	System    string         `json:"system,omitempty"`
	Prompt    string         `json:"prompt"`
	Stream    bool           `json:"stream"`
	Think     bool           `json:"think"`
	KeepAlive string         `json:"keep_alive"`
	Options   map[string]any `json:"options,omitempty"`
}

type generateChunk struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
	Error    string `json:"error"`
}

// Health reports whether the Ollama server answers /api/tags.
func (c *Client) Health(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+"/api/tags", nil)
	if err != nil {
		return false
	}
	res, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	io.Copy(io.Discard, res.Body)
	return res.StatusCode == http.StatusOK
}

// Warmup asks Ollama to load the model and pin it in memory, without generating
// anything. Fire-and-forget, mirroring warmupOllama() in the TS version.
func (c *Client) Warmup() {
	go func() {
		body, _ := json.Marshal(generateRequest{
			Model: c.cfg.Model, Prompt: "", Think: false, KeepAlive: "24h",
		})
		req, err := http.NewRequest(http.MethodPost, c.cfg.BaseURL+"/api/generate", bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := c.http.Do(req)
		if err != nil {
			return
		}
		defer res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()
}

// Translate streams the translation of src, emitting each token to onChunk.
// It returns when the stream ends, ctx is cancelled, or an error occurs.
// Cancelling ctx closes the HTTP connection, which stops Ollama generating.
func (c *Client) Translate(ctx context.Context, src string, dir Direction, onChunk func(string)) error {
	body, err := json.Marshal(generateRequest{
		Model:     c.cfg.Model,
		System:    dir.SystemPrompt(),
		Prompt:    src,
		Stream:    true,
		Think:     false,
		KeepAlive: "24h",
		Options:   map[string]any{"temperature": 0},
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("ollama error %d: %s", res.StatusCode, bytes.TrimSpace(msg))
	}

	return decodeStream(res.Body, onChunk)
}

// decodeStream reads Ollama's NDJSON body. json.Decoder consumes one JSON value
// per call and buffers partial reads itself, so a chunk split mid-object across
// TCP packets is handled without any manual line buffering.
func decodeStream(r io.Reader, onChunk func(string)) error {
	dec := json.NewDecoder(r)
	for {
		var chunk generateChunk
		if err := dec.Decode(&chunk); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if chunk.Error != "" {
			return fmt.Errorf("ollama: %s", chunk.Error)
		}
		if chunk.Response != "" {
			onChunk(chunk.Response)
		}
		if chunk.Done {
			return nil
		}
	}
}
