package account

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/italogfx/radar-finance/internal/db"
)

// newService opens a fresh migrated database for one test.
func newService(t *testing.T) *Service {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "radar.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	return NewService(conn)
}

// TestCreateAndFind covers user creation and lookup by login (RF-01).
func TestCreateAndFind(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, " Ana ", "Ana Ficticia"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		login   string
		wantErr error
	}{
		{name: "stored in lowercase", login: "ana"},
		{name: "lookup ignores case and spaces", login: " ANA "},
		{name: "unknown login", login: "bruno", wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := s.GetByLogin(ctx, tt.login)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (u.Name != "Ana Ficticia" || u.Login != "ana") {
				t.Fatalf("user = %+v", u)
			}
		})
	}
}

// TestCreateValidation rejects duplicates, empty fields and a third user.
func TestCreateValidation(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, "ana", "Ana Ficticia"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, login, person string
	}{
		{name: "duplicate login, other case", login: "Ana", person: "Outra"},
		{name: "empty login", login: " ", person: "Outra"},
		{name: "empty name", login: "bruno", person: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.Create(ctx, tt.login, tt.person); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	// A failed creation must not leave an orphan person behind (RNF-04).
	var persons int
	if err := s.conn.QueryRow("SELECT count(*) FROM persons").Scan(&persons); err != nil {
		t.Fatal(err)
	}
	if persons != 1 {
		t.Fatalf("persons = %d, want 1", persons)
	}

	if _, err := s.Create(ctx, "bruno", "Bruno Ficticio"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, "carla", "Carla Ficticia"); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("third user: err = %v, want ErrLimitReached", err)
	}
	users, _ := s.List(ctx)
	if len(users) != 2 || users[0].Login != "ana" || users[1].Login != "bruno" {
		t.Fatalf("users = %+v", users)
	}
}

// TestPreferences covers theme and haptics changes.
func TestPreferences(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	u, err := s.Create(ctx, "ana", "Ana Ficticia")
	if err != nil {
		t.Fatal(err)
	}
	dark, off, bad := "dark", false, "purple"
	got, err := s.UpdatePreferences(ctx, u.ID, Preferences{Theme: &dark, Haptics: &off})
	if err != nil || got.Theme != "dark" || got.Haptics {
		t.Fatalf("update = %+v, %v", got, err)
	}
	if _, err := s.UpdatePreferences(ctx, u.ID, Preferences{Theme: &bad}); err == nil {
		t.Fatal("invalid theme accepted")
	}
}
