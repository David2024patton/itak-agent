package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/embed"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
)

// RegisterDebugResearchRoutes mounts DebugMemory and WebResearch API endpoints.
//
// DebugMemory endpoints:
//   POST /v1/debug/errors       - Store a new error
//   POST /v1/debug/fixes        - Store a fix for an error
//   POST /v1/debug/search       - Search for known errors/solutions
//
// WebResearch endpoints:
//   POST /v1/research/store     - Store website research
//   POST /v1/research/search    - Search past research
func RegisterDebugResearchRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	mux.HandleFunc("/v1/debug/errors", handleStoreError(backend))
	mux.HandleFunc("/v1/debug/fixes", handleStoreFix(backend))
	mux.HandleFunc("/v1/debug/search", handleSearchErrors(backend))

	mux.HandleFunc("/v1/research/store", handleStoreResearch(backend))
	mux.HandleFunc("/v1/research/search", handleSearchResearch(backend))
}

// ── DebugMemory Handlers ─────────────────────────────────────────

func handleStoreError(backend memory.GraphBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req struct {
			SessionID string `json:"session_id"`
			Message   string `json:"message"`
			ErrorType string `json:"error_type"` // "compile", "runtime", "network", "config", etc.
			Source    string `json:"source"`     // file or tool that produced the error
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Message == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message required"})
			return
		}
		if req.ErrorType == "" {
			req.ErrorType = "unknown"
		}

		// Generate embedding for semantic search later.
		var embedding []float32
		e := embed.Get()
		if e.Dimensions() > 0 {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			embedding, _ = e.Embed(ctx, req.Message)
		}

		nodeID := backend.StoreError(req.SessionID, req.Message, req.ErrorType, req.Source, embedding)

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"status":    "stored",
			"error_id":  nodeID,
			"type":      req.ErrorType,
			"source":    req.Source,
			"has_embed": len(embedding) > 0,
		})
	}
}

func handleStoreFix(backend memory.GraphBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req struct {
			ErrorID     uint64 `json:"error_id"`
			Description string `json:"description"`
			Code        string `json:"code"`   // the fix code/command
			Agent       string `json:"agent"`  // which agent found the fix
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.ErrorID == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "error_id required"})
			return
		}
		if req.Description == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "description required"})
			return
		}

		// Generate embedding for the fix description.
		var embedding []float32
		e := embed.Get()
		if e.Dimensions() > 0 {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			embedding, _ = e.Embed(ctx, req.Description)
		}

		backend.StoreFix(req.ErrorID, req.Description, req.Code, req.Agent, embedding)

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"status":    "stored",
			"error_id":  req.ErrorID,
			"agent":     req.Agent,
			"has_embed": len(embedding) > 0,
		})
	}
}

func handleSearchErrors(backend memory.GraphBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req struct {
			Query string `json:"query"` // the error message to search for
			Limit int    `json:"limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Query == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query required"})
			return
		}
		if req.Limit <= 0 || req.Limit > 20 {
			req.Limit = 5
		}

		// Generate query embedding.
		e := embed.Get()
		if e.Dimensions() == 0 {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"results": []interface{}{},
				"message": "no embedding provider configured, cannot search semantically",
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		queryEmbed, err := e.Embed(ctx, req.Query)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "embedding failed: " + err.Error()})
			return
		}

		results, err := backend.SearchErrors(queryEmbed, req.Limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "search failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"results": results,
			"count":   len(results),
			"query":   req.Query,
		})
	}
}

// ── WebResearch Handlers ─────────────────────────────────────────

func handleStoreResearch(backend memory.GraphBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req struct {
			SessionID string `json:"session_id"`
			URL       string `json:"url"`
			Domain    string `json:"domain"`
			Title     string `json:"title"`
			Content   string `json:"content"`   // extracted page content
			Findings  string `json:"findings"`  // summary of what was found
			Topic     string `json:"topic"`     // research topic/category
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url required"})
			return
		}

		// Generate embedding from findings + title for semantic search.
		var embedding []float32
		e := embed.Get()
		if e.Dimensions() > 0 {
			embedText := req.Title + " " + req.Findings
			if len(embedText) > 4000 {
				embedText = embedText[:4000]
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			embedding, _ = e.Embed(ctx, embedText)
		}

		nodeID := backend.StoreResearch(
			req.SessionID, req.URL, req.Domain,
			req.Title, req.Content, req.Findings, req.Topic,
			embedding,
		)

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"status":      "stored",
			"research_id": nodeID,
			"domain":      req.Domain,
			"has_embed":   len(embedding) > 0,
		})
	}
}

func handleSearchResearch(backend memory.GraphBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Query == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query required"})
			return
		}
		if req.Limit <= 0 || req.Limit > 20 {
			req.Limit = 5
		}

		e := embed.Get()
		if e.Dimensions() == 0 {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"results": []interface{}{},
				"message": "no embedding provider configured, cannot search semantically",
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		queryEmbed, err := e.Embed(ctx, req.Query)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "embedding failed: " + err.Error()})
			return
		}

		results, err := backend.SearchResearch(queryEmbed, req.Limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "search failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"results": results,
			"count":   len(results),
			"query":   req.Query,
		})
	}
}
