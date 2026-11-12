package report

import (
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/civil"
)

// TestSpending sums every source by category, counting only the couple share (RN-01, RN-04, RN-07, RN-08, RN-15).
func TestSpending(t *testing.T) {
	w := newWorld(t)
	s := NewService(w.conn, w.cards)
	got, err := s.Spending(groupCtx, nov, nov)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("months = %d", len(got))
	}
	m := got[0]
	if m.TotalCents != 209_280 {
		t.Fatalf("total = %d, want 209280", m.TotalCents)
	}
	tests := []struct {
		category string
		cents    int64
		subs     map[string]int64
	}{
		{category: "Moradia", cents: 173_000, subs: map[string]int64{"Financiamento da casa": 150_000, "Energia": 18_000, "Móveis e utilidades": 5_000}},
		{category: "Alimentação", cents: 34_590, subs: map[string]int64{"Compras para casa": 32_000, "Delivery": 2_590}},
		{category: "Assinaturas", cents: 2_190, subs: map[string]int64{"Streaming": 2_190}},
		{category: "Sem categoria", cents: -500, subs: map[string]int64{}},
	}
	if len(m.Categories) != len(tests) {
		t.Fatalf("categories = %+v", m.Categories)
	}
	for i, tt := range tests {
		c := m.Categories[i]
		if c.Name != tt.category || c.Cents != tt.cents || len(c.Subs) != len(tt.subs) {
			t.Errorf("category %d = %+v, want %s %d", i, c, tt.category, tt.cents)
			continue
		}
		for _, sub := range c.Subs {
			if tt.subs[sub.Name] != sub.Cents {
				t.Errorf("%s > %s = %d, want %d", c.Name, sub.Name, sub.Cents, tt.subs[sub.Name])
			}
		}
	}
}

// TestSpendingEvolution returns one entry per month, empty months included (RF-59).
func TestSpendingEvolution(t *testing.T) {
	w := newWorld(t)
	s := NewService(w.conn, w.cards)
	got, err := s.Spending(groupCtx, civil.Month{Year: 2026, Month: time.October}, civil.Month{Year: 2026, Month: time.December})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		month string
		total int64
	}{{"2026-10", 0}, {"2026-11", 209_280}, {"2026-12", 7_190}}
	if len(got) != len(want) {
		t.Fatalf("months = %+v", got)
	}
	for i, w := range want {
		if got[i].Month.String() != w.month || got[i].TotalCents != w.total {
			t.Errorf("month %d = %s %d, want %s %d", i, got[i].Month, got[i].TotalCents, w.month, w.total)
		}
	}
}
