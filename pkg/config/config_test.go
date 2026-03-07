package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "itakagent.yaml")

	yaml := `
orchestrator:
  llm:
    provider: test
    model: test-model
    api_base: http://localhost
  max_delegations: 3
agents:
  - name: scout
    role: Scout
    personality: "test scout"
    goals: [accuracy]
    tools: [file_read]
data_dir: ./data
memory:
  window_size: 10
  auto_reflect: true
shell_safety:
  denied_cmds: ["format"]
  protected_paths: ["./cmd"]
`
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.Orchestrator.LLM.Model != "test-model" {
		t.Errorf("expected model 'test-model', got %q", cfg.Orchestrator.LLM.Model)
	}
	if cfg.Orchestrator.MaxDelegations != 3 {
		t.Errorf("expected max_delegations 3, got %d", cfg.Orchestrator.MaxDelegations)
	}
	if len(cfg.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "scout" {
		t.Errorf("expected agent name 'scout', got %q", cfg.Agents[0].Name)
	}
}

func TestLoadDefaults(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "itakagent.yaml")

	yaml := `
orchestrator:
  llm:
    model: test-model
agents:
  - name: scout
    role: Scout
    personality: "test"
    tools: [file_read]
`
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.Orchestrator.MaxDelegations != 5 {
		t.Errorf("default max_delegations should be 5, got %d", cfg.Orchestrator.MaxDelegations)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("default data_dir should be './data', got %q", cfg.DataDir)
	}
	if cfg.Memory.WindowSize != 20 {
		t.Errorf("default window_size should be 20, got %d", cfg.Memory.WindowSize)
	}
}

func TestLoadEnvExpansion(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "itakagent.yaml")

	os.Setenv("TEST_GOAGENT_MODEL", "my-special-model")
	defer os.Unsetenv("TEST_GOAGENT_MODEL")

	yaml := `
orchestrator:
  llm:
    model: ${TEST_GOAGENT_MODEL}
agents:
  - name: scout
    role: Scout
    personality: "test"
    tools: [file_read]
`
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.Orchestrator.LLM.Model != "my-special-model" {
		t.Errorf("expected 'my-special-model', got %q", cfg.Orchestrator.LLM.Model)
	}
}

func TestLoadInheritLLM(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "itakagent.yaml")

	yaml := `
orchestrator:
  llm:
    model: parent-model
    api_base: http://parent
agents:
  - name: scout
    role: Scout
    personality: "test"
    tools: [file_read]
`
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	// Agent without its own LLM should inherit from orchestrator.
	if cfg.Agents[0].LLM.Model != "parent-model" {
		t.Errorf("agent should inherit model, got %q", cfg.Agents[0].LLM.Model)
	}
}

func TestLoadBadYAML(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "itakagent.yaml")
	os.WriteFile(cfgPath, []byte("orchestrator:\n  llm:\n    - invalid\n    model: [broken: yaml: {{{"), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/itakagent.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidateNoModel(t *testing.T) {
	cfg := &Config{
		Agents: []AgentYAML{{Name: "scout", Role: "Scout"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("should fail when orchestrator model is empty")
	}
}

func TestValidateNoAgents(t *testing.T) {
	cfg := &Config{
		Orchestrator: OrchestratorYAML{},
	}
	cfg.Orchestrator.LLM.Model = "test"

	if err := cfg.Validate(); err == nil {
		t.Error("should fail when no agents defined")
	}
}

func TestValidateDuplicateAgentNames(t *testing.T) {
	cfg := &Config{
		Orchestrator: OrchestratorYAML{},
		Agents: []AgentYAML{
			{Name: "scout", Role: "Scout"},
			{Name: "Scout", Role: "Other Scout"}, // case-insensitive duplicate
		},
	}
	cfg.Orchestrator.LLM.Model = "test"

	if err := cfg.Validate(); err == nil {
		t.Error("should fail on duplicate agent names (case-insensitive)")
	}
}

func TestValidateNoAgentName(t *testing.T) {
	cfg := &Config{
		Orchestrator: OrchestratorYAML{},
		Agents:       []AgentYAML{{Name: "", Role: "Scout"}},
	}
	cfg.Orchestrator.LLM.Model = "test"

	if err := cfg.Validate(); err == nil {
		t.Error("should fail when agent has no name")
	}
}

func TestToAgentConfig(t *testing.T) {
	a := AgentYAML{
		Name:        "scout",
		Personality: "careful observer",
		Role:        "System Scout",
		Goals:       []string{"accuracy"},
		Tools:       []string{"file_read", "dir_list"},
		MaxLoops:    8,
		MaxSkills:   5,
	}

	cfg := a.ToAgentConfig()
	if cfg.Name != "scout" {
		t.Errorf("expected name 'scout', got %q", cfg.Name)
	}
	if len(cfg.ToolNames) != 2 {
		t.Errorf("expected 2 tools, got %d", len(cfg.ToolNames))
	}
	if cfg.MaxLoops != 8 {
		t.Errorf("expected max_loops 8, got %d", cfg.MaxLoops)
	}
}
