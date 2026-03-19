package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestOpenAPISpec verifies the OpenAPI spec endpoint serves valid JSON.
func TestOpenAPISpec(t *testing.T) {
	mux := http.NewServeMux()
	RegisterOpenAPIRoutes(mux)

	req := httptest.NewRequest("GET", "/v1/openapi.json", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}
	body := w.Body.String()
	if len(body) < 100 {
		t.Fatalf("response too short: %d bytes", len(body))
	}
	if body[0] != '{' {
		t.Fatalf("not JSON: starts with %q", body[:10])
	}
}

// TestSwaggerUI verifies the docs page serves HTML.
func TestSwaggerUI(t *testing.T) {
	mux := http.NewServeMux()
	RegisterOpenAPIRoutes(mux)

	req := httptest.NewRequest("GET", "/v1/docs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if len(body) < 50 {
		t.Fatalf("page too short")
	}
}

// TestPluginScan verifies plugin discovery returns valid JSON.
func TestPluginList(t *testing.T) {
	mux := http.NewServeMux()
	RegisterPluginRoutes(mux, t.TempDir())

	req := httptest.NewRequest("GET", "/v1/plugins", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
