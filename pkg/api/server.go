package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/agent"
	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/eventbus"
	"github.com/David2024patton/iTaKAgent/web"
)

// Server provides the REST API and embedded dashboard for iTaKAgent.
type Server struct {
	mu     sync.Mutex
	orch   *agent.Orchestrator
	bus    *eventbus.EventBus
	server *http.Server
	port   int
	start  time.Time

	// WebSocket clients for live event streaming.
	wsClients   map[chan []byte]struct{}
	wsClientsMu sync.Mutex
}

// NewServer creates an API server wired to the orchestrator and event bus.
func NewServer(orch *agent.Orchestrator, bus *eventbus.EventBus, port int) *Server {
	return &Server{
		orch:      orch,
		bus:       bus,
		port:      port,
		start:     time.Now(),
		wsClients: make(map[chan []byte]struct{}),
	}
}

// Start begins listening. Non-blocking.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// ── API endpoints ─────────────────────────────────────────
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/v1/chat", s.handleChat)
	mux.HandleFunc("/v1/agents", s.handleAgents)
	mux.HandleFunc("/v1/agents/", s.handleAgentChat) // /v1/agents/{name}/chat
	mux.HandleFunc("/v1/status", s.handleStatus)
	mux.HandleFunc("/v1/tokens", s.handleTokens)
	mux.HandleFunc("/v1/doctor", s.handleDoctor)
	mux.HandleFunc("/v1/events", s.handleEventsWS)
	mux.HandleFunc("/debug/snapshot", s.handleDebugSnapshot)

	// ── Dashboard static files (embedded) ─────────────────────
	// Serve web/index.html, web/styles.css, web/app.js from Go embed.
	staticFS, err := fs.Sub(web.Assets, ".")
	if err != nil {
		return fmt.Errorf("failed to create sub-filesystem: %w", err)
	}
	fileServer := http.FileServer(http.FS(staticFS))

	// Catch-all: serve static files or fall back to index.html for SPA routes.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// If it's a known static file, serve it directly.
		if path == "/styles.css" || path == "/app.js" || path == "/favicon.ico" {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Everything else gets index.html (SPA hash routing).
		data, err := fs.ReadFile(staticFS, "index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: corsMiddleware(mux),
	}

	// Subscribe to event bus for WebSocket forwarding.
	go s.forwardEvents()

	go func() {
		debug.Info("api", "Dashboard + API on http://localhost:%d", s.port)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			debug.Error("api", "API server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the API server.
func (s *Server) Stop() {
	if s.server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s.server.Shutdown(ctx)

	// Close all WebSocket clients.
	s.wsClientsMu.Lock()
	for ch := range s.wsClients {
		close(ch)
		delete(s.wsClients, ch)
	}
	s.wsClientsMu.Unlock()

	debug.Info("api", "API server stopped")
}

// ── Core Handlers ─────────────────────────────────────────────────

// handleHealth returns a simple health check.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"version": "0.2.0",
		"uptime":  time.Since(s.start).String(),
	})
}

// ChatRequest is the JSON body for /v1/chat.
type ChatRequest struct {
	Message string `json:"message"`
}

// ChatResponse is the JSON response from /v1/chat.
type ChatResponse struct {
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

// handleChat processes a user message through the orchestrator.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "'message' is required"})
		return
	}

	debug.Info("api", "Chat request: %s", truncate(req.Message, 80))

	response, err := s.orch.Run(r.Context(), req.Message)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ChatResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ChatResponse{Response: response})
}

// handleAgents returns the list of available agents and their capabilities.
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	// Only match exact /v1/agents path (not /v1/agents/foo).
	if r.URL.Path != "/v1/agents" {
		http.NotFound(w, r)
		return
	}

	agents := make([]AgentInfo, 0, len(s.orch.Agents))
	for _, ag := range s.orch.Agents {
		agents = append(agents, AgentInfo{
			Name:        ag.Config.Name,
			Role:        ag.Config.Role,
			Personality: ag.Config.Personality,
			Goals:       ag.Config.Goals,
			Tools:       ag.Tools.Names(),
			MaxLoops:    ag.Config.MaxLoops,
			Model:       ag.Config.LLM.Model,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"agents": agents,
		"count":  len(agents),
	})
}

// AgentInfo holds the public info for an agent.
type AgentInfo struct {
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Personality string   `json:"personality"`
	Goals       []string `json:"goals"`
	Tools       []string `json:"tools"`
	MaxLoops    int      `json:"max_loops"`
	Model       string   `json:"model"`
}

// handleAgentChat routes directly to a specific agent (bypasses orchestrator).
// Path: POST /v1/agents/{name}/chat
func (s *Server) handleAgentChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}

	// Extract agent name from path: /v1/agents/{name}/chat
	path := strings.TrimPrefix(r.URL.Path, "/v1/agents/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[1] != "chat" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expected /v1/agents/{name}/chat"})
		return
	}
	agentName := parts[0]

	fa, ok := s.orch.Agents[agentName]
	if !ok {
		names := make([]string, 0, len(s.orch.Agents))
		for n := range s.orch.Agents {
			names = append(names, n)
		}
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error":     fmt.Sprintf("unknown agent: %s", agentName),
			"available": strings.Join(names, ", "),
		})
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "'message' is required"})
		return
	}

	debug.Info("api", "Direct chat -> %s: %s", agentName, truncate(req.Message, 80))

	result := fa.Run(r.Context(), agent.TaskPayload{
		Agent: agentName,
		Task:  req.Message,
	})

	if result.Success {
		writeJSON(w, http.StatusOK, ChatResponse{Response: result.Output})
	} else {
		writeJSON(w, http.StatusInternalServerError, ChatResponse{Error: result.Error})
	}
}

// handleStatus returns the current system status.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":     "0.2.0",
		"uptime":      time.Since(s.start).String(),
		"uptime_secs": time.Since(s.start).Seconds(),
		"agents":      len(s.orch.Agents),
		"memory_mb":   mem.Alloc / 1024 / 1024,
		"goroutines":  runtime.NumGoroutine(),
		"go_version":  runtime.Version(),
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
	})
}

// handleTokens returns token usage statistics.
func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request) {
	if s.orch.Tokens == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"total_tokens":   0,
			"input_tokens":   0,
			"output_tokens":  0,
			"estimated_cost": 0.0,
		})
		return
	}

	g := s.orch.Tokens.GlobalTotal()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_tokens":   g.TotalTokens,
		"input_tokens":   g.PromptTokens,
		"output_tokens":  g.CompletionTokens,
		"request_count":  g.RequestCount,
		"estimated_cost": 0.0,
		"summary":        s.orch.Tokens.Summary(),
	})
}

// handleDoctor returns the Doctor agent status.
func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	result := map[string]interface{}{
		"available": false,
	}

	if s.orch.Doctor != nil {
		result["available"] = true
		result["healing"] = s.orch.Doctor.IsHealing()
		result["fix_count"] = s.orch.Doctor.FixCount()
	}

	writeJSON(w, http.StatusOK, result)
}

// handleDebugSnapshot returns a live system snapshot.
func (s *Server) handleDebugSnapshot(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	// Build agent status.
	agentStatus := make(map[string]interface{})
	for name, ag := range s.orch.Agents {
		agentStatus[name] = map[string]interface{}{
			"role":       ag.Config.Role,
			"tools":      ag.Tools.Names(),
			"tool_count": len(ag.Tools.Names()),
			"max_loops":  ag.Config.MaxLoops,
			"model":      ag.Config.LLM.Model,
		}
	}

	// Token stats.
	tokenStats := map[string]interface{}{}
	if s.orch.Tokens != nil {
		tokenStats["summary"] = s.orch.Tokens.Summary()
	}

	// Task tracker stats.
	taskStats := map[string]interface{}{}
	if s.orch.Tasks != nil {
		taskStats["active"]   = s.orch.Tasks.ActiveCount()
		taskStats["archived"] = s.orch.Tasks.ArchivedCount()
	}

	// Memory stats from the orchestrator.
	memoryStats := map[string]interface{}{}
	if s.orch.Memory != nil {
		memoryStats["session_messages"] = s.orch.Memory.Session.Count()
		memoryStats["entities"]         = s.orch.Memory.Entities.Count()
		memoryStats["reflections"]      = s.orch.Memory.Reflections.Count()
		memoryStats["data_dir"]         = s.orch.Memory.DataDir
	}

	snapshot := map[string]interface{}{
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"version":    "0.2.0",
		"uptime":     time.Since(s.start).String(),
		"system": map[string]interface{}{
			"goroutines":   runtime.NumGoroutine(),
			"heap_mb":      mem.HeapAlloc / 1024 / 1024,
			"total_alloc_mb": mem.TotalAlloc / 1024 / 1024,
			"gc_cycles":    mem.NumGC,
			"os":           runtime.GOOS,
			"arch":         runtime.GOARCH,
			"cpus":         runtime.NumCPU(),
		},
		"agents":  agentStatus,
		"tokens":  tokenStats,
		"tasks":   taskStats,
		"memory":  memoryStats,
	}

	writeJSON(w, http.StatusOK, snapshot)
}

// ── WebSocket Event Streaming ─────────────────────────────────────

// handleEventsWS upgrades to WebSocket and streams events from the EventBus.
// Uses a simple upgrade without gorilla/websocket -- raw HTTP hijack.
func (s *Server) handleEventsWS(w http.ResponseWriter, r *http.Request) {
	// We use SSE (Server-Sent Events) as a simpler alternative to WebSocket
	// that works without external dependencies and through all proxies.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Nginx compatibility

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Register this client.
	ch := make(chan []byte, 64)
	s.wsClientsMu.Lock()
	s.wsClients[ch] = struct{}{}
	s.wsClientsMu.Unlock()

	defer func() {
		s.wsClientsMu.Lock()
		delete(s.wsClients, ch)
		s.wsClientsMu.Unlock()
	}()

	// Send initial connected event.
	fmt.Fprintf(w, "data: {\"type\":\"connected\",\"message\":\"Event stream connected\"}\n\n")
	flusher.Flush()

	// Stream events until client disconnects.
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// forwardEvents subscribes to the event bus and fans out to all SSE clients.
func (s *Server) forwardEvents() {
	if s.bus == nil {
		return
	}

	// Subscribe to ALL events (no topic filter = wildcard).
	subID, ch := s.bus.Subscribe(128)
	_ = subID

	go func() {
		for evt := range ch {
			data, err := json.Marshal(map[string]interface{}{
				"type":    "event",
				"topic":   evt.Topic,
				"level":   "info",
				"source":  evt.Agent,
				"message": evt.Message,
				"data":    evt.Data,
				"time":    evt.Timestamp.UTC().Format(time.RFC3339),
			})
			if err != nil {
				continue
			}

			s.wsClientsMu.Lock()
			for client := range s.wsClients {
				select {
				case client <- data:
				default:
					// Client buffer full, skip.
				}
			}
			s.wsClientsMu.Unlock()
		}
	}()
}

// ── Helpers ────────────────────────────────────────────────────────

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// corsMiddleware adds CORS headers for dashboard access.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// truncate shortens a string for logging.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
