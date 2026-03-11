package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/David2024patton/iTaKAgent/pkg/eventbus"
)

func TestHealthEndpoint(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()

	s := NewServer(nil, bus, 0)
	// Create test handler directly.
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", resp["status"])
	}
	if resp["version"] != "0.2.0" {
		t.Errorf("expected version=0.2.0, got %v", resp["version"])
	}
}

func TestChatEndpointRequiresPOST(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()

	s := NewServer(nil, bus, 0)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat", s.handleChat)

	req := httptest.NewRequest("GET", "/v1/chat", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET, got %d", w.Code)
	}
}

func TestChatEndpointRequiresMessage(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()

	s := NewServer(nil, bus, 0)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat", s.handleChat)

	body := strings.NewReader(`{"message":""}`)
	req := httptest.NewRequest("POST", "/v1/chat", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty message, got %d", w.Code)
	}
}

func TestCORSHeaders(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()

	s := NewServer(nil, bus, 0)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)

	handler := corsMiddleware(mux)
	req := httptest.NewRequest("OPTIONS", "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for OPTIONS, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}
}
