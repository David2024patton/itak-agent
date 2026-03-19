package api

import (
	"encoding/json"
	"net/http"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// WhiteLabelAPI allows Agency-tier users to customize branding.
//
// What: REST endpoints for managing custom branding per tenant.
// Why:  Agency users want to resell iTaK Agent under their own brand.
// How:  Stores brand config per tenant and serves it to the frontend.
type WhiteLabelConfig struct {
	TenantID    string `json:"tenant_id"`
	CompanyName string `json:"company_name"`
	LogoURL     string `json:"logo_url"`
	FaviconURL  string `json:"favicon_url"`
	PrimaryColor string `json:"primary_color"`
	AccentColor  string `json:"accent_color"`
	CustomDomain string `json:"custom_domain,omitempty"`
	FooterText   string `json:"footer_text,omitempty"`
	HideITaKBrand bool  `json:"hide_itak_brand"`
}

// RegisterWhiteLabelRoutes adds white-label branding endpoints.
func RegisterWhiteLabelRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/whitelabel", handleWhiteLabel)
	mux.HandleFunc("/v1/whitelabel/preview", handleWhiteLabelPreview)
	debug.Info("api", "White-Label API registered (/v1/whitelabel*)")
}

// handleWhiteLabel gets or sets white-label branding for a tenant.
// GET /v1/whitelabel?tenant_id=xxx
// PUT /v1/whitelabel
func handleWhiteLabel(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Return default branding or tenant-specific.
		tenantID := r.URL.Query().Get("tenant_id")
		cfg := getWhiteLabelConfig(tenantID)
		json.NewEncoder(w).Encode(map[string]interface{}{"config": cfg})

	case http.MethodPut:
		var cfg WhiteLabelConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
			return
		}
		// In production, store to tenant database.
		debug.Info("whitelabel", "Updated branding for tenant %s: %s", cfg.TenantID, cfg.CompanyName)
		json.NewEncoder(w).Encode(map[string]string{"status": "updated"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleWhiteLabelPreview(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	// Returns a preview of what the white-labeled dashboard would look like.
	json.NewEncoder(w).Encode(map[string]interface{}{
		"preview_url": "/preview",
		"css_vars": map[string]string{
			"--accent":     r.URL.Query().Get("primary") ,
			"--bg-sidebar": r.URL.Query().Get("sidebar_bg"),
		},
	})
}

func getWhiteLabelConfig(tenantID string) WhiteLabelConfig {
	// Default branding.
	return WhiteLabelConfig{
		TenantID:     tenantID,
		CompanyName:  "iTaK Agent",
		PrimaryColor: "#6366f1",
		AccentColor:  "#8b5cf6",
		FooterText:   "Powered by iTaK Agent Cloud",
	}
}
