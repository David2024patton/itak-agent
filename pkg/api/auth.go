package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// AuthAPI handles user registration, login, token validation, and role management.
type AuthAPI struct {
	mu     sync.RWMutex
	users  map[string]*AuthUser // email -> user
	tokens map[string]string    // token -> email
}

// AuthUser stores account info including role and enabled features.
type AuthUser struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	Password    string   `json:"-"`
	Role        string   `json:"role"` // "superadmin", "admin", "user"
	Tier        string   `json:"tier"` // "pro", "agency"
	Features    []string `json:"features"`
	Disabled    bool     `json:"disabled"`
	CreatedAt   string   `json:"created_at"`
	LastLogin   string   `json:"last_login,omitempty"`
}

// AllFeatures is the complete list of toggleable modules.
var AllFeatures = []string{
	"chat", "agents", "knowledge", "tasks", "automations",
	"personas", "connectors", "credentials", "browser",
	"voice", "plugins", "agency", "billing", "whitelabel",
	"analytics", "crm", "social_planner", "sites_funnels",
	"reputation", "calendar", "email_marketing",
}

// Shared singleton so admin_api.go can access users.
var sharedAuth *AuthAPI

func RegisterAuthRoutes(mux *http.ServeMux) {
	a := &AuthAPI{
		users:  make(map[string]*AuthUser),
		tokens: make(map[string]string),
	}
	sharedAuth = a

	// Seed super admin.
	a.users["david@itak.live"] = &AuthUser{
		ID:          "superadmin-001",
		Email:       "david@itak.live",
		DisplayName: "David",
		Password:    "Wildcats@4113",
		Role:        "superadmin",
		Tier:        "agency",
		Features:    append([]string{}, AllFeatures...),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	debug.Info("auth", "Super admin seeded: david@itak.live")

	mux.HandleFunc("/v1/auth/register", a.handleRegister)
	mux.HandleFunc("/v1/auth/login", a.handleLogin)
	mux.HandleFunc("/v1/auth/me", a.handleMe)
	mux.HandleFunc("/v1/auth/logout", a.handleLogout)
	debug.Info("api", "Auth API registered (/v1/auth/*)")
}

func (a *AuthAPI) handleRegister(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions { w.WriteHeader(200); return }
	if r.Method != http.MethodPost { w.WriteHeader(405); return }

	var req struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Password    string `json:"password"`
		Tier        string `json:"tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" || req.DisplayName == "" {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "all fields required"})
		return
	}
	if len(req.Password) < 8 {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "password must be at least 8 characters"})
		return
	}
	if req.Tier == "" { req.Tier = "pro" }

	// Default features for new users based on tier.
	features := []string{"chat", "agents", "knowledge", "tasks", "automations", "personas", "voice", "plugins"}
	if req.Tier == "agency" {
		features = append(features, "agency", "billing", "whitelabel", "analytics", "crm",
			"social_planner", "sites_funnels", "reputation", "connectors", "credentials",
			"browser", "calendar", "email_marketing")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.users[req.Email]; exists {
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]string{"error": "email already registered"})
		return
	}

	user := &AuthUser{
		ID:          generateID(),
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Password:    req.Password,
		Role:        "user",
		Tier:        req.Tier,
		Features:    features,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	a.users[req.Email] = user

	token := generateToken()
	a.tokens[token] = req.Email

	debug.Info("auth", "New user registered: %s (%s tier)", req.Email, req.Tier)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token": token,
		"user":  user,
	})
}

func (a *AuthAPI) handleLogin(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions { w.WriteHeader(200); return }
	if r.Method != http.MethodPost { w.WriteHeader(405); return }

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	a.mu.RLock()
	user, exists := a.users[req.Email]
	a.mu.RUnlock()

	if !exists || user.Password != req.Password {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid email or password"})
		return
	}
	if user.Disabled {
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]string{"error": "account disabled"})
		return
	}

	a.mu.Lock()
	token := generateToken()
	a.tokens[token] = req.Email
	user.LastLogin = time.Now().UTC().Format(time.RFC3339)
	a.mu.Unlock()

	debug.Info("auth", "User logged in: %s (role: %s)", req.Email, user.Role)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token": token,
		"user":  user,
	})
}

func (a *AuthAPI) handleMe(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	token := extractToken(r)
	if token == "" {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "not authenticated"})
		return
	}
	a.mu.RLock()
	email, ok := a.tokens[token]
	if !ok {
		a.mu.RUnlock()
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid token"})
		return
	}
	user := a.users[email]
	a.mu.RUnlock()
	json.NewEncoder(w).Encode(map[string]interface{}{"user": user})
}

func (a *AuthAPI) handleLogout(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	token := extractToken(r)
	if token != "" {
		a.mu.Lock()
		delete(a.tokens, token)
		a.mu.Unlock()
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
}

// getUserByToken returns the authenticated user or nil.
func (a *AuthAPI) getUserByToken(r *http.Request) *AuthUser {
	token := extractToken(r)
	if token == "" { return nil }
	a.mu.RLock()
	defer a.mu.RUnlock()
	email, ok := a.tokens[token]
	if !ok { return nil }
	return a.users[email]
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return r.URL.Query().Get("token")
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
