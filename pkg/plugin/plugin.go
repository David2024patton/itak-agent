// Package plugin defines the channel plugin interface for iTaK Agent.
//
// What: A Plugin receives input from an external source (HTTP, WebSocket,
//       Discord, CLI, VisionClaw glasses, etc.) and routes messages to the agent.
// Why:  Decouples I/O channels from the agent core so new channels can be
//       added without modifying main.go. Each plugin is independently
//       enabled/disabled via config.
// How:  Each plugin implements Start/Stop lifecycle and calls the shared
//       MessageHandler to route messages to orch.Run().
package plugin

import "context"

// Plugin is the interface all channel plugins implement.
type Plugin interface {
	// Name returns the plugin identifier (e.g. "web", "discord", "visionclaw").
	Name() string

	// Start begins accepting input. Called once during startup.
	// The context is cancelled on shutdown.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the plugin.
	Stop() error
}

// MessageHandler is the callback plugins use to send messages to the agent.
// It routes through the orchestrator and returns the agent's response.
type MessageHandler func(ctx context.Context, msg InboundMessage) (string, error)

// InboundMessage is the standardized input from any plugin channel.
type InboundMessage struct {
	// Text is the user's message content.
	Text string

	// Channel identifies the plugin source (e.g. "web", "discord", "glasses").
	Channel string

	// UserID is an optional platform-specific user identifier.
	UserID string

	// SessionID is an optional session to continue (0 = new session).
	SessionID int64

	// Media holds optional attachments (images from glasses, audio, etc.).
	Media []MediaAttachment

	// Metadata holds optional platform-specific key-value pairs.
	Metadata map[string]string
}

// MediaAttachment represents a binary attachment from a plugin.
type MediaAttachment struct {
	// Type is the MIME type (e.g. "image/jpeg", "audio/pcm").
	Type string

	// Data holds the raw bytes (mutually exclusive with URL).
	Data []byte

	// URL is an alternative to Data -- a URL to fetch the media from.
	URL string

	// Filename is an optional original filename.
	Filename string
}

// PluginInfo describes a running plugin for status reporting.
type PluginInfo struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	Port    int    `json:"port,omitempty"`
	Clients int    `json:"clients,omitempty"`
}
