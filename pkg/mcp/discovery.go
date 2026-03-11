package mcp

import (
	"context"
	"fmt"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// ServerConfig defines how to connect to a single MCP server.
type ServerConfig struct {
	Name      string   `yaml:"name"`      // human-readable name (becomes tool prefix)
	Transport string   `yaml:"transport"` // "stdio" or "sse"
	Command   string   `yaml:"command"`   // path to server binary (for stdio)
	Args      []string `yaml:"args"`      // command arguments (for stdio)
	URL       string   `yaml:"url"`       // URL for SSE transport
	Enabled   bool     `yaml:"enabled"`   // default: true
}

// Manager handles lifecycle of multiple MCP server connections.
// It discovers servers from config, connects them, and provides
// a unified list of all available MCP tools.
type Manager struct {
	clients []*Client
}

// NewManager creates a Manager and connects to all configured MCP servers.
func NewManager(ctx context.Context, configs []ServerConfig) (*Manager, error) {
	m := &Manager{}

	for _, cfg := range configs {
		if !cfg.Enabled {
			debug.Debug("mcp", "Skipping disabled MCP server: %s", cfg.Name)
			continue
		}

		client, err := connectServer(ctx, cfg)
		if err != nil {
			// Non-fatal: log the error but continue with other servers.
			debug.Warn("mcp", "Failed to connect MCP server %q: %v", cfg.Name, err)
			continue
		}

		m.clients = append(m.clients, client)
	}

	debug.Info("mcp", "MCP Manager initialized: %d/%d servers connected", len(m.clients), len(configs))
	return m, nil
}

// Clients returns all connected MCP clients.
func (m *Manager) Clients() []*Client {
	return m.clients
}

// Close disconnects from all MCP servers.
func (m *Manager) Close() {
	for _, c := range m.clients {
		if err := c.Close(); err != nil {
			debug.Warn("mcp", "Error closing MCP server %q: %v", c.Name, err)
		}
	}
}

// connectServer creates a transport, connects, and initializes a single MCP server.
func connectServer(ctx context.Context, cfg ServerConfig) (*Client, error) {
	var transport Transport
	var err error

	switch cfg.Transport {
	case "stdio", "":
		if cfg.Command == "" {
			return nil, fmt.Errorf("stdio transport requires 'command'")
		}
		transport, err = NewStdioTransport(cfg.Command, cfg.Args...)
		if err != nil {
			return nil, err
		}

	case "sse":
		return nil, fmt.Errorf("SSE transport not yet implemented")

	default:
		return nil, fmt.Errorf("unknown transport: %q", cfg.Transport)
	}

	client := NewClient(cfg.Name, transport)
	if err := client.Connect(ctx); err != nil {
		_ = transport.Close()
		return nil, err
	}

	return client, nil
}
