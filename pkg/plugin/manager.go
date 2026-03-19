package plugin

import (
	"context"
	"fmt"
	"sync"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// Manager handles the lifecycle of all registered channel plugins.
//
// What: Central registry that starts, stops, and monitors all plugins.
// Why:  Single point of control replaces the scattered server init in main.go.
// How:  Plugins register themselves, then StartAll/StopAll manages lifecycle.
type Manager struct {
	mu      sync.Mutex
	plugins map[string]Plugin
	handler MessageHandler
	running bool
}

// NewManager creates a plugin manager wired to the given message handler.
// The handler is called by plugins when they receive input -- it typically
// wraps orch.Run() to route messages through the agent.
func NewManager(handler MessageHandler) *Manager {
	return &Manager{
		plugins: make(map[string]Plugin),
		handler: handler,
	}
}

// Handler returns the shared MessageHandler for plugins to call.
func (m *Manager) Handler() MessageHandler {
	return m.handler
}

// Register adds a plugin to the manager. Must be called before StartAll.
func (m *Manager) Register(p Plugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := p.Name()
	if _, exists := m.plugins[name]; exists {
		return fmt.Errorf("plugin %q already registered", name)
	}
	m.plugins[name] = p
	debug.Info("plugin", "Registered plugin: %s", name)
	return nil
}

// StartAll starts all registered plugins. Plugins that fail to start
// are logged but do not block other plugins from starting.
func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var started int
	for name, p := range m.plugins {
		if err := p.Start(ctx); err != nil {
			debug.Error("plugin", "Failed to start plugin %q: %v", name, err)
			continue
		}
		started++
		debug.Info("plugin", "Started plugin: %s", name)
	}

	m.running = true
	debug.Info("plugin", "Plugin manager started: %d/%d plugins active", started, len(m.plugins))
	return nil
}

// StopAll gracefully shuts down all running plugins.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, p := range m.plugins {
		if err := p.Stop(); err != nil {
			debug.Warn("plugin", "Error stopping plugin %q: %v", name, err)
		} else {
			debug.Info("plugin", "Stopped plugin: %s", name)
		}
	}
	m.running = false
	debug.Info("plugin", "All plugins stopped")
}

// List returns info about all registered plugins.
func (m *Manager) List() []PluginInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	var infos []PluginInfo
	for name := range m.plugins {
		infos = append(infos, PluginInfo{
			Name:    name,
			Running: m.running,
		})
	}
	return infos
}

// Get returns a plugin by name, or nil if not found.
func (m *Manager) Get(name string) Plugin {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.plugins[name]
}

// Count returns the number of registered plugins.
func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.plugins)
}
