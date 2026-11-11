package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestAleloRoutes covers the Alelo card, credit, purchases and month over HTTP (RF-31 to RF-34).
func TestAleloRoutes(t *testing.T) {
	srv, _ := testServer(t)
	c := newClient(t)
	postJSON(t, c, srv, "/api/auth/login", `{"login":"ana"}`)
	var persons []struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/persons").Body).Decode(&persons)
	resp := postJSON(t, c, srv, "/api/alelo", fmt.Sprintf(`{"name":"Alelo Ana","personId":%d,"monthlyCreditCents":80000,"creditDay":5,"initialBalanceCents":12000}`, persons[0].ID))
	var card struct {
		ID         int64  `json:"id"`
		StartMonth string `json:"startMonth"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&card)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d", resp.StatusCode)
	}
	m := card.StartMonth

	tests := []struct {
		name, method, path, body string
		wantStatus               int
		wantCode                 string
	}{
		{name: "confirm credit", method: http.MethodPost, path: fmt.Sprintf("/api/alelo/%d/months/%s/credit", card.ID, m), body: `{}`, wantStatus: http.StatusCreated},
		{name: "confirm twice", method: http.MethodPost, path: fmt.Sprintf("/api/alelo/%d/months/%s/credit", card.ID, m), body: `{}`, wantStatus: http.StatusConflict, wantCode: "alelo_credit_confirmed"},
		{name: "purchase", method: http.MethodPost, path: fmt.Sprintf("/api/alelo/%d/purchases", card.ID), body: fmt.Sprintf(`{"date":"%s-10","description":"Mercado","amountCents":30000}`, m), wantStatus: http.StatusCreated},
		{name: "purchase without description", method: http.MethodPost, path: fmt.Sprintf("/api/alelo/%d/purchases", card.ID), body: fmt.Sprintf(`{"date":"%s-10","description":" ","amountCents":1}`, m), wantStatus: http.StatusBadRequest, wantCode: "alelo_description_required"},
		{name: "delete with history", method: http.MethodDelete, path: fmt.Sprintf("/api/alelo/%d", card.ID), wantStatus: http.StatusConflict, wantCode: "alelo_in_use"},
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
		OpeningCents int64 `json:"openingCents"`
		CreditCents  int64 `json:"creditCents"`
		SpentCents   int64 `json:"spentCents"`
		ClosingCents int64 `json:"closingCents"`
	}
	_ = json.NewDecoder(get(t, c, fmt.Sprintf("%s/api/alelo/%d/months/%s", srv.URL, card.ID, m)).Body).Decode(&month)
	if month.OpeningCents != 12_000 || month.CreditCents != 80_000 || month.SpentCents != 30_000 || month.ClosingCents != 62_000 {
		t.Fatalf("month = %+v (RN-15)", month)
	}
}
