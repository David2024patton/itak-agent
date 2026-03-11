package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/tool"
)

// WrapToolsForRegistry takes the discovered MCP tools and wraps each one
// into the iTaK tool.Tool interface so agents can call them seamlessly.
// The wrapped tools appear in the agent's tool list just like built-in tools.
//
// Why: Agents don't need to know whether a tool is built-in or external.
// MCP tools get the same interface as shell, file_read, etc. This is the
// bridge between MCP protocol and the iTaK tool registry.
//
// How: Each MCP tool becomes an MCPTool struct that implements tool.Tool.
// When Execute() is called, it forwards the call to the MCP client's
// CallTool() method and returns the result.
func WrapToolsForRegistry(client *Client) []tool.Tool {
	wrapped := make([]tool.Tool, 0, len(client.Tools))
	for _, ti := range client.Tools {
		t := &MCPTool{
			client:   client,
			toolInfo: ti,
		}
		wrapped = append(wrapped, t)
		debug.Debug("mcp", "Wrapped MCP tool: %s -> %s", client.Name, ti.Name)
	}
	return wrapped
}

// MCPTool wraps a single MCP server tool as an iTaK tool.Tool.
type MCPTool struct {
	client   *Client
	toolInfo ToolInfo
}

// Name returns the tool name, prefixed with the MCP server name.
// Example: "mcp_github_create_issue" for a GitHub MCP server tool called "create_issue".
func (t *MCPTool) Name() string {
	return fmt.Sprintf("mcp_%s_%s", t.client.Name, t.toolInfo.Name)
}

// Description returns the tool's description from the MCP server.
func (t *MCPTool) Description() string {
	return fmt.Sprintf("[MCP:%s] %s", t.client.Name, t.toolInfo.Description)
}

// Schema returns the input schema as a Go map (compatible with iTaK's tool format).
func (t *MCPTool) Schema() map[string]interface{} {
	if t.toolInfo.InputSchema == nil {
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(t.toolInfo.InputSchema, &schema); err != nil {
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}
	return schema
}

// Execute calls the MCP server tool and returns the result string.
func (t *MCPTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	result, err := t.client.CallTool(ctx, t.toolInfo.Name, args)
	if err != nil {
		return "", err
	}
	return result, nil
}
