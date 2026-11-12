package db

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
)

// lastSingleCoupleVersion is the schema of the Radar for one couple, before groups (Fase 8).
const lastSingleCoupleVersion = 19

// seedCouple fills a database at version 19 with fictional data in every table the group migration touches.
func seedCouple(t *testing.T, conn *sql.DB) {
	t.Helper()
	now := "2026-11-10 12:00:00+00:00"
	stmts := []string{
		`INSERT INTO persons (id, name, created_at) VALUES (1, 'Ana Ficticia', '` + now + `'), (2, 'Bruno Ficticio', '` + now + `')`,
		`INSERT INTO users (id, login, person_id, theme, haptics, created_at, updated_at) VALUES (1, 'ana', 1, 'system', 1, '` + now + `', '` + now + `'), (2, 'bruno', 2, 'system', 1, '` + now + `', '` + now + `')`,
		`INSERT INTO accounts (id, name, person_id, initial_balance_cents, active, created_by, created_at, updated_at) VALUES (1, 'Nubank Ana', 1, 100000, 1, 1, '` + now + `', '` + now + `')`,
		`INSERT INTO entries (id, account_id, kind, amount_cents, occurred_on, confirmed, category_id, description, created_by, created_at, updated_at) VALUES
			(1, 1, 'expense', -4590, '2026-11-02', 1, 22, 'Mercado', 1, '` + now + `', '` + now + `'),
			(2, 1, 'income', 450000, '2026-11-05', 1, NULL, 'Salário', 2, '` + now + `', '` + now + `')`,
		`INSERT INTO cards (id, name, person_id, limit_cents, closing_day, due_day, color, finish, active, created_by, created_at, updated_at) VALUES (1, 'Nubank', 1, 500000, 3, 10, 'purple', 'matte', 1, 1, '` + now + `', '` + now + `')`,
		`INSERT INTO purchases (id, card_id, kind, purchased_on, merchant, description, category_id, total_cents, installment_count, first_invoice, created_by, created_at, updated_at) VALUES (1, 1, 'purchase', '2026-11-04', 'Loja', '', 22, 30000, 3, '2026-12', 1, '` + now + `', '` + now + `')`,
		`INSERT INTO installments (purchase_id, card_id, invoice_month, number, amount_cents) VALUES (1, 1, '2026-12', 1, 10000), (1, 1, '2027-01', 2, 10000), (1, 1, '2027-02', 3, 10000)`,
		`INSERT INTO parties (id, name, created_by, created_at) VALUES (1, 'Carla Ficticia', 1, '` + now + `')`,
		`INSERT INTO splits (purchase_id, party_id, share_cents) VALUES (1, 1, 15000)`,
		`INSERT INTO budgets (category_id, amount_cents, created_by, updated_at) VALUES (2, 80000, 1, '` + now + `')`,
		`INSERT INTO markets (id, name, neighborhood, cnpj, active, created_by, created_at) VALUES (1, 'Mercado Ficticio', 'Centro', '12345678000190', 1, 1, '` + now + `')`,
		`INSERT INTO products (id, name, brand, by_weight, size_milli, unit, created_by, created_at) VALUES (1, 'Arroz', 'Marca', 0, 5000, 'kg', 1, '` + now + `')`,
		`INSERT INTO receipts (id, market_id, access_key, issued_on, total_cents, item_count, created_by, created_at) VALUES (1, 1, '52261112345678000190650010000000011000000010', '2026-11-08', 2599, 1, 1, '` + now + `')`,
		`INSERT INTO prices (product_id, market_id, price_cents, observed_on, source, created_by, created_at, receipt_id) VALUES (1, 1, 2599, '2026-11-08', 'receipt', 1, '` + now + `', 1)`,
		`INSERT INTO receipt_links (market_id, item_key, product_id, ignored) VALUES (1, 'arroz marca 5kg', 1, 0)`,
		`UPDATE market_settings SET max_markets = 3, min_savings_cents = 500, stale_days = 20 WHERE id = 1`,
		`INSERT INTO shopping_lists (id, name, status, created_by, created_at, paid_with) VALUES (1, 'Semana', 'open', 1, '` + now + `', '')`,
		`INSERT INTO list_items (list_id, product_id, quantity_milli, picked, created_at) VALUES (1, 1, 2000, 0, '` + now + `')`,
		`INSERT INTO savings_boxes (id, name, target_cents, percent_bp, initial_balance_cents, created_by, created_at, updated_at) VALUES (1, 'Reserva', 1000000, 1000, 0, 1, '` + now + `', '` + now + `')`,
	}
	for _, s := range stmts {
		if _, err := conn.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// migratedTo opens a fresh database migrated up to version.
func migratedTo(t *testing.T, version int64) *sql.DB {
	t.Helper()
	conn, err := Open(filepath.Join(t.TempDir(), "radar.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := MigrateTo(context.Background(), conn, slog.New(slog.NewTextHandler(io.Discard, nil)), version); err != nil {
		t.Fatal(err)
	}
	return conn
}

// scalar runs a query that returns one integer.
func scalar(t *testing.T, conn *sql.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := conn.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// TestMigrateCoupleToGroup turns the couple of a running Radar into its first group, with every row kept,
// the money unchanged and the names that were unique now unique inside each group (Fase 8, RF-78, RF-85).
func TestMigrateCoupleToGroup(t *testing.T) {
	conn := migratedTo(t, lastSingleCoupleVersion)
	seedCouple(t, conn)

	// Rows and money of every touched table, before the migration.
	tables := []string{"entries", "purchases", "installments", "parties", "splits", "budgets", "markets", "products", "receipts", "prices", "receipt_links", "shopping_lists", "list_items", "savings_boxes", "market_settings"}
	before := map[string]int64{}
	for _, table := range tables {
		before[table] = scalar(t, conn, `SELECT count(*) FROM `+table)
	}
	money := []string{
		`SELECT coalesce(sum(amount_cents), 0) FROM entries`,
		`SELECT coalesce(sum(amount_cents), 0) FROM installments`,
		`SELECT coalesce(sum(share_cents), 0) FROM splits`,
		`SELECT coalesce(sum(amount_cents), 0) FROM budgets`,
		`SELECT coalesce(sum(price_cents), 0) FROM prices`,
	}
	moneyBefore := make([]int64, len(money))
	for i, q := range money {
		moneyBefore[i] = scalar(t, conn, q)
	}

	if err := Migrate(context.Background(), conn, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}

	for _, table := range tables {
		if got := scalar(t, conn, `SELECT count(*) FROM `+table); got != before[table] {
			t.Errorf("%s has %d rows, had %d", table, got, before[table])
		}
	}
	for i, q := range money {
		if got := scalar(t, conn, q); got != moneyBefore[i] {
			t.Errorf("%s = %d, was %d", q, got, moneyBefore[i])
		}
	}
	if n := scalar(t, conn, `SELECT count(*) FROM groups WHERE id = 1 AND name = 'Casa'`); n != 1 {
		t.Fatalf("first group missing")
	}
	if n := scalar(t, conn, `SELECT count(*) FROM group_members WHERE group_id = 1 AND role = 'admin' AND weight_bp = 5000`); n != 2 {
		t.Fatalf("members with half the weight each = %d, want 2", n)
	}
	for _, table := range []string{"entries", "purchases", "recurrences", "bills", "financings", "incomes", "alelo_purchases", "savings_boxes", "shopping_lists", "products", "parties", "budgets", "markets"} {
		if n := scalar(t, conn, `SELECT count(*) FROM `+table+` WHERE group_id IS NOT 1`); n != 0 {
			t.Errorf("%s has %d rows outside the first group", table, n)
		}
	}
	if n := scalar(t, conn, `SELECT count(*) FROM market_settings WHERE group_id = 1 AND max_markets = 3 AND min_savings_cents = 500 AND stale_days = 20`); n != 1 {
		t.Fatalf("market settings not carried to the first group")
	}
	rows, err := conn.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign key check found broken references")
	}
	if n := scalar(t, conn, `SELECT count(*) FROM pragma_foreign_keys`); n != 1 {
		t.Fatal("foreign keys left off after the migration")
	}

	// A second group may reuse the names and keys of the first, but not repeat them inside itself.
	exec := func(q string) error { _, err := conn.Exec(q); return err }
	now := "'2026-11-12 12:00:00+00:00'"
	if err := exec(`INSERT INTO groups (id, name, created_at) VALUES (2, 'Casa de praia', ` + now + `)`); err != nil {
		t.Fatal(err)
	}
	unique := []struct{ name, first, again string }{
		{"party", `INSERT INTO parties (group_id, name, created_by, created_at) VALUES (2, 'carla ficticia', 1, ` + now + `)`, `INSERT INTO parties (group_id, name, created_by, created_at) VALUES (2, 'Carla Ficticia', 1, ` + now + `)`},
		{"budget", `INSERT INTO budgets (group_id, category_id, amount_cents, created_by, updated_at) VALUES (2, 2, 100, 1, ` + now + `)`, `INSERT INTO budgets (group_id, category_id, amount_cents, created_by, updated_at) VALUES (2, 2, 200, 1, ` + now + `)`},
		{"market cnpj", `INSERT INTO markets (id, group_id, name, neighborhood, cnpj, active, created_by, created_at) VALUES (2, 2, 'Outro nome', '', '12345678000190', 1, 1, ` + now + `)`, `INSERT INTO markets (group_id, name, neighborhood, cnpj, active, created_by, created_at) VALUES (2, 'Mais um', '', '12345678000190', 1, 1, ` + now + `)`},
		{"market name", `INSERT INTO markets (group_id, name, neighborhood, active, created_by, created_at) VALUES (2, 'Mercado Ficticio', 'Centro', 1, 1, ` + now + `)`, `INSERT INTO markets (group_id, name, neighborhood, active, created_by, created_at) VALUES (2, 'mercado ficticio', 'centro', 1, 1, ` + now + `)`},
		{"product", `INSERT INTO products (group_id, name, brand, by_weight, size_milli, unit, created_by, created_at) VALUES (2, 'Arroz', 'Marca', 0, 5000, 'kg', 1, ` + now + `)`, `INSERT INTO products (group_id, name, brand, by_weight, size_milli, unit, created_by, created_at) VALUES (2, 'arroz', 'marca', 0, 5000, 'kg', 1, ` + now + `)`},
		{"receipt", `INSERT INTO receipts (market_id, access_key, issued_on, total_cents, item_count, created_by, created_at) VALUES (2, '52261112345678000190650010000000011000000010', '2026-11-08', 2599, 1, 1, ` + now + `)`, `INSERT INTO receipts (market_id, access_key, issued_on, total_cents, item_count, created_by, created_at) VALUES (2, '52261112345678000190650010000000011000000010', '2026-11-08', 2599, 1, 1, ` + now + `)`},
		{"market settings", `INSERT INTO market_settings (group_id, max_markets, min_savings_cents, stale_days) VALUES (2, 2, 1000, 30)`, `INSERT INTO market_settings (group_id, max_markets, min_savings_cents, stale_days) VALUES (2, 2, 1000, 30)`},
	}
	for _, u := range unique {
		if err := exec(u.first); err != nil {
			t.Errorf("%s in a second group: %v", u.name, err)
		}
		if err := exec(u.again); err == nil {
			t.Errorf("%s repeated inside the second group was accepted", u.name)
		}
	}
}

// TestMigrateEmptyDatabase creates no group on a new installation; the first one comes from the terminal (RF-80).
func TestMigrateEmptyDatabase(t *testing.T) {
	conn := migratedTo(t, 0)
	if n := scalar(t, conn, `SELECT count(*) FROM groups`); n != 0 {
		t.Fatalf("groups = %d, want none", n)
	}
	// The default settings row stays, without a group, for the code that still reads id 1.
	if n := scalar(t, conn, `SELECT count(*) FROM market_settings WHERE id = 1 AND group_id IS NULL`); n != 1 {
		t.Fatalf("default market settings row = %d, want 1 without a group", n)
	}
}
