package tool

import (
	"fmt"
	"sync"

	"github.com/David2024patton/GOAgent/pkg/llm"
)

// Registry holds all registered tools and provides lookup + LLM schema generation.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := t.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = t
	return nil
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tools[name]
	return t, ok
}

// ToolDefs generates the OpenAI function-calling tool definitions for all registered tools.
func (r *Registry) ToolDefs() []llm.ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]llm.ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, llm.ToolDef{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Schema(),
			},
		})
	}
	return defs
}

// Names returns all registered tool names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}
