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

// ContactAPI manages CRM contacts per agency/sub-account.
type ContactAPI struct {
	backend memory.GraphBackend
}

// Contact represents a client/lead for a sub-account.
type Contact struct {
	ID           uint64 `json:"id"`
	AgencyID     string `json:"agency_id,omitempty"`
	SubAccountID string `json:"subaccount_id,omitempty"`
	Name         string `json:"name"`
	Email        string `json:"email,omitempty"`
	Phone        string `json:"phone,omitempty"`
	Company      string `json:"company,omitempty"`
	Tags         string `json:"tags,omitempty"`
	Notes        string `json:"notes,omitempty"`
	Status       string `json:"status"` // lead, active, inactive
	Source       string `json:"source,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// RegisterContactRoutes adds contact management endpoints.
func RegisterContactRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	if backend == nil {
		return
	}
	c := &ContactAPI{backend: backend}
	mux.HandleFunc("/v1/contacts", c.handleContacts)
	mux.HandleFunc("/v1/contacts/", c.handleContactByID)
	debug.Info("api", "Contact API registered (/v1/contacts)")
}

func (c *ContactAPI) handleContacts(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	switch r.Method {
	case http.MethodGet:
		c.listContacts(w, r)
	case http.MethodPost:
		c.createContact(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (c *ContactAPI) handleContactByID(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/contacts/")
	switch r.Method {
	case http.MethodPut:
		c.updateContact(w, r, id)
	case http.MethodDelete:
		c.deleteContact(w, r, id)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (c *ContactAPI) listContacts(w http.ResponseWriter, r *http.Request) {
	itakBackend, ok := c.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"contacts": []Contact{}, "count": 0})
		return
	}
	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Contact")

	agencyFilter := r.URL.Query().Get("agency_id")
	subFilter := r.URL.Query().Get("subaccount_id")

	contacts := make([]Contact, 0)
	for _, n := range nodes {
		if agencyFilter != "" && pStr(n.Properties, "agency_id") != agencyFilter {
			continue
		}
		if subFilter != "" && pStr(n.Properties, "subaccount_id") != subFilter {
			continue
		}
		contacts = append(contacts, Contact{
			ID: n.ID, Name: pStr(n.Properties, "name"),
			Email: pStr(n.Properties, "email"), Phone: pStr(n.Properties, "phone"),
			Company: pStr(n.Properties, "company"), Tags: pStr(n.Properties, "tags"),
			Notes: pStr(n.Properties, "notes"), Status: pStr(n.Properties, "status"),
			Source: pStr(n.Properties, "source"),
			AgencyID: pStr(n.Properties, "agency_id"), SubAccountID: pStr(n.Properties, "subaccount_id"),
			CreatedAt: pStr(n.Properties, "created_at"),
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"contacts": contacts, "count": len(contacts)})
}

func (c *ContactAPI) createContact(w http.ResponseWriter, r *http.Request) {
	itakBackend, ok := c.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var ct Contact
	if err := json.NewDecoder(r.Body).Decode(&ct); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if ct.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "name required"})
		return
	}
	if ct.Status == "" {
		ct.Status = "lead"
	}
	ct.CreatedAt = time.Now().Format(time.RFC3339)
	props := map[string]interface{}{
		"name": ct.Name, "email": ct.Email, "phone": ct.Phone,
		"company": ct.Company, "tags": ct.Tags, "notes": ct.Notes,
		"status": ct.Status, "source": ct.Source,
		"agency_id": ct.AgencyID, "subaccount_id": ct.SubAccountID,
		"created_at": ct.CreatedAt,
	}
	id, err := itakBackend.DB().CreateNode([]string{"Contact"}, props, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "created", "id": id})
}

func (c *ContactAPI) updateContact(w http.ResponseWriter, r *http.Request, id string) {
	itakBackend, ok := c.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var updates map[string]interface{}
	json.NewDecoder(r.Body).Decode(&updates)
	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Contact")
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

func (c *ContactAPI) deleteContact(w http.ResponseWriter, _ *http.Request, id string) {
	itakBackend, ok := c.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Contact")
	for _, n := range nodes {
		if fmt.Sprintf("%d", n.ID) == id {
			db.Graph.DeleteNode(n.ID)
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted", "id": id})
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}
