package events

import (
	"testing"
	"time"
)

// TestHub delivers each change to every subscriber until it leaves (RF-71).
func TestHub(t *testing.T) {
	h := NewHub()
	a, leaveA := h.Subscribe()
	b, leaveB := h.Subscribe()
	defer leaveB()
	h.Publish(Event{Topic: "market", UserID: 1})
	for name, ch := range map[string]<-chan Event{"a": a, "b": b} {
		select {
		case e := <-ch:
			if e.Topic != "market" || e.UserID != 1 {
				t.Fatalf("%s got %+v", name, e)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s got nothing", name)
		}
	}
	leaveA()
	if h.Count() != 1 {
		t.Fatalf("subscribers = %d", h.Count())
	}
	// A slow subscriber never blocks the publisher.
	for range 100 {
		h.Publish(Event{Topic: "cards"})
	}
}

// TestHubClose ends every open stream when the server shuts down, and later streams end at once.
func TestHubClose(t *testing.T) {
	h := NewHub()
	a, leaveA := h.Subscribe()
	h.Close()
	if _, open := <-a; open {
		t.Fatal("stream still open after Close")
	}
	leaveA()
	b, leaveB := h.Subscribe()
	defer leaveB()
	if _, open := <-b; open {
		t.Fatal("stream opened after Close")
	}
	h.Publish(Event{Topic: "market"})
	h.Close()
}

// TestTopic takes the area of the API from the path, skipping the ones that do not change shared data.
func TestTopic(t *testing.T) {
	tests := []struct{ path, want string }{
		{"/api/market/lists/3/items", "market"},
		{"/api/cards", "cards"},
		{"/api/auth/login", ""},
		{"/api/me/preferences", ""},
		{"/api/system/backups", ""},
		{"/other", ""},
	}
	for _, tt := range tests {
		if got := Topic(tt.path); got != tt.want {
			t.Errorf("Topic(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
