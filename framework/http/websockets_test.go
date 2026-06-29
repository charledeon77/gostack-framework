package http

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

// ─── Existing Tests (Regression Guard) ────────────────────────────────────────

func TestHub_Broadcast(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hub.Run(ctx)

	server := httptest.NewServer(hub.Handler())
	defer server.Close()

	wsURL := "ws" + server.URL[4:]

	ws1, err := websocket.Dial(wsURL, "", "http://localhost/")
	if err != nil {
		t.Fatalf("Failed to connect ws1: %v", err)
	}
	defer ws1.Close()

	ws2, err := websocket.Dial(wsURL, "", "http://localhost/")
	if err != nil {
		t.Fatalf("Failed to connect ws2: %v", err)
	}
	defer ws2.Close()

	time.Sleep(100 * time.Millisecond)

	msg := []byte("hello, world!")
	hub.Broadcast(msg)

	var rx1, rx2 []byte
	if err = websocket.Message.Receive(ws1, &rx1); err != nil {
		t.Fatalf("Failed to receive on ws1: %v", err)
	}
	if err = websocket.Message.Receive(ws2, &rx2); err != nil {
		t.Fatalf("Failed to receive on ws2: %v", err)
	}
	if string(rx1) != string(msg) {
		t.Errorf("Expected %q on ws1, got %q", msg, rx1)
	}
	if string(rx2) != string(msg) {
		t.Errorf("Expected %q on ws2, got %q", msg, rx2)
	}
}

func TestHub_ClientMessageBroadcasts(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hub.Run(ctx)

	server := httptest.NewServer(hub.Handler())
	defer server.Close()

	wsURL := "ws" + server.URL[4:]

	ws1, err := websocket.Dial(wsURL, "", "http://localhost/")
	if err != nil {
		t.Fatalf("Failed to connect ws1: %v", err)
	}
	defer ws1.Close()

	ws2, err := websocket.Dial(wsURL, "", "http://localhost/")
	if err != nil {
		t.Fatalf("Failed to connect ws2: %v", err)
	}
	defer ws2.Close()

	time.Sleep(100 * time.Millisecond)

	msg := []byte("message from client 1")
	if err = websocket.Message.Send(ws1, msg); err != nil {
		t.Fatalf("Failed to send on ws1: %v", err)
	}

	var rx2 []byte
	if err = websocket.Message.Receive(ws2, &rx2); err != nil {
		t.Fatalf("Failed to receive on ws2: %v", err)
	}
	if string(rx2) != string(msg) {
		t.Errorf("Expected %q on ws2, got %q", msg, rx2)
	}
}

// ─── New Tests ────────────────────────────────────────────────────────────────

func TestHub_SendTo(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Alice's connection
	aliceServer := httptest.NewServer(hub.HandlerForUser("alice"))
	defer aliceServer.Close()

	// Bob's connection
	bobServer := httptest.NewServer(hub.HandlerForUser("bob"))
	defer bobServer.Close()

	wsAlice, err := websocket.Dial("ws"+aliceServer.URL[4:], "", "http://localhost/")
	if err != nil {
		t.Fatalf("Failed to connect as alice: %v", err)
	}
	defer wsAlice.Close()

	wsBob, err := websocket.Dial("ws"+bobServer.URL[4:], "", "http://localhost/")
	if err != nil {
		t.Fatalf("Failed to connect as bob: %v", err)
	}
	defer wsBob.Close()

	time.Sleep(100 * time.Millisecond)

	// Send a private message to Alice only
	msg := []byte("private message for alice")
	hub.SendTo("alice", msg)

	// Alice should receive it
	var rxAlice []byte
	wsAlice.SetDeadline(time.Now().Add(500 * time.Millisecond))
	if err = websocket.Message.Receive(wsAlice, &rxAlice); err != nil {
		t.Fatalf("Alice did not receive the message: %v", err)
	}
	if string(rxAlice) != string(msg) {
		t.Errorf("Expected %q on alice, got %q", msg, rxAlice)
	}

	// Bob should NOT receive it — verify by timeout
	var rxBob []byte
	wsBob.SetDeadline(time.Now().Add(150 * time.Millisecond))
	err = websocket.Message.Receive(wsBob, &rxBob)
	if err == nil {
		t.Errorf("Bob should not have received the private message, but got: %q", rxBob)
	}
}

func TestHub_RoomBroadcast(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Two connections in "room-1", one outside
	serverA := httptest.NewServer(hub.HandlerForUser("user-a"))
	defer serverA.Close()
	serverB := httptest.NewServer(hub.HandlerForUser("user-b"))
	defer serverB.Close()
	serverC := httptest.NewServer(hub.HandlerForUser("user-c"))
	defer serverC.Close()

	wsA, _ := websocket.Dial("ws"+serverA.URL[4:], "", "http://localhost/")
	defer wsA.Close()
	wsB, _ := websocket.Dial("ws"+serverB.URL[4:], "", "http://localhost/")
	defer wsB.Close()
	wsC, _ := websocket.Dial("ws"+serverC.URL[4:], "", "http://localhost/")
	defer wsC.Close()

	time.Sleep(150 * time.Millisecond)

	// Manually join A and B into "room-1"
	hub.mu.RLock()
	for conn := range hub.users["user-a"] {
		hub.mu.RUnlock()
		hub.JoinRoom("room-1", conn)
		hub.mu.RLock()
	}
	for conn := range hub.users["user-b"] {
		hub.mu.RUnlock()
		hub.JoinRoom("room-1", conn)
		hub.mu.RLock()
	}
	hub.mu.RUnlock()

	// Broadcast to room-1
	msg := []byte("hello room-1")
	hub.BroadcastToRoom("room-1", msg)

	// A and B should receive it
	for label, ws := range map[string]*websocket.Conn{"A": wsA, "B": wsB} {
		var rx []byte
		ws.SetDeadline(time.Now().Add(500 * time.Millisecond))
		if err := websocket.Message.Receive(ws, &rx); err != nil {
			t.Errorf("User %s did not receive room message: %v", label, err)
		} else if string(rx) != string(msg) {
			t.Errorf("User %s expected %q, got %q", label, msg, rx)
		}
	}

	// C should NOT receive it
	var rxC []byte
	wsC.SetDeadline(time.Now().Add(150 * time.Millisecond))
	if err := websocket.Message.Receive(wsC, &rxC); err == nil {
		t.Errorf("User C should not have received the room message, but got: %q", rxC)
	}
}

func TestHub_MultiDeviceSendTo(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Same user "alice" connected from two devices
	server1 := httptest.NewServer(hub.HandlerForUser("alice"))
	defer server1.Close()
	server2 := httptest.NewServer(hub.HandlerForUser("alice"))
	defer server2.Close()

	wsDevice1, _ := websocket.Dial("ws"+server1.URL[4:], "", "http://localhost/")
	defer wsDevice1.Close()
	wsDevice2, _ := websocket.Dial("ws"+server2.URL[4:], "", "http://localhost/")
	defer wsDevice2.Close()

	time.Sleep(100 * time.Millisecond)

	msg := []byte("sync message for all alice devices")
	hub.SendTo("alice", msg)

	for label, ws := range map[string]*websocket.Conn{"device1": wsDevice1, "device2": wsDevice2} {
		var rx []byte
		ws.SetDeadline(time.Now().Add(500 * time.Millisecond))
		if err := websocket.Message.Receive(ws, &rx); err != nil {
			t.Errorf("Alice's %s did not receive the message: %v", label, err)
		} else if string(rx) != string(msg) {
			t.Errorf("Alice's %s expected %q, got %q", label, msg, rx)
		}
	}
}
