package ledger

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/category"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/scope"
	"github.com/italogfx/radar-finance/internal/testutil"
)

// fixture holds a migrated database with two fictional users and the service.
type fixture struct {
	s       *Service
	conn    *sql.DB
	userID  int64
	persons [2]int64
	// delivery is an active subcategory used for expenses.
	delivery int64
}

// groupCtx acts on the first group of a fresh database, created by every fixture.
var groupCtx = scope.WithGroup(context.Background(), 1)

// newFixture opens a fresh migrated database for one test, with the group "Casa" of two fictional users.
func newFixture(t *testing.T) fixture {
	t.Helper()
	conn := testutil.OpenDB(t)
	ana := testutil.NewUser(t, conn, "ana", "Ana Ficticia")
	bruno := testutil.NewUser(t, conn, "bruno", "Bruno Ficticio")
	if id, _ := scope.Group(testutil.NewGroup(t, conn, "Casa", ana, bruno)); id != 1 {
		t.Fatalf("first group id = %d, want 1", id)
	}
	persons := []account.Person{{ID: ana.PersonID}, {ID: bruno.PersonID}}
	f := fixture{s: NewService(conn), conn: conn, userID: ana.ID, persons: [2]int64{persons[0].ID, persons[1].ID}}
	if err := conn.QueryRow(`SELECT id FROM categories WHERE name = 'Delivery'`).Scan(&f.delivery); err != nil {
		t.Fatal(err)
	}
	return f
}

// day parses an ISO date or fails the test.
func day(t *testing.T, s string) civil.Date {
	t.Helper()
	d, err := civil.ParseDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// newAccount creates an account for the first person with the initial balance.
func (f fixture) newAccount(t *testing.T, name string, initial int64) Account {
	t.Helper()
	a, err := f.s.CreateAccount(groupCtx, f.userID, NewAccount{Name: name, PersonID: f.persons[0], InitialBalanceCents: initial})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// balance returns the current balance of an account.
func (f fixture) balance(t *testing.T, id int64) int64 {
	t.Helper()
	a, err := f.s.GetAccount(groupCtx, id)
	if err != nil {
		t.Fatal(err)
	}
	return a.BalanceCents
}

// insertIncome writes an income entry directly, standing in for the income step (2.3).
func (f fixture) insertIncome(t *testing.T, accountID, cents int64, on string, confirmed bool) {
	t.Helper()
	_, err := f.conn.Exec(`INSERT INTO entries (account_id, kind, amount_cents, occurred_on, confirmed, created_by, created_at, updated_at, group_id)
		VALUES (?, 'income', ?, ?, ?, ?, ?, ?, 1)`, accountID, cents, on, confirmed, f.userID, time.Now(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
}

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// TestCreateAccount covers account registration with holder and initial balance (RF-04).
func TestCreateAccount(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	f.newAccount(t, "Nubank Ana", 0)

	tests := []struct {
		name    string
		in      NewAccount
		wantErr error
	}{
		{name: "positive initial balance", in: NewAccount{Name: " Inter ", PersonID: f.persons[1], InitialBalanceCents: 152_034}},
		{name: "negative initial balance", in: NewAccount{Name: "Caixa", PersonID: f.persons[0], InitialBalanceCents: -5_000}},
		{name: "duplicate name ignoring case", in: NewAccount{Name: "NUBANK ana", PersonID: f.persons[1]}, wantErr: ErrDuplicateAccount},
		{name: "empty name", in: NewAccount{Name: "  ", PersonID: f.persons[0]}, wantErr: ErrAccountNameRequired},
		{name: "name too long", in: NewAccount{Name: strings.Repeat("x", 41), PersonID: f.persons[0]}, wantErr: ErrAccountNameTooLong},
		{name: "unknown holder", in: NewAccount{Name: "Itaú", PersonID: 999}, wantErr: ErrPersonNotFound},
		{name: "absurd initial balance", in: NewAccount{Name: "Itaú", PersonID: f.persons[0], InitialBalanceCents: MaxAmountCents + 1}, wantErr: ErrAmountTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := f.s.CreateAccount(ctx, f.userID, tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if a.Name != strings.TrimSpace(tt.in.Name) || a.BalanceCents != tt.in.InitialBalanceCents || a.PersonID != tt.in.PersonID || a.PersonName == "" {
				t.Fatalf("account = %+v", a)
			}
		})
	}
}

// TestBalance checks that only confirmed entries change the balance (RF-05, RN-13).
func TestBalance(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	a := f.newAccount(t, "Nubank Ana", 100_000)

	steps := []struct {
		name string
		do   func(t *testing.T)
		want int64
	}{
		{name: "initial balance", do: func(*testing.T) {}, want: 100_000},
		{
			name: "one off expense", want: 84_990,
			do: func(t *testing.T) {
				if _, err := f.s.AddExpense(ctx, f.userID, NewExpense{AccountID: a.ID, AmountCents: 15_010, Date: day(t, "2026-11-05"), CategoryID: f.delivery}); err != nil {
					t.Fatal(err)
				}
			},
		},
		{name: "planned income does not count", do: func(t *testing.T) { f.insertIncome(t, a.ID, 500_000, "2026-11-30", false) }, want: 84_990},
		{name: "confirmed income counts", do: func(t *testing.T) { f.insertIncome(t, a.ID, 320_000, "2026-11-05", true) }, want: 404_990},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			st.do(t)
			if got := f.balance(t, a.ID); got != st.want {
				t.Fatalf("balance = %d, want %d", got, st.want)
			}
		})
	}
	list, err := f.s.ListAccounts(ctx)
	if err != nil || len(list) != 1 || list[0].BalanceCents != 404_990 {
		t.Fatalf("list = %+v, %v", list, err)
	}
}

// TestAdjustBalance records the difference to the informed balance as an adjustment (RF-06).
func TestAdjustBalance(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	a := f.newAccount(t, "Nubank Ana", 100_000)
	closed := f.newAccount(t, "Antiga", 0)
	if _, err := f.s.UpdateAccount(ctx, closed.ID, AccountChanges{Active: ptr(false)}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		account    int64
		target     int64
		wantAmount int64
		wantErr    error
	}{
		{name: "balance higher than recorded", account: a.ID, target: 123_456, wantAmount: 23_456},
		{name: "balance lower than recorded", account: a.ID, target: 23_456, wantAmount: -100_000},
		{name: "balance goes negative", account: a.ID, target: -1_000, wantAmount: -24_456},
		{name: "same balance", account: a.ID, target: -1_000, wantErr: ErrNoChange},
		{name: "inactive account", account: closed.ID, target: 10, wantErr: ErrAccountInactive},
		{name: "missing account", account: 999, target: 10, wantErr: ErrAccountNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := f.s.AdjustBalance(ctx, f.userID, Adjustment{AccountID: tt.account, BalanceCents: tt.target, Date: day(t, "2026-11-11")})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if e.Kind != KindAdjustment || e.AmountCents != tt.wantAmount || !e.Confirmed {
				t.Fatalf("entry = %+v", e)
			}
			if got := f.balance(t, tt.account); got != tt.target {
				t.Fatalf("balance = %d, want %d", got, tt.target)
			}
		})
	}
}

// TestTransfer moves money between accounts without counting it as spending or income (RF-07, RN-08).
func TestTransfer(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	from := f.newAccount(t, "Nubank Ana", 100_000)
	to := f.newAccount(t, "Inter Bruno", 5_000)
	closed := f.newAccount(t, "Antiga", 0)
	if _, err := f.s.UpdateAccount(ctx, closed.ID, AccountChanges{Active: ptr(false)}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		in      NewTransfer
		wantErr error
	}{
		{name: "same account", in: NewTransfer{FromAccountID: from.ID, ToAccountID: from.ID, AmountCents: 100}, wantErr: ErrSameAccount},
		{name: "zero amount", in: NewTransfer{FromAccountID: from.ID, ToAccountID: to.ID}, wantErr: ErrAmountRequired},
		{name: "negative amount", in: NewTransfer{FromAccountID: from.ID, ToAccountID: to.ID, AmountCents: -5}, wantErr: ErrAmountRequired},
		{name: "inactive destination", in: NewTransfer{FromAccountID: from.ID, ToAccountID: closed.ID, AmountCents: 100}, wantErr: ErrAccountInactive},
		{name: "missing origin", in: NewTransfer{FromAccountID: 999, ToAccountID: to.ID, AmountCents: 100}, wantErr: ErrAccountNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Date = day(t, "2026-11-11")
			if _, err := f.s.Transfer(ctx, f.userID, tt.in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}

	tr, err := f.s.Transfer(ctx, f.userID, NewTransfer{FromAccountID: from.ID, ToAccountID: to.ID, AmountCents: 30_000, Date: day(t, "2026-11-11"), Description: "Divisão do aluguel"})
	if err != nil {
		t.Fatal(err)
	}
	if f.balance(t, from.ID) != 70_000 || f.balance(t, to.ID) != 35_000 {
		t.Fatalf("balances = %d, %d", f.balance(t, from.ID), f.balance(t, to.ID))
	}
	flow, err := f.s.MonthFlow(ctx, civil.Month{Year: 2026, Month: time.November})
	if err != nil || flow != (Flow{}) {
		t.Fatalf("flow = %+v, %v; transfers are not income or spending (RN-08)", flow, err)
	}

	// Deleting either side removes the whole transfer.
	if err := f.s.DeleteEntry(ctx, tr.InEntryID); err != nil {
		t.Fatal(err)
	}
	if f.balance(t, from.ID) != 100_000 || f.balance(t, to.ID) != 5_000 {
		t.Fatalf("balances after delete = %d, %d", f.balance(t, from.ID), f.balance(t, to.ID))
	}
	var left int
	_ = f.conn.QueryRow(`SELECT count(*) FROM entries`).Scan(&left)
	if left != 0 {
		t.Fatalf("entries left = %d", left)
	}
}

// TestAddExpense covers one off expenses paid straight from an account (section 5.6).
func TestAddExpense(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	a := f.newAccount(t, "Nubank Ana", 100_000)
	closed := f.newAccount(t, "Antiga", 0)
	if _, err := f.s.UpdateAccount(ctx, closed.ID, AccountChanges{Active: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	var topLevel, inactiveSub int64
	_ = f.conn.QueryRow(`SELECT id FROM categories WHERE name = 'Alimentação'`).Scan(&topLevel)
	_ = f.conn.QueryRow(`SELECT id FROM categories WHERE name = 'Streaming'`).Scan(&inactiveSub)
	if _, err := category.NewService(f.conn).Update(ctx, inactiveSub, category.Changes{Active: ptr(false)}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		in      NewExpense
		wantErr error
	}{
		{name: "valid", in: NewExpense{AccountID: a.ID, AmountCents: 4_590, CategoryID: f.delivery, Description: " Pizza "}},
		{name: "zero amount", in: NewExpense{AccountID: a.ID, CategoryID: f.delivery}, wantErr: ErrAmountRequired},
		{name: "absurd amount", in: NewExpense{AccountID: a.ID, AmountCents: MaxAmountCents + 1, CategoryID: f.delivery}, wantErr: ErrAmountTooLarge},
		{name: "top level category", in: NewExpense{AccountID: a.ID, AmountCents: 100, CategoryID: topLevel}, wantErr: category.ErrNotSelectable},
		{name: "inactive category", in: NewExpense{AccountID: a.ID, AmountCents: 100, CategoryID: inactiveSub}, wantErr: category.ErrNotSelectable},
		{name: "missing category", in: NewExpense{AccountID: a.ID, AmountCents: 100, CategoryID: 9999}, wantErr: category.ErrNotSelectable},
		{name: "inactive account", in: NewExpense{AccountID: closed.ID, AmountCents: 100, CategoryID: f.delivery}, wantErr: ErrAccountInactive},
		{name: "description too long", in: NewExpense{AccountID: a.ID, AmountCents: 100, CategoryID: f.delivery, Description: strings.Repeat("a", 121)}, wantErr: ErrDescriptionTooLong},
		{name: "missing date", in: NewExpense{AccountID: a.ID, AmountCents: 100, CategoryID: f.delivery}, wantErr: ErrDateRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.wantErr, ErrDateRequired) {
				tt.in.Date = day(t, "2026-11-11")
			}
			e, err := f.s.AddExpense(ctx, f.userID, tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (e.AmountCents != -tt.in.AmountCents || e.Kind != KindExpense || e.Description != "Pizza") {
				t.Fatalf("entry = %+v", e)
			}
		})
	}
}

// TestMonthFlow counts only real income and spending of the month (RN-08, RN-13).
func TestMonthFlow(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	a := f.newAccount(t, "Nubank Ana", 100_000)
	b := f.newAccount(t, "Inter Bruno", 0)
	nov := civil.Month{Year: 2026, Month: time.November}

	f.insertIncome(t, a.ID, 400_000, "2026-11-05", true)
	f.insertIncome(t, a.ID, 90_000, "2026-11-20", false) // planned, not received yet
	f.insertIncome(t, a.ID, 70_000, "2026-10-31", true)  // other month
	for _, e := range []NewExpense{
		{AccountID: a.ID, AmountCents: 12_000, Date: day(t, "2026-11-01"), CategoryID: f.delivery},
		{AccountID: b.ID, AmountCents: 3_000, Date: day(t, "2026-11-30"), CategoryID: f.delivery},
		{AccountID: a.ID, AmountCents: 9_999, Date: day(t, "2026-12-01"), CategoryID: f.delivery},
	} {
		if _, err := f.s.AddExpense(ctx, f.userID, e); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.Transfer(ctx, f.userID, NewTransfer{FromAccountID: a.ID, ToAccountID: b.ID, AmountCents: 50_000, Date: day(t, "2026-11-10")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AdjustBalance(ctx, f.userID, Adjustment{AccountID: a.ID, BalanceCents: 1, Date: day(t, "2026-11-15")}); err != nil {
		t.Fatal(err)
	}

	got, err := f.s.MonthFlow(ctx, nov)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Flow{IncomeCents: 400_000, ExpenseCents: 15_000}); got != want {
		t.Fatalf("flow = %+v, want %+v", got, want)
	}
}

// TestStatement lists the month of an account with opening and closing balances (RF-03, RF-05).
func TestStatement(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	a := f.newAccount(t, "Nubank Ana", 100_000)
	b := f.newAccount(t, "Inter Bruno", 0)
	if _, err := f.s.AddExpense(ctx, f.userID, NewExpense{AccountID: a.ID, AmountCents: 10_000, Date: day(t, "2026-10-20"), CategoryID: f.delivery}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AddExpense(ctx, f.userID, NewExpense{AccountID: a.ID, AmountCents: 2_500, Date: day(t, "2026-11-03"), CategoryID: f.delivery, Description: "Lanche"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Transfer(ctx, f.userID, NewTransfer{FromAccountID: a.ID, ToAccountID: b.ID, AmountCents: 7_500, Date: day(t, "2026-11-08")}); err != nil {
		t.Fatal(err)
	}
	f.insertIncome(t, a.ID, 50_000, "2026-11-25", false)

	st, err := f.s.Statement(ctx, a.ID, civil.Month{Year: 2026, Month: time.November})
	if err != nil {
		t.Fatal(err)
	}
	if st.OpeningCents != 90_000 || st.ClosingCents != 80_000 {
		t.Fatalf("opening, closing = %d, %d; want 90000, 80000", st.OpeningCents, st.ClosingCents)
	}
	tests := []struct {
		kind        Kind
		date        string
		amount      int64
		confirmed   bool
		counterpart string
		category    string
	}{
		{kind: KindIncome, date: "2026-11-25", amount: 50_000},
		{kind: KindTransfer, date: "2026-11-08", amount: -7_500, confirmed: true, counterpart: "Inter Bruno"},
		{kind: KindExpense, date: "2026-11-03", amount: -2_500, confirmed: true, category: "Delivery"},
	}
	if len(st.Entries) != len(tests) {
		t.Fatalf("entries = %d, want %d", len(st.Entries), len(tests))
	}
	for i, tt := range tests {
		e := st.Entries[i]
		if e.Kind != tt.kind || e.Date.String() != tt.date || e.AmountCents != tt.amount || e.Confirmed != tt.confirmed ||
			e.CounterpartAccountName != tt.counterpart || e.CategoryName != tt.category || e.CreatedByName != "Ana Ficticia" {
			t.Errorf("entry %d = %+v", i, e)
		}
	}
	if _, err := f.s.Statement(ctx, 999, civil.Month{Year: 2026, Month: time.November}); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("missing account: err = %v", err)
	}
}

// TestUpdateAndDeleteAccount covers edits, deactivation and deletion of unused accounts.
func TestUpdateAndDeleteAccount(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	used := f.newAccount(t, "Nubank Ana", 0)
	unused := f.newAccount(t, "Inter Bruno", 0)
	if _, err := f.s.AddExpense(ctx, f.userID, NewExpense{AccountID: used.ID, AmountCents: 100, Date: day(t, "2026-11-11"), CategoryID: f.delivery}); err != nil {
		t.Fatal(err)
	}

	updates := []struct {
		name    string
		id      int64
		changes AccountChanges
		wantErr error
		check   func(a Account) bool
	}{
		{name: "rename", id: used.ID, changes: AccountChanges{Name: ptr("Nubank")}, check: func(a Account) bool { return a.Name == "Nubank" }},
		{name: "rename to other account name", id: used.ID, changes: AccountChanges{Name: ptr("inter bruno")}, wantErr: ErrDuplicateAccount},
		{name: "change holder", id: used.ID, changes: AccountChanges{PersonID: &f.persons[1]}, check: func(a Account) bool { return a.PersonID == f.persons[1] }},
		{name: "fix initial balance", id: used.ID, changes: AccountChanges{InitialBalanceCents: ptr(int64(10_000))}, check: func(a Account) bool { return a.BalanceCents == 9_900 }},
		{name: "deactivate keeps balance", id: used.ID, changes: AccountChanges{Active: ptr(false)}, check: func(a Account) bool { return !a.Active && a.BalanceCents == 9_900 }},
		{name: "unknown holder", id: used.ID, changes: AccountChanges{PersonID: ptr(int64(999))}, wantErr: ErrPersonNotFound},
		{name: "missing account", id: 999, changes: AccountChanges{Name: ptr("x")}, wantErr: ErrAccountNotFound},
	}
	for _, tt := range updates {
		t.Run(tt.name, func(t *testing.T) {
			a, err := f.s.UpdateAccount(ctx, tt.id, tt.changes)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && !tt.check(a) {
				t.Fatalf("account = %+v", a)
			}
		})
	}

	deletes := []struct {
		name    string
		id      int64
		wantErr error
	}{
		{name: "account with entries", id: used.ID, wantErr: ErrAccountInUse},
		{name: "unused account", id: unused.ID},
		{name: "missing account", id: 999, wantErr: ErrAccountNotFound},
	}
	for _, tt := range deletes {
		t.Run("delete "+tt.name, func(t *testing.T) {
			if err := f.s.DeleteAccount(ctx, tt.id); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestDeleteEntry removes one off entries and refuses unknown ones.
func TestDeleteEntry(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	a := f.newAccount(t, "Nubank Ana", 1_000)
	e, err := f.s.AddExpense(ctx, f.userID, NewExpense{AccountID: a.ID, AmountCents: 300, Date: day(t, "2026-11-11"), CategoryID: f.delivery})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.DeleteEntry(ctx, e.ID); err != nil || f.balance(t, a.ID) != 1_000 {
		t.Fatalf("delete = %v, balance %d", err, f.balance(t, a.ID))
	}
	if err := f.s.DeleteEntry(ctx, e.ID); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("second delete: err = %v", err)
	}
}

// TestAddIncome records one off incomes that raise the balance and count as income (RF-13, RN-08).
func TestAddIncome(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	a := f.newAccount(t, "Nubank Ana", 1_000)
	tests := []struct {
		name    string
		in      NewIncome
		wantErr error
	}{
		{name: "freelance", in: NewIncome{AccountID: a.ID, AmountCents: 35_000, Date: day(t, "2026-11-11"), Description: "Freela de design"}},
		{name: "zero amount", in: NewIncome{AccountID: a.ID, Date: day(t, "2026-11-11")}, wantErr: ErrAmountRequired},
		{name: "missing date", in: NewIncome{AccountID: a.ID, AmountCents: 100}, wantErr: ErrDateRequired},
		{name: "missing account", in: NewIncome{AccountID: 999, AmountCents: 100, Date: day(t, "2026-11-11")}, wantErr: ErrAccountNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := f.s.AddIncome(ctx, f.userID, tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (e.Kind != KindIncome || e.AmountCents != tt.in.AmountCents || !e.Confirmed) {
				t.Fatalf("entry = %+v", e)
			}
		})
	}
	if got := f.balance(t, a.ID); got != 36_000 {
		t.Fatalf("balance = %d, want 36000", got)
	}
	if flow, _ := f.s.MonthFlow(ctx, civil.Month{Year: 2026, Month: time.November}); flow.IncomeCents != 35_000 {
		t.Fatalf("flow = %+v", flow)
	}
}

// TestGroupIsolation shows that a second group sees nothing of the first one (RF-82, RN-27).
func TestGroupIsolation(t *testing.T) {
	f := newFixture(t)
	a := f.newAccount(t, "Nubank Ana", 1_000)
	e, err := f.s.AddExpense(groupCtx, f.userID, NewExpense{AccountID: a.ID, AmountCents: 300, Date: day(t, "2026-11-11"), CategoryID: f.delivery})
	if err != nil {
		t.Fatal(err)
	}
	carla := testutil.NewUser(t, f.conn, "carla", "Carla Ficticia")
	other := testutil.NewGroup(t, f.conn, "Outra", carla)
	name := "Renomeada"
	nov := civil.Month{Year: 2026, Month: time.November}

	tests := []struct {
		name    string
		run     func() error
		wantErr error
	}{
		{name: "get account", run: func() error { _, err := f.s.GetAccount(other, a.ID); return err }, wantErr: ErrAccountNotFound},
		{name: "update account", run: func() error {
			_, err := f.s.UpdateAccount(other, a.ID, AccountChanges{Name: &name})
			return err
		}, wantErr: ErrAccountNotFound},
		{name: "delete account", run: func() error { return f.s.DeleteAccount(other, a.ID) }, wantErr: ErrAccountNotFound},
		{name: "statement", run: func() error { _, err := f.s.Statement(other, a.ID, nov); return err }, wantErr: ErrAccountNotFound},
		{name: "get entry", run: func() error { _, err := f.s.Entry(other, e.ID); return err }, wantErr: ErrEntryNotFound},
		{name: "delete entry", run: func() error { return f.s.DeleteEntry(other, e.ID) }, wantErr: ErrEntryNotFound},
		{name: "expense on account of another group", run: func() error {
			_, err := f.s.AddExpense(other, carla.ID, NewExpense{AccountID: a.ID, AmountCents: 100, Date: day(t, "2026-11-11"), CategoryID: f.delivery})
			return err
		}, wantErr: ErrAccountNotFound},
		{name: "holder from another group", run: func() error {
			_, err := f.s.CreateAccount(other, carla.ID, NewAccount{Name: "Intrusa", PersonID: f.persons[0]})
			return err
		}, wantErr: ErrPersonNotFound},
		{name: "no active group", run: func() error { _, err := f.s.ListAccounts(context.Background()); return err }, wantErr: scope.ErrNoGroup},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}

	if list, err := f.s.ListAccounts(other); err != nil || len(list) != 0 {
		t.Fatalf("accounts of the other group = %v, %v", list, err)
	}
	if flow, err := f.s.MonthFlow(other, nov); err != nil || flow != (Flow{}) {
		t.Fatalf("flow of the other group = %+v, %v", flow, err)
	}
	if got := f.balance(t, a.ID); got != 700 {
		t.Fatalf("balance of the first group = %d, want 700", got)
	}
}
