package server

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/config"
)

// secretMark is in every name the first group writes, so any response of another group that shows it leaked.
const secretMark = "Secreto"

// seedSecrets writes rows marked with secretMark in group 1 across the tables, dated in the current month.
func seedSecrets(t *testing.T, conn *sql.DB, month, day string) {
	t.Helper()
	now := "'2026-11-10 12:00:00+00:00'"
	m, d := "'"+month+"'", "'"+day+"'"
	stmts := []string{
		`INSERT INTO accounts (id, name, person_id, initial_balance_cents, active, created_by, created_at, updated_at) VALUES (901, 'Conta Secreto', 1, 100000, 1, 1, ` + now + `, ` + now + `)`,
		`INSERT INTO entries (account_id, kind, amount_cents, occurred_on, confirmed, category_id, description, created_by, created_at, updated_at, group_id) VALUES (901, 'expense', -4590, ` + d + `, 1, 22, 'Gasto Secreto', 1, ` + now + `, ` + now + `, 1)`,
		`INSERT INTO cards (id, name, person_id, limit_cents, closing_day, due_day, color, finish, active, created_by, created_at, updated_at) VALUES (901, 'Cartao Secreto', 1, 500000, 3, 10, 'purple', 'matte', 1, 1, ` + now + `, ` + now + `)`,
		`INSERT INTO purchases (id, card_id, kind, purchased_on, merchant, description, category_id, total_cents, installment_count, first_invoice, created_by, created_at, updated_at, group_id) VALUES (901, 901, 'purchase', ` + d + `, 'Loja Secreto', '', 22, 30000, 1, ` + m + `, 1, ` + now + `, ` + now + `, 1)`,
		`INSERT INTO installments (purchase_id, card_id, invoice_month, number, amount_cents) VALUES (901, 901, ` + m + `, 1, 30000)`,
		`INSERT INTO parties (id, group_id, name, created_by, created_at) VALUES (901, 1, 'Pessoa Secreto', 1, ` + now + `)`,
		`INSERT INTO splits (purchase_id, party_id, share_cents) VALUES (901, 901, 10000)`,
		`INSERT INTO bills (name, category_id, account_id, due_day, expected_cents, start_month, created_by, created_at, updated_at, group_id) VALUES ('Conta Fixa Secreto', 16, 901, 15, 18000, ` + m + `, 1, ` + now + `, ` + now + `, 1)`,
		`INSERT INTO financings (name, category_id, account_id, due_day, installment_cents, total_installments, initial_paid, initial_balance_cents, start_month, created_by, created_at, updated_at, group_id) VALUES ('Financiamento Secreto', 14, 901, 20, 150000, 360, 24, 30000000, ` + m + `, 1, ` + now + `, ` + now + `, 1)`,
		`INSERT INTO incomes (kind, name, person_id, account_id, amount_cents, day, start_month, created_by, created_at, updated_at, group_id) VALUES ('recurring', 'Renda Secreto', 1, 901, 450000, 5, ` + m + `, 1, ` + now + `, ` + now + `, 1)`,
		`INSERT INTO alelo_cards (id, name, person_id, monthly_credit_cents, credit_day, initial_balance_cents, start_month, active, created_by, created_at, updated_at) VALUES (901, 'Alelo Secreto', 1, 80000, 1, 0, ` + m + `, 1, 1, ` + now + `, ` + now + `)`,
		`INSERT INTO alelo_purchases (alelo_id, purchased_on, description, amount_cents, category_id, created_by, created_at, group_id) VALUES (901, ` + d + `, 'Feira Secreto', 5000, 22, 1, ` + now + `, 1)`,
		`INSERT INTO savings_boxes (name, target_cents, percent_bp, initial_balance_cents, created_by, created_at, updated_at, group_id) VALUES ('Caixinha Secreto', 1000000, 1000, 0, 1, ` + now + `, ` + now + `, 1)`,
		`INSERT INTO budgets (group_id, category_id, amount_cents, created_by, updated_at) VALUES (1, 2, 80000, 1, ` + now + `)`,
		`INSERT INTO markets (id, group_id, name, neighborhood, cnpj, active, created_by, created_at) VALUES (901, 1, 'Mercado Secreto', 'Bairro Secreto', '12345678000190', 1, 1, ` + now + `)`,
		`INSERT INTO products (id, group_id, name, brand, by_weight, size_milli, unit, created_by, created_at) VALUES (901, 1, 'Produto Secreto', 'Marca Secreto', 0, 1000, 'kg', 1, ` + now + `)`,
		`INSERT INTO receipts (id, market_id, access_key, issued_on, total_cents, item_count, created_by, created_at) VALUES (901, 901, '52261112345678000190650010000000011000000010', ` + d + `, 2599, 1, 1, ` + now + `)`,
		`INSERT INTO prices (product_id, market_id, price_cents, observed_on, source, created_by, created_at, receipt_id) VALUES (901, 901, 2599, ` + d + `, 'receipt', 1, ` + now + `, 901)`,
		`INSERT INTO receipt_links (market_id, item_key, product_id, ignored) VALUES (901, 'item secreto', 901, 0)`,
		`INSERT INTO shopping_lists (id, name, status, created_by, created_at, paid_with, group_id) VALUES (901, 'Lista Secreto', 'open', 1, ` + now + `, '', 1)`,
		`INSERT INTO list_items (list_id, product_id, quantity_milli, picked, created_at) VALUES (901, 901, 2000, 0, ` + now + `)`,
		`INSERT INTO alias_rules (original, friendly_name, category_id, created_by, created_at) VALUES ('original secreto', 'Apelido Secreto', 22, 1, ` + now + `)`,
		`INSERT INTO imports (card_id, invoice_month, file_name, line_count, created_by, created_at) VALUES (901, ` + m + `, 'fatura-secreto.csv', 1, 1, ` + now + `)`,
	}
	for _, s := range stmts {
		if _, err := conn.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// getRoutes reads every GET route of the contract, so a new route is checked without touching this test.
func getRoutes(t *testing.T) []string {
	t.Helper()
	f, err := os.Open("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	pathLine := regexp.MustCompile(`^  (/[^:]+):\s*$`)
	var routes []string
	current := ""
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		if m := pathLine.FindStringSubmatch(lines.Text()); m != nil {
			current = m[1]
			continue
		}
		if current != "" && lines.Text() == "    get:" {
			routes = append(routes, current)
		}
		if strings.HasPrefix(lines.Text(), "components:") {
			break
		}
	}
	if len(routes) < 30 {
		t.Fatalf("only %d GET routes read from the contract", len(routes))
	}
	return routes
}

// TestNoDataCrossesGroups calls every GET route of the API as a member of another group, with ids of the
// first group, and checks that nothing marked by the first group comes back (RF-02, RF-85).
func TestNoDataCrossesGroups(t *testing.T) {
	srv, conn := testServer(t)
	loc, _ := time.LoadLocation(config.Location)
	today := time.Now().In(loc)
	month, day := today.Format("2006-01"), today.Format("2006-01-02")
	seedSecrets(t, conn, month, day)

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
	outsider, ana := newClient(t), newClient(t)
	if resp := postJSON(t, outsider, srv, "/api/auth/login", fmt.Sprintf(`{"groupId":%d,"userId":%d,"pin":"9264"}`, beach.ID, carla.ID)); resp.StatusCode != http.StatusOK {
		t.Fatalf("carla login = %d", resp.StatusCode)
	}
	signIn(t, ana, srv, "ana")

	query := fmt.Sprintf("month=%s&from=%s-01-01&to=%s-12-31&text=secreto", month, today.Format("2006"), today.Format("2006"))
	// read returns the body of a GET, skipping the live stream, which never ends.
	read := func(c *http.Client, path string) (int, string) {
		resp := get(t, c, srv.URL+"/api"+path+"?"+query)
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	seen, routes := 0, getRoutes(t)
	for _, route := range routes {
		if route == "/events" {
			continue
		}
		for _, id := range []string{"901", "1"} {
			path := strings.NewReplacer("{id}", id, "{month}", month).Replace(route)
			status, body := read(outsider, path)
			if strings.Contains(strings.ToLower(body), strings.ToLower(secretMark)) {
				t.Errorf("GET %s from another group answered %d with data of the first group: %.300s", path, status, body)
			}
			if _, own := read(ana, path); strings.Contains(strings.ToLower(own), strings.ToLower(secretMark)) {
				seen++
			}
		}
	}
	t.Logf("%d GET routes checked from another group; the first group saw its marked rows on %d calls", len(routes), seen)
	// The first group does see its rows, so the check above is not empty.
	if seen < 10 {
		t.Fatalf("the first group saw its own marked rows on only %d routes; the seed is not reaching the API", seen)
	}
}
