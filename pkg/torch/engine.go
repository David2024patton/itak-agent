package torch

import (
	"context"
	"fmt"
	"sync"
)

// Engine is the interface for any inference backend.
// The CGo llama.cpp backend will implement this, but so can a mock for testing.
type Engine interface {
	// Complete runs text completion and returns the generated text.
	Complete(ctx context.Context, messages []ChatMessage, params CompletionParams) (string, error)
	// ModelName returns the name of the currently loaded model.
	ModelName() string
	// GetStats returns engine performance stats.
	GetStats() EngineStats
	// Close unloads the model and frees resources.
	Close() error
}

// MockEngine is a test/placeholder engine that returns canned responses.
// Used when CGo/llama.cpp is not available (like on Windows without MinGW).
type MockEngine struct {
	name string
	mu   sync.Mutex
}

// NewMockEngine creates a mock engine for testing without CGo.
func NewMockEngine(name string) *MockEngine {
	return &MockEngine{name: name}
}

func (m *MockEngine) Complete(ctx context.Context, messages []ChatMessage, params CompletionParams) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(messages) == 0 {
		return "", fmt.Errorf("no messages provided")
	}

	// Return a simple response based on the last user message.
	last := messages[len(messages)-1]
	return fmt.Sprintf("[GOTorch Mock / %s] Received: %q", m.name, last.Content), nil
}

func (m *MockEngine) ModelName() string {
	return m.name
}

func (m *MockEngine) GetStats() EngineStats {
	return EngineStats{}
}

func (m *MockEngine) Close() error {
	return nil
}
