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

// Client is the interface for all LLM providers.
type Client interface {
	Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error)
}

// ProviderConfig holds connection details for an LLM provider.
type ProviderConfig struct {
	Provider string `yaml:"provider" json:"provider"`
	Model    string `yaml:"model" json:"model"`
	APIBase  string `yaml:"api_base" json:"api_base"`
	APIKey   string `yaml:"api_key" json:"api_key"`
}

// OpenAIClient implements Client for any OpenAI-compatible API.
type OpenAIClient struct {
	config          ProviderConfig
	httpClient      *http.Client
	noToolsSupport  bool // set when model returns "does not support tools"
}

// NewOpenAIClient creates a client for any OpenAI-compatible endpoint.
func NewOpenAIClient(cfg ProviderConfig) *OpenAIClient {
	if cfg.APIBase == "" {
		cfg.APIBase = "https://api.openai.com/v1"
	}
	return &OpenAIClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// NoToolsSupport returns true if this model was detected as not supporting
// structured tool definitions. Callers should include tool info in the system
// prompt instead and rely on text-based tool call parsing.
func (c *OpenAIClient) NoToolsSupport() bool {
	return c.noToolsSupport
}

// chatRequest is the request body for /chat/completions.
type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []ToolDef `json:"tools,omitempty"`
}

// chatResponse is the response body from /chat/completions.
type chatResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage,omitempty"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Chat sends a chat completion request and returns the parsed response.
func (c *OpenAIClient) Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error) {
	reqBody := chatRequest{
		Model:    c.config.Model,
		Messages: messages,
	}
	// Only send tools if the model supports them.
	if len(tools) > 0 && !c.noToolsSupport {
		reqBody.Tools = tools
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.config.APIBase + "/chat/completions"
	debug.Debug("llm", "POST %s (model: %s, messages: %d, tools: %d, body: %d bytes)",
		url, c.config.Model, len(messages), len(tools), len(bodyBytes))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		debug.Error("llm", "HTTP request failed after %s: %v", elapsed, err)
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	debug.Debug("llm", "Response: HTTP %d, %d bytes, %s elapsed", resp.StatusCode, len(respBytes), elapsed.Round(time.Millisecond))

	// Detect "does not support tools" error and retry without tools.
	if resp.StatusCode == http.StatusBadRequest && len(tools) > 0 && !c.noToolsSupport {
		respStr := string(respBytes)
		if strings.Contains(respStr, "does not support tools") ||
			strings.Contains(respStr, "tool use is not supported") ||
			strings.Contains(respStr, "tools is not supported") {
			debug.Warn("llm", "Model %q does not support structured tools. Switching to text-based tool calling.", c.config.Model)
			c.noToolsSupport = true
			// Retry without tools.
			return c.Chat(ctx, messages, tools)
		}
	}

	if resp.StatusCode != http.StatusOK {
		debug.Error("llm", "API error (HTTP %d): %s", resp.StatusCode, truncateStr(string(respBytes), 500))
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		debug.Error("llm", "Failed to parse response JSON: %v", err)
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if chatResp.Error != nil {
		debug.Error("llm", "API returned error: %s (%s)", chatResp.Error.Message, chatResp.Error.Type)
		return nil, fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		debug.Error("llm", "No choices in response")
		return nil, fmt.Errorf("no choices in response")
	}

	choice := chatResp.Choices[0]

	// Strip thinking tags (Qwen3, DeepSeek-R1 wrap reasoning in <think>...</think>).
	// Do this at the client level so every consumer gets clean content.
	content := stripThinkTags(choice.Message.Content)

	result := &Response{
		Content:      content,
		ToolCalls:    choice.Message.ToolCalls,
		FinishReason: choice.FinishReason,
		Usage:        chatResp.Usage,
	}

	debug.Debug("llm", "Finish reason: %s, tool_calls: %d, content length: %d",
		result.FinishReason, len(result.ToolCalls), len(result.Content))

	return result, nil
}

// ModelInfo holds basic info about an available model.
type ModelInfo struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by,omitempty"`
}

// modelsResponse is the response from GET /models.
type modelsResponse struct {
	Data []ModelInfo `json:"data"`
}

// ListModels calls the /models endpoint and returns available model IDs.
func (c *OpenAIClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	url := c.config.APIBase + "/models"
	debug.Debug("llm", "GET %s", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models API error (status %d): %s", resp.StatusCode, truncateStr(string(respBytes), 200))
	}

	var mr modelsResponse
	if err := json.Unmarshal(respBytes, &mr); err != nil {
		return nil, fmt.Errorf("unmarshal models: %w", err)
	}

	debug.Debug("llm", "Found %d models from %s", len(mr.Data), c.config.APIBase)
	return mr.Data, nil
}

// truncateStr is a local helper (avoids circular import).
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// stripThinkTags removes <think>...</think> blocks from model output.
// Qwen3 and DeepSeek-R1 wrap chain-of-thought reasoning in these tags.
// Stripping them at the client level ensures no downstream consumer
// accidentally parses thinking text as tool calls or delegation data.
func stripThinkTags(content string) string {
	result := content
	for {
		start := strings.Index(result, "<think>")
		if start < 0 {
			break
		}
		end := strings.Index(result, "</think>")
		if end < 0 {
			// Unclosed think tag: remove from <think> to end.
			result = result[:start]
			break
		}
		result = result[:start] + result[end+len("</think>"):]
	}
	return strings.TrimSpace(result)
}
