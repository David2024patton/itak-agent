package builtins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileReadTool reads file contents.
type FileReadTool struct{}

func (f *FileReadTool) Name() string { return "file_read" }

func (f *FileReadTool) Description() string {
	return "Read the contents of a file. Returns the full text content."
}

func (f *FileReadTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Absolute or relative path to the file to read",
			},
		},
		"required": []string{"path"},
	}
}

func (f *FileReadTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("missing required argument: path")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("File not found: %s (this file does not exist yet)", absPath), nil
		}
		return "", fmt.Errorf("read file: %w", err)
	}

	return string(data), nil
}

// FileWriteTool writes content to a file, creating directories as needed.
type FileWriteTool struct {
	ProtectedPaths []string // paths the agent is NOT allowed to write to (self-preservation)
}

func (f *FileWriteTool) Name() string { return "file_write" }

func (f *FileWriteTool) Description() string {
	return "Write content to a file. Creates the file and parent directories if they don't exist."
}

func (f *FileWriteTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to write",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (f *FileWriteTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("missing required argument: path")
	}

	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("missing required argument: content")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	// ─── Self-preservation: block writes to protected paths ───
	for _, protected := range f.ProtectedPaths {
		protAbs, _ := filepath.Abs(protected)
		if protAbs != "" && strings.HasPrefix(strings.ToLower(absPath), strings.ToLower(protAbs)) {
			return fmt.Sprintf("BLOCKED: Cannot write to protected path %q. "+
				"This path is part of iTaKAgent's core and is protected by self-preservation guardrails.", protAbs), nil
		}
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create directories: %w", err)
	}

	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return fmt.Sprintf("Wrote %d bytes to %s", len(content), absPath), nil
}
