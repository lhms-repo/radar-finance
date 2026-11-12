package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestFinancingAndAgendaRoutes covers financings, their payment, the correction and the agenda over HTTP (RF-38 to RF-42).
func TestFinancingAndAgendaRoutes(t *testing.T) {
	srv, conn := testServer(t)
	c := newClient(t)
	signIn(t, c, srv, "ana")
	var persons []struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/persons").Body).Decode(&persons)
	var acc struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(postJSON(t, c, srv, "/api/accounts", fmt.Sprintf(`{"name":"Caixa Ana","personId":%d,"initialBalanceCents":1000000}`, persons[0].ID)).Body).Decode(&acc)
	var casa int64
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Financiamento da casa'`).Scan(&casa)

	resp := postJSON(t, c, srv, "/api/financings", fmt.Sprintf(`{"name":"Casa","categoryId":%d,"accountId":%d,"dueDay":20,"installmentCents":150000,"totalInstallments":360,"paidInstallments":24,"balanceCents":28000000,"nextMonth":"2026-11"}`, casa, acc.ID))
	var f struct {
		ID        int64  `json:"id"`
		NextMonth string `json:"nextMonth"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&f)
	if resp.StatusCode != http.StatusCreated || f.NextMonth != "2026-11" {
		t.Fatalf("create = %d %+v", resp.StatusCode, f)
	}

	tests := []struct {
		name, method, path, body string
		wantStatus               int
		wantCode                 string
	}{
		{name: "pay with balance", method: http.MethodPost, path: fmt.Sprintf("/api/financings/%d/months/2026-11/payment", f.ID), body: `{"date":"2026-11-20","balanceAfterCents":27860000}`, wantStatus: http.StatusCreated},
		{name: "pay before the next month", method: http.MethodPost, path: fmt.Sprintf("/api/financings/%d/months/2026-10/payment", f.ID), body: `{}`, wantStatus: http.StatusBadRequest, wantCode: "financing_no_occurrence"},
		{name: "correction", method: http.MethodPost, path: fmt.Sprintf("/api/financings/%d/corrections", f.ID), body: `{"balanceCents":27800000,"installmentCents":149000}`, wantStatus: http.StatusOK},
		{name: "negative correction", method: http.MethodPost, path: fmt.Sprintf("/api/financings/%d/corrections", f.ID), body: `{"balanceCents":-1,"installmentCents":1}`, wantStatus: http.StatusBadRequest, wantCode: "financing_balance_invalid"},
		{name: "delete with payments", method: http.MethodDelete, path: fmt.Sprintf("/api/financings/%d", f.ID), wantStatus: http.StatusConflict, wantCode: "financing_in_use"},
		{name: "agenda with bad month", method: http.MethodGet, path: "/api/agenda?month=nov", wantStatus: http.StatusBadRequest, wantCode: "invalid_month"},
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

	var agenda struct {
		TotalCents int64 `json:"totalCents"`
		PaidCount  int   `json:"paidCount"`
		Items      []struct {
			Kind                  string `json:"kind"`
			Number                int    `json:"number"`
			SuggestedBalanceCents int64  `json:"suggestedBalanceCents"`
		}
		Days []struct{ Day, Count, PaidCount int }
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/agenda?month=2026-12").Body).Decode(&agenda)
	if len(agenda.Items) != 1 || agenda.Items[0].Kind != "financing" || agenda.Items[0].Number != 26 || agenda.Items[0].SuggestedBalanceCents != 27_800_000-149_000 ||
		agenda.TotalCents != 149_000 || len(agenda.Days) != 1 || agenda.Days[0].Day != 20 {
		t.Fatalf("agenda = %+v", agenda)
	}
}
