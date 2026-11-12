package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestBillRoutes covers registering, real amount, month view and payment of fixed bills over HTTP (RF-35 to RF-37).
func TestBillRoutes(t *testing.T) {
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
	_ = json.NewDecoder(postJSON(t, c, srv, "/api/accounts", fmt.Sprintf(`{"name":"Nubank Ana","personId":%d,"initialBalanceCents":100000}`, persons[0].ID)).Body).Decode(&acc)
	var energia int64
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Energia'`).Scan(&energia)

	resp := postJSON(t, c, srv, "/api/bills", fmt.Sprintf(`{"name":"Energia","categoryId":%d,"accountId":%d,"dueDay":15,"expectedCents":18000,"startMonth":"2026-11"}`, energia, acc.ID))
	var b struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&b)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create bill = %d", resp.StatusCode)
	}

	tests := []struct {
		name, method, path, body string
		wantStatus               int
		wantCode                 string
	}{
		{name: "bad start month", method: http.MethodPost, path: "/api/bills", body: fmt.Sprintf(`{"name":"x","categoryId":%d,"accountId":%d,"dueDay":1,"expectedCents":1,"startMonth":"nov"}`, energia, acc.ID), wantStatus: http.StatusBadRequest, wantCode: "invalid_month"},
		{name: "real amount", method: http.MethodPut, path: fmt.Sprintf("/api/bills/%d/months/2026-11/actual", b.ID), body: `{"actualCents":17450}`, wantStatus: http.StatusNoContent},
		{name: "real amount before start", method: http.MethodPut, path: fmt.Sprintf("/api/bills/%d/months/2026-10/actual", b.ID), body: `{"actualCents":1}`, wantStatus: http.StatusBadRequest, wantCode: "bill_no_occurrence"},
		{name: "pay", method: http.MethodPost, path: fmt.Sprintf("/api/bills/%d/months/2026-11/payment", b.ID), body: `{"date":"2026-11-14"}`, wantStatus: http.StatusCreated},
		{name: "pay twice", method: http.MethodPost, path: fmt.Sprintf("/api/bills/%d/months/2026-11/payment", b.ID), body: `{}`, wantStatus: http.StatusConflict, wantCode: "bill_already_paid"},
		{name: "delete paid bill", method: http.MethodDelete, path: fmt.Sprintf("/api/bills/%d", b.ID), wantStatus: http.StatusConflict, wantCode: "bill_in_use"},
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
		TotalCents int64 `json:"totalCents"`
		PaidCount  int   `json:"paidCount"`
		Bills      []struct {
			ExpectedCents int64  `json:"expectedCents"`
			ActualCents   int64  `json:"actualCents"`
			Paid          bool   `json:"paid"`
			PaidOn        string `json:"paidOn"`
		}
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/bills/month?month=2026-11").Body).Decode(&month)
	if month.TotalCents != 17_450 || month.PaidCount != 1 || month.Bills[0].ExpectedCents != 18_000 || month.Bills[0].ActualCents != 17_450 || month.Bills[0].PaidOn != "2026-11-14" {
		t.Fatalf("month = %+v", month)
	}
}
