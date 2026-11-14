package importer

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/income"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/scope"
	"github.com/italogfx/radar-finance/internal/testutil"
)

// stmtFixture holds Casa with Ana and Bruno, an account and a card of Ana, her salary and the fictional statement.
type stmtFixture struct {
	s         *Statements
	conn      *sql.DB
	ana       context.Context
	bruno     context.Context
	accountID int64
	cardID    int64
	salaryID  int64
	content   string
	cat       map[string]int64
}

// newStmtFixture builds the fixture with today on 2026-11-14, after the November invoice of the card closed.
func newStmtFixture(t *testing.T) stmtFixture {
	t.Helper()
	conn := testutil.OpenDB(t)
	ana := testutil.NewUser(t, conn, "ana", "Ana Ficticia")
	bruno := testutil.NewUser(t, conn, "bruno", "Bruno Ficticio")
	testutil.FirstGroup(t, conn, ana, bruno)
	asAna := auth.WithUserID(scope.WithGroup(context.Background(), 1), ana.ID)
	today := func() civil.Date { return civil.Date{Year: 2026, Month: time.November, Day: 14} }
	cards := card.NewService(conn).WithToday(today)
	l := ledger.NewService(conn)
	a, err := l.CreateAccount(asAna, ana.ID, ledger.NewAccount{Name: "Conta Ana", PersonID: ana.PersonID, InitialBalanceCents: 100_000})
	if err != nil {
		t.Fatal(err)
	}
	c, err := cards.CreateCard(asAna, ana.ID, card.CardInput{Name: "Cartão Ana", PersonID: ana.PersonID, LimitCents: 1_000_000, ClosingDay: 3, DueDay: 10, Color: "purple", Finish: "matte"})
	if err != nil {
		t.Fatal(err)
	}
	f := stmtFixture{
		s: NewStatements(conn, cards), conn: conn, ana: asAna, bruno: auth.WithUserID(scope.WithGroup(context.Background(), 1), bruno.ID),
		accountID: a.ID, cardID: c.ID, cat: map[string]int64{},
	}
	for _, name := range []string{"Delivery", "Sem categoria definida", "Tarifas bancárias", "Impostos", "Juros e multas", "Investimentos"} {
		var id int64
		if err := conn.QueryRow(`SELECT id FROM categories WHERE name = ?`, name).Scan(&id); err != nil {
			t.Fatal(name, err)
		}
		f.cat[name] = id
	}
	// The November invoice closes on the 3rd with R$ 450,00, the card payment of the statement.
	if _, err := cards.AddPurchase(asAna, ana.ID, card.PurchaseInput{CardID: c.ID, Date: civil.Date{Year: 2026, Month: time.October, Day: 20}, Merchant: "Loja Ficticia", CategoryID: f.cat["Delivery"], AmountCents: 45_000, Installments: 1}); err != nil {
		t.Fatal(err)
	}
	inc, err := income.NewService(conn).Create(asAna, ana.ID, income.Input{Kind: income.KindRecurring, Name: "Salário Ana", PersonID: ana.PersonID, AccountID: a.ID, AmountCents: 250_000, Day: 5, StartMonth: civil.Month{Year: 2026, Month: time.November}})
	if err != nil {
		t.Fatal(err)
	}
	f.salaryID = inc.ID
	// The bakery of the 14th was already recorded by hand on the 13th.
	if _, err := l.AddExpense(asAna, ana.ID, ledger.NewExpense{AccountID: a.ID, AmountCents: 2_350, Date: civil.Date{Year: 2026, Month: time.November, Day: 13}, CategoryID: f.cat["Delivery"], Description: "Padaria"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("testdata/santander-ficticio.csv")
	if err != nil {
		t.Fatal(err)
	}
	f.content = string(b)
	return f
}

// TestStatementPreview gives each line of the statement what it becomes: expenses with their default category,
// the salary confirming the income, the card payment on the closed invoice and the duplicate left out (RF-86 to RF-89, RN-32).
func TestStatementPreview(t *testing.T) {
	f := newStmtFixture(t)
	p, err := f.s.Preview(f.ana, f.accountID, f.content)
	if err != nil {
		t.Fatal(err)
	}
	if p.ClosingCents != 187_650 || len(p.Lines) != 12 {
		t.Fatalf("preview = closing %d with %d lines", p.ClosingCents, len(p.Lines))
	}
	nov := civil.Month{Year: 2026, Month: time.November}
	tests := []struct {
		name     string
		index    int
		category string
		skip     bool
		dup      bool
		income   bool
		card     bool
	}{
		{name: "bakery recorded by hand is a duplicate", index: 0, category: "Sem categoria definida", skip: true, dup: true},
		{name: "pix sent", index: 1, category: "Sem categoria definida"},
		{name: "card payment on the closed invoice", index: 2, card: true},
		{name: "salary confirms the income", index: 3, income: true},
		{name: "fee", index: 4, category: "Tarifas bancárias"},
		{name: "iof", index: 5, category: "Impostos"},
		{name: "interest", index: 6, category: "Juros e multas"},
		{name: "application", index: 7, category: "Investimentos"},
		{name: "pix received", index: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := p.Lines[tt.index]
			if tt.category != "" && (l.CategoryID == nil || *l.CategoryID != f.cat[tt.category]) {
				t.Fatalf("category = %v, want %s", l.CategoryID, tt.category)
			}
			if l.Skip != tt.skip || (l.Duplicate != nil) != tt.dup {
				t.Fatalf("skip = %v, duplicate = %+v", l.Skip, l.Duplicate)
			}
			if got := l.IncomeID != nil && *l.IncomeID == f.salaryID && l.IncomeMonth == nov; got != tt.income {
				t.Fatalf("income = %v %v", l.IncomeID, l.IncomeMonth)
			}
			if got := l.CardID != nil && *l.CardID == f.cardID && l.InvoiceMonth == nov; got != tt.card {
				t.Fatalf("card = %v %v", l.CardID, l.InvoiceMonth)
			}
		})
	}
}

// TestStatementCommitAndUndo records the confirmed lines in one go and undoes them whole: the income goes back to
// expected and the invoice to unpaid (RF-86 to RF-90).
func TestStatementCommitAndUndo(t *testing.T) {
	f := newStmtFixture(t)
	p, err := f.s.Preview(f.ana, f.accountID, f.content)
	if err != nil {
		t.Fatal(err)
	}
	lines := make([]StatementCommitLine, 0, len(p.Lines))
	for _, l := range p.Lines {
		lines = append(lines, StatementCommitLine{
			StatementLine: l.StatementLine, FriendlyName: l.FriendlyName, CategoryID: l.CategoryID, Skip: l.Skip,
			IncomeID: l.IncomeID, IncomeMonth: l.IncomeMonth, CardID: l.CardID, InvoiceMonth: l.InvoiceMonth,
			Remember: l.Index == 1,
		})
	}
	lines[1].FriendlyName = "Fulano"
	res, err := f.s.Commit(f.ana, auth.UserID(f.ana), f.accountID, "extrato.csv", p.ClosingCents, lines)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 11 || res.Skipped != 1 || res.RulesSaved != 1 {
		t.Fatalf("result = %+v", res)
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := f.conn.QueryRow(query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM entries WHERE statement_import_id = ?`, res.ImportID); n != 11 {
		t.Fatalf("entries of the import = %d, want 11", n)
	}
	if n := count(`SELECT count(*) FROM entries WHERE income_id = ? AND income_month = '2026-11'`, f.salaryID); n != 1 {
		t.Fatalf("salary confirmations = %d, want 1", n)
	}
	// The invoice is paid by its owner, in her personal space (RF-83, RN-07).
	if n := count(`SELECT count(*) FROM invoice_payments WHERE card_id = ? AND invoice_month = '2026-11' AND group_id = 2`, f.cardID); n != 1 {
		t.Fatalf("invoice payments in the personal space = %d, want 1", n)
	}
	if n := count(`SELECT count(*) FROM alias_rules WHERE original = 'fulano ficticio' AND friendly_name = 'Fulano'`); n != 1 {
		t.Fatalf("rules saved = %d", n)
	}
	// A second preview sees what the first import recorded as duplicates.
	again, err := f.s.Preview(f.ana, f.accountID, f.content)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range again.Lines {
		if !l.Skip {
			t.Fatalf("line %d of the same statement again is not left out: %+v", l.Index, l)
		}
	}
	if again.Lines[1].FriendlyName != "Fulano" {
		t.Fatalf("rule not applied: %q", again.Lines[1].FriendlyName)
	}

	if err := f.s.Undo(f.ana, res.ImportID); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT count(*) FROM entries WHERE statement_import_id IS NOT NULL`); n != 0 {
		t.Fatalf("entries left after undo = %d", n)
	}
	if n := count(`SELECT count(*) FROM invoice_payments`); n != 0 {
		t.Fatalf("invoice payments left after undo = %d", n)
	}
	if n := count(`SELECT count(*) FROM entries WHERE income_id = ?`, f.salaryID); n != 0 {
		t.Fatalf("salary confirmations left after undo = %d", n)
	}
	if err := f.s.Undo(f.ana, res.ImportID); !errors.Is(err, ErrImportNotFound) {
		t.Fatalf("second undo: %v", err)
	}
}

// TestStatementCardPaymentRules pays an invoice only by its owner and only with the invoice total (RF-89).
func TestStatementCardPaymentRules(t *testing.T) {
	nov := civil.Month{Year: 2026, Month: time.November}
	tests := []struct {
		name    string
		as      func(f stmtFixture) context.Context
		amount  int64
		wantErr error
	}{
		{name: "another member pays the card of ana", as: func(f stmtFixture) context.Context { return f.bruno }, amount: 45_000, wantErr: card.ErrPayInPersonal},
		{name: "amount differs from the invoice", as: func(f stmtFixture) context.Context { return f.ana }, amount: 40_000, wantErr: card.ErrPaymentAmount},
		{name: "owner with the total", as: func(f stmtFixture) context.Context { return f.ana }, amount: 45_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newStmtFixture(t)
			cardID := f.cardID
			line := StatementCommitLine{
				StatementLine: StatementLine{Date: civil.Date{Year: 2026, Month: time.November, Day: 10}, Description: "Pagamento cartao credito bce", Merchant: "Pagamento cartao credito bce", AmountCents: tt.amount, Kind: StatementCardPayment},
				CardID:        &cardID, InvoiceMonth: nov,
			}
			ctx := tt.as(f)
			_, err := f.s.Commit(ctx, auth.UserID(ctx), f.accountID, "extrato.csv", 0, []StatementCommitLine{line})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
