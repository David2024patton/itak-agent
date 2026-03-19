package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// VisionClient processes images using a multimodal LLM via Ollama's native API.
// Ollama's /api/chat endpoint supports images as base64-encoded data alongside text,
// which is how models like qwen3-vl, gemma3, and llava "see" images.
type VisionClient struct {
	ollamaBase string // e.g. "http://localhost:11434"
	model      string // e.g. "qwen3-vl:2b"
	httpClient *http.Client
}

// NewVisionClient creates a client for processing images via Ollama vision models.
func NewVisionClient(ollamaBase, model string) *VisionClient {
	if ollamaBase == "" {
		ollamaBase = "http://localhost:11434"
	}
	if model == "" {
		model = "qwen3-vl:2b"
	}
	return &VisionClient{
		ollamaBase: strings.TrimSuffix(ollamaBase, "/"),
		model:      model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// ollamaChatMessage is a message for Ollama's native /api/chat endpoint.
type ollamaChatMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"` // base64-encoded image data (no data URL prefix)
}

// ollamaChatRequest is the request body for /api/chat.
type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

// ollamaChatResponse is the response from /api/chat (non-streaming).
type ollamaChatResponse struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

// DescribeImage sends a base64 image to the vision model and returns a text description.
// The dataURL should be a full data URL (data:image/png;base64,...) or raw base64.
// The prompt controls what the model should analyze (e.g. "Describe this image" or "Read all text in this image").
func (v *VisionClient) DescribeImage(ctx context.Context, dataURL string, prompt string) (string, error) {
	// Extract raw base64 from data URL if present.
	base64Data := dataURL
	if idx := strings.Index(dataURL, ","); idx >= 0 {
		base64Data = dataURL[idx+1:]
	}

	if prompt == "" {
		prompt = "Describe this image in detail. If there is any text visible in the image, read and transcribe ALL of it exactly as written. Include layout context (headers, labels, buttons, etc.)."
	}

	reqBody := ollamaChatRequest{
		Model: v.model,
		Messages: []ollamaChatMessage{
			{
				Role:    "user",
				Content: prompt,
				Images:  []string{base64Data},
			},
		},
		Stream: false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal vision request: %w", err)
	}

	url := v.ollamaBase + "/api/chat"
	debug.Info("vision", "POST %s (model: %s, image size: %d bytes)", url, v.model, len(base64Data))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create vision request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := v.httpClient.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		debug.Error("vision", "HTTP request failed after %s: %v", elapsed, err)
		return "", fmt.Errorf("vision http request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read vision response: %w", err)
	}

	debug.Info("vision", "Response: HTTP %d, %d bytes, %s elapsed", resp.StatusCode, len(respBytes), elapsed.Round(time.Millisecond))

	if resp.StatusCode != http.StatusOK {
		debug.Error("vision", "API error (HTTP %d): %s", resp.StatusCode, truncateStr(string(respBytes), 300))
		return "", fmt.Errorf("vision API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var chatResp ollamaChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshal vision response: %w", err)
	}

	description := strings.TrimSpace(chatResp.Message.Content)
	if description == "" {
		return "[Vision model returned empty description]", nil
	}

	debug.Info("vision", "Image described in %s (%d chars)", elapsed.Round(time.Millisecond), len(description))
	return description, nil
}

// DescribeImages processes multiple base64 images and returns combined descriptions.
func (v *VisionClient) DescribeImages(ctx context.Context, images []string, userMessage string) (string, error) {
	if len(images) == 0 {
		return "", nil
	}

	var descriptions []string
	for i, img := range images {
		prompt := "Describe this image in detail. If there is any text visible, read and transcribe ALL of it exactly."
		if userMessage != "" {
			prompt = fmt.Sprintf("The user said: \"%s\"\n\nLook at this image and respond to the user's request. If there is any text visible, read and transcribe it.", userMessage)
		}

		desc, err := v.DescribeImage(ctx, img, prompt)
		if err != nil {
			debug.Warn("vision", "Failed to describe image %d: %v", i+1, err)
			descriptions = append(descriptions, fmt.Sprintf("[Image %d: vision processing failed: %v]", i+1, err))
			continue
		}
		if len(images) > 1 {
			descriptions = append(descriptions, fmt.Sprintf("**Image %d:**\n%s", i+1, desc))
		} else {
			descriptions = append(descriptions, desc)
		}
	}

	return strings.Join(descriptions, "\n\n"), nil
}
