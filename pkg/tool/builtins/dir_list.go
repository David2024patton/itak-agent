package builtins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DirListTool lists files and subdirectories in a given path.
type DirListTool struct{}

func (d *DirListTool) Name() string { return "dir_list" }

func (d *DirListTool) Description() string {
	return "List files and directories at a given path. Returns names, sizes, and types. Use this to explore the filesystem."
}

func (d *DirListTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory path to list (absolute or relative)",
			},
			"recursive": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, list recursively (max 3 levels deep). Default: false",
			},
		},
		"required": []string{"path"},
	}
}

func (d *DirListTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("missing required argument: path")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	recursive, _ := args["recursive"].(bool)
	maxDepth := 1
	if recursive {
		maxDepth = 3
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Directory: %s\n\n", absPath))

	err = listDir(absPath, "", 0, maxDepth, &result)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Directory not found: %s (this directory does not exist)", absPath), nil
		}
		return "", fmt.Errorf("list directory: %w", err)
	}

	return result.String(), nil
}

func listDir(path, indent string, depth, maxDepth int, result *strings.Builder) error {
	if depth >= maxDepth {
		return nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if entry.IsDir() {
			result.WriteString(fmt.Sprintf("%s📁 %s/\n", indent, entry.Name()))
			if depth+1 < maxDepth {
				listDir(filepath.Join(path, entry.Name()), indent+"  ", depth+1, maxDepth, result)
			}
		} else {
			size := formatSize(info.Size())
			result.WriteString(fmt.Sprintf("%s📄 %s  (%s)\n", indent, entry.Name(), size))
		}
	}
	return nil
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/1024/1024)
	case bytes >= 1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
