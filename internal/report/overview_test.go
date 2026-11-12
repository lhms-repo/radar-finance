package report

import (
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/civil"
)

// TestForecast is the income of the month minus every expected way out, by group (RF-58, RN-13).
func TestForecast(t *testing.T) {
	w := newWorld(t)
	f, err := newReport(w).Forecast(groupCtx, nov)
	if err != nil {
		t.Fatal(err)
	}
	if f.IncomeCents != 550_000 || f.ReceivedCents != 450_000 || f.ExpectedCents != 100_000 {
		t.Fatalf("income = %d, received %d, expected %d", f.IncomeCents, f.ReceivedCents, f.ExpectedCents)
	}
	want := []struct {
		key   GroupKey
		cents int64
		bp    int
	}{
		{key: GroupHousing, cents: 168_000, bp: 3_054},
		{key: GroupInvoices, cents: 4_690, bp: 85},
		{key: GroupBills, cents: 0, bp: 0},
		{key: GroupDayToDay, cents: 4_590, bp: 83},
		{key: GroupSavings, cents: 45_000, bp: 818},
	}
	if len(f.Groups) != len(want) {
		t.Fatalf("groups = %+v", f.Groups)
	}
	for i, w := range want {
		g := f.Groups[i]
		if g.Key != w.key || g.Cents != w.cents || g.ShareBP != w.bp {
			t.Errorf("group %d = %+v, want %+v", i, g, w)
		}
	}
	if f.OutCents != 222_280 || f.BalanceCents != 327_720 || f.BalanceBP != 5_958 {
		t.Fatalf("out %d, balance %d, share %d", f.OutCents, f.BalanceCents, f.BalanceBP)
	}
}

// TestHousing measures the share of the income that goes to the house, for the couple and for one person (section 5.3).
func TestHousing(t *testing.T) {
	w := newWorld(t)
	h, err := newReport(w).Housing(groupCtx, nov, w.persons[0])
	if err != nil {
		t.Fatal(err)
	}
	if h.HousingCents != 173_000 || h.CoupleIncomeCents != 550_000 || h.PersonIncomeCents != 450_000 ||
		h.CoupleBP != 3_145 || h.PersonBP != 3_844 || h.ReferenceBP != 3_000 {
		t.Fatalf("housing = %+v", h)
	}
}

// TestCommitments sums installments and financings of the next months against the expected income (RF-60).
func TestCommitments(t *testing.T) {
	w := newWorld(t)
	got, err := newReport(w).Commitments(groupCtx, civil.Month{Year: 2026, Month: time.December}, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		month          string
		card, fin, bp  int64
		incomeExpected int64
	}{
		{month: "2026-12", card: 5_000, fin: 150_000, bp: 2_818, incomeExpected: 550_000},
		{month: "2027-01", card: 5_000, fin: 150_000, bp: 2_818, incomeExpected: 550_000},
		{month: "2027-02", card: 0, fin: 150_000, bp: 2_727, incomeExpected: 550_000},
	}
	if len(got) != len(want) {
		t.Fatalf("commitments = %+v", got)
	}
	for i, w := range want {
		c := got[i]
		if c.Month.String() != w.month || c.CardCents != w.card || c.FinancingCents != w.fin || c.TotalCents != w.card+w.fin || int64(c.BP) != w.bp || c.IncomeCents != w.incomeExpected {
			t.Errorf("month %d = %+v, want %+v", i, c, w)
		}
	}
}

// TestClosing sums what came in, what was spent, what was saved and what others owe (RF-61).
func TestClosing(t *testing.T) {
	w := newWorld(t)
	c, err := newReport(w).Closing(groupCtx, nov)
	if err != nil {
		t.Fatal(err)
	}
	if c.IncomeCents != 450_000 || c.SpendingCents != 209_280 || c.SavedCents != 45_000 || c.ReceivablePendingCents != 10_000 || c.ReceivedCents != 5_000 {
		t.Fatalf("closing = %+v", c)
	}
}

// TestNetWorth is accounts and savings minus financing balances and closed unpaid invoices (RF-62).
func TestNetWorth(t *testing.T) {
	w := newWorld(t)
	ctx := groupCtx
	s := newReport(w)
	n, err := s.NetWorth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n.AccountsCents != 1_427_720 || n.SavingsCents != 45_000 || n.FinancingCents != 27_850_000 || n.CardDebtCents != 0 || n.NetCents != 1_427_720+45_000-27_850_000 {
		t.Fatalf("net worth = %+v", n)
	}
	// On December 5 the December invoice is closed and unpaid: air conditioner 100,00 and streaming 21,90.
	w.cards.WithToday(func() civil.Date { return d(2026, time.December, 5) })
	if n, _ := s.NetWorth(ctx); n.CardDebtCents != 12_190 {
		t.Fatalf("card debt on December 5 = %d, want 12190", n.CardDebtCents)
	}
}
