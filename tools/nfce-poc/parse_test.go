package main

import (
	"os"
	"testing"
)

// TestParseFictionalReceipt parses a fictional page in the national portal layout.
func TestParseFictionalReceipt(t *testing.T) {
	f, err := os.Open("testdata/fictional.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rec, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	if rec.StoreName != "MERCADO FICTICIO LTDA" {
		t.Errorf("store = %q", rec.StoreName)
	}
	if rec.StoreCNPJ != "00000000000191" {
		t.Errorf("cnpj = %q", rec.StoreCNPJ)
	}
	if rec.IssuedAt != "05/01/2026 18:42:10" {
		t.Errorf("issuedAt = %q", rec.IssuedAt)
	}
	if rec.TotalCents != 3187 {
		t.Errorf("total = %d", rec.TotalCents)
	}
	want := []Item{
		{Description: "CAFE TRES COR 500G", Code: "7890000000017", Quantity: "1", Unit: "UN", UnitPriceCents: 1899, TotalCents: 1899},
		{Description: "BANANA PRATA KG", Code: "2001", Quantity: "1,288", Unit: "KG", UnitPriceCents: 1000, TotalCents: 1288},
	}
	if len(rec.Items) != len(want) {
		t.Fatalf("items = %d, want %d", len(rec.Items), len(want))
	}
	for i := range want {
		if rec.Items[i] != want[i] {
			t.Errorf("item %d = %+v, want %+v", i, rec.Items[i], want[i])
		}
	}
}

// TestParseCents checks Brazilian amount parsing into integer cents (RN-21).
func TestParseCents(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"18,99", 1899},
		{"1.234,56", 123456},
		{"R$ 7,5", 750},
		{"10", 1000},
		{"0,01", 1},
	}
	for _, tt := range tests {
		got, err := parseCents(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("parseCents(%q) = %d, %v; want %d", tt.in, got, err, tt.want)
		}
	}
}
