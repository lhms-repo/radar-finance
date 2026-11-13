package notify

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/agenda"
	"github.com/italogfx/radar-finance/internal/bill"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/financing"
	"github.com/italogfx/radar-finance/internal/income"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/report"
	"github.com/italogfx/radar-finance/internal/savings"
	"github.com/italogfx/radar-finance/internal/scope"
	"github.com/italogfx/radar-finance/internal/testutil"
)

// groupCtx acts on the first group of a fresh database, created by every fixture.
var groupCtx = scope.WithGroup(context.Background(), 1)

// itoa writes an id as text.
func itoa(id int64) string { return strconv.FormatInt(id, 10) }

// d builds a civil date.
func d(y int, m time.Month, day int) civil.Date { return civil.Date{Year: y, Month: m, Day: day} }

// TestNotifications derives every kind of warning from the state on 2026-11-11 and keeps reads per user (RF-52, RF-64).
func TestNotifications(t *testing.T) {
	ctx := groupCtx
	conn := testutil.OpenDB(t)
	ana := testutil.NewUser(t, conn, "ana", "Ana Ficticia")
	bruno := testutil.NewUser(t, conn, "bruno", "Bruno Ficticio")
	testutil.FirstGroup(t, conn, ana, bruno)
	persons := []account.Person{{ID: ana.PersonID}, {ID: bruno.PersonID}}
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
	// Registered at the start of the month of this fictional today, whatever the real clock says.
	for _, table := range []string{"bills", "cards", "financings"} {
		_, err := conn.Exec(`UPDATE ` + table + ` SET created_at = '2026-11-01 12:00:00+00:00'`)
		must(err)
	}

	list, err := s.List(ctx, ana.ID)
	must(err)
	want := []struct {
		key      string
		kind     Kind
		severity Severity
		title    string
	}{
		{key: "g1:overdue:bill:2:2026-11", kind: KindOverdue, severity: SeverityDanger, title: "Conta atrasada: Internet"},
		{key: "g1:budget_over:" + itoa(cat["Lazer"]) + ":2026-11", kind: KindBudgetOver, severity: SeverityDanger, title: "Orçamento de Lazer estourado"},
		{key: "g1:due_soon:bill:1:2026-11", kind: KindDueSoon, severity: SeverityWarning, title: "Vence em breve: Água"},
		{key: "g1:budget_warning:" + itoa(cat["Alimentação"]) + ":2026-11", kind: KindBudgetWarning, severity: SeverityWarning, title: "Orçamento de Alimentação em 90%"},
		{key: "g1:savings_pending:1:2026-11", kind: KindSavingsPending, severity: SeverityInfo, title: "Aporte pendente: Viagem"},
		{key: "g1:income_ending:1:2026-12-01", kind: KindIncomeEnding, severity: SeverityInfo, title: "Bolsa termina em 01/12/2026"},
	}
	if len(list.Items) != len(want) || list.Unread != len(want) {
		t.Fatalf("notifications = %d, unread %d: %+v", len(list.Items), list.Unread, list.Items)
	}
	// The overdue invoice warns only the owner, in her personal space, where she pays it (RF-83).
	if mine, err := s.List(scope.WithGroup(context.Background(), 2), ana.ID); err != nil || len(mine.Items) != 1 || mine.Items[0].Key != "g2:overdue:invoice:1:2026-11" || mine.Items[0].Title != "Fatura atrasada: Nubank" {
		t.Fatalf("personal notifications = %+v, %v", mine.Items, err)
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

// TestOverdueSinceRegistration warns about every unpaid month since the item was registered, up to
// 12 months back, and never about months before it (RF-64).
func TestOverdueSinceRegistration(t *testing.T) {
	ctx := groupCtx
	conn := testutil.OpenDB(t)
	ana := testutil.NewUser(t, conn, "ana", "Ana Ficticia")
	testutil.FirstGroup(t, conn, ana)
	persons := []account.Person{{ID: ana.PersonID}}
	var water int64
	if err := conn.QueryRow(`SELECT id FROM categories WHERE name = ?`, "Água").Scan(&water); err != nil {
		t.Fatal(err)
	}
	l := ledger.NewService(conn)
	acc, err := l.CreateAccount(ctx, ana.ID, ledger.NewAccount{Name: "Nubank Ana", PersonID: persons[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	today := d(2026, time.November, 11)
	clock := func() civil.Date { return today }
	bills := bill.NewService(conn)
	tests := []struct {
		name       string
		start      civil.Month
		registered string
		want       []string
	}{
		{
			name: "registered in June with a January start", start: civil.Month{Year: 2026, Month: time.January}, registered: "2026-06-15 12:00:00+00:00",
			want: []string{"2026-06", "2026-07", "2026-08", "2026-09", "2026-10", "2026-11"},
		},
		{
			// 01:00 UTC on June 1st is still May 31st in São Paulo, so May counts.
			name: "registration month read in São Paulo", start: civil.Month{Year: 2026, Month: time.January}, registered: "2026-06-01 01:00:00+00:00",
			want: []string{"2026-05", "2026-06", "2026-07", "2026-08", "2026-09", "2026-10", "2026-11"},
		},
		{
			name: "registered long ago stops at 12 months", start: civil.Month{Year: 2024, Month: time.January}, registered: "2024-01-10 12:00:00+00:00",
			want: []string{"2025-11", "2025-12", "2026-01", "2026-02", "2026-03", "2026-04", "2026-05", "2026-06", "2026-07", "2026-08", "2026-09", "2026-10", "2026-11"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := conn.Exec(`DELETE FROM bills`); err != nil {
				t.Fatal(err)
			}
			b, err := bills.Create(ctx, ana.ID, bill.Input{Name: "Água", CategoryID: water, AccountID: acc.ID, DueDay: 5, ExpectedCents: 9_000, StartMonth: tt.start})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := conn.Exec(`UPDATE bills SET created_at = ? WHERE id = ?`, tt.registered, b.ID); err != nil {
				t.Fatal(err)
			}
			cards := card.NewService(conn).WithToday(clock)
			s := NewService(conn, agenda.NewService(bills, financing.NewService(conn), cards), report.NewService(conn, cards), savings.NewService(conn), income.NewService(conn))
			s.today = clock
			list, err := s.List(ctx, ana.ID)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, n := range list.Items {
				if n.Kind == KindOverdue {
					got = append(got, n.Date.MonthOf().String())
				}
			}
			sort.Strings(got)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("overdue months = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGroupIsolation shows that the warnings of one group never reach another group (RF-64, RF-82, RN-27).
func TestGroupIsolation(t *testing.T) {
	conn := testutil.OpenDB(t)
	ana := testutil.NewUser(t, conn, "ana", "Ana Ficticia")
	carla := testutil.NewUser(t, conn, "carla", "Carla Ficticia")
	first := testutil.FirstGroup(t, conn, ana)
	other := testutil.NewGroup(t, conn, "Outra", carla)
	var water int64
	if err := conn.QueryRow(`SELECT id FROM categories WHERE name = ?`, "Água").Scan(&water); err != nil {
		t.Fatal(err)
	}
	acc, err := ledger.NewService(conn).CreateAccount(first, ana.ID, ledger.NewAccount{Name: "Nubank Ana", PersonID: ana.PersonID})
	if err != nil {
		t.Fatal(err)
	}
	today := d(2026, time.November, 11)
	clock := func() civil.Date { return today }
	bills := bill.NewService(conn)
	if _, err := bills.Create(first, ana.ID, bill.Input{Name: "Água", CategoryID: water, AccountID: acc.ID, DueDay: 5, ExpectedCents: 9_000, StartMonth: today.MonthOf()}); err != nil {
		t.Fatal(err)
	}
	cards := card.NewService(conn).WithToday(clock)
	s := NewService(conn, agenda.NewService(bills, financing.NewService(conn), cards), report.NewService(conn, cards), savings.NewService(conn), income.NewService(conn))
	s.today = clock
	if list, err := s.List(first, ana.ID); err != nil || len(list.Items) == 0 {
		t.Fatalf("notifications of the first group = %+v, %v", list, err)
	}
	if list, err := s.List(other, carla.ID); err != nil || len(list.Items) != 0 || list.Unread != 0 {
		t.Fatalf("notifications of the other group = %+v, %v", list, err)
	}
	if _, err := s.List(context.Background(), ana.ID); !errors.Is(err, scope.ErrNoGroup) {
		t.Fatalf("no group: err = %v", err)
	}
}
