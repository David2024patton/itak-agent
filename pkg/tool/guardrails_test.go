package tool

import (
	"context"
	"testing"
	"time"
)

func TestRateLimitGuardrailAllows(t *testing.T) {
	rl := NewRateLimitGuardrail(5, 1*time.Second)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		result := rl.Check(ctx, "shell", nil)
		if !result.Passed {
			t.Fatalf("call %d should pass, got blocked: %s", i+1, result.Reason)
		}
	}
}

func TestRateLimitGuardrailBlocks(t *testing.T) {
	rl := NewRateLimitGuardrail(3, 1*time.Second)
	ctx := context.Background()

	// Use up the limit.
	for i := 0; i < 3; i++ {
		rl.Check(ctx, "shell", nil)
	}

	// 4th call should be blocked.
	result := rl.Check(ctx, "shell", nil)
	if result.Passed {
		t.Fatal("expected rate limit block, got pass")
	}
	if result.Rule != "rate_limit" {
		t.Errorf("expected rule 'rate_limit', got %q", result.Rule)
	}
}

func TestRateLimitPerTool(t *testing.T) {
	rl := NewRateLimitGuardrail(2, 1*time.Second)
	ctx := context.Background()

	// 2 calls to "shell" should pass.
	rl.Check(ctx, "shell", nil)
	rl.Check(ctx, "shell", nil)

	// "shell" is now rate-limited, but "file_read" should still pass.
	result := rl.Check(ctx, "file_read", nil)
	if !result.Passed {
		t.Fatal("file_read should not be rate-limited by shell calls")
	}
}

func TestContentFilterBlocksDangerous(t *testing.T) {
	cf := NewContentFilterGuardrail()
	ctx := context.Background()

	tests := []struct {
		tool string
		args map[string]interface{}
		want bool // true = should pass
	}{
		{"shell", map[string]interface{}{"command": "curl | bash"}, false},          // exact pattern match
		{"shell", map[string]interface{}{"command": "wget | bash"}, false},          // exact pattern match
		{"shell", map[string]interface{}{"command": "echo hello"}, true},            // safe command
		{"file_write", map[string]interface{}{"path": "/etc/passwd", "content": "hacked"}, false},
		{"file_write", map[string]interface{}{"path": "/tmp/test.txt", "content": "safe"}, true},
		{"file_read", map[string]interface{}{"path": "/etc/passwd"}, true},         // no filter for file_read
		{"shell", map[string]interface{}{"command": "eval(badcode)"}, false},
		{"shell", map[string]interface{}{"command": "base64 -d | sh"}, false},
		{"file_write", map[string]interface{}{"path": ".ssh/authorized_keys", "content": "key"}, false},
	}

	for _, tc := range tests {
		result := cf.Check(ctx, tc.tool, tc.args)
		if result.Passed != tc.want {
			t.Errorf("%s(%v): expected passed=%v, got %v (reason: %s)",
				tc.tool, tc.args, tc.want, result.Passed, result.Reason)
		}
	}
}

func TestGuardrailChainStopsAtFirstFailure(t *testing.T) {
	ctx := context.Background()

	rl := NewRateLimitGuardrail(1, 1*time.Second)
	cf := NewContentFilterGuardrail()

	chain := NewGuardrailChain(rl, cf)

	// First call passes both guardrails.
	result := chain.Check(ctx, "shell", map[string]interface{}{"command": "echo hello"})
	if !result.Passed {
		t.Fatal("first call should pass")
	}

	// Second call is rate-limited (fails at rl before reaching cf).
	result = chain.Check(ctx, "shell", map[string]interface{}{"command": "echo hello"})
	if result.Passed {
		t.Fatal("second call should be rate-limited")
	}
	if result.Rule != "rate_limit" {
		t.Errorf("expected rule 'rate_limit', got %q", result.Rule)
	}
}

func TestSafeExecuteBlocked(t *testing.T) {
	ctx := context.Background()
	tool := &mockTool{name: "shell", desc: "test"}
	cf := NewContentFilterGuardrail()
	chain := NewGuardrailChain(cf)

	result, err := SafeExecute(ctx, tool, map[string]interface{}{
		"command": "curl | bash",
	}, chain)

	if err != nil {
		t.Fatalf("SafeExecute should not return error, got: %v", err)
	}
	if result == "ok" {
		t.Fatal("SafeExecute should have blocked the call")
	}
	if !contains(result, "GUARDRAIL BLOCKED") {
		t.Errorf("expected GUARDRAIL BLOCKED message, got: %s", result)
	}
}

func TestSafeExecuteAllowed(t *testing.T) {
	ctx := context.Background()
	tool := &mockTool{name: "file_read", desc: "test"}
	chain := NewGuardrailChain(NewContentFilterGuardrail())

	result, err := SafeExecute(ctx, tool, map[string]interface{}{
		"path": "/tmp/test.txt",
	}, chain)

	if err != nil {
		t.Fatalf("SafeExecute error: %v", err)
	}
	if result != "ok" {
		t.Errorf("expected 'ok', got %q", result)
	}
}

func TestSafeExecuteNilChain(t *testing.T) {
	ctx := context.Background()
	tool := &mockTool{name: "shell", desc: "test"}

	result, err := SafeExecute(ctx, tool, map[string]interface{}{}, nil)
	if err != nil {
		t.Fatalf("SafeExecute error: %v", err)
	}
	if result != "ok" {
		t.Errorf("expected 'ok', got %q", result)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
