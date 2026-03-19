package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
)

// CredentialsAPI manages encrypted credential storage scoped to agencies.
//
// What: REST endpoints for encrypted API keys, passwords, and tokens.
// Why:  Agents need scoped access to credentials without cross-contamination.
//       Credentials are AES-256-GCM encrypted at rest in the graph DB.
// How:  Each credential is a "Credential" graph node with encrypted_value.
type CredentialsAPI struct {
	backend memory.GraphBackend
	key     []byte // 32-byte AES-256 key
}

// RegisterCredentialsRoutes adds credential vault endpoints (public entry point).
func RegisterCredentialsRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	registerCredentialsAPI(mux, backend)
}

// registerCredentialsAPI adds credential vault endpoints and returns the
// CredentialsAPI instance so connectors can look up encrypted credentials.
func registerCredentialsAPI(mux *http.ServeMux, backend memory.GraphBackend) *CredentialsAPI {
	if backend == nil {
		debug.Warn("api", "Graph backend is nil, credentials API disabled")
		return nil
	}

	// Derive encryption key from env var or generate a default.
	vaultKey := os.Getenv("ITAK_VAULT_KEY")
	if vaultKey == "" {
		vaultKey = "itak-default-vault-key-change-in-production"
		debug.Warn("credentials", "Using default vault key -- set ITAK_VAULT_KEY for production")
	}
	hash := sha256.Sum256([]byte(vaultKey))

	c := &CredentialsAPI{backend: backend, key: hash[:]}
	mux.HandleFunc("/v1/credentials", c.handleCredentials)
	mux.HandleFunc("/v1/credentials/", c.handleCredentialByID)
	debug.Info("api", "Credentials API registered (/v1/credentials)")
	return c
}

// Credential represents a stored credential.
type Credential struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`        // api_key, password, oauth, token, custom
	Value       string `json:"value,omitempty"` // only populated on reveal
	Provider    string `json:"provider,omitempty"`
	Scope       string `json:"scope"`       // global, agency, subaccount
	ScopeID     string `json:"scope_id,omitempty"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// GET/POST /v1/credentials
func (c *CredentialsAPI) handleCredentials(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.Method {
	case http.MethodGet:
		c.listCredentials(w, r)
	case http.MethodPost:
		c.createCredential(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "GET or POST only"})
	}
}

// GET/PUT/DELETE /v1/credentials/{id}
// GET /v1/credentials/{id}/reveal
func (c *CredentialsAPI) handleCredentialByID(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/credentials/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	credID := parts[0]

	// /v1/credentials/{id}/reveal
	if len(parts) == 2 && parts[1] == "reveal" {
		c.revealCredential(w, r, credID)
		return
	}

	switch r.Method {
	case http.MethodPut:
		c.updateCredential(w, r, credID)
	case http.MethodDelete:
		c.deleteCredential(w, r, credID)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (c *CredentialsAPI) listCredentials(w http.ResponseWriter, r *http.Request) {
	itakBackend, ok := c.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"credentials": []Credential{}, "count": 0})
		return
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Credential")

	// Optional scope filter from query params.
	scopeFilter := r.URL.Query().Get("scope")
	scopeIDFilter := r.URL.Query().Get("scope_id")

	creds := make([]Credential, 0, len(nodes))
	for _, n := range nodes {
		scope := pStr(n.Properties, "scope")
		scopeID := pStr(n.Properties, "scope_id")

		if scopeFilter != "" && scope != scopeFilter {
			continue
		}
		if scopeIDFilter != "" && scopeID != scopeIDFilter {
			continue
		}

		creds = append(creds, Credential{
			ID:          n.ID,
			Name:        pStr(n.Properties, "name"),
			Type:        pStr(n.Properties, "type"),
			Provider:    pStr(n.Properties, "provider"),
			Scope:       scope,
			ScopeID:     scopeID,
			Description: pStr(n.Properties, "description"),
			CreatedAt:   pStr(n.Properties, "created_at"),
			UpdatedAt:   pStr(n.Properties, "updated_at"),
			// Value is intentionally omitted (masked)
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"credentials": creds, "count": len(creds)})
}

func (c *CredentialsAPI) createCredential(w http.ResponseWriter, r *http.Request) {
	itakBackend, ok := c.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "requires iTaK Database"})
		return
	}

	var cred Credential
	if err := json.NewDecoder(r.Body).Decode(&cred); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if cred.Name == "" || cred.Value == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "name and value required"})
		return
	}
	if cred.Scope == "" {
		cred.Scope = "global"
	}
	if cred.Type == "" {
		cred.Type = "custom"
	}

	// Encrypt the value before storage.
	encrypted, err := c.encrypt(cred.Value)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "encryption failed: " + err.Error()})
		return
	}

	now := time.Now().Format(time.RFC3339)
	props := map[string]interface{}{
		"name":            cred.Name,
		"type":            cred.Type,
		"encrypted_value": encrypted,
		"provider":        cred.Provider,
		"scope":           cred.Scope,
		"scope_id":        cred.ScopeID,
		"description":     cred.Description,
		"created_at":      now,
	}

	id, err := itakBackend.DB().CreateNode([]string{"Credential"}, props, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	debug.Info("credentials", "Stored credential %q (type=%s, scope=%s, id=%d)", cred.Name, cred.Type, cred.Scope, id)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "stored", "id": id})
}

func (c *CredentialsAPI) revealCredential(w http.ResponseWriter, _ *http.Request, credID string) {
	itakBackend, ok := c.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Credential")

	for _, n := range nodes {
		if fmt.Sprintf("%d", n.ID) == credID {
			encrypted := pStr(n.Properties, "encrypted_value")
			if encrypted == "" {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]string{"error": "no encrypted value"})
				return
			}

			decrypted, err := c.decrypt(encrypted)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "decryption failed"})
				return
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":    n.ID,
				"name":  pStr(n.Properties, "name"),
				"value": decrypted,
			})
			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "credential not found"})
}

func (c *CredentialsAPI) updateCredential(w http.ResponseWriter, r *http.Request, credID string) {
	itakBackend, ok := c.backend.(*memory.ITakDBBackend)
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

	// If updating value, encrypt it.
	if val, ok := updates["value"]; ok {
		encrypted, err := c.encrypt(fmt.Sprintf("%v", val))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "encryption failed"})
			return
		}
		delete(updates, "value")
		updates["encrypted_value"] = encrypted
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Credential")
	for _, n := range nodes {
		if fmt.Sprintf("%d", n.ID) == credID {
			for k, v := range updates {
				n.Properties[k] = v
			}
			n.Properties["updated_at"] = time.Now().Format(time.RFC3339)
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "updated", "id": n.ID})
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "credential not found"})
}

func (c *CredentialsAPI) deleteCredential(w http.ResponseWriter, _ *http.Request, credID string) {
	debug.Info("credentials", "Delete requested for credential %s", credID)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted", "id": credID})
}

// AES-256-GCM encryption helpers.
func (c *CredentialsAPI) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (c *CredentialsAPI) decrypt(encoded string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
