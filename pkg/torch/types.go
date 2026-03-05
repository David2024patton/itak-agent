package torch

import (
	"encoding/json"
	"time"
)

// ---------- Engine Options ----------

// EngineOpts configures how a GGUF model is loaded and run.
type EngineOpts struct {
	// ContextSize is the max context window (n_ctx). Default: 2048.
	ContextSize int `json:"context_size" yaml:"context_size"`
	// Threads is the number of CPU threads to use. Default: runtime.NumCPU().
	Threads int `json:"threads" yaml:"threads"`
	// GPULayers is how many layers to offload to GPU. 0 = CPU only.
	GPULayers int `json:"gpu_layers" yaml:"gpu_layers"`
}

// CompletionParams controls inference behavior per request.
type CompletionParams struct {
	MaxTokens   int      `json:"max_tokens,omitempty"`
	Temperature float64  `json:"temperature,omitempty"`
	TopP        float64  `json:"top_p,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

// ---------- Model Manager Types ----------

// ModelEntry represents a cached GGUF model file.
type ModelEntry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	LastUsed time.Time `json:"last_used"`
}

// ---------- OpenAI-Compatible API Types ----------

// ChatRequest is the incoming request body for /v1/chat/completions.
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	TopP        *float64      `json:"top_p,omitempty"`
	Stop        []string      `json:"stop,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

// ChatMessage is a single message in a chat conversation.
// Handles both OpenAI text format (content as string) and vision format
// (content as array of content parts with text + image_url entries).
type ChatMessage struct {
	Role      string        `json:"role"`
	Content   string        `json:"content"`
	ImageData []ContentPart `json:"-"` // populated from content array for vision
}

// UnmarshalJSON handles both string content and array content (OpenAI vision format).
func (m *ChatMessage) UnmarshalJSON(data []byte) error {
	// Try the simple string-content format first.
	type plainMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	var plain plainMessage
	if err := json.Unmarshal(data, &plain); err == nil && plain.Content != "" {
		m.Role = plain.Role
		m.Content = plain.Content
		return nil
	}

	// Try array-content format (OpenAI vision API).
	type arrayMessage struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	var arrMsg arrayMessage
	if err := json.Unmarshal(data, &arrMsg); err != nil {
		return err
	}
	m.Role = arrMsg.Role

	var parts []ContentPart
	if err := json.Unmarshal(arrMsg.Content, &parts); err != nil {
		// Content was neither string nor array - try raw string again.
		var s string
		if err2 := json.Unmarshal(arrMsg.Content, &s); err2 == nil {
			m.Content = s
			return nil
		}
		return err
	}

	// Split parts into text content and image data.
	for _, part := range parts {
		if part.Type == "text" {
			if m.Content != "" {
				m.Content += "\n"
			}
			m.Content += part.Text
		} else {
			m.ImageData = append(m.ImageData, part)
		}
	}
	return nil
}

// ContentPart represents a single content block in a multi-modal message.
// Type is either "text" or "image_url".
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL contains the URL (or base64 data URI) for an image.
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // "auto", "low", "high"
}

// ChatResponse is the response body for /v1/chat/completions.
type ChatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   ChatUsage    `json:"usage"`
}

// ChatChoice is a single completion choice.
type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// ChatUsage tracks token consumption for a request.
type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ModelsResponse is the response body for GET /v1/models.
type ModelsResponse struct {
	Object string      `json:"object"`
	Data   []ModelInfo `json:"data"`
}

// ModelInfo describes a single model available on this server.
type ModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// HealthResponse is the response body for GET /health.
type HealthResponse struct {
	Status    string `json:"status"`
	ModelName string `json:"model_name,omitempty"`
	Uptime    string `json:"uptime,omitempty"`
}
