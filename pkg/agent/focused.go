package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/David2024patton/GOAgent/pkg/debug"
	"github.com/David2024patton/GOAgent/pkg/eventbus"
	"github.com/David2024patton/GOAgent/pkg/llm"
	"github.com/David2024patton/GOAgent/pkg/memory"
	"github.com/David2024patton/GOAgent/pkg/tool"
)

// focusedSystemPrompt builds the system prompt for a focused agent.
func focusedSystemPrompt(cfg AgentConfig) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are %s.\n", cfg.Name))
	sb.WriteString(fmt.Sprintf("Role: %s\n", cfg.Role))
	sb.WriteString(fmt.Sprintf("Personality: %s\n", cfg.Personality))

	if len(cfg.Goals) > 0 {
		sb.WriteString(fmt.Sprintf("Goals: %s\n", strings.Join(cfg.Goals, ", ")))
	}

	sb.WriteString(`
You are a FOCUSED AGENT in the GOAgent framework.
You receive specific tasks from the Orchestrator and execute them using your tools.

HOW TO WORK:
1. Use your tools to gather the information or perform the action requested.
2. After getting tool results, review what you found.
3. STOP and write your final answer as plain text (NO tool calls).

RULES:
- Stay focused on your assigned task. Do not go beyond scope.
- Use 1-3 tool calls maximum, then REPORT your findings.
- When a tool returns "File not found" or "Directory not found", that IS useful information. Report it.
- Do NOT keep retrying the same tool or similar paths if something isn't found.

IMPORTANT: When you have enough information to answer (even partially), you MUST stop calling tools
and respond with a plain text summary of what you found. NEVER loop more than 3-4 tool calls.
If you cannot find something after 2-3 tries, report what you DID find and note what was missing.
`)
	return sb.String()
}

// NewFocusedAgent creates a focused agent with its own LLM client and tools.
func NewFocusedAgent(cfg AgentConfig, client llm.Client, tools *tool.Registry, mem *memory.Manager, trace *debug.StepLogger, tokens *llm.TokenTracker, bus *eventbus.EventBus, sessionID string) *FocusedAgent {
	if cfg.MaxSkills == 0 {
		cfg.MaxSkills = DefaultMaxSkills
	}
	if cfg.MaxLoops == 0 {
		cfg.MaxLoops = DefaultMaxLoops
	}
	return &FocusedAgent{
		Config:    cfg,
		LLMClient: client,
		Tools:     tools,
		Memory:    mem,
		Trace:     trace,
		Tokens:    tokens,
		Bus:       bus,
		SessionID: sessionID,
	}
}

// Run executes a delegated task through the ReAct loop.
func (a *FocusedAgent) Run(ctx context.Context, task TaskPayload) Result {
	tag := a.Config.Name
	sysPrompt := focusedSystemPrompt(a.Config)

	userMsg := fmt.Sprintf("TASK: %s", task.Task)
	if task.Context != "" {
		userMsg += fmt.Sprintf("\n\nCONTEXT:\n%s", task.Context)
	}

	debug.Info(tag, "Starting task: %s", truncate(task.Task, 100))
	debug.Debug(tag, "System prompt length: %d chars, tools: %v", len(sysPrompt), a.Tools.Names())

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: sysPrompt},
		{Role: llm.RoleUser, Content: userMsg},
	}

	toolDefs := a.Tools.ToolDefs()

	// ReAct loop: reason → act → observe → decide
	for i := 0; i < a.Config.MaxLoops; i++ {
		debug.Info(tag, "Loop %d/%d", i+1, a.Config.MaxLoops)

		resp, err := a.LLMClient.Chat(ctx, messages, toolDefs)
		if err != nil {
			debug.Error(tag, "LLM call failed on loop %d: %v", i+1, err)
			return Result{
				Agent:   a.Config.Name,
				Success: false,
				Error:   fmt.Sprintf("LLM error on loop %d: %v", i+1, err),
			}
		}

		if resp.Usage != nil {
			debug.Debug(tag, "Tokens  -  prompt: %d, completion: %d, total: %d",
				resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
		}

		// If no tool calls, the agent is done.
		if len(resp.ToolCalls) == 0 {
			debug.Info(tag, "✓ Task complete (loop %d)", i+1)
			debug.Debug(tag, "Result: %s", truncate(resp.Content, 300))
			return Result{
				Agent:   a.Config.Name,
				Success: true,
				Output:  resp.Content,
			}
		}

		// Add the assistant's response (with tool calls) to the conversation.
		assistantMsg := llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		// Execute each tool call and add results.
		for _, tc := range resp.ToolCalls {
			debug.Info(tag, "Calling tool %q (id: %s)", tc.Function.Name, tc.ID)
			debug.Debug(tag, "Tool args: %s", truncate(tc.Function.Arguments, 200))

			// Trace + Bus: tool invoked.
			if a.Trace != nil {
				a.Trace.Record(debug.StepToolInvoked, tag, tc.Function.Name, tc.Function.Arguments, "", nil)
			}
			if a.Bus != nil {
				a.Bus.Publish(eventbus.ToolEvent(eventbus.TopicAgentToolCall, tag, tc.Function.Name, truncate(tc.Function.Arguments, 200), nil))
			}

			toolStart := time.Now()
			toolResult := a.executeTool(ctx, tc)
			debug.Debug(tag, "Tool result: %s", truncate(toolResult, 300))

			// Trace + Bus: tool result.
			if a.Trace != nil {
				a.Trace.RecordTimed(debug.StepToolResult, tag, tc.Function.Name, tc.Function.Arguments, toolStart, truncate(toolResult, 500))
			}
			if a.Bus != nil {
				a.Bus.Publish(eventbus.ToolEvent(eventbus.TopicAgentToolResult, tag, tc.Function.Name, truncate(toolResult, 300), map[string]interface{}{
					"duration_ms": time.Since(toolStart).Milliseconds(),
				}))
			}

			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    toolResult,
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
			})
		}
	}

	debug.Warn(tag, "Max loops (%d) reached without completion", a.Config.MaxLoops)
	return Result{
		Agent:   a.Config.Name,
		Success: false,
		Error:   fmt.Sprintf("max iterations (%d) reached without completion", a.Config.MaxLoops),
	}
}

// executeTool runs a single tool call and returns the result string.
func (a *FocusedAgent) executeTool(ctx context.Context, tc llm.ToolCall) string {
	tag := a.Config.Name

	t, ok := a.Tools.Get(tc.Function.Name)
	if !ok {
		debug.Error(tag, "Unknown tool: %q", tc.Function.Name)
		return fmt.Sprintf("ERROR: unknown tool %q. Available tools: %s",
			tc.Function.Name, strings.Join(a.Tools.Names(), ", "))
	}

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		debug.Error(tag, "Invalid tool arguments for %q: %v", tc.Function.Name, err)
		return fmt.Sprintf("ERROR: invalid tool arguments for %q: %v. Expected JSON object.", tc.Function.Name, err)
	}

	result, err := t.Execute(ctx, args)
	if err != nil {
		debug.Error(tag, "Tool %q execution failed: %v", tc.Function.Name, err)
		return fmt.Sprintf("ERROR: tool %q failed: %v", tc.Function.Name, err)
	}

	return result
}

// truncate shortens a string for logging.
func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
