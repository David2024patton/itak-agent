package builtins

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/David2024patton/iTaKAgent/pkg/codex"
	"github.com/David2024patton/iTaKDatabase/pkg/itakdb"
)

// ══════════════════════════════════════════════════════════════════
// code_index  -  index a codebase into the knowledge graph
// ══════════════════════════════════════════════════════════════════

type CodeIndexTool struct {
	DB *itakdb.DB
}

func (t *CodeIndexTool) Name() string { return "code_index" }
func (t *CodeIndexTool) Description() string {
	return "Index a Go codebase directory into the knowledge graph. Parses all Go source files, extracts functions, types, interfaces, imports, and call chains. Use before code_query. Args: path (directory to index)."
}
func (t *CodeIndexTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory path to index (e.g. '.' for current project)",
			},
		},
		"required": []string{"path"},
	}
}

func (t *CodeIndexTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}

	if t.DB == nil {
		return "", fmt.Errorf("code_index: database not initialized")
	}

	indexer := codex.NewIndexer(t.DB)
	stats, err := indexer.Index(path)
	if err != nil {
		return "", fmt.Errorf("code_index: %w", err)
	}

	result, _ := json.MarshalIndent(stats, "", "  ")
	return fmt.Sprintf("Codebase indexed successfully:\n%s", string(result)), nil
}

// ══════════════════════════════════════════════════════════════════
// code_query  -  query the code knowledge graph
// ══════════════════════════════════════════════════════════════════

type CodeQueryTool struct {
	DB *itakdb.DB
}

func (t *CodeQueryTool) Name() string { return "code_query" }
func (t *CodeQueryTool) Description() string {
	return "Search the code knowledge graph. Find functions, types, interfaces, imports, and call chains. Use after code_index. Examples: 'Run', 'WebNavigateTool', 'http_fetch', 'interface'. Args: query (symbol name or search term)."
}
func (t *CodeQueryTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Symbol name or search term (e.g. 'Run', 'WebNavigateTool', 'interface')",
			},
		},
		"required": []string{"query"},
	}
}

func (t *CodeQueryTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "", fmt.Errorf("missing required argument: query")
	}

	if t.DB == nil {
		return "", fmt.Errorf("code_query: database not initialized")
	}

	indexer := codex.NewIndexer(t.DB)
	results, err := indexer.Query(query)
	if err != nil {
		return "", fmt.Errorf("code_query: %w", err)
	}

	if len(results) == 0 {
		return fmt.Sprintf("No code symbols found matching %q. Make sure the codebase has been indexed with code_index first.", query), nil
	}

	// Format results nicely.
	var output string
	for i, r := range results {
		if i >= 15 {
			output += fmt.Sprintf("\n... and %d more results", len(results)-15)
			break
		}

		output += fmt.Sprintf("\n[%s] %s", r.Type, r.Name)
		if pkg, ok := r.Properties["package"].(string); ok {
			output += fmt.Sprintf(" (package: %s)", pkg)
		}
		if sig, ok := r.Properties["signature"].(string); ok && sig != "" {
			output += fmt.Sprintf("\n  Signature: %s", sig)
		}
		if filePath, ok := r.Properties["file_path"].(string); ok {
			line, _ := r.Properties["line"].(float64)
			output += fmt.Sprintf("\n  Location: %s:%d", filePath, int(line))
		}
		if doc, ok := r.Properties["doc"].(string); ok && doc != "" {
			if len(doc) > 200 {
				doc = doc[:200] + "..."
			}
			output += fmt.Sprintf("\n  Doc: %s", doc)
		}
		output += "\n"
	}

	return fmt.Sprintf("Found %d results for %q:%s", len(results), query, output), nil
}
