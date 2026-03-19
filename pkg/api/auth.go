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

// AuthAPI handles user registration, login, and token validation for SaaS.
//
// What: Simple token-based auth system for the SaaS landing/dashboard flow.
// Why:  Premium users need accounts to access billing, usage, white-label features.
// How:  In-memory user store with bcrypt-free password checking (for demo), JWT-like tokens.
type AuthAPI struct {
	mu    sync.RWMutex
	users map[string]*AuthUser  // email -> user
	tokens map[string]string    // token -> email
}

type AuthUser struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"-"`
	Tier        string `json:"tier"`
	CreatedAt   string `json:"created_at"`
}

func RegisterAuthRoutes(mux *http.ServeMux) {
	a := &AuthAPI{
		users:  make(map[string]*AuthUser),
		tokens: make(map[string]string),
	}
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

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.users[req.Email]; exists {
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]string{"error": "email already registered"})
		return
	}

	id := generateID()
	user := &AuthUser{
		ID:          id,
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Password:    req.Password,
		Tier:        req.Tier,
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

	a.mu.Lock()
	token := generateToken()
	a.tokens[token] = req.Email
	a.mu.Unlock()

	debug.Info("auth", "User logged in: %s", req.Email)
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
