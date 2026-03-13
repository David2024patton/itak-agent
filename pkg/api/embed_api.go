package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/David2024patton/iTaKAgent/pkg/embed"
)

// RegisterEmbedRoutes mounts embedding model management endpoints.
//
// Endpoints:
//   GET  /v1/embed/status     - Current embedder status (provider, model, dimensions)
//   GET  /v1/embed/models     - List available + installed embedding models
//   POST /v1/embed/models/pull - Download an embedding model from Ollama registry
//   POST /v1/embed/config     - Update embedding config (provider, model, API key)
//   POST /v1/embed/test       - Test embedding with sample text
func RegisterEmbedRoutes(mux *http.ServeMux, mgr *embed.ModelManager) {
	mux.HandleFunc("/v1/embed/status", handleEmbedStatus)
	mux.HandleFunc("/v1/embed/models", handleEmbedModels(mgr))
	mux.HandleFunc("/v1/embed/models/pull", handleEmbedPull(mgr))
	mux.HandleFunc("/v1/embed/config", handleEmbedConfig)
	mux.HandleFunc("/v1/embed/test", handleEmbedTest)
}

// handleEmbedStatus returns the current embedding provider status.
func handleEmbedStatus(w http.ResponseWriter, r *http.Request) {
	e := embed.Get()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"provider":   e.Name(),
		"dimensions": e.Dimensions(),
		"active":     e.Name() != "noop",
	})
}

// handleEmbedModels returns the model catalog with install status.
func handleEmbedModels(mgr *embed.ModelManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "GET required"})
			return
		}
		models := mgr.ListModels()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"models": models,
			"count":  len(models),
		})
	}
}

// handleEmbedPull downloads an embedding model from the Ollama registry.
// Streams progress as SSE events.
func handleEmbedPull(mgr *embed.ModelManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Model == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model name required"})
			return
		}

		// Check if already installed.
		if path := mgr.GetModelPath(req.Model); path != "" {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":  "already_installed",
				"model":   req.Model,
				"path":    path,
			})
			return
		}

		// Start download and stream progress via SSE.
		progressCh, err := mgr.PullModel(req.Model)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			// Non-streaming fallback: just wait for completion.
			var lastProgress embed.PullProgress
			for p := range progressCh {
				lastProgress = p
			}
			if lastProgress.Error != "" {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": lastProgress.Error})
			} else {
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"status": lastProgress.Status,
					"model":  req.Model,
				})
			}
			return
		}

		// SSE streaming.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		for progress := range progressCh {
			data, _ := json.Marshal(progress)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// handleEmbedConfig updates the embedding configuration.
func handleEmbedConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Return current config (without exposing full API key).
		e := embed.Get()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"provider":   e.Name(),
			"dimensions": e.Dimensions(),
		})
		return
	}

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "GET or POST required"})
		return
	}

	var cfg embed.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	if err := embed.Init(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reinit failed: " + err.Error()})
		return
	}

	e := embed.Get()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "updated",
		"provider":   e.Name(),
		"dimensions": e.Dimensions(),
	})
}

// handleEmbedTest runs a test embedding on sample text.
func handleEmbedTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}

	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Text == "" {
		req.Text = "The quick brown fox jumps over the lazy dog."
	}

	e := embed.Get()
	vec, err := e.Embed(r.Context(), req.Text)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "embedding failed: " + err.Error(),
		})
		return
	}

	// Show first 10 dimensions as preview.
	preview := vec
	if len(preview) > 10 {
		preview = preview[:10]
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"provider":   e.Name(),
		"dimensions": len(vec),
		"preview":    preview,
		"text":       truncateText(req.Text, 100),
	})
}

func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
