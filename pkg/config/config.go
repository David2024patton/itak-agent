package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/David2024patton/GOAgent/pkg/agent"
	"github.com/David2024patton/GOAgent/pkg/llm"

	"gopkg.in/yaml.v3"
)

// Config is the top-level GOAgent configuration.
type Config struct {
	Orchestrator OrchestratorYAML       `yaml:"orchestrator"`
	Agents       []AgentYAML            `yaml:"agents"`
	Integrations map[string]Integration `yaml:"integrations,omitempty"`
	DataDir      string                 `yaml:"data_dir,omitempty"`
	Memory       MemoryYAML             `yaml:"memory,omitempty"`
	ShellSafety  ShellSafetyYAML        `yaml:"shell_safety,omitempty"`
}

// MemoryYAML is the YAML representation of memory config.
type MemoryYAML struct {
	WindowSize       int  `yaml:"window_size,omitempty"`       // default: 20
	AutoReflect      bool `yaml:"auto_reflect,omitempty"`      // default: true
	AutoEntities     bool `yaml:"auto_entities,omitempty"`     // default: true
	SessionWorkspace bool `yaml:"session_workspace,omitempty"` // default: true
}

// ShellSafetyYAML holds shell security settings.
type ShellSafetyYAML struct {
	DeniedCommands []string `yaml:"denied_cmds,omitempty"`    // additional blocked commands
	SandboxEnabled bool     `yaml:"sandbox_enabled,omitempty"` // future: Docker sandbox
}

// OrchestratorYAML is the YAML representation of orchestrator config.
type OrchestratorYAML struct {
	LLM            llm.ProviderConfig `yaml:"llm"`
	SystemPrompt   string             `yaml:"system_prompt,omitempty"`
	MaxDelegations int                `yaml:"max_delegations,omitempty"`
}

// AgentYAML is the YAML representation of a focused agent.
type AgentYAML struct {
	Name        string             `yaml:"name"`
	Personality string             `yaml:"personality"`
	Role        string             `yaml:"role"`
	Goals       []string           `yaml:"goals"`
	Heartbeat   string             `yaml:"heartbeat,omitempty"`
	LLM         llm.ProviderConfig `yaml:"llm"`
	Tools       []string           `yaml:"tools"`
	SkillsDir   string             `yaml:"skills_dir,omitempty"`
	DataDirs    []string           `yaml:"data,omitempty"`
	MaxSkills   int                `yaml:"max_skills,omitempty"`
	MaxLoops    int                `yaml:"max_loops,omitempty"`
}

// Integration holds an external service connection config.
type Integration struct {
	APIKey   string `yaml:"api_key,omitempty"`
	Endpoint string `yaml:"endpoint,omitempty"`
}

// Load reads and parses a GOAgent config file, expanding environment variables.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Expand ${ENV_VAR} references.
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Set defaults.
	if cfg.Orchestrator.MaxDelegations == 0 {
		cfg.Orchestrator.MaxDelegations = 5
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	if cfg.Memory.WindowSize == 0 {
		cfg.Memory.WindowSize = 20
	}
	// Enable memory features by default (YAML omits = zero value = false,
	// so we flip the logic: disabled_* fields, or just default to true here)
	// For simplicity, we default both to true if not explicitly set in YAML.
	// Users set auto_reflect: false to disable.
	if !cfg.Memory.AutoReflect {
		cfg.Memory.AutoReflect = true
	}
	if !cfg.Memory.AutoEntities {
		cfg.Memory.AutoEntities = true
	}

	for i := range cfg.Agents {
		if cfg.Agents[i].MaxSkills == 0 {
			cfg.Agents[i].MaxSkills = agent.DefaultMaxSkills
		}
		if cfg.Agents[i].MaxLoops == 0 {
			cfg.Agents[i].MaxLoops = agent.DefaultMaxLoops
		}
		// If agent doesn't specify its own LLM, inherit from orchestrator.
		if cfg.Agents[i].LLM.Model == "" {
			cfg.Agents[i].LLM = cfg.Orchestrator.LLM
		}
	}

	return &cfg, nil
}

// ToAgentConfig converts a YAML agent definition to the runtime AgentConfig.
func (a *AgentYAML) ToAgentConfig() agent.AgentConfig {
	return agent.AgentConfig{
		Name:        a.Name,
		Personality: a.Personality,
		Role:        a.Role,
		Goals:       a.Goals,
		Heartbeat:   a.Heartbeat,
		SkillsDir:   a.SkillsDir,
		DataDirs:    a.DataDirs,
		ToolNames:   a.Tools,
		MaxSkills:   a.MaxSkills,
		MaxLoops:    a.MaxLoops,
		LLM:         a.LLM,
	}
}

// Validate checks the config for obvious errors.
func (c *Config) Validate() error {
	if c.Orchestrator.LLM.Model == "" {
		return fmt.Errorf("orchestrator.llm.model is required")
	}
	if len(c.Agents) == 0 {
		return fmt.Errorf("at least one agent must be defined")
	}

	names := make(map[string]bool)
	for _, a := range c.Agents {
		if a.Name == "" {
			return fmt.Errorf("every agent must have a name")
		}
		lower := strings.ToLower(a.Name)
		if names[lower] {
			return fmt.Errorf("duplicate agent name: %s", a.Name)
		}
		names[lower] = true

		if a.MaxSkills > agent.DefaultMaxSkills {
			return fmt.Errorf("agent %q: max_skills cannot exceed %d", a.Name, agent.DefaultMaxSkills)
		}
	}
	return nil
}
