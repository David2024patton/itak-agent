package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ────────────────────────────────────────────────────────────────────
// Auth System: JWT + bcrypt, file-backed user store
// ────────────────────────────────────────────────────────────────────

// User tiers control feature gating.
const (
	TierStarter = "starter" // Free: Chat + Monitor
	TierPro     = "pro"     // $29/mo: + Agents, Tasks, Automations
	TierAgency  = "agency"  // $99/mo: + Full Agency module
)

// User represents a registered user.
type User struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	DisplayName  string `json:"display_name"`
	PasswordHash string `json:"-"` // never serialized to JSON
	Tier         string `json:"tier"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// AuthStore manages user accounts with a JSON file backend.
// Simple, zero-dependency, works well for single-instance deployments.
type AuthStore struct {
	mu       sync.RWMutex
	users    map[string]*User // keyed by email (lowercase)
	filePath string
	jwtKey   []byte
}

// NewAuthStore creates or loads the user store.
func NewAuthStore(dataDir string) (*AuthStore, error) {
	fp := filepath.Join(dataDir, "users.json")
	store := &AuthStore{
		users:    make(map[string]*User),
		filePath: fp,
	}

	// JWT signing key: use ITAK_JWT_SECRET env or generate a persistent one.
	if secret := os.Getenv("ITAK_JWT_SECRET"); secret != "" {
		store.jwtKey = []byte(secret)
	} else {
		keyFile := filepath.Join(dataDir, ".jwt_key")
		if data, err := os.ReadFile(keyFile); err == nil && len(data) == 32 {
			store.jwtKey = data
		} else {
			key := make([]byte, 32)
			if _, err := rand.Read(key); err != nil {
				return nil, fmt.Errorf("failed to generate JWT key: %w", err)
			}
			os.MkdirAll(dataDir, 0o755)
			os.WriteFile(keyFile, key, 0o600)
			store.jwtKey = key
		}
	}

	// Load existing users.
	if data, err := os.ReadFile(fp); err == nil {
		var users []*User
		if err := json.Unmarshal(data, &users); err == nil {
			for _, u := range users {
				store.users[strings.ToLower(u.Email)] = u
			}
		}
	}

	return store, nil
}

// save persists users to disk.
func (s *AuthStore) save() error {
	users := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u)
	}
	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(s.filePath), 0o755)
	return os.WriteFile(s.filePath, data, 0o644)
}

// Register creates a new user account.
func (s *AuthStore) Register(email, password, displayName string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return nil, fmt.Errorf("invalid email address")
	}
	if len(password) < 6 {
		return nil, fmt.Errorf("password must be at least 6 characters")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[email]; exists {
		return nil, fmt.Errorf("account already exists")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Generate a short unique ID.
	idBytes := make([]byte, 12)
	rand.Read(idBytes)
	id := base64.RawURLEncoding.EncodeToString(idBytes)

	now := time.Now().UTC().Format(time.RFC3339)
	if displayName == "" {
		displayName = strings.Split(email, "@")[0]
	}

	user := &User{
		ID:           id,
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: string(hash),
		Tier:         TierStarter,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	s.users[email] = user
	if err := s.save(); err != nil {
		delete(s.users, email)
		return nil, fmt.Errorf("failed to save user: %w", err)
	}

	return user, nil
}

// Authenticate verifies credentials and returns the user.
func (s *AuthStore) Authenticate(email, password string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	s.mu.RLock()
	user, exists := s.users[email]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	return user, nil
}

// GetUser returns a user by email.
func (s *AuthStore) GetUser(email string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[strings.ToLower(email)]
	return u, ok
}

// UpdateTier changes a user's subscription tier.
func (s *AuthStore) UpdateTier(email, tier string) error {
	email = strings.ToLower(email)
	s.mu.Lock()
	defer s.mu.Unlock()

	user, exists := s.users[email]
	if !exists {
		return fmt.Errorf("user not found")
	}

	switch tier {
	case TierStarter, TierPro, TierAgency:
		// valid
	default:
		return fmt.Errorf("invalid tier: %s", tier)
	}

	user.Tier = tier
	user.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.save()
}

// ── JWT (HMAC-SHA256, zero external deps) ─────────────────────────

type jwtClaims struct {
	UserID string `json:"sub"`
	Email  string `json:"email"`
	Tier   string `json:"tier"`
	Exp    int64  `json:"exp"`
}

func (s *AuthStore) SignJWT(user *User) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	claims := jwtClaims{
		UserID: user.ID,
		Email:  user.Email,
		Tier:   user.Tier,
		Exp:    time.Now().Add(7 * 24 * time.Hour).Unix(), // 7-day expiry
	}
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)

	sigInput := header + "." + payload
	mac := hmac.New(sha256.New, s.jwtKey)
	mac.Write([]byte(sigInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return sigInput + "." + sig, nil
}

func (s *AuthStore) VerifyJWT(tokenStr string) (*jwtClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed token")
	}

	sigInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, s.jwtKey)
	mac.Write([]byte(sigInput))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, fmt.Errorf("invalid signature")
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid payload")
	}

	var claims jwtClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("invalid claims")
	}

	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

// ── HTTP Handlers ─────────────────────────────────────────────────

// RegisterAuthRoutes mounts auth endpoints on the given mux.
func RegisterAuthRoutes(mux *http.ServeMux, store *AuthStore) {
	// POST /v1/auth/register
	mux.HandleFunc("/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Email       string `json:"email"`
			Password    string `json:"password"`
			DisplayName string `json:"display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		user, err := store.Register(req.Email, req.Password, req.DisplayName)
		if err != nil {
			jsonError(w, err.Error(), http.StatusConflict)
			return
		}

		token, err := store.SignJWT(user)
		if err != nil {
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Set httpOnly cookie for browser auth.
		http.SetCookie(w, &http.Cookie{
			Name:     "itak_token",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   7 * 24 * 60 * 60, // 7 days
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token": token,
			"user":  user,
		})
	})

	// POST /v1/auth/login
	mux.HandleFunc("/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		user, err := store.Authenticate(req.Email, req.Password)
		if err != nil {
			jsonError(w, err.Error(), http.StatusUnauthorized)
			return
		}

		token, err := store.SignJWT(user)
		if err != nil {
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "itak_token",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   7 * 24 * 60 * 60,
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token": token,
			"user":  user,
		})
	})

	// GET /v1/auth/me
	mux.HandleFunc("/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		claims, err := extractClaims(r, store)
		if err != nil {
			jsonError(w, "not authenticated", http.StatusUnauthorized)
			return
		}

		user, ok := store.GetUser(claims.Email)
		if !ok {
			jsonError(w, "user not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	})

	// POST /v1/auth/logout
	mux.HandleFunc("/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     "itak_token",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
	})

	// POST /v1/auth/upgrade
	mux.HandleFunc("/v1/auth/upgrade", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		claims, err := extractClaims(r, store)
		if err != nil {
			jsonError(w, "not authenticated", http.StatusUnauthorized)
			return
		}
		var req struct {
			Tier string `json:"tier"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err := store.UpdateTier(claims.Email, req.Tier); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Re-sign JWT with new tier.
		user, _ := store.GetUser(claims.Email)
		token, _ := store.SignJWT(user)
		http.SetCookie(w, &http.Cookie{
			Name:     "itak_token",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   7 * 24 * 60 * 60,
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token": token,
			"user":  user,
		})
	})
}

// ── Helpers ───────────────────────────────────────────────────────

// extractClaims pulls JWT claims from Authorization header or cookie.
func extractClaims(r *http.Request, store *AuthStore) (*jwtClaims, error) {
	// Try Authorization: Bearer <token>
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return store.VerifyJWT(strings.TrimPrefix(auth, "Bearer "))
	}
	// Try cookie
	if cookie, err := r.Cookie("itak_token"); err == nil {
		return store.VerifyJWT(cookie.Value)
	}
	return nil, fmt.Errorf("no token")
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
