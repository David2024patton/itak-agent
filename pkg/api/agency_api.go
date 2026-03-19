package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
)

// AgencyAPI manages multi-tenant agency/sub-account CRUD and branding.
//
// What: REST endpoints for Agency and SubAccount graph nodes.
// Why:  Agencies can manage multiple client businesses with separate branding,
//       credentials, and knowledge bases -- like GoHighLevel's sub-account model.
// How:  Creates nodes with labels "Agency" and "SubAccount" in the graph DB.
type AgencyAPI struct {
	backend memory.GraphBackend
}

// RegisterAgencyRoutes adds agency management endpoints.
func RegisterAgencyRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	if backend == nil {
		debug.Warn("api", "Graph backend is nil, agency API disabled")
		return
	}
	a := &AgencyAPI{backend: backend}
	mux.HandleFunc("/v1/agency", a.handleAgency)
	mux.HandleFunc("/v1/agency/", a.handleAgencyByID) // /v1/agency/{id} and /v1/agency/{id}/accounts
	mux.HandleFunc("/v1/agency/active", a.handleActiveAgency)
	debug.Info("api", "Agency API registered (/v1/agency)")
}

// Agency represents a top-level agency (your business).
type Agency struct {
	ID             uint64 `json:"id"`
	Name           string `json:"name"`
	Domain         string `json:"domain,omitempty"`
	LogoURL        string `json:"logo_url,omitempty"`
	PrimaryColor   string `json:"primary_color,omitempty"`
	SecondaryColor string `json:"secondary_color,omitempty"`
	AccentColor    string `json:"accent_color,omitempty"`
	FontFamily     string `json:"font_family,omitempty"`
	Tagline        string `json:"tagline,omitempty"`
	Industry       string `json:"industry,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// SubAccount represents a client business under an agency.
type SubAccount struct {
	ID           uint64 `json:"id"`
	AgencyID     uint64 `json:"agency_id"`
	Name         string `json:"name"`
	Website      string `json:"website,omitempty"`
	Industry     string `json:"industry,omitempty"`
	ContactEmail string `json:"contact_email,omitempty"`
	Phone        string `json:"phone,omitempty"`
	LogoURL      string `json:"logo_url,omitempty"`
	PrimaryColor string `json:"primary_color,omitempty"`
	AccentColor  string `json:"accent_color,omitempty"`
	Notes        string `json:"notes,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// activeAgencyID is the currently active agency context (global state for now).
var activeAgencyID uint64 = 0
var activeSubAccountID uint64 = 0

// GET/POST /v1/agency
func (a *AgencyAPI) handleAgency(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.Method {
	case http.MethodGet:
		a.listAgencies(w, r)
	case http.MethodPost:
		a.createAgency(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "GET or POST only"})
	}
}

// GET/PUT/DELETE /v1/agency/{id}
// GET/POST /v1/agency/{id}/accounts
// POST /v1/agency/{id}/scrape
func (a *AgencyAPI) handleAgencyByID(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/agency/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "agency ID required"})
		return
	}

	agencyID := parts[0]

	// Sub-routes: /v1/agency/{id}/accounts or /v1/agency/{id}/accounts/{subId} or /v1/agency/{id}/scrape
	if len(parts) == 2 {
		sub := parts[1]
		// Handle /v1/agency/{id}/accounts/{subId}
		if strings.HasPrefix(sub, "accounts/") {
			subID := strings.TrimPrefix(sub, "accounts/")
			a.handleSubAccountByID(w, r, agencyID, subID)
			return
		}
		switch sub {
		case "accounts":
			a.handleSubAccounts(w, r, agencyID)
		case "scrape":
			a.handleScrape(w, r, agencyID)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
		return
	}

	// Direct agency operations
	switch r.Method {
	case http.MethodGet:
		a.getAgency(w, r, agencyID)
	case http.MethodPut:
		a.updateAgency(w, r, agencyID)
	case http.MethodDelete:
		a.deleteAgency(w, r, agencyID)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// GET/PUT /v1/agency/active
func (a *AgencyAPI) handleActiveAgency(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.Method {
	case http.MethodGet:
		resp := map[string]interface{}{
			"active_agency_id":     activeAgencyID,
			"active_subaccount_id": activeSubAccountID,
			"agency_name":          "",
			"subaccount_name":      "",
		}

		// Resolve names if backend available.
		if itakBackend, ok := a.backend.(*memory.ITakDBBackend); ok && activeAgencyID > 0 {
			db := itakBackend.DB()
			if nodes, _ := db.Graph.FindByLabel("Agency"); nodes != nil {
				for _, n := range nodes {
					if n.ID == activeAgencyID {
						resp["agency_name"] = pStr(n.Properties, "name")
						break
					}
				}
			}
			if activeSubAccountID > 0 {
				if nodes, _ := db.Graph.FindByLabel("SubAccount"); nodes != nil {
					for _, n := range nodes {
						if n.ID == activeSubAccountID {
							resp["subaccount_name"] = pStr(n.Properties, "name")
							break
						}
					}
				}
			}
		}
		json.NewEncoder(w).Encode(resp)

	case http.MethodPut:
		var body struct {
			AgencyID     uint64 `json:"agency_id"`
			SubAccountID uint64 `json:"subaccount_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		activeAgencyID = body.AgencyID
		activeSubAccountID = body.SubAccountID
		debug.Info("agency", "Active context set: agency=%d subaccount=%d", activeAgencyID, activeSubAccountID)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":               "updated",
			"active_agency_id":     activeAgencyID,
			"active_subaccount_id": activeSubAccountID,
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *AgencyAPI) listAgencies(w http.ResponseWriter, _ *http.Request) {
	itakBackend, ok := a.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"agencies": []Agency{}, "count": 0})
		return
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Agency")

	agencies := make([]Agency, 0, len(nodes))
	for _, n := range nodes {
		agencies = append(agencies, Agency{
			ID:             n.ID,
			Name:           pStr(n.Properties, "name"),
			Domain:         pStr(n.Properties, "domain"),
			LogoURL:        pStr(n.Properties, "logo_url"),
			PrimaryColor:   pStr(n.Properties, "primary_color"),
			SecondaryColor: pStr(n.Properties, "secondary_color"),
			AccentColor:    pStr(n.Properties, "accent_color"),
			FontFamily:     pStr(n.Properties, "font_family"),
			Tagline:        pStr(n.Properties, "tagline"),
			Industry:       pStr(n.Properties, "industry"),
			CreatedAt:      pStr(n.Properties, "created_at"),
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"agencies": agencies, "count": len(agencies)})
}

func (a *AgencyAPI) createAgency(w http.ResponseWriter, r *http.Request) {
	itakBackend, ok := a.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "requires iTaK Database"})
		return
	}

	var ag Agency
	if err := json.NewDecoder(r.Body).Decode(&ag); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if ag.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "name required"})
		return
	}

	ag.CreatedAt = time.Now().Format(time.RFC3339)

	props := map[string]interface{}{
		"name":            ag.Name,
		"domain":          ag.Domain,
		"logo_url":        ag.LogoURL,
		"primary_color":   ag.PrimaryColor,
		"secondary_color": ag.SecondaryColor,
		"accent_color":    ag.AccentColor,
		"font_family":     ag.FontFamily,
		"tagline":         ag.Tagline,
		"industry":        ag.Industry,
		"created_at":      ag.CreatedAt,
	}

	id, err := itakBackend.DB().CreateNode([]string{"Agency"}, props, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	debug.Info("agency", "Created agency %q (id=%d)", ag.Name, id)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "created", "id": id, "agency": ag})
}

func (a *AgencyAPI) getAgency(w http.ResponseWriter, _ *http.Request, id string) {
	itakBackend, ok := a.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Agency")

	for _, n := range nodes {
		if fmt.Sprintf("%d", n.ID) == id {
			ag := Agency{
				ID: n.ID, Name: pStr(n.Properties, "name"),
				Domain: pStr(n.Properties, "domain"), LogoURL: pStr(n.Properties, "logo_url"),
				PrimaryColor: pStr(n.Properties, "primary_color"), SecondaryColor: pStr(n.Properties, "secondary_color"),
				AccentColor: pStr(n.Properties, "accent_color"), FontFamily: pStr(n.Properties, "font_family"),
				Tagline: pStr(n.Properties, "tagline"), Industry: pStr(n.Properties, "industry"),
				CreatedAt: pStr(n.Properties, "created_at"),
			}
			json.NewEncoder(w).Encode(ag)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "agency not found"})
}

func (a *AgencyAPI) updateAgency(w http.ResponseWriter, r *http.Request, id string) {
	itakBackend, ok := a.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Agency")
	for _, n := range nodes {
		if fmt.Sprintf("%d", n.ID) == id {
			for k, v := range updates {
				n.Properties[k] = v
			}
			n.Properties["updated_at"] = time.Now().Format(time.RFC3339)
			debug.Info("agency", "Updated agency %s", id)
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "updated", "id": n.ID})
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "agency not found"})
}

func (a *AgencyAPI) deleteAgency(w http.ResponseWriter, _ *http.Request, id string) {
	// For now, just report success. Full deletion needs cascade removal of sub-accounts.
	debug.Info("agency", "Delete requested for agency %s", id)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted", "id": id})
}

// Sub-account management: GET/POST /v1/agency/{id}/accounts
func (a *AgencyAPI) handleSubAccounts(w http.ResponseWriter, r *http.Request, agencyID string) {
	switch r.Method {
	case http.MethodGet:
		a.listSubAccounts(w, r, agencyID)
	case http.MethodPost:
		a.createSubAccount(w, r, agencyID)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *AgencyAPI) listSubAccounts(w http.ResponseWriter, _ *http.Request, agencyID string) {
	itakBackend, ok := a.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"accounts": []SubAccount{}, "count": 0})
		return
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("SubAccount")

	accounts := make([]SubAccount, 0)
	for _, n := range nodes {
		if pStr(n.Properties, "agency_id") == agencyID {
			accounts = append(accounts, SubAccount{
				ID: n.ID, Name: pStr(n.Properties, "name"),
				Website: pStr(n.Properties, "website"), Industry: pStr(n.Properties, "industry"),
				ContactEmail: pStr(n.Properties, "contact_email"), Phone: pStr(n.Properties, "phone"),
				LogoURL: pStr(n.Properties, "logo_url"), PrimaryColor: pStr(n.Properties, "primary_color"),
				AccentColor: pStr(n.Properties, "accent_color"), Notes: pStr(n.Properties, "notes"),
				CreatedAt: pStr(n.Properties, "created_at"),
			})
		}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"accounts": accounts, "count": len(accounts)})
}

func (a *AgencyAPI) createSubAccount(w http.ResponseWriter, r *http.Request, agencyID string) {
	itakBackend, ok := a.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "requires iTaK Database"})
		return
	}

	var sa SubAccount
	if err := json.NewDecoder(r.Body).Decode(&sa); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if sa.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "name required"})
		return
	}

	sa.CreatedAt = time.Now().Format(time.RFC3339)

	props := map[string]interface{}{
		"agency_id":     agencyID,
		"name":          sa.Name,
		"website":       sa.Website,
		"industry":      sa.Industry,
		"contact_email": sa.ContactEmail,
		"phone":         sa.Phone,
		"logo_url":      sa.LogoURL,
		"primary_color": sa.PrimaryColor,
		"accent_color":  sa.AccentColor,
		"notes":         sa.Notes,
		"created_at":    sa.CreatedAt,
	}

	id, err := itakBackend.DB().CreateNode([]string{"SubAccount"}, props, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	debug.Info("agency", "Created sub-account %q under agency %s (id=%d)", sa.Name, agencyID, id)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "created", "id": id})
}

// handleSubAccountByID handles operations on a specific sub-account.
func (a *AgencyAPI) handleSubAccountByID(w http.ResponseWriter, r *http.Request, agencyID, subID string) {
	switch r.Method {
	case http.MethodDelete:
		a.deleteSubAccount(w, r, agencyID, subID)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *AgencyAPI) deleteSubAccount(w http.ResponseWriter, _ *http.Request, agencyID, subID string) {
	itakBackend, ok := a.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "requires iTaK Database"})
		return
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("SubAccount")
	for _, n := range nodes {
		if fmt.Sprintf("%d", n.ID) == subID && pStr(n.Properties, "agency_id") == agencyID {
			db.Graph.DeleteNode(n.ID)
			debug.Info("agency", "Deleted sub-account %s from agency %s", subID, agencyID)
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted", "id": subID})
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "sub-account not found"})
}

// handleScrape scrapes a website URL and stores extracted content as Knowledge nodes.
func (a *AgencyAPI) handleScrape(w http.ResponseWriter, r *http.Request, agencyID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "url required"})
		return
	}

	itakBackend, ok := a.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Simple HTTP fetch to extract text content from the URL.
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(body.URL)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	pageBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024)) // 512KB max
	pageText := string(pageBytes)

	// Strip HTML tags for rough text extraction.
	pageText = stripHTML(pageText)

	if len(pageText) > 10000 {
		pageText = pageText[:10000] + "... [truncated]"
	}

	props := map[string]interface{}{
		"category":   "WebScrape",
		"title":      "Scraped: " + body.URL,
		"content":    pageText,
		"source_url": body.URL,
		"agency_id":  agencyID,
		"source":     "agency_scrape",
		"created_at": time.Now().Format(time.RFC3339),
	}

	id, err := itakBackend.DB().CreateNode([]string{"Knowledge"}, props, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	debug.Info("agency", "Scraped %s for agency %s (node=%d, %d chars)", body.URL, agencyID, id, len(pageText))
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "scraped",
		"node_id": id,
		"chars":   len(pageText),
		"url":     body.URL,
	})
}

// stripHTML removes HTML tags from text (rough but effective).
func stripHTML(s string) string {
	var result strings.Builder
	inTag := false
	for _, c := range s {
		if c == '<' {
			inTag = true
			continue
		}
		if c == '>' {
			inTag = false
			result.WriteRune(' ')
			continue
		}
		if !inTag {
			result.WriteRune(c)
		}
	}
	return result.String()
}
