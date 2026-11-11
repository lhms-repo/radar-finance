package notify

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/agenda"
	"github.com/italogfx/radar-finance/internal/bill"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/financing"
	"github.com/italogfx/radar-finance/internal/income"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/report"
	"github.com/italogfx/radar-finance/internal/savings"
)

// itoa writes an id as text.
func itoa(id int64) string { return strconv.FormatInt(id, 10) }

// d builds a civil date.
func d(y int, m time.Month, day int) civil.Date { return civil.Date{Year: y, Month: m, Day: day} }

// TestNotifications derives every kind of warning from the state on 2026-11-11 and keeps reads per user (RF-52, RF-64).
func TestNotifications(t *testing.T) {
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
	bruno, _ := users.Create(ctx, "bruno", "Bruno Ficticio")
	persons, _ := users.ListPersons(ctx)
	cat := map[string]int64{}
	for _, name := range []string{"Água", "Internet", "Energia", "Delivery", "Hobbies", "Alimentação", "Lazer"} {
		var id int64
		if err := conn.QueryRow(`SELECT id FROM categories WHERE name = ?`, name).Scan(&id); err != nil {
			t.Fatal(name, err)
		}
		cat[name] = id
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	today := d(2026, time.November, 11)
	nov := today.MonthOf()
	clock := func() civil.Date { return today }

	l := ledger.NewService(conn)
	acc, err := l.CreateAccount(ctx, ana.ID, ledger.NewAccount{Name: "Nubank Ana", PersonID: persons[0].ID, InitialBalanceCents: 1_000_000})
	must(err)
	bills := bill.NewService(conn)
	for _, b := range []bill.Input{
		{Name: "Água", CategoryID: cat["Água"], AccountID: acc.ID, DueDay: 13, ExpectedCents: 9_000, StartMonth: nov},
		{Name: "Internet", CategoryID: cat["Internet"], AccountID: acc.ID, DueDay: 5, ExpectedCents: 12_990, StartMonth: nov},
		{Name: "Energia", CategoryID: cat["Energia"], AccountID: acc.ID, DueDay: 15, ExpectedCents: 18_000, StartMonth: nov},
	} {
		_, err := bills.Create(ctx, ana.ID, b)
		must(err)
	}
	cards := card.NewService(conn).WithToday(clock)
	c, err := cards.CreateCard(ctx, ana.ID, card.CardInput{Name: "Nubank", PersonID: persons[0].ID, LimitCents: 500_000, ClosingDay: 3, DueDay: 10, Color: "purple", Finish: "matte"})
	must(err)
	_, err = cards.AddPurchase(ctx, ana.ID, card.PurchaseInput{CardID: c.ID, Date: d(2026, time.October, 20), Merchant: "Loja", CategoryID: cat["Delivery"], AmountCents: 5_000, Installments: 1})
	must(err)
	_, err = l.AddExpense(ctx, ana.ID, ledger.NewExpense{AccountID: acc.ID, AmountCents: 4_000, Date: d(2026, time.November, 2), CategoryID: cat["Delivery"]})
	must(err)
	_, err = l.AddExpense(ctx, ana.ID, ledger.NewExpense{AccountID: acc.ID, AmountCents: 6_000, Date: d(2026, time.November, 3), CategoryID: cat["Hobbies"]})
	must(err)
	reports := report.NewService(conn, cards)
	must(reports.SetBudget(ctx, ana.ID, cat["Alimentação"], 10_000)) // 4.000 cash + 5.000 card = 90%
	must(reports.SetBudget(ctx, ana.ID, cat["Lazer"], 5_000))        // 6.000 = 120%
	_, err = l.AddIncome(ctx, ana.ID, ledger.NewIncome{AccountID: acc.ID, AmountCents: 100_000, Date: d(2026, time.November, 5)})
	must(err)
	sav := savings.NewService(conn)
	_, err = sav.Create(ctx, ana.ID, savings.Input{Name: "Viagem", TargetCents: 1_000_000, PercentBP: 1_000})
	must(err)
	incomes := income.NewService(conn)
	_, err = incomes.Create(ctx, ana.ID, income.Input{Kind: income.KindRecurring, Name: "Bolsa", PersonID: persons[1].ID, AccountID: acc.ID, AmountCents: 150_000, Day: 20, StartMonth: nov, EndOn: &civil.Date{Year: 2026, Month: time.December, Day: 1}})
	must(err)

	s := NewService(conn, agenda.NewService(bills, financing.NewService(conn), cards), reports, sav, incomes)
	s.today = clock

	list, err := s.List(ctx, ana.ID)
	must(err)
	want := []struct {
		key      string
		kind     Kind
		severity Severity
		title    string
	}{
		{key: "overdue:bill:2:2026-11", kind: KindOverdue, severity: SeverityDanger, title: "Conta atrasada: Internet"},
		{key: "overdue:invoice:1:2026-11", kind: KindOverdue, severity: SeverityDanger, title: "Fatura atrasada: Nubank"},
		{key: "budget_over:" + itoa(cat["Lazer"]) + ":2026-11", kind: KindBudgetOver, severity: SeverityDanger, title: "Orçamento de Lazer estourado"},
		{key: "due_soon:bill:1:2026-11", kind: KindDueSoon, severity: SeverityWarning, title: "Vence em breve: Água"},
		{key: "budget_warning:" + itoa(cat["Alimentação"]) + ":2026-11", kind: KindBudgetWarning, severity: SeverityWarning, title: "Orçamento de Alimentação em 90%"},
		{key: "savings_pending:1:2026-11", kind: KindSavingsPending, severity: SeverityInfo, title: "Aporte pendente: Viagem"},
		{key: "income_ending:1:2026-12-01", kind: KindIncomeEnding, severity: SeverityInfo, title: "Bolsa termina em 01/12/2026"},
	}
	if len(list.Items) != len(want) || list.Unread != len(want) {
		t.Fatalf("notifications = %d, unread %d: %+v", len(list.Items), list.Unread, list.Items)
	}
	for i, w := range want {
		n := list.Items[i]
		if n.Key != w.key || n.Kind != w.kind || n.Severity != w.severity || n.Title != w.title || n.Message == "" || n.Link == "" || n.Read {
			t.Errorf("notification %d = %+v, want %+v", i, n, w)
		}
	}

	// Reads are per user.
	must(s.MarkRead(ctx, ana.ID, []string{want[0].key, want[3].key}))
	if got, _ := s.List(ctx, ana.ID); got.Unread != len(want)-2 || !got.Items[0].Read {
		t.Fatalf("after reading = unread %d", got.Unread)
	}
	if got, _ := s.List(ctx, bruno.ID); got.Unread != len(want) {
		t.Fatalf("other user = unread %d", got.Unread)
	}
	must(s.MarkAllRead(ctx, ana.ID))
	if got, _ := s.List(ctx, ana.ID); got.Unread != 0 {
		t.Fatalf("after reading all = unread %d", got.Unread)
	}
}
