package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
)

// GraphAPI provides HTTP endpoints for exploring and editing the knowledge graph.
//
// What: REST API for graph visualization, exploration, and editing.
// Why:  Interactive graph browsing like Neo4j Browser, served from
//       the agent's own HTTP server.
// How:  Reads/writes the GraphBackend (iTaK Database) and returns JSON
//       for the D3.js force-directed graph frontend.
type GraphAPI struct {
	backend memory.GraphBackend
}

// RegisterGraphRoutes adds graph exploration and editing endpoints to the server mux.
func RegisterGraphRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	if backend == nil {
		debug.Warn("api", "Graph backend is nil, graph API disabled")
		return
	}

	g := &GraphAPI{backend: backend}

	// Read endpoints
	mux.HandleFunc("/v1/graph/nodes", g.handleNodes)
	mux.HandleFunc("/v1/graph/node/", g.handleNodeByID) // /v1/graph/node/{id}
	mux.HandleFunc("/v1/graph/neighbors/", g.handleNeighbors)
	mux.HandleFunc("/v1/graph/stats", g.handleStats)
	mux.HandleFunc("/v1/graph/search", g.handleSearch)

	// Edge creation
	mux.HandleFunc("/v1/graph/edges", g.handleCreateEdge)

	debug.Info("api", "Graph API endpoints registered (/v1/graph/*)")
}

// corsHeaders sets standard CORS + JSON headers.
func corsHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// GET /v1/graph/nodes?label=Action&limit=50
// POST /v1/graph/nodes  (create new node)
func (g *GraphAPI) handleNodes(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method == http.MethodPost {
		g.handleCreateNode(w, r)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "GET or POST only", http.StatusMethodNotAllowed)
		return
	}

	itakBackend, ok := g.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]string{"error": "graph API requires iTaK Database backend"})
		return
	}

	label := r.URL.Query().Get("label")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	db := itakBackend.DB()
	type graphResponse struct {
		Nodes []map[string]interface{} `json:"nodes"`
		Edges []map[string]interface{} `json:"edges"`
	}

	resp := graphResponse{
		Nodes: make([]map[string]interface{}, 0),
		Edges: make([]map[string]interface{}, 0),
	}

	labels := []string{"Session", "Action", "Page", "Search", "Message", "Entity", "Fact", "BrowserSession", "Conversation"}
	if label != "" {
		labels = []string{label}
	}

	nodeIDs := make(map[uint64]bool)
	for _, lbl := range labels {
		nodes, err := db.Graph.FindByLabel(lbl)
		if err != nil {
			continue
		}
		for _, n := range nodes {
			if len(resp.Nodes) >= limit {
				break
			}
			nodeIDs[n.ID] = true
			nodeMap := map[string]interface{}{
				"id":        n.ID,
				"labels":    n.Labels,
				"props":     n.Properties,
				"createdAt": n.CreatedAt,
			}
			resp.Nodes = append(resp.Nodes, nodeMap)
		}
	}

	allEdges := db.Graph.AllEdges()
	for _, e := range allEdges {
		if nodeIDs[e.SourceID] && nodeIDs[e.TargetID] {
			edgeMap := map[string]interface{}{
				"id":     e.ID,
				"type":   e.Type,
				"source": e.SourceID,
				"target": e.TargetID,
			}
			resp.Edges = append(resp.Edges, edgeMap)
		}
	}

	json.NewEncoder(w).Encode(resp)
}

// POST /v1/graph/nodes  { "labels": ["Entity"], "props": {"name":"foo"} }
func (g *GraphAPI) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	itakBackend, ok := g.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]string{"error": "requires iTaK Database backend"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to read body"})
		return
	}

	var req struct {
		Labels []string               `json:"labels"`
		Props  map[string]interface{} `json:"props"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	if len(req.Labels) == 0 {
		json.NewEncoder(w).Encode(map[string]string{"error": "labels required"})
		return
	}

	id, err := itakBackend.DB().CreateNode(req.Labels, req.Props, nil)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "status": "created"})
}

// GET/PUT/DELETE /v1/graph/node/{id}
func (g *GraphAPI) handleNodeByID(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	itakBackend, ok := g.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]string{"error": "requires iTaK Database backend"})
		return
	}

	// Extract ID from path: /v1/graph/node/123
	parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
	if len(parts) < 5 {
		json.NewEncoder(w).Encode(map[string]string{"error": "missing node ID"})
		return
	}
	id, err := strconv.ParseUint(parts[4], 10, 64)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid node ID"})
		return
	}

	db := itakBackend.DB()

	switch r.Method {
	case http.MethodGet:
		node, err := db.Graph.GetNode(id)
		if err != nil || node == nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "node not found"})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":        node.ID,
			"labels":    node.Labels,
			"props":     node.Properties,
			"createdAt": node.CreatedAt,
		})

	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to read body"})
			return
		}
		var req struct {
			Props map[string]interface{} `json:"props"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
			return
		}
		if err := db.Graph.UpdateNode(id, req.Props); err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "updated"})

	case http.MethodDelete:
		if err := db.Graph.DeleteNode(id); err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})

	default:
		http.Error(w, "GET, PUT, or DELETE only", http.StatusMethodNotAllowed)
	}
}

// GET /v1/graph/neighbors/{id}?depth=1
func (g *GraphAPI) handleNeighbors(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)

	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}

	itakBackend, ok := g.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]string{"error": "requires iTaK Database backend"})
		return
	}

	parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
	if len(parts) < 5 {
		json.NewEncoder(w).Encode(map[string]string{"error": "missing node ID"})
		return
	}
	id, err := strconv.ParseUint(parts[4], 10, 64)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid node ID"})
		return
	}

	depth := 1
	if d := r.URL.Query().Get("depth"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 && n <= 5 {
			depth = n
		}
	}

	db := itakBackend.DB()
	results, err := db.Graph.Traverse(id, depth, "")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(results)
}

// GET /v1/graph/stats
func (g *GraphAPI) handleStats(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)

	itakBackend, ok := g.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]string{"error": "requires iTaK Database backend"})
		return
	}

	stats := itakBackend.DB().Stats()
	json.NewEncoder(w).Encode(stats)
}

// GET /v1/graph/search?q=query&limit=10
func (g *GraphAPI) handleSearch(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)

	query := r.URL.Query().Get("q")
	if strings.TrimSpace(query) == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "missing query parameter 'q'"})
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	results, err := g.backend.SemanticSearch(nil, limit)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(results)
}

// POST /v1/graph/edges  { "type": "RELATES_TO", "source": 1, "target": 2, "props": {} }
func (g *GraphAPI) handleCreateEdge(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	itakBackend, ok := g.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]string{"error": "requires iTaK Database backend"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to read body"})
		return
	}

	var req struct {
		Type   string                 `json:"type"`
		Source uint64                 `json:"source"`
		Target uint64                 `json:"target"`
		Props  map[string]interface{} `json:"props"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Type == "" || req.Source == 0 || req.Target == 0 {
		json.NewEncoder(w).Encode(map[string]string{"error": "type, source, and target required"})
		return
	}

	id, err := itakBackend.DB().Graph.CreateEdge(req.Type, req.Source, req.Target, req.Props)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "status": "created"})
}

