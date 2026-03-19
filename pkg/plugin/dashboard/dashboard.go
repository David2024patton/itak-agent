// Package dashboard wraps the WebSocket event relay as a channel plugin.
package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/eventbus"
	"github.com/David2024patton/iTaKAgent/pkg/plugin"
	"github.com/gorilla/websocket"
)

// Config holds settings for the dashboard WebSocket plugin.
type Config struct {
	Port int `yaml:"port"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type client struct {
	conn   *websocket.Conn
	send   chan []byte
	id     int
	closed bool
}

func (c *client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

// Plugin provides WebSocket event relay for the dashboard.
type Plugin struct {
	port    int
	bus     *eventbus.EventBus
	handler plugin.MessageHandler
	server  *http.Server
	clients map[int]*client
	nextID  int
	subID   int
	inputCh chan string
	mu      sync.Mutex
}

// New creates a dashboard WebSocket plugin.
func New(cfg Config, bus *eventbus.EventBus, handler plugin.MessageHandler) *Plugin {
	return &Plugin{
		port:    cfg.Port,
		bus:     bus,
		handler: handler,
		clients: make(map[int]*client),
		inputCh: make(chan string, 16),
	}
}

func (p *Plugin) Name() string { return "dashboard" }

func (p *Plugin) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", p.handleWebSocket)
	mux.HandleFunc("/health", p.handleHealth)

	p.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", p.port),
		Handler: mux,
	}

	// Subscribe to all bus events.
	subID, events := p.bus.Subscribe(128)
	p.subID = subID

	// Relay events to connected clients.
	go p.relayLoop(events)

	go func() {
		debug.Info("plugin:dashboard", "WebSocket listening on ws://localhost:%d/ws", p.port)
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			debug.Error("plugin:dashboard", "Server error: %v", err)
		}
	}()

	return nil
}

func (p *Plugin) Stop() error {
	p.bus.Unsubscribe(p.subID)

	p.mu.Lock()
	for id, c := range p.clients {
		if !c.closed {
			c.closed = true
			close(c.send)
		}
		delete(p.clients, id)
	}
	p.mu.Unlock()

	if p.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return p.server.Shutdown(ctx)
	}
	return nil
}

// InputChannel returns a read-only channel of user input from dashboard clients.
func (p *Plugin) InputChannel() <-chan string {
	return p.inputCh
}

// Port returns the configured port.
func (p *Plugin) Port() int { return p.port }

// ── HTTP + WebSocket handlers ──────────────────────────────────────

func (p *Plugin) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"plugin":  "dashboard",
		"clients": p.clientCount(),
	})
}

func (p *Plugin) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		debug.Error("plugin:dashboard", "Upgrade failed: %v", err)
		return
	}

	c := p.addClient(conn)
	debug.Info("plugin:dashboard", "Client #%d connected (total: %d)", c.id, p.clientCount())

	// Welcome event.
	welcome := eventbus.NewEvent(eventbus.TopicSystemStatus, "connected")
	welcome.Data = map[string]interface{}{"client_id": c.id, "plugin": "dashboard"}
	p.safeSend(c, welcome.JSON())

	go c.writePump()
	p.readLoop(c)
}

func (p *Plugin) addClient(conn *websocket.Conn) *client {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := &client{conn: conn, send: make(chan []byte, 64), id: p.nextID}
	p.nextID++
	p.clients[c.id] = c
	return c
}

func (p *Plugin) removeClient(id int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[id]; ok {
		if !c.closed {
			c.closed = true
			close(c.send)
		}
		delete(p.clients, id)
	}
}

func (p *Plugin) clientCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.clients)
}

func (p *Plugin) safeSend(c *client, data []byte) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- data:
		return true
	default:
		return false
	}
}

func (p *Plugin) broadcast(data []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.clients {
		select {
		case c.send <- data:
		default:
		}
	}
}

func (p *Plugin) relayLoop(events <-chan eventbus.Event) {
	for event := range events {
		p.broadcast(event.JSON())
	}
}

type inboundMsg struct {
	Action  string `json:"action"`
	Message string `json:"message"`
}

func (p *Plugin) readLoop(c *client) {
	defer p.removeClient(c.id)
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg inboundMsg
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}
		if msg.Action == "user_input" && msg.Message != "" {
			select {
			case p.inputCh <- msg.Message:
			default:
			}
		}
	}
}
