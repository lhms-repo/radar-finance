package auth

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/db"
)

// openDB returns a migrated database for one test.
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "radar.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	return conn
}

// TestLateResponseSendsNoCookie checks, through the real middleware, that a slow request finishing after the
// logout keeps its body but neither saves the session nor sends the dead cookie back (RF-01).
func TestLateResponseSendsNoCookie(t *testing.T) {
	sm := NewSessionManager(openDB(t), SessionOptions{Idle: time.Hour, Lifetime: 24 * time.Hour})
	entered, release := make(chan struct{}), make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		if err := SignIn(r.Context(), sm, 7); err != nil {
			t.Error(err)
		}
	})
	mux.HandleFunc("POST /logout", func(w http.ResponseWriter, r *http.Request) {
		if err := SignOut(r.Context(), sm); err != nil {
			t.Error(err)
		}
	})
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /me", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", int(CurrentUser(r.Context(), sm)))))
	})
	h := sm.LoadAndSave(mux)

	// do runs one request with the cookie and returns the response.
	do := func(method, path string, cookie *http.Cookie) *http.Response {
		req := httptest.NewRequest(method, path, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Result()
	}
	cookies := do(http.MethodPost, "/login", nil).Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login cookies = %v", cookies)
	}
	session := cookies[0]

	slow := make(chan *http.Response)
	go func() { slow <- do(http.MethodGet, "/slow", session) }()
	<-entered
	do(http.MethodPost, "/logout", session)
	close(release)
	late := <-slow

	body, _ := io.ReadAll(late.Body)
	if late.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("slow response = %d %q", late.StatusCode, body)
	}
	if c := late.Header.Values("Set-Cookie"); len(c) != 0 {
		t.Fatalf("slow response sent cookies %v after the logout", c)
	}
	me, _ := io.ReadAll(do(http.MethodGet, "/me", session).Body)
	if len(me) != 0 {
		t.Fatalf("session user after logout = %d, want signed out", len(me))
	}
}

// TestSignOutSurvivesLateCommit keeps a session signed out when a request that loaded it before the
// logout finishes after it; with an idle timeout scs saves the session at the end of every request (RF-01).
func TestSignOutSurvivesLateCommit(t *testing.T) {
	ctx := context.Background()
	sm := NewSessionManager(openDB(t), SessionOptions{Idle: time.Hour, Lifetime: 24 * time.Hour})

	tests := []struct {
		name       string
		lateCommit bool
	}{
		{name: "logout alone"},
		{name: "request started before the logout commits after it", lateCommit: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			login, err := sm.Load(ctx, "")
			if err != nil {
				t.Fatal(err)
			}
			if err := SignIn(login, sm, 7); err != nil {
				t.Fatal(err)
			}
			token, _, err := sm.Commit(login)
			if err != nil {
				t.Fatal(err)
			}

			// A slow request loads the session, then the logout destroys it.
			slow, err := sm.Load(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			logout, err := sm.Load(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			if err := SignOut(logout, sm); err != nil {
				t.Fatal(err)
			}
			if tt.lateCommit {
				if _, _, err := sm.Commit(slow); !errors.Is(err, errRevoked) {
					t.Fatalf("late commit err = %v, want errRevoked", err)
				}
			}

			after, err := sm.Load(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			if id := CurrentUser(after, sm); id != 0 {
				t.Fatalf("user after logout = %d, want signed out", id)
			}
		})
	}
}
