package llm

import (
	"context"
	"testing"
)

func TestTokenTrackerBasic(t *testing.T) {
	tr := NewTokenTracker()

	tr.Track("session-0001", "scout", &Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	})

	global := tr.GlobalTotal()
	if global.TotalTokens != 150 {
		t.Errorf("expected 150 total tokens, got %d", global.TotalTokens)
	}
	if global.RequestCount != 1 {
		t.Errorf("expected 1 request, got %d", global.RequestCount)
	}
}

func TestTokenTrackerMultipleSessions(t *testing.T) {
	tr := NewTokenTracker()

	tr.Track("session-0001", "scout", &Usage{TotalTokens: 100, PromptTokens: 60, CompletionTokens: 40})
	tr.Track("session-0001", "operator", &Usage{TotalTokens: 200, PromptTokens: 120, CompletionTokens: 80})
	tr.Track("session-0002", "scout", &Usage{TotalTokens: 50, PromptTokens: 30, CompletionTokens: 20})

	sess1 := tr.SessionTotal("session-0001")
	if sess1.TotalTokens != 300 {
		t.Errorf("session-0001 total: expected 300, got %d", sess1.TotalTokens)
	}

	sess2 := tr.SessionTotal("session-0002")
	if sess2.TotalTokens != 50 {
		t.Errorf("session-0002 total: expected 50, got %d", sess2.TotalTokens)
	}

	global := tr.GlobalTotal()
	if global.TotalTokens != 350 {
		t.Errorf("global total: expected 350, got %d", global.TotalTokens)
	}
}

func TestTokenTrackerAgentTotal(t *testing.T) {
	tr := NewTokenTracker()

	tr.Track("session-0001", "scout", &Usage{TotalTokens: 100})
	tr.Track("session-0001", "scout", &Usage{TotalTokens: 200})
	tr.Track("session-0001", "operator", &Usage{TotalTokens: 50})

	scoutTotal := tr.AgentTotal("session-0001", "scout")
	if scoutTotal.TotalTokens != 300 {
		t.Errorf("scout total: expected 300, got %d", scoutTotal.TotalTokens)
	}
}

func TestTokenTrackerNilUsage(t *testing.T) {
	tr := NewTokenTracker()
	tr.Track("session-0001", "scout", nil) // should not panic
	global := tr.GlobalTotal()
	if global.TotalTokens != 0 {
		t.Error("nil usage should not add tokens")
	}
}

func TestTokenTrackerMissingSession(t *testing.T) {
	tr := NewTokenTracker()
	total := tr.SessionTotal("nonexistent")
	if total.TotalTokens != 0 {
		t.Error("missing session should return zero stats")
	}
}

func TestTokenTrackerSummary(t *testing.T) {
	tr := NewTokenTracker()
	tr.Track("session-0001", "scout", &Usage{TotalTokens: 500, PromptTokens: 300, CompletionTokens: 200})

	summary := tr.Summary()
	if summary == "" {
		t.Fatal("summary should not be empty")
	}
	if !containsStr(summary, "500") {
		t.Errorf("summary should contain total tokens, got: %s", summary)
	}
}

// ── BudgetClient Tests ──────────────────────────────────────────────

// mockClient implements Client for testing.
type mockClient struct {
	usage *Usage
}

func (m *mockClient) Chat(_ context.Context, _ []Message, _ []ToolDef) (*Response, error) {
	return &Response{
		Content: "mock response",
		Usage:   m.usage,
	}, nil
}

func TestBudgetClientPrimary(t *testing.T) {
	primary := &mockClient{usage: &Usage{TotalTokens: 100}}
	fallback := &mockClient{usage: &Usage{TotalTokens: 10}}

	bc := NewBudgetClient(primary, fallback, 1000)

	resp, err := bc.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if resp.Content != "mock response" {
		t.Error("should use primary client")
	}
	if bc.BudgetRemaining() != 900 {
		t.Errorf("expected 900 remaining, got %d", bc.BudgetRemaining())
	}
}

func TestBudgetClientFallback(t *testing.T) {
	primary := &mockClient{usage: &Usage{TotalTokens: 100}}
	fallback := &mockClient{usage: &Usage{TotalTokens: 10}}

	bc := NewBudgetClient(primary, fallback, 100)
	// Set used to 80 (80% threshold).
	bc.Used = 80

	_, err := bc.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	// Should have switched to fallback (budget at 80%), so used = 80 + 10 = 90.
	if bc.BudgetRemaining() != 10 {
		t.Errorf("expected 10 remaining after fallback, got %d", bc.BudgetRemaining())
	}
}

func TestBudgetClientExhausted(t *testing.T) {
	primary := &mockClient{usage: &Usage{TotalTokens: 100}}

	bc := NewBudgetClient(primary, nil, 100)
	bc.Used = 100

	_, err := bc.Chat(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error when budget exhausted")
	}
}

func TestBudgetRemaining(t *testing.T) {
	bc := NewBudgetClient(nil, nil, 1000)
	bc.Used = 1500

	if bc.BudgetRemaining() != 0 {
		t.Errorf("remaining should be 0 when over budget, got %d", bc.BudgetRemaining())
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
