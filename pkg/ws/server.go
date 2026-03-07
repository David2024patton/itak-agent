package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/eventbus"
	"github.com/gorilla/websocket"
)

// ── WebSocket upgrader ─────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow all origins for local development. The dashboard and CLI
	// both connect from localhost.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ── Client ─────────────────────────────────────────────────────────

// client is a single connected WebSocket session.
type client struct {
	conn   *websocket.Conn
	send   chan []byte
	id     int
	closed bool // guards against double-close of send channel
}

// writePump drains the send channel and writes to the WebSocket.
func (c *client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

// ── Inbound message from dashboard ─────────────────────────────────

// InboundMessage is a JSON message received from a WebSocket client.
type InboundMessage struct {
	Action  string `json:"action"`  // "user_input", "cancel", "subscribe"
	Message string `json:"message"` // payload for user_input
}

// ── Server ─────────────────────────────────────────────────────────

// Server manages WebSocket connections and relays events from the bus.
type Server struct {
	mu       sync.Mutex
	clients  map[int]*client
	nextID   int
	bus      *eventbus.EventBus
	subID    int
	server   *http.Server
	port     int
	inputCh  chan string // user input received from dashboard clients
}

// NewServer creates a WebSocket server wired to the given event bus.
func NewServer(bus *eventbus.EventBus, port int) *Server {
	return &Server{
		clients: make(map[int]*client),
		bus:     bus,
		port:    port,
		inputCh: make(chan string, 16),
	}
}

// InputChannel returns a read-only channel of user input strings
// received from connected dashboard clients.
func (s *Server) InputChannel() <-chan string {
	return s.inputCh
}

// Port returns the port the server is configured to listen on.
func (s *Server) Port() int {
	return s.port
}

// Start begins listening for WebSocket connections and relaying events.
// This is non-blocking; it starts the HTTP server in a goroutine.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	// Subscribe to ALL events from the bus.
	subID, events := s.bus.Subscribe(128)
	s.subID = subID

	// Relay goroutine: reads from bus, broadcasts to all clients.
	go s.relayLoop(events)

	// HTTP server goroutine.
	go func() {
		debug.Info("ws", "WebSocket server listening on ws://localhost:%d/ws", s.port)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			debug.Error("ws", "WebSocket server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the WebSocket server.
func (s *Server) Stop() {
	debug.Info("ws", "Shutting down WebSocket server...")

	// Unsubscribe from the bus (closes the event channel, ending relayLoop).
	s.bus.Unsubscribe(s.subID)

	// Close all client connections.
	s.mu.Lock()
	for id, c := range s.clients {
		if !c.closed {
			c.closed = true
			close(c.send)
		}
		delete(s.clients, id)
	}
	s.mu.Unlock()

	// Shut down the HTTP server.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if s.server != nil {
		s.server.Shutdown(ctx)
	}

	debug.Info("ws", "WebSocket server stopped")
}

// ── HTTP handlers ──────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	debug.Debug("ws", "Health check from %s", r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"clients": s.clientCount(),
		"port":    s.port,
	})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		debug.Error("ws", "Upgrade failed: %v", err)
		return
	}

	c := s.addClient(conn)
	debug.Info("ws", "Client #%d connected from %s (total: %d)", c.id, r.RemoteAddr, s.clientCount())

	// Send a welcome event (safely, in case Stop() races).
	welcome := eventbus.NewEvent(eventbus.TopicSystemStatus, "connected")
	welcome.Data = map[string]interface{}{
		"client_id": c.id,
		"version":   "0.2.0",
	}
	if !s.safeSend(c, welcome.JSON()) {
		return // client was closed before we could send
	}

	// Writer goroutine.
	go c.writePump()

	// Reader loop (handles inbound messages from the dashboard).
	s.readLoop(c)
}

// ── Internal methods ───────────────────────────────────────────────

func (s *Server) addClient(conn *websocket.Conn) *client {
	s.mu.Lock()
	defer s.mu.Unlock()

	c := &client{
		conn: conn,
		send: make(chan []byte, 64),
		id:   s.nextID,
	}
	s.nextID++
	s.clients[c.id] = c
	return c
}

func (s *Server) removeClient(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if c, ok := s.clients[id]; ok {
		if !c.closed {
			c.closed = true
			close(c.send)
		}
		delete(s.clients, id)
		debug.Info("ws", "Client #%d disconnected (total: %d)", id, len(s.clients))
	}
}

func (s *Server) clientCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients)
}

// safeSend attempts to send data to a client's send channel.
// Returns false if the client was already closed (avoids send-on-closed panic).
func (s *Server) safeSend(c *client, data []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- data:
		return true
	default:
		debug.Warn("ws", "Client #%d send buffer full during safeSend", c.id)
		return false
	}
}

// broadcast sends a JSON payload to all connected clients.
func (s *Server) broadcast(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, c := range s.clients {
		select {
		case c.send <- data:
		default:
			// Client too slow, skip.
			debug.Warn("ws", "Client #%d send buffer full, dropping event", id)
		}
	}
}

// relayLoop reads events from the bus and broadcasts to all clients.
func (s *Server) relayLoop(events <-chan eventbus.Event) {
	for event := range events {
		debug.Debug("ws", "Relaying event [%s] to %d client(s)", event.Topic, s.clientCount())
		s.broadcast(event.JSON())
	}
	debug.Debug("ws", "Relay loop exited (bus channel closed)")
}

// readLoop reads inbound JSON messages from a WebSocket client.
func (s *Server) readLoop(c *client) {
	defer s.removeClient(c.id)

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				debug.Warn("ws", "Client #%d unexpected close: %v", c.id, err)
			} else {
				debug.Debug("ws", "Client #%d disconnected: %v", c.id, err)
			}
			return // client disconnected
		}

		var msg InboundMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		switch msg.Action {
		case "user_input":
			if msg.Message != "" {
				debug.Debug("ws", "Received user_input from client #%d: %s", c.id, msg.Message)
				select {
				case s.inputCh <- msg.Message:
				default:
					debug.Warn("ws", "Input channel full, dropping message from client #%d", c.id)
				}
			}
		case "cancel":
			debug.Info("ws", "Cancel requested by client #%d", c.id)
			// Future: wire into context cancellation
		}
	}
}
