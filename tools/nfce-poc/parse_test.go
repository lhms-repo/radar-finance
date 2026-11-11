package main

import (
	"bytes"
	"net/url"
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

// TestGoiasShellFrameURL finds the receipt iframe in the Goiás shell page.
func TestGoiasShellFrameURL(t *testing.T) {
	page, err := os.ReadFile("testdata/fictional-goias-shell.html")
	if err != nil {
		t.Fatal(err)
	}
	base, _ := url.Parse("https://nfeweb.sefaz.go.gov.br/nfeweb/sites/nfce/danfeNFCe?p=x")
	got, ok := frameURL(page, base)
	want := "https://nfeweb.sefaz.go.gov.br/nfeweb/sites/nfce/render/danfeNFCe?chNFe=00000000000000000000000000000000000000000000"
	if !ok || got != want {
		t.Fatalf("frameURL = %q, %v; want %q", got, ok, want)
	}
	if _, ok := frameURL([]byte("<html></html>"), base); ok {
		t.Fatal("found an iframe in a page without one")
	}
}

// TestGoiasEmbeddedReceipt unwraps the receipt passed to new DanfeNFCe and parses it like the national layout.
func TestGoiasEmbeddedReceipt(t *testing.T) {
	frame, err := os.ReadFile("testdata/fictional-goias-frame.html")
	if err != nil {
		t.Fatal(err)
	}
	inner, err := embeddedReceipt(frame)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := Parse(bytes.NewReader(inner))
	if err != nil {
		t.Fatal(err)
	}
	if rec.StoreName != "MERCADO FICTICIO LTDA" || rec.TotalCents != 3187 || len(rec.Items) != 2 {
		t.Fatalf("receipt = %+v", rec)
	}
	if _, err := embeddedReceipt([]byte("<html>sem script</html>")); err == nil {
		t.Fatal("expected an error without DanfeNFCe")
	}
}

// TestReadJSString decodes the escapes the portal uses in the embedded HTML.
func TestReadJSString(t *testing.T) {
	tests := []struct {
		in, want string
		consumed int
	}{
		{in: `'abc' rest`, want: "abc", consumed: 5},
		{in: `'<\/p>\n\t\"x\" \'y\''`, want: "</p>\n\t\"x\" 'y'", consumed: 22},
		{in: `'caf\u00e9'`, want: "café", consumed: 11},
	}
	for _, tt := range tests {
		got, n, err := readJSString(tt.in)
		if err != nil || got != tt.want || n != tt.consumed {
			t.Errorf("readJSString(%q) = %q, %d, %v; want %q, %d", tt.in, got, n, err, tt.want, tt.consumed)
		}
	}
	if _, _, err := readJSString(`'unterminated`); err == nil {
		t.Error("expected an error for an unterminated string")
	}
}
