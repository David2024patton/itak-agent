package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// PluginAPI provides auto-discovery of user plugins from the /plugins directory.
//
// What: REST endpoints for listing and managing plugin scripts.
// Why:  Users can drop scripts into /plugins and they auto-register as tools.
// How:  Scans the plugins directory for supported file types and returns metadata.
type PluginAPI struct {
	pluginsDir string
}

// PluginInfo describes a discovered plugin.
type PluginInfo struct {
	Name     string `json:"name"`
	File     string `json:"file"`
	Type     string `json:"type"` // "python", "bash", "javascript", "go"
	Size     int64  `json:"size"`
	Enabled  bool   `json:"enabled"`
	Category string `json:"category,omitempty"`
}

// RegisterPluginRoutes adds plugin management endpoints.
func RegisterPluginRoutes(mux *http.ServeMux, dataDir string) {
	pluginsDir := filepath.Join(dataDir, "plugins")
	os.MkdirAll(pluginsDir, 0755)

	p := &PluginAPI{pluginsDir: pluginsDir}
	mux.HandleFunc("/v1/plugins", p.handleList)
	mux.HandleFunc("/v1/plugins/toggle", p.handleToggle)
	debug.Info("api", "Plugin API registered (/v1/plugins*), dir=%s", pluginsDir)
}

var pluginExtensions = map[string]string{
	".py":   "python",
	".sh":   "bash",
	".bash": "bash",
	".js":   "javascript",
	".ts":   "javascript",
	".go":   "go",
	".lua":  "lua",
}

// handleList returns all discovered plugins.
// GET /v1/plugins
func (p *PluginAPI) handleList(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	plugins := p.scanPlugins()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"plugins": plugins,
		"count":   len(plugins),
		"dir":     p.pluginsDir,
	})
}

// handleToggle enables or disables a plugin by renaming it.
// POST /v1/plugins/toggle  { "file": "my_tool.py", "enabled": true }
func (p *PluginAPI) handleToggle(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		File    string `json:"file"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	// Sanitize filename to prevent path traversal.
	base := filepath.Base(req.File)
	if base != req.File || strings.Contains(base, "..") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid filename"})
		return
	}

	srcPath := filepath.Join(p.pluginsDir, base)
	if req.Enabled {
		// Enable: remove .disabled suffix.
		if strings.HasSuffix(base, ".disabled") {
			dstPath := filepath.Join(p.pluginsDir, strings.TrimSuffix(base, ".disabled"))
			os.Rename(srcPath, dstPath)
		}
	} else {
		// Disable: add .disabled suffix.
		if !strings.HasSuffix(base, ".disabled") {
			dstPath := filepath.Join(p.pluginsDir, base+".disabled")
			os.Rename(srcPath, dstPath)
		}
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (p *PluginAPI) scanPlugins() []PluginInfo {
	var plugins []PluginInfo

	entries, err := os.ReadDir(p.pluginsDir)
	if err != nil {
		return plugins
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))

		// Check if disabled.
		disabled := false
		if strings.HasSuffix(name, ".disabled") {
			disabled = true
			ext = strings.ToLower(filepath.Ext(strings.TrimSuffix(name, ".disabled")))
		}

		pluginType, ok := pluginExtensions[ext]
		if !ok {
			continue
		}

		info, _ := entry.Info()
		var size int64
		if info != nil {
			size = info.Size()
		}

		displayName := strings.TrimSuffix(name, ".disabled")
		displayName = strings.TrimSuffix(displayName, ext)
		displayName = strings.ReplaceAll(displayName, "_", " ")
		displayName = strings.ReplaceAll(displayName, "-", " ")

		plugins = append(plugins, PluginInfo{
			Name:    displayName,
			File:    name,
			Type:    pluginType,
			Size:    size,
			Enabled: !disabled,
		})
	}

	return plugins
}
