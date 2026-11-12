package market

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/nfce"
)

// fakeFetcher returns fictional receipts by address and counts the downloads.
type fakeFetcher struct {
	receipts map[string]nfce.Receipt
	calls    int
}

// Fetch returns the receipt registered for the address.
func (f *fakeFetcher) Fetch(_ context.Context, url string) (nfce.Receipt, error) {
	f.calls++
	r, ok := f.receipts[url]
	if !ok {
		return nfce.Receipt{}, nfce.ErrNotReceiptURL
	}
	return r, nil
}

// receiptKey builds a fictional 44 digit access key ending in n.
func receiptKey(n string) string {
	k := "52261100000000000191650010000001231000000000"
	return k[:44-len(n)] + n
}

// fictionalReceipt is a receipt of the Mercado Ficticio with coffee, banana by weight, fuel price and a bag.
func fictionalReceipt(key string, day int) nfce.Receipt {
	return nfce.Receipt{
		StoreName: "MERCADO FICTICIO LTDA", StoreCNPJ: "00000000000191", Neighborhood: "BAIRRO TESTE", AccessKey: key,
		IssuedOn: civil.Date{Year: 2026, Month: time.November, Day: day}, TotalCents: 3_217,
		Items: []nfce.Item{
			{Description: "CAFE FICT 500G", Code: "7890000000017", QuantityMilli: 1_000, Unit: "UN", UnitPriceCents: 1_899, TotalCents: 1_899},
			{Description: "BANANA PRATA KG", Code: "2001", QuantityMilli: 1_288, Unit: "KG", UnitPriceCents: 1_000, TotalCents: 1_288},
			{Description: "SACOLA  PLASTICA", QuantityMilli: 1_000, Unit: "UN", UnitPriceCents: 30, TotalCents: 30},
		},
	}
}

// TestReceiptFlow reads a receipt, creates the market with its CNPJ, links lines to products, ignores the bag,
// recognizes everything on the next receipt and refuses the same receipt twice (RF-68, RF-69).
func TestReceiptFlow(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	fetch := &fakeFetcher{receipts: map[string]nfce.Receipt{
		"https://nfce.sefaz.go.gov.br/1": fictionalReceipt(receiptKey("1"), 5),
		"https://nfce.sefaz.go.gov.br/2": fictionalReceipt(receiptKey("2"), 9),
	}}
	f.s.receipts = fetch
	banana, _ := f.s.CreateProduct(ctx, f.userID, ProductInput{Name: "Banana prata", Brand: "Sem marca", ByWeight: true})

	first, err := f.s.PreviewReceipt(ctx, "https://nfce.sefaz.go.gov.br/1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.MarketID != nil || first.StoreName != "MERCADO FICTICIO LTDA" || len(first.Items) != 3 || first.Items[0].ProductID != nil {
		t.Fatalf("first preview = %+v", first)
	}
	res, err := f.s.CommitReceipt(ctx, f.userID, ReceiptCommit{
		URL:       "https://nfce.sefaz.go.gov.br/1",
		NewMarket: &MarketInput{Name: "Mercado Ficticio", Neighborhood: "Bairro Teste"},
		Items: []CommitItem{
			{Index: 0, NewProduct: &ProductInput{Name: "Café", Brand: "Marca Ficticia", SizeMilli: 500_000, Unit: UnitGram}},
			{Index: 1, ProductID: banana.ID},
			{Index: 2, Ignore: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Prices != 2 || res.Ignored != 1 {
		t.Fatalf("result = %+v", res)
	}
	// The download is reused between preview and commit.
	if fetch.calls != 1 {
		t.Fatalf("downloads = %d", fetch.calls)
	}
	latest, _ := f.s.Latest(ctx)
	markets, _ := f.s.Markets(ctx)
	if p := latest[Key{ProductID: banana.ID, MarketID: markets[0].ID}]; p.PriceCents != 1_000 || p.Source != SourceReceipt || p.Date.Day != 5 {
		t.Fatalf("banana price = %+v", p)
	}

	// The next receipt of the same store finds the market by CNPJ and every line by its link (RF-69).
	next, err := f.s.PreviewReceipt(ctx, "https://nfce.sefaz.go.gov.br/2", 0)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		linked  bool
		ignored bool
	}{
		{name: "coffee by code", linked: true},
		{name: "banana by code", linked: true},
		{name: "bag by text, ignored", ignored: true},
	}
	if next.MarketID == nil || *next.MarketID != markets[0].ID {
		t.Fatalf("market = %v", next.MarketID)
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := next.Items[i]
			if (it.ProductID != nil) != tt.linked || it.Ignored != tt.ignored {
				t.Fatalf("item = %+v", it)
			}
		})
	}

	// The same receipt never enters twice.
	if _, err := f.s.CommitReceipt(ctx, f.userID, ReceiptCommit{URL: "https://nfce.sefaz.go.gov.br/1", MarketID: markets[0].ID, Items: []CommitItem{{Index: 2, Ignore: true}}}); !errors.Is(err, ErrReceiptImported) {
		t.Fatalf("second commit: %v", err)
	}
	again, _ := f.s.PreviewReceipt(ctx, "https://nfce.sefaz.go.gov.br/1", 0)
	if again.ImportedID == nil {
		t.Fatal("preview does not tell the receipt was imported")
	}

	// Removing the receipt removes its prices, the links stay learned.
	if err := f.s.DeleteReceipt(ctx, *again.ImportedID); err != nil {
		t.Fatal(err)
	}
	if latest, _ := f.s.Latest(ctx); len(latest) != 0 {
		t.Fatalf("prices after removal = %+v", latest)
	}
	if err := f.s.DeleteReceipt(ctx, 999); !errors.Is(err, ErrReceiptNotFound) {
		t.Fatalf("missing receipt: %v", err)
	}
}

// TestReceiptCommitErrors validates the lines and the market of a commit.
func TestReceiptCommitErrors(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	f.s.receipts = &fakeFetcher{receipts: map[string]nfce.Receipt{"https://nfce.sefaz.go.gov.br/1": fictionalReceipt(receiptKey("1"), 5)}}
	m := f.mustMarket(t, "Mercado Ficticio", "")
	cafe := f.mustProduct(t, "Café", "Marca Ficticia", 500_000, UnitGram)
	tests := []struct {
		name    string
		in      ReceiptCommit
		wantErr error
	}{
		{name: "line out of the receipt", in: ReceiptCommit{MarketID: m.ID, Items: []CommitItem{{Index: 7, ProductID: cafe.ID}}}, wantErr: ErrReceiptLine},
		{name: "same line twice", in: ReceiptCommit{MarketID: m.ID, Items: []CommitItem{{Index: 0, ProductID: cafe.ID}, {Index: 0, Ignore: true}}}, wantErr: ErrReceiptLine},
		{name: "line without decision", in: ReceiptCommit{MarketID: m.ID, Items: []CommitItem{{Index: 0}}}, wantErr: ErrReceiptLine},
		{name: "unknown product", in: ReceiptCommit{MarketID: m.ID, Items: []CommitItem{{Index: 0, ProductID: 999}}}, wantErr: ErrProductNotFound},
		{name: "without market", in: ReceiptCommit{Items: []CommitItem{{Index: 0, ProductID: cafe.ID}}}, wantErr: ErrMarketNotFound},
		{name: "nothing chosen", in: ReceiptCommit{MarketID: m.ID}, wantErr: ErrReceiptEmpty},
		{name: "new product with invalid brand", in: ReceiptCommit{MarketID: m.ID, Items: []CommitItem{{Index: 0, NewProduct: &ProductInput{Name: "X", SizeMilli: 1, Unit: UnitUnit}}}}, wantErr: ErrProductBrand},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.URL = "https://nfce.sefaz.go.gov.br/1"
			if _, err := f.s.CommitReceipt(ctx, f.userID, tt.in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
	// Nothing was saved by the failed commits.
	if latest, _ := f.s.Latest(ctx); len(latest) != 0 {
		t.Fatalf("prices = %+v", latest)
	}
}
