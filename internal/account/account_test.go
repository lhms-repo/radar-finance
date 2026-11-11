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
	s, err := NewService(conn)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestCreateAndAuthenticate covers user creation and login checks (RF-01).
func TestCreateAndAuthenticate(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, "ana", "Ana Ficticia", "senha-ficticia-1"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		login    string
		password string
		wantErr  error
	}{
		{name: "correct", login: "ana", password: "senha-ficticia-1"},
		{name: "login ignores case and spaces", login: " ANA ", password: "senha-ficticia-1"},
		{name: "wrong password", login: "ana", password: "outra-senha", wantErr: ErrInvalidCredentials},
		{name: "unknown login", login: "bruno", password: "senha-ficticia-1", wantErr: ErrInvalidCredentials},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := s.Authenticate(ctx, tt.login, tt.password)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && u.Name != "Ana Ficticia" {
				t.Fatalf("name = %q", u.Name)
			}
		})
	}
}

// TestCreateValidation rejects duplicates, empty fields and short passwords.
func TestCreateValidation(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, "ana", "Ana Ficticia", "senha-ficticia-1"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, login, person, password string
	}{
		{name: "duplicate login, other case", login: "Ana", person: "Outra", password: "senha-ficticia-2"},
		{name: "empty login", login: " ", person: "Outra", password: "senha-ficticia-2"},
		{name: "empty name", login: "bruno", person: "", password: "senha-ficticia-2"},
		{name: "short password", login: "bruno", person: "Bruno", password: "curta"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.Create(ctx, tt.login, tt.person, tt.password); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	// A failed creation must not leave an orphan person behind (RNF-04).
	users, _ := s.List(ctx)
	var persons int
	if err := s.conn.QueryRow("SELECT count(*) FROM persons").Scan(&persons); err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || persons != 1 {
		t.Fatalf("users = %d, persons = %d, want 1 and 1", len(users), persons)
	}
}

// TestPreferencesAndPassword covers theme, haptics and password change.
func TestPreferencesAndPassword(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	u, err := s.Create(ctx, "ana", "Ana Ficticia", "senha-ficticia-1")
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
	if err := s.SetPassword(ctx, "ana", "nova-senha-ficticia"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, "ana", "nova-senha-ficticia"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPassword(ctx, "ninguem", "nova-senha-ficticia"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
