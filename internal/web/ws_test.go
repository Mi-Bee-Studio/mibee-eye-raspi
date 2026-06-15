package web

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestCheckOrigin_SameOrigin(t *testing.T) {
	req := httptest.NewRequest("GET", "http://192.168.63.162:8088/ws", nil)
	req.Host = "192.168.63.162:8088"
	req.Header.Set("Origin", "http://192.168.63.162:8088")

	if !checkOrigin(req) {
		t.Error("expected same-origin request to be accepted")
	}
}

func TestCheckOrigin_CrossOrigin(t *testing.T) {
	req := httptest.NewRequest("GET", "http://192.168.63.162:8088/ws", nil)
	req.Host = "192.168.63.162:8088"
	req.Header.Set("Origin", "http://evil.com")

	if checkOrigin(req) {
		t.Error("expected cross-origin request to be rejected")
	}
}

func TestCheckOrigin_NoOrigin(t *testing.T) {
	req := httptest.NewRequest("GET", "http://192.168.63.162:8088/ws", nil)
	req.Host = "192.168.63.162:8088"

	if !checkOrigin(req) {
		t.Error("expected request without Origin header to be accepted")
	}
}

func TestCheckOrigin_HTTPS(t *testing.T) {
	req := httptest.NewRequest("GET", "https://192.168.63.162:8088/ws", nil)
	req.Host = "192.168.63.162:8088"
	req.Header.Set("Origin", "https://192.168.63.162:8088")

	if !checkOrigin(req) {
		t.Error("expected same-origin HTTPS request to be accepted")
	}
}

func TestCheckOrigin_SameOriginNoPort(t *testing.T) {
	req := httptest.NewRequest("GET", "http://localhost/ws", nil)
	req.Host = "localhost"
	req.Header.Set("Origin", "http://localhost")

	if !checkOrigin(req) {
		t.Error("expected same-origin request without port to be accepted")
	}
}

func TestCheckOrigin_SameOriginDifferentPort(t *testing.T) {
	req := httptest.NewRequest("GET", "http://192.168.63.162:8088/ws", nil)
	req.Host = "192.168.63.162:8088"
	req.Header.Set("Origin", "http://192.168.63.162:9999")

	if checkOrigin(req) {
		t.Error("expected request from different port to be rejected")
	}
}


// ============================================================================
// wsHub tests
// ============================================================================

func TestWSHub_SendEvent(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	hub := newWSHub(logger)

	// sendEvent should not block
	hub.sendEvent(wsEvent{Type: "test", Name: "param", Value: 42})

	// Should have queued one event (capacity 64, we sent 1)
	if len(hub.events) != 1 {
		t.Errorf("expected 1 event in channel, got %d", len(hub.events))
	}
}

func TestWSHub_SendEvent_DropsWhenFull(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	hub := newWSHub(logger)

	// Fill the channel (capacity 64)
	for i := 0; i < 65; i++ {
		hub.sendEvent(wsEvent{Type: "test"})
	}

	// Should not panic or block — non-blocking drop
	// Channel should be full (= 64) with no room for more
	if len(hub.events) != 64 {
		t.Errorf("expected channel length 64, got %d", len(hub.events))
	}
}

func TestWSHub_Close_Idempotent(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	hub := newWSHub(logger)

	// close should be idempotent
	hub.close()
	hub.close() // second call should not panic
}

func TestWSHub_CloseStopsRun(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	hub := newWSHub(logger)
	hub.close()

	// After close, done channel is closed, so run() should exit immediately
	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		hub.run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(time.Second):
		t.Error("run should exit immediately after close")
	}
}

func TestWSHub_ContextStopsRun(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	hub := newWSHub(logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		hub.run(ctx)
		close(done)
	}()

	cancel() // cancel context

	select {
	case <-done:
		// success
	case <-time.After(time.Second):
		t.Error("run should exit after context cancellation")
	}
}

func TestWSHub_Broadcast(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	hub := newWSHub(logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.run(ctx)

	// Create a WebSocket server that upgrades connections and registers with hub
	testUpgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade failed: %v", err)
			return
		}
		client := &wsClient{conn: conn}
		hub.addClient(client)
		// Keep connection alive by reading in a loop
		// This goroutine exits when the connection is closed
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	// Connect a WebSocket client
	wsURL := "ws://" + server.Listener.Addr().String() + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Wait a moment for the upgrade and hub registration to complete
	time.Sleep(100 * time.Millisecond)

	// Send event through hub
	hub.sendEvent(wsEvent{Type: "param-changed", Name: "Brightness", Value: 1.0})

	// Read directly from the client-side websocket connection
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read broadcast message: %v", err)
	}

	var evt wsEvent
	if err := json.Unmarshal(msg, &evt); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}
	if evt.Type != "param-changed" {
		t.Errorf("expected type 'param-changed', got %q", evt.Type)
	}
	if evt.Name != "Brightness" {
		t.Errorf("expected name 'Brightness', got %q", evt.Name)
	}
}

func TestWSHub_BroadcastMultiple(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	hub := newWSHub(logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.run(ctx)

	testUpgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		client := &wsClient{conn: conn}
		hub.addClient(client)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	wsURL := "ws://" + server.Listener.Addr().String() + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	time.Sleep(100 * time.Millisecond)

	// Send two events
	hub.sendEvent(wsEvent{Type: "first", Name: "a", Value: 1})
	hub.sendEvent(wsEvent{Type: "second", Name: "b", Value: 2})

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	// Read first event
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var evt wsEvent
	json.Unmarshal(msg, &evt)
	if evt.Type != "first" {
		t.Errorf("expected 'first', got %q", evt.Type)
	}

	// Read second event
	_, msg, err = conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(msg, &evt)
	if evt.Type != "second" {
		t.Errorf("expected 'second', got %q", evt.Type)
	}
}

func TestWSHub_BroadcastCleanupDisconnected(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	hub := newWSHub(logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.run(ctx)

	// Create a test server that registers clients and keeps them alive
	testUpgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		client := &wsClient{conn: conn}
		hub.addClient(client)
		// Block until connection dies, then remove client (like wsReadPump)
		conn.ReadMessage()
		hub.removeClient(client)
	}))
	defer server.Close()

	wsURL := "ws://" + server.Listener.Addr().String() + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify client is registered
	hub.mu.RLock()
	initialCount := len(hub.clients)
	hub.mu.RUnlock()

	if initialCount != 1 {
		t.Fatalf("expected 1 client after connect, got %d", initialCount)
	}

	// Close the client side
	conn.Close()
	time.Sleep(100 * time.Millisecond)

	// Send an event — hub.run will try to write, fail, and remove the dead client
	hub.sendEvent(wsEvent{Type: "cleanup-test"})

	// Give hub.run time to process and remove
	time.Sleep(200 * time.Millisecond)

	// Hub should have cleaned up the dead client
	hub.mu.RLock()
	clientCount := len(hub.clients)
	hub.mu.RUnlock()

	if clientCount != 0 {
		t.Errorf("expected 0 clients after disconnect+cleanup, got %d", clientCount)
	}
}