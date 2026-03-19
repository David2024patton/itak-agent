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

// PipelineAPI manages automation pipelines and node-graph workflows.
type PipelineAPI struct {
	backend memory.GraphBackend
}

// ── Legacy linear pipeline model (kept for compatibility) ───────────

// PipelineStep represents a single step in a linear pipeline.
type PipelineStep struct {
	Order    int    `json:"order"`
	Action   string `json:"action"`
	Agent    string `json:"agent,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
	Config   string `json:"config,omitempty"`
	DelayMin int    `json:"delay_min,omitempty"`
}

// ── New workflow node graph model ───────────────────────────────────

// WorkflowNode is a single block on the visual canvas.
// Type determines what it does when executed.
type WorkflowNode struct {
	ID     string                 `json:"id"`     // client-generated uuid
	Type   string                 `json:"type"`   // prompt, agent, webhook, api_call, websocket, condition, transform, delay
	Label  string                 `json:"label"`  // display name on canvas
	X      int                    `json:"x"`      // canvas X position
	Y      int                    `json:"y"`      // canvas Y position
	Config map[string]interface{} `json:"config"` // type-specific settings
}

// WorkflowEdge is a directed connection between two nodes.
type WorkflowEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"` // source node id
	Target string `json:"target"` // target node id
	Label  string `json:"label,omitempty"`
}

// Pipeline represents a workflow (supports both linear steps and node graph).
type Pipeline struct {
	ID           uint64         `json:"id"`
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	AgencyID     string         `json:"agency_id,omitempty"`
	SubAccountID string         `json:"subaccount_id,omitempty"`
	ProjectID    string         `json:"project_id,omitempty"`
	Steps        []PipelineStep `json:"steps,omitempty"`        // legacy linear
	Nodes        []WorkflowNode `json:"nodes,omitempty"`        // node graph
	Edges        []WorkflowEdge `json:"edges,omitempty"`        // connections
	ScheduleType string         `json:"schedule_type,omitempty"`
	Schedule     string         `json:"schedule,omitempty"`
	Status       string         `json:"status"`
	LastRun      string         `json:"last_run,omitempty"`
	RunCount     int            `json:"run_count"`
	CreatedAt    string         `json:"created_at"`
}

// RegisterPipelineRoutes adds pipeline/workflow management endpoints.
func RegisterPipelineRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	if backend == nil {
		return
	}
	p := &PipelineAPI{backend: backend}
	mux.HandleFunc("/v1/pipelines", p.handlePipelines)
	mux.HandleFunc("/v1/pipelines/", p.handlePipelineByID)
	mux.HandleFunc("/v1/workflows", p.handlePipelines)          // alias
	mux.HandleFunc("/v1/workflows/", p.handlePipelineByID)      // alias
	debug.Info("api", "Pipeline/Workflow API registered (/v1/pipelines, /v1/workflows)")
}

func (p *PipelineAPI) handlePipelines(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	switch r.Method {
	case http.MethodGet:
		p.listPipelines(w, r)
	case http.MethodPost:
		p.createPipeline(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (p *PipelineAPI) handlePipelineByID(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	// Parse ID from either /v1/pipelines/{id} or /v1/workflows/{id}
	id := r.URL.Path
	id = strings.TrimPrefix(id, "/v1/pipelines/")
	id = strings.TrimPrefix(id, "/v1/workflows/")

	// Check for execute action: /v1/workflows/{id}/execute
	if strings.HasSuffix(id, "/execute") {
		id = strings.TrimSuffix(id, "/execute")
		if r.Method == http.MethodPost {
			p.executeWorkflow(w, r, id)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	switch r.Method {
	case http.MethodGet:
		p.getPipeline(w, id)
	case http.MethodPut:
		p.updatePipeline(w, r, id)
	case http.MethodDelete:
		p.deletePipeline(w, id)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (p *PipelineAPI) listPipelines(w http.ResponseWriter, _ *http.Request) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"pipelines": []Pipeline{}, "count": 0})
		return
	}
	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Pipeline")
	pipelines := make([]Pipeline, 0)
	for _, n := range nodes {
		pl := Pipeline{
			ID: n.ID, Name: pStr(n.Properties, "name"),
			Description: pStr(n.Properties, "description"),
			AgencyID: pStr(n.Properties, "agency_id"), SubAccountID: pStr(n.Properties, "subaccount_id"),
			ProjectID: pStr(n.Properties, "project_id"),
			ScheduleType: pStr(n.Properties, "schedule_type"), Schedule: pStr(n.Properties, "schedule"),
			Status: pStr(n.Properties, "status"), LastRun: pStr(n.Properties, "last_run"),
			CreatedAt: pStr(n.Properties, "created_at"),
		}
		if v := pStr(n.Properties, "run_count"); v != "" {
			fmt.Sscanf(v, "%d", &pl.RunCount)
		}
		// Deserialize legacy steps.
		if stepsJSON := pStr(n.Properties, "steps"); stepsJSON != "" {
			json.Unmarshal([]byte(stepsJSON), &pl.Steps)
		}
		// Deserialize node graph.
		if nodesJSON := pStr(n.Properties, "nodes"); nodesJSON != "" {
			json.Unmarshal([]byte(nodesJSON), &pl.Nodes)
		}
		if edgesJSON := pStr(n.Properties, "edges"); edgesJSON != "" {
			json.Unmarshal([]byte(edgesJSON), &pl.Edges)
		}
		pipelines = append(pipelines, pl)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"pipelines": pipelines, "count": len(pipelines)})
}

func (p *PipelineAPI) createPipeline(w http.ResponseWriter, r *http.Request) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var pl Pipeline
	if err := json.NewDecoder(r.Body).Decode(&pl); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if pl.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "name required"})
		return
	}
	if pl.Status == "" {
		pl.Status = "draft"
	}
	pl.CreatedAt = time.Now().Format(time.RFC3339)

	stepsJSON, _ := json.Marshal(pl.Steps)
	nodesJSON, _ := json.Marshal(pl.Nodes)
	edgesJSON, _ := json.Marshal(pl.Edges)

	props := map[string]interface{}{
		"name": pl.Name, "description": pl.Description,
		"agency_id": pl.AgencyID, "subaccount_id": pl.SubAccountID,
		"project_id": pl.ProjectID, "steps": string(stepsJSON),
		"nodes": string(nodesJSON), "edges": string(edgesJSON),
		"schedule_type": pl.ScheduleType, "schedule": pl.Schedule,
		"status": pl.Status, "run_count": "0", "created_at": pl.CreatedAt,
	}
	id, err := itakBackend.DB().CreateNode([]string{"Pipeline"}, props, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "created", "id": id})
}

func (p *PipelineAPI) getPipeline(w http.ResponseWriter, id string) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Pipeline")
	for _, n := range nodes {
		if fmt.Sprintf("%d", n.ID) == id {
			pl := Pipeline{ID: n.ID, Name: pStr(n.Properties, "name"), Description: pStr(n.Properties, "description"),
				Status: pStr(n.Properties, "status"), CreatedAt: pStr(n.Properties, "created_at")}
			if stepsJSON := pStr(n.Properties, "steps"); stepsJSON != "" {
				json.Unmarshal([]byte(stepsJSON), &pl.Steps)
			}
			if nodesJSON := pStr(n.Properties, "nodes"); nodesJSON != "" {
				json.Unmarshal([]byte(nodesJSON), &pl.Nodes)
			}
			if edgesJSON := pStr(n.Properties, "edges"); edgesJSON != "" {
				json.Unmarshal([]byte(edgesJSON), &pl.Edges)
			}
			json.NewEncoder(w).Encode(pl)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

func (p *PipelineAPI) updatePipeline(w http.ResponseWriter, r *http.Request, id string) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var updates map[string]interface{}
	json.NewDecoder(r.Body).Decode(&updates)
	// Serialize array fields to JSON strings for graph storage.
	for _, key := range []string{"steps", "nodes", "edges"} {
		if val, ok := updates[key]; ok {
			if j, err := json.Marshal(val); err == nil {
				updates[key] = string(j)
			}
		}
	}
	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Pipeline")
	for _, n := range nodes {
		if fmt.Sprintf("%d", n.ID) == id {
			for k, v := range updates {
				n.Properties[k] = v
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "updated", "id": n.ID})
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

func (p *PipelineAPI) deletePipeline(w http.ResponseWriter, id string) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Pipeline")
	for _, n := range nodes {
		if fmt.Sprintf("%d", n.ID) == id {
			db.Graph.DeleteNode(n.ID)
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted", "id": id})
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

// executeWorkflow runs a workflow's node graph as a DAG.
// POST /v1/workflows/{id}/execute
func (p *PipelineAPI) executeWorkflow(w http.ResponseWriter, _ *http.Request, id string) {
	itakBackend, ok := p.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	db := itakBackend.DB()
	gnodes, _ := db.Graph.FindByLabel("Pipeline")
	var pl Pipeline
	found := false
	for _, n := range gnodes {
		if fmt.Sprintf("%d", n.ID) == id {
			pl = Pipeline{ID: n.ID, Name: pStr(n.Properties, "name"), Status: pStr(n.Properties, "status")}
			if nodesJSON := pStr(n.Properties, "nodes"); nodesJSON != "" {
				json.Unmarshal([]byte(nodesJSON), &pl.Nodes)
			}
			if edgesJSON := pStr(n.Properties, "edges"); edgesJSON != "" {
				json.Unmarshal([]byte(edgesJSON), &pl.Edges)
			}
			// Update last_run and run_count.
			n.Properties["last_run"] = time.Now().Format(time.RFC3339)
			rc := 0
			if v := pStr(n.Properties, "run_count"); v != "" {
				fmt.Sscanf(v, "%d", &rc)
			}
			n.Properties["run_count"] = fmt.Sprintf("%d", rc+1)
			found = true
			break
		}
	}
	if !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if len(pl.Nodes) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "error", "message": "workflow has no nodes",
		})
		return
	}

	// Build adjacency and find start nodes (no incoming edges).
	incoming := map[string]bool{}
	for _, e := range pl.Edges {
		incoming[e.Target] = true
	}
	startNodes := []string{}
	for _, n := range pl.Nodes {
		if !incoming[n.ID] {
			startNodes = append(startNodes, n.ID)
		}
	}

	// Topological order for execution planning.
	nodeMap := map[string]WorkflowNode{}
	for _, n := range pl.Nodes {
		nodeMap[n.ID] = n
	}
	adj := map[string][]string{}
	for _, e := range pl.Edges {
		adj[e.Source] = append(adj[e.Source], e.Target)
	}

	// BFS traversal from start nodes.
	executionOrder := []WorkflowNode{}
	visited := map[string]bool{}
	queue := append([]string{}, startNodes...)
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if visited[curr] {
			continue
		}
		visited[curr] = true
		if n, ok := nodeMap[curr]; ok {
			executionOrder = append(executionOrder, n)
		}
		for _, next := range adj[curr] {
			if !visited[next] {
				queue = append(queue, next)
			}
		}
	}

	// Build execution plan (the actual execution is async, this returns the plan).
	plan := make([]map[string]interface{}, 0, len(executionOrder))
	for i, node := range executionOrder {
		plan = append(plan, map[string]interface{}{
			"step":   i + 1,
			"node":   node.ID,
			"type":   node.Type,
			"label":  node.Label,
			"config": node.Config,
		})
	}

	debug.Info("workflow", "Executing workflow %q (%d nodes, %d edges)", pl.Name, len(pl.Nodes), len(pl.Edges))

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":          "executing",
		"workflow":        pl.Name,
		"workflow_id":     pl.ID,
		"nodes_total":     len(pl.Nodes),
		"execution_order": plan,
	})
}
