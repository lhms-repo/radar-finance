package report

import (
	"context"
	"errors"
	"testing"

	"github.com/italogfx/radar-finance/internal/scope"
	"github.com/italogfx/radar-finance/internal/testutil"
)

// TestGroupIsolation shows that a second group sees none of the money of the first, in any report (RF-82, RN-27).
func TestGroupIsolation(t *testing.T) {
	w := newWorld(t)
	s := newReport(w)
	if err := s.SetBudget(groupCtx, w.userID, w.cat["Moradia"], 150_000); err != nil {
		t.Fatal(err)
	}
	carla := testutil.NewUser(t, w.conn, "carla", "Carla Ficticia")
	other := testutil.NewGroup(t, w.conn, "Outra", carla)

	if got, err := s.Budgets(other, nov); err != nil || len(got) != 0 {
		t.Fatalf("budgets of the other group = %v, %v", got, err)
	}
	// The same category gets its own budget in the other group and leaves the first one alone.
	if err := s.SetBudget(other, carla.ID, w.cat["Moradia"], 90_000); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Budgets(groupCtx, nov); err != nil || len(got) != 1 || got[0].BudgetCents != 150_000 {
		t.Fatalf("budgets of the first group = %+v, %v", got, err)
	}
	if got, err := s.Budgets(other, nov); err != nil || len(got) != 1 || got[0].BudgetCents != 90_000 || got[0].SpentCents != 0 {
		t.Fatalf("budgets of the other group = %+v, %v", got, err)
	}
	if err := s.SetBudget(other, carla.ID, w.cat["Moradia"], 0); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Budgets(groupCtx, nov); len(got) != 1 {
		t.Fatalf("removing the budget of the other group reached the first: %+v", got)
	}

	spend, err := s.Spending(other, nov, nov)
	if err != nil || len(spend) != 1 || spend[0].TotalCents != 0 || len(spend[0].Categories) != 0 {
		t.Fatalf("spending of the other group = %+v, %v", spend, err)
	}
	if first, _ := s.Spending(groupCtx, nov, nov); first[0].TotalCents == 0 {
		t.Fatal("the first group lost its spending")
	}
	if nw, err := s.NetWorth(other); err != nil || nw != (NetWorth{}) {
		t.Fatalf("net worth of the other group = %+v, %v", nw, err)
	}
	if c, err := s.Closing(other, nov); err != nil || c != (Closing{Month: nov}) {
		t.Fatalf("closing of the other group = %+v, %v", c, err)
	}
	if lines, err := s.Search(other, Filter{From: d(2026, 1, 1), To: d(2026, 12, 31)}); err != nil || len(lines) != 0 {
		t.Fatalf("search of the other group = %v, %v", lines, err)
	}
	if lines, _ := s.Search(groupCtx, Filter{From: d(2026, 1, 1), To: d(2026, 12, 31)}); len(lines) == 0 {
		t.Fatal("the first group lost its lines")
	}
	cm, err := s.Commitments(other, nov, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cm {
		if c.CardCents != 0 || c.FinancingCents != 0 || c.IncomeCents != 0 {
			t.Fatalf("commitment of the other group = %+v", c)
		}
	}
	if _, err := s.NetWorth(context.Background()); !errors.Is(err, scope.ErrNoGroup) {
		t.Fatalf("no group: err = %v", err)
	}
	f, err := s.Forecast(other, nov)
	if err != nil || f.IncomeCents != 0 || f.OutCents != 0 || f.BalanceCents != 0 {
		t.Fatalf("forecast of the other group = %+v, %v", f, err)
	}
}
