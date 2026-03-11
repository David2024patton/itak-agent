package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// Transport abstracts the communication layer to an MCP server.
// Supported transports: stdio (subprocess), SSE (HTTP streaming).
type Transport interface {
	// Send sends a JSON-RPC message to the server.
	Send(ctx context.Context, msg json.RawMessage) error

	// Receive blocks until a JSON-RPC message is received.
	Receive(ctx context.Context) (json.RawMessage, error)

	// Close shuts down the transport.
	Close() error
}

// StdioTransport communicates with an MCP server via stdin/stdout of a subprocess.
// This is the primary transport for local MCP servers (e.g., file system, git).
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	mu     sync.Mutex
}

// NewStdioTransport starts an MCP server as a subprocess and connects via stdio.
func NewStdioTransport(command string, args ...string) (*StdioTransport, error) {
	cmd := exec.Command(command, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: start server %q: %w", command, err)
	}

	debug.Info("mcp", "Started stdio server: %s (pid: %d)", command, cmd.Process.Pid)

	return &StdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewScanner(stdout),
	}, nil
}

// Send writes a JSON-RPC message to the server's stdin.
func (t *StdioTransport) Send(_ context.Context, msg json.RawMessage) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	line := append(msg, '\n')
	_, err := t.stdin.Write(line)
	return err
}

// Receive reads the next JSON-RPC message from the server's stdout.
func (t *StdioTransport) Receive(_ context.Context) (json.RawMessage, error) {
	if t.stdout.Scan() {
		return json.RawMessage(t.stdout.Bytes()), nil
	}
	if err := t.stdout.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// Close kills the subprocess.
func (t *StdioTransport) Close() error {
	_ = t.stdin.Close()
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	return t.cmd.Wait()
}

// ── JSON-RPC 2.0 types ────────────────────────────────────────────

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("MCP error %d: %s", e.Code, e.Message)
}

// ── MCP Protocol types ─────────────────────────────────────────────

// ToolInfo describes a tool exposed by an MCP server.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"` // JSON Schema
}

// ServerInfo holds metadata about the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitResult is the response from the initialize handshake.
type InitResult struct {
	ServerInfo   ServerInfo `json:"serverInfo"`
	ProtocolVersion string `json:"protocolVersion"`
}

// ToolsListResult is the response from tools/list.
type ToolsListResult struct {
	Tools []ToolInfo `json:"tools"`
}

// ToolCallResult is the response from tools/call.
type ToolCallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError"`
}

// ContentBlock is a piece of tool output.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ── Client ─────────────────────────────────────────────────────────

// Client is an MCP protocol client that connects to an external MCP server.
// It handles the JSON-RPC 2.0 handshake, tool discovery, and tool calling.
//
// Why: MCP is the standard way AI agents connect to external services.
// By supporting MCP, iTaK Agent can use any MCP server (GitHub, filesystem,
// databases, Context7, etc.) without writing custom integrations.
//
// How: The client sends JSON-RPC messages over a Transport (stdio or SSE).
// On Connect(), it performs the initialize handshake and discovers tools.
// Tool calls go through CallTool() which handles the request/response cycle.
type Client struct {
	Name      string // human-readable name for this server connection
	Transport Transport
	Tools     []ToolInfo // discovered tools
	Server    ServerInfo // server metadata

	nextID atomic.Int64
}

// NewClient creates an MCP client with the given transport.
func NewClient(name string, transport Transport) *Client {
	return &Client{
		Name:      name,
		Transport: transport,
	}
}

// Connect performs the MCP initialize handshake and discovers tools.
func (c *Client) Connect(ctx context.Context) error {
	debug.Info("mcp", "Connecting to MCP server %q...", c.Name)

	// Step 1: Initialize handshake.
	initParams, _ := json.Marshal(map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]string{
			"name":    "iTaKAgent",
			"version": "0.2.0",
		},
	})

	initResp, err := c.call(ctx, "initialize", initParams)
	if err != nil {
		return fmt.Errorf("mcp: initialize failed: %w", err)
	}

	var initResult InitResult
	if err := json.Unmarshal(initResp, &initResult); err != nil {
		return fmt.Errorf("mcp: parse init result: %w", err)
	}

	c.Server = initResult.ServerInfo
	debug.Info("mcp", "Connected to %q (version: %s, protocol: %s)",
		c.Server.Name, c.Server.Version, initResult.ProtocolVersion)

	// Step 2: Send initialized notification (no response expected).
	notif := Request{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	notifBytes, _ := json.Marshal(notif)
	_ = c.Transport.Send(ctx, notifBytes)

	// Step 3: Discover tools.
	toolsResp, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		debug.Warn("mcp", "tools/list failed (server may not expose tools): %v", err)
		return nil
	}

	var toolsResult ToolsListResult
	if err := json.Unmarshal(toolsResp, &toolsResult); err != nil {
		return fmt.Errorf("mcp: parse tools list: %w", err)
	}

	c.Tools = toolsResult.Tools
	debug.Info("mcp", "Discovered %d tools from %q", len(c.Tools), c.Name)
	for _, t := range c.Tools {
		debug.Debug("mcp", "  Tool: %s - %s", t.Name, t.Description)
	}

	return nil
}

// CallTool invokes a tool on the MCP server.
func (c *Client) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	params, _ := json.Marshal(map[string]interface{}{
		"name":      toolName,
		"arguments": args,
	})

	resp, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return "", fmt.Errorf("mcp: call %q: %w", toolName, err)
	}

	var toolResult ToolCallResult
	if err := json.Unmarshal(resp, &toolResult); err != nil {
		return "", fmt.Errorf("mcp: parse tool result: %w", err)
	}

	if toolResult.IsError {
		var errText string
		for _, block := range toolResult.Content {
			if block.Type == "text" {
				errText += block.Text
			}
		}
		return "", fmt.Errorf("mcp tool error: %s", errText)
	}

	// Concatenate text content blocks.
	var result string
	for _, block := range toolResult.Content {
		if block.Type == "text" {
			result += block.Text
		}
	}

	return result, nil
}

// Close disconnects from the MCP server.
func (c *Client) Close() error {
	debug.Info("mcp", "Disconnecting from %q", c.Name)
	return c.Transport.Close()
}

// call sends a JSON-RPC request and waits for the response.
func (c *Client) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	id := c.nextID.Add(1)

	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	debug.Debug("mcp", "-> %s (id=%d)", method, id)

	if err := c.Transport.Send(ctx, reqBytes); err != nil {
		return nil, err
	}

	// Wait for response with timeout.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for {
		respBytes, err := c.Transport.Receive(ctx)
		if err != nil {
			return nil, err
		}

		var resp Response
		if err := json.Unmarshal(respBytes, &resp); err != nil {
			continue // skip malformed messages
		}

		if resp.ID == id {
			if resp.Error != nil {
				return nil, resp.Error
			}
			debug.Debug("mcp", "<- %s (id=%d) result=%d bytes", method, id, len(resp.Result))
			return resp.Result, nil
		}
		// Not our response -- keep reading (could be a notification).
	}
}
