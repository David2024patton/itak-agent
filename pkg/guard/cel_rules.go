// Package guard provides CEL-powered expression rules for dynamic policy evaluation.
//
// What: Evaluates user-defined CEL expressions at runtime to allow/block requests.
// Why:  Hardcoded regex patterns can't cover every scenario. CEL gives admins the
//       ability to write custom policy rules like "input.length < 10000" or
//       "source != 'external' || user.role == 'admin'" without touching Go code.
// How:  Load rules from config YAML, compile them once at startup, evaluate each
//       incoming request against all active rules. Rules return true (block) or false (allow).

package guard

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// CELRule is a single user-defined expression rule.
type CELRule struct {
	Name        string   `yaml:"name" json:"name"`               // Human-readable rule name.
	Expression  string   `yaml:"expression" json:"expression"`     // CEL expression that returns bool.
	Severity    Severity `yaml:"severity" json:"severity"`         // Severity if rule matches.
	Description string   `yaml:"description" json:"description"`   // What this rule detects.
	Enabled     bool     `yaml:"enabled" json:"enabled"`
}

// compiledRule is a pre-compiled CEL program ready for fast evaluation.
type compiledRule struct {
	CELRule
	program cel.Program
}

// CELEngine evaluates user-defined CEL expressions against request context.
// Thread-safe: rules are compiled once and evaluated concurrently.
type CELEngine struct {
	mu    sync.RWMutex
	env   *cel.Env
	rules []compiledRule
}

// NewCELEngine creates a CEL engine with standard variables available to rules.
//
// Available variables in CEL expressions:
//   - input       (string)  : the message text
//   - source      (string)  : "user", "external", "email", "tool_output"
//   - input_len   (int)     : length of the input
//   - agent_name  (string)  : target agent name
//   - has_code    (bool)    : whether the input contains code blocks
//   - word_count  (int)     : number of words in the input
func NewCELEngine() (*CELEngine, error) {
	env, err := cel.NewEnv(
		cel.Declarations(
			decls.NewVar("input", decls.String),
			decls.NewVar("source", decls.String),
			decls.NewVar("input_len", decls.Int),
			decls.NewVar("agent_name", decls.String),
			decls.NewVar("has_code", decls.Bool),
			decls.NewVar("word_count", decls.Int),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("cel env: %w", err)
	}

	return &CELEngine{env: env}, nil
}

// LoadRules compiles a set of CEL rules. Invalid rules are logged and skipped.
func (e *CELEngine) LoadRules(rules []CELRule) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.rules = nil

	for _, r := range rules {
		if !r.Enabled || r.Expression == "" {
			continue
		}

		ast, issues := e.env.Compile(r.Expression)
		if issues != nil && issues.Err() != nil {
			debug.Warn("cel", "Failed to compile rule %q: %v", r.Name, issues.Err())
			continue
		}

		prg, err := e.env.Program(ast)
		if err != nil {
			debug.Warn("cel", "Failed to program rule %q: %v", r.Name, err)
			continue
		}

		e.rules = append(e.rules, compiledRule{
			CELRule: r,
			program: prg,
		})

		debug.Debug("cel", "Loaded rule: %s [severity=%s] %s", r.Name, r.Severity, r.Expression)
	}

	debug.Info("cel", "Loaded %d/%d CEL rules", len(e.rules), len(rules))
}

// Evaluate runs all active rules against the given request context.
// Returns a ScanResult with the highest severity match.
func (e *CELEngine) Evaluate(input, source, agentName string) ScanResult {
	e.mu.RLock()
	defer e.mu.RUnlock()

	start := time.Now()

	result := ScanResult{
		Severity: SeveritySafe,
		Action:   ActionAllow,
		Source:   source,
	}

	if len(e.rules) == 0 {
		return result
	}

	// Build the activation context (variables available to CEL expressions).
	hasCode := strings.Contains(input, "```") || strings.Contains(input, "    ")
	wordCount := len(strings.Fields(input))

	activation := map[string]interface{}{
		"input":      input,
		"source":     source,
		"input_len":  int64(len(input)),
		"agent_name": agentName,
		"has_code":   hasCode,
		"word_count": int64(wordCount),
	}

	for _, r := range e.rules {
		out, _, err := r.program.Eval(activation)
		if err != nil {
			debug.Debug("cel", "Rule %q eval error: %v", r.Name, err)
			continue
		}

		// Rule must return a boolean. true = threat detected.
		if val, ok := out.Value().(bool); ok && val {
			result.Reasons = append(result.Reasons, "cel:"+r.Name)
			if r.Severity > result.Severity {
				result.Severity = r.Severity
			}
		}
	}

	// Determine action based on severity.
	switch {
	case result.Severity >= SeverityHigh:
		result.Action = ActionBlock
		result.Blocked = true
	case result.Severity == SeverityMedium:
		result.Action = ActionWarn
	case result.Severity == SeverityLow:
		result.Action = ActionLog
	}

	result.ScanTimeMs = time.Since(start).Milliseconds()

	if result.Blocked {
		debug.Warn("cel", "BLOCKED by CEL rules: %v", result.Reasons)
	}

	return result
}

// DefaultRules returns a set of standard CEL rules that ship with iTaK.
func DefaultRules() []CELRule {
	return []CELRule{
		{
			Name:        "input_too_long",
			Expression:  "input_len > 100000",
			Severity:    SeverityMedium,
			Description: "Block extremely long inputs (> 100K chars) that could cause OOM.",
			Enabled:     true,
		},
		{
			Name:        "external_code_injection",
			Expression:  `source != "user" && has_code && input_len > 500`,
			Severity:    SeverityHigh,
			Description: "Block code blocks from untrusted external sources.",
			Enabled:     true,
		},
		{
			Name:        "empty_input",
			Expression:  "input_len == 0",
			Severity:    SeverityLow,
			Description: "Flag empty inputs.",
			Enabled:     true,
		},
		{
			Name:        "high_word_count_external",
			Expression:  `source == "external" && word_count > 5000`,
			Severity:    SeverityMedium,
			Description: "Flag very long external content that may be a prompt stuffing attack.",
			Enabled:     true,
		},
	}
}
