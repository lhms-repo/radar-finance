package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestImportRoutes covers preview, commit, history, undo and rules over HTTP (RF-53 to RF-57).
func TestImportRoutes(t *testing.T) {
	srv, conn := testServer(t)
	c := newClient(t)
	postJSON(t, c, srv, "/api/auth/login", `{"login":"ana"}`)
	var persons []struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/persons").Body).Decode(&persons)
	var card struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(postJSON(t, c, srv, "/api/cards", fmt.Sprintf(`{"name":"Nubank","personId":%d,"limitCents":500000,"closingDay":3,"dueDay":10,"color":"purple","finish":"matte"}`, persons[0].ID)).Body).Decode(&card)
	var delivery int64
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Delivery'`).Scan(&delivery)
	csv := `date,title,amount\n2026-11-05,IFOOD *PIZZA,52.00\n2026-11-06,Loja - Parcela 2/3,100.00\n2026-11-15,Pagamento recebido,-900.00\n`

	resp := postJSON(t, c, srv, "/api/imports/preview", fmt.Sprintf(`{"cardId":%d,"invoiceMonth":"2026-12","content":%q}`, card.ID, strings.ReplaceAll(csv, `\n`, "\n")))
	var preview []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&preview)
	if resp.StatusCode != http.StatusOK || len(preview) != 3 || preview[2]["skip"] != true || preview[1]["installment"] != float64(2) {
		t.Fatalf("preview = %d %+v", resp.StatusCode, preview)
	}

	resp = postJSON(t, c, srv, "/api/imports/preview", fmt.Sprintf(`{"cardId":%d,"invoiceMonth":"2026-12","content":"date,title,amount\n2026-13-01,X,1.00\n"}`, card.ID))
	var e struct{ Code, Message string }
	_ = json.NewDecoder(resp.Body).Decode(&e)
	if resp.StatusCode != http.StatusBadRequest || e.Code != "import_line_invalid" || !strings.Contains(e.Message, "Linha 2") {
		t.Fatalf("bad line = %d %+v", resp.StatusCode, e)
	}

	lines := fmt.Sprintf(`[
		{"date":"2026-11-05","title":"IFOOD *PIZZA","merchant":"IFOOD *PIZZA","amountCents":5200,"kind":"purchase","friendlyName":"iFood","categoryId":%d,"skip":false,"remember":true},
		{"date":"2026-11-06","title":"Loja - Parcela 2/3","merchant":"Loja","amountCents":10000,"kind":"purchase","installment":2,"installments":3,"friendlyName":"Loja","categoryId":%d,"skip":false,"remember":false},
		{"date":"2026-11-15","title":"Pagamento recebido","merchant":"Pagamento recebido","amountCents":90000,"kind":"payment","friendlyName":"Pagamento recebido","skip":true,"remember":false}]`, delivery, delivery)
	resp = postJSON(t, c, srv, "/api/imports", fmt.Sprintf(`{"cardId":%d,"invoiceMonth":"2026-12","fileName":"Nubank.csv","lines":%s}`, card.ID, lines))
	var res struct {
		ImportID   int64 `json:"importId"`
		Created    int   `json:"created"`
		RulesSaved int   `json:"rulesSaved"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	if resp.StatusCode != http.StatusCreated || res.Created != 2 || res.RulesSaved != 1 {
		t.Fatalf("commit = %d %+v", resp.StatusCode, res)
	}
	var rules []struct{ FriendlyName string }
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/alias-rules").Body).Decode(&rules)
	if len(rules) != 1 || rules[0].FriendlyName != "iFood" {
		t.Fatalf("rules = %+v", rules)
	}
	if resp := send(t, c, srv, http.MethodDelete, fmt.Sprintf("/api/imports/%d", res.ImportID), ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("undo = %d", resp.StatusCode)
	}
}
