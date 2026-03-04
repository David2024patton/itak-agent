package llm

import (
	"testing"
)

func TestProviderCatalogHasAll40Plus(t *testing.T) {
	all := AllProviders()
	if len(all) < 40 {
		t.Errorf("expected at least 40 providers, got %d", len(all))
	}
}

func TestNoDuplicateSlugs(t *testing.T) {
	seen := make(map[string]bool)
	for _, p := range AllProviders() {
		if seen[p.Slug] {
			t.Errorf("duplicate slug: %s", p.Slug)
		}
		seen[p.Slug] = true
	}
}

func TestAllProvidersHaveNames(t *testing.T) {
	for _, p := range AllProviders() {
		if p.Name == "" {
			t.Errorf("provider with slug %q has no name", p.Slug)
		}
		if p.Slug == "" {
			t.Errorf("provider with name %q has no slug", p.Name)
		}
		if p.Category == "" {
			t.Errorf("provider %q has no category", p.Slug)
		}
	}
}

func TestGetProviderKnown(t *testing.T) {
	tests := []struct {
		slug     string
		wantName string
	}{
		{"openai", "OpenAI"},
		{"groq", "Groq"},
		{"openrouter", "OpenRouter"},
		{"nvidia_nim", "NVIDIA NIM"},
		{"ollama", "Ollama (Local)"},
		{"deepseek", "DeepSeek"},
	}
	for _, tc := range tests {
		p, ok := GetProvider(tc.slug)
		if !ok {
			t.Errorf("GetProvider(%q) not found", tc.slug)
			continue
		}
		if p.Name != tc.wantName {
			t.Errorf("GetProvider(%q).Name = %q, want %q", tc.slug, p.Name, tc.wantName)
		}
	}
}

func TestGetProviderUnknown(t *testing.T) {
	_, ok := GetProvider("this_does_not_exist")
	if ok {
		t.Error("expected GetProvider for unknown slug to return false")
	}
}

func TestCompatibleProviders(t *testing.T) {
	compat := CompatibleProviders()
	if len(compat) == 0 {
		t.Fatal("expected at least 1 compatible provider")
	}
	for _, p := range compat {
		if !p.Compatible {
			t.Errorf("CompatibleProviders returned non-compatible: %s", p.Slug)
		}
		if p.APIBase == "" {
			t.Errorf("CompatibleProviders returned empty APIBase: %s", p.Slug)
		}
	}
}

func TestBuildProviderConfigsEmpty(t *testing.T) {
	configs := BuildProviderConfigs(map[string]string{})
	if len(configs) != 0 {
		t.Errorf("expected 0 configs with no keys, got %d", len(configs))
	}
}

func TestBuildProviderConfigsWithKeys(t *testing.T) {
	keys := map[string]string{
		"openai":       "sk-test123",
		"groq":         "gsk-test456",
		"nonexistent":  "should-be-ignored",
		"anthropic":    "ignored-not-compatible",
	}
	configs := BuildProviderConfigs(keys)

	// Should include openai and groq (compatible), skip nonexistent and anthropic (not compatible).
	found := make(map[string]bool)
	for _, c := range configs {
		found[c.Provider] = true
		if c.APIKey == "" {
			t.Errorf("config for %s has empty API key", c.Provider)
		}
		if c.APIBase == "" {
			t.Errorf("config for %s has empty APIBase", c.Provider)
		}
	}

	if !found["openai"] {
		t.Error("expected openai in configs")
	}
	if !found["groq"] {
		t.Error("expected groq in configs")
	}
	if found["nonexistent"] {
		t.Error("nonexistent should not be in configs")
	}
	if found["anthropic"] {
		t.Error("anthropic should not be in configs (not compatible)")
	}
}
