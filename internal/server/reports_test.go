package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestReportRoutes covers the overview, spending, budgets, search and notifications over HTTP (RF-51 to RF-64).
func TestReportRoutes(t *testing.T) {
	srv, conn := testServer(t)
	c := newClient(t)
	postJSON(t, c, srv, "/api/auth/login", `{"login":"ana"}`)
	var persons []struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/persons").Body).Decode(&persons)
	var acc struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(postJSON(t, c, srv, "/api/accounts", fmt.Sprintf(`{"name":"Nubank Ana","personId":%d,"initialBalanceCents":100000}`, persons[0].ID)).Body).Decode(&acc)
	var delivery, alimentacao int64
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Delivery'`).Scan(&delivery)
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Alimentação'`).Scan(&alimentacao)
	postJSON(t, c, srv, fmt.Sprintf("/api/accounts/%d/incomes", acc.ID), `{"amountCents":500000,"date":"2026-11-05"}`)
	postJSON(t, c, srv, fmt.Sprintf("/api/accounts/%d/expenses", acc.ID), fmt.Sprintf(`{"amountCents":9000,"date":"2026-11-06","categoryId":%d,"description":"Pizza"}`, delivery))

	tests := []struct {
		name, method, path, body string
		wantStatus               int
		wantCode                 string
	}{
		{name: "budget on a top level category", method: http.MethodPut, path: fmt.Sprintf("/api/budgets/%d", alimentacao), body: `{"amountCents":10000}`, wantStatus: http.StatusNoContent},
		{name: "budget on a subcategory", method: http.MethodPut, path: fmt.Sprintf("/api/budgets/%d", delivery), body: `{"amountCents":10000}`, wantStatus: http.StatusBadRequest, wantCode: "budget_top_level"},
		{name: "spending range too long", method: http.MethodGet, path: "/api/reports/spending?from=2024-01&to=2026-11", wantStatus: http.StatusBadRequest, wantCode: "invalid_range"},
		{name: "search with the end before the start", method: http.MethodGet, path: "/api/search?from=2026-11-30&to=2026-11-01", wantStatus: http.StatusBadRequest, wantCode: "invalid_range"},
		{name: "overview", method: http.MethodGet, path: "/api/overview?month=2026-11", wantStatus: http.StatusOK},
		{name: "mark all notifications read", method: http.MethodPost, path: "/api/notifications/read", body: `{}`, wantStatus: http.StatusNoContent},
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

	var budgets []struct {
		Level  string `json:"level"`
		UsedBp int    `json:"usedBp"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/budgets?month=2026-11").Body).Decode(&budgets)
	if len(budgets) != 1 || budgets[0].Level != "warning" || budgets[0].UsedBp != 9_000 {
		t.Fatalf("budgets = %+v", budgets)
	}
	var lines []struct {
		Description string `json:"description"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/search?from=2026-11-01&to=2026-11-30&text=pizza").Body).Decode(&lines)
	if len(lines) != 1 || lines[0].Description != "Pizza" {
		t.Fatalf("search = %+v", lines)
	}
	var ov struct {
		Forecast struct {
			IncomeCents int64 `json:"incomeCents"`
		}
		Commitments []struct{} `json:"commitments"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/overview?month=2026-11").Body).Decode(&ov)
	if ov.Forecast.IncomeCents != 500_000 || len(ov.Commitments) != 6 {
		t.Fatalf("overview = %+v", ov)
	}
	var notes struct {
		Unread int `json:"unread"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/notifications").Body).Decode(&notes)
	if notes.Unread != 0 {
		t.Fatalf("unread after reading all = %d", notes.Unread)
	}
}
