package api

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
)

// EcosystemAPI ingests local project directories into the knowledge graph.
//
// What: Walks a local directory tree and creates structured nodes/edges.
// Why:  So the agent knows about all its own code, projects, and docs.
// How:  Creates Project hub nodes per directory, File nodes for source files,
//       and CONTAINS edges linking them. Also creates a root Ecosystem node.
type EcosystemAPI struct {
	backend memory.GraphBackend
}

// RegisterEcosystemRoutes adds local directory ingestion endpoints.
func RegisterEcosystemRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	if backend == nil {
		debug.Warn("api", "Graph backend is nil, ecosystem API disabled")
		return
	}

	e := &EcosystemAPI{backend: backend}
	mux.HandleFunc("/v1/ecosystem/ingest", e.handleIngest)
	debug.Info("api", "Ecosystem API registered (POST /v1/ecosystem/ingest)")
}

// sourceExts determines which file extensions to ingest as source code.
var sourceExts = map[string]string{
	".go":    "Go",
	".js":    "JavaScript",
	".ts":    "TypeScript",
	".py":    "Python",
	".html":  "HTML",
	".css":   "CSS",
	".json":  "JSON",
	".yaml":  "YAML",
	".yml":   "YAML",
	".toml":  "TOML",
	".md":    "Markdown",
	".sh":    "Shell",
	".ps1":   "PowerShell",
	".sql":   "SQL",
	".proto": "Protobuf",
	".mod":   "GoMod",
	".sum":   "GoSum",
}

// skipDirs are directories we never enter.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	".venv":        true,
	"dist":         true,
	"build":        true,
	".next":        true,
}

// POST /v1/ecosystem/ingest
//
// Body: { "root": "E:/.agent/iTaK Eco", "name": "iTaK Ecosystem" }
//
// Walks the root directory. For each top-level subdirectory (project),
// creates a Project node. For source files inside, creates File nodes
// with CONTAINS edges. Ties everything to a root Ecosystem node.
func (e *EcosystemAPI) handleIngest(w http.ResponseWriter, r *http.Request) {
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

	itakBackend, ok := e.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "requires iTaK Database backend"})
		return
	}

	var req struct {
		Root string `json:"root"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Root == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "root path required"})
		return
	}
	if req.Name == "" {
		req.Name = filepath.Base(req.Root)
	}

	info, err := os.Stat(req.Root)
	if err != nil || !info.IsDir() {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "root path not a valid directory"})
		return
	}

	debug.Info("ecosystem", "Ingesting ecosystem '%s' from: %s", req.Name, req.Root)
	db := itakBackend.DB()

	// Create root Ecosystem node
	ecoID, _ := db.CreateNode([]string{"Ecosystem"}, map[string]interface{}{
		"name":      req.Name,
		"root_path": req.Root,
		"ingested":  time.Now().Format(time.RFC3339),
	}, nil)

	var projectCount, fileCount, edgeCount int

	// Walk top-level directories as projects
	entries, err := os.ReadDir(req.Root)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "read dir: " + err.Error()})
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectName := entry.Name()
		if skipDirs[projectName] {
			continue
		}

		projectPath := filepath.Join(req.Root, projectName)

		// Create Project node
		projectID, _ := db.CreateNode([]string{"Project"}, map[string]interface{}{
			"name":      projectName,
			"path":      projectPath,
			"ecosystem": req.Name,
			"ingested":  time.Now().Format(time.RFC3339),
		}, nil)
		projectCount++

		// Edge: Ecosystem --HAS_PROJECT--> Project
		db.Graph.CreateEdge("HAS_PROJECT", ecoID, projectID, nil)
		edgeCount++

		// Walk source files inside each project
		filepath.WalkDir(projectPath, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				if skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}

			ext := strings.ToLower(filepath.Ext(d.Name()))
			lang, ok := sourceExts[ext]
			if !ok {
				return nil
			}

			relPath, _ := filepath.Rel(req.Root, path)
			finfo, _ := d.Info()
			size := int64(0)
			if finfo != nil {
				size = finfo.Size()
			}

			// Content fingerprint for dedup
			fp := fmt.Sprintf("%x", md5.Sum([]byte(relPath)))

			fileID, _ := db.CreateNode([]string{"File", lang}, map[string]interface{}{
				"name":        d.Name(),
				"path":        relPath,
				"abs_path":    path,
				"project":     projectName,
				"language":    lang,
				"size_bytes":  size,
				"fingerprint": fp,
				"ingested":    time.Now().Format(time.RFC3339),
			}, nil)

			// Edge: Project --CONTAINS--> File
			db.Graph.CreateEdge("CONTAINS", projectID, fileID, nil)
			edgeCount++
			fileCount++

			return nil
		})

		debug.Info("ecosystem", "Project '%s': ingested files", projectName)
	}

	// Also ingest root-level files (README, Feature_List, etc.)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		lang, ok := sourceExts[ext]
		if !ok {
			continue
		}
		fullPath := filepath.Join(req.Root, entry.Name())
		finfo, _ := entry.Info()
		size := int64(0)
		if finfo != nil {
			size = finfo.Size()
		}

		fileID, _ := db.CreateNode([]string{"File", lang}, map[string]interface{}{
			"name":       entry.Name(),
			"path":       entry.Name(),
			"abs_path":   fullPath,
			"project":    req.Name,
			"language":   lang,
			"size_bytes": size,
			"ingested":   time.Now().Format(time.RFC3339),
		}, nil)

		db.Graph.CreateEdge("CONTAINS", ecoID, fileID, nil)
		edgeCount++
		fileCount++
	}

	debug.Info("ecosystem", "Ingest complete: %d projects, %d files, %d edges", projectCount, fileCount, edgeCount)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ingested",
		"ecosystem": req.Name,
		"projects":  projectCount,
		"files":     fileCount,
		"edges":     edgeCount,
		"eco_node":  ecoID,
	})
}
