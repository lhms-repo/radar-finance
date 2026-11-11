package agenda

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/bill"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/financing"
	"github.com/italogfx/radar-finance/internal/ledger"
)

// TestMonth joins bills, financing installments and card invoices of the month by due date, pending first (RF-38, RF-40).
func TestMonth(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(filepath.Join(t.TempDir(), "radar.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(ctx, conn, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	users := account.NewService(conn)
	ana, _ := users.Create(ctx, "ana", "Ana Ficticia")
	persons, _ := users.ListPersons(ctx)
	l := ledger.NewService(conn)
	acc, _ := l.CreateAccount(ctx, ana.ID, ledger.NewAccount{Name: "Nubank Ana", PersonID: persons[0].ID, InitialBalanceCents: 1_000_000})
	var energia, casa, delivery int64
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Energia'`).Scan(&energia)
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Financiamento da casa'`).Scan(&casa)
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Delivery'`).Scan(&delivery)
	nov := civil.Month{Year: 2026, Month: time.November}

	bills := bill.NewService(conn)
	financings := financing.NewService(conn)
	cards := card.NewService(conn)
	power, _ := bills.Create(ctx, ana.ID, bill.Input{Name: "Energia", CategoryID: energia, AccountID: acc.ID, DueDay: 15, ExpectedCents: 18_000, StartMonth: nov})
	if _, err := bills.Create(ctx, ana.ID, bill.Input{Name: "Água", CategoryID: energia, AccountID: acc.ID, DueDay: 15, ExpectedCents: 9_000, StartMonth: nov}); err != nil {
		t.Fatal(err)
	}
	if _, err := financings.Create(ctx, ana.ID, financing.Input{Name: "Casa", CategoryID: casa, AccountID: acc.ID, DueDay: 20, InstallmentCents: 150_000, TotalInstallments: 360, PaidInstallments: 24, BalanceCents: 28_000_000, NextMonth: nov}); err != nil {
		t.Fatal(err)
	}
	nubank, _ := cards.CreateCard(ctx, ana.ID, card.CardInput{Name: "Nubank", PersonID: persons[0].ID, LimitCents: 500_000, ClosingDay: 3, DueDay: 10, Color: "purple", Finish: "matte"})
	empty, _ := cards.CreateCard(ctx, ana.ID, card.CardInput{Name: "Inter", PersonID: persons[0].ID, LimitCents: 500_000, ClosingDay: 3, DueDay: 12, Color: "orange", Finish: "matte"})
	if _, err := cards.AddPurchase(ctx, ana.ID, card.PurchaseInput{CardID: nubank.ID, Date: civil.Date{Year: 2026, Month: time.October, Day: 20}, Merchant: "Loja", CategoryID: delivery, AmountCents: 25_000, Installments: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := bills.Pay(ctx, ana.ID, bill.Payment{BillID: power.ID, Month: nov, Date: civil.Date{Year: 2026, Month: time.November, Day: 14}}); err != nil {
		t.Fatal(err)
	}

	s := NewService(bills, financings, cards)
	got, err := s.Month(ctx, nov)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		kind Kind
		name string
		due  string
		cent int64
		paid bool
	}{
		{kind: KindInvoice, name: "Nubank", due: "2026-11-10", cent: 25_000},
		{kind: KindBill, name: "Água", due: "2026-11-15", cent: 9_000},
		{kind: KindFinancing, name: "Casa", due: "2026-11-20", cent: 150_000},
		// Paid items go to the end of the list (section 8.6).
		{kind: KindBill, name: "Energia", due: "2026-11-15", cent: 18_000, paid: true},
	}
	if len(got.Items) != len(want) {
		t.Fatalf("items = %+v (an empty invoice, like card %d, stays out)", got.Items, empty.ID)
	}
	for i, w := range want {
		it := got.Items[i]
		if it.Kind != w.kind || it.Name != w.name || it.DueOn.String() != w.due || it.AmountCents != w.cent || it.Paid != w.paid {
			t.Errorf("item %d = %+v, want %+v", i, it, w)
		}
	}
	if got.TotalCents != 202_000 || got.PaidCents != 18_000 || got.Count != 4 || got.PaidCount != 1 {
		t.Fatalf("totals = %+v", got)
	}
	days := []Day{{Day: 10, Count: 1}, {Day: 15, Count: 2, PaidCount: 1}, {Day: 20, Count: 1}}
	if len(got.Days) != len(days) {
		t.Fatalf("days = %+v", got.Days)
	}
	for i := range days {
		if got.Days[i] != days[i] {
			t.Errorf("day %d = %+v, want %+v", i, got.Days[i], days[i])
		}
	}
}
