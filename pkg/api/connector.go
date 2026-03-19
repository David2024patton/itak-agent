package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
)

// ────────────────────────────────────────────────────────────────────
// Connector: pluggable third-party service integration layer.
//
// What: A registry of connectors that proxy requests to external APIs
//       (Twilio, Stripe, GHL, Odoo, Vonage, FieldRoutes, etc).
// Why:  Agency pages need real data from third-party services.
//       Users pick which provider to use per category (phone, payments, crm).
// How:  Each connector implements the Connector interface.
//       Frontend calls /v1/connector/{category}/{action} and the
//       active connector for that category handles it.
// ────────────────────────────────────────────────────────────────────

// Connector is the interface every third-party integration must implement.
type Connector interface {
	// Name returns the connector identifier (e.g. "twilio", "stripe").
	Name() string

	// Category returns the service category (e.g. "phone", "payments", "crm").
	Category() string

	// TestConnection verifies that stored credentials are valid.
	TestConnection() error

	// Do executes an action with the given params and returns a JSON-able result.
	// Actions are connector-specific (e.g. "send-sms", "list-invoices").
	Do(action string, params map[string]interface{}) (interface{}, error)

	// Actions returns a list of supported action names for discovery.
	Actions() []string
}

// ConnectorRegistry manages registered connectors and the active connector
// per category.
type ConnectorRegistry struct {
	mu         sync.RWMutex
	connectors map[string][]Connector          // category -> available connectors
	active     map[string]string               // category -> active connector name
	credAPI    *CredentialsAPI                  // for credential lookup
	backend    memory.GraphBackend
}

// NewConnectorRegistry creates a new registry wired to the credentials vault.
func NewConnectorRegistry(backend memory.GraphBackend, credAPI *CredentialsAPI) *ConnectorRegistry {
	return &ConnectorRegistry{
		connectors: make(map[string][]Connector),
		active:     make(map[string]string),
		credAPI:    credAPI,
		backend:    backend,
	}
}

// Register adds a connector to the registry.
func (r *ConnectorRegistry) Register(c Connector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connectors[c.Category()] = append(r.connectors[c.Category()], c)

	// Auto-activate first registered connector per category.
	if _, ok := r.active[c.Category()]; !ok {
		r.active[c.Category()] = c.Name()
	}
	debug.Info("connector", "Registered connector: %s (category: %s)", c.Name(), c.Category())
}

// SetActive sets the active connector for a category.
func (r *ConnectorRegistry) SetActive(category, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.connectors[category] {
		if c.Name() == name {
			r.active[category] = name
			debug.Info("connector", "Active connector for %s set to %s", category, name)
			return nil
		}
	}
	return fmt.Errorf("connector %q not found in category %q", name, category)
}

// GetActive returns the active connector for a category (or nil).
func (r *ConnectorRegistry) GetActive(category string) Connector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	activeName, ok := r.active[category]
	if !ok {
		return nil
	}
	for _, c := range r.connectors[category] {
		if c.Name() == activeName {
			return c
		}
	}
	return nil
}

// ListAll returns all registered connectors grouped by category.
func (r *ConnectorRegistry) ListAll() map[string][]ConnectorInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]ConnectorInfo)
	for cat, conns := range r.connectors {
		activeName := r.active[cat]
		for _, c := range conns {
			result[cat] = append(result[cat], ConnectorInfo{
				Name:     c.Name(),
				Category: c.Category(),
				Active:   c.Name() == activeName,
				Actions:  c.Actions(),
			})
		}
	}
	return result
}

// ConnectorInfo is the JSON-safe metadata for a connector.
type ConnectorInfo struct {
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Active   bool     `json:"active"`
	Actions  []string `json:"actions"`
}

// ── Credential Lookup ─────────────────────────────────────────────

// ConnectorCreds holds decrypted credentials for a connector.
type ConnectorCreds struct {
	Fields map[string]string
}

// GetConnectorCreds looks up credentials by provider name from the vault.
// Returns the decrypted field values or an error if not found.
func (r *ConnectorRegistry) GetConnectorCreds(providerName string) (*ConnectorCreds, error) {
	if r.credAPI == nil || r.backend == nil {
		return nil, fmt.Errorf("credentials API not available")
	}

	itakBackend, ok := r.backend.(*memory.ITakDBBackend)
	if !ok {
		return nil, fmt.Errorf("requires iTaK Database backend")
	}

	db := itakBackend.DB()
	nodes, _ := db.Graph.FindByLabel("Credential")

	for _, n := range nodes {
		provider := pStr(n.Properties, "provider")
		name := pStr(n.Properties, "name")
		if !strings.EqualFold(provider, providerName) && !strings.EqualFold(name, providerName) {
			continue
		}

		encrypted := pStr(n.Properties, "encrypted_value")
		if encrypted == "" {
			continue
		}

		decrypted, err := r.credAPI.decrypt(encrypted)
		if err != nil {
			return nil, fmt.Errorf("decryption failed for %s: %w", providerName, err)
		}

		// Try to parse as JSON fields object (e.g. {"account_sid":"AC...", "auth_token":"..."}).
		fields := make(map[string]string)
		if err := json.Unmarshal([]byte(decrypted), &fields); err != nil {
			// Not JSON, treat the whole value as a single "api_key" field.
			fields["api_key"] = decrypted
		}
		return &ConnectorCreds{Fields: fields}, nil
	}

	return nil, fmt.Errorf("no credentials found for provider %q", providerName)
}

// ── HTTP Proxy Client ─────────────────────────────────────────────

// ConnectorHTTPClient is a thin wrapper around http.Client with auth injection.
type ConnectorHTTPClient struct {
	client  *http.Client
	baseURL string
	headers map[string]string
}

// NewConnectorHTTPClient creates an HTTP client for a specific service.
func NewConnectorHTTPClient(baseURL string, headers map[string]string) *ConnectorHTTPClient {
	return &ConnectorHTTPClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: strings.TrimRight(baseURL, "/"),
		headers: headers,
	}
}

// DoRequest performs an HTTP request and returns the decoded JSON response.
func (c *ConnectorHTTPClient) DoRequest(method, path string, body interface{}) (map[string]interface{}, error) {
	url := c.baseURL + "/" + strings.TrimLeft(path, "/")

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(raw))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		// Some APIs return arrays at top level.
		var arr []interface{}
		if err2 := json.Unmarshal(raw, &arr); err2 == nil {
			return map[string]interface{}{"items": arr}, nil
		}
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result, nil
}

// DoFormRequest performs a form-encoded POST (used by Twilio).
func (c *ConnectorHTTPClient) DoFormRequest(path string, formData map[string]string) (map[string]interface{}, error) {
	url := c.baseURL + "/" + strings.TrimLeft(path, "/")

	form := make([]string, 0, len(formData))
	for k, v := range formData {
		form = append(form, fmt.Sprintf("%s=%s", k, v))
	}
	body := strings.Join(form, "&")

	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(raw))
	}

	var result map[string]interface{}
	json.Unmarshal(raw, &result)
	return result, nil
}

// ── Route Registration ────────────────────────────────────────────

// RegisterConnectorRoutes adds the universal connector proxy endpoints.
func RegisterConnectorRoutes(mux *http.ServeMux, registry *ConnectorRegistry) {
	if registry == nil {
		debug.Warn("connector", "Registry is nil, connector API disabled")
		return
	}

	// List all connectors and their status.
	mux.HandleFunc("/v1/connectors", func(w http.ResponseWriter, r *http.Request) {
		corsHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"connectors": registry.ListAll(),
		})
	})

	// Set active connector: POST /v1/connectors/activate
	mux.HandleFunc("/v1/connectors/activate", func(w http.ResponseWriter, r *http.Request) {
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
			Category string `json:"category"`
			Name     string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if err := registry.SetActive(req.Category, req.Name); err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "activated"})
	})

	// Test connection: POST /v1/connectors/test
	mux.HandleFunc("/v1/connectors/test", func(w http.ResponseWriter, r *http.Request) {
		corsHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		var req struct {
			Category string `json:"category"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		c := registry.GetActive(req.Category)
		if c == nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "no active connector for " + req.Category})
			return
		}
		if err := c.TestConnection(); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "error", "error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "connected"})
	})

	// Universal action proxy: POST /v1/connector/{category}/{action}
	mux.HandleFunc("/v1/connector/", func(w http.ResponseWriter, r *http.Request) {
		corsHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Parse: /v1/connector/{category}/{action}
		path := strings.TrimPrefix(r.URL.Path, "/v1/connector/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) < 2 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "expected /v1/connector/{category}/{action}",
			})
			return
		}

		category := parts[0]
		action := parts[1]

		c := registry.GetActive(category)
		if c == nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{
				"error":    fmt.Sprintf("no active connector for category %q", category),
				"hint":     "Store credentials and activate a connector via /v1/connectors/activate",
				"category": category,
			})
			return
		}

		// Parse request params.
		var params map[string]interface{}
		if r.Method == http.MethodPost {
			json.NewDecoder(r.Body).Decode(&params)
		}
		if params == nil {
			params = make(map[string]interface{})
		}

		// Pass query params too.
		for k, v := range r.URL.Query() {
			if len(v) == 1 {
				params[k] = v[0]
			} else {
				params[k] = v
			}
		}

		debug.Info("connector", "%s/%s -> %s.Do(%s)", category, action, c.Name(), action)

		result, err := c.Do(action, params)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]string{
				"error":     err.Error(),
				"connector": c.Name(),
				"action":    action,
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data":      result,
			"connector": c.Name(),
			"action":    action,
		})
	})

	debug.Info("connector", "Connector API registered (/v1/connector/, /v1/connectors)")
}
