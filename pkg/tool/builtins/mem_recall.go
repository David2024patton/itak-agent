package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/David2024patton/iTaKAgent/pkg/memory"
)

// ── mem_grep ───────────────────────────────────────────────────────

// MemGrepTool searches all messages and summaries in the current session
// for a query string. Lets agents recall details from compacted history.
type MemGrepTool struct {
	Compaction *memory.CompactionEngine
	Archive    *memory.JSONArchive
	SessionID  int
}

func (t *MemGrepTool) Name() string { return "mem_grep" }
func (t *MemGrepTool) Description() string {
	return "Search through conversation history (including compacted summaries) for a keyword or phrase. Returns matching content from raw messages and summary nodes."
}
func (t *MemGrepTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The search term or phrase to find in conversation history",
			},
		},
		"required": []string{"query"},
	}
}

func (t *MemGrepTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "Error: query is required", nil
	}

	var results []map[string]interface{}
	queryLower := strings.ToLower(query)

	// Search summary nodes.
	if t.Compaction != nil {
		matches := t.Compaction.GrepSummaries(query)
		for _, m := range matches {
			results = append(results, map[string]interface{}{
				"type":    "summary",
				"id":      m.ID,
				"depth":   m.Depth,
				"content": highlightMatch(m.Content, queryLower, 200),
			})
		}
	}

	// Search raw messages.
	if t.Archive != nil && t.SessionID > 0 {
		msgs, err := t.Archive.LoadConversation(t.SessionID)
		if err == nil {
			for i, msg := range msgs {
				if strings.Contains(strings.ToLower(msg.Content), queryLower) {
					results = append(results, map[string]interface{}{
						"type":    "message",
						"index":   i,
						"role":    msg.Role,
						"content": highlightMatch(msg.Content, queryLower, 200),
					})
				}
			}
		}
	}

	if len(results) == 0 {
		return fmt.Sprintf("No results found for %q in conversation history.", query), nil
	}

	// Cap results.
	if len(results) > 10 {
		results = results[:10]
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	return fmt.Sprintf("Found %d match(es) for %q:\n%s", len(results), query, string(data)), nil
}

// ── mem_describe ───────────────────────────────────────────────────

// MemDescribeTool returns metadata and a content preview for a summary node.
type MemDescribeTool struct {
	Compaction *memory.CompactionEngine
}

func (t *MemDescribeTool) Name() string { return "mem_describe" }
func (t *MemDescribeTool) Description() string {
	return "Get metadata and a content preview for a specific summary node by its ID. Use this to understand what a summary covers before deciding to expand it."
}
func (t *MemDescribeTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"summary_id": map[string]interface{}{
				"type":        "string",
				"description": "The summary node ID (e.g., sum_a1b2c3d4)",
			},
		},
		"required": []string{"summary_id"},
	}
}

func (t *MemDescribeTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	summaryID, _ := args["summary_id"].(string)
	if summaryID == "" {
		return "Error: summary_id is required", nil
	}

	if t.Compaction == nil {
		return "No compaction engine available.", nil
	}

	node := t.Compaction.NodeByID(summaryID)
	if node == nil {
		return fmt.Sprintf("Summary %q not found.", summaryID), nil
	}

	info := map[string]interface{}{
		"id":          node.ID,
		"depth":       node.Depth,
		"token_count": node.TokenCount,
		"earliest_at": node.EarliestAt.Format("2006-01-02 15:04"),
		"latest_at":   node.LatestAt.Format("2006-01-02 15:04"),
		"created_at":  node.CreatedAt.Format("2006-01-02 15:04"),
	}

	if node.Depth == 0 {
		info["source_message_count"] = len(node.SourceMsgSeqs)
		info["source_message_range"] = fmt.Sprintf("%d-%d", node.SourceMsgSeqs[0], node.SourceMsgSeqs[len(node.SourceMsgSeqs)-1])
	} else {
		info["parent_summary_count"] = len(node.ParentIDs)
		info["parent_ids"] = node.ParentIDs
	}

	// Content preview (first 500 chars).
	preview := node.Content
	if len(preview) > 500 {
		preview = preview[:500] + "..."
	}
	info["content_preview"] = preview

	data, _ := json.MarshalIndent(info, "", "  ")
	return string(data), nil
}

// ── mem_expand ─────────────────────────────────────────────────────

// MemExpandTool drills down into a summary node. For leaf summaries,
// returns the original raw messages. For condensed summaries, returns
// the child summaries that were merged into it.
type MemExpandTool struct {
	Compaction *memory.CompactionEngine
	Archive    *memory.JSONArchive
	SessionID  int
}

func (t *MemExpandTool) Name() string { return "mem_expand" }
func (t *MemExpandTool) Description() string {
	return "Drill down into a summary to see its source material. For leaf summaries, returns the original raw messages. For condensed summaries, returns the child summaries."
}
func (t *MemExpandTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"summary_id": map[string]interface{}{
				"type":        "string",
				"description": "The summary node ID to expand (e.g., sum_a1b2c3d4)",
			},
		},
		"required": []string{"summary_id"},
	}
}

func (t *MemExpandTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	summaryID, _ := args["summary_id"].(string)
	if summaryID == "" {
		return "Error: summary_id is required", nil
	}

	if t.Compaction == nil {
		return "No compaction engine available.", nil
	}

	node := t.Compaction.NodeByID(summaryID)
	if node == nil {
		return fmt.Sprintf("Summary %q not found.", summaryID), nil
	}

	if node.Depth == 0 {
		// Leaf node: return the original raw messages.
		if t.Archive == nil || t.SessionID <= 0 {
			return "Archive not available to retrieve raw messages.", nil
		}

		allMsgs, err := t.Archive.LoadConversation(t.SessionID)
		if err != nil {
			return fmt.Sprintf("Failed to load conversation: %v", err), nil
		}

		var expanded []map[string]interface{}
		for _, seq := range node.SourceMsgSeqs {
			if seq < len(allMsgs) {
				expanded = append(expanded, map[string]interface{}{
					"index":   seq,
					"role":    allMsgs[seq].Role,
					"content": allMsgs[seq].Content,
				})
			}
		}

		data, _ := json.MarshalIndent(expanded, "", "  ")
		return fmt.Sprintf("Expanded leaf summary %s (%d original messages):\n%s",
			summaryID, len(expanded), string(data)), nil

	}

	// Condensed node: return child summaries.
	var children []map[string]interface{}
	for _, pid := range node.ParentIDs {
		child := t.Compaction.NodeByID(pid)
		if child != nil {
			preview := child.Content
			if len(preview) > 300 {
				preview = preview[:300] + "..."
			}
			children = append(children, map[string]interface{}{
				"id":          child.ID,
				"depth":       child.Depth,
				"token_count": child.TokenCount,
				"content":     preview,
			})
		}
	}

	data, _ := json.MarshalIndent(children, "", "  ")
	return fmt.Sprintf("Expanded condensed summary %s (%d child summaries):\n%s",
		summaryID, len(children), string(data)), nil
}

// ── Helpers ────────────────────────────────────────────────────────

// highlightMatch extracts a window around the first occurrence of query in text.
func highlightMatch(text, queryLower string, windowSize int) string {
	textLower := strings.ToLower(text)
	idx := strings.Index(textLower, queryLower)
	if idx < 0 {
		if len(text) > windowSize {
			return text[:windowSize] + "..."
		}
		return text
	}

	// Extract a window around the match.
	start := idx - windowSize/2
	if start < 0 {
		start = 0
	}
	end := idx + len(queryLower) + windowSize/2
	if end > len(text) {
		end = len(text)
	}

	snippet := text[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(text) {
		snippet = snippet + "..."
	}
	return snippet
}
