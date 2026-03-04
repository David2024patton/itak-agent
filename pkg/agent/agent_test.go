package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ── parseDelegation Tests ───────────────────────────────────────────

func TestParseDelegationValidJSON(t *testing.T) {
	raw := `{
		"reasoning": "User wants file listing",
		"delegations": [
			{"agent": "scout", "task": "list files in /tmp", "context": "looking for config files"}
		]
	}`

	del, direct, err := parseDelegation(raw)
	if err != nil {
		t.Fatalf("parseDelegation error: %v", err)
	}
	if direct != "" {
		t.Fatalf("expected no direct response, got %q", direct)
	}
	if del.Reasoning != "User wants file listing" {
		t.Errorf("reasoning mismatch: %q", del.Reasoning)
	}
	if len(del.Delegations) != 1 {
		t.Fatalf("expected 1 delegation, got %d", len(del.Delegations))
	}
	if del.Delegations[0].Agent != "scout" {
		t.Errorf("expected agent 'scout', got %q", del.Delegations[0].Agent)
	}
	if del.Delegations[0].Task != "list files in /tmp" {
		t.Errorf("expected task mismatch: %q", del.Delegations[0].Task)
	}
}

func TestParseDelegationDirectResponse(t *testing.T) {
	raw := `{
		"reasoning": "greeting",
		"delegations": [],
		"direct_response": "Hello! How can I help?"
	}`

	del, direct, err := parseDelegation(raw)
	if err != nil {
		t.Fatalf("parseDelegation error: %v", err)
	}
	if del != nil {
		t.Error("expected nil delegation for direct response")
	}
	if direct != "Hello! How can I help?" {
		t.Errorf("expected direct response, got %q", direct)
	}
}

func TestParseDelegationNoJSON(t *testing.T) {
	raw := "I'm not sure how to help with that. Could you rephrase?"

	del, direct, err := parseDelegation(raw)
	if err != nil {
		t.Fatalf("parseDelegation error: %v", err)
	}
	if del != nil {
		t.Error("expected nil delegation for plain text")
	}
	if direct == "" {
		t.Error("expected direct response for plain text")
	}
}

func TestParseDelegationWrappedInMarkdown(t *testing.T) {
	raw := "```json\n{\"reasoning\": \"test\", \"delegations\": [{\"agent\": \"scout\", \"task\": \"check files\"}]}\n```"

	del, _, err := parseDelegation(raw)
	if err != nil {
		t.Fatalf("parseDelegation error: %v", err)
	}
	if del == nil {
		t.Fatal("expected delegation, got nil")
	}
	if len(del.Delegations) != 1 {
		t.Errorf("expected 1 delegation, got %d", len(del.Delegations))
	}
}

func TestParseDelegationEmptyString(t *testing.T) {
	_, _, err := parseDelegation("")
	if err == nil {
		t.Error("expected error for empty string")
	}
}

func TestParseDelegationMultipleAgents(t *testing.T) {
	raw := `{
		"reasoning": "need both scout and coder",
		"delegations": [
			{"agent": "scout", "task": "find config file"},
			{"agent": "coder", "task": "fix the bug"}
		]
	}`

	del, _, err := parseDelegation(raw)
	if err != nil {
		t.Fatalf("parseDelegation error: %v", err)
	}
	if len(del.Delegations) != 2 {
		t.Errorf("expected 2 delegations, got %d", len(del.Delegations))
	}
}

// ── extractJSON Tests ───────────────────────────────────────────────

func TestExtractJSONClean(t *testing.T) {
	input := `{"key": "value"}`
	result := extractJSON(input)
	if result != input {
		t.Errorf("expected %q, got %q", input, result)
	}
}

func TestExtractJSONWithSurroundingText(t *testing.T) {
	input := `Here is the JSON response: {"reasoning": "test", "delegations": []} and some trailing text`
	result := extractJSON(input)
	if !strings.HasPrefix(result, "{") || !strings.HasSuffix(result, "}") {
		t.Errorf("expected JSON object, got %q", result)
	}
}

func TestExtractJSONFromMarkdownBlock(t *testing.T) {
	input := "```json\n{\"reasoning\": \"test\"}\n```"
	result := extractJSON(input)
	if !strings.HasPrefix(result, "{") {
		t.Errorf("expected JSON, got %q", result)
	}
}

func TestExtractJSONNoJSON(t *testing.T) {
	input := "Just plain text with no JSON"
	result := extractJSON(input)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

// ── parseFlexibleContext Tests ───────────────────────────────────────

func TestParseFlexibleContextString(t *testing.T) {
	raw := json.RawMessage(`"look for .yaml files"`)
	result := parseFlexibleContext(raw)
	if result != "look for .yaml files" {
		t.Errorf("expected string, got %q", result)
	}
}

func TestParseFlexibleContextObject(t *testing.T) {
	raw := json.RawMessage(`{"ip": "192.168.1.1", "port": 8080}`)
	result := parseFlexibleContext(raw)
	if result == "" {
		t.Error("expected non-empty result for object context")
	}
}

func TestParseFlexibleContextEmpty(t *testing.T) {
	result := parseFlexibleContext(nil)
	if result != "" {
		t.Errorf("expected empty for nil, got %q", result)
	}
}

// ── focusedSystemPrompt Tests ───────────────────────────────────────

func TestFocusedSystemPrompt(t *testing.T) {
	cfg := AgentConfig{
		Name:        "scout",
		Role:        "System Scout",
		Personality: "careful observer",
		Goals:       []string{"accuracy", "speed"},
	}

	prompt := focusedSystemPrompt(cfg)
	if !strings.Contains(prompt, "scout") {
		t.Error("prompt should contain agent name")
	}
	if !strings.Contains(prompt, "System Scout") {
		t.Error("prompt should contain role")
	}
	if !strings.Contains(prompt, "accuracy") {
		t.Error("prompt should contain goals")
	}
	if !strings.Contains(prompt, "FOCUSED AGENT") {
		t.Error("prompt should contain framework identity")
	}
}

// ── NewFocusedAgent Defaults ─────────────────────────────────────────

func TestNewFocusedAgentDefaults(t *testing.T) {
	cfg := AgentConfig{Name: "test"}
	agent := NewFocusedAgent(cfg, nil, nil, nil, nil, nil, nil, "session-test12345")

	if agent.Config.MaxSkills != DefaultMaxSkills {
		t.Errorf("expected MaxSkills %d, got %d", DefaultMaxSkills, agent.Config.MaxSkills)
	}
	if agent.Config.MaxLoops != DefaultMaxLoops {
		t.Errorf("expected MaxLoops %d, got %d", DefaultMaxLoops, agent.Config.MaxLoops)
	}
	if agent.SessionID != "session-test12345" {
		t.Errorf("expected session ID mismatch")
	}
}

func TestNewFocusedAgentCustomValues(t *testing.T) {
	cfg := AgentConfig{Name: "custom", MaxSkills: 5, MaxLoops: 8}
	agent := NewFocusedAgent(cfg, nil, nil, nil, nil, nil, nil, "sess-001234567")

	if agent.Config.MaxSkills != 5 {
		t.Errorf("expected MaxSkills 5, got %d", agent.Config.MaxSkills)
	}
	if agent.Config.MaxLoops != 8 {
		t.Errorf("expected MaxLoops 8, got %d", agent.Config.MaxLoops)
	}
}

// ── NewEvaluator Tests ──────────────────────────────────────────────

func TestNewEvaluatorDefaults(t *testing.T) {
	ev := NewEvaluator(0)
	if ev.NumRuns != 3 {
		t.Errorf("expected default 3 runs, got %d", ev.NumRuns)
	}

	ev2 := NewEvaluator(-1)
	if ev2.NumRuns != 3 {
		t.Errorf("expected default 3 for negative input, got %d", ev2.NumRuns)
	}
}

func TestNewEvaluatorCustom(t *testing.T) {
	ev := NewEvaluator(10)
	if ev.NumRuns != 10 {
		t.Errorf("expected 10 runs, got %d", ev.NumRuns)
	}
}

// ── EvalReport.Summary Tests ─────────────────────────────────────────

func TestEvalReportSummary(t *testing.T) {
	report := &EvalReport{
		TaskName:    "file-listing",
		Agent:       "scout",
		TotalRuns:   5,
		Successes:   4,
		SuccessRate: 80.0,
		AvgDuration: 150 * time.Millisecond,
	}

	summary := report.Summary()
	if !strings.Contains(summary, "file-listing") {
		t.Error("summary should contain task name")
	}
	if !strings.Contains(summary, "scout") {
		t.Error("summary should contain agent name")
	}
	if !strings.Contains(summary, "80%") {
		t.Error("summary should contain success rate")
	}
}

// ── Pattern Runner Tests ─────────────────────────────────────────────

func TestNewChainRunner(t *testing.T) {
	cr := NewChainRunner("pipeline-1")
	if cr.Name != "pipeline-1" {
		t.Errorf("expected name 'pipeline-1', got %q", cr.Name)
	}
	if len(cr.Agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(cr.Agents))
	}
}

func TestNewParallelRunner(t *testing.T) {
	pr := NewParallelRunner("fan-out")
	if pr.Name != "fan-out" {
		t.Errorf("expected name 'fan-out', got %q", pr.Name)
	}
	if len(pr.Agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(pr.Agents))
	}
}

// ── truncate Tests ──────────────────────────────────────────────────

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		within bool // true = result length <= maxLen
	}{
		{"short", 10, true},
		{"this is a much longer string that should be truncated", 20, true},
		{"exact", 5, true},
		{"", 10, true},
		{"multi\nline\ntext", 50, true}, // newlines get replaced with spaces
	}

	for _, tc := range tests {
		result := truncate(tc.input, tc.maxLen)
		if tc.within && len(result) > tc.maxLen+3 { // +3 for "..."
			t.Errorf("truncate(%q, %d) = %q (len %d), expected <= %d",
				tc.input, tc.maxLen, result, len(result), tc.maxLen+3)
		}
	}
}

func TestTruncateNewlines(t *testing.T) {
	result := truncate("line1\nline2\r\nline3", 50)
	if strings.Contains(result, "\n") || strings.Contains(result, "\r") {
		t.Error("truncate should replace newlines")
	}
}

// ── Constants Tests ──────────────────────────────────────────────────

func TestConstants(t *testing.T) {
	if DefaultMaxSkills != 7 {
		t.Errorf("DefaultMaxSkills should be 7, got %d", DefaultMaxSkills)
	}
	if DefaultMaxLoops != 10 {
		t.Errorf("DefaultMaxLoops should be 10, got %d", DefaultMaxLoops)
	}
}
