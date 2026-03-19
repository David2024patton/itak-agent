package api

import (
	"encoding/json"
	"net/http"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// OnboardingAPI provides a guided setup flow for new SaaS users.
//
// What: REST endpoints for tracking onboarding progress and providing setup steps.
// Why:  New users need guidance to get value fast (connect LLM, create agent, run task).
// How:  Returns a checklist of steps and tracks completion per user.

// OnboardingStep represents a single setup step.
type OnboardingStep struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Action      string `json:"action"`      // URL or page to navigate to
	Completed   bool   `json:"completed"`
	Order       int    `json:"order"`
}

// RegisterOnboardingRoutes adds onboarding wizard endpoints.
func RegisterOnboardingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/onboarding", handleOnboarding)
	mux.HandleFunc("/v1/onboarding/complete", handleOnboardingComplete)
	mux.HandleFunc("/v1/onboarding/skip", handleOnboardingSkip)
	debug.Info("api", "Onboarding API registered (/v1/onboarding*)")
}

var defaultOnboardingSteps = []OnboardingStep{
	{
		ID: "connect_llm", Title: "Connect an LLM",
		Description: "Choose a local model (Ollama, LM Studio) or cloud API (OpenAI, Anthropic).",
		Action: "#settings", Order: 1,
	},
	{
		ID: "test_chat", Title: "Send your first message",
		Description: "Try chatting with the AI in the Chat tab.",
		Action: "#chat", Order: 2,
	},
	{
		ID: "create_agent", Title: "Create an Agent",
		Description: "Deploy a specialized agent (researcher, coder, or browser).",
		Action: "#agents", Order: 3,
	},
	{
		ID: "upload_knowledge", Title: "Add to Knowledge Base",
		Description: "Upload a document or URL to build your agent's memory.",
		Action: "#knowledge", Order: 4,
	},
	{
		ID: "create_task", Title: "Create a Task",
		Description: "Add a task to the board and assign it to an agent.",
		Action: "#tasks", Order: 5,
	},
	{
		ID: "explore_settings", Title: "Customize Settings",
		Description: "Set up your preferences, embeddings, and system config.",
		Action: "#settings", Order: 6,
	},
}

// In-memory completion tracking (per user).
var onboardingState = make(map[string]map[string]bool) // userID -> stepID -> completed

func handleOnboarding(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		userID = "default"
	}

	steps := make([]OnboardingStep, len(defaultOnboardingSteps))
	copy(steps, defaultOnboardingSteps)

	if completed, ok := onboardingState[userID]; ok {
		for i := range steps {
			if completed[steps[i].ID] {
				steps[i].Completed = true
			}
		}
	}

	completedCount := 0
	for _, s := range steps {
		if s.Completed {
			completedCount++
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"steps":     steps,
		"completed": completedCount,
		"total":     len(steps),
		"done":      completedCount == len(steps),
	})
}

func handleOnboardingComplete(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID string `json:"user_id"`
		StepID string `json:"step_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.UserID == "" {
		req.UserID = "default"
	}

	if onboardingState[req.UserID] == nil {
		onboardingState[req.UserID] = make(map[string]bool)
	}
	onboardingState[req.UserID][req.StepID] = true

	debug.Info("onboarding", "User %s completed step: %s", req.UserID, req.StepID)
	json.NewEncoder(w).Encode(map[string]string{"status": "completed"})
}

func handleOnboardingSkip(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID string `json:"user_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.UserID == "" {
		req.UserID = "default"
	}

	// Mark all steps complete (skip).
	onboardingState[req.UserID] = make(map[string]bool)
	for _, step := range defaultOnboardingSteps {
		onboardingState[req.UserID][step.ID] = true
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "skipped"})
}
