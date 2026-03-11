package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// GuardrailAction defines what a guardrail does when it triggers.
type GuardrailAction string

const (
	GuardrailBlock GuardrailAction = "block"
	GuardrailWarn  GuardrailAction = "warn"
	GuardrailAsk   GuardrailAction = "ask" // human-in-the-loop (future)
)

// GuardrailResult is the outcome of a guardrail check.
type GuardrailResult struct {
	Passed  bool
	Action  GuardrailAction
	Reason  string
	Rule    string
}

// Guardrail is a pre-execution check for tool calls.
type Guardrail interface {
	Check(ctx context.Context, toolName string, args map[string]interface{}) GuardrailResult
}

// RateLimitGuardrail prevents excessive tool calls.
type RateLimitGuardrail struct {
	mu        sync.Mutex
	calls     map[string][]time.Time
	MaxCalls  int           // max calls per window
	Window    time.Duration // time window
}

// NewRateLimitGuardrail creates a rate limiter.
func NewRateLimitGuardrail(maxCalls int, window time.Duration) *RateLimitGuardrail {
	return &RateLimitGuardrail{
		calls:    make(map[string][]time.Time),
		MaxCalls: maxCalls,
		Window:   window,
	}
}

func (r *RateLimitGuardrail) Check(ctx context.Context, toolName string, args map[string]interface{}) GuardrailResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.Window)

	// Clean old entries.
	recent := make([]time.Time, 0)
	for _, t := range r.calls[toolName] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= r.MaxCalls {
		return GuardrailResult{
			Passed: false,
			Action: GuardrailBlock,
			Reason: fmt.Sprintf("Rate limit: %s called %d times in %s (max %d)", toolName, len(recent), r.Window, r.MaxCalls),
			Rule:   "rate_limit",
		}
	}

	recent = append(recent, now)
	r.calls[toolName] = recent

	return GuardrailResult{Passed: true}
}

// ContentFilterGuardrail blocks tool calls with dangerous content patterns.
type ContentFilterGuardrail struct {
	BlockedPatterns map[string][]string // tool name → blocked patterns in args
}

// NewContentFilterGuardrail creates a content filter with default patterns.
func NewContentFilterGuardrail() *ContentFilterGuardrail {
	return &ContentFilterGuardrail{
		BlockedPatterns: map[string][]string{
			"file_write": {
				"/etc/passwd",
				"/etc/shadow",
				"C:\\Windows\\System32",
				".ssh/authorized_keys",
			},
			"shell": {
				"curl | bash",
				"wget | bash",
				"eval(",
				"base64 -d |",
			},
		},
	}
}

func (cf *ContentFilterGuardrail) Check(ctx context.Context, toolName string, args map[string]interface{}) GuardrailResult {
	patterns, exists := cf.BlockedPatterns[toolName]
	if !exists {
		return GuardrailResult{Passed: true}
	}

	// Check all string args against blocked patterns.
	for _, val := range args {
		s, ok := val.(string)
		if !ok {
			continue
		}
		lower := strings.ToLower(s)
		for _, pattern := range patterns {
			if strings.Contains(lower, strings.ToLower(pattern)) {
				return GuardrailResult{
					Passed: false,
					Action: GuardrailBlock,
					Reason: fmt.Sprintf("Content filter: blocked pattern %q in %s args", pattern, toolName),
					Rule:   "content_filter",
				}
			}
		}
	}

	return GuardrailResult{Passed: true}
}

// SSRFGuardrail blocks tool calls that target private-network addresses.
// Checks URL-like arguments in http, web_search, and browser tools.
// Mirrors OpenClaw v2026.3.8: "Browser/SSRF: block private-network
// intermediate redirect hops in strict browser navigation flows."
type SSRFGuardrail struct {
	// TargetTools lists tool names whose args should be checked for URLs.
	// If empty, defaults to http, web_search, browser.
	TargetTools map[string]bool
}

// NewSSRFGuardrail creates an SSRF guardrail with default tool targets.
func NewSSRFGuardrail() *SSRFGuardrail {
	return &SSRFGuardrail{
		TargetTools: map[string]bool{
			"http":       true,
			"web_search": true,
			"browser":    true,
			"http_get":   true,
			"http_post":  true,
			"fetch_url":  true,
		},
	}
}

func (sg *SSRFGuardrail) Check(ctx context.Context, toolName string, args map[string]interface{}) GuardrailResult {
	if !sg.TargetTools[toolName] {
		return GuardrailResult{Passed: true}
	}
	debug.Debug("ssrf", "Checking tool %q args for SSRF", toolName)

	// Check all string args that look like URLs.
	for key, val := range args {
		s, ok := val.(string)
		if !ok || s == "" {
			continue
		}

		// Only check args that are likely URLs.
		lower := strings.ToLower(key)
		isURLArg := lower == "url" || lower == "href" || lower == "endpoint" ||
			lower == "target" || lower == "address" ||
			strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") ||
			strings.HasPrefix(s, "ftp://") || strings.HasPrefix(s, "file://")

		if !isURLArg {
			continue
		}

		// Validate the scheme first.
		if err := ValidateScheme(s); err != nil {
			debug.Warn("ssrf", "BLOCKED: scheme violation on %s.%s = %q: %v", toolName, key, s, err)
			return GuardrailResult{
				Passed: false,
				Action: GuardrailBlock,
				Reason: fmt.Sprintf("SSRF: %s (tool: %s, arg: %s)", err, toolName, key),
				Rule:   "ssrf_scheme",
			}
		}

		// Check if the URL resolves to a private IP.
		if err := ValidateURL(s); err != nil {
			debug.Warn("ssrf", "BLOCKED: private IP on %s.%s = %q: %v", toolName, key, s, err)
			return GuardrailResult{
				Passed: false,
				Action: GuardrailBlock,
				Reason: fmt.Sprintf("SSRF: %s (tool: %s, arg: %s)", err, toolName, key),
				Rule:   "ssrf_private_ip",
			}
		}
	}

	debug.Debug("ssrf", "Tool %q passed SSRF check", toolName)
	return GuardrailResult{Passed: true}
}

// ScriptSnapshotGuardrail prevents TOCTOU attacks on script execution.
// When a script file is approved for execution, its SHA-256 hash is recorded.
// Before actual execution, the hash is re-checked. If the file was modified
// between approval and execution, the call is blocked.
// Mirrors OpenClaw v2026.3.8: "Security/system.run: bind approved script
// operands to on-disk file snapshots so post-approval script rewrites are
// denied before execution."
type ScriptSnapshotGuardrail struct {
	mu        sync.Mutex
	snapshots map[string]string // file path -> SHA-256 hash at approval time
}

// NewScriptSnapshotGuardrail creates a script snapshot guardrail.
func NewScriptSnapshotGuardrail() *ScriptSnapshotGuardrail {
	return &ScriptSnapshotGuardrail{
		snapshots: make(map[string]string),
	}
}

// Approve records a snapshot of a script file that the user approved for execution.
// Call this when the user explicitly approves running a specific script.
func (ss *ScriptSnapshotGuardrail) Approve(filePath string) error {
	hash, err := hashFile(filePath)
	if err != nil {
		return fmt.Errorf("snapshot: cannot hash %q: %w", filePath, err)
	}

	ss.mu.Lock()
	ss.snapshots[filePath] = hash
	ss.mu.Unlock()

	debug.Info("snapshot", "Approved script snapshot: %s (sha256: %s)", filePath, hash[:16])
	return nil
}

// Check verifies that any script file referenced in a shell tool call hasn't
// been modified since it was approved.
func (ss *ScriptSnapshotGuardrail) Check(ctx context.Context, toolName string, args map[string]interface{}) GuardrailResult {
	if toolName != "shell" {
		return GuardrailResult{Passed: true}
	}

	command, ok := args["command"].(string)
	if !ok {
		return GuardrailResult{Passed: true}
	}

	// Extract script operands from common script runners.
	scriptPaths := extractScriptPaths(command)
	if len(scriptPaths) == 0 {
		return GuardrailResult{Passed: true}
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	for _, path := range scriptPaths {
		approvedHash, wasApproved := ss.snapshots[path]
		if !wasApproved {
			// Script was never explicitly approved -- allow (guardrail only
			// protects approved scripts from post-approval modification).
			continue
		}

		currentHash, err := hashFile(path)
		if err != nil {
			return GuardrailResult{
				Passed: false,
				Action: GuardrailBlock,
				Reason: fmt.Sprintf("Script snapshot: cannot re-hash %q: %v", path, err),
				Rule:   "script_snapshot",
			}
		}

		if currentHash != approvedHash {
			return GuardrailResult{
				Passed: false,
				Action: GuardrailBlock,
				Reason: fmt.Sprintf("Script snapshot: %q was modified after approval (approved: %s, current: %s)",
					path, approvedHash[:16], currentHash[:16]),
				Rule: "script_snapshot_mismatch",
			}
		}
	}

	return GuardrailResult{Passed: true}
}

// hashFile computes the SHA-256 hash of a file's contents.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// extractScriptPaths pulls file path operands from common script-runner
// command patterns: bash/sh/zsh, python, node, bun, deno run, powershell.
func extractScriptPaths(command string) []string {
	runners := []string{
		"bash ", "sh ", "zsh ",
		"python ", "python3 ",
		"node ", "bun ", "bun run ",
		"deno run ",
		"powershell ", "pwsh ",
	}

	var paths []string
	lower := strings.ToLower(command)

	for _, runner := range runners {
		idx := strings.Index(lower, runner)
		if idx == -1 {
			continue
		}

		// The file path is the token after the runner.
		rest := strings.TrimSpace(command[idx+len(runner):])
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			continue
		}

		candidate := parts[0]
		// Skip flags.
		if strings.HasPrefix(candidate, "-") {
			if len(parts) > 1 {
				candidate = parts[1]
			} else {
				continue
			}
		}

		// Only include if it looks like a file path (has an extension or /).
		if strings.Contains(candidate, ".") || strings.Contains(candidate, "/") || strings.Contains(candidate, "\\") {
			paths = append(paths, candidate)
		}
	}

	return paths
}

// ─── IP helpers used by SSRFGuardrail ──────────────────────────────

// isArgPrivateIP checks if a raw string is a private IP address.
// Used as a fast-path check before DNS resolution.
func isArgPrivateIP(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil {
		return false
	}
	return IsPrivateIP(ip)
}

// GuardrailChain runs multiple guardrails in sequence.
type GuardrailChain struct {
	guardrails []Guardrail
}

// NewGuardrailChain creates a chain of guardrails.
func NewGuardrailChain(guardrails ...Guardrail) *GuardrailChain {
	return &GuardrailChain{guardrails: guardrails}
}

// Check runs all guardrails. Stops at the first failure.
func (gc *GuardrailChain) Check(ctx context.Context, toolName string, args map[string]interface{}) GuardrailResult {
	for _, g := range gc.guardrails {
		result := g.Check(ctx, toolName, args)
		if !result.Passed {
			debug.Warn("guardrail", "BLOCKED %s: %s (rule: %s)", toolName, result.Reason, result.Rule)
			return result
		}
	}
	return GuardrailResult{Passed: true}
}

// SafeExecute wraps a tool execution with guardrail checks.
func SafeExecute(ctx context.Context, t Tool, args map[string]interface{}, chain *GuardrailChain) (string, error) {
	if chain != nil {
		result := chain.Check(ctx, t.Name(), args)
		if !result.Passed {
			return fmt.Sprintf("GUARDRAIL BLOCKED: %s", result.Reason), nil
		}
	}
	return t.Execute(ctx, args)
}
