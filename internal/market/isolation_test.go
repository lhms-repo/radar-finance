package market

import (
	"context"
	"errors"
	"testing"

	"github.com/italogfx/radar-finance/internal/nfce"
	"github.com/italogfx/radar-finance/internal/scope"
	"github.com/italogfx/radar-finance/internal/testutil"
)

// TestGroupIsolation shows that a second group sees none of the markets, products, prices, lists, receipts and
// settings of the first, and that the same receipt can be read by both (RF-65 to RF-77, RF-82, RF-85, RN-27).
func TestGroupIsolation(t *testing.T) {
	f := newFixture(t)
	f.s.receipts = &fakeFetcher{receipts: map[string]nfce.Receipt{"https://nfce.sefaz.go.gov.br/1": fictionalReceipt(receiptKey("1"), 5)}}
	m := f.mustMarket(t, "Mercado Ficticio", "Centro")
	p := f.mustProduct(t, "Café", "Marca Ficticia", 500_000, UnitGram)
	priceID, err := f.s.AddPrice(groupCtx, f.userID, PriceInput{ProductID: p.ID, MarketID: m.ID, PriceCents: 1_899, Date: day(5)})
	if err != nil {
		t.Fatal(err)
	}
	l, err := f.s.CreateList(groupCtx, f.userID, "Compra")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.AddItem(groupCtx, l.ID, p.ID, 1_000); err != nil {
		t.Fatal(err)
	}
	detail, err := f.s.ListDetail(groupCtx, l.ID)
	if err != nil || len(detail.Items) != 1 {
		t.Fatalf("detail = %+v, %v", detail, err)
	}
	itemID := detail.Items[0].ID
	commit := ReceiptCommit{
		URL: "https://nfce.sefaz.go.gov.br/1", NewMarket: &MarketInput{Name: "Mercado do CNPJ", Neighborhood: "Bairro Teste"},
		Items: []CommitItem{{Index: 0, ProductID: p.ID}, {Index: 1, Ignore: true}, {Index: 2, Ignore: true}},
	}
	if _, err := f.s.CommitReceipt(groupCtx, f.userID, commit); err != nil {
		t.Fatal(err)
	}
	receipts, err := f.s.Receipts(groupCtx)
	if err != nil || len(receipts) != 1 {
		t.Fatalf("receipts = %v, %v", receipts, err)
	}
	custom := Settings{MaxMarkets: 3, MinSavingsCents: 500, StaleDays: 10}
	if err := f.s.UpdateSettings(groupCtx, custom); err != nil {
		t.Fatal(err)
	}

	carla := testutil.NewUser(t, f.conn, "carla", "Carla Ficticia")
	other := testutil.NewGroup(t, f.conn, "Outra", carla)

	tests := []struct {
		name    string
		run     func() error
		wantErr error
	}{
		{name: "update market", run: func() error {
			_, err := f.s.UpdateMarket(other, m.ID, MarketInput{Name: "X", Active: true})
			return err
		}, wantErr: ErrMarketNotFound},
		{name: "delete market", run: func() error { return f.s.DeleteMarket(other, m.ID) }, wantErr: ErrMarketNotFound},
		{name: "update product", run: func() error {
			_, err := f.s.UpdateProduct(other, p.ID, ProductInput{Name: "X", Brand: "Y", SizeMilli: 1_000, Unit: UnitUnit})
			return err
		}, wantErr: ErrProductNotFound},
		{name: "delete product", run: func() error { return f.s.DeleteProduct(other, p.ID) }, wantErr: ErrProductNotFound},
		{name: "history", run: func() error { _, err := f.s.History(other, p.ID); return err }, wantErr: ErrProductNotFound},
		{name: "update price", run: func() error { return f.s.UpdatePrice(other, priceID, PriceChanges{PriceCents: 1, Date: day(5)}) }, wantErr: ErrPriceNotFound},
		{name: "delete price", run: func() error { return f.s.DeletePrice(other, priceID) }, wantErr: ErrPriceNotFound},
		{name: "price of another group", run: func() error {
			_, err := f.s.AddPrice(other, carla.ID, PriceInput{ProductID: p.ID, MarketID: m.ID, PriceCents: 100, Date: day(5)})
			return err
		}, wantErr: ErrProductNotFound},
		{name: "list detail", run: func() error { _, err := f.s.ListDetail(other, l.ID); return err }, wantErr: ErrListNotFound},
		{name: "rename list", run: func() error { return f.s.RenameList(other, l.ID, "X") }, wantErr: ErrListNotFound},
		{name: "delete list", run: func() error { return f.s.DeleteList(other, l.ID) }, wantErr: ErrListNotFound},
		{name: "add item", run: func() error { return f.s.AddItem(other, l.ID, p.ID, 1_000) }, wantErr: ErrListNotFound},
		{name: "update item", run: func() error { return f.s.UpdateItem(other, itemID, 2_000) }, wantErr: ErrListItemNotFound},
		{name: "pick item", run: func() error { return f.s.SetPicked(other, carla.ID, itemID, true) }, wantErr: ErrListItemNotFound},
		{name: "remove item", run: func() error { return f.s.RemoveItem(other, itemID) }, wantErr: ErrListItemNotFound},
		{name: "compare", run: func() error { _, err := f.s.Compare(other, l.ID); return err }, wantErr: ErrListNotFound},
		{name: "finish", run: func() error {
			return f.s.FinishList(other, carla.ID, l.ID, Finish{Method: PayAlelo, AleloID: 1, TotalCents: 100, Date: day(10), Description: "X"})
		}, wantErr: ErrListNotFound},
		{name: "delete receipt", run: func() error { return f.s.DeleteReceipt(other, receipts[0].ID) }, wantErr: ErrReceiptNotFound},
		{name: "no active group", run: func() error { _, err := f.s.Markets(context.Background()); return err }, wantErr: scope.ErrNoGroup},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}

	if got, err := f.s.Markets(other); err != nil || len(got) != 0 {
		t.Fatalf("markets of the other group = %v, %v", got, err)
	}
	if got, err := f.s.Products(other); err != nil || len(got) != 0 {
		t.Fatalf("products of the other group = %v, %v", got, err)
	}
	if got, err := f.s.Lists(other); err != nil || len(got) != 0 {
		t.Fatalf("lists of the other group = %v, %v", got, err)
	}
	if got, err := f.s.Receipts(other); err != nil || len(got) != 0 {
		t.Fatalf("receipts of the other group = %v, %v", got, err)
	}
	if got, err := f.s.Latest(other); err != nil || len(got) != 0 {
		t.Fatalf("prices of the other group = %v, %v", got, err)
	}
	if got, err := f.s.Catalog(other); err != nil || len(got) != 0 {
		t.Fatalf("catalog of the other group = %v, %v", got, err)
	}

	// Settings: the other group starts with the defaults and its own save leaves the first group alone.
	if got, err := f.s.Settings(other); err != nil || got != DefaultSettings {
		t.Fatalf("settings of the other group = %+v, %v", got, err)
	}
	if err := f.s.UpdateSettings(other, Settings{MaxMarkets: 4, MinSavingsCents: 0, StaleDays: 60}); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.s.Settings(groupCtx); got != custom {
		t.Fatalf("settings of the first group = %+v, want %+v", got, custom)
	}
	if got, _ := f.s.Settings(other); got.MaxMarkets != 4 || got.StaleDays != 60 {
		t.Fatalf("settings of the other group = %+v", got)
	}

	// The same name, neighborhood and receipt are free in the other group, which keeps its own catalog.
	if _, err := f.s.CreateMarket(other, carla.ID, MarketInput{Name: "Mercado Ficticio", Neighborhood: "Centro"}); err != nil {
		t.Fatalf("same market in the other group: %v", err)
	}
	if _, err := f.s.CreateProduct(other, carla.ID, ProductInput{Name: "Café", Brand: "Marca Ficticia", SizeMilli: 500_000, Unit: UnitGram}); err != nil {
		t.Fatalf("same product in the other group: %v", err)
	}
	preview, err := f.s.PreviewReceipt(other, "https://nfce.sefaz.go.gov.br/1", 0)
	if err != nil || preview.ImportedID != nil || preview.MarketID != nil {
		t.Fatalf("preview of the other group = %+v, %v", preview, err)
	}
	if _, err := f.s.CommitReceipt(other, carla.ID, ReceiptCommit{
		URL: "https://nfce.sefaz.go.gov.br/1", NewMarket: &MarketInput{Name: "Mercado do CNPJ", Neighborhood: "Bairro Teste"},
		Items: []CommitItem{{Index: 0, Ignore: true}, {Index: 1, Ignore: true}, {Index: 2, Ignore: true}},
	}); err != nil {
		t.Fatalf("same receipt in the other group: %v", err)
	}
	if got, _ := f.s.Receipts(groupCtx); len(got) != 1 {
		t.Fatalf("receipts of the first group = %d, want 1", len(got))
	}
}
