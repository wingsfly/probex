package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// SSEEvent represents a server-sent event.
type SSEEvent struct {
	Type string `json:"type"` // "result", "node_status", "alert"
	Data any    `json:"data"`
}

// SSEBroker manages SSE client subscriptions and broadcasts events.
type SSEBroker struct {
	mu      sync.RWMutex
	clients map[chan SSEEvent]struct{}
}

func NewSSEBroker() *SSEBroker {
	return &SSEBroker{clients: make(map[chan SSEEvent]struct{})}
}

func (b *SSEBroker) Subscribe() chan SSEEvent {
	ch := make(chan SSEEvent, 32)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *SSEBroker) Unsubscribe(ch chan SSEEvent) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

// Publish sends an event to all connected SSE clients.
func (b *SSEBroker) Publish(event SSEEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- event:
		default:
			// Drop if client is slow
		}
	}
}

// HandleSSE is the HTTP handler for GET /api/v1/stream.
func (b *SSEBroker) HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	// Send initial connected event
	fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	for {
		select {
		case event := <-ch:
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// SetSSEBroker enables SSE on the server.
func (s *Server) SetSSEBroker(broker *SSEBroker) {
	s.router.Get("/api/v1/stream", broker.HandleSSE)
}
