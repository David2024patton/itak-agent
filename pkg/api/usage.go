package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// UsageAPI tracks per-user API usage and provides analytics for the SaaS dashboard.
//
// What: REST endpoints for tracking and displaying usage metrics.
// Why:  SaaS users need visibility into their consumption to justify subscription cost.
// How:  In-memory counters per user, persisted to the tenant database.
type UsageAPI struct {
	// In-memory counters (per-user).
	counters map[string]*UserUsage
}

// UserUsage tracks usage metrics for a single user.
type UserUsage struct {
	UserID       string    `json:"user_id"`
	ChatMessages int       `json:"chat_messages"`
	TokensUsed   int64     `json:"tokens_used"`
	AgentRuns    int       `json:"agent_runs"`
	TasksCreated int       `json:"tasks_created"`
	FilesStored  int       `json:"files_stored"`
	StorageMB    float64   `json:"storage_mb"`
	APIRequests  int       `json:"api_requests"`
	LastActive   time.Time `json:"last_active"`
	CreatedAt    time.Time `json:"created_at"`
}

// RegisterUsageRoutes adds usage analytics endpoints.
func RegisterUsageRoutes(mux *http.ServeMux) {
	u := &UsageAPI{
		counters: make(map[string]*UserUsage),
	}
	mux.HandleFunc("/v1/usage", u.handleGetUsage)
	mux.HandleFunc("/v1/usage/track", u.handleTrackEvent)
	mux.HandleFunc("/v1/usage/analytics", u.handleAnalytics)
	debug.Info("api", "Usage API registered (/v1/usage/*)")
}

// handleGetUsage returns usage stats for a user.
// GET /v1/usage?user_id=xxx
func (u *UsageAPI) handleGetUsage(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		userID = "default"
	}
	usage := u.getOrCreate(userID)
	json.NewEncoder(w).Encode(map[string]interface{}{"usage": usage})
}

// handleTrackEvent increments a usage counter.
// POST /v1/usage/track { "user_id": "xxx", "event": "chat_message", "value": 1 }
func (u *UsageAPI) handleTrackEvent(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID string `json:"user_id"`
		Event  string `json:"event"`
		Value  int64  `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.UserID == "" {
		req.UserID = "default"
	}

	usage := u.getOrCreate(req.UserID)
	usage.LastActive = time.Now()

	switch req.Event {
	case "chat_message":
		usage.ChatMessages += int(req.Value)
	case "tokens":
		usage.TokensUsed += req.Value
	case "agent_run":
		usage.AgentRuns += int(req.Value)
	case "task":
		usage.TasksCreated += int(req.Value)
	case "api_request":
		usage.APIRequests += int(req.Value)
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "tracked"})
}

// handleAnalytics returns aggregate analytics for admin.
// GET /v1/usage/analytics
func (u *UsageAPI) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)

	totalUsers := len(u.counters)
	totalMessages := 0
	totalTokens := int64(0)
	totalAgentRuns := 0
	activeToday := 0
	today := time.Now().Truncate(24 * time.Hour)

	for _, usage := range u.counters {
		totalMessages += usage.ChatMessages
		totalTokens += usage.TokensUsed
		totalAgentRuns += usage.AgentRuns
		if usage.LastActive.After(today) {
			activeToday++
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_users":      totalUsers,
		"active_today":     activeToday,
		"total_messages":   totalMessages,
		"total_tokens":     totalTokens,
		"total_agent_runs": totalAgentRuns,
	})
}

func (u *UsageAPI) getOrCreate(userID string) *UserUsage {
	if usage, ok := u.counters[userID]; ok {
		return usage
	}
	usage := &UserUsage{
		UserID:    userID,
		CreatedAt: time.Now(),
	}
	u.counters[userID] = usage
	return usage
}
