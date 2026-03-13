package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAIEmbedder calls any OpenAI-compatible embedding API with Bearer token auth.
//
// What: Cloud embedding provider for OpenAI, Cohere, Mistral, Together, etc.
// Why:  Users with API keys should be able to use cloud embeddings without
//       downloading local models. Any provider that supports the standard
//       POST /v1/embeddings format works here.
// How:  HTTP POST with Authorization header and standard request body.
type OpenAIEmbedder struct {
	apiBase    string
	apiKey     string
	model      string
	dimensions int
	client     *http.Client
}

// NewOpenAIEmbedder creates an OpenAI-compatible embedding provider.
func NewOpenAIEmbedder(cfg Config) (*OpenAIEmbedder, error) {
	cfg.Defaults()

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openai embedding requires api_key")
	}

	apiBase := cfg.APIBase
	if apiBase == "" || apiBase == "https://generativelanguage.googleapis.com" {
		apiBase = "https://api.openai.com"
	}

	return &OpenAIEmbedder{
		apiBase:    apiBase,
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		dimensions: cfg.Dimensions,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (o *OpenAIEmbedder) Name() string    { return "openai/" + o.model }
func (o *OpenAIEmbedder) Dimensions() int { return o.dimensions }

// Embed generates a single embedding vector.
func (o *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := o.BatchEmbed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 || len(results[0]) == 0 {
		return nil, fmt.Errorf("openai returned empty embedding")
	}
	return results[0], nil
}

// BatchEmbed generates embeddings for multiple texts via POST /v1/embeddings.
func (o *OpenAIEmbedder) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	url := o.apiBase + "/v1/embeddings"

	type embedReq struct {
		Input          []string `json:"input"`
		Model          string   `json:"model"`
		EncodingFormat string   `json:"encoding_format,omitempty"`
	}

	body, err := json.Marshal(embedReq{
		Input:          texts,
		Model:          o.model,
		EncodingFormat: "float",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal openai embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed API call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai embed error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// OpenAI /v1/embeddings response format:
	// {"data": [{"embedding": [0.1, 0.2, ...], "index": 0}], ...}
	type embData struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	}
	type embedResp struct {
		Data []embData `json:"data"`
	}

	var result embedResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode openai embed response: %w", err)
	}

	vectors := make([][]float32, len(result.Data))
	for _, d := range result.Data {
		if d.Index < len(vectors) {
			vectors[d.Index] = d.Embedding
		}
	}

	return vectors, nil
}
