package space

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/bill"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/financing"
	"github.com/italogfx/radar-finance/internal/income"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/savings"
	"github.com/italogfx/radar-finance/internal/scope"
	"github.com/italogfx/radar-finance/internal/testutil"
)

// Spaces of the fixture: Casa is created first with Ana, so her personal space is 2 and the one of Bruno 3.
const (
	home         int64 = 1
	anaPersonal  int64 = 2
	brunoPersona int64 = 3
)

// world holds a group Casa of Ana and Bruno, an account of each, a card of Ana and one item of every kind.
type world struct {
	s    *Service
	conn *sql.DB
	ctx  context.Context
	ids  map[string]int64
}

// newWorld fills a fresh database with fictional items in Casa, acting as Ana.
func newWorld(t *testing.T) world {
	t.Helper()
	conn := testutil.OpenDB(t)
	ana := testutil.NewUser(t, conn, "ana", "Ana Ficticia")
	bruno := testutil.NewUser(t, conn, "bruno", "Bruno Ficticio")
	testutil.FirstGroup(t, conn, ana, bruno)
	ctx := auth.WithUserID(scope.WithGroup(context.Background(), home), ana.ID)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	cat := func(name string) int64 {
		var id int64
		must(conn.QueryRow(`SELECT id FROM categories WHERE name = ?`, name).Scan(&id))
		return id
	}
	nov := civil.Month{Year: 2026, Month: time.November}
	day := func(d int) civil.Date { return civil.Date{Year: 2026, Month: time.November, Day: d} }
	ids := map[string]int64{}

	l := ledger.NewService(conn)
	a, err := l.CreateAccount(ctx, ana.ID, ledger.NewAccount{Name: "Nubank Ana", PersonID: ana.PersonID, InitialBalanceCents: 100_000})
	must(err)
	b, err := l.CreateAccount(ctx, ana.ID, ledger.NewAccount{Name: "Inter Bruno", PersonID: bruno.PersonID})
	must(err)
	ids["account ana"], ids["account bruno"] = a.ID, b.ID
	e, err := l.AddExpense(ctx, ana.ID, ledger.NewExpense{AccountID: a.ID, AmountCents: 1_000, Date: day(3), CategoryID: cat("Delivery")})
	must(err)
	ids["expense ana"] = e.ID
	e, err = l.AddExpense(ctx, ana.ID, ledger.NewExpense{AccountID: b.ID, AmountCents: 2_000, Date: day(3), CategoryID: cat("Delivery")})
	must(err)
	ids["expense bruno"] = e.ID

	cards := card.NewService(conn)
	c, err := cards.CreateCard(ctx, ana.ID, card.CardInput{Name: "Nubank", PersonID: ana.PersonID, LimitCents: 500_000, ClosingDay: 3, DueDay: 10, Color: "purple", Finish: "matte"})
	must(err)
	p, err := cards.AddPurchase(ctx, ana.ID, card.PurchaseInput{CardID: c.ID, Date: day(5), Merchant: "Loja Ficticia", CategoryID: cat("Delivery"), AmountCents: 30_000, Installments: 3})
	must(err)
	ids["purchase"] = p.ID
	p, err = cards.AddPurchase(ctx, ana.ID, card.PurchaseInput{CardID: c.ID, Date: day(6), Merchant: "Loja Dividida", CategoryID: cat("Delivery"), AmountCents: 10_000, Installments: 1})
	must(err)
	ids["split purchase"] = p.ID
	_, err = conn.Exec(`INSERT INTO parties (id, group_id, name, created_by, created_at) VALUES (90, 1, 'Carla Ficticia', ?, '2026-11-06 12:00:00+00:00')`, ana.ID)
	must(err)
	_, err = conn.Exec(`INSERT INTO splits (purchase_id, party_id, share_cents) VALUES (?, 90, 5000)`, p.ID)
	must(err)

	bills := bill.NewService(conn)
	power, err := bills.Create(ctx, ana.ID, bill.Input{Name: "Energia", CategoryID: cat("Energia"), AccountID: a.ID, DueDay: 15, ExpectedCents: 18_000, StartMonth: nov})
	must(err)
	paid, err := bills.Pay(ctx, ana.ID, bill.Payment{BillID: power.ID, Month: nov, Date: day(14)})
	must(err)
	ids["bill ana"], ids["bill payment"] = power.ID, paid.ID
	water, err := bills.Create(ctx, ana.ID, bill.Input{Name: "Água", CategoryID: cat("Água"), AccountID: b.ID, DueDay: 15, ExpectedCents: 9_000, StartMonth: nov})
	must(err)
	ids["bill bruno"] = water.ID

	fins := financing.NewService(conn)
	f, err := fins.Create(ctx, ana.ID, financing.Input{Name: "Carro", CategoryID: cat("Financiamento da casa"), AccountID: a.ID, DueDay: 20, InstallmentCents: 50_000, TotalInstallments: 48, PaidInstallments: 2, BalanceCents: 2_000_000, NextMonth: nov})
	must(err)
	_, err = fins.Pay(ctx, ana.ID, financing.Payment{FinancingID: f.ID, Month: nov, Date: day(20)})
	must(err)
	ids["financing"] = f.ID

	incomes := income.NewService(conn)
	salary, err := incomes.Create(ctx, ana.ID, income.Input{Kind: income.KindRecurring, Name: "Salário", PersonID: ana.PersonID, AccountID: a.ID, AmountCents: 450_000, Day: 5, StartMonth: nov})
	must(err)
	_, err = incomes.Confirm(ctx, ana.ID, income.Confirmation{IncomeID: salary.ID, Month: nov, Date: day(5)})
	must(err)
	ids["income ana"] = salary.ID
	grant, err := incomes.Create(ctx, ana.ID, income.Input{Kind: income.KindRecurring, Name: "Bolsa", PersonID: bruno.PersonID, AccountID: a.ID, AmountCents: 100_000, Day: 20, StartMonth: nov})
	must(err)
	ids["income bruno"] = grant.ID

	boxes := savings.NewService(conn)
	box, err := boxes.Create(ctx, ana.ID, savings.Input{Name: "Reserva", TargetCents: 1_000_000, PercentBP: 1_000})
	must(err)
	_, err = boxes.Deposit(ctx, ana.ID, savings.Move{BoxID: box.ID, Month: nov, AmountCents: 10_000, AccountID: a.ID, Date: day(6)})
	must(err)
	ids["savings ana"] = box.ID
	shared, err := boxes.Create(ctx, ana.ID, savings.Input{Name: "Viagem", TargetCents: 500_000, PercentBP: 500})
	must(err)
	_, err = boxes.Deposit(ctx, ana.ID, savings.Move{BoxID: shared.ID, Month: nov, AmountCents: 5_000, AccountID: b.ID, Date: day(6)})
	must(err)
	ids["savings bruno"] = shared.ID

	return world{s: NewService(conn), conn: conn, ctx: ctx, ids: ids}
}

// groupOf reads the space of a row.
func (w world) groupOf(t *testing.T, table string, id int64) int64 {
	t.Helper()
	var g int64
	if err := w.conn.QueryRow(`SELECT group_id FROM `+table+` WHERE id = ?`, id).Scan(&g); err != nil {
		t.Fatalf("%s %d: %v", table, id, err)
	}
	return g
}

// count runs a query that returns one number.
func (w world) count(t *testing.T, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := w.conn.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// TestMove takes items of Casa to the personal space of Ana with the entries that belong to them, and refuses
// what would land where its account, card or person cannot be seen, or what is tied to another record (RF-83).
func TestMove(t *testing.T) {
	w := newWorld(t)
	tests := []struct {
		name    string
		kind    Kind
		item    string
		target  int64
		wantErr error
		// check runs after a move that worked.
		check func(t *testing.T)
	}{
		{name: "expense on her account", kind: KindEntry, item: "expense ana", target: anaPersonal},
		{name: "expense on the account of bruno", kind: KindEntry, item: "expense bruno", target: anaPersonal, wantErr: ErrCannotMove},
		{name: "payment of a bill alone", kind: KindEntry, item: "bill payment", target: anaPersonal, wantErr: ErrCannotMove},
		{name: "purchase on her card", kind: KindPurchase, item: "purchase", target: anaPersonal},
		{name: "purchase split with other people", kind: KindPurchase, item: "split purchase", target: anaPersonal, wantErr: ErrCannotMove},
		{name: "bill with its payment", kind: KindBill, item: "bill ana", target: anaPersonal, check: func(t *testing.T) {
			if g := w.groupOf(t, "entries", w.ids["bill payment"]); g != anaPersonal {
				t.Fatalf("payment of the bill in %d, want %d", g, anaPersonal)
			}
		}},
		{name: "bill paid from the account of bruno", kind: KindBill, item: "bill bruno", target: anaPersonal, wantErr: ErrCannotMove},
		{name: "financing with its installments", kind: KindFinancing, item: "financing", target: anaPersonal, check: func(t *testing.T) {
			if n := w.count(t, `SELECT count(*) FROM entries e JOIN financing_events fe ON fe.entry_id = e.id WHERE fe.financing_id = ? AND e.group_id = ?`, w.ids["financing"], anaPersonal); n != 1 {
				t.Fatalf("installments moved = %d, want 1", n)
			}
		}},
		{name: "income with its confirmation", kind: KindIncome, item: "income ana", target: anaPersonal, check: func(t *testing.T) {
			if n := w.count(t, `SELECT count(*) FROM entries WHERE income_id = ? AND group_id = ?`, w.ids["income ana"], anaPersonal); n != 1 {
				t.Fatalf("confirmations moved = %d, want 1", n)
			}
		}},
		{name: "income of bruno", kind: KindIncome, item: "income bruno", target: anaPersonal, wantErr: ErrCannotMove},
		{name: "savings box with its deposit", kind: KindSavings, item: "savings ana", target: anaPersonal, check: func(t *testing.T) {
			if n := w.count(t, `SELECT count(*) FROM entries e JOIN savings_moves sm ON sm.entry_id = e.id WHERE sm.box_id = ? AND e.group_id = ?`, w.ids["savings ana"], anaPersonal); n != 1 {
				t.Fatalf("deposits moved = %d, want 1", n)
			}
		}},
		{name: "savings box with a deposit of bruno", kind: KindSavings, item: "savings bruno", target: anaPersonal, wantErr: ErrCannotMove},
		{name: "personal space of someone else", kind: KindEntry, item: "expense bruno", target: brunoPersona, wantErr: ErrNotMember},
		{name: "unknown kind", kind: "alelo", item: "expense ana", target: anaPersonal, wantErr: ErrUnknownKind},
	}
	tables := map[Kind]string{KindEntry: "entries", KindPurchase: "purchases", KindBill: "bills", KindFinancing: "financings", KindIncome: "incomes", KindSavings: "savings_boxes"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := w.ids[tt.item]
			err := w.s.Move(w.ctx, tt.kind, id, tt.target)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("move: %v, want %v", err, tt.wantErr)
			}
			table := tables[tt.kind]
			if table == "" {
				return
			}
			want := tt.target
			if tt.wantErr != nil {
				want = home
			}
			if g := w.groupOf(t, table, id); g != want {
				t.Fatalf("%s in %d, want %d", tt.item, g, want)
			}
			if tt.check != nil {
				tt.check(t)
			}
		})
	}
	// An item that left the space is no longer found there, and coming back works from the personal space.
	if err := w.s.Move(w.ctx, KindEntry, w.ids["expense ana"], anaPersonal); !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("move again from Casa: %v, want ErrItemNotFound", err)
	}
	personal := auth.WithUserID(scope.WithGroup(context.Background(), anaPersonal), auth.UserID(w.ctx))
	if err := w.s.Move(personal, KindEntry, w.ids["expense ana"], home); err != nil {
		t.Fatalf("move back to Casa: %v", err)
	}
	if g := w.groupOf(t, "entries", w.ids["expense ana"]); g != home {
		t.Fatalf("expense back in %d, want %d", g, home)
	}
}

// TestMoveOnlyByCreator keeps an item where it is when someone other than whoever recorded it tries to move it (RF-83).
func TestMoveOnlyByCreator(t *testing.T) {
	w := newWorld(t)
	var bruno int64
	if err := w.conn.QueryRow(`SELECT id FROM users WHERE login = 'bruno'`).Scan(&bruno); err != nil {
		t.Fatal(err)
	}
	asBruno := auth.WithUserID(scope.WithGroup(context.Background(), home), bruno)
	if err := w.s.Move(asBruno, KindEntry, w.ids["expense bruno"], brunoPersona); !errors.Is(err, ErrNotCreator) {
		t.Fatalf("move by someone else: %v, want ErrNotCreator", err)
	}
	if g := w.groupOf(t, "entries", w.ids["expense bruno"]); g != home {
		t.Fatalf("expense in %d, want %d", g, home)
	}
}
