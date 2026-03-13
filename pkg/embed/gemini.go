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

// GeminiEmbedder calls the Gemini Embedding API (v1beta).
//
// What: Cloud embedding provider using Google's Gemini Embedding 2.
// Why:  Best-in-class multimodal embeddings (text, image, video, audio).
//       8k token input, flexible 128-3072 output dimensions.
// How:  REST calls to generativelanguage.googleapis.com/v1beta.
type GeminiEmbedder struct {
	apiKey     string
	apiBase    string
	model      string
	dimensions int
	client     *http.Client
}

// NewGeminiEmbedder creates a Gemini embedding provider.
func NewGeminiEmbedder(cfg Config) (*GeminiEmbedder, error) {
	cfg.Defaults()

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("gemini embedding requires api_key (set GEMINI_API_KEY env var)")
	}

	return &GeminiEmbedder{
		apiKey:     cfg.APIKey,
		apiBase:    cfg.APIBase,
		model:      cfg.Model,
		dimensions: cfg.Dimensions,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (g *GeminiEmbedder) Name() string    { return "gemini/" + g.model }
func (g *GeminiEmbedder) Dimensions() int { return g.dimensions }

// Embed generates a single embedding vector.
func (g *GeminiEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := g.BatchEmbed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("gemini returned empty embedding")
	}
	return results[0], nil
}

// BatchEmbed generates embeddings for multiple texts in one API call.
func (g *GeminiEmbedder) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	// Build the request body for the embedContent endpoint.
	// Gemini supports batch via the batchEmbedContents endpoint.
	url := fmt.Sprintf("%s/v1beta/models/%s:batchEmbedContents?key=%s",
		g.apiBase, g.model, g.apiKey)

	// Build requests array.
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Parts []part `json:"parts"`
	}
	type embedReq struct {
		Model                string  `json:"model"`
		Content              content `json:"content"`
		TaskType             string  `json:"taskType,omitempty"`
		OutputDimensionality int     `json:"outputDimensionality,omitempty"`
	}
	type batchReq struct {
		Requests []embedReq `json:"requests"`
	}

	reqs := make([]embedReq, len(texts))
	for i, text := range texts {
		reqs[i] = embedReq{
			Model: "models/" + g.model,
			Content: content{
				Parts: []part{{Text: text}},
			},
			TaskType:             "RETRIEVAL_DOCUMENT",
			OutputDimensionality: g.dimensions,
		}
	}

	body, err := json.Marshal(batchReq{Requests: reqs})
	if err != nil {
		return nil, fmt.Errorf("marshal batch request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini API call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse response.
	type embeddingValues struct {
		Values []float32 `json:"values"`
	}
	type batchResp struct {
		Embeddings []embeddingValues `json:"embeddings"`
	}

	var result batchResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode gemini response: %w", err)
	}

	vectors := make([][]float32, len(result.Embeddings))
	for i, emb := range result.Embeddings {
		vectors[i] = emb.Values
	}

	return vectors, nil
}
