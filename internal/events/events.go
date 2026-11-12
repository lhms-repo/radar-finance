// Package events tells the open screens that shared data changed, so the other
// iPhone refreshes without reloading, as the shared shopping lists need (RF-71).
package events

import (
	"strings"
	"sync"
)

// Event says which area of the API changed, who changed it and from which browser tab.
type Event struct {
	Topic    string `json:"topic"`
	UserID   int64  `json:"userId"`
	ClientID string `json:"clientId,omitempty"`
}

// Hub fans events out to every open connection.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// NewHub returns a hub without subscribers.
func NewHub() *Hub {
	return &Hub{subs: map[chan Event]struct{}{}}
}

// Subscribe returns the channel of a new connection and the function that ends it.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 16)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

// Publish sends the event to every connection, dropping it for one that is not reading.
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Count returns the number of open connections.
func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// private are the areas whose changes concern only the session itself.
var private = map[string]bool{"auth": true, "me": true, "system": true, "events": true}

// Topic returns the area of an API path, such as market for /api/market/lists, or empty for private areas.
func Topic(path string) string {
	rest, ok := strings.CutPrefix(path, "/api/")
	if !ok {
		return ""
	}
	topic, _, _ := strings.Cut(rest, "/")
	if private[topic] {
		return ""
	}
	return topic
}
