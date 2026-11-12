package auth

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/db"
)

// TestSignOutSurvivesLateCommit keeps a session signed out when a request that loaded it before the
// logout finishes after it; with an idle timeout scs saves the session at the end of every request (RF-01).
func TestSignOutSurvivesLateCommit(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(filepath.Join(t.TempDir(), "radar.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(ctx, conn, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	sm := NewSessionManager(conn, SessionOptions{Idle: time.Hour, Lifetime: 24 * time.Hour})

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
				if _, _, err := sm.Commit(slow); err != nil {
					t.Fatal(err)
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
