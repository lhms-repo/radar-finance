package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestAccountRoutes covers accounts, expenses, transfers, statement and month flow over HTTP (RF-04 to RF-07, RN-08).
func TestAccountRoutes(t *testing.T) {
	srv, conn := testServer(t)
	c := newClient(t)
	signIn(t, c, srv, "ana")

	var persons []struct {
		ID   int64 `json:"id"`
		Name string
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/persons").Body).Decode(&persons)
	if len(persons) != 2 {
		t.Fatalf("persons = %+v", persons)
	}

	// create returns the id of a new account.
	create := func(name string, initial int64) int64 {
		resp := postJSON(t, c, srv, "/api/accounts", fmt.Sprintf(`{"name":%q,"personId":%d,"initialBalanceCents":%d}`, name, persons[0].ID, initial))
		var a struct {
			ID           int64 `json:"id"`
			BalanceCents int64 `json:"balanceCents"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&a)
		if resp.StatusCode != http.StatusCreated || a.BalanceCents != initial {
			t.Fatalf("create %s = %d %+v", name, resp.StatusCode, a)
		}
		return a.ID
	}
	nubank := create("Nubank Ana", 100_000)
	inter := create("Inter Bruno", 0)
	var delivery int64
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Delivery'`).Scan(&delivery)

	tests := []struct {
		name, method, path, body string
		wantStatus               int
		wantCode                 string
	}{
		{name: "expense", method: http.MethodPost, path: fmt.Sprintf("/api/accounts/%d/expenses", nubank), body: fmt.Sprintf(`{"amountCents":4590,"date":"2026-11-11","categoryId":%d}`, delivery), wantStatus: http.StatusCreated},
		{name: "expense with bad date", method: http.MethodPost, path: fmt.Sprintf("/api/accounts/%d/expenses", nubank), body: fmt.Sprintf(`{"amountCents":1,"date":"11/11/2026","categoryId":%d}`, delivery), wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "transfer", method: http.MethodPost, path: "/api/transfers", body: fmt.Sprintf(`{"fromAccountId":%d,"toAccountId":%d,"amountCents":10000,"date":"2026-11-11"}`, nubank, inter), wantStatus: http.StatusCreated},
		{name: "transfer to same account", method: http.MethodPost, path: "/api/transfers", body: fmt.Sprintf(`{"fromAccountId":%d,"toAccountId":%d,"amountCents":1,"date":"2026-11-11"}`, nubank, nubank), wantStatus: http.StatusBadRequest, wantCode: "transfer_same_account"},
		{name: "adjust to same balance", method: http.MethodPost, path: fmt.Sprintf("/api/accounts/%d/adjustments", inter), body: `{"balanceCents":10000,"date":"2026-11-11"}`, wantStatus: http.StatusBadRequest, wantCode: "adjustment_no_change"},
		{name: "statement with bad month", method: http.MethodGet, path: fmt.Sprintf("/api/accounts/%d/statement?month=2026-13", nubank), wantStatus: http.StatusBadRequest, wantCode: "invalid_month"},
		{name: "statement of missing account", method: http.MethodGet, path: "/api/accounts/999/statement?month=2026-11", wantStatus: http.StatusNotFound, wantCode: "account_not_found"},
		{name: "delete account with entries", method: http.MethodDelete, path: fmt.Sprintf("/api/accounts/%d", nubank), wantStatus: http.StatusConflict, wantCode: "account_in_use"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := send(t, c, srv, tt.method, tt.path, tt.body)
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if tt.wantCode != "" {
				var e struct{ Code string }
				_ = json.NewDecoder(resp.Body).Decode(&e)
				if e.Code != tt.wantCode {
					t.Fatalf("code = %q, want %q", e.Code, tt.wantCode)
				}
			}
		})
	}

	var st struct {
		Full         bool   `json:"full"`
		OpeningCents *int64 `json:"openingCents"`
		ClosingCents *int64 `json:"closingCents"`
		Entries      []struct {
			Kind                   string `json:"kind"`
			AmountCents            int64  `json:"amountCents"`
			Date                   string `json:"date"`
			CounterpartAccountName string `json:"counterpartAccountName"`
			CreatedByName          string `json:"createdByName"`
		}
	}
	// In the group the statement has the movements of the group and no balances (RF-83).
	_ = json.NewDecoder(get(t, c, fmt.Sprintf("%s/api/accounts/%d/statement?month=2026-11", srv.URL, nubank)).Body).Decode(&st)
	if st.Full || st.OpeningCents != nil || st.ClosingCents != nil || len(st.Entries) != 2 {
		t.Fatalf("statement in the group = %+v", st)
	}
	// Ana holds the account and reads it in full in her personal space, id 2.
	if resp := send(t, c, srv, http.MethodPut, "/api/me/group", `{"groupId":2}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("switch to the personal space = %d", resp.StatusCode)
	}
	st.OpeningCents, st.ClosingCents = nil, nil
	_ = json.NewDecoder(get(t, c, fmt.Sprintf("%s/api/accounts/%d/statement?month=2026-11", srv.URL, nubank)).Body).Decode(&st)
	if resp := send(t, c, srv, http.MethodPut, "/api/me/group", `{"groupId":1}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("switch back to the group = %d", resp.StatusCode)
	}
	if !st.Full || st.OpeningCents == nil || *st.OpeningCents != 100_000 || *st.ClosingCents != 85_410 || len(st.Entries) != 2 ||
		st.Entries[0].Kind != "transfer" || st.Entries[0].CounterpartAccountName != "Inter Bruno" || st.Entries[1].Date != "2026-11-11" ||
		st.Entries[1].CreatedByName != "Ana Ficticia" {
		t.Fatalf("statement = %+v", st)
	}

	var flow struct {
		IncomeCents  int64 `json:"incomeCents"`
		ExpenseCents int64 `json:"expenseCents"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/month-flow?month=2026-11").Body).Decode(&flow)
	if flow.IncomeCents != 0 || flow.ExpenseCents != 4590 {
		t.Fatalf("flow = %+v; the transfer must not count (RN-08)", flow)
	}
}
