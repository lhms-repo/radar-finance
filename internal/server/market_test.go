package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestMarketRoutes covers markets, products, manual prices, history and settings over HTTP (RF-65 to RF-67, RF-70, RF-74, RF-76).
func TestMarketRoutes(t *testing.T) {
	srv, _ := testServer(t)
	c := newClient(t)
	postJSON(t, c, srv, "/api/auth/login", `{"login":"ana"}`)

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
