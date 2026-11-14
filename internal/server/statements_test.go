package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
)

// TestStatementRoutes detects the Santander statement, previews it, records the lines as they come, lists the import
// with the balance for the holder and undoes it (RF-86 to RF-90).
func TestStatementRoutes(t *testing.T) {
	srv, _ := testServer(t)
	c := newClient(t)
	signIn(t, c, srv, "ana")
	raw, err := os.ReadFile("../importer/testdata/santander-ficticio.csv")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := json.Marshal(string(raw))

	var detected struct{ Format string }
	_ = json.NewDecoder(postJSON(t, c, srv, "/api/imports/detect", fmt.Sprintf(`{"content":%s}`, content)).Body).Decode(&detected)
	if detected.Format != "santander_statement" {
		t.Fatalf("format = %q", detected.Format)
	}
	resp := postJSON(t, c, srv, "/api/accounts", fmt.Sprintf(`{"name":"Conta Ana","personId":%d,"initialBalanceCents":0}`, testUserIDs["ana"]))
	var acc struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&acc)

	resp = postJSON(t, c, srv, "/api/statements/preview", fmt.Sprintf(`{"accountId":%d,"content":%s}`, acc.ID, content))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview = %d", resp.StatusCode)
	}
	var preview struct {
		ClosingCents int64             `json:"closingCents"`
		ClosingOn    string            `json:"closingOn"`
		Lines        []json.RawMessage `json:"lines"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&preview)
	if preview.ClosingCents != 187_650 || len(preview.Lines) != 12 {
		t.Fatalf("preview = %d lines, closing %d", len(preview.Lines), preview.ClosingCents)
	}
	// The preview lines go back as they came, with remember off.
	lines := make([]map[string]any, 0, len(preview.Lines))
	for _, l := range preview.Lines {
		var m map[string]any
		_ = json.Unmarshal(l, &m)
		m["remember"] = false
		lines = append(lines, m)
	}
	body, _ := json.Marshal(map[string]any{"accountId": acc.ID, "fileName": "extrato.csv", "closingCents": preview.ClosingCents, "closingOn": preview.ClosingOn, "lines": lines})
	resp = postJSON(t, c, srv, "/api/statements", string(body))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("commit = %d", resp.StatusCode)
	}
	var res struct {
		ImportID      int64  `json:"importId"`
		Created       int    `json:"created"`
		Skipped       int    `json:"skipped"`
		BalanceCents  *int64 `json:"balanceCents"`
		AdjustToCents *int64 `json:"adjustToCents"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	// No card in the Radar: the card payment stays out; every other line goes in, and the holder reads the balance.
	if res.Created != 11 || res.Skipped != 1 || res.BalanceCents == nil || res.AdjustToCents == nil || *res.AdjustToCents != 187_650 {
		t.Fatalf("result = %+v", res)
	}

	var list []struct {
		ID         int64 `json:"id"`
		EntryCount int   `json:"entryCount"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/statements").Body).Decode(&list)
	if len(list) != 1 || list[0].EntryCount != 11 {
		t.Fatalf("history = %+v", list)
	}
	if r := send(t, c, srv, http.MethodDelete, fmt.Sprintf("/api/statements/%d", res.ImportID), ""); r.StatusCode != http.StatusNoContent {
		t.Fatalf("undo = %d", r.StatusCode)
	}
	if r := postJSON(t, c, srv, "/api/statements/preview", fmt.Sprintf(`{"accountId":%d,"content":"date,title,amount\n"}`, acc.ID)); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("nubank file as a statement = %d, want 400", r.StatusCode)
	}
}
