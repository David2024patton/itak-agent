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

// ReportAPI manages report templates and generated reports.
type ReportAPI struct {
	backend memory.GraphBackend
}

// ReportTemplate represents a reusable report format.
type ReportTemplate struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`               // social_media, email_activity, engagement, custom
	AgencyID    string `json:"agency_id,omitempty"`
	ProjectID   string `json:"project_id,omitempty"`
	Agent       string `json:"agent,omitempty"`     // which agent generates this
	Prompt      string `json:"prompt,omitempty"`    // generation prompt
	Schedule    string `json:"schedule,omitempty"`  // cron expression or empty for manual
	LastRun     string `json:"last_run,omitempty"`
	RunCount    int    `json:"run_count"`
	CreatedAt   string `json:"created_at"`
}

// GeneratedReport represents a single generated report instance.
type GeneratedReport struct {
	ID         uint64 `json:"id"`
	TemplateID string `json:"template_id"`
	Name       string `json:"name"`
	Content    string `json:"content"`
	AgencyID   string `json:"agency_id,omitempty"`
	GeneratedAt string `json:"generated_at"`
}

// RegisterReportRoutes adds report management endpoints.
func RegisterReportRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	if backend == nil {
		return
	}
	r := &ReportAPI{backend: backend}
	mux.HandleFunc("/v1/reports/templates", r.handleTemplates)
	mux.HandleFunc("/v1/reports/templates/", r.handleTemplateByID)
	mux.HandleFunc("/v1/reports/generated", r.handleGenerated)
	debug.Info("api", "Report API registered (/v1/reports)")
}

func (r *ReportAPI) handleTemplates(w http.ResponseWriter, req *http.Request) {
	corsHeaders(w)
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	switch req.Method {
	case http.MethodGet:
		r.listTemplates(w)
	case http.MethodPost:
		r.createTemplate(w, req)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *ReportAPI) handleTemplateByID(w http.ResponseWriter, req *http.Request) {
	corsHeaders(w)
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	id := strings.TrimPrefix(req.URL.Path, "/v1/reports/templates/")
	switch req.Method {
	case http.MethodPut:
		r.updateTemplate(w, req, id)
	case http.MethodDelete:
		r.deleteTemplate(w, id)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *ReportAPI) handleGenerated(w http.ResponseWriter, req *http.Request) {
	corsHeaders(w)
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	switch req.Method {
	case http.MethodGet:
		r.listGenerated(w)
	case http.MethodPost:
		r.createGenerated(w, req)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *ReportAPI) listTemplates(w http.ResponseWriter) {
	itakBackend, ok := r.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"templates": []ReportTemplate{}, "count": 0})
		return
	}
	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("ReportTemplate")
	templates := make([]ReportTemplate, 0)
	for _, n := range nodes {
		t := ReportTemplate{
			ID: n.ID, Name: pStr(n.Properties, "name"),
			Description: pStr(n.Properties, "description"), Type: pStr(n.Properties, "type"),
			AgencyID: pStr(n.Properties, "agency_id"), ProjectID: pStr(n.Properties, "project_id"),
			Agent: pStr(n.Properties, "agent"), Prompt: pStr(n.Properties, "prompt"),
			Schedule: pStr(n.Properties, "schedule"), LastRun: pStr(n.Properties, "last_run"),
			CreatedAt: pStr(n.Properties, "created_at"),
		}
		if v := pStr(n.Properties, "run_count"); v != "" {
			fmt.Sscanf(v, "%d", &t.RunCount)
		}
		templates = append(templates, t)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"templates": templates, "count": len(templates)})
}

func (r *ReportAPI) createTemplate(w http.ResponseWriter, req *http.Request) {
	itakBackend, ok := r.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var t ReportTemplate
	if err := json.NewDecoder(req.Body).Decode(&t); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if t.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "name required"})
		return
	}
	if t.Type == "" {
		t.Type = "custom"
	}
	t.CreatedAt = time.Now().Format(time.RFC3339)
	props := map[string]interface{}{
		"name": t.Name, "description": t.Description, "type": t.Type,
		"agency_id": t.AgencyID, "project_id": t.ProjectID,
		"agent": t.Agent, "prompt": t.Prompt, "schedule": t.Schedule,
		"run_count": "0", "created_at": t.CreatedAt,
	}
	id, err := itakBackend.DB().CreateNode([]string{"ReportTemplate"}, props, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "created", "id": id})
}

func (r *ReportAPI) updateTemplate(w http.ResponseWriter, req *http.Request, id string) {
	itakBackend, ok := r.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var updates map[string]interface{}
	json.NewDecoder(req.Body).Decode(&updates)
	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("ReportTemplate")
	for _, n := range nodes {
		if fmt.Sprintf("%d", n.ID) == id {
			for k, v := range updates {
				n.Properties[k] = v
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "updated"})
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

func (r *ReportAPI) deleteTemplate(w http.ResponseWriter, id string) {
	itakBackend, ok := r.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("ReportTemplate")
	for _, n := range nodes {
		if fmt.Sprintf("%d", n.ID) == id {
			db.Graph.DeleteNode(n.ID)
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted"})
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

func (r *ReportAPI) listGenerated(w http.ResponseWriter) {
	itakBackend, ok := r.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"reports": []GeneratedReport{}, "count": 0})
		return
	}
	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("GeneratedReport")
	reports := make([]GeneratedReport, 0)
	for _, n := range nodes {
		reports = append(reports, GeneratedReport{
			ID: n.ID, TemplateID: pStr(n.Properties, "template_id"),
			Name: pStr(n.Properties, "name"), Content: pStr(n.Properties, "content"),
			AgencyID: pStr(n.Properties, "agency_id"),
			GeneratedAt: pStr(n.Properties, "generated_at"),
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"reports": reports, "count": len(reports)})
}

func (r *ReportAPI) createGenerated(w http.ResponseWriter, req *http.Request) {
	itakBackend, ok := r.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var rpt GeneratedReport
	if err := json.NewDecoder(req.Body).Decode(&rpt); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	rpt.GeneratedAt = time.Now().Format(time.RFC3339)
	props := map[string]interface{}{
		"template_id": rpt.TemplateID, "name": rpt.Name, "content": rpt.Content,
		"agency_id": rpt.AgencyID, "generated_at": rpt.GeneratedAt,
	}
	id, _ := itakBackend.DB().CreateNode([]string{"GeneratedReport"}, props, nil)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "created", "id": id})
}
