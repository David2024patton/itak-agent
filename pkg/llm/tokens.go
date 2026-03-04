package llm

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/David2024patton/GOAgent/pkg/debug"
)

// TokenTracker tracks cumulative token usage across agents, sessions, and requests.
type TokenTracker struct {
	mu       sync.RWMutex
	sessions map[string]*SessionUsage
	global   UsageStats
}

// SessionUsage tracks token usage for a specific session.
type SessionUsage struct {
	SessionID string
	Agents    map[string]*AgentUsage
	Total     UsageStats
	StartTime time.Time
}

// AgentUsage tracks token usage for a specific agent within a session.
type AgentUsage struct {
	AgentName    string
	RequestCount int
	Total        UsageStats
}

// UsageStats holds cumulative token counts.
type UsageStats struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	RequestCount     int64
}

// NewTokenTracker creates a new token tracker.
func NewTokenTracker() *TokenTracker {
	return &TokenTracker{
		sessions: make(map[string]*SessionUsage),
	}
}

// Track records token usage for a given agent in a session.
func (t *TokenTracker) Track(sessionID, agentName string, usage *Usage) {
	if usage == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Get or create session.
	sess, ok := t.sessions[sessionID]
	if !ok {
		sess = &SessionUsage{
			SessionID: sessionID,
			Agents:    make(map[string]*AgentUsage),
			StartTime: time.Now(),
		}
		t.sessions[sessionID] = sess
	}

	// Get or create agent usage.
	au, ok := sess.Agents[agentName]
	if !ok {
		au = &AgentUsage{AgentName: agentName}
		sess.Agents[agentName] = au
	}

	// Accumulate.
	au.RequestCount++
	au.Total.PromptTokens += int64(usage.PromptTokens)
	au.Total.CompletionTokens += int64(usage.CompletionTokens)
	au.Total.TotalTokens += int64(usage.TotalTokens)

	sess.Total.PromptTokens += int64(usage.PromptTokens)
	sess.Total.CompletionTokens += int64(usage.CompletionTokens)
	sess.Total.TotalTokens += int64(usage.TotalTokens)
	sess.Total.RequestCount++

	atomic.AddInt64(&t.global.PromptTokens, int64(usage.PromptTokens))
	atomic.AddInt64(&t.global.CompletionTokens, int64(usage.CompletionTokens))
	atomic.AddInt64(&t.global.TotalTokens, int64(usage.TotalTokens))
	atomic.AddInt64(&t.global.RequestCount, 1)

	debug.Debug("tokens", "[%s/%s] +%d tokens (session total: %d, global total: %d)",
		sessionID[:8], agentName, usage.TotalTokens, sess.Total.TotalTokens, t.global.TotalTokens)
}

// SessionTotal returns total usage for a session.
func (t *TokenTracker) SessionTotal(sessionID string) UsageStats {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if sess, ok := t.sessions[sessionID]; ok {
		return sess.Total
	}
	return UsageStats{}
}

// AgentTotal returns total usage for a specific agent in a session.
func (t *TokenTracker) AgentTotal(sessionID, agentName string) UsageStats {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if sess, ok := t.sessions[sessionID]; ok {
		if au, ok := sess.Agents[agentName]; ok {
			return au.Total
		}
	}
	return UsageStats{}
}

// GlobalTotal returns total usage across all sessions.
func (t *TokenTracker) GlobalTotal() UsageStats {
	return UsageStats{
		PromptTokens:     atomic.LoadInt64(&t.global.PromptTokens),
		CompletionTokens: atomic.LoadInt64(&t.global.CompletionTokens),
		TotalTokens:      atomic.LoadInt64(&t.global.TotalTokens),
		RequestCount:     atomic.LoadInt64(&t.global.RequestCount),
	}
}

// Summary returns a human-readable summary.
func (t *TokenTracker) Summary() string {
	g := t.GlobalTotal()
	t.mu.RLock()
	sessionCount := len(t.sessions)
	t.mu.RUnlock()

	return fmt.Sprintf("Token usage: %d total (%d prompt + %d completion) across %d requests in %d sessions",
		g.TotalTokens, g.PromptTokens, g.CompletionTokens, g.RequestCount, sessionCount)
}

// ─── Budget Client ─────────────────────────────────────────────────

// BudgetClient wraps a Client with token budget enforcement.
// When the budget reaches 80%, it auto-falls back to a cheaper model.
type BudgetClient struct {
	Primary     Client
	Fallback    Client
	MaxTokens   int64
	Used        int64
	FallbackPct float64 // default: 0.8 (switch at 80%)
	switched    bool
	mu          sync.Mutex
}

// NewBudgetClient creates a budget-aware client.
func NewBudgetClient(primary, fallback Client, maxTokens int64) *BudgetClient {
	return &BudgetClient{
		Primary:     primary,
		Fallback:    fallback,
		MaxTokens:   maxTokens,
		FallbackPct: 0.8,
	}
}

// Chat routes to primary or fallback based on budget consumption.
func (bc *BudgetClient) Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error) {
	bc.mu.Lock()
	threshold := int64(float64(bc.MaxTokens) * bc.FallbackPct)
	overBudget := bc.Used >= bc.MaxTokens
	fallbackZone := bc.Used >= threshold
	bc.mu.Unlock()

	if overBudget {
		debug.Warn("budget", "Token budget exhausted (%d/%d)  -  rejecting request", bc.Used, bc.MaxTokens)
		return nil, fmt.Errorf("token budget exhausted: used %d of %d", bc.Used, bc.MaxTokens)
	}

	client := bc.Primary
	if fallbackZone && bc.Fallback != nil {
		if !bc.switched {
			debug.Info("budget", "Budget %.0f%% consumed (%d/%d)  -  switching to fallback model",
				float64(bc.Used)/float64(bc.MaxTokens)*100, bc.Used, bc.MaxTokens)
			bc.switched = true
		}
		client = bc.Fallback
	}

	resp, err := client.Chat(ctx, messages, tools)
	if err != nil {
		return nil, err
	}

	// Track consumed tokens.
	if resp.Usage != nil {
		bc.mu.Lock()
		bc.Used += int64(resp.Usage.TotalTokens)
		pct := float64(bc.Used) / float64(bc.MaxTokens) * 100
		bc.mu.Unlock()
		debug.Debug("budget", "Token budget: %d/%d (%.1f%%)", bc.Used, bc.MaxTokens, pct)
	}

	return resp, nil
}

// BudgetRemaining returns the remaining token budget.
func (bc *BudgetClient) BudgetRemaining() int64 {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	remaining := bc.MaxTokens - bc.Used
	if remaining < 0 {
		return 0
	}
	return remaining
}
