package torch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Server is the OpenAI-compatible HTTP server for GOTorch.
// It wraps an Engine and serves chat completions on localhost.
type Server struct {
	engine    Engine
	port      int
	server    *http.Server
	startTime time.Time
	mu        sync.RWMutex
}

// NewServer creates a GOTorch server bound to the given port.
func NewServer(engine Engine, port int) *Server {
	s := &Server{
		engine: engine,
		port:   port,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	return s
}

// Start starts the HTTP server. Blocks until the server is stopped.
func (s *Server) Start() error {
	s.startTime = time.Now()
	fmt.Printf("[GOTorch] Server starting on http://localhost:%d\n", s.port)
	fmt.Printf("[GOTorch] Model: %s\n", s.engine.ModelName())
	fmt.Printf("[GOTorch] Endpoints:\n")
	fmt.Printf("  POST /v1/chat/completions\n")
	fmt.Printf("  GET  /v1/models\n")
	fmt.Printf("  GET  /health\n")

	err := s.server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

// Port returns the port the server is bound to.
func (s *Server) Port() int {
	return s.port
}

// handleChatCompletions handles POST /v1/chat/completions.
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed, use POST")
		return
	}

	// Read and parse request body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	var req ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages array is empty")
		return
	}

	// Build completion params from request.
	params := CompletionParams{
		MaxTokens: req.MaxTokens,
		Stop:      req.Stop,
	}
	if req.Temperature != nil {
		params.Temperature = *req.Temperature
	} else {
		params.Temperature = 0.7
	}
	if req.TopP != nil {
		params.TopP = *req.TopP
	} else {
		params.TopP = 0.9
	}
	if params.MaxTokens == 0 {
		params.MaxTokens = 512
	}

	// Run inference.
	start := time.Now()
	result, err := s.engine.Complete(r.Context(), req.Messages, params)
	elapsed := time.Since(start)

	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("inference error: %v", err))
		return
	}

	// Estimate token counts (rough: 4 chars per token).
	promptTokens := 0
	for _, m := range req.Messages {
		promptTokens += len(m.Content) / 4
	}
	completionTokens := len(result) / 4

	// Build response in OpenAI format.
	resp := ChatResponse{
		ID:      fmt.Sprintf("gotorch-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   s.engine.ModelName(),
		Choices: []ChatChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    "assistant",
					Content: result,
				},
				FinishReason: "stop",
			},
		},
		Usage: ChatUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}

	_ = elapsed // Will be used for metrics later.

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleModels handles GET /v1/models.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed, use GET")
		return
	}

	resp := ModelsResponse{
		Object: "list",
		Data: []ModelInfo{
			{
				ID:      s.engine.ModelName(),
				Object:  "model",
				OwnedBy: "gotorch",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleHealth handles GET /health.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startTime).Round(time.Second).String()

	resp := HealthResponse{
		Status:    "ok",
		ModelName: s.engine.ModelName(),
		Uptime:    uptime,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// writeError sends a JSON error response in OpenAI error format.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	errResp := struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}{
		Error: struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		}{
			Message: message,
			Type:    errorTypeFromStatus(status),
		},
	}
	json.NewEncoder(w).Encode(errResp)
}

func errorTypeFromStatus(status int) string {
	switch {
	case status == 400:
		return "invalid_request_error"
	case status == 404:
		return "not_found_error"
	case status == 405:
		return "invalid_request_error"
	case status >= 500:
		return "server_error"
	default:
		return "api_error"
	}
}

// BuildPrompt converts chat messages into a single prompt string
// for models that don't support chat format natively.
func BuildPrompt(messages []ChatMessage) string {
	var sb strings.Builder
	for _, m := range messages {
		switch m.Role {
		case "system":
			sb.WriteString(fmt.Sprintf("System: %s\n\n", m.Content))
		case "user":
			sb.WriteString(fmt.Sprintf("User: %s\n\n", m.Content))
		case "assistant":
			sb.WriteString(fmt.Sprintf("Assistant: %s\n\n", m.Content))
		default:
			sb.WriteString(fmt.Sprintf("%s: %s\n\n", m.Role, m.Content))
		}
	}
	sb.WriteString("Assistant: ")
	return sb.String()
}
