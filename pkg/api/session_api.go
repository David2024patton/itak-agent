package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/David2024patton/iTaKAgent/pkg/agent"
	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// SessionAPI provides REST endpoints for chat session management.
//
// What: CRUD for chat sessions - list, resume, compact, archive.
// Why:  Users need to see their chat history as cards, click to resume,
//       and manage context window growth via compaction.
// How:  Wraps the orchestrator's Memory.Archive (JSONArchive) with HTTP handlers.
//       Sessions are tagged with channel (web, discord, whatsapp, telegram, api).
type SessionAPI struct {
	orch *agent.Orchestrator
}

// RegisterSessionRoutes adds session management endpoints.
func RegisterSessionRoutes(mux *http.ServeMux, orch *agent.Orchestrator) {
	if orch == nil || orch.Memory == nil {
		debug.Warn("api", "Orchestrator or memory is nil, sessions API disabled")
		return
	}
	s := &SessionAPI{orch: orch}
	mux.HandleFunc("/v1/sessions", s.handleSessions)
	mux.HandleFunc("/v1/sessions/", s.handleSessionByID)
	debug.Info("api", "Sessions API registered (/v1/sessions)")
}

// GET/POST /v1/sessions
func (s *SessionAPI) handleSessions(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.listSessions(w, r)
	case http.MethodPost:
		s.createSession(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// /v1/sessions/{id}, /v1/sessions/{id}/messages, /v1/sessions/{id}/compact
func (s *SessionAPI) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(parts[0])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid session ID"})
		return
	}

	// Sub-routes.
	if len(parts) == 2 {
		switch parts[1] {
		case "messages":
			s.getSessionMessages(w, r, id)
		case "compact":
			s.compactSession(w, r, id)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getSession(w, r, id)
	case http.MethodDelete:
		s.deleteSession(w, r, id)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *SessionAPI) listSessions(w http.ResponseWriter, r *http.Request) {
	archive := s.orch.Memory.Archive
	all := archive.ListAll()

	// Reverse for most recent first.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}

	// Filter by channel if requested.
	channel := r.URL.Query().Get("channel")
	if channel != "" {
		var filtered []interface{}
		for _, c := range all {
			if c.Channel == channel {
				filtered = append(filtered, c)
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"sessions": filtered,
			"count":    len(filtered),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"sessions": all,
		"count":    len(all),
	})
}

func (s *SessionAPI) createSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Title   string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if req.Channel == "" {
		req.Channel = "web"
	}
	if req.Title == "" {
		req.Title = "New Session"
	}

	id := s.orch.Memory.Archive.StartSessionWithMeta(req.Channel, req.Title)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"session_id": id,
		"channel":    req.Channel,
		"title":      req.Title,
	})
}

func (s *SessionAPI) getSession(w http.ResponseWriter, _ *http.Request, id int) {
	meta := s.orch.Memory.Archive.GetSessionMeta(id)
	if meta == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "session not found"})
		return
	}
	json.NewEncoder(w).Encode(meta)
}

func (s *SessionAPI) getSessionMessages(w http.ResponseWriter, _ *http.Request, id int) {
	msgs, err := s.orch.Memory.Archive.LoadConversation(id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"session_id": id,
		"messages":   msgs,
		"count":      len(msgs),
	})
}

func (s *SessionAPI) compactSession(w http.ResponseWriter, r *http.Request, id int) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Keep last 10 messages by default.
	keepRecent := 10
	if k := r.URL.Query().Get("keep"); k != "" {
		if val, err := strconv.Atoi(k); err == nil && val > 0 {
			keepRecent = val
		}
	}

	if err := s.orch.Memory.Archive.CompactSession(id, keepRecent); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "compacted",
		"session_id":  id,
		"keep_recent": keepRecent,
	})
}

func (s *SessionAPI) deleteSession(w http.ResponseWriter, _ *http.Request, id int) {
	// Mark as archived instead of actually deleting.
	s.orch.Memory.Archive.UpdateSessionMeta(id, "", "Deleted by user", 0)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "archived",
		"id":     id,
	})
}
