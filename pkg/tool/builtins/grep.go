package builtins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// ── grep_search - recursive text search across files ──────────────

// GrepSearchTool searches for text patterns in files recursively.
type GrepSearchTool struct{}

func (g *GrepSearchTool) Name() string { return "grep_search" }

func (g *GrepSearchTool) Description() string {
	return "Search for text patterns in files recursively. Returns matching lines with file paths and line numbers."
}

func (g *GrepSearchTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Text pattern or regex to search for",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory or file to search in (default: current directory)",
			},
			"extensions": map[string]interface{}{
				"type":        "string",
				"description": "Comma-separated file extensions to include (e.g. 'go,py,js'). Empty = all files.",
			},
			"ignore_case": map[string]interface{}{
				"type":        "boolean",
				"description": "Case-insensitive search (default: false)",
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of matching lines to return (default: 50)",
			},
		},
		"required": []string{"pattern"},
	}
}

func (g *GrepSearchTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		return "ERROR: 'pattern' is required", nil
	}

	searchPath, _ := args["path"].(string)
	if searchPath == "" {
		searchPath = "."
	}

	extFilter, _ := args["extensions"].(string)
	ignoreCase, _ := args["ignore_case"].(bool)

	maxResults := 50
	if mr, ok := args["max_results"].(float64); ok && mr > 0 {
		maxResults = int(mr)
	}

	// Build regex.
	flags := ""
	if ignoreCase {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + pattern)
	if err != nil {
		// Fall back to literal search if regex is invalid.
		escaped := regexp.QuoteMeta(pattern)
		re, _ = regexp.Compile(flags + escaped)
	}

	// Parse extension filter.
	allowedExts := make(map[string]bool)
	if extFilter != "" {
		for _, ext := range strings.Split(extFilter, ",") {
			ext = strings.TrimSpace(ext)
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			allowedExts[strings.ToLower(ext)] = true
		}
	}

	var results []string
	matchCount := 0

	err = filepath.Walk(searchPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // skip inaccessible paths
		}
		if matchCount >= maxResults {
			return filepath.SkipAll
		}
		if info.IsDir() {
			// Skip hidden directories and common non-source directories.
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") && path != searchPath {
				return filepath.SkipDir
			}
			switch base {
			case "node_modules", "vendor", "__pycache__", ".git":
				return filepath.SkipDir
			}
			return nil
		}

		// Skip binary and large files.
		if info.Size() > 1024*1024 { // 1MB limit
			return nil
		}

		// Extension filter.
		if len(allowedExts) > 0 {
			ext := strings.ToLower(filepath.Ext(path))
			if !allowedExts[ext] {
				return nil
			}
		}

		// Read file and search.
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if matchCount >= maxResults {
				break
			}
			if re.MatchString(line) {
				matchCount++
				trimmed := strings.TrimSpace(line)
				if len(trimmed) > 200 {
					trimmed = trimmed[:200] + "..."
				}
				results = append(results, fmt.Sprintf("%s:%d: %s", path, i+1, trimmed))
			}
		}
		return nil
	})

	if err != nil {
		debug.Warn("grep", "Walk error: %v", err)
	}

	if len(results) == 0 {
		return fmt.Sprintf("No matches found for pattern %q in %s", pattern, searchPath), nil
	}

	header := fmt.Sprintf("Found %d match(es) for %q:\n", matchCount, pattern)
	if matchCount >= maxResults {
		header = fmt.Sprintf("Found %d+ match(es) for %q (truncated at %d):\n", matchCount, pattern, maxResults)
	}
	return header + strings.Join(results, "\n"), nil
}
