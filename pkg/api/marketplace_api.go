package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/David2024patton/iTaKAgent/pkg/config"
	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/skill"
)

// ── Marketplace Types ─────────────────────────────────────────────

// MarketplaceItem is a single entry in the marketplace catalog.
type MarketplaceItem struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"` // "skill", "agent", "plugin", "tool"
	DisplayName    string   `json:"display_name"`
	Description    string   `json:"description"`
	Category       string   `json:"category"`
	Division       string   `json:"division,omitempty"`
	Author         string   `json:"author"`
	Version        string   `json:"version"`
	Tags           []string `json:"tags"`
	Icon           string   `json:"icon"`
	IsCore         bool     `json:"is_core"`
	RequiresSkills []string `json:"requires_skills,omitempty"`
	RequiresTools  []string `json:"requires_tools,omitempty"`
	Tools          []string `json:"tools,omitempty"`
	DownloadURL    string   `json:"download_url,omitempty"`
	Installed      bool     `json:"installed"`
}

// marketplaceHandler holds references needed by marketplace endpoints.
type marketplaceHandler struct {
	skillRepo *skill.Repository
	cfg       *config.Config
	dataDir   string
}

// RegisterMarketplaceRoutes adds marketplace endpoints to the mux.
func RegisterMarketplaceRoutes(mux *http.ServeMux, skillRepo *skill.Repository, cfg *config.Config, dataDir string) {
	h := &marketplaceHandler{
		skillRepo: skillRepo,
		cfg:       cfg,
		dataDir:   dataDir,
	}

	mux.HandleFunc("/v1/marketplace/catalog", h.handleCatalog)
	mux.HandleFunc("/v1/marketplace/installed", h.handleInstalled)
	mux.HandleFunc("/v1/marketplace/install", h.handleInstall)
	mux.HandleFunc("/v1/marketplace/uninstall", h.handleUninstall)
	mux.HandleFunc("/v1/marketplace/dependencies", h.handleDependencies)
}

// ── Handlers ──────────────────────────────────────────────────────

// handleCatalog returns the full marketplace catalog with installed status.
func (h *marketplaceHandler) handleCatalog(w http.ResponseWriter, r *http.Request) {
	items := h.loadCatalog()

	// Mark installed items.
	installedSkills := h.getInstalledSkills()
	enabledPlugins := h.getEnabledPlugins()

	for i := range items {
		switch items[i].Type {
		case "skill":
			if _, ok := installedSkills[items[i].Name]; ok {
				items[i].Installed = true
			}
		case "plugin":
			if enabledPlugins[items[i].Name] {
				items[i].Installed = true
			}
		case "agent":
			// Agents from the catalog are always "available" (seed data).
			// They become "installed" when added to the active config.
			items[i].Installed = h.isAgentActive(items[i].Name)
		}
	}

	// Compute category counts.
	counts := map[string]int{}
	for _, item := range items {
		counts[item.Type]++
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"total": len(items),
		"counts": counts,
	})
}

// handleInstalled returns only installed/active items.
func (h *marketplaceHandler) handleInstalled(w http.ResponseWriter, r *http.Request) {
	var installed []MarketplaceItem

	// Installed skills.
	if h.skillRepo != nil {
		for _, s := range h.skillRepo.List() {
			installed = append(installed, MarketplaceItem{
				Name:        s.Name,
				Type:        "skill",
				DisplayName: s.Name,
				Description: s.Description,
				Category:    "skill",
				Author:      "local",
				Version:     "installed",
				Tags:        s.Tags,
				Icon:        "📦",
				Installed:   true,
			})
		}
	}

	// Enabled plugins.
	enabledPlugins := h.getEnabledPlugins()
	for name, enabled := range enabledPlugins {
		if enabled {
			installed = append(installed, MarketplaceItem{
				Name:        name,
				Type:        "plugin",
				DisplayName: name,
				Description: "Enabled channel plugin",
				Category:    "channel",
				Author:      "iTaK Core",
				Version:     "0.2.0",
				Icon:        "🔌",
				Installed:   true,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": installed,
		"total": len(installed),
	})
}

// handleInstall installs an item and auto-installs its dependencies.
func (h *marketplaceHandler) handleInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}

	var req struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "'name' is required"})
		return
	}

	debug.Info("marketplace", "Install request: type=%s name=%s", req.Type, req.Name)

	// Find the catalog entry.
	catalog := h.loadCatalog()
	var target *MarketplaceItem
	for i := range catalog {
		if catalog[i].Name == req.Name && catalog[i].Type == req.Type {
			target = &catalog[i]
			break
		}
	}

	switch req.Type {
	case "skill":
		h.installSkill(w, req.Name, target)

	case "agent":
		// Auto-install required skills for this agent.
		var depsInstalled []map[string]string
		if target != nil && len(target.RequiresSkills) > 0 {
			installedSkills := h.getInstalledSkills()
			for _, skillName := range target.RequiresSkills {
				if installedSkills[skillName] {
					continue // already installed
				}
				debug.Info("marketplace", "Auto-installing dependency skill: %s for agent %s", skillName, req.Name)
				if err := h.installSkillByName(skillName, catalog); err != nil {
					debug.Warn("marketplace", "Failed to auto-install skill %s: %v", skillName, err)
				} else {
					depsInstalled = append(depsInstalled, map[string]string{"name": skillName, "type": "skill"})
				}
			}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":                 "installed",
			"name":                   req.Name,
			"type":                   "agent",
			"message":                "Agent activated. Use the Agents page to configure it.",
			"dependencies_installed": depsInstalled,
		})

	case "plugin":
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "info",
			"message": "Plugins are enabled via the plugins section in itakagent.yaml. Toggle the 'enabled' flag and restart.",
		})

	case "tool":
		// Tools are builtin Go code - always available.
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "info",
			"name":    req.Name,
			"message": "Tools are built into the agent binary. Assign them to agents via the Agents config.",
		})

	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown type: " + req.Type})
	}
}

// installSkill handles installing a single skill via the API response.
func (h *marketplaceHandler) installSkill(w http.ResponseWriter, name string, entry *MarketplaceItem) {
	if h.skillRepo == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "skill repository not initialized"})
		return
	}

	var downloadURL string
	if entry != nil {
		downloadURL = entry.DownloadURL
	}

	if downloadURL == "" {
		destDir, err := h.skillRepo.ValidateInstallPath(name)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create dir: " + err.Error()})
			return
		}
		skillMD := fmt.Sprintf("---\nname: %s\ndescription: Installed from marketplace\n---\n\n# %s\n\nInstalled via iTaK Marketplace.\n", name, name)
		if err := os.WriteFile(filepath.Join(destDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write SKILL.md: " + err.Error()})
			return
		}
		h.skillRepo.Refresh()
	} else {
		if err := h.skillRepo.Install(name, downloadURL); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "installed", "name": name, "type": "skill"})
}

// installSkillByName installs a skill without writing an HTTP response (for dependency auto-install).
func (h *marketplaceHandler) installSkillByName(name string, catalog []MarketplaceItem) error {
	if h.skillRepo == nil {
		return fmt.Errorf("skill repository not initialized")
	}

	var downloadURL string
	for _, item := range catalog {
		if item.Name == name && item.Type == "skill" {
			downloadURL = item.DownloadURL
			break
		}
	}

	if downloadURL == "" {
		destDir, err := h.skillRepo.ValidateInstallPath(name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return err
		}
		skillMD := fmt.Sprintf("---\nname: %s\ndescription: Installed from marketplace\n---\n\n# %s\n\nInstalled via iTaK Marketplace.\n", name, name)
		if err := os.WriteFile(filepath.Join(destDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
			return err
		}
		h.skillRepo.Refresh()
	} else {
		if err := h.skillRepo.Install(name, downloadURL); err != nil {
			return err
		}
	}
	return nil
}

// handleDependencies previews what installing an item would require.
func (h *marketplaceHandler) handleDependencies(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	itemType := r.URL.Query().Get("type")
	if name == "" || itemType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "'name' and 'type' query params required"})
		return
	}

	catalog := h.loadCatalog()
	var target *MarketplaceItem
	for i := range catalog {
		if catalog[i].Name == name && catalog[i].Type == itemType {
			target = &catalog[i]
			break
		}
	}

	if target == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "item not found"})
		return
	}

	// Build dependency list.
	installedSkills := h.getInstalledSkills()
	var missingSkills []string
	for _, s := range target.RequiresSkills {
		if !installedSkills[s] {
			missingSkills = append(missingSkills, s)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":            target.Name,
		"type":            target.Type,
		"is_core":         target.IsCore,
		"requires_skills": target.RequiresSkills,
		"requires_tools":  target.RequiresTools,
		"missing_skills":  missingSkills,
		"tools":           target.Tools,
	})
}

// handleUninstall removes an installed skill. Core items cannot be uninstalled.
func (h *marketplaceHandler) handleUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}

	var req struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	// Check if this is a core item.
	catalog := h.loadCatalog()
	for _, item := range catalog {
		if item.Name == req.Name && item.Type == req.Type && item.IsCore {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "cannot uninstall core item: " + req.Name,
			})
			return
		}
	}

	if req.Type != "skill" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "only skills can be uninstalled. Plugins are toggled via config; agents/tools are built-in.",
		})
		return
	}

	if h.skillRepo == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "skill repository not initialized"})
		return
	}

	destDir, err := h.skillRepo.ValidateInstallPath(req.Name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := os.RemoveAll(destDir); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "remove: " + err.Error()})
		return
	}

	h.skillRepo.Refresh()
	debug.Info("marketplace", "Uninstalled skill: %s", req.Name)

	writeJSON(w, http.StatusOK, map[string]string{"status": "uninstalled", "name": req.Name})
}

// ── Helpers ───────────────────────────────────────────────────────

// loadCatalog reads the marketplace catalog JSON from disk.
func (h *marketplaceHandler) loadCatalog() []MarketplaceItem {
	catalogPath := filepath.Join(h.dataDir, "marketplace", "catalog.json")
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		debug.Warn("marketplace", "Failed to read catalog: %v", err)
		return nil
	}

	var items []MarketplaceItem
	if err := json.Unmarshal(data, &items); err != nil {
		debug.Warn("marketplace", "Failed to parse catalog: %v", err)
		return nil
	}
	return items
}

// getInstalledSkills returns a map of installed skill names.
func (h *marketplaceHandler) getInstalledSkills() map[string]bool {
	installed := make(map[string]bool)
	if h.skillRepo != nil {
		for _, s := range h.skillRepo.List() {
			installed[s.Name] = true
		}
	}
	return installed
}

// getEnabledPlugins returns which plugins are enabled in config.
func (h *marketplaceHandler) getEnabledPlugins() map[string]bool {
	if h.cfg == nil {
		return nil
	}
	return map[string]bool{
		"web":        h.cfg.Plugins.Web.Enabled,
		"dashboard":  h.cfg.Plugins.Dashboard.Enabled,
		"discord":    h.cfg.Plugins.Discord.Enabled,
		"visionclaw": h.cfg.Plugins.VisionClaw.Enabled,
		"cli":        h.cfg.Plugins.CLI.Enabled,
	}
}

// isAgentActive checks if an agent name exists in the current config.
func (h *marketplaceHandler) isAgentActive(name string) bool {
	if h.cfg == nil {
		return false
	}
	for _, a := range h.cfg.Agents {
		if a.Name == name {
			return true
		}
	}
	return false
}
