package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
	"github.com/David2024patton/iTaKDatabase/pkg/table"
)

// KnowledgeAPI provides repo ingestion, unified search, auto-documentation,
// and dependency auditing on top of the multi-database engine.
//
// What: High-level knowledge operations that combine all 4 database engines.
// Why:  Gives the agent a single API to ingest any Git repo, search across
//       all engines, auto-document codebases, and audit dependencies.
// How:  Downloads repo archives from GitHub/GitLab/Bitbucket/Codeberg,
//       feeds them through the existing multi-DB pipeline, then provides
//       cross-engine queries.
type KnowledgeAPI struct {
	backend memory.GraphBackend
}

// RegisterKnowledgeRoutes adds all knowledge endpoints.
func RegisterKnowledgeRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	if backend == nil {
		debug.Warn("api", "Graph backend is nil, knowledge API disabled")
		return
	}

	k := &KnowledgeAPI{backend: backend}

	// Repo ingestion
	mux.HandleFunc("/v1/graph/ingest/repo", k.handleRepoIngest)

	// Unified cross-database search
	mux.HandleFunc("/v1/knowledge/search", k.handleUnifiedSearch)

	// Auto-documentation
	mux.HandleFunc("/v1/knowledge/describe/", k.handleDescribe)

	// Dependency audit
	mux.HandleFunc("/v1/knowledge/deps/", k.handleDeps)

	// List all ingested templates/repos
	mux.HandleFunc("/v1/knowledge/list", k.handleListKnowledge)

	debug.Info("api", "Knowledge API registered: /v1/graph/ingest/repo, /v1/knowledge/{search,describe,deps,list}")
}

// ── Repo URL Parsing ─────────────────────────────────────────────

// repoInfo holds parsed repository details.
type repoInfo struct {
	Platform string // "github", "gitlab", "bitbucket", "codeberg", "gitea"
	Owner    string
	Repo     string
	Branch   string
	ZipURL   string
	Name     string // display name: "owner/repo"
}

// parseRepoURL extracts owner/repo/branch from any supported Git hosting URL.
//
// Supports:
//   - github.com/owner/repo
//   - gitlab.com/owner/repo
//   - bitbucket.org/owner/repo
//   - codeberg.org/owner/repo
//   - Any Gitea instance (gitea.example.com/owner/repo)
//   - Shorthand: "owner/repo" (defaults to GitHub)
func parseRepoURL(rawURL string) (*repoInfo, error) {
	rawURL = strings.TrimSpace(rawURL)

	// Handle shorthand: "owner/repo" or "owner/repo@branch"
	if !strings.Contains(rawURL, "://") && !strings.Contains(rawURL, ".") {
		parts := strings.SplitN(rawURL, "@", 2)
		ownerRepo := parts[0]
		branch := "main"
		if len(parts) > 1 {
			branch = parts[1]
		}
		segments := strings.SplitN(ownerRepo, "/", 2)
		if len(segments) != 2 {
			return nil, fmt.Errorf("invalid shorthand %q, expected owner/repo", rawURL)
		}
		return &repoInfo{
			Platform: "github",
			Owner:    segments[0],
			Repo:     segments[1],
			Branch:   branch,
			ZipURL:   fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/%s.zip", segments[0], segments[1], branch),
			Name:     ownerRepo,
		}, nil
	}

	// Strip protocol
	url := rawURL
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimSuffix(url, "/")

	parts := strings.SplitN(url, "/", 4)
	if len(parts) < 3 {
		return nil, fmt.Errorf("cannot parse URL %q, expected host/owner/repo", rawURL)
	}

	host := strings.ToLower(parts[0])
	owner := parts[1]
	repo := parts[2]

	// Extract branch from URL if present (e.g., /tree/main)
	branch := "main"
	if len(parts) > 3 {
		rest := parts[3]
		if strings.HasPrefix(rest, "tree/") || strings.HasPrefix(rest, "-/tree/") {
			branchPart := strings.TrimPrefix(rest, "-/")
			branchPart = strings.TrimPrefix(branchPart, "tree/")
			branch = strings.SplitN(branchPart, "/", 2)[0]
		}
	}

	info := &repoInfo{
		Owner:  owner,
		Repo:   repo,
		Branch: branch,
		Name:   owner + "/" + repo,
	}

	switch {
	case strings.Contains(host, "github.com"):
		info.Platform = "github"
		info.ZipURL = fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/%s.zip", owner, repo, branch)
	case strings.Contains(host, "gitlab.com"):
		info.Platform = "gitlab"
		info.ZipURL = fmt.Sprintf("https://gitlab.com/%s/%s/-/archive/%s/%s-%s.zip", owner, repo, branch, repo, branch)
	case strings.Contains(host, "bitbucket.org"):
		info.Platform = "bitbucket"
		info.ZipURL = fmt.Sprintf("https://bitbucket.org/%s/%s/get/%s.zip", owner, repo, branch)
	case strings.Contains(host, "codeberg.org"):
		info.Platform = "codeberg"
		info.ZipURL = fmt.Sprintf("https://codeberg.org/%s/%s/archive/%s.zip", owner, repo, branch)
	default:
		// Assume Gitea-compatible
		info.Platform = "gitea"
		info.ZipURL = fmt.Sprintf("https://%s/%s/%s/archive/%s.zip", host, owner, repo, branch)
	}

	return info, nil
}

// ── Repo Ingest Handler ──────────────────────────────────────────

// POST /v1/graph/ingest/repo
//
// Body: { "url": "https://github.com/owner/repo" }
//   or: { "url": "owner/repo" }  (shorthand, defaults to GitHub)
//   or: { "url": "owner/repo@branch" }
func (k *KnowledgeAPI) handleRepoIngest(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "POST only"})
		return
	}

	var req struct {
		URL    string `json:"url"`
		Branch string `json:"branch,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	if req.URL == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing 'url' field"})
		return
	}

	// Parse the URL
	info, err := parseRepoURL(req.URL)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if req.Branch != "" {
		info.Branch = req.Branch
		// Rebuild zip URL with override branch
		rebuilt, _ := parseRepoURL(req.URL)
		if rebuilt != nil {
			rebuilt.Branch = req.Branch
			info = rebuilt
		}
	}

	debug.Info("ingest", "Downloading repo: %s (%s) branch=%s", info.Name, info.Platform, info.Branch)
	debug.Info("ingest", "ZIP URL: %s", info.ZipURL)

	// Download the zip archive
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(info.ZipURL)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": "download failed: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{
			"error":       fmt.Sprintf("download returned %d", resp.StatusCode),
			"zip_url":     info.ZipURL,
			"hint":        "Check that the repo is public and the branch exists",
		})
		return
	}

	// Save to temp file
	tmpFile, err := os.CreateTemp("", "repo-ingest-*.zip")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "temp file: " + err.Error()})
		return
	}
	defer os.Remove(tmpFile.Name())

	written, err := io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "save zip: " + err.Error()})
		return
	}

	debug.Info("ingest", "Downloaded %d bytes for %s", written, info.Name)

	// Get raw DB
	itakBackend, ok := k.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "backend does not support direct DB access"})
		return
	}
	db := itakBackend.DB()

	// Create IngestAPI and delegate to shared ingest pipeline.
	ig := &IngestAPI{backend: k.backend}
	result := ig.ingestZipFile(db, tmpFile.Name(), info.Name)

	// Add repo metadata to the template node
	if result.TemplateID > 0 {
		tplNode, _ := db.Graph.GetNode(result.TemplateID)
		if tplNode != nil {
			tplNode.Properties["platform"] = info.Platform
			tplNode.Properties["owner"] = info.Owner
			tplNode.Properties["repo"] = info.Repo
			tplNode.Properties["branch"] = info.Branch
			tplNode.Properties["zip_url"] = info.ZipURL
			srcURL := fmt.Sprintf("https://%s.com/%s/%s", info.Platform, info.Owner, info.Repo)
			if info.Platform == "bitbucket" {
				srcURL = fmt.Sprintf("https://bitbucket.org/%s/%s", info.Owner, info.Repo)
			}
			tplNode.Properties["source_url"] = srcURL
			tplNode.Properties["download_size"] = written
			db.Graph.UpdateNode(result.TemplateID, tplNode.Properties)
			result.Platform = info.Platform
			result.SourceURL = srcURL
		}
	}

	debug.Info("ingest", "Repo %s ingested: %d files across 4 engines", info.Name, result.Files)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// ── Shared Result Type ───────────────────────────────────────────

// processResult holds the outcome of a multi-DB ingest.
type processResult struct {
	TemplateID uint64                 `json:"template_id"`
	Template   string                 `json:"template"`
	Files      int                    `json:"files"`
	Engines    map[string]interface{} `json:"engines"`
	FileNodes  map[string]uint64      `json:"file_nodes"`
	Platform   string                 `json:"platform,omitempty"`
	SourceURL  string                 `json:"source_url,omitempty"`
	Status     string                 `json:"status"`
}

// ── Unified Search ───────────────────────────────────────────────

// GET /v1/knowledge/search?q=query&limit=20
//
// Searches across all 4 engines and merges results:
//   - FTS: keyword match with BM25 scoring
//   - Graph: node property text search
//   - Table: metadata query on ingested_files
func (k *KnowledgeAPI) handleUnifiedSearch(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	q := r.URL.Query().Get("q")
	if q == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing 'q' parameter"})
		return
	}

	itakBackend, ok := k.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "backend not available"})
		return
	}
	db := itakBackend.DB()

	var hits []knowledgeHit
	seen := map[uint64]bool{}

	// 1. FTS search
	ftsResults, _ := db.Search.Search(q, 20)
	for _, r := range ftsResults {
		seen[r.DocID] = true
		node, _ := db.Graph.GetNode(r.DocID)
		var labels []string
		var props map[string]interface{}
		if node != nil {
			labels = node.Labels
			// Return props without content (too big)
			props = make(map[string]interface{})
			for k, v := range node.Properties {
				if k != "content" {
					props[k] = v
				}
			}
		}
		hits = append(hits, knowledgeHit{
			NodeID: r.DocID, Labels: labels, Source: "fts",
			Score: r.Score, Props: props,
		})
	}

	// 2. Graph: search node properties for the query
	allLabels := []string{
		"Template", "Page", "Stylesheet", "Script", "Image", "Font",
		"SourceCode", "Config", "Document", "File", "Audio", "Video",
		"Session", "Action", "Message", "Entity", "Fact", "Search",
	}
	qLower := strings.ToLower(q)
	for _, lbl := range allLabels {
		nodes, _ := db.Graph.FindByLabel(lbl)
		for _, n := range nodes {
			if seen[n.ID] {
				continue
			}
			// Check if any property value contains the query
			for _, v := range n.Properties {
				if strings.Contains(strings.ToLower(fmt.Sprintf("%v", v)), qLower) {
					props := make(map[string]interface{})
					for pk, pv := range n.Properties {
						if pk != "content" {
							props[pk] = pv
						}
					}
					hits = append(hits, knowledgeHit{
						NodeID: n.ID, Labels: n.Labels, Source: "graph",
						Score: 0.5, Props: props,
					})
					seen[n.ID] = true
					break
				}
			}
		}
	}

	// 3. Table: search ingested_files by filename/path
	rows, err := db.Table.Select("ingested_files", []table.Condition{
		{Column: "filename", Op: "LIKE", Value: "%" + q + "%"},
	}, 20)
	if err == nil {
		for _, row := range rows {
			nodeID, _ := row.Data["graph_node_id"].(float64)
			nid := uint64(nodeID)
			if !seen[nid] && nid > 0 {
				seen[nid] = true
				hits = append(hits, knowledgeHit{
					NodeID: nid, Source: "table",
					Score: 0.3, Props: row.Data,
				})
			}
		}
	}

	// Sort by score descending
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].Score > hits[j].Score
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"query":   q,
		"results": hits,
		"count":   len(hits),
		"engines": map[string]int{
			"fts":   len(ftsResults),
			"graph": countHitsBySource(hits, "graph"),
			"table": countHitsBySource(hits, "table"),
		},
	})
}

// ── Auto-Documentation ──────────────────────────────────────────

// GET /v1/knowledge/describe/{template_id}
//
// Generates auto-documentation for an ingested template/repo by
// analyzing its graph structure, file types, and relationships.
func (k *KnowledgeAPI) handleDescribe(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)

	itakBackend, ok := k.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "backend not available"})
		return
	}
	db := itakBackend.DB()

	// Parse template ID from URL
	parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing template ID"})
		return
	}
	var tplID uint64
	fmt.Sscanf(parts[len(parts)-1], "%d", &tplID)

	tplNode, _ := db.Graph.GetNode(tplID)
	if tplNode == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "template not found"})
		return
	}

	// Get all CONTAINS edges from this template
	neighbors, _ := db.Graph.Traverse(tplID, 1, "CONTAINS")
	fileSummary := map[string]int{} // label -> count
	var fileList []map[string]interface{}
	var totalSize uint64

	fileSet := map[uint64]bool{}
	for _, n := range neighbors {
		fileSet[n.Node.ID] = true
		if len(n.Node.Labels) > 0 {
			fileSummary[n.Node.Labels[0]]++
		}
		size, _ := n.Node.Properties["size"].(float64)
		totalSize += uint64(size)
		fileList = append(fileList, map[string]interface{}{
			"id": n.Node.ID, "path": n.Node.Properties["path"],
			"label": n.Node.Labels, "size": n.Node.Properties["size"],
		})
	}

	// Count relationship types
	allEdges := db.Graph.AllEdges()
	relCounts := map[string]int{}
	for _, e := range allEdges {
		if fileSet[e.SourceID] || fileSet[e.TargetID] {
			relCounts[e.Type]++
		}
	}

	// Build description
	name, _ := tplNode.Properties["name"].(string)
	platform, _ := tplNode.Properties["platform"].(string)
	sourceURL, _ := tplNode.Properties["source_url"].(string)

	doc := map[string]interface{}{
		"template_id":   tplID,
		"name":          name,
		"platform":      platform,
		"source_url":    sourceURL,
		"total_files":   len(neighbors),
		"total_size":    totalSize,
		"file_types":    fileSummary,
		"relationships": relCounts,
		"files":         fileList,
		"summary":       buildAutoSummary(name, fileSummary, relCounts, len(neighbors)),
	}

	json.NewEncoder(w).Encode(doc)
}

func buildAutoSummary(name string, types map[string]int, rels map[string]int, total int) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("**%s** contains %d files", name, total))

	typeStr := []string{}
	for label, count := range types {
		typeStr = append(typeStr, fmt.Sprintf("%d %s", count, label))
	}
	if len(typeStr) > 0 {
		sort.Strings(typeStr)
		parts = append(parts, fmt.Sprintf("(%s)", strings.Join(typeStr, ", ")))
	}

	if rels["IMPORTS"] > 0 || rels["INCLUDES"] > 0 {
		parts = append(parts, fmt.Sprintf("with %d imports and %d includes",
			rels["IMPORTS"], rels["INCLUDES"]))
	}

	return strings.Join(parts, " ")
}

// ── Dependency Audit ─────────────────────────────────────────────

// GET /v1/knowledge/deps/{template_id}
//
// Maps all dependency relationships for an ingested template/repo.
// Shows what imports what, external references, and the dependency tree.
func (k *KnowledgeAPI) handleDeps(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)

	itakBackend, ok := k.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "backend not available"})
		return
	}
	db := itakBackend.DB()

	parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing template ID"})
		return
	}
	var tplID uint64
	fmt.Sscanf(parts[len(parts)-1], "%d", &tplID)

	tplNode, _ := db.Graph.GetNode(tplID)
	if tplNode == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "template not found"})
		return
	}

	// Get all files in this template
	neighbors, _ := db.Graph.Traverse(tplID, 1, "CONTAINS")
	fileIDs := map[uint64]string{} // ID -> path
	for _, n := range neighbors {
		path, _ := n.Node.Properties["path"].(string)
		fileIDs[n.Node.ID] = path
	}

	// Find all dependency edges between these files
	type dep struct {
		From     string `json:"from"`
		To       string `json:"to"`
		Type     string `json:"type"` // IMPORTS, INCLUDES, REFERENCES
		FromID   uint64 `json:"from_id"`
		ToID     uint64 `json:"to_id"`
	}

	var deps []dep
	allEdges := db.Graph.AllEdges()
	for _, e := range allEdges {
		if e.Type == "CONTAINS" {
			continue // Skip structural edges
		}
		fromPath, fromOK := fileIDs[e.SourceID]
		toPath, toOK := fileIDs[e.TargetID]
		if fromOK && toOK {
			deps = append(deps, dep{
				From: fromPath, To: toPath, Type: e.Type,
				FromID: e.SourceID, ToID: e.TargetID,
			})
		}
	}

	// Build dependency tree (which files are imported the most)
	importCounts := map[string]int{}
	for _, d := range deps {
		importCounts[d.To]++
	}

	// Find entry points (files that are not imported by anything)
	imported := map[string]bool{}
	for _, d := range deps {
		imported[d.To] = true
	}
	var entryPoints []string
	for _, path := range fileIDs {
		if !imported[path] {
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".html" || ext == ".js" || ext == ".ts" {
				entryPoints = append(entryPoints, path)
			}
		}
	}
	sort.Strings(entryPoints)

	name, _ := tplNode.Properties["name"].(string)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"template_id":   tplID,
		"name":          name,
		"total_files":   len(fileIDs),
		"dependencies":  deps,
		"dep_count":     len(deps),
		"import_counts": importCounts,
		"entry_points":  entryPoints,
	})
}

// ── List Knowledge ───────────────────────────────────────────────

// GET /v1/knowledge/list
//
// Lists all ingested templates and repos with their stats.
func (k *KnowledgeAPI) handleListKnowledge(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)

	itakBackend, ok := k.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "backend not available"})
		return
	}
	db := itakBackend.DB()

	templates, _ := db.Graph.FindByLabel("Template")
	var items []map[string]interface{}

	for _, t := range templates {
		// Count files under this template
		children, _ := db.Graph.Traverse(t.ID, 1, "CONTAINS")
		item := map[string]interface{}{
			"id":         t.ID,
			"name":       t.Properties["name"],
			"platform":   t.Properties["platform"],
			"source_url": t.Properties["source_url"],
			"file_count": len(children),
			"uploaded":   t.Properties["uploaded"],
		}
		items = append(items, item)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"templates": items,
		"count":     len(items),
	})
}

// knowledgeHit is a search result from any engine.
type knowledgeHit struct {
	NodeID uint64                 `json:"node_id"`
	Labels []string               `json:"labels"`
	Source string                 `json:"source"`
	Score  float64                `json:"score"`
	Props  map[string]interface{} `json:"props,omitempty"`
}

// countHitsBySource counts search hits from a specific engine.
func countHitsBySource(hits []knowledgeHit, source string) int {
	c := 0
	for _, h := range hits {
		if h.Source == source {
			c++
		}
	}
	return c
}

// Ensure imports are used.
var _ = (*table.Column)(nil)
