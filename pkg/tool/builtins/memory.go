package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/David2024patton/GOAgent/pkg/memory"
)

// ─── memory_save ───────────────────────────────────────────────────

// MemorySaveTool lets agents store facts for later recall.
type MemorySaveTool struct {
	Manager *memory.Manager
}

func (t *MemorySaveTool) Name() string { return "memory_save" }

func (t *MemorySaveTool) Description() string {
	return "Save a fact or piece of information to persistent memory. Use this to remember things for future sessions — user preferences, learned facts, project context, etc."
}

func (t *MemorySaveTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "Short identifier for this fact (e.g., 'user_name', 'vps_ip')",
			},
			"value": map[string]interface{}{
				"type":        "string",
				"description": "The fact to remember",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Category: user_prefs, learned_facts, project_context, or credentials",
			},
			"importance": map[string]interface{}{
				"type":        "integer",
				"description": "Importance 1-10 (default 5). Higher = more important to remember",
			},
		},
		"required": []string{"key", "value"},
	}
}

func (t *MemorySaveTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	key, _ := args["key"].(string)
	value, _ := args["value"].(string)
	if key == "" || value == "" {
		return "", fmt.Errorf("missing required arguments: key and value")
	}

	category := "learned_facts"
	if c, ok := args["category"].(string); ok && c != "" {
		category = c
	}

	importance := 5
	if imp, ok := args["importance"].(float64); ok {
		importance = int(imp)
	}

	if err := t.Manager.Facts.Save(key, value, category, importance); err != nil {
		return "", fmt.Errorf("save failed: %w", err)
	}

	return fmt.Sprintf("Saved to memory: %s = %s [category: %s, importance: %d]", key, value, category, importance), nil
}

// ─── memory_recall ─────────────────────────────────────────────────

// MemoryRecallTool lets agents search stored facts.
type MemoryRecallTool struct {
	Manager *memory.Manager
}

func (t *MemoryRecallTool) Name() string { return "memory_recall" }

func (t *MemoryRecallTool) Description() string {
	return "Search persistent memory for stored facts. Returns matching facts by key, value, or category. Use this to recall information saved in previous sessions."
}

func (t *MemoryRecallTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query — matches against fact keys, values, and categories",
			},
		},
		"required": []string{"query"},
	}
}

func (t *MemoryRecallTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "", fmt.Errorf("missing required argument: query")
	}

	facts := t.Manager.Facts.Recall(query)
	if len(facts) == 0 {
		return "No matching facts found in memory.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d matching fact(s):\n\n", len(facts)))
	for _, f := range facts {
		sb.WriteString(fmt.Sprintf("• [%s] %s = %s (importance: %d)\n", f.Category, f.Key, f.Value, f.Importance))
	}
	return sb.String(), nil
}

// ─── conversation_search ───────────────────────────────────────────

// ConversationSearchTool lets agents search past conversation archives.
type ConversationSearchTool struct {
	Manager *memory.Manager
}

func (t *ConversationSearchTool) Name() string { return "conversation_search" }

func (t *ConversationSearchTool) Description() string {
	return "Search past conversation archives by topic, entity, or keyword. Returns conversation summaries. Use conversation_read to load the full transcript of a specific conversation."
}

func (t *ConversationSearchTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query — matches against conversation titles, summaries, tags, and entities",
			},
		},
		"required": []string{"query"},
	}
}

func (t *ConversationSearchTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "", fmt.Errorf("missing required argument: query")
	}

	results := t.Manager.Archive.Search(query)
	if len(results) == 0 {
		return "No matching conversations found in the archive.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d conversation(s):\n\n", len(results)))
	for _, c := range results {
		sb.WriteString(fmt.Sprintf("─── Conversation #%d: %s ───\n", c.ID, c.Title))
		sb.WriteString(fmt.Sprintf("  Date: %s\n", c.Timestamp.Format("2006-01-02 15:04")))
		sb.WriteString(fmt.Sprintf("  Messages: %d\n", c.MessageCount))
		sb.WriteString(fmt.Sprintf("  Summary: %s\n", c.Summary))
		if len(c.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("  Tags: %s\n", strings.Join(c.Tags, ", ")))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("Use conversation_read with the conversation ID# to load the full transcript.")
	return sb.String(), nil
}

// ─── conversation_read ─────────────────────────────────────────────

// ConversationReadTool lets agents load full conversation transcripts.
type ConversationReadTool struct {
	Manager *memory.Manager
}

func (t *ConversationReadTool) Name() string { return "conversation_read" }

func (t *ConversationReadTool) Description() string {
	return "Load the full transcript of a past conversation by its ID number. Use conversation_search first to find the right ID."
}

func (t *ConversationReadTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "integer",
				"description": "The conversation ID number (from conversation_search results)",
			},
		},
		"required": []string{"id"},
	}
}

func (t *ConversationReadTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	idFloat, ok := args["id"].(float64)
	if !ok {
		return "", fmt.Errorf("missing required argument: id (integer)")
	}
	id := int(idFloat)

	messages, err := t.Manager.Archive.LoadConversation(id)
	if err != nil {
		return "", fmt.Errorf("load conversation: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("─── Full Transcript: Conversation #%d (%d messages) ───\n\n", id, len(messages)))
	for _, msg := range messages {
		prefix := msg.Role
		if msg.Agent != "" {
			prefix = fmt.Sprintf("%s (%s)", msg.Role, msg.Agent)
		}
		if msg.Tool != "" {
			prefix = fmt.Sprintf("tool:%s", msg.Tool)
		}
		sb.WriteString(fmt.Sprintf("[%s] %s:\n%s\n\n", msg.Timestamp.Format("15:04:05"), prefix, msg.Content))
	}

	// Truncate if too long for context.
	result := sb.String()
	if len(result) > 50000 {
		result = result[:50000] + "\n\n...(transcript truncated at 50KB)"
	}
	return result, nil
}

// ─── entity tools (internal helper, not a standalone tool) ─────────

// FormatEntities returns a formatted string of all tracked entities for injection into prompts.
func FormatEntities(mgr *memory.Manager) string {
	entities := mgr.Entities.All()
	if len(entities) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("KNOWN ENTITIES:\n")
	for _, e := range entities {
		sb.WriteString(fmt.Sprintf("• %s (%s)", e.Name, e.Type))
		if len(e.Attributes) > 0 {
			attrs, _ := json.Marshal(e.Attributes)
			sb.WriteString(fmt.Sprintf(" — %s", string(attrs)))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// FormatRecentReflections returns recent reflections for an agent.
func FormatRecentReflections(mgr *memory.Manager, agent string) string {
	reflections := mgr.Reflections.RecentForAgent(agent, 3)
	if len(reflections) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("PAST LESSONS LEARNED:\n")
	for _, r := range reflections {
		sb.WriteString(fmt.Sprintf("• [%s] Task: %s → %s\n  Lesson: %s\n",
			r.Timestamp.Format("Jan 2"), r.TaskSummary, r.Outcome, r.Lessons))
	}
	return sb.String()
}
