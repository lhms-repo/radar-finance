package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/config"
)

// Personal spaces of the test server: Casa is created with Ana first, so hers is 2 and the one of Bruno 3.
const (
	anaSpace   = 2
	brunoSpace = 3
)

// sendIn sends a write like send, creating its item in the space given by the X-Radar-Space header.
func sendIn(t *testing.T, c *http.Client, srvURL string, space int64, method, path, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, srvURL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srvURL)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("X-Radar-Space", strconv.FormatInt(space, 10))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// TestSpaceRoutes creates an expense in the personal space from the group, moves another one there, and refuses
// the personal space of someone else in both ways (RF-83).
func TestSpaceRoutes(t *testing.T) {
	srv, conn := testServer(t)
	c := newClient(t)
	signIn(t, c, srv, "ana")
	var delivery int64
	if err := conn.QueryRow(`SELECT id FROM categories WHERE name = 'Delivery'`).Scan(&delivery); err != nil {
		t.Fatal(err)
	}
	resp := postJSON(t, c, srv, "/api/accounts", fmt.Sprintf(`{"name":"Nubank Ana","personId":%d,"initialBalanceCents":100000}`, testUserIDs["ana"]))
	var acc struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&acc)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("account = %d", resp.StatusCode)
	}
	today := time.Now().Format("2006-01-02")
	expense := func(desc string) string {
		return fmt.Sprintf(`{"amountCents":1000,"date":%q,"categoryId":%d,"description":%q}`, today, delivery, desc)
	}
	path := fmt.Sprintf("/api/accounts/%d/expenses", acc.ID)

	if resp := sendIn(t, c, srv.URL, anaSpace, http.MethodPost, path, expense("Presente pessoal")); resp.StatusCode != http.StatusCreated {
		t.Fatalf("expense in her personal space = %d", resp.StatusCode)
	}
	if resp := sendIn(t, c, srv.URL, brunoSpace, http.MethodPost, path, expense("Intruso")); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expense in the personal space of bruno = %d, want 403", resp.StatusCode)
	}
	resp = postJSON(t, c, srv, path, expense("Para mover"))
	var moved struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&moved)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expense in the group = %d", resp.StatusCode)
	}
	move := func(target int64) int {
		return send(t, c, srv, http.MethodPost, fmt.Sprintf("/api/spaces/%d/moves", target), fmt.Sprintf(`{"kind":"entry","itemId":%d}`, moved.ID)).StatusCode
	}
	if got := move(brunoSpace); got != http.StatusForbidden {
		t.Fatalf("move to the personal space of bruno = %d, want 403", got)
	}
	if got := move(anaSpace); got != http.StatusNoContent {
		t.Fatalf("move to her personal space = %d, want 204", got)
	}

	// statement returns the descriptions of the month in the active space.
	statement := func() []string {
		var st struct {
			Entries []struct{ Description string }
		}
		_ = json.NewDecoder(get(t, c, fmt.Sprintf("%s/api/accounts/%d/statement?month=%s", srv.URL, acc.ID, today[:7])).Body).Decode(&st)
		out := []string{}
		for _, e := range st.Entries {
			out = append(out, e.Description)
		}
		return out
	}
	if got := statement(); len(got) != 0 {
		t.Fatalf("group statement = %v, want nothing of the personal space", got)
	}
	if resp := send(t, c, srv, http.MethodPut, "/api/me/group", fmt.Sprintf(`{"groupId":%d}`, anaSpace)); resp.StatusCode != http.StatusOK {
		t.Fatalf("switch to her personal space = %d", resp.StatusCode)
	}
	if got := strings.Join(statement(), ","); !strings.Contains(got, "Presente pessoal") || !strings.Contains(got, "Para mover") {
		t.Fatalf("personal statement = %s", got)
	}
}

// TestPersonalSpaceStaysPrivate puts marked rows in the personal space of Bruno, on his own account and card, and
// calls every GET route as Ana, in the group and in her personal space: nothing marked comes back (RF-83).
func TestPersonalSpaceStaysPrivate(t *testing.T) {
	srv, conn := testServer(t)
	loc, _ := time.LoadLocation(config.Location)
	now := time.Now().In(loc)
	month, day := now.Format("2006-01"), now.Format("2006-01-02")
	seedSecrets(t, conn, month, day)
	// The rows go to the personal space of Bruno, on accounts and cards he holds, with neutral names on what
	// the group sees by design (RF-82).
	moves := []string{
		`UPDATE accounts SET person_id = 2, name = 'Conta B' WHERE id = 901`,
		`UPDATE cards SET person_id = 2, name = 'Cartao B' WHERE id = 901`,
		`UPDATE alelo_cards SET person_id = 2, name = 'Alelo B' WHERE id = 901`,
		`UPDATE incomes SET person_id = 2`,
		`UPDATE alias_rules SET created_by = 2`,
		`UPDATE imports SET created_by = 2`,
	}
	for _, table := range []string{"entries", "purchases", "recurrences", "bills", "financings", "incomes", "alelo_purchases", "savings_boxes", "shopping_lists", "products", "imports", "parties", "budgets", "markets"} {
		moves = append(moves, fmt.Sprintf(`UPDATE %s SET group_id = %d WHERE group_id = 1`, table, brunoSpace))
	}
	for _, m := range moves {
		if _, err := conn.Exec(m); err != nil {
			t.Fatalf("%s: %v", m, err)
		}
	}

	ana, bruno := newClient(t), newClient(t)
	signIn(t, ana, srv, "ana")
	signIn(t, bruno, srv, "bruno")
	if resp := send(t, bruno, srv, http.MethodPut, "/api/me/group", fmt.Sprintf(`{"groupId":%d}`, brunoSpace)); resp.StatusCode != http.StatusOK {
		t.Fatalf("bruno to his personal space = %d", resp.StatusCode)
	}
	query := fmt.Sprintf("month=%s&from=%s-01-01&to=%s-12-31&text=secreto", month, now.Format("2006"), now.Format("2006"))
	// read returns the body of a GET in lower case, with the files of the export opened.
	read := func(c *http.Client, path string) string {
		resp := get(t, c, srv.URL+"/api"+path+"?"+query)
		body, _ := io.ReadAll(resp.Body)
		return strings.ToLower(unzipped(t, body))
	}
	mark := strings.ToLower(secretMark)
	seen := 0
	for _, space := range []int64{1, anaSpace} {
		if resp := send(t, ana, srv, http.MethodPut, "/api/me/group", fmt.Sprintf(`{"groupId":%d}`, space)); resp.StatusCode != http.StatusOK {
			t.Fatalf("ana to space %d = %d", space, resp.StatusCode)
		}
		for _, route := range getRoutes(t) {
			if route == "/events" {
				continue
			}
			for _, id := range []string{"901", "1"} {
				p := strings.NewReplacer("{id}", id, "{month}", month).Replace(route)
				if body := read(ana, p); strings.Contains(body, mark) {
					t.Errorf("GET %s by ana in space %d shows the personal space of bruno: %.300s", p, space, body)
				}
				if space == 1 && strings.Contains(read(bruno, p), mark) {
					seen++
				}
			}
		}
	}
	// Bruno does see his rows, so the check above is not empty.
	if seen < 10 {
		t.Fatalf("bruno saw his marked rows on only %d calls", seen)
	}
}
