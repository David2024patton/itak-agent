package builtins

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/David2024patton/GOAgent/pkg/debug"
)

// Default denied commands per OS. These are blocked unless explicitly overridden.
var defaultDeniedUnix = []string{
	"rm -rf /",
	"rm -rf /*",
	"rm -rf ~",
	"dd if=/dev/zero",
	"dd if=/dev/random",
	"mkfs",
	":(){ :|:& };:",      // fork bomb
	"shutdown",
	"reboot",
	"halt",
	"init 0",
	"init 6",
	"chmod -R 777 /",
	"> /dev/sda",
	"mv / /dev/null",
}

var defaultDeniedWindows = []string{
	"format c:",
	"format d:",
	"del /f /s /q c:\\",
	"rd /s /q c:\\",
	"deltree",
	"shutdown /s",
	"shutdown /r",
	"cipher /w:c",
	"diskpart",
}

// ShellTool executes shell commands with safety checks.
type ShellTool struct {
	DeniedCommands []string // additional denied commands from config
	WorkDir        string   // optional working directory (e.g., session workspace)
}

func (s *ShellTool) Name() string { return "shell" }

func (s *ShellTool) Description() string {
	return "Execute a shell command and return the output. Use for running programs, checking system state, installing packages, etc. Some dangerous commands are blocked for safety."
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

	// ─── Safety check: denied commands ───
	if blocked := s.checkDenied(command); blocked != "" {
		debug.Warn("shell", "BLOCKED dangerous command: %q (matched: %q)", command, blocked)
		return fmt.Sprintf("BLOCKED: Command rejected for safety. Matched denied pattern: %q. "+
			"This command could cause data loss or system damage.", blocked), nil
	}

	timeout := 30
	if t, ok := args["timeout_seconds"].(float64); ok {
		timeout = int(t)
	}

	debug.Info("shell", "Executing: %s (timeout: %ds, os: %s)", truncateShell(command, 100), timeout, runtime.GOOS)

	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Auto-detect OS and pick the right shell interpreter.
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(cmdCtx, "cmd", "/C", command)
	case "darwin":
		// macOS defaults to zsh since Catalina (10.15+).
		cmd = exec.CommandContext(cmdCtx, "zsh", "-c", command)
	case "linux", "android", "freebsd", "openbsd", "netbsd":
		cmd = exec.CommandContext(cmdCtx, "sh", "-c", command)
	default:
		// Safe fallback for any other GOOS (plan9, solaris, etc.)
		cmd = exec.CommandContext(cmdCtx, "sh", "-c", command)
	}

	// Set working directory if session workspace is configured.
	if s.WorkDir != "" {
		cmd.Dir = s.WorkDir
		debug.Debug("shell", "WorkDir: %s", s.WorkDir)
	}

	output, err := cmd.CombinedOutput()

	result := strings.TrimSpace(string(output))
	if err != nil {
		debug.Debug("shell", "Command exited with error: %v", err)
		if result != "" {
			return fmt.Sprintf("EXIT ERROR: %v\nOUTPUT:\n%s", err, result), nil
		}
		return "", fmt.Errorf("command failed: %w", err)
	}

	debug.Debug("shell", "Output (%d bytes): %s", len(result), truncateShell(result, 200))

	if result == "" {
		return "(no output)", nil
	}
	return result, nil
}

// checkDenied checks if a command matches any denied pattern.
// Returns the matched pattern or empty string.
func (s *ShellTool) checkDenied(command string) string {
	lower := strings.ToLower(strings.TrimSpace(command))

	// Check OS-specific defaults.
	var defaults []string
	if runtime.GOOS == "windows" {
		defaults = defaultDeniedWindows
	} else {
		defaults = defaultDeniedUnix
	}

	for _, denied := range defaults {
		if strings.Contains(lower, strings.ToLower(denied)) {
			return denied
		}
	}

	// Check user-configured denied commands.
	for _, denied := range s.DeniedCommands {
		if strings.Contains(lower, strings.ToLower(denied)) {
			return denied
		}
	}

	return ""
}

// truncateShell shortens a string for shell logging.
func truncateShell(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
