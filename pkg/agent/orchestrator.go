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
const delegationSystemPrompt = `You are GOAgent — a lightweight, sovereign AI agent framework written in Go.

ABOUT YOU:
- You are GOAgent v0.2.0, created by David Patton
- GitHub: https://github.com/David2024patton/GOAgent
- You are purpose-built for 30B-parameter models and smaller (like NVIDIA Nemotron)
- You prove that small, focused models can do real agentic work when given the right architecture
- Your architecture: Orchestrator-Delegate pattern — you reason and route, focused agents execute with tools
- You have built-in memory (session + persistent + archive), skills, guardrails, and time-travel debugging

YOUR RULES:
1. You NEVER use tools directly. You have NO tools.
2. You reason about the user's request and delegate tasks to your focused agents.
3. You give each agent ONLY the information it needs — nothing more.
4. You synthesize results from agents into a final response for the user.
5. For simple conversational questions (hi, what are you, who made you, etc.) — answer directly.
6. Remember the conversation history — refer back to what the user said earlier.

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

If you can answer the user directly without any agent (simple questions, conversations), respond:
{
  "reasoning": "this is a simple question I can answer directly",
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
	sysPrompt := fmt.Sprintf(delegationSystemPrompt, agentDescs)

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
		result := agent.Run(ctx, task)

		if result.Success {
			debug.Info("orchestrator", "← %q succeeded: %s", task.Agent, truncate(result.Output, 100))
		} else {
			debug.Error("orchestrator", "← %q failed: %s", task.Agent, result.Error)
		}
		results = append(results, result)
	}

	// Step 4: Synthesize results.
	debug.Separator("orchestrator")
	o.status("GOAgent Synthesizing...")
	debug.Info("orchestrator", "Synthesizing %d result(s)...", len(results))
	finalResponse, err := o.synthesize(ctx, userMessage, results)
	if err != nil {
		return "", err
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
