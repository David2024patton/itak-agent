package agent

import (
	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/eventbus"
	"github.com/David2024patton/iTaKAgent/pkg/guard"
	"github.com/David2024patton/iTaKAgent/pkg/llm"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
	"github.com/David2024patton/iTaKAgent/pkg/task"
	"github.com/David2024patton/iTaKAgent/pkg/tool"
)


// AgentConfig defines a focused agent's identity and capabilities.
type AgentConfig struct {
	Name        string   `yaml:"name" json:"name"`
	Personality string   `yaml:"personality" json:"personality"`
	Role        string   `yaml:"role" json:"role"`
	Goals       []string `yaml:"goals" json:"goals"` // max 3 narrow KPIs
	Heartbeat   string   `yaml:"heartbeat" json:"heartbeat,omitempty"` // cron expression
	SkillsDir   string   `yaml:"skills_dir" json:"skills_dir,omitempty"`
	DataDirs    []string `yaml:"data" json:"data,omitempty"` // paths to reference data
	ToolNames   []string `yaml:"tools" json:"tools,omitempty"`
	MaxSkills   int      `yaml:"max_skills" json:"max_skills"` // default 7
	MaxLoops    int      `yaml:"max_loops" json:"max_loops"`   // default 10

	// Autonomy: how independently this agent operates (0=supervised, 4=autopilot).
	Autonomy AutonomyLevel `yaml:"autonomy" json:"autonomy"`

	// ContextBudget: max chars of context to send to LLM (0 = unlimited).
	// When set, GOSqueeze compresses tool outputs and conversation history.
	ContextBudget int `yaml:"context_budget" json:"context_budget"`

	// LLM config: each agent can use a different model.
	LLM llm.ProviderConfig `yaml:"llm" json:"llm"`
}

// DefaultMaxSkills is the hard cap on skills per focused agent.
const DefaultMaxSkills = 7

// DefaultMaxLoops is the default max iterations for the agent loop.
const DefaultMaxLoops = 10

// TaskPayload is what the orchestrator sends to a focused agent.
type TaskPayload struct {
	Agent   string `json:"agent"`   // target agent name
	Task    string `json:"task"`    // concise task description
	Context string `json:"context"` // only the info the agent needs
}

// Delegation is the orchestrator's parsed decision to delegate work.
type Delegation struct {
	Reasoning   string        `json:"reasoning"`
	Delegations []TaskPayload `json:"delegations"`
}

// Result is what a focused agent returns after completing a task.
type Result struct {
	Agent   string `json:"agent"`
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

// MemoryConfig holds memory-related settings for the orchestrator.
type MemoryConfig struct {
	AutoReflect  bool
	AutoEntities bool
}

// OrchestratorConfig defines the orchestrator's settings.
type OrchestratorConfig struct {
	LLM            llm.ProviderConfig `yaml:"llm" json:"llm"`
	SystemPrompt   string             `yaml:"system_prompt" json:"system_prompt,omitempty"`
	MaxDelegations int                `yaml:"max_delegations" json:"max_delegations"`
	Memory         MemoryConfig
	Autonomy       AutonomyLevel `yaml:"autonomy" json:"autonomy"` // global default autonomy
}

// Orchestrator coordinates focused agents without using tools itself.
type Orchestrator struct {
	Config     OrchestratorConfig
	LLMClient  llm.Client
	Agents     map[string]*FocusedAgent
	Registry   *AgentRegistry // capability-based agent lookup
	Memory     *memory.Manager
	Trace      *debug.StepLogger
	Tokens     *llm.TokenTracker
	Bus        *eventbus.EventBus
	Tasks      *task.Tracker
	Guard      *guard.InputGuard // prompt injection defense
	Doctor     *Doctor           // reference for escalation chain
	StatusFunc func(string)      // callback to display status to user
}

// FocusedAgent executes tasks using tools and skills.
type FocusedAgent struct {
	Config     AgentConfig
	LLMClient  llm.Client
	Tools      *tool.Registry
	Guardrails *tool.GuardrailChain
	Memory     *memory.Manager
	Trace      *debug.StepLogger
	Tokens     *llm.TokenTracker
	Bus        *eventbus.EventBus
	Guard      *guard.InputGuard // prompt injection defense
	SessionID  string
}


