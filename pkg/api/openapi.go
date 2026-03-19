package api

import (
	"encoding/json"
	"net/http"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// OpenAPISpec serves a self-documenting API spec.
//
// What: Auto-generated OpenAPI 3.0 spec for all iTaK Agent REST endpoints.
// Why:  Developer-friendly API docs and enables third-party integrations.
// How:  Serves a static spec at /v1/openapi.json.
func RegisterOpenAPIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/openapi.json", handleOpenAPISpec)
	mux.HandleFunc("/v1/docs", handleSwaggerUI)
	debug.Info("api", "OpenAPI docs registered (/v1/openapi.json, /v1/docs)")
}

func handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(openAPISpec())
}

func handleSwaggerUI(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<!DOCTYPE html>
<html><head>
<title>iTaK Agent API Docs</title>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head><body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>SwaggerUIBundle({url:'/v1/openapi.json',dom_id:'#swagger-ui',deepLinking:true,presets:[SwaggerUIBundle.presets.apis,SwaggerUIBundle.SwaggerUIStandalonePreset],layout:'BaseLayout'})</script>
</body></html>`))
}

func openAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"openapi": "3.0.3",
		"info": map[string]interface{}{
			"title":       "iTaK Agent API",
			"version":     "1.0.0",
			"description": "REST API for iTaK Agent - AI-powered automation engine",
		},
		"servers": []map[string]string{
			{"url": "http://localhost:42800", "description": "Local"},
		},
		"paths": map[string]interface{}{
			"/v1/status": pathDef("GET", "System", "Get system status", "System health and version info"),
			"/v1/chat": pathDef("POST", "Chat", "Send chat message", "Send a message to the orchestrator or a specific agent"),
			"/v1/agents": pathDef("GET", "Agents", "List agents", "Get all registered agents and their status"),
			"/v1/agents/launch": pathDef("POST", "Agents", "Launch agent", "Start a new agent with specified config"),
			"/v1/agents/{id}/stop": pathDef("POST", "Agents", "Stop agent", "Terminate a running agent"),
			"/v1/tasks": pathDef("GET", "Tasks", "List tasks", "Get all tasks from the task board"),
			"/v1/tasks/create": pathDef("POST", "Tasks", "Create task", "Create a new task"),
			"/v1/models/providers": pathDef("GET", "Models", "List providers", "Get the full LLM provider catalog"),
			"/v1/models/list": pathDef("POST", "Models", "List models", "Fetch available models from a provider"),
			"/v1/models/global": pathDef("GET", "Models", "Get config", "Get the global model configuration"),
			"/v1/plugins": pathDef("GET", "Plugins", "List plugins", "Get all discovered plugins"),
			"/v1/plugins/toggle": pathDef("POST", "Plugins", "Toggle plugin", "Enable or disable a plugin"),
			"/v1/graph/nodes": pathDef("GET", "Database", "List nodes", "Get graph nodes with optional filters"),
			"/v1/embed/status": pathDef("GET", "Embeddings", "Get status", "Get embedding model status"),
			"/v1/knowledge/search": pathDef("POST", "Knowledge", "Search", "Semantic search across knowledge base"),
			"/v1/personas": pathDef("GET", "Personas", "List personas", "Get all configured personas"),
			"/v1/automations": pathDef("GET", "Automations", "List jobs", "Get all automation jobs"),
			"/v1/credentials": pathDef("GET", "Credentials", "List credentials", "Get stored API credentials"),
			"/v1/connectors": pathDef("GET", "Connectors", "List connectors", "Get all available connectors"),
			"/v1/openapi.json": pathDef("GET", "System", "OpenAPI spec", "This specification"),
		},
	}
}

func pathDef(method, tag, summary, desc string) map[string]interface{} {
	return map[string]interface{}{
		method: map[string]interface{}{
			"tags":        []string{tag},
			"summary":     summary,
			"description": desc,
			"responses": map[string]interface{}{
				"200": map[string]string{"description": "Success"},
			},
		},
	}
}
