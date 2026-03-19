package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// ProjectServeAPI serves generated project files for the canvas preview.
// It uses Go's http.FileServer under the hood but adds security checks
// to prevent path traversal attacks.
type ProjectServeAPI struct {
	dataDir string // base data directory (e.g., /app/data)
}

// RegisterProjectServeRoutes adds the static file serving endpoints
// for generated project previews.
//
// GET /v1/projects/latest/preview     - serves index.html from latest project
// GET /v1/projects/latest/preview/*   - serves any file from latest project
// GET /v1/projects/latest/files       - lists all files in latest project
func RegisterProjectServeRoutes(mux *http.ServeMux, dataDir string) {
	if dataDir == "" {
		dataDir = "/app/data"
	}
	ps := &ProjectServeAPI{dataDir: dataDir}
	mux.HandleFunc("/v1/projects/latest/preview", ps.handlePreview)
	mux.HandleFunc("/v1/projects/latest/preview/", ps.handlePreview)
	mux.HandleFunc("/v1/projects/latest/files", ps.handleListFiles)
	debug.Info("api", "Project serve API registered (/v1/projects/latest/preview)")
}

// handlePreview serves files from the latest project directory.
// If no specific file is requested, serves index.html.
func (ps *ProjectServeAPI) handlePreview(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	projectDir := filepath.Join(ps.dataDir, "projects", "latest")

	// Extract the file path from the URL.
	filePath := strings.TrimPrefix(r.URL.Path, "/v1/projects/latest/preview")
	filePath = strings.TrimPrefix(filePath, "/")
	if filePath == "" {
		filePath = "index.html"
	}

	// Security: prevent path traversal.
	filePath = filepath.Clean(filePath)
	if strings.Contains(filePath, "..") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	fullPath := filepath.Join(projectDir, filePath)

	// Verify the file exists.
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Set content type based on extension.
	ext := filepath.Ext(filePath)
	switch ext {
	case ".html", ".htm":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case ".json":
		w.Header().Set("Content-Type", "application/json")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".xml":
		w.Header().Set("Content-Type", "application/xml")
	}

	http.ServeFile(w, r, fullPath)
}

// handleListFiles returns a JSON list of all files in the latest project.
func (ps *ProjectServeAPI) handleListFiles(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	projectDir := filepath.Join(ps.dataDir, "projects", "latest")

	type FileInfo struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		Path string `json:"path"`
	}

	var files []FileInfo
	err := filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(projectDir, path)
		// Normalize path separators for the API response.
		relPath = strings.ReplaceAll(relPath, "\\", "/")
		files = append(files, FileInfo{
			Name: info.Name(),
			Size: info.Size(),
			Path: relPath,
		})
		return nil
	})

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if files == nil {
		files = []FileInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"files": files,
		"count": len(files),
		"preview_url": "/v1/projects/latest/preview",
	})
}
