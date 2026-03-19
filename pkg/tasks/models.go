package tasks

import (
	"time"
)

// Status represents the state of a Task.
type Status string

const (
	StatusTodo       Status = "Todo"
	StatusInProgress Status = "In Progress"
	StatusBlocked    Status = "Blocked"
	StatusReview     Status = "Review"
	StatusDone       Status = "Done"
	StatusFailed     Status = "Failed"
	StatusEscalated  Status = "Escalated"
	StatusWaiting    Status = "Waiting"   // Paused waiting for external webhook/event
	StatusPaused     Status = "Paused"    // Paused due to budget exceeded
)

// Priority levels for task ordering.
type Priority int

const (
	PriorityLow    Priority = 0
	PriorityMedium Priority = 1
	PriorityHigh   Priority = 2
	PriorityUrgent Priority = 3
)

// AutonomyLevel controls how much freedom the AI has.
type AutonomyLevel string

const (
	AutonomyDraftOnly  AutonomyLevel = "draft_only" // AI works, human must approve
	AutonomySupervised AutonomyLevel = "supervised" // AI runs, human reviews after
	AutonomyFull       AutonomyLevel = "autonomous" // AI executes and closes
)

// SubItem is a checklist entry within a task.
type SubItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// TaskHistoryEntry represents a single event in a task's life.
type TaskHistoryEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	WorkerAgent string    `json:"worker_agent,omitempty"`
	Action      string    `json:"action"`
}

// Comment represents a discussion entry on a task.
type Comment struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// Attachment represents a file attached to a task.
type Attachment struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	MimeType   string    `json:"mime_type"`
	UploadedAt time.Time `json:"uploaded_at"`
}

// AgentConfig holds AI-specific execution parameters for a task.
// This is what makes the system agentic rather than a plain Kanban board.
type AgentConfig struct {
	// SystemPrompt is per-task behavioral instructions for the agent.
	SystemPrompt string `json:"system_prompt,omitempty"`

	// OutputSchema defines what "done" looks like (JSON schema, format hint).
	OutputSchema string `json:"output_schema,omitempty"`

	// AllowedTools is a whitelist of tools/APIs the agent can use.
	AllowedTools []string `json:"allowed_tools,omitempty"`

	// MaxRetries is how many times to retry on failure before escalating.
	MaxRetries int `json:"max_retries,omitempty"`

	// MaxTokens caps compute spend for this task.
	MaxTokens int `json:"max_tokens,omitempty"`

	// TTLSeconds is the maximum wall-clock time allowed.
	TTLSeconds int `json:"ttl_seconds,omitempty"`

	// AutonomyLevel controls whether the AI needs human approval.
	AutonomyLevel AutonomyLevel `json:"autonomy_level,omitempty"`

	// SkillsRequired lists capabilities needed (for agent routing).
	SkillsRequired []string `json:"skills_required,omitempty"`

	// ContextRefs are pointers to external memory (graph nodes, vector IDs).
	ContextRefs []string `json:"context_refs,omitempty"`

	// ConfidenceThreshold is the minimum confidence score (0-100) before auto-routing to approval.
	ConfidenceThreshold int `json:"confidence_threshold,omitempty"`

	// AllowedHosts is a whitelist of network hosts the agent can reach (zero-trust).
	AllowedHosts []string `json:"allowed_hosts,omitempty"`

	// SandboxRequired forces execution inside an ephemeral Docker container.
	SandboxRequired bool `json:"sandbox_required,omitempty"`
}

// ReasoningEntry is one step in the AI's chain-of-thought scratchpad.
type ReasoningEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Step      string    `json:"step"`
	ToolCall  string    `json:"tool_call,omitempty"`
	Result    string    `json:"result,omitempty"`
}

// Task represents a unit of work on the Kanban board.
type Task struct {
	ID            string             `json:"id"`
	Title         string             `json:"title"`
	Description   string             `json:"description"`
	Status        Status             `json:"status"`
	Priority      Priority           `json:"priority"`
	Labels        []string           `json:"labels,omitempty"`
	DueDate       *time.Time         `json:"due_date,omitempty"`
	SubItems      []SubItem          `json:"sub_items,omitempty"`
	ProjectID     string             `json:"project_id,omitempty"`
	BlockedBy     []string           `json:"blocked_by,omitempty"`
	Blocks        []string           `json:"blocks,omitempty"`
	Comments      []Comment          `json:"comments,omitempty"`
	RecurPattern  string             `json:"recur_pattern,omitempty"`
	AssignedAgent string             `json:"assigned_agent,omitempty"`
	Review        *ReviewResult      `json:"review,omitempty"`
	Attachments   []Attachment       `json:"attachments,omitempty"`
	AutoExecute   bool               `json:"auto_execute,omitempty"`
	// AI Execution Parameters
	AgentConfig    *AgentConfig     `json:"agent_config,omitempty"`
	Reasoning      []ReasoningEntry `json:"reasoning,omitempty"`
	Progress       int              `json:"progress"`
	TaskType       string           `json:"task_type,omitempty"`
	EscalationNote string           `json:"escalation_note,omitempty"`
	RetryCount     int              `json:"retry_count,omitempty"`
	// State Machine & Dynamic Splitting
	ParentTaskID   string           `json:"parent_task_id,omitempty"`
	ChildTaskIDs   []string         `json:"child_task_ids,omitempty"`
	ConfidenceScore int             `json:"confidence_score,omitempty"`
	TokensUsed     int              `json:"tokens_used,omitempty"`
	WallClockStart *time.Time       `json:"wall_clock_start,omitempty"`
	WakeOnWebhook  string           `json:"wake_on_webhook,omitempty"`
	ModifiedFiles  []string         `json:"modified_files,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	History        []TaskHistoryEntry `json:"history"`
}
