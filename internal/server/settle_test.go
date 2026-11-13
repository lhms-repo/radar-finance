package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestSettlementRoutes divides an expense by value between Ana and Bruno, reads the balances and the suggestion,
// records the payment that closes them, reads the division back and changes the weights (RF-79, RF-84, RN-28 to RN-30).
func TestSettlementRoutes(t *testing.T) {
	srv, conn := testServer(t)
	c := newClient(t)
	signIn(t, c, srv, "ana")
	var delivery int64
	if err := conn.QueryRow(`SELECT id FROM categories WHERE name = 'Delivery'`).Scan(&delivery); err != nil {
		t.Fatal(err)
	}
	// The settlement of the test group started before today.
	if _, err := conn.Exec(`UPDATE groups SET settles_from = '2026-01-01' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`UPDATE group_weights SET from_day = '2026-01-01' WHERE group_id = 1`); err != nil {
		t.Fatal(err)
	}
	account := func(name string, person int64) int64 {
		resp := postJSON(t, c, srv, "/api/accounts", fmt.Sprintf(`{"name":%q,"personId":%d,"initialBalanceCents":100000}`, name, person))
		var a struct {
			ID int64 `json:"id"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&a)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("account %s = %d", name, resp.StatusCode)
		}
		return a.ID
	}
	ana, bruno := account("Nubank Ana", testUserIDs["ana"]), account("Inter Bruno", testUserIDs["bruno"])
	today := time.Now().Format("2006-01-02")
	resp := postJSON(t, c, srv, fmt.Sprintf("/api/accounts/%d/expenses", ana), fmt.Sprintf(
		`{"amountCents":10000,"date":%q,"categoryId":%d,"memberSplit":{"mode":"custom","members":[{"personId":%d,"shareCents":7000},{"personId":%d,"shareCents":3000}]}}`,
		today, delivery, testUserIDs["ana"], testUserIDs["bruno"]))
	var entry struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&entry)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expense = %d", resp.StatusCode)
	}
	if resp := postJSON(t, c, srv, fmt.Sprintf("/api/accounts/%d/expenses", ana), fmt.Sprintf(`{"amountCents":10000,"date":%q,"categoryId":%d,"memberSplit":{"mode":"custom","members":[{"personId":%d,"shareCents":9000}]}}`, today, delivery, testUserIDs["ana"])); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("values that do not add up = %d, want 400", resp.StatusCode)
	}

	type settlement struct {
		Members []struct {
			PersonID     int64 `json:"personId"`
			BalanceCents int64 `json:"balanceCents"`
		}
		Suggestions []struct {
			FromPersonID int64 `json:"fromPersonId"`
			AmountCents  int64 `json:"amountCents"`
		}
		Lines []struct{ Description string }
	}
	read := func() settlement {
		var st settlement
		r := get(t, c, srv.URL+"/api/settlement?month="+today[:7])
		_ = json.NewDecoder(r.Body).Decode(&st)
		if r.StatusCode != http.StatusOK {
			t.Fatalf("settlement = %d", r.StatusCode)
		}
		return st
	}
	st := read()
	if len(st.Members) != 2 || st.Members[0].BalanceCents != 3000 || st.Members[1].BalanceCents != -3000 || len(st.Lines) != 1 {
		t.Fatalf("settlement = %+v", st)
	}
	if len(st.Suggestions) != 1 || st.Suggestions[0].FromPersonID != testUserIDs["bruno"] || st.Suggestions[0].AmountCents != 3000 {
		t.Fatalf("suggestions = %+v", st.Suggestions)
	}
	if resp := postJSON(t, c, srv, "/api/settlement/payments", fmt.Sprintf(`{"fromAccountId":%d,"toAccountId":%d,"amountCents":3000,"date":%q}`, bruno, ana, today)); resp.StatusCode != http.StatusCreated {
		t.Fatalf("payment = %d", resp.StatusCode)
	}
	if st := read(); st.Members[0].BalanceCents != 0 || st.Members[1].BalanceCents != 0 || len(st.Suggestions) != 0 {
		t.Fatalf("after the payment = %+v", st)
	}

	var split struct {
		Mode    string
		Members []struct {
			PersonID   int64 `json:"personId"`
			ShareCents int64 `json:"shareCents"`
		}
	}
	_ = json.NewDecoder(get(t, c, fmt.Sprintf("%s/api/member-splits/entry/%d", srv.URL, entry.ID)).Body).Decode(&split)
	if split.Mode != "custom" || len(split.Members) != 2 {
		t.Fatalf("division read back = %+v", split)
	}
	if r := get(t, c, srv.URL+"/api/member-splits/entry/999"); r.StatusCode != http.StatusNotFound {
		t.Fatalf("division of a missing entry = %d, want 404", r.StatusCode)
	}

	weights := fmt.Sprintf(`{"weights":[{"userId":%d,"weightBp":6000},{"userId":%d,"weightBp":4000}]}`, testUserIDs["ana"], testUserIDs["bruno"])
	if resp := send(t, c, srv, http.MethodPut, "/api/group/weights", weights); resp.StatusCode != http.StatusOK {
		t.Fatalf("weights by the administrator = %d", resp.StatusCode)
	}
	other := newClient(t)
	signIn(t, other, srv, "bruno")
	if resp := send(t, other, srv, http.MethodPut, "/api/group/weights", weights); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("weights by a member = %d, want 403", resp.StatusCode)
	}
	if resp := send(t, c, srv, http.MethodPut, "/api/me/group", `{"groupId":2}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("switch to her personal space = %d", resp.StatusCode)
	}
	if r := get(t, c, srv.URL+"/api/settlement?month="+today[:7]); r.StatusCode != http.StatusConflict {
		t.Fatalf("settlement of the personal space = %d, want 409", r.StatusCode)
	}
}
