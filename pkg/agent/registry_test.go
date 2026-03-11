package agent

import (
	"strings"
	"testing"
)

func TestRegistryRegisterAndLookup(t *testing.T) {
	r := NewRegistry()

	entry := AgentEntry{
		Name:        "coder",
		Role:        "engineer",
		Personality: "writes Go code",
		Tools:       []string{"shell", "file_read", "file_write"},
	}
	r.Register(entry)

	got, ok := r.Lookup("coder")
	if !ok {
		t.Fatal("expected to find 'coder' in registry")
	}
	if got.Role != "engineer" {
		t.Errorf("expected role 'engineer', got %q", got.Role)
	}
	if got.Status != "ready" {
		t.Errorf("expected default status 'ready', got %q", got.Status)
	}
}

func TestRegistryInferCapabilities(t *testing.T) {
	caps := inferCapabilities([]string{"shell", "file_read", "http_fetch", "web_navigate"})

	capSet := make(map[Capability]bool)
	for _, c := range caps {
		capSet[c] = true
	}

	expected := []Capability{CapShell, CapFileOps, CapResearch, CapBrowse}
	for _, exp := range expected {
		if !capSet[exp] {
			t.Errorf("expected capability %s to be inferred", exp)
		}
	}
}

func TestRegistryFindByCapability(t *testing.T) {
	r := NewRegistry()
	r.Register(AgentEntry{
		Name:  "browser-agent",
		Role:  "browser",
		Tools: []string{"web_navigate", "web_screenshot"},
	})
	r.Register(AgentEntry{
		Name:  "coder-agent",
		Role:  "coder",
		Tools: []string{"shell", "file_write"},
	})

	browsers := r.FindByCapability(CapBrowse)
	if len(browsers) != 1 || browsers[0].Name != "browser-agent" {
		t.Errorf("expected 1 browser agent, got %d", len(browsers))
	}

	shells := r.FindByCapability(CapShell)
	if len(shells) != 1 || shells[0].Name != "coder-agent" {
		t.Errorf("expected 1 shell agent, got %d", len(shells))
	}
}

func TestRegistryListAllSorted(t *testing.T) {
	r := NewRegistry()
	r.Register(AgentEntry{Name: "zebra", Role: "z"})
	r.Register(AgentEntry{Name: "alpha", Role: "a"})
	r.Register(AgentEntry{Name: "mid", Role: "m"})

	all := r.ListAll()
	if len(all) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(all))
	}
	if all[0].Name != "alpha" || all[1].Name != "mid" || all[2].Name != "zebra" {
		t.Errorf("expected sorted order, got [%s, %s, %s]", all[0].Name, all[1].Name, all[2].Name)
	}
}

func TestRegistrySetStatus(t *testing.T) {
	r := NewRegistry()
	r.Register(AgentEntry{Name: "worker", Role: "test"})
	r.SetStatus("worker", "busy")

	e, ok := r.Lookup("worker")
	if !ok {
		t.Fatal("expected to find worker")
	}
	if e.Status != "busy" {
		t.Errorf("expected status 'busy', got %q", e.Status)
	}
}

func TestRegistryDescribe(t *testing.T) {
	r := NewRegistry()
	r.Register(AgentEntry{
		Name:        "helper",
		Role:        "assistant",
		Personality: "helpful and kind",
		Tools:       []string{"shell"},
	})

	desc := r.Describe()
	if desc == "" {
		t.Fatal("expected non-empty description")
	}
	if !strings.Contains(desc, "helper") || !strings.Contains(desc, "assistant") {
		t.Errorf("description should contain agent name and role, got: %s", desc)
	}
}
