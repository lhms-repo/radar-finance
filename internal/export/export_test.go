package export

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/scope"
	"github.com/italogfx/radar-finance/internal/testutil"
)

// readZip returns the text of each file of a zip.
func readZip(t *testing.T, b []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(data)
	}
	return out
}

// TestWriteZipScopesTheGroup shows that the export carries only the rows of the active group, the people who
// are members and the personal data of the user, never a PIN hash (RNF-18, RF-82, RN-27).
func TestWriteZipScopesTheGroup(t *testing.T) {
	conn := testutil.OpenDB(t)
	ana := testutil.NewUser(t, conn, "ana", "Ana Ficticia")
	bruno := testutil.NewUser(t, conn, "bruno", "Bruno Ficticio")
	carla := testutil.NewUser(t, conn, "carla", "Carla Ficticia")
	first := testutil.NewGroup(t, conn, "Casa", ana, bruno)
	other := testutil.NewGroup(t, conn, "Outra", carla)
	if _, err := conn.Exec(`UPDATE users SET pin_hash = 'segredo-hash' WHERE id = ?`, ana.ID); err != nil {
		t.Fatal(err)
	}
	var delivery int64
	if err := conn.QueryRow(`SELECT id FROM categories WHERE name = 'Delivery'`).Scan(&delivery); err != nil {
		t.Fatal(err)
	}
	l := ledger.NewService(conn)
	on := civil.Date{Year: 2026, Month: time.November, Day: 11}
	add := func(ctx context.Context, user, holder int64, name, desc string) {
		t.Helper()
		a, err := l.CreateAccount(ctx, user, ledger.NewAccount{Name: name, PersonID: holder, InitialBalanceCents: 10_000})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := l.AddExpense(ctx, user, ledger.NewExpense{AccountID: a.ID, AmountCents: 100, Date: on, CategoryID: delivery, Description: desc}); err != nil {
			t.Fatal(err)
		}
	}
	add(first, ana.ID, ana.PersonID, "Conta da Ana", "gasto-da-casa")
	add(other, carla.ID, carla.PersonID, "Conta da Carla", "gasto-da-outra")

	var buf bytes.Buffer
	if err := WriteZip(auth.WithUserID(first, ana.ID), conn, &buf); err != nil {
		t.Fatal(err)
	}
	files := readZip(t, buf.Bytes())
	for name, want := range map[string]struct{ in, out string }{
		"entries.csv":  {"gasto-da-casa", "gasto-da-outra"},
		"accounts.csv": {"Conta da Ana", "Conta da Carla"},
		"persons.csv":  {"Bruno Ficticio", "Carla Ficticia"},
		"users.csv":    {"bruno", "carla"},
		"groups.csv":   {"Casa", "Outra"},
	} {
		got, ok := files[name]
		if !ok {
			t.Fatalf("%s missing from the export", name)
		}
		if !strings.Contains(got, want.in) || strings.Contains(got, want.out) {
			t.Fatalf("%s = %q, want %q and not %q", name, got, want.in, want.out)
		}
	}
	for name, got := range files {
		if strings.Contains(got, "segredo-hash") || strings.Contains(got, "pin_hash") {
			t.Fatalf("%s carries PIN data", name)
		}
	}
	if _, ok := files["sessions.csv"]; ok {
		t.Fatal("sessions exported")
	}

	// The other group gets its own rows only.
	buf.Reset()
	if err := WriteZip(auth.WithUserID(other, carla.ID), conn, &buf); err != nil {
		t.Fatal(err)
	}
	files = readZip(t, buf.Bytes())
	if got := files["entries.csv"]; !strings.Contains(got, "gasto-da-outra") || strings.Contains(got, "gasto-da-casa") {
		t.Fatalf("entries of the other group = %q", got)
	}
	if err := WriteZip(context.Background(), conn, &buf); !errors.Is(err, scope.ErrNoGroup) {
		t.Fatalf("no group: err = %v", err)
	}
}

// TestEveryTableHasARule keeps a new table from being exported whole by accident.
func TestEveryTableHasARule(t *testing.T) {
	conn := testutil.OpenDB(t)
	rows, err := conn.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if _, ok := rules[name]; !ok && !skippedTables[name] && name != "users" {
			t.Errorf("table %s has no export rule", name)
		}
	}
}
