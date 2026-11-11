package nfce

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/civil"
)

// TestParseFictionalReceipt parses a fictional page in the national portal layout (RF-68).
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
	if rec.StoreName != "MERCADO FICTICIO LTDA" || rec.StoreCNPJ != "00000000000191" || rec.Neighborhood != "BAIRRO TESTE" {
		t.Errorf("store = %q %q %q", rec.StoreName, rec.StoreCNPJ, rec.Neighborhood)
	}
	if rec.IssuedOn != (civil.Date{Year: 2026, Month: time.January, Day: 5}) || rec.TotalCents != 3_187 {
		t.Errorf("issued = %v, total = %d", rec.IssuedOn, rec.TotalCents)
	}
	want := []Item{
		{Description: "CAFE TRES COR 500G", Code: "7890000000017", QuantityMilli: 1_000, Unit: "UN", UnitPriceCents: 1_899, TotalCents: 1_899},
		{Description: "BANANA PRATA KG", Code: "2001", QuantityMilli: 1_288, Unit: "KG", UnitPriceCents: 1_000, TotalCents: 1_288},
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

// TestTruncateCents drops the third decimal of receipt prices instead of rounding (RN-21).
func TestTruncateCents(t *testing.T) {
	tests := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"6,499", 649, true}, {"6,49", 649, true}, {"6,4", 640, true}, {"6", 600, true}, {"1.234,567", 123_456, true},
		{"R$ 18,99", 1_899, true}, {"0,009", 0, true}, {"", 0, false}, {"abc", 0, false}, {"6,4999", 0, false},
	}
	for _, tt := range tests {
		got, err := TruncateCents(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("TruncateCents(%q) = %d, %v", tt.in, got, err)
		}
	}
}

// TestQuantityMilli reads receipt quantities in thousandths, weights included.
func TestQuantityMilli(t *testing.T) {
	tests := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"1", 1_000, true}, {"2", 2_000, true}, {"0,535", 535, true}, {"1,5", 1_500, true}, {"1.000", 1_000_000, true},
		{"", 0, false}, {"x", 0, false}, {"1,2345", 0, false}, {"0", 0, false},
	}
	for _, tt := range tests {
		got, err := quantityMilli(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("quantityMilli(%q) = %d, %v", tt.in, got, err)
		}
	}
}

// TestNeighborhood takes the part before the city and the state in the store address.
func TestNeighborhood(t *testing.T) {
	tests := []struct{ in, want string }{
		{"RUA DE EXEMPLO, 100, BAIRRO TESTE, GOIANIA, GO", "BAIRRO TESTE"},
		{"AV FICTICIA, 10, , SETOR FICTICIO, GOIANIA, GO", "SETOR FICTICIO"},
		{"GOIANIA, GO", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := neighborhood(tt.in); got != tt.want {
			t.Errorf("neighborhood(%q) = %q, want %q", tt.in, got, tt.want)
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
	if rec.StoreName != "MERCADO FICTICIO LTDA" || rec.TotalCents != 3_187 || len(rec.Items) != 2 {
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

// TestCheckURL only lets the server fetch government receipt pages, never other hosts of the network.
func TestCheckURL(t *testing.T) {
	tests := []struct {
		in string
		ok bool
	}{
		{"https://nfeweb.sefaz.go.gov.br/nfeweb/sites/nfce/danfeNFCe?p=123|3|1", true},
		{"http://www.nfce.fazenda.sp.gov.br/qrcode?p=1", true},
		{"nfeweb.sefaz.go.gov.br/nfeweb/sites/nfce/danfeNFCe?p=123|3|1", true},
		{"  HTTPS://NFEWEB.SEFAZ.GO.GOV.BR/nfeweb/sites/nfce/danfeNFCe?p=123  ", true},
		{"https://192.168.0.10/admin", false},
		{"00020126580014br.gov.bcb.pix0136", false},
		{"https://sefaz.go.gov.br.example.com/x", false},
		{"ftp://nfeweb.sefaz.go.gov.br/x", false},
		{"not a url", false},
	}
	for _, tt := range tests {
		if _, err := CheckURL(tt.in); (err == nil) != tt.ok {
			t.Errorf("CheckURL(%q) = %v", tt.in, err)
		}
	}
}

// TestFetchGoias follows the shell page to the iframe with the same cookie and reads the embedded receipt.
func TestFetchGoias(t *testing.T) {
	shell, _ := os.ReadFile("testdata/fictional-goias-shell.html")
	frame, _ := os.ReadFile("testdata/fictional-goias-frame.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/render/") {
			if c, err := r.Cookie("JSESSIONID"); err != nil || c.Value != "s1" {
				http.Error(w, "no session", http.StatusForbidden)
				return
			}
			_, _ = w.Write(frame)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "s1", Path: "/"})
		_, _ = w.Write(shell)
	}))
	defer srv.Close()
	c := NewClient()
	// The test server stands in for the portal, whose iframe address is relative.
	c.allowHost = func(string) bool { return true }
	rec, err := c.Fetch(context.Background(), srv.URL+"/nfeweb/sites/nfce/danfeNFCe?p=x")
	if err != nil {
		t.Fatal(err)
	}
	if rec.StoreName != "MERCADO FICTICIO LTDA" || len(rec.Items) != 2 {
		t.Fatalf("receipt = %+v", rec)
	}
	if _, err := NewClient().Fetch(context.Background(), "https://192.168.0.10/x"); !errors.Is(err, ErrNotReceiptURL) {
		t.Fatalf("private host: %v", err)
	}
}

// fixtureURL builds a fixture address.
func fixtureURL(name, key, cnpj string) string {
	return "https://nfce.fixture.gov.br/?p=" + url.QueryEscape(name+"|"+key+"|"+cnpj)
}

// TestFixtureClient reads a fictional receipt from a folder with the access key and CNPJ of the address.
func TestFixtureClient(t *testing.T) {
	c := FixtureClient{Dir: "testdata"}
	key := strings.Repeat("1", 44)
	rec, err := c.Fetch(context.Background(), fixtureURL("fictional", key, "11222333000144"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.AccessKey != key || rec.StoreCNPJ != "11222333000144" || len(rec.Items) != 2 {
		t.Fatalf("receipt = %+v", rec)
	}
	for _, bad := range []string{fixtureURL("../secret", key, ""), fixtureURL("missing", key, ""), "https://10.0.0.1/?p=fictional"} {
		if _, err := c.Fetch(context.Background(), bad); !errors.Is(err, ErrNotReceiptURL) {
			t.Errorf("Fetch(%q) = %v", bad, err)
		}
	}
}
