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

// LocalEmbedder calls Ollama or iTaK Torch for local embeddings.
//
// What: Local embedding provider for air-gapped or cost-sensitive deployments.
// Why:  Not everyone wants to pay for cloud embeddings. Ollama and Torch both
//       expose OpenAI-compatible /api/embed endpoints that return float32 vectors.
// How:  HTTP POST to the local server with the model name and input text.
type LocalEmbedder struct {
	endpoint   string
	model      string
	dimensions int
	client     *http.Client
}

// NewLocalEmbedder creates a local embedding provider.
func NewLocalEmbedder(cfg Config) (*LocalEmbedder, error) {
	cfg.Defaults()

	endpoint := cfg.LocalEndpoint
	model := cfg.LocalModel
	if cfg.Provider == "local" || cfg.Provider == "ollama" || cfg.Provider == "torch" {
		if cfg.Model != "" && cfg.Model != "gemini-embedding-2-preview" {
			model = cfg.Model
		}
	}

	return &LocalEmbedder{
		endpoint:   endpoint,
		model:      model,
		dimensions: cfg.Dimensions,
		client: &http.Client{
			Timeout: 60 * time.Second, // Local models can be slow on CPU
		},
	}, nil
}

func (l *LocalEmbedder) Name() string    { return "local/" + l.model }
func (l *LocalEmbedder) Dimensions() int { return l.dimensions }

// Embed generates a single embedding vector via Ollama API.
func (l *LocalEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := l.BatchEmbed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 || len(results[0]) == 0 {
		return nil, fmt.Errorf("local model returned empty embedding")
	}
	return results[0], nil
}

// BatchEmbed generates embeddings for multiple texts.
// Ollama's /api/embed supports a single "input" field that can be string or []string.
func (l *LocalEmbedder) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	url := l.endpoint + "/api/embed"

	type embedReq struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}

	body, err := json.Marshal(embedReq{
		Model: l.model,
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal local embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("local embed API call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("local embed error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Ollama /api/embed response format:
	// {"model": "...", "embeddings": [[0.1, 0.2, ...], [0.3, 0.4, ...]]}
	type embedResp struct {
		Embeddings [][]float32 `json:"embeddings"`
	}

	var result embedResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode local embed response: %w", err)
	}

	return result.Embeddings, nil
}
