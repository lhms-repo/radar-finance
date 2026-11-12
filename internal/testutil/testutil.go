// Package testutil holds the helpers the tests of the services share.
package testutil

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/scope"
)

// OpenDB opens a fresh migrated database that is closed when the test ends.
func OpenDB(t testing.TB) *sql.DB {
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

// NewGroup creates a group whose first user is the administrator and the others are members,
// and returns a context that acts on it (RF-78, RF-80).
func NewGroup(t testing.TB, conn *sql.DB, name string, users ...account.User) context.Context {
	t.Helper()
	ctx := context.Background()
	accounts := account.NewService(conn)
	g, err := accounts.CreateGroup(ctx, name, users[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users[1:] {
		if err := accounts.AddMember(ctx, g.ID, u.ID, account.RoleMember); err != nil {
			t.Fatal(err)
		}
	}
	return scope.WithGroup(ctx, g.ID)
}

// NewUser creates a fictional user with its person.
func NewUser(t testing.TB, conn *sql.DB, login, name string) account.User {
	t.Helper()
	u, err := account.NewService(conn).Create(context.Background(), login, name)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// FirstGroup creates the group "Casa" like NewGroup and checks that it got id 1, which the services' tests
// rely on to use one shared context for the group of a fresh database.
func FirstGroup(t testing.TB, conn *sql.DB, users ...account.User) context.Context {
	t.Helper()
	ctx := NewGroup(t, conn, "Casa", users...)
	if id, _ := scope.Group(ctx); id != 1 {
		t.Fatalf("first group id = %d, want 1", id)
	}
	return ctx
}
