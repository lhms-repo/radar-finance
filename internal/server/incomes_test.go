package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestIncomeRoutes covers registering, listing the month, confirming and one off incomes over HTTP (RF-09 to RF-13).
func TestIncomeRoutes(t *testing.T) {
	srv, _ := testServer(t)
	c := newClient(t)
	postJSON(t, c, srv, "/api/auth/login", `{"login":"ana"}`)

	var persons []struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/persons").Body).Decode(&persons)
	resp := postJSON(t, c, srv, "/api/accounts", fmt.Sprintf(`{"name":"Nubank Ana","personId":%d,"initialBalanceCents":0}`, persons[0].ID))
	var acc struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&acc)

	resp = postJSON(t, c, srv, "/api/incomes", fmt.Sprintf(`{"kind":"recurring","name":"Salário","personId":%d,"accountId":%d,"amountCents":450000,"day":5,"startMonth":"2026-11"}`, persons[0].ID, acc.ID))
	var inc struct {
		ID  int64 `json:"id"`
		Day int   `json:"day"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&inc)
	if resp.StatusCode != http.StatusCreated || inc.Day != 5 {
		t.Fatalf("create income = %d %+v", resp.StatusCode, inc)
	}

	tests := []struct {
		name, method, path, body string
		wantStatus               int
		wantCode                 string
	}{
		{name: "unknown kind", method: http.MethodPost, path: "/api/incomes", body: fmt.Sprintf(`{"kind":"weekly","name":"x","personId":%d,"accountId":%d,"amountCents":1}`, persons[0].ID, acc.ID), wantStatus: http.StatusBadRequest, wantCode: "income_kind_invalid"},
		{name: "bad start month", method: http.MethodPost, path: "/api/incomes", body: fmt.Sprintf(`{"kind":"recurring","name":"x","personId":%d,"accountId":%d,"amountCents":1,"day":1,"startMonth":"11/2026"}`, persons[0].ID, acc.ID), wantStatus: http.StatusBadRequest, wantCode: "invalid_month"},
		{name: "confirm", method: http.MethodPost, path: fmt.Sprintf("/api/incomes/%d/confirm", inc.ID), body: `{"month":"2026-11"}`, wantStatus: http.StatusCreated},
		{name: "confirm twice", method: http.MethodPost, path: fmt.Sprintf("/api/incomes/%d/confirm", inc.ID), body: `{"month":"2026-11"}`, wantStatus: http.StatusConflict, wantCode: "income_already_confirmed"},
		{name: "confirm before start", method: http.MethodPost, path: fmt.Sprintf("/api/incomes/%d/confirm", inc.ID), body: `{"month":"2026-10"}`, wantStatus: http.StatusBadRequest, wantCode: "income_no_occurrence"},
		{name: "one off income", method: http.MethodPost, path: fmt.Sprintf("/api/accounts/%d/incomes", acc.ID), body: `{"amountCents":20000,"date":"2026-11-03","description":"Venda"}`, wantStatus: http.StatusCreated},
		{name: "delete income with receipt", method: http.MethodDelete, path: fmt.Sprintf("/api/incomes/%d", inc.ID), wantStatus: http.StatusConflict, wantCode: "income_in_use"},
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
		PendingCents  int64 `json:"pendingCents"`
		ReceivedCents int64 `json:"receivedCents"`
		Occurrences   []struct {
			Confirmed  bool   `json:"confirmed"`
			ReceivedOn string `json:"receivedOn"`
		}
		OneOffs []struct{ Description string }
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/incomes/month?month=2026-11").Body).Decode(&month)
	if month.PendingCents != 0 || month.ReceivedCents != 470_000 || len(month.Occurrences) != 1 || !month.Occurrences[0].Confirmed ||
		month.Occurrences[0].ReceivedOn != "2026-11-05" || len(month.OneOffs) != 1 {
		t.Fatalf("month = %+v", month)
	}
	var a struct {
		BalanceCents int64 `json:"balanceCents"`
	}
	_ = json.NewDecoder(get(t, c, fmt.Sprintf("%s/api/accounts/%d", srv.URL, acc.ID)).Body).Decode(&a)
	if a.BalanceCents != 470_000 {
		t.Fatalf("balance = %d", a.BalanceCents)
	}
}
