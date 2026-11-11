package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestReceivableRoutes covers a split purchase, the list by person, the receipt and the charge message over HTTP (RF-24 to RF-30).
func TestReceivableRoutes(t *testing.T) {
	srv, conn := testServer(t)
	c := newClient(t)
	postJSON(t, c, srv, "/api/auth/login", `{"login":"ana"}`)
	var persons []struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/persons").Body).Decode(&persons)
	var acc, card struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(postJSON(t, c, srv, "/api/accounts", fmt.Sprintf(`{"name":"Nubank Ana","personId":%d,"initialBalanceCents":0}`, persons[0].ID)).Body).Decode(&acc)
	_ = json.NewDecoder(postJSON(t, c, srv, "/api/cards", fmt.Sprintf(`{"name":"Nubank","personId":%d,"limitCents":500000,"closingDay":3,"dueDay":10,"color":"purple","finish":"matte"}`, persons[0].ID)).Body).Decode(&card)
	var delivery int64
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Delivery'`).Scan(&delivery)

	resp := postJSON(t, c, srv, fmt.Sprintf("/api/cards/%d/purchases", card.ID), fmt.Sprintf(`{"date":"2026-11-20","merchant":"Ar-condicionado","categoryId":%d,"amountCents":300000,"installments":3,"split":{"mode":"equal","coupleParts":1,"parties":[{"name":"João Ficticio"},{"name":"Maria Ficticia"}]}}`, delivery))
	var p struct {
		CoupleShareCents int64 `json:"coupleShareCents"`
		Splits           []struct {
			PartyName  string `json:"partyName"`
			ShareCents int64  `json:"shareCents"`
		}
	}
	_ = json.NewDecoder(resp.Body).Decode(&p)
	if resp.StatusCode != http.StatusCreated || p.CoupleShareCents != 100_000 || len(p.Splits) != 2 || p.Splits[0].ShareCents != 100_000 {
		t.Fatalf("split purchase = %d %+v", resp.StatusCode, p)
	}

	var people []struct {
		PartyID      int64 `json:"partyId"`
		Name         string
		PendingCents int64 `json:"pendingCents"`
		Items        []struct {
			ID int64 `json:"id"`
		}
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/receivables").Body).Decode(&people)
	if len(people) != 2 || people[0].PendingCents != 100_000 || len(people[0].Items) != 3 {
		t.Fatalf("people = %+v", people)
	}

	resp = postJSON(t, c, srv, "/api/receivables/receipts", fmt.Sprintf(`{"itemIds":[%d],"accountId":%d,"date":"2026-12-10"}`, people[0].Items[0].ID, acc.ID))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("receipt = %d", resp.StatusCode)
	}
	resp = postJSON(t, c, srv, "/api/receivables/receipts", fmt.Sprintf(`{"itemIds":[%d,%d],"accountId":%d,"date":"2026-12-10"}`, people[0].Items[1].ID, people[1].Items[0].ID, acc.ID))
	var e struct{ Code string }
	_ = json.NewDecoder(resp.Body).Decode(&e)
	if resp.StatusCode != http.StatusBadRequest || e.Code != "receivable_mixed" {
		t.Fatalf("mixed people = %d %s", resp.StatusCode, e.Code)
	}

	var msg struct{ Text string }
	_ = json.NewDecoder(get(t, c, fmt.Sprintf("%s/api/receivables/%d/message", srv.URL, people[0].PartyID)).Body).Decode(&msg)
	if !strings.Contains(msg.Text, "Total: R$ 666,66") {
		t.Fatalf("message = %q", msg.Text)
	}
}
