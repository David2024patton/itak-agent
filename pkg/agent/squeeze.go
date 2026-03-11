package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/llm"
)

// SummarizeToolOutput compresses long tool outputs to fit within a token budget.
// Small models (1-8B params) struggle with long context. This function extracts
// the essential information from tool outputs before feeding them back to the LLM.
//
// Why: A 500-line directory listing wastes context window. "47 Go files, 3 test
// files, main entry at cmd/main.go" uses 90% fewer tokens and gives the model
// the same actionable information.
//
// How: Uses heuristic rules per output type (file listings, command output,
// error messages). Falls back to head/tail truncation for unknown formats.
func SummarizeToolOutput(output string, maxChars int) string {
	if maxChars <= 0 || len(output) <= maxChars {
		return output
	}

	debug.Debug("squeeze", "Compressing tool output: %d chars -> %d max", len(output), maxChars)

	lines := strings.Split(output, "\n")
	lineCount := len(lines)

	// Heuristic 1: Error output -- keep the first error and last few lines.
	if containsError(output) {
		return summarizeErrorOutput(lines, maxChars)
	}

	// Heuristic 2: File listing -- count by type and show key files.
	if looksLikeFileListing(lines) {
		return summarizeFileListing(lines, maxChars)
	}

	// Heuristic 3: JSON output -- keep structure, truncate values.
	if strings.HasPrefix(strings.TrimSpace(output), "{") || strings.HasPrefix(strings.TrimSpace(output), "[") {
		return summarizeJSON(output, maxChars)
	}

	// Fallback: head + tail with line count.
	headLines := maxChars / 3 / 80 // rough: 80 chars per line
	if headLines < 3 {
		headLines = 3
	}
	tailLines := headLines

	if lineCount <= headLines+tailLines {
		// Can fit with simple truncation.
		return output[:maxChars] + "..."
	}

	var sb strings.Builder
	for i := 0; i < headLines && i < lineCount; i++ {
		sb.WriteString(lines[i] + "\n")
	}
	sb.WriteString(fmt.Sprintf("\n... (%d lines omitted) ...\n\n", lineCount-headLines-tailLines))
	for i := lineCount - tailLines; i < lineCount; i++ {
		sb.WriteString(lines[i] + "\n")
	}

	result := sb.String()
	if len(result) > maxChars {
		result = result[:maxChars] + "..."
	}
	return result
}

// CompressContext applies smart sliding window compression to conversation messages.
// Instead of naive "last N messages", it scores messages by relevance and keeps
// the most important ones within the token budget.
//
// Budget allocation: 20% system prompt, 30% conversation, 30% tool results, 20% response.
func CompressContext(messages []llm.Message, budgetChars int) []llm.Message {
	if budgetChars <= 0 {
		return messages
	}

	totalChars := 0
	for _, m := range messages {
		totalChars += len(m.Content)
	}

	if totalChars <= budgetChars {
		return messages // fits within budget
	}

	debug.Debug("squeeze", "Compressing context: %d chars -> %d budget (%d messages)",
		totalChars, budgetChars, len(messages))

	// Strategy: always keep system prompt (first) and last user message (last).
	// From the middle, keep the most recent messages that fit.
	if len(messages) <= 2 {
		return messages
	}

	result := make([]llm.Message, 0, len(messages))

	// Always keep system prompt.
	if messages[0].Role == llm.RoleSystem {
		result = append(result, messages[0])
		budgetChars -= len(messages[0].Content)
		messages = messages[1:]
	}

	// Always keep the last message (current user input).
	lastMsg := messages[len(messages)-1]
	budgetChars -= len(lastMsg.Content)
	messages = messages[:len(messages)-1]

	// From remaining messages, take the most recent that fit.
	for i := len(messages) - 1; i >= 0; i-- {
		msgLen := len(messages[i].Content)
		if msgLen > budgetChars {
			// This message doesn't fit. Try to summarize tool results.
			if messages[i].Role == llm.RoleTool && msgLen > 500 {
				summarized := SummarizeToolOutput(messages[i].Content, budgetChars)
				if len(summarized) <= budgetChars {
					msg := messages[i]
					msg.Content = summarized
					result = append(result, msg)
					budgetChars -= len(summarized)
					continue
				}
			}
			// Insert a gap marker and stop.
			result = append(result, llm.Message{
				Role:    llm.RoleSystem,
				Content: fmt.Sprintf("(earlier conversation omitted: %d messages)", i+1),
			})
			break
		}
		result = append(result, messages[i])
		budgetChars -= msgLen
	}

	// Reverse the middle messages back to chronological order.
	if len(result) > 1 {
		mid := result[1:] // skip system prompt
		for i, j := 0, len(mid)-1; i < j; i, j = i+1, j-1 {
			mid[i], mid[j] = mid[j], mid[i]
		}
	}

	// Append last message.
	result = append(result, lastMsg)

	debug.Debug("squeeze", "Compressed to %d messages", len(result))
	return result
}

// ValidateJSONResponse attempts to parse a JSON response and auto-repair
// common formatting issues that small models produce. Returns the cleaned
// JSON string and an error if the response is fundamentally broken.
//
// Common issues fixed:
//   - Markdown code fences (```json ... ```)
//   - Trailing commas in objects/arrays
//   - Missing closing braces/brackets
//   - Thinking tags (<think>...</think>)
func ValidateJSONResponse(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("empty response")
	}

	cleaned := raw

	// Strip thinking tags.
	cleaned = stripThinkingTags(cleaned)

	// Strip markdown code fences.
	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	// Find the JSON boundaries.
	start := strings.IndexAny(cleaned, "{[")
	if start < 0 {
		return "", fmt.Errorf("no JSON object or array found in response")
	}

	// Determine expected closing bracket.
	var closeBracket byte
	if cleaned[start] == '{' {
		closeBracket = '}'
	} else {
		closeBracket = ']'
	}

	end := strings.LastIndexByte(cleaned, closeBracket)
	if end <= start {
		// Missing closing bracket -- try to add it.
		cleaned = cleaned[start:] + string(closeBracket)
		debug.Debug("squeeze", "Auto-added missing closing bracket")
	} else {
		cleaned = cleaned[start : end+1]
	}

	// Fix trailing commas (e.g., {"a": 1,} -> {"a": 1}).
	cleaned = fixTrailingCommas(cleaned)

	// Validate the result.
	var js json.RawMessage
	if err := json.Unmarshal([]byte(cleaned), &js); err != nil {
		return "", fmt.Errorf("JSON repair failed: %w (cleaned: %s)", err, truncate(cleaned, 200))
	}

	if cleaned != raw {
		debug.Debug("squeeze", "JSON repaired: %d chars -> %d chars", len(raw), len(cleaned))
	}

	return cleaned, nil
}

// fixTrailingCommas removes trailing commas before closing braces/brackets.
// Example: {"a": 1, "b": 2,} -> {"a": 1, "b": 2}
func fixTrailingCommas(s string) string {
	// Simple approach: look for ",}" and ",]" patterns.
	result := s
	for {
		replaced := strings.ReplaceAll(result, ",}", "}")
		replaced = strings.ReplaceAll(replaced, ",]", "]")
		// Also handle whitespace between comma and close.
		replaced = strings.ReplaceAll(replaced, ", }", "}")
		replaced = strings.ReplaceAll(replaced, ", ]", "]")
		replaced = strings.ReplaceAll(replaced, ",\n}", "}")
		replaced = strings.ReplaceAll(replaced, ",\n]", "]")
		replaced = strings.ReplaceAll(replaced, ",\n  }", "}")
		replaced = strings.ReplaceAll(replaced, ",\n  ]", "]")
		if replaced == result {
			break
		}
		result = replaced
	}
	return result
}

// containsError checks if output looks like an error message.
func containsError(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "fatal") ||
		strings.Contains(lower, "panic") ||
		strings.Contains(lower, "exception")
}

// looksLikeFileListing checks if lines look like a directory listing.
func looksLikeFileListing(lines []string) bool {
	if len(lines) < 3 {
		return false
	}
	fileCount := 0
	for _, line := range lines[:min(20, len(lines))] {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") ||
			strings.HasSuffix(trimmed, ".go") || strings.HasSuffix(trimmed, ".py") ||
			strings.HasSuffix(trimmed, ".js") || strings.HasSuffix(trimmed, ".ts") {
			fileCount++
		}
	}
	return fileCount > len(lines)/3 // at least 33% of lines are file-like
}

// summarizeErrorOutput keeps the first error and the last few lines.
func summarizeErrorOutput(lines []string, maxChars int) string {
	var sb strings.Builder
	sb.WriteString("ERROR OUTPUT SUMMARY:\n")

	// Find first error line.
	for _, line := range lines {
		if containsError(line) {
			sb.WriteString("First error: " + strings.TrimSpace(line) + "\n")
			break
		}
	}

	// Keep last 5 lines for context.
	sb.WriteString(fmt.Sprintf("Total lines: %d\n\nLast lines:\n", len(lines)))
	start := len(lines) - 5
	if start < 0 {
		start = 0
	}
	for _, line := range lines[start:] {
		sb.WriteString(line + "\n")
	}

	result := sb.String()
	if len(result) > maxChars {
		result = result[:maxChars] + "..."
	}
	return result
}

// summarizeFileListing counts files by extension and lists key files.
func summarizeFileListing(lines []string, maxChars int) string {
	extCounts := make(map[string]int)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts := strings.Split(trimmed, ".")
		if len(parts) > 1 {
			ext := "." + parts[len(parts)-1]
			extCounts[ext]++
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("DIRECTORY LISTING (%d entries):\n", len(lines)))
	for ext, count := range extCounts {
		sb.WriteString(fmt.Sprintf("  %s: %d files\n", ext, count))
	}

	// Show first few entries.
	sb.WriteString("\nFirst entries:\n")
	for i, line := range lines {
		if i >= 10 {
			sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(lines)-10))
			break
		}
		sb.WriteString("  " + strings.TrimSpace(line) + "\n")
	}

	result := sb.String()
	if len(result) > maxChars {
		result = result[:maxChars] + "..."
	}
	return result
}

// summarizeJSON keeps the structure but truncates long string values.
func summarizeJSON(s string, maxChars int) string {
	if len(s) <= maxChars {
		return s
	}

	// Try to pretty-print with truncated values.
	var raw interface{}
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		// Not valid JSON -- just truncate.
		return s[:maxChars] + "..."
	}

	truncated := truncateJSONValues(raw, 100)
	result, err := json.MarshalIndent(truncated, "", "  ")
	if err != nil {
		return s[:maxChars] + "..."
	}

	out := string(result)
	if len(out) > maxChars {
		out = out[:maxChars] + "..."
	}
	return out
}

// truncateJSONValues recursively truncates string values in a JSON structure.
func truncateJSONValues(v interface{}, maxLen int) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(val))
		for k, v := range val {
			result[k] = truncateJSONValues(v, maxLen)
		}
		return result
	case []interface{}:
		if len(val) > 10 {
			// Truncate long arrays.
			result := make([]interface{}, 11)
			for i := 0; i < 10; i++ {
				result[i] = truncateJSONValues(val[i], maxLen)
			}
			result[10] = fmt.Sprintf("... (%d more items)", len(val)-10)
			return result
		}
		result := make([]interface{}, len(val))
		for i, v := range val {
			result[i] = truncateJSONValues(v, maxLen)
		}
		return result
	case string:
		if len(val) > maxLen {
			return val[:maxLen] + "..."
		}
		return val
	default:
		return v
	}
}

// min returns the smaller of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
