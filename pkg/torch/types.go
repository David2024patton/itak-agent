package torch

import "time"

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
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
