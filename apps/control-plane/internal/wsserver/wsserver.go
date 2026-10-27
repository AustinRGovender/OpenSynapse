// Package wsserver implements the WebSocket API from
// docs/04-data-model-and-api.md section 3: clients subscribe to channels
// (runs.{id}.metrics, runs.{id}.events, runs.{id}.status, runs.{id}.errors,
// crawls.{id}.progress, system.notifications) and receive "event" messages.
package wsserver

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// Message is the WebSocket message format per docs/04-data-model-and-api.md section 3.
type Message struct {
	Type    string          `json:"type"`    // subscribe, unsubscribe, event, auth, error
	Channel string          `json:"channel"` // e.g. "runs.{id}.metrics"
	Payload json.RawMessage `json:"payload,omitempty"`
}

// sendQueue is how many frames may wait for a slow client before new ones are dropped.
const sendQueue = 128

// Server manages WebSocket connections and channel subscriptions.
type Server struct {
	mu      sync.RWMutex
	clients map[*client]struct{}

	// Authenticate, when set, must accept the token in the client's first
	// {"type":"auth","payload":{"token":"..."}} message before subscriptions
	// are honoured.
	Authenticate func(token string) bool
	// OriginPatterns are host patterns allowed to open connections from a
	// browser (see websocket.AcceptOptions). Empty means same-origin only.
	OriginPatterns []string

	dropped atomic.Int64
}

type client struct {
	conn     *websocket.Conn
	send     chan []byte
	mu       sync.Mutex
	channels map[string]bool
	authed   bool
}

// New creates a new WebSocket server.
func New() *Server {
	return &Server{clients: make(map[*client]struct{})}
}

// HandleWS is the HTTP handler for the WebSocket endpoint.
func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// The websocket library rejects cross-origin browser connections
		// unless the origin matches one of these patterns (ADR-0005, amended).
		OriginPatterns: s.OriginPatterns,
	})
	if err != nil {
		log.Printf("ws accept: %v", err)
		return
	}
	conn.SetReadLimit(64 * 1024)

	c := &client{
		conn:     conn,
		send:     make(chan []byte, sendQueue),
		channels: make(map[string]bool),
		authed:   s.Authenticate == nil,
	}

	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(r.Context())
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
		conn.Close(websocket.StatusNormalClosure, "")
	}()

	go c.writeLoop(ctx)

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
		case "auth":
			var p struct {
				Token string `json:"token"`
			}
			json.Unmarshal(msg.Payload, &p)
			if s.Authenticate == nil || s.Authenticate(p.Token) {
				c.authed = true
			} else {
				c.mu.Unlock()
				conn.Close(websocket.StatusPolicyViolation, "invalid token")
				return
			}
		case "subscribe":
			if c.authed {
				c.channels[msg.Channel] = true
			}
		case "unsubscribe":
			delete(c.channels, msg.Channel)
		}
		c.mu.Unlock()
	}
}

func (c *client) writeLoop(ctx context.Context) {
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case frame := <-c.send:
			wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := c.conn.Write(wctx, websocket.MessageText, frame)
			cancel()
			if err != nil {
				return
			}
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := c.conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

// Broadcast sends a message to all clients subscribed to the given channel.
// Frames for clients that cannot keep up are dropped, not queued forever.
func (s *Server) Broadcast(channel string, payload interface{}) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return
	}
	msgJSON, err := json.Marshal(Message{Type: "event", Channel: channel, Payload: payloadJSON})
	if err != nil {
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	for c := range s.clients {
		c.mu.Lock()
		subscribed := c.channels[channel]
		c.mu.Unlock()
		if !subscribed {
			continue
		}
		select {
		case c.send <- msgJSON:
		default:
			s.dropped.Add(1)
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

// DroppedFrames returns how many frames were dropped for slow clients.
func (s *Server) DroppedFrames() int64 {
	return s.dropped.Load()
}
