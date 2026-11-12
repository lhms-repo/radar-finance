package market

import (
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/civil"
)

// compareWorld is a list priced in three markets, with a weighed item, a missing price and a stale one.
type compareWorld struct {
	f          *fixture
	listID     int64
	a, b, c, d Market
	noPrice    Product
}

// newCompareWorld builds the list of 2 rice, 1 coffee, 0,535 kg of meat and 1 soap in markets A, B, C and D.
func newCompareWorld(t *testing.T) compareWorld {
	t.Helper()
	f := newFixture(t)
	ctx := groupCtx
	w := compareWorld{f: f, a: f.mustMarket(t, "A", ""), b: f.mustMarket(t, "B", ""), c: f.mustMarket(t, "C", ""), d: f.mustMarket(t, "D", "")}
	rice := f.mustProduct(t, "Arroz", "X", 5_000, UnitKilogram)
	coffee := f.mustProduct(t, "Café", "X", 500_000, UnitGram)
	meat, _ := f.s.CreateProduct(ctx, f.userID, ProductInput{Name: "Carne", Brand: "Sem marca", ByWeight: true})
	soap := f.mustProduct(t, "Sabão", "X", 1_000, UnitUnit)
	w.noPrice = f.mustProduct(t, "Sal", "X", 1_000, UnitKilogram)
	l, _ := f.s.CreateList(ctx, f.userID, "Compra")
	w.listID = l.ID
	for _, it := range []struct {
		p   Product
		qty int64
	}{{rice, 2_000}, {coffee, 1_000}, {meat, 535}, {soap, 1_000}} {
		if err := f.s.AddItem(ctx, l.ID, it.p.ID, it.qty); err != nil {
			t.Fatal(err)
		}
	}
	old := civil.Date{Year: 2026, Month: time.September, Day: 1}
	prices := []struct {
		m     Market
		p     Product
		cents int64
		date  civil.Date
	}{
		{w.a, rice, 1_000, day(10)}, {w.a, coffee, 500, day(10)}, {w.a, meat, 3_990, day(10)}, {w.a, soap, 800, day(10)},
		{w.b, rice, 900, old}, {w.b, coffee, 600, day(10)}, {w.b, meat, 3_500, day(10)},
		{w.c, rice, 1_100, day(10)}, {w.c, coffee, 400, day(10)}, {w.c, meat, 4_000, day(10)}, {w.c, soap, 700, day(10)},
	}
	for _, p := range prices {
		if _, err := f.s.AddPrice(ctx, f.userID, PriceInput{ProductID: p.p.ID, MarketID: p.m.ID, PriceCents: p.cents, Date: p.date}); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

// TestRanking puts complete markets first by total, then the incomplete ones, and leaves out markets without prices (RF-72, RF-75, RN-22, RN-23, RN-25).
func TestRanking(t *testing.T) {
	w := newCompareWorld(t)
	got, err := w.f.s.Compare(groupCtx, w.listID)
	if err != nil {
		t.Fatal(err)
	}
	// A: 2×10,00 + 5,00 + 0,535×39,90 (21,3465 truncated) + 8,00 = 54,34.
	// C: 22,00 + 4,00 + 21,40 + 7,00 = 54,40. B: 18,00 + 6,00 + 18,725 truncated = 42,72, without soap.
	want := []struct {
		market  int64
		total   int64
		missing int
		stale   int
	}{
		{market: w.a.ID, total: 5_434},
		{market: w.c.ID, total: 5_440},
		{market: w.b.ID, total: 4_272, missing: 1, stale: 1},
	}
	if len(got.Ranking) != len(want) {
		t.Fatalf("ranking = %+v", got.Ranking)
	}
	for i, wnt := range want {
		r := got.Ranking[i]
		if r.MarketID != wnt.market || r.TotalCents != wnt.total || r.MissingCount != wnt.missing || r.StaleCount != wnt.stale {
			t.Errorf("position %d = %+v, want %+v", i+1, r, wnt)
		}
	}
}

// TestBestSplit adds a market only when it saves the minimum over the best combination with one less (RF-73, RF-74, RN-24).
func TestBestSplit(t *testing.T) {
	tests := []struct {
		name        string
		settings    Settings
		wantMarkets []string
		wantTotal   int64
		wantSavings int64
	}{
		// The best pair is B with C at 47,72: it saves 6,62 over A alone.
		{name: "saving below the minimum keeps one market", settings: Settings{MaxMarkets: 2, MinSavingsCents: 1_000, StaleDays: 365}, wantMarkets: []string{"A"}, wantTotal: 5_434},
		{name: "saving above the minimum splits in two", settings: Settings{MaxMarkets: 2, MinSavingsCents: 500, StaleDays: 365}, wantMarkets: []string{"B", "C"}, wantTotal: 4_772, wantSavings: 662},
		{name: "a third market that would sit idle is not added", settings: Settings{MaxMarkets: 3, MinSavingsCents: 0, StaleDays: 365}, wantMarkets: []string{"B", "C"}, wantTotal: 4_772, wantSavings: 662},
		{name: "only one market allowed", settings: Settings{MaxMarkets: 1, MinSavingsCents: 0, StaleDays: 365}, wantMarkets: []string{"A"}, wantTotal: 5_434},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newCompareWorld(t)
			ctx := groupCtx
			if err := w.f.s.UpdateSettings(ctx, tt.settings); err != nil {
				t.Fatal(err)
			}
			got, err := w.f.s.Compare(ctx, w.listID)
			if err != nil {
				t.Fatal(err)
			}
			s := got.Split
			if s == nil {
				t.Fatal("no split")
			}
			var names []string
			items := 0
			for _, m := range s.Markets {
				names = append(names, m.Name)
				items += len(m.Items)
			}
			if len(names) != len(tt.wantMarkets) || s.TotalCents != tt.wantTotal || s.SavingsCents == nil || *s.SavingsCents != tt.wantSavings || items != 4 {
				t.Fatalf("split = %v total %d savings %v items %d", names, s.TotalCents, s.SavingsCents, items)
			}
			for i := range names {
				if names[i] != tt.wantMarkets[i] {
					t.Fatalf("markets = %v, want %v", names, tt.wantMarkets)
				}
			}
		})
	}
}

// TestSplitUncovered leaves out the item without any price and warns about it (RN-24).
func TestSplitUncovered(t *testing.T) {
	w := newCompareWorld(t)
	ctx := groupCtx
	if err := w.f.s.AddItem(ctx, w.listID, w.noPrice.ID, 1_000); err != nil {
		t.Fatal(err)
	}
	got, err := w.f.s.Compare(ctx, w.listID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Uncovered) != 1 || got.Uncovered[0].Product.ID != w.noPrice.ID {
		t.Fatalf("uncovered = %+v", got.Uncovered)
	}
	if got.Split == nil || got.Split.TotalCents != 5_434 {
		t.Fatalf("split = %+v", got.Split)
	}
	// Every market now misses the salt, so none is complete.
	for _, r := range got.Ranking {
		if r.MissingCount == 0 {
			t.Fatalf("complete market = %+v", r)
		}
	}
}

// TestCompareEmpty has no ranking and no split without priced items.
func TestCompareEmpty(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	l, _ := f.s.CreateList(ctx, f.userID, "Vazia")
	got, err := f.s.Compare(ctx, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Ranking) != 0 || got.Split != nil {
		t.Fatalf("compare = %+v", got)
	}
}
