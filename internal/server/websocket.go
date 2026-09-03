package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"sys-sentient/internal/models"

	"github.com/gorilla/websocket"
)

var ErrBroadcastQueueFull = errors.New("websocket broadcast queue full")

// WSMessage represents a message sent over WebSocket
type WSMessage struct {
	Type    string              `json:"type"` // "metrics"
	Payload *models.SystemState `json:"payload"`
}

// Client represents a single WebSocket connection
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	// done makes Run terminable. Without it Run was an infinite for/select
	// that leaked on every shutdown and left clients hanging.
	done      chan struct{}
	closeOnce sync.Once
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		done:       make(chan struct{}),
	}
}

// Run starts the hub's event loop
// Close stops Run and disconnects every client. Safe to call more than once.
func (h *Hub) Close() {
	h.closeOnce.Do(func() {
		close(h.done)
	})
}

func (h *Hub) Run() {
	// On exit, close every client's send channel so their write pumps finish
	// instead of blocking on a hub that is no longer reading.
	defer func() {
		h.mu.Lock()
		for client := range h.clients {
			delete(h.clients, client)
			close(client.send)
		}
		h.mu.Unlock()
	}()

	for {
		select {
		case <-h.done:
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			slog.Debug("websocket client connected", "clients", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			slog.Debug("websocket client disconnected", "clients", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client buffer full, skip
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastMetrics sends metrics to all connected clients
func (h *Hub) BroadcastMetrics(state *models.SystemState) error {
	msg := WSMessage{
		Type:    "metrics",
		Payload: state,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}
	select {
	case h.broadcast <- data:
		return nil
	default:
		return ErrBroadcastQueueFull
	}
}

// ClientCount returns the number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

type websocketOriginValidatedKey struct{}

func markWebSocketOriginValidated(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), websocketOriginValidatedKey{}, true))
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		if r.Header.Get("Origin") == "" {
			return true
		}
		validated, _ := r.Context().Value(websocketOriginValidatedKey{}).(bool)
		return validated
	},
}

// writePump pumps messages from the hub to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			// A deadline error means the connection is already gone; the write
			// below surfaces the real failure and returns.
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump reads messages from the WebSocket (mostly for handling close)
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		// Returning the error would abort the read loop on a connection that
		// is already closing; ReadMessage reports it.
		return c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// ServeWs handles WebSocket requests from clients
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("websocket upgrade failed", "error", err)
		return
	}

	client := &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),
	}
	hub.register <- client

	go client.writePump()
	go client.readPump()
}
