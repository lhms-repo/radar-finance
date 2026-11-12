package bill

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/category"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/ledger"
)

// fixture holds a migrated database where today is 2026-11-11, with a user, two accounts and the services.
type fixture struct {
	s       *Service
	ledger  *ledger.Service
	conn    *sql.DB
	userID  int64
	nubank  int64
	inter   int64
	energia int64
	agua    int64
}

// newFixture opens a fresh database for one test.
func newFixture(t *testing.T) fixture {
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
	persons, _ := users.ListPersons(ctx)
	l := ledger.NewService(conn)
	nubank, err := l.CreateAccount(ctx, ana.ID, ledger.NewAccount{Name: "Nubank Ana", PersonID: persons[0].ID, InitialBalanceCents: 500_000})
	if err != nil {
		t.Fatal(err)
	}
	inter, _ := l.CreateAccount(ctx, ana.ID, ledger.NewAccount{Name: "Inter Ana", PersonID: persons[0].ID})
	f := fixture{s: NewService(conn), ledger: l, conn: conn, userID: ana.ID, nubank: nubank.ID, inter: inter.ID}
	f.s.today = func() civil.Date { return civil.Date{Year: 2026, Month: time.November, Day: 11} }
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Energia'`).Scan(&f.energia)
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Água'`).Scan(&f.agua)
	return f
}

// month builds a civil month.
func month(y int, m time.Month) civil.Month { return civil.Month{Year: y, Month: m} }

// date builds a civil date.
func date(y int, m time.Month, d int) civil.Date { return civil.Date{Year: y, Month: m, Day: d} }

// power returns a valid bill input: power bill due on the 15th, expected R$ 180,00.
func (f fixture) power() Input {
	return Input{Name: "Energia", CategoryID: f.energia, AccountID: f.nubank, DueDay: 15, ExpectedCents: 18_000, StartMonth: month(2026, time.November)}
}

// TestCreate validates the bill registration (RF-35).
func TestCreate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var topLevel int64
	_ = f.conn.QueryRow(`SELECT id FROM categories WHERE name = 'Moradia'`).Scan(&topLevel)
	tests := []struct {
		name    string
		mod     func(*Input)
		wantErr error
	}{
		{name: "valid", mod: func(*Input) {}},
		{name: "with last month", mod: func(in *Input) { in.EndMonth = &civil.Month{Year: 2027, Month: time.June} }},
		{name: "empty name", mod: func(in *Input) { in.Name = " " }, wantErr: ErrNameRequired},
		{name: "top level category", mod: func(in *Input) { in.CategoryID = topLevel }, wantErr: category.ErrNotSelectable},
		{name: "missing account", mod: func(in *Input) { in.AccountID = 999 }, wantErr: ledger.ErrAccountNotFound},
		{name: "day 0", mod: func(in *Input) { in.DueDay = 0 }, wantErr: ErrInvalidDay},
		{name: "day 32", mod: func(in *Input) { in.DueDay = 32 }, wantErr: ErrInvalidDay},
		{name: "zero expected", mod: func(in *Input) { in.ExpectedCents = 0 }, wantErr: ledger.ErrAmountRequired},
		{name: "missing start", mod: func(in *Input) { in.StartMonth = civil.Month{} }, wantErr: ErrStartRequired},
		{name: "end before start", mod: func(in *Input) { in.EndMonth = &civil.Month{Year: 2026, Month: time.October} }, wantErr: ErrEndBeforeStart},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := f.power()
			tt.mod(&in)
			b, err := f.s.Create(ctx, f.userID, in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (b.Name != "Energia" || b.AccountName != "Nubank Ana" || b.CategoryName != "Energia") {
				t.Fatalf("bill = %+v", b)
			}
		})
	}
}

// TestMonth derives the bills of each month, using the expected amount until the real one is informed (RF-36, RN-14).
func TestMonth(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	power, _ := f.s.Create(ctx, f.userID, f.power())
	water, _ := f.s.Create(ctx, f.userID, Input{Name: "Água", CategoryID: f.agua, AccountID: f.nubank, DueDay: 31, ExpectedCents: 9_000, StartMonth: month(2026, time.December), EndMonth: &civil.Month{Year: 2027, Month: time.February}})
	if err := f.s.SetActual(ctx, power.ID, month(2026, time.December), 21_337); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		month civil.Month
		want  map[int64][2]any // bill id to due date and amount
		total int64
	}{
		{name: "before any start", month: month(2026, time.October), want: map[int64][2]any{}},
		{name: "power only, expected amount", month: month(2026, time.November), want: map[int64][2]any{power.ID: {"2026-11-15", int64(18_000)}}, total: 18_000},
		{name: "real amount informed, day 31", month: month(2026, time.December), want: map[int64][2]any{power.ID: {"2026-12-15", int64(21_337)}, water.ID: {"2026-12-31", int64(9_000)}}, total: 30_337},
		{name: "February clamps to the 28th", month: month(2027, time.February), want: map[int64][2]any{power.ID: {"2027-02-15", int64(18_000)}, water.ID: {"2027-02-28", int64(9_000)}}, total: 27_000},
		{name: "water ended", month: month(2027, time.March), want: map[int64][2]any{power.ID: {"2027-03-15", int64(18_000)}}, total: 18_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := f.s.Month(ctx, tt.month)
			if err != nil {
				t.Fatal(err)
			}
			if len(v.Bills) != len(tt.want) || v.TotalCents != tt.total || v.PaidCount != 0 {
				t.Fatalf("view = %+v", v)
			}
			for _, o := range v.Bills {
				w := tt.want[o.BillID]
				if o.DueOn.String() != w[0] || o.AmountCents != w[1] || o.ExpectedCents == 0 {
					t.Fatalf("bill %d = %+v, want %v", o.BillID, o, w)
				}
			}
		})
	}
}

// TestUpdateKeepsPaidMonths refuses changes that leave a paid month outside the bill (RF-37).
func TestUpdateKeepsPaidMonths(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		edit    func(in *Input)
		wantErr error
	}{
		{name: "start after the paid month", edit: func(in *Input) { in.StartMonth = month(2027, time.January) }, wantErr: ErrPaidOutside},
		{name: "end before the paid month", edit: func(in *Input) { in.EndMonth = &civil.Month{Year: 2026, Month: time.November} }, wantErr: ErrPaidOutside},
		{name: "end on the paid month", edit: func(in *Input) { in.EndMonth = &civil.Month{Year: 2026, Month: time.December} }},
		{name: "day and amount changed", edit: func(in *Input) { in.DueDay, in.ExpectedCents = 20, 19_000 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			in := f.power()
			b, err := f.s.Create(ctx, f.userID, in)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.Pay(ctx, f.userID, Payment{BillID: b.ID, Month: month(2026, time.December)}); err != nil {
				t.Fatal(err)
			}
			tt.edit(&in)
			if _, err := f.s.Update(ctx, b.ID, in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestPay records the payment as spending of the bill category and as the real amount of the month (RF-37).
func TestPay(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	power, _ := f.s.Create(ctx, f.userID, f.power())
	nov := month(2026, time.November)

	tests := []struct {
		name        string
		in          Payment
		wantErr     error
		wantAmount  int64
		wantAccount int64
		wantDate    string
	}{
		{name: "month without the bill", in: Payment{BillID: power.ID, Month: month(2026, time.October)}, wantErr: ErrNoOccurrence},
		{name: "missing bill", in: Payment{BillID: 999, Month: nov}, wantErr: ErrNotFound},
		{name: "defaults to expected amount, bill account and today", in: Payment{BillID: power.ID, Month: nov}, wantAmount: 18_000, wantAccount: f.nubank, wantDate: "2026-11-11"},
		{name: "twice", in: Payment{BillID: power.ID, Month: nov}, wantErr: ErrAlreadyPaid},
		{
			name: "other amount, account and date", in: Payment{BillID: power.ID, Month: month(2026, time.December), AmountCents: 19_990, AccountID: f.inter, Date: date(2026, time.December, 14)},
			wantAmount: 19_990, wantAccount: f.inter, wantDate: "2026-12-14",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := f.s.Pay(ctx, f.userID, tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if e.Kind != ledger.KindExpense || e.AmountCents != -tt.wantAmount || e.AccountID != tt.wantAccount || e.Date.String() != tt.wantDate || e.CategoryName != "Energia" {
				t.Fatalf("entry = %+v", e)
			}
		})
	}

	if a, _ := f.ledger.GetAccount(ctx, f.nubank); a.BalanceCents != 500_000-18_000 {
		t.Fatalf("balance = %d", a.BalanceCents)
	}
	if flow, _ := f.ledger.MonthFlow(ctx, nov); flow.ExpenseCents != 18_000 {
		t.Fatalf("bill payment must count as spending: %+v", flow)
	}
	dec, _ := f.s.Month(ctx, month(2026, time.December))
	if o := dec.Bills[0]; !o.Paid || o.ActualCents == nil || *o.ActualCents != 19_990 || o.PaidFrom != "Inter Ana" || dec.PaidCount != 1 {
		t.Fatalf("december = %+v", o)
	}

	// Deleting the entry undoes the payment; the real amount stays.
	if err := f.ledger.DeleteEntry(ctx, *dec.Bills[0].EntryID); err != nil {
		t.Fatal(err)
	}
	dec, _ = f.s.Month(ctx, month(2026, time.December))
	if o := dec.Bills[0]; o.Paid || o.AmountCents != 19_990 {
		t.Fatalf("after undo = %+v", o)
	}
}

// TestSetActual informs and clears the real amount of a month (RF-36, RN-14).
func TestSetActual(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	power, _ := f.s.Create(ctx, f.userID, f.power())
	nov := month(2026, time.November)
	tests := []struct {
		name    string
		bill    int64
		month   civil.Month
		cents   int64
		want    int64
		wantErr error
	}{
		{name: "inform real", bill: power.ID, month: nov, cents: 17_450, want: 17_450},
		{name: "clear goes back to expected", bill: power.ID, month: nov, cents: 0, want: 18_000},
		{name: "negative", bill: power.ID, month: nov, cents: -1, wantErr: ledger.ErrAmountRequired},
		{name: "month without the bill", bill: power.ID, month: month(2026, time.October), cents: 1, wantErr: ErrNoOccurrence},
		{name: "missing bill", bill: 999, month: nov, cents: 1, wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := f.s.SetActual(ctx, tt.bill, tt.month, tt.cents); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			v, _ := f.s.Month(ctx, nov)
			if v.Bills[0].AmountCents != tt.want || v.Bills[0].ExpectedCents != 18_000 {
				t.Fatalf("bill = %+v", v.Bills[0])
			}
		})
	}
}

// TestUpdateAndDelete changes the expected amount of months without a real one and keeps paid bills.
func TestUpdateAndDelete(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	power, _ := f.s.Create(ctx, f.userID, f.power())
	unused, _ := f.s.Create(ctx, f.userID, Input{Name: "Água", CategoryID: f.agua, AccountID: f.nubank, DueDay: 10, ExpectedCents: 9_000, StartMonth: month(2026, time.November)})
	if _, err := f.s.Pay(ctx, f.userID, Payment{BillID: power.ID, Month: month(2026, time.November)}); err != nil {
		t.Fatal(err)
	}
	in := f.power()
	in.ExpectedCents = 20_000
	if _, err := f.s.Update(ctx, power.ID, in); err != nil {
		t.Fatal(err)
	}
	nov, _ := f.s.Month(ctx, month(2026, time.November))
	dec, _ := f.s.Month(ctx, month(2026, time.December))
	if nov.Bills[1].AmountCents != 18_000 || dec.Bills[1].AmountCents != 20_000 {
		t.Fatalf("nov %+v, dec %+v", nov.Bills[1], dec.Bills[1])
	}
	deletes := []struct {
		name    string
		id      int64
		wantErr error
	}{
		{name: "paid bill", id: power.ID, wantErr: ErrInUse},
		{name: "unpaid bill", id: unused.ID},
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
