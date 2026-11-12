package income

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

// fixture holds a migrated database, two fictional users, two accounts and the services.
type fixture struct {
	s       *Service
	ledger  *ledger.Service
	conn    *sql.DB
	userID  int64
	persons [2]int64
	nubank  int64
	inter   int64
}

// groupCtx acts on the first group of a fresh database, created by every fixture.
var groupCtx = scope.WithGroup(context.Background(), 1)

// newFixture opens a fresh database where today is 2026-11-11.
func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := groupCtx
	conn := testutil.OpenDB(t)
	ana := testutil.NewUser(t, conn, "ana", "Ana Ficticia")
	bruno := testutil.NewUser(t, conn, "bruno", "Bruno Ficticio")
	testutil.FirstGroup(t, conn, ana, bruno)
	persons := []account.Person{{ID: ana.PersonID}, {ID: bruno.PersonID}}
	l := ledger.NewService(conn)
	nubank, err := l.CreateAccount(ctx, ana.ID, ledger.NewAccount{Name: "Nubank Ana", PersonID: persons[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	inter, err := l.CreateAccount(ctx, ana.ID, ledger.NewAccount{Name: "Inter Bruno", PersonID: persons[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(conn)
	s.today = func() civil.Date { return civil.Date{Year: 2026, Month: time.November, Day: 11} }
	return fixture{s: s, ledger: l, conn: conn, userID: ana.ID, persons: [2]int64{persons[0].ID, persons[1].ID}, nubank: nubank.ID, inter: inter.ID}
}

// month builds a civil month.
func month(y int, m time.Month) civil.Month { return civil.Month{Year: y, Month: m} }

// date builds a civil date.
func date(y int, m time.Month, d int) civil.Date { return civil.Date{Year: y, Month: m, Day: d} }

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// salary returns a valid recurring income input for the first person.
func (f fixture) salary() Input {
	return Input{Kind: KindRecurring, Name: "Salário", PersonID: f.persons[0], AccountID: f.nubank, AmountCents: 450_000, Day: 5, StartMonth: month(2026, time.November)}
}

// TestCreate validates recurring and eventual incomes (RF-09, RF-11, RF-12).
func TestCreate(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	tests := []struct {
		name    string
		in      func() Input
		wantErr error
	}{
		{name: "recurring salary", in: f.salary},
		{name: "recurring with end date", in: func() Input {
			in := f.salary()
			in.Name, in.EndOn = "Bolsa", ptr(date(2027, time.February, 28))
			return in
		}},
		{name: "eventual 13th", in: func() Input {
			return Input{Kind: KindEventual, Name: "13º salário", PersonID: f.persons[1], AccountID: f.inter, AmountCents: 300_000, ExpectedOn: date(2026, time.December, 18)}
		}},
		{name: "unknown kind", in: func() Input { in := f.salary(); in.Kind = "weekly"; return in }, wantErr: ErrInvalidKind},
		{name: "empty name", in: func() Input { in := f.salary(); in.Name = " "; return in }, wantErr: ErrNameRequired},
		{name: "zero amount", in: func() Input { in := f.salary(); in.AmountCents = 0; return in }, wantErr: ledger.ErrAmountRequired},
		{name: "day out of range", in: func() Input { in := f.salary(); in.Day = 32; return in }, wantErr: ErrInvalidDay},
		{name: "missing start month", in: func() Input { in := f.salary(); in.StartMonth = civil.Month{}; return in }, wantErr: ErrStartRequired},
		{name: "end before start", in: func() Input { in := f.salary(); in.EndOn = ptr(date(2026, time.October, 31)); return in }, wantErr: ErrEndBeforeStart},
		{name: "eventual without date", in: func() Input {
			return Input{Kind: KindEventual, Name: "Férias", PersonID: f.persons[0], AccountID: f.nubank, AmountCents: 100}
		}, wantErr: ErrExpectedRequired},
		{name: "unknown person", in: func() Input { in := f.salary(); in.PersonID = 999; return in }, wantErr: ErrPersonNotFound},
		{name: "unknown account", in: func() Input { in := f.salary(); in.AccountID = 999; return in }, wantErr: ledger.ErrAccountNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in()
			got, err := f.s.Create(ctx, f.userID, in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (got.Name != in.Name || got.AmountCents != in.AmountCents || got.PersonName == "" || got.AccountName == "") {
				t.Fatalf("income = %+v", got)
			}
		})
	}
}

// TestMonthOccurrences derives the planned incomes of each month (RF-10, RF-11, RF-12).
func TestMonthOccurrences(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	salary, _ := f.s.Create(ctx, f.userID, Input{Kind: KindRecurring, Name: "Salário", PersonID: f.persons[0], AccountID: f.nubank, AmountCents: 450_000, Day: 31, StartMonth: month(2026, time.November)})
	grant, _ := f.s.Create(ctx, f.userID, Input{Kind: KindRecurring, Name: "Bolsa", PersonID: f.persons[1], AccountID: f.inter, AmountCents: 150_000, Day: 10, StartMonth: month(2026, time.November), EndOn: ptr(date(2027, time.January, 10))})
	thirteenth, _ := f.s.Create(ctx, f.userID, Input{Kind: KindEventual, Name: "13º salário", PersonID: f.persons[0], AccountID: f.nubank, AmountCents: 225_000, ExpectedOn: date(2026, time.December, 18)})

	tests := []struct {
		name  string
		month civil.Month
		want  map[int64]string // income id to expected date
	}{
		{name: "before the start month", month: month(2026, time.October), want: map[int64]string{}},
		{name: "first month, day 31 in a 30 day month", month: month(2026, time.November), want: map[int64]string{salary.ID: "2026-11-30", grant.ID: "2026-11-10"}},
		{name: "eventual in its month", month: month(2026, time.December), want: map[int64]string{salary.ID: "2026-12-31", grant.ID: "2026-12-10", thirteenth.ID: "2026-12-18"}},
		{name: "last month of the grant, on the end date", month: month(2027, time.January), want: map[int64]string{salary.ID: "2027-01-31", grant.ID: "2027-01-10"}},
		{name: "grant ended, February clamps to 28", month: month(2027, time.February), want: map[int64]string{salary.ID: "2027-02-28"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := f.s.Month(ctx, tt.month)
			if err != nil {
				t.Fatal(err)
			}
			got := map[int64]string{}
			var pending int64
			for _, o := range v.Occurrences {
				got[o.IncomeID] = o.ExpectedOn.String()
				pending += o.ExpectedCents
			}
			if len(got) != len(tt.want) {
				t.Fatalf("occurrences = %v, want %v", got, tt.want)
			}
			for id, d := range tt.want {
				if got[id] != d {
					t.Fatalf("income %d on %s, want %s", id, got[id], d)
				}
			}
			if v.PendingCents != pending || v.ReceivedCents != 0 {
				t.Fatalf("pending, received = %d, %d", v.PendingCents, v.ReceivedCents)
			}
		})
	}
}

// TestConfirm turns a planned income into a confirmed entry that changes the balance (RF-10, RN-13).
func TestConfirm(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	salary, _ := f.s.Create(ctx, f.userID, f.salary())
	nov := month(2026, time.November)

	if a, _ := f.ledger.GetAccount(ctx, f.nubank); a.BalanceCents != 0 {
		t.Fatalf("planned income changed the balance: %d (RN-13)", a.BalanceCents)
	}

	tests := []struct {
		name        string
		in          Confirmation
		wantErr     error
		wantAmount  int64
		wantDate    string
		wantAccount int64
	}{
		{name: "month without occurrence", in: Confirmation{IncomeID: salary.ID, Month: month(2026, time.October)}, wantErr: ErrNoOccurrence},
		{name: "missing income", in: Confirmation{IncomeID: 999, Month: nov}, wantErr: ErrNotFound},
		{name: "defaults to the expected values", in: Confirmation{IncomeID: salary.ID, Month: nov}, wantAmount: 450_000, wantDate: "2026-11-05", wantAccount: f.nubank},
		{name: "same month twice", in: Confirmation{IncomeID: salary.ID, Month: nov}, wantErr: ErrAlreadyConfirmed},
		{
			name: "real amount, date and account differ", in: Confirmation{IncomeID: salary.ID, Month: month(2026, time.December), AmountCents: 455_210, Date: date(2026, time.December, 4), AccountID: f.inter},
			wantAmount: 455_210, wantDate: "2026-12-04", wantAccount: f.inter,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := f.s.Confirm(ctx, f.userID, tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if e.AmountCents != tt.wantAmount || e.Date.String() != tt.wantDate || e.AccountID != tt.wantAccount || e.Kind != ledger.KindIncome || !e.Confirmed {
				t.Fatalf("entry = %+v", e)
			}
		})
	}

	if a, _ := f.ledger.GetAccount(ctx, f.nubank); a.BalanceCents != 450_000 {
		t.Fatalf("balance after confirm = %d", a.BalanceCents)
	}
	v, _ := f.s.Month(ctx, nov)
	if len(v.Occurrences) != 1 || !v.Occurrences[0].Confirmed || v.ReceivedCents != 450_000 || v.PendingCents != 0 {
		t.Fatalf("month view = %+v", v)
	}

	// Deleting the entry undoes the confirmation and the month is planned again.
	if err := f.ledger.DeleteEntry(ctx, *v.Occurrences[0].EntryID); err != nil {
		t.Fatal(err)
	}
	v, _ = f.s.Month(ctx, nov)
	if v.Occurrences[0].Confirmed || v.PendingCents != 450_000 {
		t.Fatalf("after undo = %+v", v)
	}
}

// TestUpdateKeepsConfirmedMonths refuses changes that leave a confirmed month outside the income (RF-10, RF-12).
func TestUpdateKeepsConfirmedMonths(t *testing.T) {
	ctx := groupCtx
	thirteenth := func(f fixture) Input {
		return Input{Kind: KindEventual, Name: "13º salário", PersonID: f.persons[0], AccountID: f.nubank, AmountCents: 100_000, ExpectedOn: date(2026, time.December, 18)}
	}
	tests := []struct {
		name    string
		base    func(f fixture) Input
		edit    func(in *Input)
		wantErr error
	}{
		{name: "eventual moved to another month", base: thirteenth, edit: func(in *Input) { in.ExpectedOn = date(2027, time.January, 10) }, wantErr: ErrConfirmedOutside},
		{name: "eventual moved inside the month", base: thirteenth, edit: func(in *Input) { in.ExpectedOn = date(2026, time.December, 20) }},
		{name: "eventual amount changed", base: thirteenth, edit: func(in *Input) { in.AmountCents = 120_000 }},
		{name: "recurring start after the confirmed month", base: fixture.salary, edit: func(in *Input) { in.StartMonth = month(2027, time.January) }, wantErr: ErrConfirmedOutside},
		{name: "recurring end before the confirmed month", base: fixture.salary, edit: func(in *Input) { in.EndOn = ptr(date(2026, time.November, 30)) }, wantErr: ErrConfirmedOutside},
		{name: "recurring end after the confirmed day", base: fixture.salary, edit: func(in *Input) { in.EndOn = ptr(date(2026, time.December, 31)) }},
		{name: "recurring day changed", base: fixture.salary, edit: func(in *Input) { in.Day = 20 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			in := tt.base(f)
			inc, err := f.s.Create(ctx, f.userID, in)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.Confirm(ctx, f.userID, Confirmation{IncomeID: inc.ID, Month: month(2026, time.December)}); err != nil {
				t.Fatal(err)
			}
			tt.edit(&in)
			if _, err := f.s.Update(ctx, inc.ID, in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			v, _ := f.s.Month(ctx, month(2026, time.December))
			if v.ReceivedCents != 100_000 && v.ReceivedCents != 450_000 {
				t.Fatalf("december received = %d, the confirmation was lost", v.ReceivedCents)
			}
		})
	}
}

// TestMonthIncludesOneOff lists one off incomes with the confirmed ones (RF-13).
func TestMonthIncludesOneOff(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	if _, err := f.ledger.AddIncome(ctx, f.userID, ledger.NewIncome{AccountID: f.inter, AmountCents: 20_000, Date: date(2026, time.November, 3), Description: "Venda de bicicleta"}); err != nil {
		t.Fatal(err)
	}
	v, err := f.s.Month(ctx, month(2026, time.November))
	if err != nil {
		t.Fatal(err)
	}
	if len(v.OneOffs) != 1 || v.OneOffs[0].Description != "Venda de bicicleta" || v.ReceivedCents != 20_000 {
		t.Fatalf("view = %+v", v)
	}
}

// TestEndingSoon warns when the end date of an income is within 30 days (RF-12).
func TestEndingSoon(t *testing.T) {
	f := newFixture(t) // today is 2026-11-11
	ctx := groupCtx
	tests := []struct {
		name      string
		end       *civil.Date
		wantSoon  bool
		wantEnded bool
	}{
		{name: "no end date"},
		{name: "ends today", end: ptr(date(2026, time.November, 11)), wantSoon: true},
		{name: "ends in 30 days", end: ptr(date(2026, time.December, 11)), wantSoon: true},
		{name: "ends in 31 days", end: ptr(date(2026, time.December, 12))},
		{name: "already ended", end: ptr(date(2026, time.November, 10)), wantEnded: true},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := f.salary()
			in.Name = "Renda " + string(rune('A'+i))
			in.EndOn = tt.end
			got, err := f.s.Create(ctx, f.userID, in)
			if err != nil {
				t.Fatal(err)
			}
			if got.EndingSoon != tt.wantSoon || got.Ended != tt.wantEnded {
				t.Fatalf("soon, ended = %v, %v; want %v, %v", got.EndingSoon, got.Ended, tt.wantSoon, tt.wantEnded)
			}
		})
	}
}

// TestUpdateAndDelete edits incomes and keeps those with receipts (RF-12).
func TestUpdateAndDelete(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	salary, _ := f.s.Create(ctx, f.userID, f.salary())
	unused, _ := f.s.Create(ctx, f.userID, Input{Kind: KindEventual, Name: "Férias", PersonID: f.persons[0], AccountID: f.nubank, AmountCents: 100, ExpectedOn: date(2027, time.January, 15)})
	if _, err := f.s.Confirm(ctx, f.userID, Confirmation{IncomeID: salary.ID, Month: month(2026, time.November)}); err != nil {
		t.Fatal(err)
	}

	raise := f.salary()
	raise.AmountCents = 480_000
	raise.EndOn = ptr(date(2027, time.June, 30))
	got, err := f.s.Update(ctx, salary.ID, raise)
	if err != nil || got.AmountCents != 480_000 || got.EndOn == nil {
		t.Fatalf("update = %+v, %v", got, err)
	}
	// The confirmed month keeps the amount received; the next one uses the new amount.
	nov, _ := f.s.Month(ctx, month(2026, time.November))
	dec, _ := f.s.Month(ctx, month(2026, time.December))
	if nov.ReceivedCents != 450_000 || dec.PendingCents != 480_000 {
		t.Fatalf("nov received %d, dec pending %d", nov.ReceivedCents, dec.PendingCents)
	}
	changeKind := raise
	changeKind.Kind = KindEventual
	if _, err := f.s.Update(ctx, salary.ID, changeKind); !errors.Is(err, ErrKindChange) {
		t.Fatalf("kind change: err = %v", err)
	}

	deletes := []struct {
		name    string
		id      int64
		wantErr error
	}{
		{name: "with receipts", id: salary.ID, wantErr: ErrInUse},
		{name: "without receipts", id: unused.ID},
		{name: "missing", id: 999, wantErr: ErrNotFound},
	}
	for _, tt := range deletes {
		t.Run(tt.name, func(t *testing.T) {
			if err := f.s.Delete(ctx, tt.id); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestGroupIsolation shows that a second group sees none of the incomes of the first (RF-82, RN-27).
func TestGroupIsolation(t *testing.T) {
	f := newFixture(t)
	inc, err := f.s.Create(groupCtx, f.userID, f.salary())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Confirm(groupCtx, f.userID, Confirmation{IncomeID: inc.ID, Month: month(2026, time.November)}); err != nil {
		t.Fatal(err)
	}
	carla := testutil.NewUser(t, f.conn, "carla", "Carla Ficticia")
	other := testutil.NewGroup(t, f.conn, "Outra", carla)

	tests := []struct {
		name    string
		run     func() error
		wantErr error
	}{
		{name: "get", run: func() error { _, err := f.s.Get(other, inc.ID); return err }, wantErr: ErrNotFound},
		{name: "update", run: func() error { _, err := f.s.Update(other, inc.ID, f.salary()); return err }, wantErr: ErrNotFound},
		{name: "delete", run: func() error { return f.s.Delete(other, inc.ID) }, wantErr: ErrNotFound},
		{name: "confirm", run: func() error {
			_, err := f.s.Confirm(other, carla.ID, Confirmation{IncomeID: inc.ID, Month: month(2026, time.November)})
			return err
		}, wantErr: ErrNotFound},
		{name: "holder from another group", run: func() error { _, err := f.s.Create(other, carla.ID, f.salary()); return err }, wantErr: ErrPersonNotFound},
		{name: "no active group", run: func() error { _, err := f.s.List(context.Background()); return err }, wantErr: scope.ErrNoGroup},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
	if list, err := f.s.List(other); err != nil || len(list) != 0 {
		t.Fatalf("incomes of the other group = %v, %v", list, err)
	}
	view, err := f.s.Month(other, month(2026, time.November))
	if err != nil || len(view.Occurrences) != 0 || len(view.OneOffs) != 0 || view.ReceivedCents != 0 {
		t.Fatalf("month of the other group = %+v, %v", view, err)
	}
}
