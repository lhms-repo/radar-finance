package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestSavingsRoutes covers a box, a deposit, a withdrawal beyond the balance and the month view over HTTP (RF-43 to RF-48).
func TestSavingsRoutes(t *testing.T) {
	srv, _ := testServer(t)
	c := newClient(t)
	signIn(t, c, srv, "ana")
	var persons []struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/persons").Body).Decode(&persons)
	var acc, box struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(postJSON(t, c, srv, "/api/accounts", fmt.Sprintf(`{"name":"Nubank Ana","personId":%d,"initialBalanceCents":1000000}`, persons[0].ID)).Body).Decode(&acc)
	resp := postJSON(t, c, srv, "/api/savings", `{"name":"Reserva","targetCents":3000000,"percentBp":1000}`)
	_ = json.NewDecoder(resp.Body).Decode(&box)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d", resp.StatusCode)
	}
	postJSON(t, c, srv, fmt.Sprintf("/api/accounts/%d/incomes", acc.ID), `{"amountCents":500000,"date":"2026-11-05"}`)

	tests := []struct {
		name, method, path, body string
		wantStatus               int
		wantCode                 string
	}{
		{name: "bad percent", method: http.MethodPost, path: "/api/savings", body: `{"name":"X","targetCents":1,"percentBp":20000}`, wantStatus: http.StatusBadRequest, wantCode: "savings_percent_invalid"},
		{name: "deposit", method: http.MethodPost, path: fmt.Sprintf("/api/savings/%d/deposits", box.ID), body: fmt.Sprintf(`{"month":"2026-11","amountCents":45000,"accountId":%d,"date":"2026-11-10"}`, acc.ID), wantStatus: http.StatusCreated},
		{name: "withdraw too much", method: http.MethodPost, path: fmt.Sprintf("/api/savings/%d/withdrawals", box.ID), body: fmt.Sprintf(`{"amountCents":45001,"accountId":%d,"date":"2026-11-10","reason":"Carro"}`, acc.ID), wantStatus: http.StatusConflict, wantCode: "savings_insufficient"},
		{name: "delete with moves", method: http.MethodDelete, path: fmt.Sprintf("/api/savings/%d", box.ID), wantStatus: http.StatusConflict, wantCode: "savings_in_use"},
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
	var month struct {
		BaseCents int64 `json:"baseCents"`
		Boxes     []struct {
			BalanceCents   int64 `json:"balanceCents"`
			SuggestedCents int64 `json:"suggestedCents"`
			DepositedCents int64 `json:"depositedCents"`
		}
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/savings?month=2026-11").Body).Decode(&month)
	if month.BaseCents != 500_000 || month.Boxes[0].BalanceCents != 45_000 || month.Boxes[0].DepositedCents != 45_000 || month.Boxes[0].SuggestedCents != 0 {
		t.Fatalf("month = %+v", month)
	}
}
