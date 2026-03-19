package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
)

// KnowledgeAPI manages agency-scoped knowledge entries.
// Knowledge entries are graph nodes that store scraped/learned information
// tagged with an agency_id so agents can filter context by business.
type KnowledgeAPI struct {
	backend memory.GraphBackend
}

// KnowledgeEntry represents a single knowledge item scoped to an agency.
type KnowledgeEntry struct {
	ID        uint64 `json:"id"`
	AgencyID  string `json:"agency_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Source    string `json:"source,omitempty"`   // url, file, scrape, manual
	Type     string `json:"type,omitempty"`     // fact, document, faq, note
	Tags     string `json:"tags,omitempty"`
	CreatedAt string `json:"created_at"`
}

// RegisterKnowledgeRoutes adds knowledge management endpoints.
func RegisterKnowledgeRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	if backend == nil {
		return
	}
	k := &KnowledgeAPI{backend: backend}
	mux.HandleFunc("/v1/knowledge", k.handleKnowledge)
	mux.HandleFunc("/v1/knowledge/", k.handleKnowledgeByID)
	debug.Info("api", "Knowledge API registered (/v1/knowledge)")
}

func (k *KnowledgeAPI) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	switch r.Method {
	case http.MethodGet:
		k.listKnowledge(w, r)
	case http.MethodPost:
		k.createKnowledge(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (k *KnowledgeAPI) handleKnowledgeByID(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/knowledge/")
	switch r.Method {
	case http.MethodDelete:
		k.deleteKnowledge(w, id)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (k *KnowledgeAPI) listKnowledge(w http.ResponseWriter, r *http.Request) {
	itakBackend, ok := k.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"entries": []KnowledgeEntry{}, "count": 0})
		return
	}
	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("AgencyKnowledge")

	agencyFilter := r.URL.Query().Get("agency_id")
	entries := make([]KnowledgeEntry, 0)
	for _, n := range nodes {
		if agencyFilter != "" && pStr(n.Properties, "agency_id") != agencyFilter {
			continue
		}
		entries = append(entries, KnowledgeEntry{
			ID: n.ID, AgencyID: pStr(n.Properties, "agency_id"),
			ProjectID: pStr(n.Properties, "project_id"),
			Title: pStr(n.Properties, "title"), Content: pStr(n.Properties, "content"),
			Source: pStr(n.Properties, "source"), Type: pStr(n.Properties, "type"),
			Tags: pStr(n.Properties, "tags"), CreatedAt: pStr(n.Properties, "created_at"),
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"entries": entries, "count": len(entries)})
}

func (k *KnowledgeAPI) createKnowledge(w http.ResponseWriter, r *http.Request) {
	itakBackend, ok := k.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var entry KnowledgeEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if entry.Title == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "title required"})
		return
	}
	if entry.Type == "" {
		entry.Type = "note"
	}
	entry.CreatedAt = time.Now().Format(time.RFC3339)
	props := map[string]interface{}{
		"agency_id": entry.AgencyID, "project_id": entry.ProjectID,
		"title": entry.Title, "content": entry.Content,
		"source": entry.Source, "type": entry.Type,
		"tags": entry.Tags, "created_at": entry.CreatedAt,
	}
	id, err := itakBackend.DB().CreateNode([]string{"AgencyKnowledge"}, props, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "created", "id": id})
}

func (k *KnowledgeAPI) deleteKnowledge(w http.ResponseWriter, id string) {
	itakBackend, ok := k.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("AgencyKnowledge")
	for _, n := range nodes {
		if fmt.Sprintf("%d", n.ID) == id {
			db.Graph.DeleteNode(n.ID)
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted"})
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}
