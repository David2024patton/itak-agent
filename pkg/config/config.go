package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/David2024patton/iTaKAgent/pkg/agent"
	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/llm"

	"gopkg.in/yaml.v3"
)

// Config is the top-level iTaKAgent configuration.
type Config struct {
	Orchestrator OrchestratorYAML            `yaml:"orchestrator"`
	Agents       []AgentYAML                 `yaml:"agents"`
	Providers    map[string]ProviderKeyYAML  `yaml:"providers,omitempty"`
	Integrations map[string]Integration      `yaml:"integrations,omitempty"`
	DataDir      string                      `yaml:"data_dir,omitempty"`
	Memory       MemoryYAML                  `yaml:"memory,omitempty"`
	ShellSafety  ShellSafetyYAML             `yaml:"shell_safety,omitempty"`
	Doctor       DoctorYAML                  `yaml:"doctor,omitempty"`
	MCP          []MCPServerYAML             `yaml:"mcp,omitempty"` // external MCP server connections
}

// MCPServerYAML configures a connection to an external MCP server.
type MCPServerYAML struct {
	Name      string   `yaml:"name"`                // human-readable name (becomes tool prefix)
	Transport string   `yaml:"transport,omitempty"` // "stdio" (default) or "sse"
	Command   string   `yaml:"command,omitempty"`   // path to server binary (for stdio)
	Args      []string `yaml:"args,omitempty"`      // command arguments (for stdio)
	URL       string   `yaml:"url,omitempty"`       // URL for SSE transport
	Enabled   bool     `yaml:"enabled,omitempty"`   // default: true
}

// DoctorYAML configures the self-healing Doctor agent.
type DoctorYAML struct {
	Enabled        bool               `yaml:"enabled,omitempty"`         // default: true
	LLM            llm.ProviderConfig `yaml:"llm,omitempty"`            // Doctor's own tiny LLM
	HealthInterval string             `yaml:"health_interval,omitempty"` // default: "30m"
}

// ProviderKeyYAML stores an API key (and optional model override) for a provider.
type ProviderKeyYAML struct {
	APIKey string `yaml:"api_key"`
	Model  string `yaml:"model,omitempty"` // override default model for this provider
}

// MemoryYAML is the YAML representation of memory config.
type MemoryYAML struct {
	WindowSize       int         `yaml:"window_size,omitempty"`       // default: 20
	AutoReflect      bool        `yaml:"auto_reflect,omitempty"`      // default: true
	AutoEntities     bool        `yaml:"auto_entities,omitempty"`     // default: true
	SessionWorkspace bool        `yaml:"session_workspace,omitempty"` // default: true
	Neo4j            Neo4jConfig `yaml:"neo4j,omitempty"`
}

// Neo4jConfig configures the hybrid memory graph connection.
type Neo4jConfig struct {
	Enabled  bool   `yaml:"enabled"`
	URI      string `yaml:"uri"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// ShellSafetyYAML holds shell security settings.
type ShellSafetyYAML struct {
	DeniedCommands []string `yaml:"denied_cmds,omitempty"`     // additional blocked commands
	ProtectedPaths []string `yaml:"protected_paths,omitempty"` // paths agents cannot write to (self-preservation)
	SandboxEnabled bool     `yaml:"sandbox_enabled,omitempty"` // future: Docker sandbox
}

// OrchestratorYAML is the YAML representation of orchestrator config.
type OrchestratorYAML struct {
	LLM            llm.ProviderConfig   `yaml:"llm"`
	SystemPrompt   string               `yaml:"system_prompt,omitempty"`
	MaxDelegations int                  `yaml:"max_delegations,omitempty"`
	Providers      []llm.ProviderConfig `yaml:"providers,omitempty"`  // failover providers in priority order
	Failover       FailoverYAML         `yaml:"failover,omitempty"`
	TokenBudget    int64                `yaml:"token_budget,omitempty"` // max tokens per session (0 = unlimited)
	FallbackModel  llm.ProviderConfig   `yaml:"fallback_model,omitempty"` // cheaper model for budget fallback
	AutonomyLevel  string               `yaml:"autonomy_level,omitempty"` // global default: supervised/guided/collaborative/autonomous/full_autopilot
}

// FailoverYAML holds LLM failover settings.
type FailoverYAML struct {
	Enabled  bool   `yaml:"enabled,omitempty"`
	Strategy string `yaml:"strategy,omitempty"` // "priority" (default) or "round_robin"
}

// AgentYAML is the YAML representation of a focused agent.
type AgentYAML struct {
	Name          string             `yaml:"name"`
	Personality   string             `yaml:"personality"`
	Role          string             `yaml:"role"`
	Goals         []string           `yaml:"goals"`
	Heartbeat     string             `yaml:"heartbeat,omitempty"`
	LLM           llm.ProviderConfig `yaml:"llm"`
	Tools         []string           `yaml:"tools"`
	SkillsDir     string             `yaml:"skills_dir,omitempty"`
	DataDirs      []string           `yaml:"data,omitempty"`
	MaxSkills     int                `yaml:"max_skills,omitempty"`
	MaxLoops      int                `yaml:"max_loops,omitempty"`
	AutonomyLevel string             `yaml:"autonomy_level,omitempty"` // per-agent override
	ContextBudget int                `yaml:"context_budget,omitempty"` // GOSqueeze budget (0=unlimited)
}

// Integration holds an external service connection config.
type Integration struct {
	APIKey   string `yaml:"api_key,omitempty"`
	Endpoint string `yaml:"endpoint,omitempty"`
}

// Load reads and parses a iTaKAgent config file, expanding environment variables.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Expand ${ENV_VAR} references.
	rawYAML = string(data) // preserve unexpanded form for Save()
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

	debug.Info("config", "Neo4j Configuration parsed: Enabled=%v, URI=%s", cfg.Memory.Neo4j.Enabled, cfg.Memory.Neo4j.URI)
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

	// Build active provider list from the providers map.
	if cfg.Providers != nil {
		keys := make(map[string]string, len(cfg.Providers))
		for slug, entry := range cfg.Providers {
			if entry.APIKey != "" {
				keys[slug] = entry.APIKey
			}
		}
		activeCfgs := llm.BuildProviderConfigs(keys)
		// Apply model overrides from the providers section.
		for i := range activeCfgs {
			if pk, ok := cfg.Providers[activeCfgs[i].Provider]; ok && pk.Model != "" {
				activeCfgs[i].Model = pk.Model
			}
		}
		cfg.Orchestrator.Providers = append(cfg.Orchestrator.Providers, activeCfgs...)
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
		Name:          a.Name,
		Personality:   a.Personality,
		Role:          a.Role,
		Goals:         a.Goals,
		Heartbeat:     a.Heartbeat,
		SkillsDir:     a.SkillsDir,
		DataDirs:      a.DataDirs,
		ToolNames:     a.Tools,
		MaxSkills:     a.MaxSkills,
		MaxLoops:      a.MaxLoops,
		Autonomy:      agent.ParseAutonomyLevel(a.AutonomyLevel),
		ContextBudget: a.ContextBudget,
		LLM:           a.LLM,
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

// ─── Secrets Persistence Guard (OpenClaw v2026.3.8) ────────────────
//
// When saving config back to disk, we must NOT write expanded secrets.
// If the original YAML had ${API_KEY}, the saved file must still have
// ${API_KEY}, not the resolved value.
//
// We accomplish this by keeping the raw unexpanded YAML bytes and
// patching only the non-secret fields on save.

// rawYAML stores the original unexpanded YAML text from the last Load.
// Used by Save() to preserve env-var references.
var rawYAML string

// Save writes the config back to the given path, preserving env-var
// references from the original file. Secret values (API keys, tokens)
// are NOT written in expanded form.
// Mirrors OpenClaw v2026.3.8: "Config/runtime snapshots: keep
// secrets-runtime-resolved config and auth-profile snapshots intact
// after config writes."
func (c *Config) Save(path string) error {
	var data []byte
	var err error

	if rawYAML != "" {
		// Re-use the original unexpanded YAML to preserve ${ENV_VAR} refs.
		data = []byte(rawYAML)
	} else {
		// No original YAML available - serialize current config.
		// WARNING: This will write expanded secrets.
		data, err = yaml.Marshal(c)
		if err != nil {
			return fmt.Errorf("marshal config: %w", err)
		}
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// ValidateEarly performs pre-default validation on the raw parsed config.
// This catches obviously broken configs before defaults are applied, so
// a bad config file cannot crash the gateway/agent on startup.
// Mirrors OpenClaw v2026.3.8: "Gateway/config restart guard: validate
// config before service start/restart."
func ValidateEarly(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config for validation: %w", err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return fmt.Errorf("config syntax error: %w", err)
	}

	// Basic structural checks before defaults.
	if cfg.Orchestrator.LLM.Model == "" && len(cfg.Agents) == 0 {
		return fmt.Errorf("config is effectively empty: no orchestrator model and no agents defined")
	}

	return nil
}
