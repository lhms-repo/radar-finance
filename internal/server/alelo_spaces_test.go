package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestAleloAndRecurrenceSpaces creates an Alelo purchase and a card recurrence straight in the personal space,
// refuses a card whose holder is not there, and moves an Alelo purchase only by whoever recorded it (RF-83).
func TestAleloAndRecurrenceSpaces(t *testing.T) {
	srv, conn := testServer(t)
	c := newClient(t)
	signIn(t, c, srv, "ana")
	var delivery int64
	if err := conn.QueryRow(`SELECT id FROM categories WHERE name = 'Delivery'`).Scan(&delivery); err != nil {
		t.Fatal(err)
	}
	created := func(path, body string) int64 {
		t.Helper()
		resp := postJSON(t, c, srv, path, body)
		var x struct {
			ID int64 `json:"id"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&x)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("%s = %d", path, resp.StatusCode)
		}
		return x.ID
	}
	alelo := func(name string, person int64) int64 {
		return created("/api/alelo", fmt.Sprintf(`{"name":%q,"personId":%d,"monthlyCreditCents":80000,"creditDay":5,"initialBalanceCents":0,"style":"tudo"}`, name, person))
	}
	ofAna, ofBruno := alelo("Alelo Ana", testUserIDs["ana"]), alelo("Alelo Bruno", testUserIDs["bruno"])
	card := created("/api/cards", fmt.Sprintf(`{"name":"Nubank","personId":%d,"limitCents":500000,"closingDay":3,"dueDay":10,"color":"purple","finish":"matte"}`, testUserIDs["ana"]))
	today := time.Now().Format("2006-01-02")
	purchase := func(desc string) string {
		return fmt.Sprintf(`{"date":%q,"description":%q,"amountCents":2500}`, today, desc)
	}
	spaceOf := func(table string, id int64) int64 {
		t.Helper()
		var g int64
		if err := conn.QueryRow(`SELECT group_id FROM `+table+` WHERE id = ?`, id).Scan(&g); err != nil {
			t.Fatal(err)
		}
		return g
	}

	tests := []struct {
		name, path, body string
		wantStatus       int
		table            string
	}{
		{name: "alelo purchase in her personal space", path: fmt.Sprintf("/api/alelo/%d/purchases", ofAna), body: purchase("Lanche"), wantStatus: http.StatusCreated, table: "alelo_purchases"},
		{name: "alelo of bruno in her personal space", path: fmt.Sprintf("/api/alelo/%d/purchases", ofBruno), body: purchase("Intruso"), wantStatus: http.StatusNotFound},
		{name: "recurrence in her personal space", path: fmt.Sprintf("/api/cards/%d/recurrences", card), body: fmt.Sprintf(`{"description":"Streaming","categoryId":%d,"amountCents":3990,"startDate":%q}`, delivery, today), wantStatus: http.StatusCreated, table: "recurrences"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := sendIn(t, c, srv.URL, anaSpace, http.MethodPost, tt.path, tt.body)
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if tt.table == "" {
				return
			}
			var x struct {
				ID int64 `json:"id"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&x)
			if g := spaceOf(tt.table, x.ID); g != anaSpace {
				t.Fatalf("%s %d in %d, want %d", tt.table, x.ID, g, anaSpace)
			}
		})
	}

	// An Alelo purchase of Casa moves only by whoever recorded it.
	moving := created(fmt.Sprintf("/api/alelo/%d/purchases", ofAna), purchase("Feira"))
	move := func(cl *http.Client, target int64) int {
		return send(t, cl, srv, http.MethodPost, fmt.Sprintf("/api/spaces/%d/moves", target), fmt.Sprintf(`{"kind":"alelo","itemId":%d}`, moving)).StatusCode
	}
	bruno := newClient(t)
	signIn(t, bruno, srv, "bruno")
	if got := move(bruno, brunoSpace); got != http.StatusForbidden {
		t.Fatalf("move by bruno = %d, want 403", got)
	}
	if got := move(c, anaSpace); got != http.StatusNoContent {
		t.Fatalf("move by ana = %d, want 204", got)
	}
	if g := spaceOf("alelo_purchases", moving); g != anaSpace {
		t.Fatalf("moved purchase in %d, want %d", g, anaSpace)
	}
}
