package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/David2024patton/GOAgent/pkg/debug"
	"github.com/David2024patton/GOAgent/pkg/llm"
)

// delegationSystemPrompt is the baked-in system prompt for the orchestrator.
// It tells the LLM to ONLY reason and delegate — never use tools.
const delegationSystemPrompt = `You are the GOAgent Orchestrator.

YOUR RULES:
1. You NEVER use tools directly. You have NO tools.
2. You ONLY reason about the user's request and delegate tasks to focused agents.
3. You give each agent ONLY the information it needs — nothing more.
4. You synthesize results from agents into a final response for the user.

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

If you can answer the user directly without any agent (simple questions), respond:
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
func NewOrchestrator(cfg OrchestratorConfig, agents map[string]*FocusedAgent) *Orchestrator {
	client := llm.NewOpenAIClient(cfg.LLM)
	return &Orchestrator{
		Config:    cfg,
		LLMClient: client,
		Agents:    agents,
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

	// Build the agent descriptions for the system prompt.
	agentDescs := o.buildAgentDescriptions()
	sysPrompt := fmt.Sprintf(delegationSystemPrompt, agentDescs)

	if o.Config.SystemPrompt != "" {
		sysPrompt = o.Config.SystemPrompt + "\n\n" + sysPrompt
	}

	debug.Debug("orchestrator", "System prompt length: %d chars, %d agents available", len(sysPrompt), len(o.Agents))

	// Step 1: Ask the LLM what to delegate.
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: sysPrompt},
		{Role: llm.RoleUser, Content: userMessage},
	}

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

	// If the orchestrator answered directly, return it.
	if directResponse != "" {
		debug.Info("orchestrator", "Direct response (no delegation): %s", truncate(directResponse, 100))
		return directResponse, nil
	}

	if len(delegation.Delegations) == 0 {
		debug.Warn("orchestrator", "No delegations and no direct response — unclear request")
		return "I wasn't sure how to help with that. Could you rephrase?", nil
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
	debug.Info("orchestrator", "Synthesizing %d result(s)...", len(results))
	return o.synthesize(ctx, userMessage, results)
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
		return nil, "", fmt.Errorf("no JSON found in response:\n%s", raw)
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
