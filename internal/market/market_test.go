package market

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/ledger"
)

// fixture holds a migrated database where today is 2026-11-11.
type fixture struct {
	s      *Service
	conn   *sql.DB
	userID int64
}

// newFixture opens a fresh database for one test.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Open(filepath.Join(t.TempDir(), "radar.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(ctx, conn, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	ana, _ := account.NewService(conn).Create(ctx, "ana", "Ana Ficticia")
	s := NewService(conn)
	s.today = func() civil.Date { return day(11) }
	return &fixture{s: s, conn: conn, userID: ana.ID}
}

// day builds a date in November 2026.
func day(d int) civil.Date { return civil.Date{Year: 2026, Month: time.November, Day: d} }

// mustMarket creates a market or fails the test.
func (f *fixture) mustMarket(t *testing.T, name, neighborhood string) Market {
	t.Helper()
	m, err := f.s.CreateMarket(context.Background(), f.userID, MarketInput{Name: name, Neighborhood: neighborhood})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// mustProduct creates a packaged product or fails the test.
func (f *fixture) mustProduct(t *testing.T, name, brand string, sizeMilli int64, unit Unit) Product {
	t.Helper()
	p, err := f.s.CreateProduct(context.Background(), f.userID, ProductInput{Name: name, Brand: brand, SizeMilli: sizeMilli, Unit: unit})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestMarkets validates name and neighborhood and keeps a market with prices (RF-65).
func TestMarkets(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tests := []struct {
		name    string
		in      MarketInput
		wantErr error
	}{
		{name: "valid", in: MarketInput{Name: " Mercado Ficticio ", Neighborhood: "Setor Bueno"}},
		{name: "same chain in another neighborhood", in: MarketInput{Name: "Mercado Ficticio", Neighborhood: "Centro"}},
		{name: "without neighborhood", in: MarketInput{Name: "Atacadão Ficticio"}},
		{name: "duplicate ignoring case", in: MarketInput{Name: "mercado ficticio", Neighborhood: "setor bueno"}, wantErr: ErrDuplicateMarket},
		{name: "empty name", in: MarketInput{Name: "  "}, wantErr: ErrMarketName},
		{name: "long name", in: MarketInput{Name: string(make([]byte, 61))}, wantErr: ErrMarketName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := f.s.CreateMarket(ctx, f.userID, tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (m.Name != "Mercado Ficticio" && m.Name != "Atacadão Ficticio" || !m.Active) {
				t.Fatalf("market = %+v", m)
			}
		})
	}

	markets, _ := f.s.Markets(ctx)
	p := f.mustProduct(t, "Café", "Marca Ficticia", 500_000, UnitGram)
	if _, err := f.s.AddPrice(ctx, f.userID, PriceInput{ProductID: p.ID, MarketID: markets[0].ID, PriceCents: 1_899, Date: day(10)}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DeleteMarket(ctx, markets[0].ID); !errors.Is(err, ErrMarketInUse) {
		t.Fatalf("delete with prices: %v", err)
	}
	if err := f.s.DeleteMarket(ctx, markets[1].ID); err != nil {
		t.Fatalf("delete unused: %v", err)
	}
	// Markets come by name, so the first one is the Atacadão; a used market is deactivated instead.
	if m, err := f.s.UpdateMarket(ctx, markets[0].ID, MarketInput{Name: markets[0].Name, Active: false}); err != nil || m.Active {
		t.Fatalf("deactivate = %+v, %v", m, err)
	}
	if _, err := f.s.UpdateMarket(ctx, markets[0].ID, MarketInput{Name: "Mercado Ficticio", Neighborhood: "Setor Bueno"}); !errors.Is(err, ErrDuplicateMarket) {
		t.Fatalf("rename to another market: %v", err)
	}
}

// TestProducts requires a brand and a size, and makes products sold by weight priced per kg (RF-66).
func TestProducts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tests := []struct {
		name      string
		in        ProductInput
		wantErr   error
		wantSize  int64
		wantUnit  Unit
		wantLabel string
	}{
		{name: "packaged", in: ProductInput{Name: "Café", Brand: "Marca Ficticia", SizeMilli: 500_000, Unit: UnitGram}, wantSize: 500_000, wantUnit: UnitGram, wantLabel: "Café Marca Ficticia 500 g"},
		{name: "decimal size", in: ProductInput{Name: "Refrigerante", Brand: "Marca Ficticia", SizeMilli: 1_500, Unit: UnitLiter}, wantSize: 1_500, wantUnit: UnitLiter, wantLabel: "Refrigerante Marca Ficticia 1,5 L"},
		{name: "same product in another size", in: ProductInput{Name: "Café", Brand: "Marca Ficticia", SizeMilli: 250_000, Unit: UnitGram}, wantSize: 250_000, wantUnit: UnitGram, wantLabel: "Café Marca Ficticia 250 g"},
		{name: "by weight is per kg whatever was sent", in: ProductInput{Name: "Banana prata", Brand: "Sem marca", ByWeight: true, SizeMilli: 7, Unit: UnitUnit}, wantSize: 1_000, wantUnit: UnitKilogram, wantLabel: "Banana prata Sem marca, por kg"},
		{name: "duplicate ignoring case", in: ProductInput{Name: "café", Brand: "marca ficticia", SizeMilli: 500_000, Unit: UnitGram}, wantErr: ErrDuplicateProduct},
		{name: "without brand", in: ProductInput{Name: "Arroz", SizeMilli: 5_000, Unit: UnitKilogram}, wantErr: ErrProductBrand},
		{name: "without name", in: ProductInput{Brand: "X", SizeMilli: 1_000, Unit: UnitUnit}, wantErr: ErrProductName},
		{name: "without size", in: ProductInput{Name: "Arroz", Brand: "X", Unit: UnitKilogram}, wantErr: ErrProductSize},
		{name: "unknown unit", in: ProductInput{Name: "Arroz", Brand: "X", SizeMilli: 1_000, Unit: "lb"}, wantErr: ErrProductUnit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := f.s.CreateProduct(ctx, f.userID, tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (p.SizeMilli != tt.wantSize || p.Unit != tt.wantUnit || p.Label() != tt.wantLabel) {
				t.Fatalf("product = %+v, label %q", p, p.Label())
			}
		})
	}
}

// TestPrices keeps the history and takes the most recent date, then the newest row, as the latest price (RF-67, RF-70, RN-22).
func TestPrices(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a, b := f.mustMarket(t, "Mercado A", ""), f.mustMarket(t, "Mercado B", "")
	cafe := f.mustProduct(t, "Café", "Marca Ficticia", 500_000, UnitGram)

	steps := []struct {
		name       string
		in         PriceInput
		wantErr    error
		wantLatest map[int64]int64
	}{
		{name: "first price in A", in: PriceInput{MarketID: a.ID, PriceCents: 1_899, Date: day(5)}, wantLatest: map[int64]int64{a.ID: 1_899}},
		{name: "newer date replaces", in: PriceInput{MarketID: a.ID, PriceCents: 1_999, Date: day(8)}, wantLatest: map[int64]int64{a.ID: 1_999}},
		{name: "older date typed later stays in the history only", in: PriceInput{MarketID: a.ID, PriceCents: 1_500, Date: day(1)}, wantLatest: map[int64]int64{a.ID: 1_999}},
		{name: "same date, newest row wins", in: PriceInput{MarketID: a.ID, PriceCents: 2_049, Date: day(8)}, wantLatest: map[int64]int64{a.ID: 2_049}},
		{name: "each market apart", in: PriceInput{MarketID: b.ID, PriceCents: 1_749, Date: day(2)}, wantLatest: map[int64]int64{a.ID: 2_049, b.ID: 1_749}},
		{name: "future date", in: PriceInput{MarketID: b.ID, PriceCents: 1_000, Date: day(12)}, wantErr: ErrPriceDate},
		{name: "zero price", in: PriceInput{MarketID: b.ID, Date: day(10)}, wantErr: ledger.ErrAmountRequired},
		{name: "unknown market", in: PriceInput{MarketID: 999, PriceCents: 1_000, Date: day(10)}, wantErr: ErrMarketNotFound},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			st.in.ProductID = cafe.ID
			if _, err := f.s.AddPrice(ctx, f.userID, st.in); !errors.Is(err, st.wantErr) {
				t.Fatalf("err = %v, want %v", err, st.wantErr)
			}
			if st.wantErr != nil {
				return
			}
			latest, err := f.s.Latest(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for market, cents := range st.wantLatest {
				if got := latest[Key{ProductID: cafe.ID, MarketID: market}].PriceCents; got != cents {
					t.Fatalf("latest in %d = %d, want %d", market, got, cents)
				}
			}
		})
	}

	history, err := f.s.History(ctx, cafe.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 5 || history[0].PriceCents != 2_049 || history[len(history)-1].PriceCents != 1_500 {
		t.Fatalf("history = %+v", history)
	}
	// Correction and removal of a wrong price (RF-70).
	if err := f.s.UpdatePrice(ctx, history[0].ID, PriceChanges{PriceCents: 1_949, Date: day(8)}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DeletePrice(ctx, history[1].ID); err != nil {
		t.Fatal(err)
	}
	latest, _ := f.s.Latest(ctx)
	if got := latest[Key{ProductID: cafe.ID, MarketID: a.ID}].PriceCents; got != 1_949 {
		t.Fatalf("after correction = %d", got)
	}
	if err := f.s.DeletePrice(ctx, 999); !errors.Is(err, ErrPriceNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
	if err := f.s.DeleteProduct(ctx, cafe.ID); !errors.Is(err, ErrProductInUse) {
		t.Fatalf("delete product with prices: %v", err)
	}
}

// TestSettings bounds the comparison settings (RF-74, RN-25).
func TestSettings(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	got, err := f.s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != (Settings{MaxMarkets: 2, MinSavingsCents: 1_000, StaleDays: 30}) {
		t.Fatalf("defaults = %+v", got)
	}
	tests := []struct {
		name    string
		in      Settings
		wantErr error
	}{
		{name: "valid", in: Settings{MaxMarkets: 3, MinSavingsCents: 500, StaleDays: 45}},
		{name: "no minimum", in: Settings{MaxMarkets: 1, StaleDays: 1}},
		{name: "zero markets", in: Settings{MaxMarkets: 0, StaleDays: 30}, wantErr: ErrSettingsMarkets},
		{name: "six markets", in: Settings{MaxMarkets: 6, StaleDays: 30}, wantErr: ErrSettingsMarkets},
		{name: "negative minimum", in: Settings{MaxMarkets: 2, MinSavingsCents: -1, StaleDays: 30}, wantErr: ErrSettingsSavings},
		{name: "zero days", in: Settings{MaxMarkets: 2, StaleDays: 0}, wantErr: ErrSettingsDays},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := f.s.UpdateSettings(ctx, tt.in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
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
		{"0,009", 0, true}, {"", 0, false}, {"abc", 0, false}, {"6,4999", 0, false},
	}
	for _, tt := range tests {
		got, err := TruncateCents(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("TruncateCents(%q) = %d, %v", tt.in, got, err)
		}
	}
}

// TestCatalog lists each product with the latest price per market and flags prices older than the configured days (RF-75, RN-25).
func TestCatalog(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a, b := f.mustMarket(t, "Mercado A", ""), f.mustMarket(t, "Mercado B", "")
	cafe := f.mustProduct(t, "Café", "Marca Ficticia", 500_000, UnitGram)
	arroz := f.mustProduct(t, "Arroz", "Marca Ficticia", 5_000, UnitKilogram)
	// Today is November 11: 30 days back is October 12, which is still fresh; October 11 is stale.
	for _, p := range []PriceInput{
		{ProductID: cafe.ID, MarketID: a.ID, PriceCents: 1_899, Date: civil.Date{Year: 2026, Month: time.October, Day: 12}},
		{ProductID: cafe.ID, MarketID: b.ID, PriceCents: 1_799, Date: civil.Date{Year: 2026, Month: time.October, Day: 11}},
	} {
		if _, err := f.s.AddPrice(ctx, f.userID, p); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := f.s.Catalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 2 || catalog[0].ID != arroz.ID || len(catalog[0].Prices) != 0 {
		t.Fatalf("catalog = %+v", catalog)
	}
	tests := []struct {
		name   string
		market int64
		cents  int64
		stale  bool
	}{
		{name: "exactly 30 days is fresh", market: a.ID, cents: 1_899},
		{name: "31 days is stale", market: b.ID, cents: 1_799, stale: true},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := catalog[1].Prices[i]
			if p.MarketID != tt.market || p.PriceCents != tt.cents || p.Stale != tt.stale {
				t.Fatalf("price = %+v", p)
			}
		})
	}
}
