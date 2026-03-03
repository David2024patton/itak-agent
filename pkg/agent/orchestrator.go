package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/David2024patton/GOAgent/pkg/debug"
	"github.com/David2024patton/GOAgent/pkg/llm"
	"github.com/David2024patton/GOAgent/pkg/memory"
)

// delegationSystemPrompt is the baked-in system prompt for the orchestrator.
// It tells the LLM to ONLY reason and delegate — never use tools.
// %d = agent count, %s = data directory, %s = agent descriptions
const delegationSystemPrompt = `You are GOAgent — a lightweight, sovereign AI agent framework written in Go.

ABOUT YOU:
- You are GOAgent v0.2.0, created by David Patton
- GitHub: https://github.com/David2024patton/GOAgent
- You are purpose-built for 30B-parameter models and smaller (like NVIDIA Nemotron)
- Your architecture: Orchestrator-Delegate pattern — you reason and route, focused agents execute with tools
- You have built-in memory (session + persistent + archive), skills, guardrails, and time-travel debugging
- You have EXACTLY %d agent(s) available — listed below in AVAILABLE AGENTS

YOUR PRIMARY JOB IS TO DELEGATE. You have NO tools yourself.
You reason about requests, then delegate to your focused agents who DO the work.

DELEGATION RULES (FOLLOW THESE FIRST):
1. Delegate to "scout" when the user asks about: files, folders, skills, messages, conversations, project structure, data on disk, or anything that requires checking the filesystem. The scout will physically look and report real data.
2. Delegate to "operator" when the user wants to: create files, save data, run commands, or make changes.
3. Delegate to "browser" when the user wants to: visit a website, read a web page, take a screenshot, or extract data from a URL.
4. Delegate to "researcher" for: general research, fetching raw URLs, gathering information.
5. Delegate to "coder" for: writing code, debugging, running programs.
6. You can chain delegations — e.g. scout checks data, then operator acts on it.

ANSWER DIRECTLY (from the AVAILABLE AGENTS section below) when asked about:
- "how many agents do you have" — count the agents listed below and name them
- "what agents/tools do you have" — list them from the AVAILABLE AGENTS section
- "what can you do" — describe your capabilities based on your agents and their tools
- Pure greetings: "hi", "hello", "hey"
- Identity questions: "what are you", "who made you", "what is GOAgent"

DELEGATE TO SCOUT (never guess) when asked about:
- "how many skills/messages/conversations are there" — scout checks the filesystem
- "list files/folders" — scout browses the directory
- "show me the data/project structure" — scout lists directories
- ANY question about data that lives on disk

When in doubt — DELEGATE to scout. It is always better to delegate than to guess.

SYSTEM INFO:
- Data directory: %s
- Conversations: {data_dir}/conversations/ (JSONL logs in numbered folders)
- Skills: {data_dir}/skills/ (SKILL.md files in subdirectories)
- Facts: {data_dir}/facts.json
- Entities: {data_dir}/entities.json

AVAILABLE AGENTS:
%s

RESPOND IN THIS EXACT JSON FORMAT:
{
  "reasoning": "your step-by-step thinking about how to handle this request",
  "delegations": [
    {
      "agent": "agent_name",
      "task": "clear, concise task description",
      "context": "only the specific info this agent needs"
    }
  ]
}

ONLY if the question is a pure greeting or identity question, respond:
{
  "reasoning": "this is a greeting/identity question I can answer directly",
  "delegations": [],
  "direct_response": "your answer here"
}
`


// synthesisSystemPrompt is used when the orchestrator combines agent results.
const synthesisSystemPrompt = `You are the GOAgent Orchestrator synthesizing results.
Given the original user request and the results from your focused agents, create a clear, helpful final response.
Be concise. Present the information naturally — don't mention agents or delegation mechanics to the user.`

// NewOrchestrator creates an orchestrator with its LLM client and registered agents.
func NewOrchestrator(cfg OrchestratorConfig, agents map[string]*FocusedAgent, mem *memory.Manager, trace *debug.StepLogger, tokens *llm.TokenTracker) *Orchestrator {
	client := llm.NewOpenAIClient(cfg.LLM)
	return &Orchestrator{
		Config:    cfg,
		LLMClient: client,
		Agents:    agents,
		Memory:    mem,
		Trace:     trace,
		Tokens:    tokens,
	}
}

// Run processes a user message through the orchestrator pipeline:
// 1. Reason about the request
// 2. Delegate to focused agents
// 3. Collect results
// 4. Synthesize final response
func (o *Orchestrator) Run(ctx context.Context, userMessage string) (string, error) {
	debug.Info("orchestrator", "Processing: %s", truncate(userMessage, 80))
	debug.Separator("orchestrator")

	// Trace: user message received.
	if o.Trace != nil {
		o.Trace.Record(debug.StepUserMessage, "orchestrator", "", userMessage, "", nil)
	}

	// Log user message to session memory and archive.
	if o.Memory != nil {
		userMsg := llm.Message{Role: llm.RoleUser, Content: userMessage}
		o.Memory.Session.Add(userMsg)
		o.Memory.Archive.LogMessage(memory.LogMessage{
			Role:      string(llm.RoleUser),
			Content:   userMessage,
			Timestamp: time.Now(),
		})

		// Auto-track entities in user messages.
		if o.Config.Memory.AutoEntities {
			o.Memory.Entities.Track(userMessage, o.Memory.Archive.NextID())
		}
	}
	// Build the agent descriptions for the system prompt.
	agentDescs := o.buildAgentDescriptions()
	dataDir := ""
	if o.Memory != nil {
		dataDir = o.Memory.DataDir
	}
	sysPrompt := fmt.Sprintf(delegationSystemPrompt, len(o.Agents), dataDir, agentDescs)

	if o.Config.SystemPrompt != "" {
		sysPrompt = o.Config.SystemPrompt + "\n\n" + sysPrompt
	}

	debug.Debug("orchestrator", "System prompt length: %d chars, %d agents available", len(sysPrompt), len(o.Agents))

	// Build messages WITH conversation history from session memory.
	messages := []llm.Message{{Role: llm.RoleSystem, Content: sysPrompt}}
	if o.Memory != nil {
		history := o.Memory.Session.GetContext()
		messages = append(messages, history...)
		debug.Debug("orchestrator", "Including %d messages of conversation history", len(history))
	} else {
		// Fallback if no memory: just use the current message.
		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: userMessage})
	}

	// Status: Thinking
	o.status("GOAgent Thinking...")

	debug.Info("orchestrator", "Calling LLM for delegation decision...")
	resp, err := o.LLMClient.Chat(ctx, messages, nil) // no tools for orchestrator
	if err != nil {
		debug.Error("orchestrator", "LLM call failed: %v", err)
		return "", fmt.Errorf("orchestrator LLM call: %w", err)
	}

	debug.JSON("orchestrator", "Raw LLM response", resp.Content)
	if resp.Usage != nil {
		debug.Debug("orchestrator", "Tokens — prompt: %d, completion: %d, total: %d",
			resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	}

	// Step 2: Parse the delegation decision.
	delegation, directResponse, err := parseDelegation(resp.Content)
	if err != nil {
		debug.Error("orchestrator", "Failed to parse delegation: %v", err)
		return "", fmt.Errorf("parse delegation: %w", err)
	}

	// If the orchestrator answered directly, log and return it.
	if directResponse != "" {
		debug.Info("orchestrator", "Direct response (no delegation): %s", truncate(directResponse, 100))
		o.logAssistantResponse(directResponse)
		return directResponse, nil
	}

	if len(delegation.Delegations) == 0 {
		debug.Warn("orchestrator", "No delegations and no direct response — unclear request")
		fallback := "I wasn't sure how to help with that. Could you rephrase?"
		o.logAssistantResponse(fallback)
		return fallback, nil
	}

	debug.Info("orchestrator", "Reasoning: %s", truncate(delegation.Reasoning, 150))
	debug.Info("orchestrator", "Delegating to %d agent(s)", len(delegation.Delegations))

	// Trace: delegation decision.
	if o.Trace != nil {
		o.Trace.Record(debug.StepDelegation, "orchestrator", "", delegation.Reasoning, "", map[string]interface{}{
			"agent_count": len(delegation.Delegations),
		})
	}

	// Step 3: Execute delegations.
	results := make([]Result, 0, len(delegation.Delegations))
	for i, task := range delegation.Delegations {
		agent, ok := o.Agents[task.Agent]
		if !ok {
			debug.Error("orchestrator", "Unknown agent %q in delegation %d", task.Agent, i+1)
			results = append(results, Result{
				Agent:   task.Agent,
				Success: false,
				Error:   fmt.Sprintf("unknown agent: %s", task.Agent),
			})
			continue
		}

		// Status: Delegating
		o.status(fmt.Sprintf("GOAgent Delegating to %s...", task.Agent))

		debug.Separator(task.Agent)
		debug.Info("orchestrator", "→ Delegating [%d/%d] to %q: %s",
			i+1, len(delegation.Delegations), task.Agent, truncate(task.Task, 100))

		// Trace: agent start.
		if o.Trace != nil {
			o.Trace.Record(debug.StepAgentStart, task.Agent, "", task.Task, "", nil)
		}

		startTime := time.Now()
		result := agent.Run(ctx, task)

		if result.Success {
			debug.Info("orchestrator", "← %q succeeded: %s", task.Agent, truncate(result.Output, 100))
		} else {
			debug.Error("orchestrator", "← %q failed: %s", task.Agent, result.Error)
		}
		results = append(results, result)

		// Trace: agent complete.
		if o.Trace != nil {
			output := result.Output
			if !result.Success {
				output = result.Error
			}
			o.Trace.RecordTimed(debug.StepAgentComplete, task.Agent, "", task.Task, startTime, truncate(output, 500))
		}

		// Auto-reflect: record what the agent learned from this task.
		if o.Config.Memory.AutoReflect && o.Memory != nil {
			outcome := "success"
			lessons := truncate(result.Output, 200)
			if !result.Success {
				outcome = "failure"
				lessons = result.Error
			}
			if err := o.Memory.Reflections.Add(task.Agent, task.Task, outcome, lessons); err != nil {
				debug.Warn("orchestrator", "Auto-reflect save failed: %v", err)
			} else {
				debug.Debug("orchestrator", "Auto-reflected for %q: %s → %s", task.Agent, outcome, truncate(lessons, 80))
			}
		}
	}

	// Step 4: Synthesize results.
	debug.Separator("orchestrator")
	o.status("GOAgent Synthesizing...")
	debug.Info("orchestrator", "Synthesizing %d result(s)...", len(results))
	finalResponse, err := o.synthesize(ctx, userMessage, results)
	if err != nil {
		return "", err
	}

	// Trace: final response.
	if o.Trace != nil {
		o.Trace.Record(debug.StepFinalResponse, "orchestrator", "", userMessage, truncate(finalResponse, 500), nil)
	}

	// Log the synthesized response.
	o.logAssistantResponse(finalResponse)
	return finalResponse, nil
}

// logAssistantResponse saves the assistant's response to session + archive.
func (o *Orchestrator) logAssistantResponse(response string) {
	if o.Memory == nil {
		return
	}
	o.Memory.Session.Add(llm.Message{Role: llm.RoleAssistant, Content: response})
	o.Memory.Archive.LogMessage(memory.LogMessage{
		Role:      string(llm.RoleAssistant),
		Content:   response,
		Timestamp: time.Now(),
	})

	// Auto-track entities in responses.
	if o.Config.Memory.AutoEntities {
		o.Memory.Entities.Track(response, o.Memory.Archive.NextID())
	}
}

// status fires the status callback if set.
func (o *Orchestrator) status(msg string) {
	if o.StatusFunc != nil {
		o.StatusFunc(msg)
	}
}

// buildAgentDescriptions generates the agent list for the system prompt.
func (o *Orchestrator) buildAgentDescriptions() string {
	var sb strings.Builder
	for name, agent := range o.Agents {
		sb.WriteString(fmt.Sprintf("- **%s** (role: %s): %s\n", name, agent.Config.Role, agent.Config.Personality))
		if len(agent.Config.Goals) > 0 {
			sb.WriteString(fmt.Sprintf("  Goals: %s\n", strings.Join(agent.Config.Goals, ", ")))
		}
		sb.WriteString(fmt.Sprintf("  Tools: %s\n", strings.Join(agent.Tools.Names(), ", ")))
	}
	return sb.String()
}

// synthesize combines agent results into a final user-facing response.
func (o *Orchestrator) synthesize(ctx context.Context, userMessage string, results []Result) (string, error) {
	// Build a summary of results for the LLM.
	var sb strings.Builder
	sb.WriteString("ORIGINAL USER REQUEST:\n")
	sb.WriteString(userMessage)
	sb.WriteString("\n\nAGENT RESULTS:\n")

	for _, r := range results {
		if r.Success {
			sb.WriteString(fmt.Sprintf("[%s] SUCCESS:\n%s\n\n", r.Agent, r.Output))
		} else {
			sb.WriteString(fmt.Sprintf("[%s] FAILED: %s\n\n", r.Agent, r.Error))
		}
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: synthesisSystemPrompt},
		{Role: llm.RoleUser, Content: sb.String()},
	}

	resp, err := o.LLMClient.Chat(ctx, messages, nil)
	if err != nil {
		debug.Warn("orchestrator", "Synthesis LLM call failed, falling back to raw results: %v", err)
		return sb.String(), nil
	}

	debug.Debug("orchestrator", "Synthesis complete, response length: %d chars", len(resp.Content))
	return resp.Content, nil
}

// rawDelegation is a flexible struct for parsing LLM output.
type rawDelegation struct {
	Reasoning      string           `json:"reasoning"`
	Delegations    []rawTaskPayload `json:"delegations"`
	DirectResponse string           `json:"direct_response,omitempty"`
}

type rawTaskPayload struct {
	Agent   string          `json:"agent"`
	Task    string          `json:"task"`
	Context json.RawMessage `json:"context,omitempty"`
}

// parseDelegation extracts the delegation JSON from the LLM response.
func parseDelegation(raw string) (*Delegation, string, error) {
	jsonStr := extractJSON(raw)
	if jsonStr == "" {
		// No JSON found — the LLM responded conversationally.
		// Treat the entire response as a direct answer (common with 30B models).
		cleaned := strings.TrimSpace(raw)
		if cleaned != "" {
			debug.Debug("orchestrator", "No JSON in response — treating as direct reply")
			return nil, cleaned, nil
		}
		return nil, "", fmt.Errorf("empty response from LLM")
	}

	var d rawDelegation
	if err := json.Unmarshal([]byte(jsonStr), &d); err != nil {
		return nil, "", fmt.Errorf("invalid delegation JSON: %w\nraw: %s", err, jsonStr)
	}

	if d.DirectResponse != "" {
		return nil, d.DirectResponse, nil
	}

	payloads := make([]TaskPayload, len(d.Delegations))
	for i, rp := range d.Delegations {
		payloads[i] = TaskPayload{
			Agent:   rp.Agent,
			Task:    rp.Task,
			Context: parseFlexibleContext(rp.Context),
		}
	}

	return &Delegation{
		Reasoning:   d.Reasoning,
		Delegations: payloads,
	}, "", nil
}

// extractJSON finds the first valid JSON object in a string.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	if strings.HasPrefix(s, "{") {
		return s
	}

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return ""
}

// parseFlexibleContext converts a json.RawMessage to a string.
func parseFlexibleContext(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err == nil {
		if len(obj) == 0 {
			return ""
		}
		b, _ := json.Marshal(obj)
		return string(b)
	}

	return string(raw)
}
