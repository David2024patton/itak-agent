// Package discord provides a Discord bot channel plugin.
//
// What: Listens for messages in configured Discord channels and routes
//       them through the agent orchestrator.
// Why:  Enables conversational access to all agent personas via Discord.
// How:  Uses discordgo to connect to Discord, listen for messages,
//       route through MessageHandler, and reply in the same channel.
package discord

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/plugin"
)

// Config holds settings for the Discord bot plugin.
type Config struct {
	Token      string   `yaml:"token"`
	GuildID    string   `yaml:"guild_id"`
	ChannelIDs []string `yaml:"channel_ids"`
	Prefix     string   `yaml:"prefix"` // command prefix, default "!"
}

// Plugin provides Discord bot integration.
// NOTE: This is a stub implementation. To fully enable, add the
// discordgo dependency (github.com/bwmarrin/discordgo) and wire
// the session handlers. The interface is ready for drop-in activation.
type Plugin struct {
	cfg     Config
	handler plugin.MessageHandler
	cancel  context.CancelFunc
	mu      sync.Mutex
	running bool
}

// New creates a Discord bot plugin.
func New(cfg Config, handler plugin.MessageHandler) *Plugin {
	if cfg.Prefix == "" {
		cfg.Prefix = "!"
	}
	return &Plugin{
		cfg:     cfg,
		handler: handler,
	}
}

func (p *Plugin) Name() string { return "discord" }

func (p *Plugin) Start(ctx context.Context) error {
	if p.cfg.Token == "" {
		return fmt.Errorf("discord token is required")
	}

	_, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	// Stub: In production, this would create a discordgo.Session,
	// register message handlers, and call session.Open().
	//
	// The handler pattern would be:
	//   session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
	//       if m.Author.Bot { return }
	//       if !p.isAllowedChannel(m.ChannelID) { return }
	//
	//       msg := plugin.InboundMessage{
	//           Text:    m.Content,
	//           Channel: "discord",
	//           UserID:  m.Author.ID,
	//           Metadata: map[string]string{
	//               "guild_id":   m.GuildID,
	//               "channel_id": m.ChannelID,
	//               "username":   m.Author.Username,
	//           },
	//       }
	//
	//       response, err := p.handler(ctx, msg)
	//       if err != nil {
	//           s.ChannelMessageSend(m.ChannelID, "Error: "+err.Error())
	//           return
	//       }
	//       // Split long responses for Discord's 2000 char limit.
	//       for _, chunk := range splitMessage(response, 1950) {
	//           s.ChannelMessageSend(m.ChannelID, chunk)
	//       }
	//   })

	p.running = true
	debug.Info("plugin:discord", "Discord bot plugin registered (stub mode - add discordgo dependency to activate)")
	return nil
}

func (p *Plugin) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}
	p.running = false
	debug.Info("plugin:discord", "Discord bot stopped")
	return nil
}

// isAllowedChannel checks if a channel ID is in the allowed list.
// Empty list means all channels are allowed.
func (p *Plugin) isAllowedChannel(channelID string) bool {
	if len(p.cfg.ChannelIDs) == 0 {
		return true
	}
	for _, id := range p.cfg.ChannelIDs {
		if id == channelID {
			return true
		}
	}
	return false
}

// splitMessage breaks a long message into chunks for Discord's char limit.
func splitMessage(msg string, maxLen int) []string {
	if len(msg) <= maxLen {
		return []string{msg}
	}
	var chunks []string
	for len(msg) > 0 {
		if len(msg) <= maxLen {
			chunks = append(chunks, msg)
			break
		}
		// Try to split at a newline.
		idx := strings.LastIndex(msg[:maxLen], "\n")
		if idx < 0 {
			idx = maxLen
		}
		chunks = append(chunks, msg[:idx])
		msg = msg[idx:]
		if len(msg) > 0 && msg[0] == '\n' {
			msg = msg[1:]
		}
	}
	return chunks
}
