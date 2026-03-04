package tool

import (
	"context"
	"testing"
)

// mockTool implements Tool for testing.
type mockTool struct {
	name string
	desc string
}

func (m *mockTool) Name() string                       { return m.name }
func (m *mockTool) Description() string                { return m.desc }
func (m *mockTool) Schema() map[string]interface{}     { return map[string]interface{}{"type": "object"} }
func (m *mockTool) Execute(_ context.Context, _ map[string]interface{}) (string, error) {
	return "ok", nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()

	tool := &mockTool{name: "test_tool", desc: "A test tool"}
	if err := r.Register(tool); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	got, ok := r.Get("test_tool")
	if !ok {
		t.Fatal("Get returned false for registered tool")
	}
	if got.Name() != "test_tool" {
		t.Errorf("expected name 'test_tool', got %q", got.Name())
	}
}

func TestRegistryDuplicateRejection(t *testing.T) {
	r := NewRegistry()

	tool1 := &mockTool{name: "dupe", desc: "first"}
	tool2 := &mockTool{name: "dupe", desc: "second"}

	if err := r.Register(tool1); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}
	if err := r.Register(tool2); err == nil {
		t.Fatal("expected error on duplicate registration, got nil")
	}
}

func TestRegistryGetMissing(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("Get should return false for missing tool")
	}
}

func TestRegistryNames(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "alpha", desc: "a"})
	r.Register(&mockTool{name: "beta", desc: "b"})
	r.Register(&mockTool{name: "gamma", desc: "c"})

	names := r.Names()
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	for _, expected := range []string{"alpha", "beta", "gamma"} {
		if !nameSet[expected] {
			t.Errorf("missing expected name: %s", expected)
		}
	}
}

func TestRegistryToolDefs(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "my_tool", desc: "Test description"})

	defs := r.ToolDefs()
	if len(defs) != 1 {
		t.Fatalf("expected 1 tool def, got %d", len(defs))
	}

	def := defs[0]
	if def.Type != "function" {
		t.Errorf("expected type 'function', got %q", def.Type)
	}
	if def.Function.Name != "my_tool" {
		t.Errorf("expected name 'my_tool', got %q", def.Function.Name)
	}
	if def.Function.Description != "Test description" {
		t.Errorf("expected description 'Test description', got %q", def.Function.Description)
	}
}
