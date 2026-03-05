package torch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	logger    *log.Logger
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithLogger sets a debug logger for request/response tracing.
func WithLogger(l *log.Logger) ServerOption {
	return func(s *Server) { s.logger = l }
}

// NewServer creates a GOTorch server bound to the given port.
func NewServer(engine Engine, port int, opts ...ServerOption) *Server {
	s := &Server{
		engine: engine,
		port:   port,
	}
	for _, opt := range opts {
		opt(s)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
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

// debugf logs a formatted debug message if a logger is set.
func (s *Server) debugf(format string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
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
	start := time.Now()
	s.debugf("[REQ] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

	if r.Method != http.MethodPost {
		s.debugf("[ERR] method not allowed: %s", r.Method)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed, use POST")
		return
	}

	// Read and parse request body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.debugf("[ERR] read body: %v", err)
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	var req ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.debugf("[ERR] parse JSON: %v", err)
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if len(req.Messages) == 0 {
		s.debugf("[ERR] empty messages")
		writeError(w, http.StatusBadRequest, "messages array is empty")
		return
	}

	s.debugf("[INF] model=%s msgs=%d max_tokens=%d", req.Model, len(req.Messages), req.MaxTokens)

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
	result, err := s.engine.Complete(r.Context(), req.Messages, params)
	elapsed := time.Since(start)

	if err != nil {
		s.debugf("[ERR] inference failed after %s: %v", elapsed, err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("inference error: %v", err))
		return
	}

	// Use actual token counts from engine metrics (much more accurate than char estimates).
	stats := s.engine.GetStats()
	promptTokens := 0
	completionTokens := 0
	if stats.LastMetrics != nil {
		promptTokens = stats.LastMetrics.PromptTokens
		completionTokens = stats.LastMetrics.CompletionTokens
	}

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

	// Log with performance data.
	tokSec := 0.0
	if stats.LastMetrics != nil {
		tokSec = stats.LastMetrics.TokensPerSecond
	}
	s.debugf("[RES] 200 OK in %s | %d tok | %.1f tok/s", elapsed, resp.Usage.TotalTokens, tokSec)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleModels handles GET /v1/models.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	s.debugf("[REQ] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

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

	s.debugf("[RES] 200 OK models=%d", len(resp.Data))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleHealth handles GET /health.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.debugf("[REQ] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
	uptime := time.Since(s.startTime).Round(time.Second).String()

	stats := s.engine.GetStats()
	currentRes := CaptureResources()

	// Extended health response with performance data.
	resp := map[string]interface{}{
		"status": "ok",
		"model":  s.engine.ModelName(),
		"uptime": uptime,
		"port":   s.port,
		"performance": map[string]interface{}{
			"model_load_time_ms": stats.ModelLoadTime.Milliseconds(),
			"request_count":      stats.RequestCount,
			"total_tokens_gen":   stats.TotalTokensGen,
			"avg_tokens_per_sec": fmt.Sprintf("%.1f", stats.AvgTokPerSec),
		},
		"resources": map[string]interface{}{
			"current": map[string]interface{}{
				"heap_mb":    fmt.Sprintf("%.1f", currentRes.HeapAllocMB),
				"sys_mb":     fmt.Sprintf("%.1f", currentRes.SysMB),
				"goroutines": currentRes.GoRoutines,
				"gc_cycles":  currentRes.NumGC,
			},
			"pre_model_load": map[string]interface{}{
				"heap_mb": fmt.Sprintf("%.1f", stats.PreLoadRes.HeapAllocMB),
				"sys_mb":  fmt.Sprintf("%.1f", stats.PreLoadRes.SysMB),
			},
			"post_model_load": map[string]interface{}{
				"heap_mb": fmt.Sprintf("%.1f", stats.PostLoadRes.HeapAllocMB),
				"sys_mb":  fmt.Sprintf("%.1f", stats.PostLoadRes.SysMB),
			},
		},
	}

	// Add last request metrics if available.
	if stats.LastMetrics != nil {
		resp["last_request"] = map[string]interface{}{
			"prompt_tokens":     stats.LastMetrics.PromptTokens,
			"completion_tokens": stats.LastMetrics.CompletionTokens,
			"tokens_per_second": fmt.Sprintf("%.1f", stats.LastMetrics.TokensPerSecond),
			"prompt_ms":         stats.LastMetrics.PromptDuration.Milliseconds(),
			"gen_ms":            stats.LastMetrics.GenDuration.Milliseconds(),
			"total_ms":          stats.LastMetrics.TotalDuration.Milliseconds(),
		}
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

// handleRoot serves the landing page on GET /.
// Returns a flashy HTML page in browsers, plain text for curl/API clients.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	// Only match exact root path.
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	s.debugf("[REQ] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

	// Plain text for curl/wget/API clients.
	userAgent := r.Header.Get("User-Agent")
	if !strings.Contains(userAgent, "Mozilla") {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "GOTorch is running.\nUptime: %s\n", time.Since(s.startTime).Round(time.Second))
		return
	}

	// HTML landing page for browsers.
	uptime := time.Since(s.startTime).Round(time.Second).String()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, landingPageHTML, uptime, s.port)
}

const landingPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>GOTorch</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: 'Segoe UI', system-ui, -apple-system, sans-serif;
    background: #0a0a0f;
    color: #e0e0e0;
    min-height: 100vh;
    display: flex; align-items: center; justify-content: center;
    overflow: hidden;
  }
  .bg-glow {
    position: fixed; top: 50%%; left: 50%%; transform: translate(-50%%, -50%%);
    width: 600px; height: 600px; border-radius: 50%%;
    background: radial-gradient(circle, rgba(255,100,0,0.08) 0%%, transparent 70%%);
    animation: breathe 4s ease-in-out infinite;
  }
  @keyframes breathe { 0%%,100%% { transform: translate(-50%%,-50%%) scale(1); opacity:0.6; } 50%% { transform: translate(-50%%,-50%%) scale(1.15); opacity:1; } }
  .card {
    position: relative; z-index: 1;
    text-align: center; padding: 48px 56px;
    background: rgba(18,18,28,0.85); border: 1px solid rgba(255,100,0,0.15);
    border-radius: 20px;
    backdrop-filter: blur(20px);
    box-shadow: 0 0 80px rgba(255,80,0,0.06);
  }
  .logo {
    font-size: 52px; font-weight: 800; letter-spacing: -2px;
    background: linear-gradient(135deg, #ff6a00, #ff3d00, #ff6a00);
    background-size: 200%% 200%%;
    -webkit-background-clip: text; -webkit-text-fill-color: transparent;
    animation: shimmer 3s ease-in-out infinite;
  }
  @keyframes shimmer { 0%%,100%% { background-position: 0%% 50%%; } 50%% { background-position: 100%% 50%%; } }
  .torch-icon {
    display: inline-block; font-size: 40px; margin-bottom: 8px;
    animation: flicker 2s ease-in-out infinite;
  }
  @keyframes flicker { 0%%,100%% { opacity:1; transform:scale(1); } 30%% { opacity:0.85; transform:scale(0.97); } 60%% { opacity:1; transform:scale(1.02); } }
  .status {
    display: inline-flex; align-items: center; gap: 8px;
    margin: 20px 0 24px; padding: 8px 20px;
    background: rgba(0,200,80,0.08); border: 1px solid rgba(0,200,80,0.25);
    border-radius: 50px; font-size: 14px; color: #4ade80;
  }
  .pulse {
    width: 8px; height: 8px; border-radius: 50%%;
    background: #4ade80; position: relative;
  }
  .pulse::after {
    content: ''; position: absolute; inset: -4px;
    border-radius: 50%%; background: rgba(74,222,128,0.3);
    animation: ping 2s cubic-bezier(0,0,0.2,1) infinite;
  }
  @keyframes ping { 75%%,100%% { transform: scale(2.5); opacity: 0; } }
  .info { margin: 16px 0 0; }
  .info-row {
    display: flex; justify-content: space-between; padding: 8px 0;
    border-bottom: 1px solid rgba(255,255,255,0.05); font-size: 14px;
  }
  .info-row:last-child { border-bottom: none; }
  .label { color: #888; } .value { color: #fff; font-family: 'Cascadia Code', monospace; }
  .endpoints { margin-top: 24px; text-align: left; }
  .endpoints h3 { font-size: 12px; text-transform: uppercase; letter-spacing: 2px; color: #666; margin-bottom: 12px; }
  .ep {
    display: flex; align-items: center; gap: 10px;
    padding: 6px 0; font-size: 13px; font-family: 'Cascadia Code', monospace;
  }
  .method {
    padding: 2px 8px; border-radius: 4px; font-size: 11px; font-weight: 700;
  }
  .method-post { background: rgba(59,130,246,0.15); color: #60a5fa; }
  .method-get { background: rgba(74,222,128,0.15); color: #4ade80; }
  .ecosystem { margin-top: 24px; text-align: left; }
  .ecosystem h3 { font-size: 12px; text-transform: uppercase; letter-spacing: 2px; color: #666; margin-bottom: 12px; }
  .eco-links { display: flex; flex-wrap: wrap; gap: 8px; }
  .eco-link {
    padding: 4px 12px; border-radius: 6px; font-size: 12px;
    background: rgba(255,255,255,0.04); border: 1px solid rgba(255,255,255,0.08);
    color: #aaa; text-decoration: none;
    transition: all 0.2s;
  }
  .eco-link:hover { background: rgba(255,100,0,0.1); border-color: rgba(255,100,0,0.3); color: #ff6a00; }
  .footer { margin-top: 28px; font-size: 11px; color: #444; }
</style>
</head>
<body>
<div class="bg-glow"></div>
<div class="card">
  <div class="torch-icon">&#128293;</div>
  <div class="logo">GOTorch</div>
  <div class="status"><div class="pulse"></div> Running</div>
  <div class="info">
    <div class="info-row"><span class="label">Uptime</span><span class="value">%s</span></div>
    <div class="info-row"><span class="label">Port</span><span class="value">%d</span></div>
  </div>
  <div class="endpoints">
    <h3>Endpoints</h3>
    <div class="ep"><span class="method method-post">POST</span> /v1/chat/completions</div>
    <div class="ep"><span class="method method-get">GET</span> /v1/models</div>
    <div class="ep"><span class="method method-get">GET</span> /health</div>
  </div>
  <div class="ecosystem">
    <h3>Ecosystem</h3>
    <div class="eco-links">
      <a class="eco-link" href="https://github.com/David2024patton/GOAgent" target="_blank">GOAgent</a>
      <a class="eco-link" href="https://github.com/David2024patton/GOTorch" target="_blank">GOTorch</a>
      <a class="eco-link" href="https://github.com/David2024patton/GOBrowser" target="_blank">GOBrowser</a>
      <a class="eco-link" href="https://github.com/David2024patton/GODashboard" target="_blank">GODashboard</a>
      <a class="eco-link" href="https://github.com/David2024patton/GOMedia" target="_blank">GOMedia</a>
      <a class="eco-link" href="https://github.com/David2024patton/GOForge" target="_blank">GOForge</a>
      <a class="eco-link" href="https://github.com/David2024patton/GOGateway" target="_blank">GOGateway</a>
    </div>
  </div>
  <div class="footer">GOAgent Framework | Go-Native Inference Engine</div>
</div>
</body>
</html>`

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
