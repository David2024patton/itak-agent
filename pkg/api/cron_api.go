package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/David2024patton/iTaKAgent/pkg/cron"
	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// CronAPI provides REST endpoints for managing scheduled automations.
//
// What: CRUD for cron jobs, manual triggers, and run history.
// Why:  Users need to create, manage, and monitor automated agent tasks
//       through the dashboard without touching config files.
// How:  Wraps the cron.Scheduler with HTTP handlers.
type CronAPI struct {
	scheduler *cron.Scheduler
}

// RegisterCronRoutes adds automation scheduling endpoints.
func RegisterCronRoutes(mux *http.ServeMux, scheduler *cron.Scheduler) {
	if scheduler == nil {
		debug.Warn("api", "Scheduler is nil, cron API disabled")
		return
	}
	c := &CronAPI{scheduler: scheduler}
	mux.HandleFunc("/v1/automations", c.handleAutomations)
	mux.HandleFunc("/v1/automations/", c.handleAutomationByID)
	debug.Info("api", "Cron API registered (/v1/automations)")
}

// GET/POST /v1/automations
func (c *CronAPI) handleAutomations(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.Method {
	case http.MethodGet:
		c.listAutomations(w, r)
	case http.MethodPost:
		c.createAutomation(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "GET or POST only"})
	}
}

// GET/PUT/DELETE/POST /v1/automations/{id}
// GET /v1/automations/{id}/history
// POST /v1/automations/{id}/trigger
func (c *CronAPI) handleAutomationByID(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/automations/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	jobID := parts[0]

	// Sub-routes.
	if len(parts) == 2 {
		switch parts[1] {
		case "trigger":
			c.triggerAutomation(w, r, jobID)
		case "history":
			c.automationHistory(w, r, jobID)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		c.getAutomation(w, r, jobID)
	case http.MethodPut:
		c.updateAutomation(w, r, jobID)
	case http.MethodDelete:
		c.deleteAutomation(w, r, jobID)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (c *CronAPI) listAutomations(w http.ResponseWriter, _ *http.Request) {
	jobs := c.scheduler.ListJobs()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"automations": jobs,
		"count":       len(jobs),
	})
}

func (c *CronAPI) createAutomation(w http.ResponseWriter, r *http.Request) {
	var job cron.Job
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if job.Name == "" || job.Agent == "" || job.Prompt == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "name, agent, and prompt required"})
		return
	}
	if job.ScheduleType == "" {
		job.ScheduleType = "every"
	}
	if job.Type == "" {
		job.Type = "cron"
	}
	job.Enabled = true

	c.scheduler.AddJob(&job)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "created", "id": job.ID, "job": job})
}

func (c *CronAPI) getAutomation(w http.ResponseWriter, _ *http.Request, id string) {
	job := c.scheduler.GetJob(id)
	if job == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "automation not found"})
		return
	}
	json.NewEncoder(w).Encode(job)
}

func (c *CronAPI) updateAutomation(w http.ResponseWriter, r *http.Request, id string) {
	existing := c.scheduler.GetJob(id)
	if existing == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "automation not found"})
		return
	}

	var updates cron.Job
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Merge updates into existing job.
	if updates.Name != "" {
		existing.Name = updates.Name
	}
	if updates.Schedule != "" {
		existing.Schedule = updates.Schedule
	}
	if updates.ScheduleType != "" {
		existing.ScheduleType = updates.ScheduleType
	}
	if updates.Agent != "" {
		existing.Agent = updates.Agent
	}
	if updates.Prompt != "" {
		existing.Prompt = updates.Prompt
	}
	if updates.ExecutionMode != "" {
		existing.ExecutionMode = updates.ExecutionMode
	}

	c.scheduler.AddJob(existing) // Re-add to recalculate next run.

	json.NewEncoder(w).Encode(map[string]interface{}{"status": "updated", "id": id})
}

func (c *CronAPI) deleteAutomation(w http.ResponseWriter, _ *http.Request, id string) {
	if c.scheduler.RemoveJob(id) {
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted", "id": id})
	} else {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "automation not found"})
	}
}

func (c *CronAPI) triggerAutomation(w http.ResponseWriter, _ *http.Request, id string) {
	if c.scheduler.TriggerJob(id) {
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "triggered", "id": id})
	} else {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "automation not found"})
	}
}

func (c *CronAPI) automationHistory(w http.ResponseWriter, _ *http.Request, id string) {
	records := c.scheduler.History(id, 50)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"history": records,
		"count":   len(records),
	})
}
