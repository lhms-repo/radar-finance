package report

import (
	"errors"
	"testing"

	"github.com/italogfx/radar-finance/internal/ledger"
)

// TestBudgets compares the spending of each top level category with its budget, warning at 80% and 100% (RF-51, RF-52).
func TestBudgets(t *testing.T) {
	w := newWorld(t)
	ctx := groupCtx
	s := NewService(w.conn, w.cards)

	sets := []struct {
		name     string
		category string
		cents    int64
		wantErr  error
	}{
		{name: "Moradia", category: "Moradia", cents: 150_000},
		{name: "Alimentação", category: "Alimentação", cents: 40_000},
		{name: "Assinaturas", category: "Assinaturas", cents: 10_000},
		{name: "Lazer without spending", category: "Lazer", cents: 30_000},
		{name: "subcategory", category: "Delivery", cents: 10_000, wantErr: ErrBudgetTopLevel},
		{name: "negative", category: "Saúde", cents: -1, wantErr: ledger.ErrAmountRequired},
		{name: "missing category", category: "", cents: 1, wantErr: ErrBudgetTopLevel},
	}
	for _, tt := range sets {
		t.Run("set "+tt.name, func(t *testing.T) {
			if err := s.SetBudget(ctx, w.userID, w.cat[tt.category], tt.cents); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}

	got, err := s.Budgets(ctx, nov)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		name         string
		budget, used int64
		bp           int
		level        Level
	}{
		// Over first, then warning, then fine; the most used first inside each level.
		{name: "Moradia", budget: 150_000, used: 173_000, bp: 11_533, level: LevelOver},
		{name: "Alimentação", budget: 40_000, used: 34_590, bp: 8_647, level: LevelWarning},
		{name: "Assinaturas", budget: 10_000, used: 2_190, bp: 2_190, level: LevelOK},
		{name: "Lazer", budget: 30_000, used: 0, bp: 0, level: LevelOK},
	}
	if len(got) != len(want) {
		t.Fatalf("budgets = %+v", got)
	}
	for i, w := range want {
		b := got[i]
		if b.Name != w.name || b.BudgetCents != w.budget || b.SpentCents != w.used || b.UsedBP != w.bp || b.Level != w.level {
			t.Errorf("budget %d = %+v, want %+v", i, b, w)
		}
	}

	// Zero removes the budget.
	if err := s.SetBudget(ctx, w.userID, w.cat["Lazer"], 0); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Budgets(ctx, nov); len(got) != 3 {
		t.Fatalf("after removing = %+v", got)
	}
}
