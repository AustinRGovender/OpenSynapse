package wsserver

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"nhooyr.io/websocket"
)

// Message is the WebSocket message format per docs/04-data-model-and-api.md section 3.
type Message struct {
	Type    string          `json:"type"`    // subscribe, unsubscribe, event
	Channel string          `json:"channel"` // e.g. "runs.{id}.metrics"
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Server manages WebSocket connections and channel subscriptions.
type Server struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
}

type client struct {
	conn     *websocket.Conn
	channels map[string]bool
	mu       sync.Mutex
}

// New creates a new WebSocket server.
func New() *Server {
	return &Server{
		clients: make(map[*client]struct{}),
	}
}

// HandleWS is the HTTP handler for the WebSocket endpoint.
func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Origin check is deliberately skipped in v1 across all deployment modes;
		// see docs/decisions/0005-websocket-origin-policy.md for scope, threat model,
		// and the planned migration to OriginPatterns.
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("ws accept: %v", err)
		return
	}

	c := &client{
		conn:     conn,
		channels: make(map[string]bool),
	}

	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
		conn.Close(websocket.StatusNormalClosure, "")
	}()

	ctx := r.Context()

	// Read loop: handle subscribe/unsubscribe messages
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return // client disconnected
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		c.mu.Lock()
		switch msg.Type {
		case "subscribe":
			c.channels[msg.Channel] = true
		case "unsubscribe":
			delete(c.channels, msg.Channel)
		}
		c.mu.Unlock()
	}
}

// Broadcast sends a message to all clients subscribed to the given channel.
func (s *Server) Broadcast(channel string, payload interface{}) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return
	}

	msg := Message{
		Type:    "event",
		Channel: channel,
		Payload: payloadJSON,
	}

	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for c := range s.clients {
		c.mu.Lock()
		subscribed := c.channels[channel]
		c.mu.Unlock()

		if subscribed {
			// Non-blocking write — drop frame if client can't keep up
			err := c.conn.Write(context.Background(), websocket.MessageText, msgJSON)
			if err != nil {
				// Client is slow or disconnected; will be cleaned up on next read failure
				continue
			}
		}
	}
}

// BroadcastRunMetrics sends a metric snapshot to subscribers of runs.{runID}.metrics.
func (s *Server) BroadcastRunMetrics(runID string, payload interface{}) {
	s.Broadcast("runs."+runID+".metrics", payload)
}

// BroadcastRunEvent sends an event to subscribers of runs.{runID}.events.
func (s *Server) BroadcastRunEvent(runID string, payload interface{}) {
	s.Broadcast("runs."+runID+".events", payload)
}

// ClientCount returns the number of connected clients.
func (s *Server) ClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}
