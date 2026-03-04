package llm

import (
	"context"
	"fmt"
	"sync"

	"github.com/David2024patton/GOAgent/pkg/debug"
)

// FailoverClient wraps multiple LLM providers with automatic failover.
// On error, it retries with the next provider in priority order.
type FailoverClient struct {
	mu        sync.Mutex
	providers []namedClient
	primary   int // index of the current primary
}

type namedClient struct {
	Name     string
	Priority int
	Client   Client
	Config   ProviderConfig
}

// NewFailoverClient creates a failover-capable LLM client from multiple provider configs.
func NewFailoverClient(configs []ProviderConfig) *FailoverClient {
	fc := &FailoverClient{}
	for i, cfg := range configs {
		client := NewOpenAIClient(cfg)
		fc.providers = append(fc.providers, namedClient{
			Name:     fmt.Sprintf("%s/%s", cfg.Provider, cfg.Model),
			Priority: i,
			Client:   client,
			Config:   cfg,
		})
	}
	debug.Info("llm", "Failover client initialized with %d providers", len(fc.providers))
	for _, p := range fc.providers {
		debug.Debug("llm", "  Provider %d: %s @ %s", p.Priority, p.Name, p.Config.APIBase)
	}
	return fc
}

// Chat tries each provider in order until one succeeds.
func (fc *FailoverClient) Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error) {
	fc.mu.Lock()
	start := fc.primary
	providers := append([]namedClient{}, fc.providers...)
	fc.mu.Unlock()

	// Reorder: start from current primary, then wrap around.
	ordered := make([]namedClient, 0, len(providers))
	for i := 0; i < len(providers); i++ {
		idx := (start + i) % len(providers)
		ordered = append(ordered, providers[idx])
	}

	var lastErr error
	for _, p := range ordered {
		debug.Debug("llm", "Attempting provider: %s", p.Name)

		resp, err := p.Client.Chat(ctx, messages, tools)
		if err == nil {
			// Success  -  promote this provider to primary if it wasn't already.
			fc.mu.Lock()
			if fc.primary != p.Priority {
				debug.Info("llm", "Failover: promoted %s to primary (was provider %d)", p.Name, fc.primary)
				fc.primary = p.Priority
			}
			fc.mu.Unlock()
			return resp, nil
		}

		debug.Warn("llm", "Provider %s failed: %v  -  trying next", p.Name, err)
		lastErr = err
	}

	return nil, fmt.Errorf("all %d providers failed, last error: %w", len(ordered), lastErr)
}

// CurrentProvider returns the name of the current primary provider.
func (fc *FailoverClient) CurrentProvider() string {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.providers) == 0 {
		return "none"
	}
	return fc.providers[fc.primary].Name
}
