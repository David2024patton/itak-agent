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

// ProjectAPI manages project CRUD for organizing agency work.
type ProjectAPI struct {
	backend memory.GraphBackend
}

// Project represents a work project scoped to an agency/sub-account.
type Project struct {
	ID             uint64   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	AgencyID       uint64   `json:"agency_id,omitempty"`
	SubAccountID   uint64   `json:"subaccount_id,omitempty"`
	AgencyName     string   `json:"agency_name,omitempty"`
	SubAccountName string   `json:"subaccount_name,omitempty"`
	AssignedAgents []string `json:"assigned_agents,omitempty"`
	AssignedSkills []string `json:"assigned_skills,omitempty"`
	SessionID      int      `json:"session_id,omitempty"`  // chat session that spawned this project
	AutoMode       bool     `json:"auto_mode"`             // true = AI-driven, false = manual user board
	Status         string   `json:"status"`                // active, paused, archived
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
}

// RegisterProjectRoutes adds project management endpoints.
func RegisterProjectRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	if backend == nil {
		debug.Warn("api", "Graph backend is nil, project API disabled")
		return
	}
	p := &ProjectAPI{backend: backend}
	mux.HandleFunc("/v1/projects", p.handleProjects)
	mux.HandleFunc("/v1/projects/", p.handleProjectByID)
	debug.Info("api", "Project API registered (/v1/projects)")
}

// GET/POST /v1/projects
func (p *ProjectAPI) handleProjects(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.Method {
	case http.MethodGet:
		p.listProjects(w, r)
	case http.MethodPost:
		p.createProject(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// GET/PUT/DELETE /v1/projects/{id}
func (p *ProjectAPI) handleProjectByID(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/v1/projects/")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "project ID required"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		p.getProject(w, r, id)
	case http.MethodPut:
		p.updateProject(w, r, id)
	case http.MethodDelete:
		p.deleteProject(w, r, id)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (p *ProjectAPI) listProjects(w http.ResponseWriter, r *http.Request) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"projects": []Project{}, "count": 0})
		return
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Project")

	// Optional filter by agency_id or session_id query param.
	filterAgency := r.URL.Query().Get("agency_id")
	filterSession := r.URL.Query().Get("session_id")

	projects := make([]Project, 0, len(nodes))
	for _, n := range nodes {
		if filterAgency != "" && pStr(n.Properties, "agency_id") != filterAgency {
			continue
		}
		if filterSession != "" && pStr(n.Properties, "session_id") != filterSession {
			continue
		}

		proj := Project{
			ID:           n.ID,
			Name:         pStr(n.Properties, "name"),
			Description:  pStr(n.Properties, "description"),
			Status:       pStr(n.Properties, "status"),
			CreatedAt:    pStr(n.Properties, "created_at"),
			UpdatedAt:    pStr(n.Properties, "updated_at"),
			AgencyName:   pStr(n.Properties, "agency_name"),
			SubAccountName: pStr(n.Properties, "subaccount_name"),
			AutoMode:     pStr(n.Properties, "auto_mode") == "true",
		}

		// Parse agency/subaccount IDs from string props.
		if v := pStr(n.Properties, "agency_id"); v != "" {
			fmt.Sscanf(v, "%d", &proj.AgencyID)
		}
		if v := pStr(n.Properties, "subaccount_id"); v != "" {
			fmt.Sscanf(v, "%d", &proj.SubAccountID)
		}
		if v := pStr(n.Properties, "session_id"); v != "" {
			fmt.Sscanf(v, "%d", &proj.SessionID)
		}

		// Parse assigned agents/skills from comma-separated strings.
		if v := pStr(n.Properties, "assigned_agents"); v != "" {
			proj.AssignedAgents = strings.Split(v, ",")
		}
		if v := pStr(n.Properties, "assigned_skills"); v != "" {
			proj.AssignedSkills = strings.Split(v, ",")
		}

		projects = append(projects, proj)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"projects": projects, "count": len(projects)})
}

func (p *ProjectAPI) createProject(w http.ResponseWriter, r *http.Request) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "requires iTaK Database"})
		return
	}

	var proj Project
	if err := json.NewDecoder(r.Body).Decode(&proj); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if proj.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "name required"})
		return
	}
	if proj.Status == "" {
		proj.Status = "active"
	}
	proj.CreatedAt = time.Now().Format(time.RFC3339)

	props := map[string]interface{}{
		"name":             proj.Name,
		"description":      proj.Description,
		"agency_id":        fmt.Sprintf("%d", proj.AgencyID),
		"subaccount_id":    fmt.Sprintf("%d", proj.SubAccountID),
		"agency_name":      proj.AgencyName,
		"subaccount_name":  proj.SubAccountName,
		"assigned_agents":  strings.Join(proj.AssignedAgents, ","),
		"assigned_skills":  strings.Join(proj.AssignedSkills, ","),
		"status":           proj.Status,
		"created_at":       proj.CreatedAt,
	}

	id, err := itakBackend.DB().CreateNode([]string{"Project"}, props, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	debug.Info("project", "Created project %q (id=%d)", proj.Name, id)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "created", "id": id, "project": proj})
}

func (p *ProjectAPI) getProject(w http.ResponseWriter, _ *http.Request, id string) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Project")
	for _, n := range nodes {
		if fmt.Sprintf("%d", n.ID) == id {
			proj := Project{
				ID: n.ID, Name: pStr(n.Properties, "name"),
				Description: pStr(n.Properties, "description"), Status: pStr(n.Properties, "status"),
				AgencyName: pStr(n.Properties, "agency_name"), SubAccountName: pStr(n.Properties, "subaccount_name"),
				CreatedAt: pStr(n.Properties, "created_at"), UpdatedAt: pStr(n.Properties, "updated_at"),
			}
			json.NewEncoder(w).Encode(proj)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "project not found"})
}

func (p *ProjectAPI) updateProject(w http.ResponseWriter, r *http.Request, id string) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Project")
	for _, n := range nodes {
		if fmt.Sprintf("%d", n.ID) == id {
			for k, v := range updates {
				n.Properties[k] = v
			}
			n.Properties["updated_at"] = time.Now().Format(time.RFC3339)
			debug.Info("project", "Updated project %s", id)
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "updated", "id": n.ID})
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "project not found"})
}

func (p *ProjectAPI) deleteProject(w http.ResponseWriter, _ *http.Request, id string) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Project")
	for _, n := range nodes {
		if fmt.Sprintf("%d", n.ID) == id {
			db.Graph.DeleteNode(n.ID)
			debug.Info("project", "Deleted project %s", id)
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted", "id": id})
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "project not found"})
}
