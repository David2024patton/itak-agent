package builtins

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPFetchTool makes HTTP requests and returns the response body.
type HTTPFetchTool struct{}

func (h *HTTPFetchTool) Name() string { return "http_fetch" }

func (h *HTTPFetchTool) Description() string {
	return "Make an HTTP request (GET or POST) to a URL and return the response body. Useful for fetching web pages, APIs, or data."
}

func (h *HTTPFetchTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to fetch",
			},
			"method": map[string]interface{}{
				"type":        "string",
				"description": "HTTP method: GET or POST (default: GET)",
				"enum":        []string{"GET", "POST"},
			},
			"body": map[string]interface{}{
				"type":        "string",
				"description": "Request body (for POST requests)",
			},
			"headers": map[string]interface{}{
				"type":        "object",
				"description": "Optional HTTP headers as key-value pairs",
			},
		},
		"required": []string{"url"},
	}
}

func (h *HTTPFetchTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	url, ok := args["url"].(string)
	if !ok || url == "" {
		return "", fmt.Errorf("missing required argument: url")
	}

	method := "GET"
	if m, ok := args["method"].(string); ok && m != "" {
		method = strings.ToUpper(m)
	}

	var bodyReader io.Reader
	if body, ok := args["body"].(string); ok && body != "" {
		bodyReader = strings.NewReader(body)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	// Apply custom headers if provided.
	if headers, ok := args["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			if vs, ok := v.(string); ok {
				req.Header.Set(k, vs)
			}
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	// Truncate very large responses.
	result := string(data)
	if len(result) > 50000 {
		result = result[:50000] + "\n...(truncated)"
	}

	return fmt.Sprintf("HTTP %d\n\n%s", resp.StatusCode, result), nil
}
