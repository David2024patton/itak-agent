// Package cli wraps the interactive REPL as a channel plugin.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/plugin"
)

// Config holds settings for the CLI REPL plugin.
type Config struct {
	Prompt string `yaml:"prompt"` // prompt string, default ">"
}

// Plugin provides an interactive stdin/stdout REPL.
type Plugin struct {
	cfg     Config
	handler plugin.MessageHandler
	cancel  context.CancelFunc
	done    chan struct{}
}

// New creates a CLI REPL plugin.
func New(cfg Config, handler plugin.MessageHandler) *Plugin {
	if cfg.Prompt == "" {
		cfg.Prompt = ">"
	}
	return &Plugin{
		cfg:     cfg,
		handler: handler,
		done:    make(chan struct{}),
	}
}

func (p *Plugin) Name() string { return "cli" }

func (p *Plugin) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	go p.replLoop(ctx)
	debug.Info("plugin:cli", "CLI REPL plugin started")
	return nil
}

func (p *Plugin) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}
	debug.Info("plugin:cli", "CLI REPL plugin stopped")
	return nil
}

// Done returns a channel that closes when the REPL exits (user typed "exit").
func (p *Plugin) Done() <-chan struct{} {
	return p.done
}

func (p *Plugin) replLoop(ctx context.Context) {
	defer close(p.done)

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(p.cfg.Prompt + " ")
		if !scanner.Scan() {
			return // EOF
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye.")
			return
		}

		msg := plugin.InboundMessage{
			Text:    input,
			Channel: "cli",
		}

		response, err := p.handler(ctx, msg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		fmt.Println(response)
	}
}
