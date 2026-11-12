package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestMarketRoutes covers markets, products, manual prices, history and settings over HTTP (RF-65 to RF-67, RF-70, RF-74, RF-76).
func TestMarketRoutes(t *testing.T) {
	srv, _ := testServer(t)
	c := newClient(t)
	signIn(t, c, srv, "ana")

	var m struct {
		ID int64 `json:"id"`
	}
	resp := postJSON(t, c, srv, "/api/market/markets", `{"name":"Mercado Ficticio","neighborhood":"Centro"}`)
	_ = json.NewDecoder(resp.Body).Decode(&m)
	if resp.StatusCode != http.StatusCreated || m.ID == 0 {
		t.Fatalf("create market = %d", resp.StatusCode)
	}
	if resp := postJSON(t, c, srv, "/api/market/markets", `{"name":"mercado ficticio","neighborhood":"centro"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate market = %d", resp.StatusCode)
	}

	var p struct {
		ID    int64  `json:"id"`
		Label string `json:"label"`
	}
	resp = postJSON(t, c, srv, "/api/market/products", `{"name":"Refrigerante","brand":"Marca Ficticia","byWeight":false,"sizeMilli":1500,"unit":"l"}`)
	_ = json.NewDecoder(resp.Body).Decode(&p)
	if resp.StatusCode != http.StatusCreated || p.Label != "Refrigerante Marca Ficticia 1,5 L" {
		t.Fatalf("create product = %d %+v", resp.StatusCode, p)
	}
	if resp := postJSON(t, c, srv, "/api/market/products", `{"name":"Arroz","brand":"","byWeight":false,"sizeMilli":5000,"unit":"kg"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("product without brand = %d", resp.StatusCode)
	}

	if resp := postJSON(t, c, srv, "/api/market/prices", fmt.Sprintf(`{"productId":%d,"marketId":%d,"priceCents":899,"date":"2026-10-01"}`, p.ID, m.ID)); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("create price = %d", resp.StatusCode)
	}
	var products []struct {
		Prices []struct {
			MarketID   int64 `json:"marketId"`
			PriceCents int64 `json:"priceCents"`
		} `json:"prices"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/market/products").Body).Decode(&products)
	if len(products) != 1 || len(products[0].Prices) != 1 || products[0].Prices[0].PriceCents != 899 {
		t.Fatalf("products = %+v", products)
	}
	var history []struct {
		ID         int64  `json:"id"`
		MarketName string `json:"marketName"`
		Source     string `json:"source"`
	}
	_ = json.NewDecoder(get(t, c, fmt.Sprintf("%s/api/market/products/%d/prices", srv.URL, p.ID)).Body).Decode(&history)
	if len(history) != 1 || history[0].MarketName != "Mercado Ficticio" || history[0].Source != "manual" {
		t.Fatalf("history = %+v", history)
	}

	if resp := send(t, c, srv, http.MethodDelete, fmt.Sprintf("/api/market/markets/%d", m.ID), ""); resp.StatusCode != http.StatusConflict {
		t.Fatalf("delete market in use = %d", resp.StatusCode)
	}
	if resp := send(t, c, srv, http.MethodPut, fmt.Sprintf("/api/market/prices/%d", history[0].ID), `{"priceCents":949,"date":"2026-10-02"}`); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("correct price = %d", resp.StatusCode)
	}
	if resp := send(t, c, srv, http.MethodPut, "/api/market/settings", `{"maxMarkets":9,"minSavingsCents":0,"staleDays":30}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid settings = %d", resp.StatusCode)
	}
	var cfg struct{ MaxMarkets, StaleDays int }
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/market/settings").Body).Decode(&cfg)
	if cfg.MaxMarkets != 2 || cfg.StaleDays != 30 {
		t.Fatalf("settings = %+v", cfg)
	}
}

// TestReceiptRoutes reads a fictional receipt, records it with a new market and refuses it twice (RF-68, RF-69).
func TestReceiptRoutes(t *testing.T) {
	srv, _ := testServer(t)
	c := newClient(t)
	signIn(t, c, srv, "ana")
	link := "https://nfce.fixture.gov.br/?p=fictional%7C" + strings.Repeat("3", 44) + "%7C11222333000144"

	resp := postJSON(t, c, srv, "/api/market/receipts/preview", fmt.Sprintf(`{"url":%q}`, link))
	var p struct {
		StoreName    string `json:"storeName"`
		Neighborhood string `json:"neighborhood"`
		IssuedOn     string `json:"issuedOn"`
		MarketID     *int64 `json:"marketId"`
		Items        []struct {
			UnitPriceCents int64 `json:"unitPriceCents"`
			QuantityMilli  int64 `json:"quantityMilli"`
		} `json:"items"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&p)
	if resp.StatusCode != http.StatusOK || p.StoreName != "MERCADO FICTICIO LTDA" || p.IssuedOn != "2026-01-05" || p.MarketID != nil || len(p.Items) != 2 || p.Items[1].QuantityMilli != 1_288 {
		t.Fatalf("preview = %d %+v", resp.StatusCode, p)
	}
	if resp := postJSON(t, c, srv, "/api/market/receipts/preview", `{"url":"https://192.168.0.1/admin"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("private address = %d", resp.StatusCode)
	}

	commit := fmt.Sprintf(`{"url":%q,"newMarket":{"name":"Mercado Ficticio","neighborhood":"Bairro Teste"},"items":[
		{"index":0,"newProduct":{"name":"Café","brand":"Marca Ficticia","byWeight":false,"sizeMilli":500000,"unit":"g"}},
		{"index":1,"ignore":true}]}`, link)
	resp = postJSON(t, c, srv, "/api/market/receipts", commit)
	var res struct{ Prices, Ignored int }
	_ = json.NewDecoder(resp.Body).Decode(&res)
	if resp.StatusCode != http.StatusCreated || res.Prices != 1 || res.Ignored != 1 {
		t.Fatalf("commit = %d %+v", resp.StatusCode, res)
	}
	if resp := postJSON(t, c, srv, "/api/market/receipts", commit); resp.StatusCode != http.StatusConflict {
		t.Fatalf("second commit = %d", resp.StatusCode)
	}
	var list []struct {
		ID         int64 `json:"id"`
		PriceCount int64 `json:"priceCount"`
	}
	_ = json.NewDecoder(get(t, c, srv.URL+"/api/market/receipts").Body).Decode(&list)
	if len(list) != 1 || list[0].PriceCount != 1 {
		t.Fatalf("receipts = %+v", list)
	}
	if resp := send(t, c, srv, http.MethodDelete, fmt.Sprintf("/api/market/receipts/%d", list[0].ID), ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d", resp.StatusCode)
	}
}

// TestCompareRoute ranks the markets of a list over HTTP (RF-72, RF-73).
func TestCompareRoute(t *testing.T) {
	srv, _ := testServer(t)
	c := newClient(t)
	signIn(t, c, srv, "ana")
	var m, p, l struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(postJSON(t, c, srv, "/api/market/markets", `{"name":"Mercado Ficticio","neighborhood":""}`).Body).Decode(&m)
	_ = json.NewDecoder(postJSON(t, c, srv, "/api/market/products", `{"name":"Carne","brand":"Sem marca","byWeight":true,"sizeMilli":1000,"unit":"kg"}`).Body).Decode(&p)
	_ = json.NewDecoder(postJSON(t, c, srv, "/api/market/lists", `{"name":"Compra"}`).Body).Decode(&l)
	postJSON(t, c, srv, "/api/market/prices", fmt.Sprintf(`{"productId":%d,"marketId":%d,"priceCents":3990,"date":"2026-10-01"}`, p.ID, m.ID))
	if resp := postJSON(t, c, srv, fmt.Sprintf("/api/market/lists/%d/items", l.ID), fmt.Sprintf(`{"productId":%d,"quantityMilli":535}`, p.ID)); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("add item = %d", resp.StatusCode)
	}
	var cmp struct {
		Ranking []struct {
			TotalCents int64 `json:"totalCents"`
		} `json:"ranking"`
		Split *struct {
			TotalCents   int64  `json:"totalCents"`
			SavingsCents *int64 `json:"savingsCents"`
		} `json:"split"`
	}
	_ = json.NewDecoder(get(t, c, fmt.Sprintf("%s/api/market/lists/%d/comparison", srv.URL, l.ID)).Body).Decode(&cmp)
	if len(cmp.Ranking) != 1 || cmp.Ranking[0].TotalCents != 2_134 || cmp.Split == nil || cmp.Split.TotalCents != 2_134 || cmp.Split.SavingsCents == nil {
		t.Fatalf("comparison = %+v", cmp)
	}
}
