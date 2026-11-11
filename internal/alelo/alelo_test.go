package alelo

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/ledger"
)

// fixture holds a migrated database where today is 2026-11-11, with two users and the service.
type fixture struct {
	s       *Service
	conn    *sql.DB
	userID  int64
	persons [2]int64
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
	if _, err := users.Create(ctx, "bruno", "Bruno Ficticio"); err != nil {
		t.Fatal(err)
	}
	persons, _ := users.ListPersons(ctx)
	s := NewService(conn)
	s.today = func() civil.Date { return civil.Date{Year: 2026, Month: time.November, Day: 11} }
	return fixture{s: s, conn: conn, userID: ana.ID, persons: [2]int64{persons[0].ID, persons[1].ID}}
}

// month builds a civil month.
func month(y int, m time.Month) civil.Month { return civil.Month{Year: y, Month: m} }

// date builds a civil date.
func date(y int, m time.Month, d int) civil.Date { return civil.Date{Year: y, Month: m, Day: d} }

// card returns a valid input: R$ 800,00 every 5th, R$ 120,00 left today.
func (f fixture) card() Input {
	return Input{Name: "Alelo Ana", PersonID: f.persons[0], MonthlyCreditCents: 80_000, CreditDay: 5, InitialBalanceCents: 12_000}
}

// TestCreate validates the Alelo registration with its starting balance (RF-31).
func TestCreate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tests := []struct {
		name    string
		mod     func(*Input)
		wantErr error
	}{
		{name: "valid", mod: func(*Input) {}},
		{name: "zero starting balance", mod: func(in *Input) { in.Name, in.InitialBalanceCents = "Alelo Bruno", 0 }},
		{name: "empty name", mod: func(in *Input) { in.Name = "" }, wantErr: ErrNameRequired},
		{name: "duplicate name", mod: func(in *Input) { in.Name = "alelo ana" }, wantErr: ErrDuplicateName},
		{name: "zero credit", mod: func(in *Input) { in.Name, in.MonthlyCreditCents = "X", 0 }, wantErr: ledger.ErrAmountRequired},
		{name: "negative starting balance", mod: func(in *Input) { in.Name, in.InitialBalanceCents = "X", -1 }, wantErr: ErrInvalidBalance},
		{name: "credit day 0", mod: func(in *Input) { in.Name, in.CreditDay = "X", 0 }, wantErr: ErrInvalidDay},
		{name: "unknown holder", mod: func(in *Input) { in.Name, in.PersonID = "X", 999 }, wantErr: ErrPersonNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := f.card()
			tt.mod(&in)
			c, err := f.s.Create(ctx, f.userID, in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (c.BalanceCents != in.InitialBalanceCents || c.StartMonth.String() != "2026-11" || c.PersonName == "") {
				t.Fatalf("card = %+v", c)
			}
		})
	}
}

// TestBalance carries the unused balance over the months (RF-32 to RF-34, RN-15).
func TestBalance(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, _ := f.s.Create(ctx, f.userID, f.card())
	nov, dec := month(2026, time.November), month(2026, time.December)

	steps := []struct {
		name                     string
		do                       func(t *testing.T)
		month                    civil.Month
		opening, credit, closing int64
		spent                    int64
		confirmed                bool
	}{
		{name: "planned credit does not count", do: func(*testing.T) {}, month: nov, opening: 12_000, closing: 12_000},
		{name: "confirmed credit enters", do: func(t *testing.T) {
			if _, err := f.s.ConfirmCredit(ctx, f.userID, Credit{AleloID: c.ID, Month: nov}); err != nil {
				t.Fatal(err)
			}
		}, month: nov, opening: 12_000, credit: 80_000, closing: 92_000, confirmed: true},
		{name: "purchases leave", do: func(t *testing.T) {
			for _, p := range []Purchase{{AleloID: c.ID, Date: date(2026, time.November, 12), Description: "Mercado", AmountCents: 45_390}, {AleloID: c.ID, Date: date(2026, time.November, 20), Description: "Padaria", AmountCents: 2_610}} {
				if _, err := f.s.AddPurchase(ctx, f.userID, p); err != nil {
					t.Fatal(err)
				}
			}
		}, month: nov, opening: 12_000, credit: 80_000, spent: 48_000, closing: 44_000, confirmed: true},
		{name: "next month starts with what was left", do: func(t *testing.T) {
			if _, err := f.s.ConfirmCredit(ctx, f.userID, Credit{AleloID: c.ID, Month: dec, AmountCents: 81_000, Date: date(2026, time.December, 4)}); err != nil {
				t.Fatal(err)
			}
		}, month: dec, opening: 44_000, credit: 81_000, closing: 125_000, confirmed: true},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			st.do(t)
			v, err := f.s.Month(ctx, c.ID, st.month)
			if err != nil {
				t.Fatal(err)
			}
			if v.OpeningCents != st.opening || v.Credit.AmountCents != st.credit || v.SpentCents != st.spent || v.ClosingCents != st.closing || v.Credit.Confirmed != st.confirmed {
				t.Fatalf("month = %+v", v)
			}
			if v.Credit.ExpectedCents != 80_000 || v.Credit.ExpectedOn.Day != 5 {
				t.Fatalf("expected credit = %+v", v.Credit)
			}
		})
	}
	got, _ := f.s.Get(ctx, c.ID)
	if got.BalanceCents != 125_000 {
		t.Fatalf("current balance = %d", got.BalanceCents)
	}
}

// TestPurchaseIsGroceries keeps the free description and files the purchase as Alimentação, Compras para casa (RF-33, RN-15).
func TestPurchaseIsGroceries(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, _ := f.s.Create(ctx, f.userID, f.card())
	tests := []struct {
		name    string
		in      Purchase
		wantErr error
	}{
		{name: "free description", in: Purchase{AleloID: c.ID, Date: date(2026, time.November, 11), Description: " Sacolão do bairro ", AmountCents: 3_450}},
		{name: "empty description", in: Purchase{AleloID: c.ID, Date: date(2026, time.November, 11), AmountCents: 1}, wantErr: ErrDescriptionRequired},
		{name: "long description", in: Purchase{AleloID: c.ID, Date: date(2026, time.November, 11), Description: strings.Repeat("a", 121), AmountCents: 1}, wantErr: ledger.ErrDescriptionTooLong},
		{name: "zero amount", in: Purchase{AleloID: c.ID, Date: date(2026, time.November, 11), Description: "x"}, wantErr: ledger.ErrAmountRequired},
		{name: "missing date", in: Purchase{AleloID: c.ID, Description: "x", AmountCents: 1}, wantErr: ledger.ErrDateRequired},
		{name: "missing card", in: Purchase{AleloID: 999, Date: date(2026, time.November, 11), Description: "x", AmountCents: 1}, wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := f.s.AddPurchase(ctx, f.userID, tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			var category, parent string
			if err := f.conn.QueryRow(`SELECT c.name, p.name FROM alelo_purchases a JOIN categories c ON c.id = a.category_id JOIN categories p ON p.id = c.parent_id WHERE a.id = ?`, p.ID).Scan(&category, &parent); err != nil {
				t.Fatal(err)
			}
			if p.Description != "Sacolão do bairro" || category != "Compras para casa" || parent != "Alimentação" {
				t.Fatalf("purchase = %+v in %s > %s", p, parent, category)
			}
		})
	}

	// A rename does not break the link, which uses the system key.
	if _, err := f.conn.Exec(`UPDATE categories SET name = 'Supermercado' WHERE system_key = 'groceries'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AddPurchase(ctx, f.userID, Purchase{AleloID: c.ID, Date: date(2026, time.November, 11), Description: "Feira", AmountCents: 100}); err != nil {
		t.Fatalf("after rename: %v", err)
	}
}

// TestCredit confirms, refuses duplicates and undoes the monthly credit (RF-32).
func TestCredit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, _ := f.s.Create(ctx, f.userID, f.card())
	nov := month(2026, time.November)
	tests := []struct {
		name    string
		in      Credit
		wantErr error
	}{
		{name: "before the first month", in: Credit{AleloID: c.ID, Month: month(2026, time.October)}, wantErr: ErrNoCredit},
		{name: "confirm", in: Credit{AleloID: c.ID, Month: nov}},
		{name: "twice", in: Credit{AleloID: c.ID, Month: nov}, wantErr: ErrAlreadyConfirmed},
		{name: "missing card", in: Credit{AleloID: 999, Month: nov}, wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.s.ConfirmCredit(ctx, f.userID, tt.in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
	if err := f.s.UndoCredit(ctx, c.ID, nov); err != nil {
		t.Fatal(err)
	}
	if err := f.s.UndoCredit(ctx, c.ID, nov); !errors.Is(err, ErrCreditNotFound) {
		t.Fatalf("second undo: %v", err)
	}
	if v, _ := f.s.Month(ctx, c.ID, nov); v.Credit.Confirmed || v.ClosingCents != 12_000 {
		t.Fatalf("after undo = %+v", v)
	}
}

// TestDelete keeps cards with history and removes purchases.
func TestDelete(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	used, _ := f.s.Create(ctx, f.userID, f.card())
	in := f.card()
	in.Name = "Alelo Bruno"
	unused, _ := f.s.Create(ctx, f.userID, in)
	p, _ := f.s.AddPurchase(ctx, f.userID, Purchase{AleloID: used.ID, Date: date(2026, time.November, 11), Description: "Mercado", AmountCents: 1_000})
	if err := f.s.Delete(ctx, used.ID); !errors.Is(err, ErrInUse) {
		t.Fatalf("delete with history: %v", err)
	}
	if err := f.s.Delete(ctx, unused.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DeletePurchase(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.s.Get(ctx, used.ID); got.BalanceCents != 12_000 {
		t.Fatalf("balance after deleting the purchase = %d", got.BalanceCents)
	}
}
