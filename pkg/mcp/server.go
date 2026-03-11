package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync/atomic"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/tool"
)

// Server exposes iTaK Agent tools as an MCP server over stdio.
// External agents (Cursor, Cline, other iTaK instances) can connect
// to this server and use iTaK tools via the MCP protocol.
//
// Why: MCP is bidirectional. The client lets iTaK consume external tools.
// The server lets external tools consume iTaK's capabilities. Together
// they make iTaK a full MCP citizen.
//
// How: Reads JSON-RPC 2.0 requests from stdin, dispatches to handlers
// for initialize, tools/list, and tools/call, writes responses to stdout.
type Server struct {
	Name    string
	Version string
	Tools   *tool.Registry
	nextID  atomic.Int64
}

// NewServer creates an MCP server that exposes the given tools.
func NewServer(name, version string, tools *tool.Registry) *Server {
	return &Server{
		Name:    name,
		Version: version,
		Tools:   tools,
	}
}

// Serve starts the MCP server loop on stdin/stdout.
// It blocks until stdin is closed or ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	debug.Info("mcp-server", "Starting MCP server %q v%s (%d tools)", s.Name, s.Version, len(s.Tools.Names()))

	scanner := bufio.NewScanner(os.Stdin)
	// Increase scanner buffer for large tool arguments.
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			debug.Warn("mcp-server", "Invalid JSON-RPC request: %v", err)
			s.writeError(os.Stdout, 0, -32700, "Parse error")
			continue
		}

		debug.Debug("mcp-server", "<- %s (id=%d)", req.Method, req.ID)

		switch req.Method {
		case "initialize":
			s.handleInitialize(os.Stdout, req)
		case "notifications/initialized":
			// Client acknowledgement -- no response needed.
			debug.Debug("mcp-server", "Client initialized")
		case "tools/list":
			s.handleToolsList(os.Stdout, req)
		case "tools/call":
			s.handleToolsCall(ctx, os.Stdout, req)
		default:
			s.writeError(os.Stdout, req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("mcp-server: stdin read error: %w", err)
	}
	return nil
}

// handleInitialize responds to the MCP initialize handshake.
func (s *Server) handleInitialize(w io.Writer, req Request) {
	result := InitResult{
		ServerInfo: ServerInfo{
			Name:    s.Name,
			Version: s.Version,
		},
		ProtocolVersion: "2025-03-26",
	}

	resultBytes, _ := json.Marshal(result)
	s.writeResponse(w, req.ID, resultBytes)
	debug.Info("mcp-server", "Initialize handshake complete")
}

// handleToolsList responds with the list of available tools.
func (s *Server) handleToolsList(w io.Writer, req Request) {
	toolInfos := make([]ToolInfo, 0, len(s.Tools.Names()))

	for _, name := range s.Tools.Names() {
		t, ok := s.Tools.Get(name)
		if !ok {
			continue
		}

		// Convert parameters to JSON Schema.
		inputSchema, _ := json.Marshal(t.Schema())

		toolInfos = append(toolInfos, ToolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: inputSchema,
		})
	}

	result := ToolsListResult{Tools: toolInfos}
	resultBytes, _ := json.Marshal(result)
	s.writeResponse(w, req.ID, resultBytes)
	debug.Info("mcp-server", "Listed %d tools", len(toolInfos))
}

// handleToolsCall executes a tool and returns the result.
func (s *Server) handleToolsCall(ctx context.Context, w io.Writer, req Request) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(w, req.ID, -32602, fmt.Sprintf("Invalid params: %v", err))
		return
	}

	t, ok := s.Tools.Get(params.Name)
	if !ok {
		s.writeToolError(w, req.ID, fmt.Sprintf("Unknown tool: %s. Available: %v", params.Name, s.Tools.Names()))
		return
	}

	debug.Info("mcp-server", "Calling tool %q", params.Name)

	result, err := t.Execute(ctx, params.Arguments)
	if err != nil {
		s.writeToolError(w, req.ID, fmt.Sprintf("Tool %q failed: %v", params.Name, err))
		return
	}

	toolResult := ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: result}},
		IsError: false,
	}
	resultBytes, _ := json.Marshal(toolResult)
	s.writeResponse(w, req.ID, resultBytes)
	debug.Debug("mcp-server", "Tool %q returned %d chars", params.Name, len(result))
}

// writeResponse sends a successful JSON-RPC response.
func (s *Server) writeResponse(w io.Writer, id int64, result json.RawMessage) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(resp)
	data = append(data, '\n')
	_, _ = w.Write(data)
}

// writeError sends a JSON-RPC error response (protocol-level error).
func (s *Server) writeError(w io.Writer, id int64, code int, message string) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	}
	data, _ := json.Marshal(resp)
	data = append(data, '\n')
	_, _ = w.Write(data)
}

// writeToolError sends a tool-level error (not a protocol error).
func (s *Server) writeToolError(w io.Writer, id int64, message string) {
	toolResult := ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: message}},
		IsError: true,
	}
	resultBytes, _ := json.Marshal(toolResult)
	s.writeResponse(w, id, resultBytes)
}
