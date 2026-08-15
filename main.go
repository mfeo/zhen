// Command zhen is a terminal UI for bidirectional Chinese <-> English
// translation backed by a local Ollama server.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"

	"zhen/internal/config"
	"zhen/internal/ollama"
	"zhen/internal/ui"
)

// version is overridden at build time with:
//
//	go build -ldflags="-X main.version=v1.2.3"
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run holds the whole entrypoint so it stays testable: nothing here calls
// os.Exit directly, and both output streams are injected.
func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("zhen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print the version and exit")
	fs.Usage = func() { fmt.Fprint(stderr, usage) }

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Fprintf(stdout, "zhen %s\n", version)
		return nil
	}

	cfg := config.Load()
	client := ollama.NewClient(cfg)

	if !client.Health(context.Background()) {
		return fmt.Errorf(
			"cannot connect to Ollama at %s\n\nStart it with:  ollama serve\nThen pull the model:  ollama pull %s",
			cfg.BaseURL, cfg.Model,
		)
	}

	// Ask Ollama to load the model now, so the first translation does not pay the
	// multi-second (cold: minute-scale) cost of loading weights into VRAM.
	client.Warmup()

	_, err := tea.NewProgram(ui.New(cfg)).Run()
	return err
}

const usage = `zhen — bidirectional Chinese <-> English translation in your terminal.

Usage:
  zhen [flags]

Flags:
  -version    print the version and exit
  -h, -help   show this help

Keys:
  Ctrl+T      translate
  Ctrl+L      toggle direction (zh->en / en->zh)
  Ctrl+Y      copy the translation to the clipboard
  Ctrl+Q, Esc quit

Environment:
  OLLAMA_URL     Ollama base URL      (default http://localhost:11434)
  OLLAMA_MODEL   model to translate with (default gemma4:e2b)

Requires a running Ollama server: https://ollama.com
`
