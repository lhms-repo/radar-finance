package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/events"
)

// TestEventsStayInGroup sends a change only to the streams of the same group, so a write in one group never
// reloads the screens of another (RF-02, RF-71).
func TestEventsStayInGroup(t *testing.T) {
	srv, conn := testServer(t)
	accounts := account.NewService(conn)
	ctx := context.Background()
	carla, err := accounts.Create(ctx, "carla", "Carla Ficticia")
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.SetPIN(ctx, carla.ID, "9264"); err != nil {
		t.Fatal(err)
	}
	beach, err := accounts.CreateGroup(ctx, "Praia", carla.ID)
	if err != nil {
		t.Fatal(err)
	}
	ana, other := newClient(t), newClient(t)
	signIn(t, ana, srv, "ana")
	if resp := postJSON(t, other, srv, "/api/auth/login", fmt.Sprintf(`{"groupId":%d,"userId":%d,"pin":"9264"}`, beach.ID, carla.ID)); resp.StatusCode != http.StatusOK {
		t.Fatalf("carla login = %d", resp.StatusCode)
	}

	streamCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(streamCtx, http.MethodGet, srv.URL+"/api/events", nil)
	resp, err := other.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	lines := bufio.NewScanner(resp.Body)
	lines.Scan() // ": connected"

	// Ana writes in her group first; the only event the beach stream may see is the write of Carla.
	postJSON(t, ana, srv, "/api/market/markets", `{"name":"Mercado Ana","neighborhood":""}`)
	postJSON(t, other, srv, "/api/market/markets", `{"name":"Mercado Praia","neighborhood":""}`)
	for lines.Scan() {
		data, ok := strings.CutPrefix(lines.Text(), "data: ")
		if !ok {
			continue
		}
		var e struct {
			UserID  int64 `json:"userId"`
			GroupID int64 `json:"groupId"`
		}
		_ = json.Unmarshal([]byte(data), &e)
		if e.UserID != carla.ID || e.GroupID != beach.ID {
			t.Fatalf("the beach stream received %s", data)
		}
		return
	}
	t.Fatalf("no event: %v", lines.Err())
}

// TestEventsStream sends the change of one user to the open stream of the other, and refuses signed out streams (RF-71).
func TestEventsStream(t *testing.T) {
	srv, _ := testServer(t)
	if resp := get(t, newClient(t), srv.URL+"/api/events"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("signed out = %d", resp.StatusCode)
	}
	ana, bruno := newClient(t), newClient(t)
	signIn(t, ana, srv, "ana")
	signIn(t, bruno, srv, "bruno")

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

	// The tab id lets another device of the same person refresh, while the tab that wrote skips the event.
	write, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/market/markets", strings.NewReader(`{"name":"Mercado Ficticio","neighborhood":""}`))
	write.Header.Set("Content-Type", "application/json")
	write.Header.Set("Origin", srv.URL)
	write.Header.Set("Sec-Fetch-Site", "same-origin")
	write.Header.Set(clientHeader, "tab-1")
	if r, err := bruno.Do(write); err != nil {
		t.Fatal(err)
	} else {
		r.Body.Close()
	}
	for lines.Scan() {
		data, ok := strings.CutPrefix(lines.Text(), "data: ")
		if !ok {
			continue
		}
		var e struct {
			Topic    string `json:"topic"`
			UserID   int64  `json:"userId"`
			ClientID string `json:"clientId"`
		}
		_ = json.Unmarshal([]byte(data), &e)
		if e.Topic != "market" || e.UserID == 0 || e.ClientID != "tab-1" {
			t.Fatalf("event = %s", data)
		}
		return
	}
	t.Fatalf("no event: %v", lines.Err())
}

// TestEventsStreamEndsOnHubClose ends an open stream when the hub closes at shutdown, so Shutdown does not
// wait for the phones that keep the app open (RF-71).
func TestEventsStreamEndsOnHubClose(t *testing.T) {
	_, conn := testServer(t)
	sm := auth.NewSessionManager(conn, auth.SessionOptions{Idle: time.Hour, Lifetime: 24 * time.Hour})
	ctx, err := sm.Load(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.SignIn(ctx, sm, 1, 1); err != nil {
		t.Fatal(err)
	}
	token, _, err := sm.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	hub := events.NewHub()
	srv := httptest.NewServer(eventsHandler(hub, sm))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.AddCookie(&http.Cookie{Name: sm.Cookie.Name, Value: token})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	lines := bufio.NewScanner(resp.Body)
	if !lines.Scan() || lines.Text() != ": connected" {
		t.Fatalf("first line = %q", lines.Text())
	}
	hub.Close()
	ended := make(chan struct{})
	go func() {
		for lines.Scan() {
		}
		close(ended)
	}()
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("stream still open after the hub closed")
	}
}
