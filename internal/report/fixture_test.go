package report

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/alelo"
	"github.com/italogfx/radar-finance/internal/bill"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/financing"
	"github.com/italogfx/radar-finance/internal/income"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/receivable"
	"github.com/italogfx/radar-finance/internal/savings"
)

// world is a fictional November 2026 touching every source of money, built once per test.
type world struct {
	conn       *sql.DB
	userID     int64
	persons    [2]int64
	nubank     int64
	inter      int64
	cardID     int64
	cat        map[string]int64
	ledger     *ledger.Service
	cards      *card.Service
	bills      *bill.Service
	financings *financing.Service
	incomes    *income.Service
	alelo      *alelo.Service
	savings    *savings.Service
	recv       *receivable.Service
}

// d builds a civil date.
func d(y int, m time.Month, day int) civil.Date { return civil.Date{Year: y, Month: m, Day: day} }

// nov is the month every test looks at.
var nov = civil.Month{Year: 2026, Month: time.November}

// newWorld opens a fresh database and registers the fictional month:
//
//	card Nubank (closes 3, due 10): November invoice has an air conditioner of 300,00 in 3x split
//	half with João (couple share 50,00), a refund of 20,00 in Delivery, a credit of 5,00 without
//	category and a streaming recurrence of 21,90; it is paid in full from Nubank Ana.
//	accounts: 4.500,00 salary confirmed on the 5th and a 1.000,00 grant still expected on the 20th;
//	one off expense of 45,90 in Delivery; power bill of 180,00 paid; house installment of 1.500,00
//	paid; transfer of 300,00 to Inter; savings deposit of 450,00; João paid back his 50,00.
//	alelo: purchase of 320,00.
func newWorld(t *testing.T) *world {
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
	users := account.NewService(conn)
	ana, _ := users.Create(ctx, "ana", "Ana Ficticia")
	if _, err := users.Create(ctx, "bruno", "Bruno Ficticio"); err != nil {
		t.Fatal(err)
	}
	persons, _ := users.ListPersons(ctx)
	w := &world{
		conn: conn, userID: ana.ID, persons: [2]int64{persons[0].ID, persons[1].ID}, cat: map[string]int64{},
		ledger: ledger.NewService(conn), cards: card.NewService(conn), bills: bill.NewService(conn), financings: financing.NewService(conn),
		incomes: income.NewService(conn), alelo: alelo.NewService(conn), savings: savings.NewService(conn), recv: receivable.NewService(conn),
	}
	loadCategories(t, conn, w.cat)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	nubank, err := w.ledger.CreateAccount(ctx, w.userID, ledger.NewAccount{Name: "Nubank Ana", PersonID: w.persons[0], InitialBalanceCents: 1_000_000})
	must(err)
	inter, err := w.ledger.CreateAccount(ctx, w.userID, ledger.NewAccount{Name: "Inter Bruno", PersonID: w.persons[1], InitialBalanceCents: 200_000})
	must(err)
	w.nubank, w.inter = nubank.ID, inter.ID

	c, err := w.cards.CreateCard(ctx, w.userID, card.CardInput{Name: "Nubank", PersonID: w.persons[0], LimitCents: 1_000_000, ClosingDay: 3, DueDay: 10, Color: "purple", Finish: "matte"})
	must(err)
	w.cardID = c.ID
	_, err = w.cards.AddPurchase(ctx, w.userID, card.PurchaseInput{
		CardID: c.ID, Date: d(2026, time.October, 20), Merchant: "Ar-condicionado", CategoryID: w.cat["Móveis e utilidades"], AmountCents: 30_000, Installments: 3,
		Split: &card.SplitInput{Mode: card.SplitEqual, CoupleParts: 1, Parties: []card.PartyShare{{Name: "João Ficticio"}}},
	})
	must(err)
	delivery := w.cat["Delivery"]
	_, err = w.cards.AddCredit(ctx, w.userID, card.CreditInput{CardID: c.ID, Date: d(2026, time.October, 25), Merchant: "Estorno pizzaria", AmountCents: 2_000, CategoryID: &delivery})
	must(err)
	_, err = w.cards.AddCredit(ctx, w.userID, card.CreditInput{CardID: c.ID, Date: d(2026, time.October, 26), Merchant: "Crédito de anuidade", AmountCents: 500})
	must(err)
	_, err = w.cards.AddRecurrence(ctx, w.userID, card.RecurrenceInput{CardID: c.ID, Description: "Streaming", CategoryID: w.cat["Streaming"], AmountCents: 2_190, StartDate: d(2026, time.October, 15)})
	must(err)

	salary, err := w.incomes.Create(ctx, w.userID, income.Input{Kind: income.KindRecurring, Name: "Salário", PersonID: w.persons[0], AccountID: w.nubank, AmountCents: 450_000, Day: 5, StartMonth: nov})
	must(err)
	_, err = w.incomes.Create(ctx, w.userID, income.Input{Kind: income.KindRecurring, Name: "Bolsa", PersonID: w.persons[1], AccountID: w.inter, AmountCents: 100_000, Day: 20, StartMonth: nov})
	must(err)
	_, err = w.incomes.Confirm(ctx, w.userID, income.Confirmation{IncomeID: salary.ID, Month: nov, Date: d(2026, time.November, 5)})
	must(err)

	_, err = w.ledger.AddExpense(ctx, w.userID, ledger.NewExpense{AccountID: w.nubank, AmountCents: 4_590, Date: d(2026, time.November, 8), CategoryID: delivery})
	must(err)
	power, err := w.bills.Create(ctx, w.userID, bill.Input{Name: "Energia", CategoryID: w.cat["Energia"], AccountID: w.nubank, DueDay: 15, ExpectedCents: 18_000, StartMonth: nov})
	must(err)
	_, err = w.bills.Pay(ctx, w.userID, bill.Payment{BillID: power.ID, Month: nov, Date: d(2026, time.November, 14)})
	must(err)
	house, err := w.financings.Create(ctx, w.userID, financing.Input{Name: "Casa", CategoryID: w.cat["Financiamento da casa"], AccountID: w.nubank, DueDay: 20, InstallmentCents: 150_000, TotalInstallments: 360, PaidInstallments: 24, BalanceCents: 28_000_000, NextMonth: nov})
	must(err)
	_, err = w.financings.Pay(ctx, w.userID, financing.Payment{FinancingID: house.ID, Month: nov, Date: d(2026, time.November, 20)})
	must(err)
	_, err = w.ledger.Transfer(ctx, w.userID, ledger.NewTransfer{FromAccountID: w.nubank, ToAccountID: w.inter, AmountCents: 30_000, Date: d(2026, time.November, 10)})
	must(err)
	box, err := w.savings.Create(ctx, w.userID, savings.Input{Name: "Reserva", TargetCents: 3_000_000, PercentBP: 1_000})
	must(err)
	_, err = w.savings.Deposit(ctx, w.userID, savings.Move{BoxID: box.ID, Month: nov, AmountCents: 45_000, AccountID: w.nubank, Date: d(2026, time.November, 6)})
	must(err)
	_, err = w.cards.Pay(ctx, w.userID, card.Payment{CardID: c.ID, Month: nov, AccountID: w.nubank, Date: d(2026, time.November, 10)})
	must(err)
	people, err := w.recv.List(ctx)
	must(err)
	_, err = w.recv.MarkReceived(ctx, w.userID, receivable.Receipt{ItemIDs: []int64{people[0].Items[0].ID}, AccountID: w.nubank, Date: d(2026, time.November, 12)})
	must(err)

	aleloCard, err := w.alelo.Create(ctx, w.userID, alelo.Input{Name: "Alelo Ana", PersonID: w.persons[0], MonthlyCreditCents: 80_000, CreditDay: 5})
	must(err)
	_, err = w.alelo.AddPurchase(ctx, w.userID, alelo.Purchase{AleloID: aleloCard.ID, Date: d(2026, time.November, 9), Description: "Mercado", AmountCents: 32_000})
	must(err)
	return w
}

// loadCategories maps every category name to its id.
func loadCategories(t *testing.T, conn *sql.DB, into map[string]int64) {
	t.Helper()
	rows, err := conn.Query(`SELECT id, name FROM categories`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatal(err)
		}
		into[name] = id
	}
}
