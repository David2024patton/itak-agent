package api

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// AdminAPI provides super admin endpoints for user and platform management.
//
// What: CRUD for users, role promotion, feature toggling, site-wide stats.
// Why:  Super admin needs full control over the SaaS platform.
// How:  All endpoints require superadmin or admin role via token auth.
func RegisterAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/admin/stats", handleAdminStats)
	mux.HandleFunc("/v1/admin/users", handleAdminUsers)
	mux.HandleFunc("/v1/admin/users/add", handleAdminAddUser)
	mux.HandleFunc("/v1/admin/users/remove", handleAdminRemoveUser)
	mux.HandleFunc("/v1/admin/users/promote", handleAdminPromoteUser)
	mux.HandleFunc("/v1/admin/users/toggle", handleAdminToggleUser)
	mux.HandleFunc("/v1/admin/users/features", handleAdminUserFeatures)
	mux.HandleFunc("/v1/admin/features", handleAdminAllFeatures)
	debug.Info("api", "Admin API registered (/v1/admin/*)")
}

// requireAdmin checks that the request has a superadmin or admin token.
func requireAdmin(w http.ResponseWriter, r *http.Request) *AuthUser {
	if sharedAuth == nil {
		http.Error(w, `{"error":"auth not initialized"}`, 403)
		return nil
	}
	user := sharedAuth.getUserByToken(r)
	if user == nil {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "not authenticated"})
		return nil
	}
	if user.Role != "superadmin" && user.Role != "admin" {
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]string{"error": "admin access required"})
		return nil
	}
	return user
}

// GET /v1/admin/stats - Platform-wide stats.
func handleAdminStats(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions { w.WriteHeader(200); return }
	if requireAdmin(w, r) == nil { return }

	sharedAuth.mu.RLock()
	totalUsers := len(sharedAuth.users)
	var admins, proUsers, agencyUsers, disabled, activeToday int
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for _, u := range sharedAuth.users {
		switch u.Role {
		case "superadmin", "admin":
			admins++
		}
		switch u.Tier {
		case "pro":
			proUsers++
		case "agency":
			agencyUsers++
		}
		if u.Disabled { disabled++ }
		if u.LastLogin != "" {
			if t, err := time.Parse(time.RFC3339, u.LastLogin); err == nil && t.After(today) {
				activeToday++
			}
		}
	}
	activeSessions := len(sharedAuth.tokens)
	sharedAuth.mu.RUnlock()

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_users":     totalUsers,
		"admins":          admins,
		"pro_users":       proUsers,
		"agency_users":    agencyUsers,
		"disabled_users":  disabled,
		"active_today":    activeToday,
		"active_sessions": activeSessions,
		"memory_mb":       int(mem.Alloc / 1024 / 1024),
		"goroutines":      runtime.NumGoroutine(),
		"uptime":          time.Now().UTC().Format(time.RFC3339),
	})
}

// GET /v1/admin/users - List all users.
func handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions { w.WriteHeader(200); return }
	if requireAdmin(w, r) == nil { return }

	sharedAuth.mu.RLock()
	users := make([]*AuthUser, 0, len(sharedAuth.users))
	for _, u := range sharedAuth.users {
		users = append(users, u)
	}
	sharedAuth.mu.RUnlock()

	json.NewEncoder(w).Encode(map[string]interface{}{"users": users})
}

// POST /v1/admin/users/add - Create a new user.
func handleAdminAddUser(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions { w.WriteHeader(200); return }
	if r.Method != http.MethodPost { w.WriteHeader(405); return }
	if requireAdmin(w, r) == nil { return }

	var req struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
		Role        string `json:"role"`
		Tier        string `json:"tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Email == "" || req.Password == "" || req.DisplayName == "" {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "email, display_name, and password required"})
		return
	}
	if req.Role == "" { req.Role = "user" }
	if req.Tier == "" { req.Tier = "pro" }

	features := []string{"chat", "agents", "knowledge", "tasks", "automations", "personas", "voice", "plugins"}
	if req.Tier == "agency" {
		features = append(features, "agency", "billing", "whitelabel", "analytics", "crm",
			"social_planner", "sites_funnels", "reputation", "connectors", "credentials", "browser")
	}

	sharedAuth.mu.Lock()
	defer sharedAuth.mu.Unlock()

	if _, exists := sharedAuth.users[req.Email]; exists {
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]string{"error": "email already exists"})
		return
	}

	user := &AuthUser{
		ID:          generateID(),
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Password:    req.Password,
		Role:        req.Role,
		Tier:        req.Tier,
		Features:    features,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	sharedAuth.users[req.Email] = user
	debug.Info("admin", "Created user: %s (role: %s, tier: %s)", req.Email, req.Role, req.Tier)
	json.NewEncoder(w).Encode(map[string]interface{}{"user": user, "status": "created"})
}

// POST /v1/admin/users/remove - Delete a user.
func handleAdminRemoveUser(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions { w.WriteHeader(200); return }
	if r.Method != http.MethodPost { w.WriteHeader(405); return }
	caller := requireAdmin(w, r)
	if caller == nil { return }

	var req struct{ Email string `json:"email"` }
	json.NewDecoder(r.Body).Decode(&req)

	if req.Email == caller.Email {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "cannot remove yourself"})
		return
	}

	sharedAuth.mu.Lock()
	defer sharedAuth.mu.Unlock()

	target, exists := sharedAuth.users[req.Email]
	if !exists {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "user not found"})
		return
	}
	if target.Role == "superadmin" && caller.Role != "superadmin" {
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]string{"error": "cannot remove superadmin"})
		return
	}

	// Remove all tokens for this user.
	for token, email := range sharedAuth.tokens {
		if email == req.Email {
			delete(sharedAuth.tokens, token)
		}
	}
	delete(sharedAuth.users, req.Email)
	debug.Info("admin", "Removed user: %s (by %s)", req.Email, caller.Email)
	json.NewEncoder(w).Encode(map[string]string{"status": "removed"})
}

// POST /v1/admin/users/promote - Change user role.
func handleAdminPromoteUser(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions { w.WriteHeader(200); return }
	if r.Method != http.MethodPost { w.WriteHeader(405); return }
	caller := requireAdmin(w, r)
	if caller == nil { return }

	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"` // "user", "admin", "superadmin"
		Tier  string `json:"tier"` // "pro", "agency"
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Only superadmin can promote to admin/superadmin.
	if (req.Role == "admin" || req.Role == "superadmin") && caller.Role != "superadmin" {
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]string{"error": "only superadmin can promote to admin"})
		return
	}

	sharedAuth.mu.Lock()
	defer sharedAuth.mu.Unlock()

	user, exists := sharedAuth.users[req.Email]
	if !exists {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "user not found"})
		return
	}

	if req.Role != "" { user.Role = req.Role }
	if req.Tier != "" { user.Tier = req.Tier }

	debug.Info("admin", "Promoted %s to role=%s tier=%s (by %s)", req.Email, user.Role, user.Tier, caller.Email)
	json.NewEncoder(w).Encode(map[string]interface{}{"user": user, "status": "updated"})
}

// POST /v1/admin/users/toggle - Enable/disable a user account.
func handleAdminToggleUser(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions { w.WriteHeader(200); return }
	if r.Method != http.MethodPost { w.WriteHeader(405); return }
	caller := requireAdmin(w, r)
	if caller == nil { return }

	var req struct {
		Email    string `json:"email"`
		Disabled bool   `json:"disabled"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	sharedAuth.mu.Lock()
	defer sharedAuth.mu.Unlock()

	user, exists := sharedAuth.users[req.Email]
	if !exists {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "user not found"})
		return
	}
	user.Disabled = req.Disabled
	json.NewEncoder(w).Encode(map[string]interface{}{"user": user, "status": "toggled"})
}

// POST /v1/admin/users/features - Set features for a user.
func handleAdminUserFeatures(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions { w.WriteHeader(200); return }
	if r.Method != http.MethodPost { w.WriteHeader(405); return }
	if requireAdmin(w, r) == nil { return }

	var req struct {
		Email    string   `json:"email"`
		Features []string `json:"features"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	sharedAuth.mu.Lock()
	defer sharedAuth.mu.Unlock()

	user, exists := sharedAuth.users[req.Email]
	if !exists {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "user not found"})
		return
	}

	user.Features = req.Features
	debug.Info("admin", "Updated features for %s: %v", req.Email, req.Features)
	json.NewEncoder(w).Encode(map[string]interface{}{"user": user, "status": "features_updated"})
}

// GET /v1/admin/features - List all available features.
func handleAdminAllFeatures(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions { w.WriteHeader(200); return }
	if requireAdmin(w, r) == nil { return }
	json.NewEncoder(w).Encode(map[string]interface{}{"features": AllFeatures})
}
