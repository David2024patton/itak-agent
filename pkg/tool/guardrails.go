package tool

import (
	"context"
	"fmt"
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
