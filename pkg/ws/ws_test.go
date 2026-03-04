package ws

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/David2024patton/GOAgent/pkg/eventbus"
	"github.com/gorilla/websocket"
)

func TestHealthEndpoint(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()

	port := 48901
	srv := NewServer(bus, port)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer srv.Stop()

	// Give the server a moment to start listening.
	time.Sleep(200 * time.Millisecond)

	// Hit the health endpoint.
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", body["status"])
	}
	if int(body["port"].(float64)) != port {
		t.Errorf("expected port %d, got %v", port, body["port"])
	}
}

func TestWebSocketConnection(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()

	port := 48902
	srv := NewServer(bus, port)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer srv.Stop()

	time.Sleep(200 * time.Millisecond)

	// Connect via WebSocket.
	wsURL := fmt.Sprintf("ws://localhost:%d/ws", port)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer conn.Close()

	// Should receive a welcome event immediately.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read welcome event: %v", err)
	}

	var welcome eventbus.Event
	if err := json.Unmarshal(message, &welcome); err != nil {
		t.Fatalf("parse welcome event: %v", err)
	}

	if welcome.Topic != eventbus.TopicSystemStatus {
		t.Errorf("expected welcome topic %q, got %q", eventbus.TopicSystemStatus, welcome.Topic)
	}
	if welcome.Message != "connected" {
		t.Errorf("expected welcome message 'connected', got %q", welcome.Message)
	}

	t.Logf("Welcome event received: %s", string(message))
}

func TestEventRelay(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()

	port := 48903
	srv := NewServer(bus, port)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer srv.Stop()

	time.Sleep(200 * time.Millisecond)

	// Connect.
	wsURL := fmt.Sprintf("ws://localhost:%d/ws", port)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer conn.Close()

	// Read and discard welcome event.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	conn.ReadMessage()

	// Publish an event via the bus.
	bus.Publish(eventbus.Event{
		Topic:   eventbus.TopicAgentStart,
		Agent:   "scout",
		Message: "listing files in data/",
	})

	// Should receive the relayed event.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read relayed event: %v", err)
	}

	var event eventbus.Event
	if err := json.Unmarshal(message, &event); err != nil {
		t.Fatalf("parse relayed event: %v", err)
	}

	if event.Topic != eventbus.TopicAgentStart {
		t.Errorf("expected topic %q, got %q", eventbus.TopicAgentStart, event.Topic)
	}
	if event.Agent != "scout" {
		t.Errorf("expected agent %q, got %q", "scout", event.Agent)
	}
	if event.Message != "listing files in data/" {
		t.Errorf("expected message %q, got %q", "listing files in data/", event.Message)
	}

	t.Logf("Relayed event received: topic=%s agent=%s", event.Topic, event.Agent)
}

func TestInboundUserInput(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()

	port := 48904
	srv := NewServer(bus, port)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer srv.Stop()

	time.Sleep(200 * time.Millisecond)

	// Connect.
	wsURL := fmt.Sprintf("ws://localhost:%d/ws", port)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer conn.Close()

	// Read and discard welcome.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	conn.ReadMessage()

	// Send user_input from the "dashboard".
	inbound := InboundMessage{
		Action:  "user_input",
		Message: "hello from dashboard",
	}
	data, _ := json.Marshal(inbound)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("write message: %v", err)
	}

	// Read from the input channel.
	select {
	case msg := <-srv.InputChannel():
		if msg != "hello from dashboard" {
			t.Errorf("expected %q, got %q", "hello from dashboard", msg)
		}
		t.Logf("Received inbound user input: %s", msg)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for user_input on InputChannel")
	}
}

func TestMultipleClients(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()

	port := 48905
	srv := NewServer(bus, port)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer srv.Stop()

	time.Sleep(200 * time.Millisecond)

	// Connect 3 clients.
	conns := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		wsURL := fmt.Sprintf("ws://localhost:%d/ws", port)
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("client %d dial failed: %v", i, err)
		}
		defer conn.Close()
		conns[i] = conn

		// Read and discard welcome.
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		conn.ReadMessage()
	}

	// Publish an event.
	bus.Publish(eventbus.NewEvent(eventbus.TopicSystemReady, "broadcast test"))

	// All 3 clients should receive it.
	for i, conn := range conns {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("client %d read failed: %v", i, err)
		}

		var event eventbus.Event
		json.Unmarshal(msg, &event)
		if event.Topic != eventbus.TopicSystemReady {
			t.Errorf("client %d: expected topic %q, got %q", i, eventbus.TopicSystemReady, event.Topic)
		}
	}

	t.Logf("All 3 clients received broadcast event")
}

func TestGracefulShutdown(t *testing.T) {
	bus := eventbus.New()

	port := 48906
	srv := NewServer(bus, port)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Connect a client.
	wsURL := fmt.Sprintf("ws://localhost:%d/ws", port)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	// Read the welcome event to ensure the connection is fully established.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read welcome event: %v", err)
	}

	// Now stop the server.
	srv.Stop()
	bus.Close()

	// Give the OS a moment to tear down the socket.
	time.Sleep(300 * time.Millisecond)

	// After shutdown, the client connection should be broken.
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Error("expected read error after server shutdown, got nil")
	}

	t.Logf("Graceful shutdown verified (read error: %v)", err)
}
