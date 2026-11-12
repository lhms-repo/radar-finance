package savings

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/scope"
	"github.com/italogfx/radar-finance/internal/testutil"
)

// fixture holds a migrated database where today is 2026-11-11, with an account and the services.
type fixture struct {
	s         *Service
	ledger    *ledger.Service
	conn      *sql.DB
	userID    int64
	accountID int64
	today     *civil.Date
}

// groupCtx acts on the first group of a fresh database, created by every fixture.
var groupCtx = scope.WithGroup(context.Background(), 1)

// newFixture opens a fresh database for one test.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := groupCtx
	conn := testutil.OpenDB(t)
	ana := testutil.NewUser(t, conn, "ana", "Ana Ficticia")
	bruno := testutil.NewUser(t, conn, "bruno", "Bruno Ficticio")
	testutil.FirstGroup(t, conn, ana, bruno)
	persons := []account.Person{{ID: ana.PersonID}, {ID: bruno.PersonID}}
	l := ledger.NewService(conn)
	acc, err := l.CreateAccount(ctx, ana.ID, ledger.NewAccount{Name: "Nubank Ana", PersonID: persons[0].ID, InitialBalanceCents: 5_000_000})
	if err != nil {
		t.Fatal(err)
	}
	today := civil.Date{Year: 2026, Month: time.November, Day: 11}
	f := &fixture{s: NewService(conn), ledger: l, conn: conn, userID: ana.ID, accountID: acc.ID, today: &today}
	f.s.today = func() civil.Date { return *f.today }
	// The clock follows the test day, at noon in São Paulo, so creation months match.
	f.s.now = func() time.Time { return time.Date(f.today.Year, f.today.Month, f.today.Day, 12, 0, 0, 0, f.s.loc) }
	return f
}

// month builds a civil month.
func month(y int, m time.Month) civil.Month { return civil.Month{Year: y, Month: m} }

// date builds a civil date.
func date(y int, m time.Month, d int) civil.Date { return civil.Date{Year: y, Month: m, Day: d} }

// income records a confirmed one off income on the day.
func (f *fixture) income(t *testing.T, cents int64, on civil.Date) {
	t.Helper()
	if _, err := f.ledger.AddIncome(groupCtx, f.userID, ledger.NewIncome{AccountID: f.accountID, AmountCents: cents, Date: on}); err != nil {
		t.Fatal(err)
	}
}

// deposit confirms a deposit of the month from the account.
func (f *fixture) deposit(t *testing.T, boxID, cents int64, m civil.Month, on civil.Date) {
	t.Helper()
	if _, err := f.s.Deposit(groupCtx, f.userID, Move{BoxID: boxID, Month: m, AmountCents: cents, AccountID: f.accountID, Date: on}); err != nil {
		t.Fatal(err)
	}
}

// box returns the box from the view of the month.
func (f *fixture) box(t *testing.T, id int64, m civil.Month) Box {
	t.Helper()
	v, err := f.s.Month(groupCtx, m)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range v.Boxes {
		if b.ID == id {
			return b
		}
	}
	t.Fatalf("box %d not in %s", id, m)
	return Box{}
}

// TestCreate validates the box registration (RF-43).
func TestCreate(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	tests := []struct {
		name    string
		in      Input
		wantErr error
	}{
		{name: "valid", in: Input{Name: "Reserva", TargetCents: 3_000_000, PercentBP: 1_000}},
		{name: "with money already saved", in: Input{Name: "Viagem", TargetCents: 1_000_000, PercentBP: 500, InitialBalanceCents: 200_000}},
		{name: "empty name", in: Input{TargetCents: 1, PercentBP: 1}, wantErr: ErrNameRequired},
		{name: "zero target", in: Input{Name: "X", PercentBP: 1}, wantErr: ledger.ErrAmountRequired},
		{name: "percent above 100", in: Input{Name: "X", TargetCents: 1, PercentBP: 10_001}, wantErr: ErrInvalidPercent},
		{name: "negative percent", in: Input{Name: "X", TargetCents: 1, PercentBP: -1}, wantErr: ErrInvalidPercent},
		{name: "negative starting balance", in: Input{Name: "X", TargetCents: 1, PercentBP: 1, InitialBalanceCents: -1}, wantErr: ErrInvalidBalance},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.s.Create(ctx, f.userID, tt.in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestSuggestion uses the confirmed income of the month, by the day it fell, limited to what is left (RN-09, RN-10, RN-12).
func TestSuggestion(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	reserve, _ := f.s.Create(ctx, f.userID, Input{Name: "Reserva", TargetCents: 3_000_000, PercentBP: 1_250})
	almost, _ := f.s.Create(ctx, f.userID, Input{Name: "Viagem", TargetCents: 1_000_000, PercentBP: 5_000, InitialBalanceCents: 980_000})
	done, _ := f.s.Create(ctx, f.userID, Input{Name: "Celular", TargetCents: 300_000, PercentBP: 1_000, InitialBalanceCents: 300_000})
	nov := month(2026, time.November)

	f.income(t, 450_001, date(2026, time.November, 5))
	f.income(t, 150_000, date(2026, time.October, 31)) // November salary that fell in October counts in October
	if _, err := f.ledger.AddExpense(ctx, f.userID, ledger.NewExpense{AccountID: f.accountID, AmountCents: 10_000, Date: date(2026, time.November, 6), CategoryID: f.category(t)}); err != nil {
		t.Fatal(err)
	}

	v, err := f.s.Month(ctx, nov)
	if err != nil {
		t.Fatal(err)
	}
	if v.BaseCents != 450_001 {
		t.Fatalf("base = %d, want 450001 (RN-09)", v.BaseCents)
	}
	tests := []struct {
		name string
		id   int64
		want int64
		done bool
	}{
		{name: "12,5% of the base, rounded down", id: reserve.ID, want: 56_250},
		{name: "limited to what is left", id: almost.ID, want: 20_000},
		{name: "done box gets no suggestion", id: done.ID, want: 0, done: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := f.box(t, tt.id, nov)
			if b.SuggestedCents != tt.want || b.Done != tt.done {
				t.Fatalf("box = %+v", b)
			}
		})
	}
}

// category returns an active subcategory for expenses.
func (f *fixture) category(t *testing.T) int64 {
	t.Helper()
	var id int64
	if err := f.conn.QueryRow(`SELECT id FROM categories WHERE name = 'Delivery'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestDeposit takes the money from the account without counting it as spending (RF-46, RN-08).
func TestDeposit(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	b, _ := f.s.Create(ctx, f.userID, Input{Name: "Reserva", TargetCents: 3_000_000, PercentBP: 1_000})
	nov := month(2026, time.November)
	tests := []struct {
		name    string
		in      Move
		wantErr error
	}{
		{name: "zero amount", in: Move{BoxID: b.ID, Month: nov, AccountID: f.accountID, Date: date(2026, time.November, 11)}, wantErr: ledger.ErrAmountRequired},
		{name: "missing account", in: Move{BoxID: b.ID, Month: nov, AmountCents: 1, AccountID: 999, Date: date(2026, time.November, 11)}, wantErr: ledger.ErrAccountNotFound},
		{name: "missing box", in: Move{BoxID: 999, Month: nov, AmountCents: 1, AccountID: f.accountID, Date: date(2026, time.November, 11)}, wantErr: ErrNotFound},
		{name: "confirmed amount", in: Move{BoxID: b.ID, Month: nov, AmountCents: 45_000, AccountID: f.accountID, Date: date(2026, time.November, 11)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.s.Deposit(ctx, f.userID, tt.in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
	got := f.box(t, b.ID, nov)
	if got.BalanceCents != 45_000 || got.DepositedCents != 45_000 || got.SuggestedCents != 0 {
		t.Fatalf("box = %+v", got)
	}
	if a, _ := f.ledger.GetAccount(ctx, f.accountID); a.BalanceCents != 5_000_000-45_000 {
		t.Fatalf("account = %d", a.BalanceCents)
	}
	if flow, _ := f.ledger.MonthFlow(ctx, nov); flow.ExpenseCents != 0 {
		t.Fatalf("deposit counted as spending: %+v (RN-08)", flow)
	}
	// Deleting the entry undoes the deposit.
	if err := f.ledger.DeleteEntry(ctx, got.Moves[0].EntryID); err != nil {
		t.Fatal(err)
	}
	if got := f.box(t, b.ID, nov); got.BalanceCents != 0 || len(got.Moves) != 0 {
		t.Fatalf("after undo = %+v", got)
	}
}

// TestWithdraw puts money back into an account with a reason, never more than the balance (RF-47).
func TestWithdraw(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	b, _ := f.s.Create(ctx, f.userID, Input{Name: "Reserva", TargetCents: 3_000_000, PercentBP: 1_000, InitialBalanceCents: 100_000})
	nov := month(2026, time.November)
	tests := []struct {
		name    string
		in      Move
		wantErr error
	}{
		{name: "without reason", in: Move{BoxID: b.ID, AmountCents: 10_000, AccountID: f.accountID, Date: date(2026, time.November, 11)}, wantErr: ErrReasonRequired},
		{name: "more than the balance", in: Move{BoxID: b.ID, AmountCents: 100_001, AccountID: f.accountID, Date: date(2026, time.November, 11), Reason: "Carro"}, wantErr: ErrInsufficient},
		{name: "valid", in: Move{BoxID: b.ID, AmountCents: 30_000, AccountID: f.accountID, Date: date(2026, time.November, 11), Reason: "Conserto do carro"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.s.Withdraw(ctx, f.userID, tt.in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
	got := f.box(t, b.ID, nov)
	if got.BalanceCents != 70_000 || got.Moves[0].Reason != "Conserto do carro" || got.Moves[0].Kind != KindWithdrawal {
		t.Fatalf("box = %+v", got)
	}
	if flow, _ := f.ledger.MonthFlow(ctx, nov); flow.IncomeCents != 0 {
		t.Fatalf("withdrawal counted as income: %+v (RN-08)", flow)
	}
}

// TestDeleteDepositKeepsBalance refuses to delete a deposit already withdrawn, which would leave the box negative.
func TestDeleteDepositKeepsBalance(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	b, _ := f.s.Create(ctx, f.userID, Input{Name: "Reserva", TargetCents: 3_000_000, PercentBP: 1_000, InitialBalanceCents: 10_000})
	nov := month(2026, time.November)
	on := date(2026, time.November, 11)
	if _, err := f.s.Deposit(ctx, f.userID, Move{BoxID: b.ID, Month: nov, AmountCents: 50_000, AccountID: f.accountID, Date: on}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Deposit(ctx, f.userID, Move{BoxID: b.ID, Month: nov, AmountCents: 5_000, AccountID: f.accountID, Date: on}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Withdraw(ctx, f.userID, Move{BoxID: b.ID, AmountCents: 60_000, AccountID: f.accountID, Date: on, Reason: "Viagem"}); err != nil {
		t.Fatal(err)
	}
	var big, small int64
	for _, mv := range f.box(t, b.ID, nov).Moves {
		switch {
		case mv.Kind == KindDeposit && mv.AmountCents == 50_000:
			big = mv.EntryID
		case mv.Kind == KindDeposit && mv.AmountCents == 5_000:
			small = mv.EntryID
		}
	}
	if err := f.ledger.DeleteEntry(ctx, big); !errors.Is(err, ledger.ErrSavingsNegative) {
		t.Fatalf("delete big deposit: %v, want ErrSavingsNegative", err)
	}
	// Balance 5.000 without the small one: still zero or more.
	if err := f.ledger.DeleteEntry(ctx, small); err != nil {
		t.Fatal(err)
	}
	if got := f.box(t, b.ID, nov); got.BalanceCents != 0 {
		t.Fatalf("balance = %d", got.BalanceCents)
	}
}

// TestDone finishes the box when the target is reached and keeps it in the history (RF-49, RN-12).
func TestDone(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	b, _ := f.s.Create(ctx, f.userID, Input{Name: "Celular", TargetCents: 300_000, PercentBP: 5_000, InitialBalanceCents: 250_000})
	f.income(t, 1_000_000, date(2026, time.November, 5))
	nov := month(2026, time.November)
	if got := f.box(t, b.ID, nov); got.Done || got.SuggestedCents != 50_000 || got.ProgressBP != 8_333 {
		t.Fatalf("before = %+v", got)
	}
	f.deposit(t, b.ID, 50_000, nov, date(2026, time.November, 11))
	if got := f.box(t, b.ID, nov); !got.Done || got.SuggestedCents != 0 || got.ProgressBP != 10_000 {
		t.Fatalf("after = %+v", got)
	}
	if got := f.box(t, b.ID, month(2026, time.December)); !got.Done || got.SuggestedCents != 0 {
		t.Fatalf("next month = %+v", got)
	}
}

// TestEstimate uses the average deposit of the three months before the current one (RN-11).
func TestEstimate(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	*f.today = date(2026, time.August, 1)
	old, _ := f.s.Create(ctx, f.userID, Input{Name: "Reserva", TargetCents: 1_600_000, PercentBP: 1_000})
	f.deposit(t, old.ID, 100_000, month(2026, time.August), date(2026, time.August, 10))
	f.deposit(t, old.ID, 100_000, month(2026, time.September), date(2026, time.September, 10))
	*f.today = date(2026, time.October, 1)
	young, _ := f.s.Create(ctx, f.userID, Input{Name: "Viagem", TargetCents: 1_000_000, PercentBP: 1_000})
	f.deposit(t, old.ID, 200_000, month(2026, time.October), date(2026, time.October, 10))
	f.deposit(t, young.ID, 250_000, month(2026, time.October), date(2026, time.October, 10))
	f.deposit(t, old.ID, 300_000, month(2026, time.November), date(2026, time.November, 10))
	*f.today = date(2026, time.November, 11)
	nov := month(2026, time.November)

	tests := []struct {
		name string
		id   int64
		want string
	}{
		// In November the window is August to October: (1.000 + 1.000 + 2.000) / 3 = 1.333,33 a month.
		// R$ 9.000,00 left takes 7 deposits; November already has one, so they run from December to June.
		{name: "three full months", id: old.ID, want: "2027-06"},
		// Created in October: only October counts, 2.500 a month. R$ 7.500,00 left takes 3 deposits;
		// November has none yet, so they run from November to January.
		{name: "younger than three months", id: young.ID, want: "2027-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.box(t, tt.id, nov)
			if got.EstimatedMonth == nil || got.EstimatedMonth.String() != tt.want {
				t.Fatalf("estimate = %v, want %s (box %+v)", got.EstimatedMonth, tt.want, got)
			}
		})
	}
	fresh, _ := f.s.Create(ctx, f.userID, Input{Name: "Nova", TargetCents: 100_000, PercentBP: 1_000})
	if got := f.box(t, fresh.ID, nov); got.EstimatedMonth != nil {
		t.Fatalf("no deposits, no estimate: %+v", got)
	}
}

// TestGroupIsolation shows that a second group sees none of the boxes and moves of the first (RF-82, RN-27).
func TestGroupIsolation(t *testing.T) {
	f := newFixture(t)
	nov := month(2026, time.November)
	b, err := f.s.Create(groupCtx, f.userID, Input{Name: "Reserva", TargetCents: 3_000_000, PercentBP: 1_000, InitialBalanceCents: 100_000})
	if err != nil {
		t.Fatal(err)
	}
	f.deposit(t, b.ID, 20_000, nov, date(2026, time.November, 11))
	carla := testutil.NewUser(t, f.conn, "carla", "Carla Ficticia")
	other := testutil.NewGroup(t, f.conn, "Outra", carla)
	mine, err := f.s.Create(other, carla.ID, Input{Name: "Reserva", TargetCents: 1_000_000, PercentBP: 500})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		run     func() error
		wantErr error
	}{
		{name: "update", run: func() error {
			_, err := f.s.Update(other, b.ID, Changes{Name: "X", TargetCents: 100, PercentBP: 100})
			return err
		}, wantErr: ErrNotFound},
		{name: "delete", run: func() error { return f.s.Delete(other, b.ID) }, wantErr: ErrNotFound},
		{name: "deposit", run: func() error {
			_, err := f.s.Deposit(other, carla.ID, Move{BoxID: b.ID, Month: nov, AmountCents: 100, AccountID: f.accountID, Date: date(2026, time.November, 11)})
			return err
		}, wantErr: ErrNotFound},
		{name: "withdraw", run: func() error {
			_, err := f.s.Withdraw(other, carla.ID, Move{BoxID: b.ID, Month: nov, AmountCents: 100, AccountID: f.accountID, Date: date(2026, time.November, 11), Reason: "Teste"})
			return err
		}, wantErr: ErrNotFound},
		{name: "account of another group", run: func() error {
			_, err := f.s.Deposit(other, carla.ID, Move{BoxID: mine.ID, Month: nov, AmountCents: 100, AccountID: f.accountID, Date: date(2026, time.November, 11)})
			return err
		}, wantErr: ledger.ErrAccountNotFound},
		{name: "no active group", run: func() error { _, err := f.s.Month(context.Background(), nov); return err }, wantErr: scope.ErrNoGroup},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
	v, err := f.s.Month(other, nov)
	if err != nil || len(v.Boxes) != 1 || v.Boxes[0].ID != mine.ID || v.Boxes[0].BalanceCents != 0 {
		t.Fatalf("month of the other group = %+v, %v", v, err)
	}
}
