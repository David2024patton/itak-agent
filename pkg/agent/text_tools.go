package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/llm"
	"github.com/David2024patton/iTaKAgent/pkg/tool"
)

// toolDescriptionsForPrompt generates a text description of all tools
// for models that don't support OpenAI-style structured tool definitions.
// The description is appended to the system prompt so the model knows
// what tools it can call using JSON text format.
func toolDescriptionsForPrompt(reg *tool.Registry) string {
	var sb strings.Builder
	sb.WriteString("\n\nAVAILABLE TOOLS:\n")

	for _, name := range reg.Names() {
		t, ok := reg.Get(name)
		if !ok {
			continue
		}

		sb.WriteString(fmt.Sprintf("\n- %s: %s\n", t.Name(), t.Description()))

		// List required parameters.
		schema := t.Schema()
		if props, ok := schema["properties"].(map[string]interface{}); ok {
			sb.WriteString("  Parameters:\n")
			for pName, pDef := range props {
				desc := ""
				if pMap, ok := pDef.(map[string]interface{}); ok {
					if d, ok := pMap["description"].(string); ok {
						desc = " - " + d
					}
				}
				sb.WriteString(fmt.Sprintf("    - %s%s\n", pName, desc))
			}
		}
	}

	sb.WriteString("\nCall a tool by writing: {\"tool\": \"tool_name\", \"arguments\": {\"param\": \"value\"}}\n")
	return sb.String()
}

// parseTextToolCalls tries to extract tool calls from the model's text content.
// Small models (1-4B params) often write tool calls as JSON in their response
// text instead of using the structured tool_calls field. This parser catches
// those cases so the agent still works with Ollama and other local models.
//
// Supported formats:
//   1. {"tool": "shell", "arguments": {"command": "ls"}}
//   2. {"name": "shell", "arguments": {"command": "ls"}}
//   3. [{"tool": "shell", ...}, {"tool": "file_read", ...}]
//   4. <tool_call>{"name": "shell", ...}</tool_call>
//   5. ```json\n{"tool": "shell", ...}\n```
func parseTextToolCalls(content string, availableTools []string) []llm.ToolCall {
	if content == "" {
		return nil
	}

	// Strip thinking tags (Qwen3 wraps reasoning in <think>...</think>).
	content = stripThinkingTags(content)
	if content == "" {
		return nil
	}

	// Build a set for fast lookup.
	toolSet := make(map[string]bool, len(availableTools))
	for _, t := range availableTools {
		toolSet[t] = true
	}

	// Try multiple extraction strategies.
	var calls []llm.ToolCall

	// Strategy 1: Look for <tool_call>...</tool_call> tags.
	calls = extractTaggedToolCalls(content, toolSet)
	if len(calls) > 0 {
		return calls
	}

	// Strategy 2: Look for JSON array of tool calls.
	calls = extractJSONArrayToolCalls(content, toolSet)
	if len(calls) > 0 {
		return calls
	}

	// Strategy 3: Look for a single JSON object with a tool name.
	calls = extractSingleJSONToolCall(content, toolSet)
	if len(calls) > 0 {
		return calls
	}

	return nil
}

// textToolCall is the flexible struct for parsing text-based tool calls.
// Supports both "tool" and "name" keys, and "arguments"/"parameters"/"args".
type textToolCall struct {
	Tool       string                 `json:"tool"`
	Name       string                 `json:"name"`
	Arguments  map[string]interface{} `json:"arguments"`
	Parameters map[string]interface{} `json:"parameters"`
	Args       map[string]interface{} `json:"args"`
}

// resolve returns the tool name and arguments, handling field aliases.
func (t textToolCall) resolve() (string, map[string]interface{}) {
	name := t.Tool
	if name == "" {
		name = t.Name
	}
	args := t.Arguments
	if args == nil {
		args = t.Parameters
	}
	if args == nil {
		args = t.Args
	}
	if args == nil {
		args = make(map[string]interface{})
	}
	return name, args
}

// toToolCall converts to the standard llm.ToolCall format.
func (t textToolCall) toToolCall(id int) llm.ToolCall {
	name, args := t.resolve()
	argsJSON, _ := json.Marshal(args)
	return llm.ToolCall{
		ID:   fmt.Sprintf("text_call_%d", id),
		Type: "function",
		Function: llm.FunctionCall{
			Name:      name,
			Arguments: string(argsJSON),
		},
	}
}

// extractTaggedToolCalls handles <tool_call>JSON</tool_call> format.
func extractTaggedToolCalls(content string, toolSet map[string]bool) []llm.ToolCall {
	var calls []llm.ToolCall
	remaining := content
	id := 0

	for {
		start := strings.Index(remaining, "<tool_call>")
		if start < 0 {
			break
		}
		end := strings.Index(remaining[start:], "</tool_call>")
		if end < 0 {
			break
		}

		jsonStr := remaining[start+len("<tool_call>") : start+end]
		jsonStr = strings.TrimSpace(jsonStr)

		var tc textToolCall
		if err := json.Unmarshal([]byte(jsonStr), &tc); err == nil {
			name, _ := tc.resolve()
			if toolSet[name] {
				calls = append(calls, tc.toToolCall(id))
				id++
				debug.Debug("text_tools", "Extracted tagged tool call: %s", name)
			}
		}

		remaining = remaining[start+end+len("</tool_call>"):]
	}

	return calls
}

// extractJSONArrayToolCalls handles [{"tool": "...", ...}, ...] format.
func extractJSONArrayToolCalls(content string, toolSet map[string]bool) []llm.ToolCall {
	// Find JSON array in content.
	arrStart := strings.Index(content, "[")
	arrEnd := strings.LastIndex(content, "]")
	if arrStart < 0 || arrEnd <= arrStart {
		return nil
	}

	jsonStr := content[arrStart : arrEnd+1]
	var arr []textToolCall
	if err := json.Unmarshal([]byte(jsonStr), &arr); err != nil {
		return nil
	}

	var calls []llm.ToolCall
	for i, tc := range arr {
		name, _ := tc.resolve()
		if name != "" && toolSet[name] {
			calls = append(calls, tc.toToolCall(i))
			debug.Debug("text_tools", "Extracted array tool call [%d]: %s", i, name)
		}
	}
	return calls
}

// extractSingleJSONToolCall handles a single {"tool": "...", "arguments": {...}} in content.
func extractSingleJSONToolCall(content string, toolSet map[string]bool) []llm.ToolCall {
	// Strip markdown code fences.
	cleaned := strings.TrimSpace(content)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	// Find JSON object.
	start := strings.Index(cleaned, "{")
	end := strings.LastIndex(cleaned, "}")
	if start < 0 || end <= start {
		return nil
	}
	jsonStr := cleaned[start : end+1]

	var tc textToolCall
	if err := json.Unmarshal([]byte(jsonStr), &tc); err != nil {
		return nil
	}

	name, _ := tc.resolve()
	if name == "" || !toolSet[name] {
		return nil
	}

	debug.Debug("text_tools", "Extracted single JSON tool call: %s", name)
	return []llm.ToolCall{tc.toToolCall(0)}
}

// stripThinkingTags removes Qwen3-style <think>...</think> reasoning blocks
// from model output. These thinking blocks contain chain-of-thought reasoning
// that should not be parsed as tool calls. Handles both single and multiline tags.
func stripThinkingTags(content string) string {
	result := content
	for {
		start := strings.Index(result, "<think>")
		if start < 0 {
			break
		}
		end := strings.Index(result, "</think>")
		if end < 0 {
			// Unclosed think tag -- remove from <think> to end of string.
			result = result[:start]
			break
		}
		// Remove the think block.
		result = result[:start] + result[end+len("</think>"):]
	}
	return strings.TrimSpace(result)
}
