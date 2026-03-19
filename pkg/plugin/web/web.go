// Package web wraps the existing REST API server as a channel plugin.
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/agent"
	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/eventbus"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
	"github.com/David2024patton/iTaKAgent/pkg/plugin"
	"github.com/David2024patton/iTaKAgent/pkg/tasks"
)

// Config holds settings for the web API plugin.
type Config struct {
	Port int `yaml:"port"`
}

// Plugin wraps the REST API server as a channel plugin.
type Plugin struct {
	port    int
	orch    *agent.Orchestrator
	bus     *eventbus.EventBus
	taskMgr *tasks.Manager
	graph   memory.GraphBackend
	dataDir string
	handler plugin.MessageHandler
	server  *http.Server
	mu      sync.Mutex
}

// New creates a web API plugin.
func New(cfg Config, orch *agent.Orchestrator, bus *eventbus.EventBus, taskMgr *tasks.Manager, graph memory.GraphBackend, dataDir string, handler plugin.MessageHandler) *Plugin {
	return &Plugin{
		port:    cfg.Port,
		orch:    orch,
		bus:     bus,
		taskMgr: taskMgr,
		graph:   graph,
		dataDir: dataDir,
		handler: handler,
	}
}

func (p *Plugin) Name() string { return "web" }

func (p *Plugin) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", p.handleHealth)
	mux.HandleFunc("/v1/chat", p.handleChat)

	p.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", p.port),
		Handler: mux,
	}

	go func() {
		debug.Info("plugin:web", "REST API listening on http://localhost:%d", p.port)
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			debug.Error("plugin:web", "Server error: %v", err)
		}
	}()

	return nil
}

func (p *Plugin) Stop() error {
	if p.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return p.server.Shutdown(ctx)
	}
	return nil
}

// Port returns the configured port.
func (p *Plugin) Port() int { return p.port }

// ── HTTP Handlers ──────────────────────────────────────────────────

func (p *Plugin) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "plugin": "web"})
}

type chatRequest struct {
	Message      string `json:"message"`
	SessionID    int64  `json:"session_id,omitempty"`
	Channel      string `json:"channel,omitempty"`
	Agent        string `json:"agent,omitempty"`
	AgencyID     int64  `json:"agency_id,omitempty"`
	SubAccountID int64  `json:"subaccount_id,omitempty"`
}

type chatResponse struct {
	Response  string `json:"response"`
	SessionID int64  `json:"session_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (p *Plugin) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "'message' is required"})
		return
	}

	channel := req.Channel
	if channel == "" {
		channel = "web"
	}

	// Inject agency context if present so the orchestrator knows which business this is about.
	userMessage := req.Message
	if req.AgencyID > 0 {
		userMessage = fmt.Sprintf("[AGENCY CONTEXT: agency_id=%d, subaccount_id=%d]\n%s", req.AgencyID, req.SubAccountID, req.Message)
	}

	msg := plugin.InboundMessage{
		Text:      userMessage,
		Channel:   channel,
		SessionID: req.SessionID,
	}

	response, err := p.handler(r.Context(), msg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, chatResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, chatResponse{Response: response, SessionID: req.SessionID})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
