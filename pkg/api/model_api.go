package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/llm"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
)

// ModelAPI provides endpoints for LLM model management, provider listing,
// and global/per-agent model configuration.
//
// What: REST endpoints for model discovery and configuration.
// Why:  Users need to select and configure LLM providers for their agents.
// How:  Uses the llm.ProviderEntry catalog and OpenAIClient.ListModels().
type ModelAPI struct {
	backend memory.GraphBackend
}

// RegisterModelRoutes adds model management endpoints.
func RegisterModelRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	m := &ModelAPI{backend: backend}
	mux.HandleFunc("/v1/models/providers", m.handleProviders)
	mux.HandleFunc("/v1/models/list", m.handleListModels)
	mux.HandleFunc("/v1/models/global", m.handleGlobalConfig)
	debug.Info("api", "Model API registered (/v1/models/*)")
}

// handleProviders returns the full provider catalog.
// GET /v1/models/providers
func (m *ModelAPI) handleProviders(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	type providerOut struct {
		Name       string `json:"name"`
		Slug       string `json:"slug"`
		APIBase    string `json:"api_base"`
		Category   string `json:"category"`
		Compatible bool   `json:"compatible"`
		SignupURL  string `json:"signup_url"`
		Notes      string `json:"notes,omitempty"`
	}

	all := llm.AllProviders()
	out := make([]providerOut, 0, len(all))
	for _, p := range all {
		out = append(out, providerOut{
			Name:       p.Name,
			Slug:       p.Slug,
			APIBase:    p.APIBase,
			Category:   p.Category,
			Compatible: p.Compatible,
			SignupURL:  p.SignupURL,
			Notes:      p.Notes,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"providers": out,
		"count":     len(out),
	})
}

// listModelsRequest is the body for POST /v1/models/list.
type listModelsRequest struct {
	Provider string `json:"provider"`
	APIBase  string `json:"api_base"`
	APIKey   string `json:"api_key"`
}

// handleListModels creates a temp client and lists available models for a provider.
// POST /v1/models/list
func (m *ModelAPI) handleListModels(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "POST only"})
		return
	}

	var req listModelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	// Resolve provider APIBase if not provided.
	apiBase := req.APIBase
	if apiBase == "" && req.Provider != "" {
		if p, ok := llm.GetProvider(req.Provider); ok {
			apiBase = p.APIBase
		}
	}
	if apiBase == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "api_base or provider required"})
		return
	}

	// Create a temporary client and list models.
	client := llm.NewOpenAIClient(llm.ProviderConfig{
		Provider: req.Provider,
		APIBase:  apiBase,
		APIKey:   req.APIKey,
	})

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	models, err := client.ListModels(ctx)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to list models: " + err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"models": models,
		"count":  len(models),
	})
}

// GlobalModelConfig is the global model settings stored in the graph.
type GlobalModelConfig struct {
	ModelType string `json:"model_type"` // "api", "torch", "ollama"
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	APIBase   string `json:"api_base"`
	APIKey    string `json:"api_key"`
	// Torch-specific
	TorchModelPath string `json:"torch_model_path,omitempty"`
	TorchRepoID    string `json:"torch_repo_id,omitempty"`
	// Ollama-specific
	OllamaModel    string `json:"ollama_model,omitempty"`
	OllamaEndpoint string `json:"ollama_endpoint,omitempty"`
}

// handleGlobalConfig gets or sets the global model config.
// GET/PUT /v1/models/global
func (m *ModelAPI) handleGlobalConfig(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	itakBackend, ok := m.backend.(*memory.ITakDBBackend)
	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"config": GlobalModelConfig{ModelType: "api", Provider: "ollama", OllamaEndpoint: "http://localhost:11434"},
		})
		return
	}

	db := itakBackend.DB()

	switch r.Method {
	case http.MethodGet:
		nodes, _ := db.Graph.FindByLabel("GlobalModelConfig")
		if len(nodes) == 0 {
			// Return defaults
			json.NewEncoder(w).Encode(map[string]interface{}{
				"config": GlobalModelConfig{
					ModelType:      "api",
					Provider:       "ollama",
					OllamaModel:    "qwen3:0.6b",
					OllamaEndpoint: "http://localhost:11434",
				},
			})
			return
		}
		cfg := nodeToGlobalConfig(nodes[0].Properties)
		json.NewEncoder(w).Encode(map[string]interface{}{"config": cfg})

	case http.MethodPut:
		var cfg GlobalModelConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
			return
		}

		props := globalConfigToMap(cfg)
		props["updated_at"] = time.Now().Format(time.RFC3339)

		// Upsert: delete existing, create new.
		existing, _ := db.Graph.FindByLabel("GlobalModelConfig")
		for _, n := range existing {
			db.Graph.DeleteNode(n.ID)
		}
		db.CreateNode([]string{"GlobalModelConfig"}, props, nil)
		debug.Info("model", "Global model config updated: type=%s provider=%s model=%s", cfg.ModelType, cfg.Provider, cfg.Model)
		json.NewEncoder(w).Encode(map[string]string{"status": "updated"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func nodeToGlobalConfig(props map[string]interface{}) GlobalModelConfig {
	return GlobalModelConfig{
		ModelType:      pStr(props, "model_type"),
		Provider:       pStr(props, "provider"),
		Model:          pStr(props, "model"),
		APIBase:        pStr(props, "api_base"),
		APIKey:         pStr(props, "api_key"),
		TorchModelPath: pStr(props, "torch_model_path"),
		TorchRepoID:    pStr(props, "torch_repo_id"),
		OllamaModel:    pStr(props, "ollama_model"),
		OllamaEndpoint: pStr(props, "ollama_endpoint"),
	}
}

func globalConfigToMap(cfg GlobalModelConfig) map[string]interface{} {
	return map[string]interface{}{
		"model_type":       cfg.ModelType,
		"provider":         cfg.Provider,
		"model":            cfg.Model,
		"api_base":         cfg.APIBase,
		"api_key":          cfg.APIKey,
		"torch_model_path": cfg.TorchModelPath,
		"torch_repo_id":    cfg.TorchRepoID,
		"ollama_model":     cfg.OllamaModel,
		"ollama_endpoint":  cfg.OllamaEndpoint,
	}
}
