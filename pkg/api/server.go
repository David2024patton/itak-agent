package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/agent"
	"github.com/David2024patton/iTaKAgent/pkg/config"
	"github.com/David2024patton/iTaKAgent/pkg/cron"
	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/embed"
	"github.com/David2024patton/iTaKAgent/pkg/eventbus"
	"github.com/David2024patton/iTaKAgent/pkg/llm"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
	"github.com/David2024patton/iTaKAgent/pkg/seed"
	"github.com/David2024patton/iTaKAgent/pkg/skill"
	"github.com/David2024patton/iTaKAgent/pkg/tasks"
	"github.com/David2024patton/iTaKAgent/web"
)

// Server provides the REST API and embedded dashboard for iTaKAgent.
type Server struct {
	mu           sync.Mutex
	orch         *agent.Orchestrator
	bus          *eventbus.EventBus
	taskMgr      *tasks.Manager
	graphBackend memory.GraphBackend
	embedMgr     *embed.ModelManager
	scheduler    *cron.Scheduler
	skillRepo    *skill.Repository
	cfg          *config.Config
	server       *http.Server
	port         int
	start        time.Time
	dataDir      string
	visionClient *llm.VisionClient // multimodal image processing via Ollama

	// WebSocket clients for live event streaming.
	wsClients   map[chan []byte]struct{}
	wsClientsMu sync.Mutex

	// Session-to-project mapping: track which sessions have auto-created projects.
	sessionProjects map[int]string // session_id -> project graph node ID
}

// NewServer creates an API server wired to the orchestrator and event bus.
func NewServer(orch *agent.Orchestrator, bus *eventbus.EventBus, taskMgr *tasks.Manager, graphBackend memory.GraphBackend, port int, dataDir string, skillRepo *skill.Repository, cfg *config.Config) *Server {
	// Create cron scheduler with a no-op dispatch for now.
	// Real dispatch will use the orchestrator once agent context is available.
	scheduler := cron.NewScheduler(dataDir, func(job *cron.Job) {
		debug.Info("cron", "Dispatching job %s to agent %s: %s", job.Name, job.Agent, job.Prompt)
	})

	// Initialize vision client for image OCR/description.
	// Uses the orchestrator's LLM config if it's pointing at Ollama.
	var vc *llm.VisionClient
	if cfg != nil && cfg.Orchestrator.LLM.Provider == "ollama" {
		ollamaBase := cfg.Orchestrator.LLM.APIBase
		if ollamaBase == "" {
			ollamaBase = "http://localhost:11434"
		}
		// VisionClient uses Ollama's native /api/chat, not OpenAI /v1/chat/completions.
		// Strip /v1 suffix if present so we get the raw Ollama base URL.
		visionBase := strings.TrimSuffix(ollamaBase, "/v1")
		vc = llm.NewVisionClient(visionBase, cfg.Orchestrator.LLM.Model)
		debug.Info("api", "Vision client initialized: %s on %s", cfg.Orchestrator.LLM.Model, visionBase)
	}

	return &Server{
		orch:            orch,
		bus:             bus,
		taskMgr:         taskMgr,
		graphBackend:    graphBackend,
		embedMgr:        embed.NewModelManager(filepath.Join(dataDir, "models", "embed")),
		scheduler:       scheduler,
		skillRepo:       skillRepo,
		cfg:             cfg,
		port:            port,
		dataDir:         dataDir,
		start:           time.Now(),
		wsClients:       make(map[chan []byte]struct{}),
		sessionProjects: make(map[int]string),
		visionClient:    vc,
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
	
	// Tasks endpoints
	mux.HandleFunc("/v1/tasks", s.handleTasks)
	mux.HandleFunc("/v1/tasks/", s.handleTaskByID)

	// Approval endpoints (human-in-the-loop)
	mux.HandleFunc("/v1/approvals", s.handleApprovals)
	mux.HandleFunc("/v1/approvals/", s.handleApprovalByID)

	// Graph visualization API
	RegisterGraphRoutes(mux, s.graphBackend)

	// ZIP ingestion API
	RegisterIngestRoutes(mux, s.graphBackend)

	// Knowledge API (repo ingestion, unified search, auto-docs, deps)
	RegisterKnowledgeRoutes(mux, s.graphBackend)

	// Embedding model management API
	RegisterEmbedRoutes(mux, s.embedMgr)

	// DebugMemory + WebResearch API
	RegisterDebugResearchRoutes(mux, s.graphBackend)

	// Ecosystem local-directory ingestion API
	RegisterEcosystemRoutes(mux, s.graphBackend)

	// Persona management API
	RegisterPersonaRoutes(mux, s.graphBackend)

	// SQL query API
	RegisterSQLRoutes(mux, s.graphBackend)

	// Model management API (provider catalog, model auto-load, global config)
	RegisterModelRoutes(mux, s.graphBackend)

	// Agent activity persistence API
	RegisterActivityRoutes(mux, s.graphBackend)

	// Plugin auto-discovery API
	RegisterPluginRoutes(mux, s.dataDir)

	// OpenAPI/Swagger documentation
	RegisterOpenAPIRoutes(mux)

	// Agency multi-tenant management API
	RegisterAgencyRoutes(mux, s.graphBackend)

	// Project management API
	RegisterProjectRoutes(mux, s.graphBackend)

	// Project file serving API (canvas live preview)
	RegisterProjectServeRoutes(mux, s.dataDir)

	// CRM contacts API
	RegisterContactRoutes(mux, s.graphBackend)

	// Pipeline/workflow automation API
	RegisterPipelineRoutes(mux, s.graphBackend)

	// Report templates API
	RegisterReportRoutes(mux, s.graphBackend)

	// (Knowledge API already registered above with the other graph APIs)

	// Encrypted credentials vault API
	credAPI := registerCredentialsAPI(mux, s.graphBackend)

	// ── Connector framework (Stripe reference + user-extensible) ──
	connectorRegistry := NewConnectorRegistry(s.graphBackend, credAPI)
	connectorRegistry.Register(NewStripeConnector(connectorRegistry))
	RegisterConnectorRoutes(mux, connectorRegistry)

	// Cron automations scheduler API
	RegisterCronRoutes(mux, s.scheduler)
	s.scheduler.Start()

	// Marketplace API (skill store, agent catalog, plugin status)
	RegisterMarketplaceRoutes(mux, s.skillRepo, s.cfg, s.dataDir)

	// Session management API
	RegisterSessionRoutes(mux, s.orch)

	// Seed knowledge injection (first boot only).
	if s.graphBackend != nil {
		if itakDB, ok := s.graphBackend.(*memory.ITakDBBackend); ok {
			seed.InjectIfNeeded(itakDB.DB())
		}
	}

	// Superagent generated web assets
	slidesDir := filepath.Join(s.dataDir, "slides")
	reportsDir := filepath.Join(s.dataDir, "reports")
	mux.Handle("/slides/", http.StripPrefix("/slides/", http.FileServer(http.Dir(slidesDir))))
	mux.Handle("/reports/", http.StripPrefix("/reports/", http.FileServer(http.Dir(reportsDir))))

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
		if path == "/styles.css" || path == "/app.js" || path == "/favicon.ico" || path == "/graph.html" {
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
		Addr:    fmt.Sprintf("0.0.0.0:%d", s.port),
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
	Message     string           `json:"message"`
	SessionID   int              `json:"session_id,omitempty"` // resume existing session (0 = new)
	Channel     string           `json:"channel,omitempty"`    // web, discord, whatsapp, telegram, api
	Agent       string           `json:"agent,omitempty"`      // route to specific agent
	Attachments []ChatAttachment `json:"attachments,omitempty"`
}

// ChatAttachment represents a file (image, document) attached to a chat message.
type ChatAttachment struct {
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data"` // base64 data URL or raw base64
}

// ChatResponse is the JSON response from /v1/chat.
type ChatResponse struct {
	Response  string `json:"response"`
	SessionID int    `json:"session_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"` // auto-created project for this session
	Error     string `json:"error,omitempty"`
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

	// Route /deep-research commands sent via the UI into full Swarm mode
	if strings.HasPrefix(req.Message, "/deep-research ") {
		topic := strings.TrimSpace(strings.TrimPrefix(req.Message, "/deep-research "))
		if topic == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Usage: /deep-research <topic>"})
			return
		}

		req.Message = fmt.Sprintf(`DEEP RESEARCH INITIATED: The user has requested a comprehensive, multi-agent deep research report on the topic: "%s".

Your mandatory delegation strategy:
1. Deploy the "researcher" or "browser" agent to scour the web, perform searches, and gather extensive raw data, statistics, and citations about this topic.
2. Deploy the "operator" or "coder" agent to analyze the gathered data, structure it, and synthesize it.
3. Deploy the "architect" or "operator" agent to use the 'report_generate' or 'slide_generate' tool to create a rich, final output artifact (JSON report or HTML slide deck) presenting the synthesized findings.

Do not ask the user for permission. Execute all steps autonomously to produce the final presentation artifact.`, topic)
	}

	debug.Info("api", "Chat request: %s", truncate(req.Message, 80))

	// Process image attachments through vision model (OCR/description).
	if len(req.Attachments) > 0 && s.visionClient != nil {
		var imageDataURLs []string
		for _, att := range req.Attachments {
			if strings.HasPrefix(att.MimeType, "image/") && att.Data != "" {
				imageDataURLs = append(imageDataURLs, att.Data)
			}
		}

		if len(imageDataURLs) > 0 {
			debug.Info("api", "Processing %d image(s) through vision model...", len(imageDataURLs))

			userMsg := req.Message
			if userMsg == fmt.Sprintf("[%d image(s) attached]", len(req.Attachments)) {
				userMsg = "" // Generic placeholder, let vision model describe freely.
			}

			visionCtx, visionCancel := context.WithTimeout(r.Context(), 45*time.Second)
			defer visionCancel()

			description, err := s.visionClient.DescribeImages(visionCtx, imageDataURLs, userMsg)
			if err != nil {
				debug.Warn("api", "Vision processing failed: %v", err)
				req.Message = req.Message + "\n\n[Image attached but vision processing failed: " + err.Error() + "]"
			} else if description != "" {
				// Inject the vision output into the message so the orchestrator can use it.
				if req.Message == "" || req.Message == fmt.Sprintf("[%d image(s) attached]", len(req.Attachments)) {
					req.Message = fmt.Sprintf("[Vision Analysis of attached image]:\n%s", description)
				} else {
					req.Message = req.Message + fmt.Sprintf("\n\n[Vision Analysis of attached image]:\n%s", description)
				}
				debug.Info("api", "Vision description injected (%d chars)", len(description))
			}
		}
	} else if len(req.Attachments) > 0 {
		// No vision client available, just note the attachments.
		var names []string
		for _, att := range req.Attachments {
			names = append(names, fmt.Sprintf("%s (%s)", att.Filename, att.MimeType))
		}
		req.Message = req.Message + "\n\n[" + fmt.Sprintf("%d", len(req.Attachments)) + " file(s) attached: " + strings.Join(names, ", ") + "]"
	}

	// Session management: create or resume session.
	sessionID := req.SessionID
	channel := req.Channel
	if channel == "" {
		channel = "web"
	}
	if sessionID == 0 && s.orch.Memory != nil {
		// Auto-create a new session with first message as title.
		title := truncate(req.Message, 60)
		sessionID = s.orch.Memory.Archive.StartSessionWithMeta(channel, title)

		// Also update the graph Session node with the descriptive title
		// so sessions display readable names in the graph explorer.
		if s.orch.Memory.Activity != nil {
			sesID := s.orch.Memory.Activity.SessionID()
			if sesID != "" && s.graphBackend != nil {
				if itakDB, ok := s.graphBackend.(*memory.ITakDBBackend); ok {
					db := itakDB.DB()
					sesNodes, _ := db.Graph.FindByProperty("session_id", sesID)
					if len(sesNodes) > 0 {
						db.Graph.UpdateNode(sesNodes[0].ID, map[string]interface{}{
							"title": title,
							"name":  title,
						})
					}
				}
			}
		}
	}

	// Auto-create a project for new sessions so tasks are grouped per chat.
	if sessionID > 0 {
		s.mu.Lock()
		if _, exists := s.sessionProjects[sessionID]; !exists && s.graphBackend != nil {
			projectName := "Session: " + truncate(req.Message, 50)
			if itakDB, ok := s.graphBackend.(*memory.ITakDBBackend); ok {
				props := map[string]interface{}{
					"name":       projectName,
					"status":     "active",
					"session_id": fmt.Sprintf("%d", sessionID),
					"auto_mode":  "true",
					"created_at": time.Now().Format(time.RFC3339),
				}
				nodeID, createErr := itakDB.DB().CreateNode([]string{"Project"}, props, nil)
				if createErr == nil {
					s.sessionProjects[sessionID] = fmt.Sprintf("%d", nodeID)
					debug.Info("api", "Auto-created project %d for session %d: %s", nodeID, sessionID, projectName)
				}
			}
		}
		s.mu.Unlock()
	}

	// Log the user message to the session archive.
	if s.orch.Memory != nil && sessionID > 0 {
		s.orch.Memory.Archive.LogMessage(memory.LogMessage{
			Role:      "user",
			Content:   req.Message,
			Timestamp: time.Now(),
		})
	}

	// Wire the dashboard task manager and project context into the orchestrator
	// so delegations create real, visible tasks on the board.
	s.orch.DashboardTasks = s.taskMgr
	s.mu.Lock()
	s.orch.ActiveProjectID = s.sessionProjects[sessionID]
	s.mu.Unlock()

	response, err := s.orch.Run(r.Context(), req.Message)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ChatResponse{Error: err.Error(), SessionID: sessionID})
		return
	}

	// Persist messages to disk after each exchange so sessions can be resumed.
	if s.orch.Memory != nil && sessionID > 0 {
		inMemMsgs := s.orch.Memory.Archive.CurrentMessages()
		if len(inMemMsgs) > 0 {
			s.orch.Memory.Archive.SaveSessionMessages(sessionID, inMemMsgs)
			s.orch.Memory.Archive.UpdateSessionMeta(sessionID, "", "", len(inMemMsgs))
		}
	}

	// Auto-compact if context gets too large (over 40 messages).
	if s.orch.Memory != nil && sessionID > 0 {
		msgs, _ := s.orch.Memory.Archive.LoadConversation(sessionID)
		if len(msgs) > 40 {
			go s.orch.Memory.Archive.CompactSession(sessionID, 20)
		}
	}

	// Auto-record agent activity for the embed pipeline.
	if s.bus != nil {
		s.bus.Publish(eventbus.Event{
			Topic:     "agent.chat_complete",
			Agent:     "orchestrator",
			Message:   truncate(response, 200),
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"input":  truncate(req.Message, 200),
				"output": truncate(response, 500),
			},
		})
	}

	// Persist to graph as AgentActivity node (embed agent pipeline).
	if s.graphBackend != nil {
		if itakDB, ok := s.graphBackend.(*memory.ITakDBBackend); ok {
			db := itakDB.DB()
			db.CreateNode([]string{"AgentActivity"}, map[string]interface{}{
				"agent":      "orchestrator",
				"action":     "chat",
				"data":       truncate(response, 1000),
				"session_id": sessionID,
				"channel":    channel,
				"timestamp":  time.Now().Format(time.RFC3339),
				"status":     "success",
			}, nil)
		}
	}

	// Look up the project_id for this session.
	var projectID string
	s.mu.Lock()
	projectID = s.sessionProjects[sessionID]
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, ChatResponse{Response: response, SessionID: sessionID, ProjectID: projectID})
}

// handleAgents returns the list of available agents and their capabilities.
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	// Only match exact /v1/agents path (not /v1/agents/foo).
	if r.URL.Path != "/v1/agents" {
		http.NotFound(w, r)
		return
	}

	// Core agents from YAML config.
	agents := make([]AgentInfo, 0, len(s.orch.Agents)+150)
	// Agents that should NOT be tagged "core" (they're specialized, not system agents).
	crmOverrides := map[string]struct{ Source, Category, Division string }{
		"ghl": {"focus", "crm", "Sales"},
	}
	for _, ag := range s.orch.Agents {
		source := "core"
		category := ""
		division := ""
		if ov, ok := crmOverrides[ag.Config.Name]; ok {
			source = ov.Source
			category = ov.Category
			division = ov.Division
		}
		agents = append(agents, AgentInfo{
			Name:        ag.Config.Name,
			Role:        ag.Config.Role,
			Personality: ag.Config.Personality,
			Goals:       ag.Config.Goals,
			Tools:       ag.Tools.Names(),
			MaxLoops:    ag.Config.MaxLoops,
			Model:       ag.Config.LLM.Model,
			Source:      source,
			Category:    category,
			Division:    division,
		})
	}

	// Focus agents from seed catalog.
	for _, fa := range seed.GetFocusAgents() {
		agents = append(agents, AgentInfo{
			Name:        fa.Name,
			Role:        fa.Role,
			Personality: fa.Description,
			Goals:       fa.Goals,
			Tools:       fa.Tools,
			Source:      fa.Source,
			Category:    fa.Category,
			Division:    fa.Division,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"agents":    agents,
		"count":     len(agents),
		"divisions": seed.GetDivisions(),
	})
}

// AgentInfo holds the public info for an agent.
type AgentInfo struct {
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Personality string   `json:"personality"`
	Goals       []string `json:"goals"`
	Tools       []string `json:"tools"`
	MaxLoops    int      `json:"max_loops,omitempty"`
	Model       string   `json:"model,omitempty"`
	Source      string   `json:"source,omitempty"`   // core, focus, agency
	Category    string   `json:"category,omitempty"` // marketing, engineering, design, etc.
	Division    string   `json:"division,omitempty"` // Engineering, Design, Sales, etc.
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

// ── Task Management ────────────────────────────────────────────────

// handleTasks handles GET /v1/tasks and POST /v1/tasks
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		allTasks := s.taskMgr.GetTasks()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"tasks": allTasks,
			"count": len(allTasks),
		})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Title       string        `json:"title"`
			Description string        `json:"description"`
			Priority    tasks.Priority `json:"priority"`
			Labels      []string      `json:"labels"`
			DueDate     *time.Time    `json:"due_date"`
			ProjectID   string        `json:"project_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Title == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title required"})
			return
		}

		t, err := s.taskMgr.CreateTask(req.Title, req.Description, req.Priority, req.Labels, req.DueDate, req.ProjectID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// Publish task.created event for real-time dashboard.
		if s.bus != nil {
			s.bus.Publish(eventbus.Event{
				Topic:     "task.created",
				Agent:     "system",
				Message:   "Task created: " + req.Title,
				Timestamp: time.Now(),
				Data:      map[string]interface{}{"task_id": t.ID, "title": t.Title},
			})
		}

		writeJSON(w, http.StatusCreated, t)
		return
	}

	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

// handleTaskByID handles GET, PUT, DELETE for /v1/tasks/{id}
func (s *Server) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/tasks/")

	// Handle /v1/tasks/{id}/comments
	if strings.HasSuffix(id, "/comments") {
		id = strings.TrimSuffix(id, "/comments")
		if r.Method == http.MethodPost {
			var req struct {
				Author string `json:"author"`
				Text   string `json:"text"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
				return
			}
			t, err := s.taskMgr.AddComment(id, req.Author, req.Text)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, t)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Handle /v1/tasks/{id}/execute - dispatch task to orchestrator
	if strings.HasSuffix(id, "/execute") {
		id = strings.TrimSuffix(id, "/execute")
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		t, err := s.taskMgr.GetTask(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}

		// Move task to In Progress immediately.
		s.taskMgr.UpdateTaskStatus(id, tasks.StatusInProgress)

		// Build the prompt from the task title + description.
		prompt := t.Title
		if t.Description != "" {
			prompt += "\n\nContext: " + t.Description
		}

		// Wire orchestrator with task context and run in background.
		s.orch.DashboardTasks = s.taskMgr
		s.orch.ActiveProjectID = t.ProjectID

		go func() {
			result, runErr := s.orch.Run(r.Context(), prompt)
			if runErr != nil {
				s.taskMgr.UpdateTaskStatus(id, tasks.StatusTodo)
				s.taskMgr.AddComment(id, "orchestrator", "Execution failed: "+runErr.Error())
				return
			}
			// Post result and move to Review for auto-checks.
			s.taskMgr.AddComment(id, "orchestrator", truncate(result, 800))
			s.taskMgr.UpdateTaskStatus(id, tasks.StatusReview)
		}()

		writeJSON(w, http.StatusAccepted, map[string]string{
			"status":  "executing",
			"task_id": id,
			"message": "Task dispatched to orchestrator",
		})
		return
	}

	// Handle /v1/tasks/{id}/attachments - file upload
	if strings.HasSuffix(id, "/attachments") {
		id = strings.TrimSuffix(id, "/attachments")
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		// Parse multipart form with 32MB limit.
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart: " + err.Error()})
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file field required"})
			return
		}
		defer file.Close()

		// Save to attachments directory.
		attDir := filepath.Join(s.dataDir, "tasks", "attachments", id)
		os.MkdirAll(attDir, 0755)
		savePath := filepath.Join(attDir, header.Filename)
		out, err := os.Create(savePath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save: " + err.Error()})
			return
		}
		defer out.Close()
		written, _ := io.Copy(out, file)

		// Record attachment on the task.
		mimeType := header.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		t, err := s.taskMgr.AddAttachment(id, header.Filename, savePath, mimeType, written)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, t)
		return
	}

	// Handle /v1/tasks/{id}/wake - resume a Waiting task via webhook
	if strings.HasSuffix(id, "/wake") {
		id = strings.TrimSuffix(id, "/wake")
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}
		var req struct {
			WebhookID string `json:"webhook_id"`
			Payload   string `json:"payload,omitempty"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.WebhookID == "" {
			req.WebhookID = "manual"
		}
		t, err := s.taskMgr.WakeTask(id, req.WebhookID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		// If there was a payload, add it as a comment for context.
		if req.Payload != "" {
			s.taskMgr.AddComment(id, "webhook", req.Payload)
		}
		writeJSON(w, http.StatusOK, t)
		return
	}

	// Handle /v1/tasks/{id}/children - spawn a child task
	if strings.HasSuffix(id, "/children") {
		id = strings.TrimSuffix(id, "/children")
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}
		var req struct {
			Title       string         `json:"title"`
			Description string         `json:"description"`
			Priority    tasks.Priority `json:"priority"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		child, err := s.taskMgr.SpawnChildTask(id, req.Title, req.Description, req.Priority)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, child)
		return
	}

	// Handle /v1/tasks/{id}/confidence - set confidence score
	if strings.HasSuffix(id, "/confidence") {
		id = strings.TrimSuffix(id, "/confidence")
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}
		var req struct {
			Score int `json:"score"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		t, err := s.taskMgr.SetConfidence(id, req.Score)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, t)
		return
	}

	if id == "" || id == r.URL.Path {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing task id"})
		return
	}

	if r.Method == http.MethodGet {
		t, err := s.taskMgr.GetTask(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, t)
		return
	}

	if r.Method == http.MethodPut {
		var req struct {
			Title         string          `json:"title"`
			Description   string          `json:"description"`
			Status        tasks.Status    `json:"status"`
			Priority      tasks.Priority  `json:"priority"`
			Labels        []string        `json:"labels"`
			DueDate       *time.Time      `json:"due_date"`
			SubItems      []tasks.SubItem `json:"sub_items"`
			ProjectID     string          `json:"project_id"`
			BlockedBy     []string        `json:"blocked_by"`
			Blocks        []string        `json:"blocks"`
			RecurPattern  string          `json:"recur_pattern"`
			AssignedAgent string          `json:"assigned_agent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		t, err := s.taskMgr.UpdateTask(id, req.Title, req.Description, req.Status, req.AssignedAgent, req.Priority, req.Labels, req.DueDate, req.SubItems, req.ProjectID, req.BlockedBy, req.Blocks, req.RecurPattern)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// Publish task.updated event for real-time dashboard.
		if s.bus != nil {
			s.bus.Publish(eventbus.Event{
				Topic:     "task.updated",
				Agent:     req.AssignedAgent,
				Message:   fmt.Sprintf("Task %s: %s", string(req.Status), req.Title),
				Timestamp: time.Now(),
				Data:      map[string]interface{}{"task_id": id, "status": string(req.Status), "agent": req.AssignedAgent},
			})
		}

		writeJSON(w, http.StatusOK, t)
		return
	}

	if r.Method == http.MethodDelete {
		if err := s.taskMgr.DeleteTask(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// Publish task.deleted event.
		if s.bus != nil {
			s.bus.Publish(eventbus.Event{
				Topic:     "task.deleted",
				Agent:     "system",
				Message:   "Task deleted",
				Timestamp: time.Now(),
				Data:      map[string]interface{}{"task_id": id},
			})
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}

	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
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
				"agent":   evt.Agent,
				"tool":    evt.Tool,
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

// handleApprovals returns pending approval requests.
func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "GET required"})
		return
	}

	if s.taskMgr == nil || s.taskMgr.Approvals == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"approvals": []interface{}{}, "count": 0})
		return
	}

	pending := s.taskMgr.Approvals.Pending()
	writeJSON(w, http.StatusOK, map[string]interface{}{"approvals": pending, "count": len(pending)})
}

// handleApprovalByID handles approve/reject actions.
func (s *Server) handleApprovalByID(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/v1/approvals/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approval ID required"})
		return
	}

	var req struct {
		Action string `json:"action"` // "approve" or "reject"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if s.taskMgr == nil || s.taskMgr.Approvals == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "approval system not available"})
		return
	}

	var err error
	switch req.Action {
	case "approve":
		err = s.taskMgr.Approvals.Approve(id, "user")
	case "reject":
		err = s.taskMgr.Approvals.Reject(id, "user")
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be 'approve' or 'reject'"})
		return
	}

	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": req.Action + "d"})
}

// truncate shortens a string for logging.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
