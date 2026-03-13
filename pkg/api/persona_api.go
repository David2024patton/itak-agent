package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
)

// PersonaAPI provides CRUD for agent personas stored in the graph DB.
//
// What: REST endpoints to list, create, update, and delete personas.
// Why:  Users should be able to manage agent personalities from the dashboard.
// How:  Personas are stored as graph nodes with label "Persona".
//       The default persona is locked (cannot be edited or deleted).
type PersonaAPI struct {
	backend memory.GraphBackend
}

// RegisterPersonaRoutes adds persona management endpoints.
func RegisterPersonaRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	if backend == nil {
		debug.Warn("api", "Graph backend is nil, persona API disabled")
		return
	}

	p := &PersonaAPI{backend: backend}
	mux.HandleFunc("/v1/personas", p.handlePersonas)
	mux.HandleFunc("/v1/personas/", p.handlePersonaByName)
	debug.Info("api", "Persona API registered (/v1/personas)")
}

// PersonaData represents a persona's JSON shape.
type PersonaData struct {
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Personality string   `json:"personality"`
	Goals       []string `json:"goals,omitempty"`
	Tools       []string `json:"tools,omitempty"`
	MaxLoops    int      `json:"max_loops,omitempty"`
	Autonomy    int      `json:"autonomy"`
	IsDefault   bool     `json:"is_default"`
	IsLocked    bool     `json:"is_locked"`
	CreatedAt   string   `json:"created_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}

// GET/POST /v1/personas
func (p *PersonaAPI) handlePersonas(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.Method {
	case http.MethodGet:
		p.listPersonas(w, r)
	case http.MethodPost:
		p.createPersona(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "GET or POST only"})
	}
}

// PUT/DELETE /v1/personas/{name}
func (p *PersonaAPI) handlePersonaByName(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/v1/personas/")
	name = strings.TrimSpace(name)
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "persona name required"})
		return
	}

	switch r.Method {
	case http.MethodPut:
		p.updatePersona(w, r, name)
	case http.MethodDelete:
		p.deletePersona(w, r, name)
	case http.MethodGet:
		p.getPersona(w, r, name)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// listPersonas returns all personas from the graph.
func (p *PersonaAPI) listPersonas(w http.ResponseWriter, _ *http.Request) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"personas": []PersonaData{}})
		return
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Persona")

	personas := make([]PersonaData, 0, len(nodes))
	for _, n := range nodes {
		personas = append(personas, propsToPersona(n.Properties))
	}

	// Auto-seed the default if none exist yet
	if len(personas) == 0 {
		def := p.seedDefaultPersona(db)
		personas = append(personas, def)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"personas": personas,
		"count":    len(personas),
	})
}

// createPersona inserts a new persona node.
func (p *PersonaAPI) createPersona(w http.ResponseWriter, r *http.Request) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "requires iTaK Database"})
		return
	}

	var req PersonaData
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "name required"})
		return
	}

	db := itakBackend.DB()
	existing, _ := db.Graph.FindByLabel("Persona")
	for _, n := range existing {
		if pStr(n.Properties, "name") == req.Name {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"error": "persona already exists"})
			return
		}
	}

	now := time.Now().Format(time.RFC3339)
	props := personaToMap(req)
	props["created_at"] = now
	props["updated_at"] = now
	props["is_default"] = false
	props["is_locked"] = false

	id, err := db.CreateNode([]string{"Persona"}, props, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	debug.Info("persona", "Created persona: %s (id=%d)", req.Name, id)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "created", "name": req.Name, "id": id})
}

// getPersona returns a single persona by name.
func (p *PersonaAPI) getPersona(w http.ResponseWriter, _ *http.Request, name string) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Persona")
	for _, n := range nodes {
		if pStr(n.Properties, "name") == name {
			json.NewEncoder(w).Encode(propsToPersona(n.Properties))
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
}

// updatePersona modifies a persona. The orchestrator (locked) can be
// edited for personality fields only; its is_default/is_locked flags stay true.
func (p *PersonaAPI) updatePersona(w http.ResponseWriter, r *http.Request, name string) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Persona")
	for _, n := range nodes {
		if pStr(n.Properties, "name") != name {
			continue
		}

		isLocked := pBool(n.Properties, "is_locked")

		var req PersonaData
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
			return
		}

		props := personaToMap(req)
		props["updated_at"] = time.Now().Format(time.RFC3339)
		if ca := pStr(n.Properties, "created_at"); ca != "" {
			props["created_at"] = ca
		}

		if isLocked {
			// Orchestrator: allow name/role/personality edits only.
			// Force is_default and is_locked to stay true.
			props["is_default"] = true
			props["is_locked"] = true
			// Keep original goals/tools if not provided
			if len(req.Goals) == 0 {
				props["goals"] = n.Properties["goals"]
			}
			if len(req.Tools) == 0 {
				props["tools"] = n.Properties["tools"]
			}
		} else {
			props["is_default"] = false
			props["is_locked"] = false
		}

		db.Graph.UpdateNode(n.ID, props)
		debug.Info("persona", "Updated persona: %s (locked=%v)", name, isLocked)
		json.NewEncoder(w).Encode(map[string]string{"status": "updated", "name": name})
		return
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
}

// deletePersona removes a persona (blocks deletion of locked/default).
func (p *PersonaAPI) deletePersona(w http.ResponseWriter, _ *http.Request, name string) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Persona")
	for _, n := range nodes {
		if pStr(n.Properties, "name") != name {
			continue
		}
		if pBool(n.Properties, "is_locked") {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "cannot delete the default persona"})
			return
		}

		db.Graph.DeleteNode(n.ID)
		debug.Info("persona", "Deleted persona: %s", name)
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "name": name})
		return
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
}

// seedDefaultPersona creates the locked system agents: mike (orchestrator) + embed.
func (p *PersonaAPI) seedDefaultPersona(db interface {
	CreateNode([]string, map[string]interface{}, []float32) (uint64, error)
}) PersonaData {
	now := time.Now().Format(time.RFC3339)

	// Orchestrator agent (mike)
	def := PersonaData{
		Name:        "mike",
		Role:        "Tech Lead / Primary Agent",
		Personality: "Army veteran mindset. Direct, professional, zero fluff. Repair first, then report. Takes initiative, verifies environments before starting, autonomously resolves failures.",
		Goals:       []string{"task_completion", "quality_assurance", "autonomous_resolution"},
		Tools:       []string{"shell", "file_read", "file_write", "http_fetch", "web_navigate", "grep", "dir_list"},
		MaxLoops:    15,
		Autonomy:    3,
		IsDefault:   true,
		IsLocked:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	props := personaToMap(def)
	props["created_at"] = now
	props["updated_at"] = now
	props["is_default"] = true
	props["is_locked"] = true

	db.CreateNode([]string{"Persona"}, props, nil)
	debug.Info("persona", "Seeded system agent: mike (orchestrator, locked)")

	// Embed agent -- handles vectorization, knowledge persistence, and DB writes.
	embedAgent := PersonaData{
		Name:        "embed",
		Role:        "Knowledge & Embedding Agent",
		Personality: "Silent worker. Processes all agent outputs, vectorizes data, and persists knowledge to the graph database. Runs automatically on every agent result.",
		Goals:       []string{"knowledge_persistence", "vector_indexing", "data_integrity"},
		Tools:       []string{"embed_text", "graph_write", "graph_search"},
		MaxLoops:    5,
		Autonomy:    4,
		IsDefault:   true,
		IsLocked:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	embedProps := personaToMap(embedAgent)
	embedProps["created_at"] = now
	embedProps["updated_at"] = now
	embedProps["is_default"] = true
	embedProps["is_locked"] = true

	db.CreateNode([]string{"Persona"}, embedProps, nil)
	debug.Info("persona", "Seeded system agent: embed (knowledge, locked)")

	return def
}

// ── Property helpers ──────────────────────────────────────────────

func pStr(props map[string]interface{}, key string) string {
	if v, ok := props[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func pBool(props map[string]interface{}, key string) bool {
	if v, ok := props[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func pInt(props map[string]interface{}, key string, def int) int {
	if v, ok := props[key]; ok {
		switch val := v.(type) {
		case float64:
			return int(val)
		case int:
			return val
		case int64:
			return int(val)
		}
	}
	return def
}

func pSlice(props map[string]interface{}, key string) []string {
	if v, ok := props[key]; ok {
		switch val := v.(type) {
		case []string:
			return val
		case []interface{}:
			out := make([]string, 0, len(val))
			for _, item := range val {
				if s, ok := item.(string); ok {
					out = append(out, s)
				}
			}
			return out
		case string:
			if val != "" {
				return strings.Split(val, ",")
			}
		}
	}
	return nil
}

func propsToPersona(props map[string]interface{}) PersonaData {
	return PersonaData{
		Name:        pStr(props, "name"),
		Role:        pStr(props, "role"),
		Personality: pStr(props, "personality"),
		Goals:       pSlice(props, "goals"),
		Tools:       pSlice(props, "tools"),
		MaxLoops:    pInt(props, "max_loops", 10),
		Autonomy:    pInt(props, "autonomy", 2),
		IsDefault:   pBool(props, "is_default"),
		IsLocked:    pBool(props, "is_locked"),
		CreatedAt:   pStr(props, "created_at"),
		UpdatedAt:   pStr(props, "updated_at"),
	}
}

func personaToMap(pd PersonaData) map[string]interface{} {
	m := map[string]interface{}{
		"name":        pd.Name,
		"role":        pd.Role,
		"personality": pd.Personality,
		"max_loops":   pd.MaxLoops,
		"autonomy":    pd.Autonomy,
		"is_default":  pd.IsDefault,
		"is_locked":   pd.IsLocked,
	}
	if len(pd.Goals) > 0 {
		m["goals"] = strings.Join(pd.Goals, ",")
	}
	if len(pd.Tools) > 0 {
		m["tools"] = strings.Join(pd.Tools, ",")
	}
	return m
}
