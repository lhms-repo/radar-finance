package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/config"
)

// TestCardRoutes covers cards, purchases, invoices, payment, recurrences and forecast over HTTP (RF-14 to RF-23).
func TestCardRoutes(t *testing.T) {
	srv, conn := testServer(t)
	c := newClient(t)
	signIn(t, c, srv, "ana")
	loc, _ := time.LoadLocation(config.Location)
	today := civil.Today(loc)
	past := today.AddDays(-75)

	var persons []struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/persons").Body).Decode(&persons)
	var acc struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(postJSON(t, c, srv, "/api/accounts", fmt.Sprintf(`{"name":"Nubank Ana","personId":%d,"initialBalanceCents":100000}`, persons[0].ID)).Body).Decode(&acc)
	var delivery, streaming int64
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Delivery'`).Scan(&delivery)
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Streaming'`).Scan(&streaming)

	resp := postJSON(t, c, srv, "/api/cards", fmt.Sprintf(`{"name":"Nubank","personId":%d,"limitCents":500000,"closingDay":3,"dueDay":10,"lastDigits":"1234","color":"purple","finish":"matte"}`, persons[0].ID))
	var card struct {
		ID                  int64  `json:"id"`
		CurrentInvoice      string `json:"currentInvoice"`
		CurrentInvoiceDueOn string `json:"currentInvoiceDueOn"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&card)
	if resp.StatusCode != http.StatusCreated || card.CurrentInvoice == "" || card.CurrentInvoiceDueOn == "" {
		t.Fatalf("create card = %d %+v", resp.StatusCode, card)
	}

	resp = postJSON(t, c, srv, fmt.Sprintf("/api/cards/%d/purchases", card.ID), fmt.Sprintf(`{"date":%q,"merchant":"Loja Ficticia","categoryId":%d,"amountCents":10000,"installments":3}`, past, delivery))
	var purchase struct {
		FirstInvoice string `json:"firstInvoice"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&purchase)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("purchase = %d", resp.StatusCode)
	}

	var inv struct {
		Status     string `json:"status"`
		TotalCents int64  `json:"totalCents"`
		NetCents   int64  `json:"netCents"`
		Items      []struct {
			Number, Count int
			AmountCents   int64 `json:"amountCents"`
		}
	}
	_ = json.NewDecoder(get(t, c, fmt.Sprintf("%s/api/cards/%d/invoices/%s", srv.URL, card.ID, purchase.FirstInvoice)).Body).Decode(&inv)
	if inv.TotalCents != 3_334 || inv.NetCents != 3_334 || len(inv.Items) != 1 || inv.Items[0].Count != 3 {
		t.Fatalf("first invoice = %+v (RN-03)", inv)
	}

	tests := []struct {
		name, method, path, body string
		wantStatus               int
		wantCode                 string
	}{
		{name: "bad month in path", method: http.MethodGet, path: fmt.Sprintf("/api/cards/%d/invoices/2026-13", card.ID), wantStatus: http.StatusBadRequest, wantCode: "invalid_month"},
		{name: "forecast too long", method: http.MethodGet, path: fmt.Sprintf("/api/cards/%d/forecast?count=30", card.ID), wantStatus: http.StatusBadRequest, wantCode: "invalid_count"},
		{name: "pay open invoice", method: http.MethodPost, path: fmt.Sprintf("/api/cards/%d/invoices/%s/payment", card.ID, card.CurrentInvoice), body: fmt.Sprintf(`{"accountId":%d,"date":%q}`, acc.ID, today), wantStatus: http.StatusConflict, wantCode: "invoice_open"},
		{name: "pay closed invoice", method: http.MethodPost, path: fmt.Sprintf("/api/cards/%d/invoices/%s/payment", card.ID, purchase.FirstInvoice), body: fmt.Sprintf(`{"accountId":%d,"date":%q}`, acc.ID, today), wantStatus: http.StatusCreated},
		{name: "recurrence", method: http.MethodPost, path: fmt.Sprintf("/api/cards/%d/recurrences", card.ID), body: fmt.Sprintf(`{"description":"Streaming","categoryId":%d,"amountCents":2190,"startDate":%q}`, streaming, today), wantStatus: http.StatusCreated},
		{name: "credit", method: http.MethodPost, path: fmt.Sprintf("/api/cards/%d/credits", card.ID), body: fmt.Sprintf(`{"date":%q,"merchant":"Estorno","amountCents":500}`, today), wantStatus: http.StatusCreated},
		{name: "invalid color", method: http.MethodPost, path: "/api/cards", body: fmt.Sprintf(`{"name":"X","personId":%d,"limitCents":1,"closingDay":1,"dueDay":8,"color":"pink","finish":"matte"}`, persons[0].ID), wantStatus: http.StatusBadRequest, wantCode: "card_look_invalid"},
		{name: "delete card in use", method: http.MethodDelete, path: fmt.Sprintf("/api/cards/%d", card.ID), wantStatus: http.StatusConflict, wantCode: "card_in_use"},
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

	var a struct {
		BalanceCents int64 `json:"balanceCents"`
	}
	_ = json.NewDecoder(get(t, c, fmt.Sprintf("%s/api/accounts/%d", srv.URL, acc.ID)).Body).Decode(&a)
	if a.BalanceCents != 100_000-3_334 {
		t.Fatalf("balance after payment = %d", a.BalanceCents)
	}
	var forecast []struct {
		Month      string `json:"month"`
		TotalCents int64  `json:"totalCents"`
	}
	_ = json.NewDecoder(get(t, c, fmt.Sprintf("%s/api/cards/%d/forecast?count=3", srv.URL, card.ID)).Body).Decode(&forecast)
	if len(forecast) != 3 || forecast[0].Month != card.CurrentInvoice {
		t.Fatalf("forecast = %+v", forecast)
	}
}
