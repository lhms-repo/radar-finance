package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestEventsStream sends the change of one user to the open stream of the other, and refuses signed out streams (RF-71).
func TestEventsStream(t *testing.T) {
	srv, _ := testServer(t)
	if resp := get(t, newClient(t), srv.URL+"/api/events"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("signed out = %d", resp.StatusCode)
	}
	ana, bruno := newClient(t), newClient(t)
	postJSON(t, ana, srv, "/api/auth/login", `{"login":"ana"}`)
	postJSON(t, bruno, srv, "/api/auth/login", `{"login":"bruno"}`)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/events", nil)
	resp, err := ana.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream = %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	lines := bufio.NewScanner(resp.Body)
	lines.Scan() // ": connected"

	postJSON(t, bruno, srv, "/api/market/markets", `{"name":"Mercado Ficticio","neighborhood":""}`)
	for lines.Scan() {
		data, ok := strings.CutPrefix(lines.Text(), "data: ")
		if !ok {
			continue
		}
		var e struct {
			Topic  string `json:"topic"`
			UserID int64  `json:"userId"`
		}
		_ = json.Unmarshal([]byte(data), &e)
		if e.Topic != "market" || e.UserID == 0 {
			t.Fatalf("event = %s", data)
		}
		return
	}
	t.Fatalf("no event: %v", lines.Err())
}
