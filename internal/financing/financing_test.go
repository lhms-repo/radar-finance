package financing

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/category"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/scope"
	"github.com/italogfx/radar-finance/internal/testutil"
)

// fixture holds a migrated database where today is 2026-11-11, with a user, an account and the services.
type fixture struct {
	s         *Service
	ledger    *ledger.Service
	conn      *sql.DB
	userID    int64
	accountID int64
	house     int64
}

// groupCtx acts on the first group of a fresh database, created by every fixture.
var groupCtx = scope.WithGroup(context.Background(), 1)

// newFixture opens a fresh database for one test.
func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := groupCtx
	conn := testutil.OpenDB(t)
	ana := testutil.NewUser(t, conn, "ana", "Ana Ficticia")
	bruno := testutil.NewUser(t, conn, "bruno", "Bruno Ficticio")
	testutil.FirstGroup(t, conn, ana, bruno)
	persons := []account.Person{{ID: ana.PersonID}, {ID: bruno.PersonID}}
	l := ledger.NewService(conn)
	acc, err := l.CreateAccount(ctx, ana.ID, ledger.NewAccount{Name: "Caixa Ana", PersonID: persons[0].ID, InitialBalanceCents: 1_000_000})
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{s: NewService(conn), ledger: l, conn: conn, userID: ana.ID, accountID: acc.ID}
	f.s.today = func() civil.Date { return civil.Date{Year: 2026, Month: time.November, Day: 11} }
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Financiamento da casa'`).Scan(&f.house)
	return f
}

// month builds a civil month.
func month(y int, m time.Month) civil.Month { return civil.Month{Year: y, Month: m} }

// houseLoan returns a valid input: 360 installments of R$ 1.500,00, 24 paid, R$ 280.000,00 left, next one in November.
func (f fixture) houseLoan() Input {
	return Input{
		Name: "Casa", CategoryID: f.house, AccountID: f.accountID, DueDay: 20, InstallmentCents: 150_000,
		TotalInstallments: 360, PaidInstallments: 24, BalanceCents: 28_000_000, NextMonth: month(2026, time.November),
	}
}

// TestCreate validates the financing registration (RF-39).
func TestCreate(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	tests := []struct {
		name    string
		mod     func(*Input)
		wantErr error
	}{
		{name: "valid", mod: func(*Input) {}},
		{name: "nothing paid yet", mod: func(in *Input) { in.PaidInstallments = 0 }},
		{name: "empty name", mod: func(in *Input) { in.Name = "" }, wantErr: ErrNameRequired},
		{name: "zero installment", mod: func(in *Input) { in.InstallmentCents = 0 }, wantErr: ledger.ErrAmountRequired},
		{name: "zero total", mod: func(in *Input) { in.TotalInstallments = 0 }, wantErr: ErrInvalidTotal},
		{name: "more paid than total", mod: func(in *Input) { in.PaidInstallments = 361 }, wantErr: ErrInvalidPaid},
		{name: "negative balance", mod: func(in *Input) { in.BalanceCents = -1 }, wantErr: ErrInvalidBalance},
		{name: "day 32", mod: func(in *Input) { in.DueDay = 32 }, wantErr: ErrInvalidDay},
		{name: "missing next month", mod: func(in *Input) { in.NextMonth = civil.Month{} }, wantErr: ErrStartRequired},
		{name: "category not selectable", mod: func(in *Input) { in.CategoryID = 9999 }, wantErr: category.ErrNotSelectable},
		{name: "missing account", mod: func(in *Input) { in.AccountID = 999 }, wantErr: ledger.ErrAccountNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := f.houseLoan()
			tt.mod(&in)
			got, err := f.s.Create(ctx, f.userID, in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (got.BalanceCents != in.BalanceCents || got.PaidInstallments != in.PaidInstallments || got.RemainingInstallments != 360-in.PaidInstallments) {
				t.Fatalf("financing = %+v", got)
			}
		})
	}
}

// TestMonth turns each installment into a bill to pay, numbered, until the last one (RF-40).
func TestMonth(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	car, err := f.s.Create(ctx, f.userID, Input{
		Name: "Carro", CategoryID: f.house, AccountID: f.accountID, DueDay: 31, InstallmentCents: 90_000,
		TotalInstallments: 48, PaidInstallments: 45, BalanceCents: 270_000, NextMonth: month(2026, time.November),
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		month  civil.Month
		number int
		due    string
	}{
		{month: month(2026, time.October)},
		{month: month(2026, time.November), number: 46, due: "2026-11-30"},
		{month: month(2027, time.January), number: 48, due: "2027-01-31"},
		{month: month(2027, time.February)},
	}
	for _, tt := range tests {
		t.Run(tt.month.String(), func(t *testing.T) {
			v, err := f.s.Month(ctx, tt.month)
			if err != nil {
				t.Fatal(err)
			}
			if tt.number == 0 {
				if len(v) != 0 {
					t.Fatalf("occurrences = %+v", v)
				}
				return
			}
			if len(v) != 1 || v[0].FinancingID != car.ID || v[0].Number != tt.number || v[0].Total != 48 || v[0].DueOn.String() != tt.due || v[0].AmountCents != 90_000 {
				t.Fatalf("occurrence = %+v", v)
			}
		})
	}
}

// TestPay lowers the balance to the value the user informs, suggested as balance minus installment (RF-41, RN-16).
func TestPay(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	loan, _ := f.s.Create(ctx, f.userID, f.houseLoan())
	nov, dec := month(2026, time.November), month(2026, time.December)

	v, _ := f.s.Month(ctx, nov)
	if v[0].SuggestedBalanceCents != 28_000_000-150_000 {
		t.Fatalf("suggested = %d", v[0].SuggestedBalanceCents)
	}

	tests := []struct {
		name        string
		in          Payment
		wantErr     error
		wantBalance int64
		wantPaid    int
	}{
		{name: "month before the next installment", in: Payment{FinancingID: loan.ID, Month: month(2026, time.October)}, wantErr: ErrNoOccurrence},
		{name: "negative balance after", in: Payment{FinancingID: loan.ID, Month: nov, BalanceAfterCents: ptr(int64(-1))}, wantErr: ErrInvalidBalance},
		{name: "suggested balance by default", in: Payment{FinancingID: loan.ID, Month: nov}, wantBalance: 27_850_000, wantPaid: 25},
		{name: "same month twice", in: Payment{FinancingID: loan.ID, Month: nov}, wantErr: ErrAlreadyPaid},
		{name: "balance informed by the bank", in: Payment{FinancingID: loan.ID, Month: dec, AmountCents: 151_234, Date: civil.Date{Year: 2026, Month: time.December, Day: 20}, BalanceAfterCents: ptr(int64(27_760_500))}, wantBalance: 27_760_500, wantPaid: 26},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.s.Pay(ctx, f.userID, tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			got, _ := f.s.Get(ctx, loan.ID)
			if got.BalanceCents != tt.wantBalance || got.PaidInstallments != tt.wantPaid || got.RemainingInstallments != 360-tt.wantPaid {
				t.Fatalf("financing = %+v", got)
			}
		})
	}

	if a, _ := f.ledger.GetAccount(ctx, f.accountID); a.BalanceCents != 1_000_000-150_000-151_234 {
		t.Fatalf("account balance = %d", a.BalanceCents)
	}
	if flow, _ := f.ledger.MonthFlow(ctx, nov); flow.ExpenseCents != 150_000 {
		t.Fatalf("installment must count as spending: %+v", flow)
	}

	// Deleting the December payment entry brings the balance and the count back.
	v, _ = f.s.Month(ctx, dec)
	if !v[0].Paid || v[0].PaidCents != 151_234 {
		t.Fatalf("december = %+v", v[0])
	}
	if err := f.ledger.DeleteEntry(ctx, *v[0].EntryID); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.s.Get(ctx, loan.ID); got.BalanceCents != 27_850_000 || got.PaidInstallments != 25 {
		t.Fatalf("after undo = %+v", got)
	}
}

// TestCorrect fixes the balance and the installment by hand, and later payments start from it (RF-42).
func TestCorrect(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	loan, _ := f.s.Create(ctx, f.userID, f.houseLoan())
	tests := []struct {
		name    string
		in      Correction
		wantErr error
	}{
		{name: "new balance and installment", in: Correction{FinancingID: loan.ID, BalanceCents: 27_500_000, InstallmentCents: 148_900}},
		{name: "negative balance", in: Correction{FinancingID: loan.ID, BalanceCents: -1, InstallmentCents: 1}, wantErr: ErrInvalidBalance},
		{name: "zero installment", in: Correction{FinancingID: loan.ID, BalanceCents: 1}, wantErr: ledger.ErrAmountRequired},
		{name: "missing financing", in: Correction{FinancingID: 999, BalanceCents: 1, InstallmentCents: 1}, wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.s.Correct(ctx, f.userID, tt.in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
	v, _ := f.s.Month(ctx, month(2026, time.November))
	if v[0].AmountCents != 148_900 || v[0].SuggestedBalanceCents != 27_500_000-148_900 {
		t.Fatalf("after correction = %+v", v[0])
	}
}

// TestPaidOff stops the installments after the last one and keeps financings with history.
func TestPaidOff(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	solar, _ := f.s.Create(ctx, f.userID, Input{
		Name: "Energia solar", CategoryID: f.house, AccountID: f.accountID, DueDay: 5, InstallmentCents: 50_000,
		TotalInstallments: 10, PaidInstallments: 9, BalanceCents: 50_000, NextMonth: month(2026, time.November),
	})
	if _, err := f.s.Pay(ctx, f.userID, Payment{FinancingID: solar.ID, Month: month(2026, time.November)}); err != nil {
		t.Fatal(err)
	}
	got, _ := f.s.Get(ctx, solar.ID)
	if !got.Done || got.RemainingInstallments != 0 || got.BalanceCents != 0 || got.NextMonth != nil {
		t.Fatalf("paid off = %+v", got)
	}
	if _, err := f.s.Pay(ctx, f.userID, Payment{FinancingID: solar.ID, Month: month(2026, time.December)}); !errors.Is(err, ErrNoOccurrence) {
		t.Fatalf("payment after the last installment: %v", err)
	}
	if err := f.s.Delete(ctx, solar.ID); !errors.Is(err, ErrInUse) {
		t.Fatalf("delete with history: %v", err)
	}
}

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// TestGroupIsolation shows that a second group sees none of the financings of the first (RF-82, RN-27).
func TestGroupIsolation(t *testing.T) {
	f := newFixture(t)
	loan, err := f.s.Create(groupCtx, f.userID, f.houseLoan())
	if err != nil {
		t.Fatal(err)
	}
	nov := month(2026, time.November)
	if _, err := f.s.Pay(groupCtx, f.userID, Payment{FinancingID: loan.ID, Month: nov}); err != nil {
		t.Fatal(err)
	}
	carla := testutil.NewUser(t, f.conn, "carla", "Carla Ficticia")
	other := testutil.NewGroup(t, f.conn, "Outra", carla)

	tests := []struct {
		name    string
		run     func() error
		wantErr error
	}{
		{name: "get", run: func() error { _, err := f.s.Get(other, loan.ID); return err }, wantErr: ErrNotFound},
		{name: "update", run: func() error {
			_, err := f.s.Update(other, loan.ID, Changes{Name: "X", CategoryID: f.house, AccountID: f.accountID, DueDay: 5})
			return err
		}, wantErr: ErrNotFound},
		{name: "delete", run: func() error { return f.s.Delete(other, loan.ID) }, wantErr: ErrNotFound},
		{name: "pay", run: func() error {
			_, err := f.s.Pay(other, carla.ID, Payment{FinancingID: loan.ID, Month: month(2026, time.December)})
			return err
		}, wantErr: ErrNotFound},
		{name: "correct", run: func() error {
			_, err := f.s.Correct(other, carla.ID, Correction{FinancingID: loan.ID, BalanceCents: 100, InstallmentCents: 100})
			return err
		}, wantErr: ErrNotFound},
		{name: "account of another group", run: func() error { _, err := f.s.Create(other, carla.ID, f.houseLoan()); return err }, wantErr: ledger.ErrAccountNotFound},
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
		t.Fatalf("financings of the other group = %v, %v", list, err)
	}
	if occ, err := f.s.Month(other, nov); err != nil || len(occ) != 0 {
		t.Fatalf("month of the other group = %v, %v", occ, err)
	}
}
