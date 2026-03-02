package builtins

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ShellTool executes shell commands and returns stdout/stderr.
type ShellTool struct{}

func (s *ShellTool) Name() string { return "shell" }

func (s *ShellTool) Description() string {
	return "Execute a shell command and return the output. Use for running programs, checking system state, installing packages, etc."
}

func (s *ShellTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"timeout_seconds": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in seconds (default 30)",
			},
		},
		"required": []string{"command"},
	}
}

func (s *ShellTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	command, ok := args["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("missing required argument: command")
	}

	timeout := 30
	if t, ok := args["timeout_seconds"].(float64); ok {
		timeout = int(t)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Auto-detect OS: cmd /C on Windows, sh -c on Unix.
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(cmdCtx, "sh", "-c", command)
	}

	output, err := cmd.CombinedOutput()

	result := strings.TrimSpace(string(output))
	if err != nil {
		if result != "" {
			return fmt.Sprintf("EXIT ERROR: %v\nOUTPUT:\n%s", err, result), nil
		}
		return "", fmt.Errorf("command failed: %w", err)
	}

	if result == "" {
		return "(no output)", nil
	}
	return result, nil
}

