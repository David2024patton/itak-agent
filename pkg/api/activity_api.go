package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
)

// ActivityAPI provides endpoints for recording and querying agent activity.
//
// What: REST endpoints for AgentActivity graph nodes.
// Why:  Every agent action should be persisted for audit, replay, and knowledge.
// How:  Creates nodes with label "AgentActivity" linked to agents and knowledge.
type ActivityAPI struct {
	backend memory.GraphBackend
}

// RegisterActivityRoutes adds agent activity endpoints.
func RegisterActivityRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	if backend == nil {
		debug.Warn("api", "Graph backend is nil, activity API disabled")
		return
	}
	a := &ActivityAPI{backend: backend}
	mux.HandleFunc("/v1/activity", a.handleActivity)
	debug.Info("api", "Activity API registered (/v1/activity)")
}

// ActivityRecord represents a single agent activity event.
type ActivityRecord struct {
	Agent     string `json:"agent"`
	Action    string `json:"action"`    // research, code, delegate, embed, fix, chat
	Data      string `json:"data"`      // result/context summary
	TaskID    string `json:"task_id,omitempty"`
	Timestamp string `json:"timestamp"`
	Status    string `json:"status,omitempty"` // success, error, partial
}

// GET/POST /v1/activity
func (a *ActivityAPI) handleActivity(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.Method {
	case http.MethodGet:
		a.listActivity(w, r)
	case http.MethodPost:
		a.recordActivity(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "GET or POST only"})
	}
}

// listActivity returns recent agent activity from the graph.
func (a *ActivityAPI) listActivity(w http.ResponseWriter, _ *http.Request) {
	itakBackend, ok := a.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"activity": []ActivityRecord{}, "count": 0})
		return
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("AgentActivity")

	records := make([]ActivityRecord, 0, len(nodes))
	for _, n := range nodes {
		records = append(records, ActivityRecord{
			Agent:     pStr(n.Properties, "agent"),
			Action:    pStr(n.Properties, "action"),
			Data:      pStr(n.Properties, "data"),
			TaskID:    pStr(n.Properties, "task_id"),
			Timestamp: pStr(n.Properties, "timestamp"),
			Status:    pStr(n.Properties, "status"),
		})
	}

	// Sort by timestamp descending (most recent first) -- simple bubble for small sets.
	for i := 0; i < len(records); i++ {
		for j := i + 1; j < len(records); j++ {
			if records[j].Timestamp > records[i].Timestamp {
				records[i], records[j] = records[j], records[i]
			}
		}
	}

	// Cap at 100 most recent.
	if len(records) > 100 {
		records = records[:100]
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"activity": records,
		"count":    len(records),
	})
}

// recordActivity persists an agent activity event to the graph.
func (a *ActivityAPI) recordActivity(w http.ResponseWriter, r *http.Request) {
	itakBackend, ok := a.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "requires iTaK Database"})
		return
	}

	var rec ActivityRecord
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if rec.Agent == "" || rec.Action == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "agent and action required"})
		return
	}

	if rec.Timestamp == "" {
		rec.Timestamp = time.Now().Format(time.RFC3339)
	}
	if rec.Status == "" {
		rec.Status = "success"
	}

	db := itakBackend.DB()
	props := map[string]interface{}{
		"agent":     rec.Agent,
		"action":    rec.Action,
		"data":      rec.Data,
		"task_id":   rec.TaskID,
		"timestamp": rec.Timestamp,
		"status":    rec.Status,
	}

	id, err := db.CreateNode([]string{"AgentActivity"}, props, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	debug.Info("activity", "Recorded: agent=%s action=%s (id=%d)", rec.Agent, rec.Action, id)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "recorded", "id": id})
}
