package http

import (
	"context"
	"net/http"
	"sync"

	"golang.org/x/net/websocket"
)

/*
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

Choice:
We use golang.org/x/net/websocket, the semi-official Go standard package for WebSockets.
It keeps our dependencies tightly aligned with the core Go project, adhering to our
zero-external-bloat philosophy. The Hub model ensures safe concurrent access across
multiple simultaneous connections.
*/

// Connection wraps a single active WebSocket connection.
type Connection struct {
	ws     *websocket.Conn
	hub    *Hub
	send   chan []byte
	UserID string // The authenticated user this connection belongs to (optional).
}

// Write pumps messages from the hub's send channel to the WebSocket connection.
func (c *Connection) Write() {
	defer c.ws.Close()
	for msg := range c.send {
		if err := websocket.Message.Send(c.ws, msg); err != nil {
			break
		}
	}
}

// Read pumps incoming messages from the WebSocket connection to the hub.
func (c *Connection) Read() {
	defer func() {
		c.hub.unregister <- c
		c.ws.Close()
	}()
	for {
		var msg []byte
		if err := websocket.Message.Receive(c.ws, &msg); err != nil {
			break
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
