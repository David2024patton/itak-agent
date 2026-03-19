// Package visionclaw provides a channel plugin for Meta Ray-Ban smart glasses.
//
// What: HTTP gateway compatible with VisionClaw's ToolCallRouter that accepts
//       voice-transcribed tasks and optional camera frames from Ray-Ban glasses.
// Why:  Enables hands-free agent interaction through smart glasses. The glasses
//       camera provides visual context while voice provides commands.
// How:  Serves an OpenClaw-compatible /v1/openclaw/execute endpoint that the
//       VisionClaw iOS/Android app can POST to. Routes tasks through the
//       shared MessageHandler to orch.Run().
//
// Source: https://github.com/Intent-Lab/VisionClaw
package visionclaw

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/plugin"
)

// Config holds settings for the VisionClaw glasses plugin.
type Config struct {
	Port           int      `yaml:"port"`
	AllowedOrigins []string `yaml:"allowed_origins"`
}

// Plugin serves an OpenClaw-compatible gateway for VisionClaw glasses.
type Plugin struct {
	port    int
	origins []string
	handler plugin.MessageHandler
	server  *http.Server
}

// New creates a VisionClaw plugin.
func New(cfg Config, handler plugin.MessageHandler) *Plugin {
	if cfg.Port == 0 {
		cfg.Port = 33321 // OpenClaw default port
	}
	if len(cfg.AllowedOrigins) == 0 {
		cfg.AllowedOrigins = []string{"*"}
	}
	return &Plugin{
		port:    cfg.Port,
		origins: cfg.AllowedOrigins,
		handler: handler,
	}
}

func (p *Plugin) Name() string { return "visionclaw" }

func (p *Plugin) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// OpenClaw-compatible endpoints (VisionClaw's ToolCallRouter targets these).
	mux.HandleFunc("/v1/openclaw/execute", p.handleExecute)
	mux.HandleFunc("/v1/openclaw/chat", p.handleChat)

	// Simple health check.
	mux.HandleFunc("/health", p.handleHealth)

	p.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", p.port),
		Handler: p.corsMiddleware(mux),
	}

	go func() {
		debug.Info("plugin:visionclaw", "VisionClaw gateway listening on http://localhost:%d", p.port)
		debug.Info("plugin:visionclaw", "Point VisionClaw app gateway URL to this address")
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			debug.Error("plugin:visionclaw", "Server error: %v", err)
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

// executeRequest matches VisionClaw's ToolCallRouter format:
//
//	{ "task": "describe what I see" }
//
// Optionally includes a base64-encoded image frame from the glasses camera.
type executeRequest struct {
	Task  string `json:"task"`
	Image string `json:"image,omitempty"` // base64-encoded JPEG from glasses camera
}

type executeResponse struct {
	Result string `json:"result"`
	Error  string `json:"error,omitempty"`
}

// handleExecute processes OpenClaw-style execute requests from VisionClaw.
// This is the primary endpoint the glasses app's ToolCallRouter POSTs to.
func (p *Plugin) handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, executeResponse{Error: "POST required"})
		return
	}

	var req executeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, executeResponse{Error: "invalid JSON: " + err.Error()})
		return
	}
	if req.Task == "" {
		writeJSON(w, http.StatusBadRequest, executeResponse{Error: "'task' is required"})
		return
	}

	debug.Info("plugin:visionclaw", "Glasses execute: %s", truncate(req.Task, 80))

	// Build inbound message with optional vision attachment.
	msg := plugin.InboundMessage{
		Text:    req.Task,
		Channel: "glasses",
		Metadata: map[string]string{
			"source": "visionclaw",
			"device": "ray-ban-meta",
		},
	}

	// Attach camera frame if provided.
	if req.Image != "" {
		imageData, err := base64.StdEncoding.DecodeString(req.Image)
		if err == nil {
			msg.Media = append(msg.Media, plugin.MediaAttachment{
				Type:     "image/jpeg",
				Data:     imageData,
				Filename: "glasses_frame.jpg",
			})
			debug.Debug("plugin:visionclaw", "Attached camera frame (%d bytes)", len(imageData))
		}
	}

	response, err := p.handler(r.Context(), msg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, executeResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, executeResponse{Result: response})
}

// chatRequest is an alternative format matching OpenClaw's chat endpoint.
type chatRequest struct {
	Message string `json:"message"`
	Channel string `json:"channel,omitempty"`
}

// handleChat processes standard chat messages (alternative to execute).
func (p *Plugin) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, executeResponse{Error: "POST required"})
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, executeResponse{Error: "invalid JSON"})
		return
	}

	channel := req.Channel
	if channel == "" {
		channel = "glasses"
	}

	msg := plugin.InboundMessage{
		Text:    req.Message,
		Channel: channel,
	}

	response, err := p.handler(r.Context(), msg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, executeResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, executeResponse{Result: response})
}

func (p *Plugin) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"plugin": "visionclaw",
		"device": "ray-ban-meta",
	})
}

// ── Middleware ──────────────────────────────────────────────────────

func (p *Plugin) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := "*"
		if len(p.origins) > 0 && p.origins[0] != "*" {
			origin = p.origins[0]
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ── Helpers ────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
