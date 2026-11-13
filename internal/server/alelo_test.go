package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestAleloRoutes covers the Alelo card, credit, purchases and month over HTTP, with balances and credit only for the
// holder in her personal space (RF-31 to RF-34, RF-83).
func TestAleloRoutes(t *testing.T) {
	srv, _ := testServer(t)
	c := newClient(t)
	signIn(t, c, srv, "ana")
	var persons []struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/persons").Body).Decode(&persons)
	resp := postJSON(t, c, srv, "/api/alelo", fmt.Sprintf(`{"name":"Alelo Ana","personId":%d,"monthlyCreditCents":80000,"creditDay":5,"initialBalanceCents":12000,"style":"tudo"}`, persons[0].ID))
	var card struct {
		ID         int64  `json:"id"`
		StartMonth string `json:"startMonth"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&card)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d", resp.StatusCode)
	}
	m := card.StartMonth

	credit := fmt.Sprintf("/api/alelo/%d/months/%s/credit", card.ID, m)
	tests := []struct {
		name, method, path, body string
		// personal runs the step in the personal space of Ana, the holder, instead of Casa.
		personal   bool
		wantStatus int
		wantCode   string
	}{
		{name: "credit outside the personal space", method: http.MethodPost, path: credit, body: `{}`, wantStatus: http.StatusConflict, wantCode: "alelo_credit_personal"},
		{name: "purchase", method: http.MethodPost, path: fmt.Sprintf("/api/alelo/%d/purchases", card.ID), body: fmt.Sprintf(`{"date":"%s-10","description":"Mercado","amountCents":30000}`, m), wantStatus: http.StatusCreated},
		{name: "purchase without description", method: http.MethodPost, path: fmt.Sprintf("/api/alelo/%d/purchases", card.ID), body: fmt.Sprintf(`{"date":"%s-10","description":" ","amountCents":1}`, m), wantStatus: http.StatusBadRequest, wantCode: "alelo_description_required"},
		{name: "delete with history", method: http.MethodDelete, path: fmt.Sprintf("/api/alelo/%d", card.ID), wantStatus: http.StatusConflict, wantCode: "alelo_in_use"},
		{name: "confirm credit", method: http.MethodPost, path: credit, body: `{}`, personal: true, wantStatus: http.StatusCreated},
		{name: "confirm twice", method: http.MethodPost, path: credit, body: `{}`, personal: true, wantStatus: http.StatusConflict, wantCode: "alelo_credit_confirmed"},
	}
	space := 1
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if want := map[bool]int{false: 1, true: 2}[tt.personal]; want != space {
				if r := send(t, c, srv, http.MethodPut, "/api/me/group", fmt.Sprintf(`{"groupId":%d}`, want)); r.StatusCode != http.StatusOK {
					t.Fatalf("switch to space %d = %d", want, r.StatusCode)
				}
				space = want
			}
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
	type monthView struct {
		Full         bool   `json:"full"`
		OpeningCents *int64 `json:"openingCents"`
		CreditCents  int64  `json:"creditCents"`
		SpentCents   int64  `json:"spentCents"`
		ClosingCents *int64 `json:"closingCents"`
		Purchases    []struct {
			GroupName string `json:"groupName"`
		} `json:"purchases"`
	}
	read := func() monthView {
		var v monthView
		_ = json.NewDecoder(get(t, c, fmt.Sprintf("%s/api/alelo/%d/months/%s", srv.URL, card.ID, m)).Body).Decode(&v)
		return v
	}
	// In her personal space the holder reads every space with the balances (RN-15, RF-83).
	if v := read(); !v.Full || v.OpeningCents == nil || *v.OpeningCents != 12_000 || v.CreditCents != 80_000 || v.SpentCents != 30_000 ||
		v.ClosingCents == nil || *v.ClosingCents != 62_000 || len(v.Purchases) != 1 || v.Purchases[0].GroupName != "Casa" {
		t.Fatalf("month in the personal space = %+v", v)
	}
	if r := send(t, c, srv, http.MethodPut, "/api/me/group", `{"groupId":1}`); r.StatusCode != http.StatusOK {
		t.Fatalf("back to Casa = %d", r.StatusCode)
	}
	// The group reads its purchases without balances or credit.
	if v := read(); v.Full || v.OpeningCents != nil || v.ClosingCents != nil || v.CreditCents != 0 || v.SpentCents != 30_000 {
		t.Fatalf("month in Casa = %+v", v)
	}
	// The balance of the card goes only to the holder.
	balance := func(cl *http.Client) *int64 {
		var cards []struct {
			BalanceCents *int64 `json:"balanceCents"`
			Owned        bool   `json:"owned"`
		}
		_ = json.NewDecoder(get(t, cl, srv.URL+"/api/alelo").Body).Decode(&cards)
		if len(cards) != 1 {
			t.Fatalf("cards = %+v", cards)
		}
		return cards[0].BalanceCents
	}
	if b := balance(c); b == nil || *b != 62_000 {
		t.Fatalf("balance for the holder = %v, want 62000", b)
	}
	bruno := newClient(t)
	signIn(t, bruno, srv, "bruno")
	if b := balance(bruno); b != nil {
		t.Fatalf("balance for another member = %d, want none", *b)
	}
}
