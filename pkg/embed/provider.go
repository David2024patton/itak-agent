package embed

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// Embedder generates vector embeddings from text.
//
// What: A provider-agnostic interface for embedding generation.
// Why:  The agent needs semantic search across its memory graph. Different
//       deployment environments have different embedding sources (Gemini API,
//       local Ollama, iTaK Torch), so we abstract behind this interface.
// How:  Implementations call the appropriate API/model and return float32 vectors.
type Embedder interface {
	// Embed returns a single embedding vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// BatchEmbed returns embedding vectors for multiple texts.
	// More efficient than calling Embed() in a loop for providers that
	// support batch requests (Gemini, Ollama).
	BatchEmbed(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the output vector dimension size.
	Dimensions() int

	// Name returns the provider name for logging.
	Name() string
}

// ── Global singleton ──────────────────────────────────────────────

var (
	globalEmbedder Embedder
	embedMu        sync.RWMutex
)

// Init creates the global embedder from config.
// Called once during agent startup.
func Init(cfg Config) error {
	embedMu.Lock()
	defer embedMu.Unlock()

	var primary Embedder
	var fallback Embedder

	switch cfg.Provider {
	case "gemini":
		p, err := NewGeminiEmbedder(cfg)
		if err != nil {
			log.Printf("[embed] Gemini init failed: %v, trying fallback", err)
		}
		if p != nil {
			primary = p
		}
	case "openai":
		p, err := NewOpenAIEmbedder(cfg)
		if err != nil {
			log.Printf("[embed] OpenAI init failed: %v, trying fallback", err)
		}
		if p != nil {
			primary = p
		}
	case "local", "ollama", "torch":
		p, err := NewLocalEmbedder(cfg)
		if err != nil {
			return fmt.Errorf("local embedder init: %w", err)
		}
		if p != nil {
			primary = p
		}
	default:
		return fmt.Errorf("unknown embedding provider: %q (use 'gemini', 'openai', 'local', or 'ollama')", cfg.Provider)
	}

	// Build fallback if configured and different from primary.
	if cfg.Fallback != "" && cfg.Fallback != cfg.Provider {
		switch cfg.Fallback {
		case "local", "ollama", "torch":
			fallback, _ = NewLocalEmbedder(cfg)
		case "gemini":
			fallback, _ = NewGeminiEmbedder(cfg)
		case "openai":
			fallback, _ = NewOpenAIEmbedder(cfg)
		}
	}

	if primary != nil {
		globalEmbedder = primary
		log.Printf("[embed] Primary embedder: %s (dims: %d)", primary.Name(), primary.Dimensions())
	} else if fallback != nil {
		globalEmbedder = fallback
		log.Printf("[embed] Using fallback embedder: %s (dims: %d)", fallback.Name(), fallback.Dimensions())
	} else {
		// No embedder available. The agent still works, just without semantic search.
		log.Printf("[embed] WARNING: No embedding provider available. Semantic search disabled.")
		globalEmbedder = &noopEmbedder{}
	}

	return nil
}

// Get returns the global embedder. Never nil after Init().
func Get() Embedder {
	embedMu.RLock()
	defer embedMu.RUnlock()
	if globalEmbedder == nil {
		return &noopEmbedder{}
	}
	return globalEmbedder
}

// ── Noop fallback ─────────────────────────────────────────────────

// noopEmbedder returns zero vectors. Used when no real provider is available.
type noopEmbedder struct{}

func (n *noopEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, nil
}

func (n *noopEmbedder) BatchEmbed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	return result, nil
}

func (n *noopEmbedder) Dimensions() int { return 0 }
func (n *noopEmbedder) Name() string    { return "noop" }
