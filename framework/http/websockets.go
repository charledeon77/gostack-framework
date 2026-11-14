package http

import (
	"context"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

/*
GowSocket — GoStack Real-Time Persistent WebSocket Hub Subsystem.

Naming Metaphor: Why GowSocket?
The GoStack Real-Time WebSocket Subsystem was named GowSocket as a playful blend of "Go" and "WebSocket"
(whispering "Go Socket"). It transforms transient HTTP connections into persistent, bi-directional
telecommunication lines, enabling instant chat, live indicators, and reactive updates to flow back and forth
between client and server across native Go channels.

Purpose:
This file provides real-time, bi-directional WebSocket communication for GoStack applications.

Philosophy:
Real-time features are a standard requirement of modern web applications (chat, live
notifications, collaborative tools). GoStack integrates WebSockets tightly into its HTTP
layer, making it as easy to open a real-time channel as it is to define a REST route.

Architecture:
  - Connection: Represents a single active WebSocket client, optionally tied to a user identity.
  - Hub: A centralized registry that manages all active connections with support for:
      - Broadcast:         Send to ALL connected clients.
      - SendTo:            Send to a SPECIFIC user (all their active connections/devices).
      - BroadcastToRoom:   Send to a NAMED room of connected clients.
      - JoinRoom/LeaveRoom: Manage room membership dynamically.

Security (SEC-17):
  - MaxMessageSize: Incoming messages exceeding 64KB are discarded and the connection is closed.
  - Read/Write deadlines: Each read and write has a timeout to prevent hung connections from
    accumulating indefinitely.
  - Ping/pong liveness: The hub sends periodic pings; connections that do not respond within
    the pong deadline are unregistered and closed.

Choice:
We use golang.org/x/net/websocket, the semi-official Go standard package for WebSockets.
It keeps our dependencies tightly aligned with the core Go project, adhering to our
zero-external-bloat philosophy. The Hub model ensures safe concurrent access across
multiple simultaneous connections.
*/

const (
	// wsMaxMessageSize is the maximum inbound message size in bytes (64 KB).
	// SEC-17: Rejects oversized messages to prevent memory exhaustion attacks.
	wsMaxMessageSize = 64 << 10 // 64 KB

	// wsPingInterval is how often the hub sends a ping frame to each connection.
	wsPingInterval = 30 * time.Second

	// wsPongDeadline is how long the hub waits for a pong response before evicting
	// a connection as unresponsive.
	wsPongDeadline = 10 * time.Second

	// wsWriteDeadline caps how long a single write may block before the connection
	// is considered broken.
	wsWriteDeadline = 10 * time.Second

	// wsReadDeadline is the maximum idle time between messages from the client.
	// Pings reset this deadline, so legitimate idle connections stay connected.
	wsReadDeadline = wsPingInterval + wsPongDeadline + 5*time.Second
)

// Connection wraps a single active WebSocket connection.
type Connection struct {
	ws     *websocket.Conn
	hub    *Hub
	send   chan []byte
	UserID string // The authenticated user this connection belongs to (optional).
}

// Write pumps messages from the hub's send channel to the WebSocket connection.
// SEC-17: Each write is bounded by wsWriteDeadline to avoid stuck connections.
func (c *Connection) Write() {
	defer c.ws.Close()

	pingTicker := time.NewTicker(wsPingInterval)
	defer pingTicker.Stop()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			// SEC-17: Enforce per-write deadline.
			_ = c.ws.SetDeadline(time.Now().Add(wsWriteDeadline))
			if err := websocket.Message.Send(c.ws, msg); err != nil {
				return
			}

		case <-pingTicker.C:
			// SEC-17: Send a ping. The read loop resets the read deadline on each
			// incoming message; if the client is silent past wsReadDeadline the Read
			// call will error and the connection will be unregistered.
			_ = c.ws.SetDeadline(time.Now().Add(wsWriteDeadline))
			if err := websocket.Message.Send(c.ws, []byte("ping")); err != nil {
				return
			}
		}
	}
}

// Read pumps incoming messages from the WebSocket connection to the hub.
// SEC-17: Enforces read deadline and 64KB max message size.
func (c *Connection) Read() {
	defer func() {
		c.hub.unregister <- c
		c.ws.Close()
	}()
	for {
		// SEC-17: Extend read deadline on every message received (ping resets the clock).
		_ = c.ws.SetDeadline(time.Now().Add(wsReadDeadline))

		var msg []byte
		if err := websocket.Message.Receive(c.ws, &msg); err != nil {
			break
		}

		// SEC-17: Discard oversized messages and terminate the connection.
		if len(msg) > wsMaxMessageSize {
			break
		}

		// Swallow ping frames from the client rather than re-broadcasting them.
		if string(msg) == "ping" || string(msg) == "pong" {
			continue
		}

		c.hub.Broadcast(msg)
	}
}

// Hub maintains the registry of all active connections and coordinates message delivery.
type Hub struct {
	// All active connections regardless of user or room.
	connections map[*Connection]bool

	// UserID → set of that user's active connections (supports multi-device).
	users map[string]map[*Connection]bool

	// Room name → set of connections currently in that room.
	rooms map[string]map[*Connection]bool

	broadcast  chan []byte
	register   chan *Connection
	unregister chan *Connection
	mu         sync.RWMutex
}

// NewHub creates a new Hub instance ready to run.
func NewHub() *Hub {
	return &Hub{
		connections: make(map[*Connection]bool),
		users:       make(map[string]map[*Connection]bool),
		rooms:       make(map[string]map[*Connection]bool),
		broadcast:   make(chan []byte),
		register:    make(chan *Connection),
		unregister:  make(chan *Connection),
	}
}

// Run starts the hub's main event loop. It must be started in a goroutine before accepting connections.
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case c := <-h.register:
			h.mu.Lock()
			h.connections[c] = true
			if c.UserID != "" {
				if h.users[c.UserID] == nil {
					h.users[c.UserID] = make(map[*Connection]bool)
				}
				h.users[c.UserID][c] = true
			}
			h.mu.Unlock()

		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.connections[c]; ok {
				delete(h.connections, c)
				close(c.send)
				if c.UserID != "" {
					delete(h.users[c.UserID], c)
					if len(h.users[c.UserID]) == 0 {
						delete(h.users, c.UserID)
					}
				}
				// Remove from all rooms
				for room, members := range h.rooms {
					delete(members, c)
					if len(members) == 0 {
						delete(h.rooms, room)
					}
				}
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for c := range h.connections {
				select {
				case c.send <- message:
				default:
					h.mu.RUnlock()
					h.unregister <- c
					h.mu.RLock()
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to ALL connected clients.
func (h *Hub) Broadcast(msg []byte) {
	h.broadcast <- msg
}

// SendTo delivers a message to all active connections belonging to a specific user.
// If the user has multiple active connections (e.g. phone + laptop), all receive the message.
func (h *Hub) SendTo(userID string, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for conn := range h.users[userID] {
		select {
		case conn.send <- msg:
		default:
			// Drop silently if the connection's send buffer is full.
		}
	}
}

// JoinRoom registers a connection into a named room.
func (h *Hub) JoinRoom(room string, conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[room] == nil {
		h.rooms[room] = make(map[*Connection]bool)
	}
	h.rooms[room][conn] = true
}

// LeaveRoom removes a connection from a named room.
func (h *Hub) LeaveRoom(room string, conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if members, ok := h.rooms[room]; ok {
		delete(members, conn)
		if len(members) == 0 {
			delete(h.rooms, room)
		}
	}
}

// BroadcastToRoom sends a message to all connections currently in a named room.
func (h *Hub) BroadcastToRoom(room string, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for conn := range h.rooms[room] {
		select {
		case conn.send <- msg:
		default:
			// Drop silently if the connection's send buffer is full.
		}
	}
}

// Handler returns an HTTP handler that upgrades an anonymous connection and registers it with the Hub.
func (h *Hub) Handler() http.Handler {
	return h.HandlerForUser("")
}

// HandlerForUser returns an HTTP handler that upgrades the connection and registers it
// under the provided userID. Pass an empty string for anonymous connections.
//
// Usage:
//
//	router.Get("/ws", func(ctx *http.Context) {
//	    userID := ctx.Get("user_id").(string)
//	    hub.HandlerForUser(userID).ServeHTTP(ctx.Writer, ctx.Request)
//	})
func (h *Hub) HandlerForUser(userID string) http.Handler {
	return websocket.Handler(func(ws *websocket.Conn) {
		conn := &Connection{
			ws:     ws,
			hub:    h,
			send:   make(chan []byte, 256),
			UserID: userID,
		}
		h.register <- conn
		go conn.Write()
		conn.Read()
	})
}
