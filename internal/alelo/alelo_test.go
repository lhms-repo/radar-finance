package alelo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/scope"
	"github.com/italogfx/radar-finance/internal/testutil"
)

// fixture holds a migrated database where today is 2026-11-11, with two users and the service.
type fixture struct {
	s       *Service
	conn    *sql.DB
	userID  int64
	persons [2]int64
}

// groupCtx acts on the first group of a fresh database, created by every fixture.
var groupCtx = scope.WithGroup(context.Background(), 1)

// anaPersonal is the personal space of Ana, the holder of the test card, created right after Casa.
var anaPersonal = scope.WithGroup(context.Background(), 2)

// newFixture opens a fresh database for one test.
func newFixture(t *testing.T) fixture {
	t.Helper()
	conn := testutil.OpenDB(t)
	ana := testutil.NewUser(t, conn, "ana", "Ana Ficticia")
	bruno := testutil.NewUser(t, conn, "bruno", "Bruno Ficticio")
	testutil.FirstGroup(t, conn, ana, bruno)
	persons := []account.Person{{ID: ana.PersonID}, {ID: bruno.PersonID}}
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
	return Input{Name: "Alelo Ana", PersonID: f.persons[0], MonthlyCreditCents: 80_000, CreditDay: 5, InitialBalanceCents: 12_000, Style: "tudo"}
}

// TestCreate validates the Alelo registration with its starting balance (RF-31).
func TestCreate(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	tests := []struct {
		name    string
		mod     func(*Input)
		wantErr error
	}{
		{name: "valid", mod: func(*Input) {}},
		{name: "zero starting balance", mod: func(in *Input) { in.Name, in.InitialBalanceCents = "Alelo Bruno", 0 }},
		{name: "classic look", mod: func(in *Input) { in.Name, in.Style = "Alelo Classico", "classic" }},
		{name: "unknown look", mod: func(in *Input) { in.Name, in.Style = "X", "pink" }, wantErr: ErrInvalidStyle},
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
			if err == nil && (c.BalanceCents != in.InitialBalanceCents || c.StartMonth.String() != "2026-11" || c.PersonName == "" || c.Style != in.Style) {
				t.Fatalf("card = %+v", c)
			}
		})
	}
}

// TestBalance carries the unused balance over the months, read by the holder in her personal space with the
// purchases of every space (RF-32 to RF-34, RN-15, RF-83).
func TestBalance(t *testing.T) {
	f := newFixture(t)
	ctx := anaPersonal
	c, _ := f.s.Create(groupCtx, f.userID, f.card())
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
				if _, err := f.s.AddPurchase(groupCtx, f.userID, p); err != nil {
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
	ctx := groupCtx
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

// TestCredit confirms, refuses duplicates and undoes the monthly credit, only in the personal space of the holder (RF-32, RF-83).
func TestCredit(t *testing.T) {
	f := newFixture(t)
	ctx := anaPersonal
	c, _ := f.s.Create(groupCtx, f.userID, f.card())
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
	if _, err := f.s.ConfirmCredit(groupCtx, f.userID, Credit{AleloID: c.ID, Month: month(2026, time.December)}); !errors.Is(err, ErrCreditInPersonal) {
		t.Fatalf("confirm in the group = %v, want ErrCreditInPersonal", err)
	}
	if err := f.s.UndoCredit(groupCtx, c.ID, nov); !errors.Is(err, ErrCreditInPersonal) {
		t.Fatalf("undo in the group = %v, want ErrCreditInPersonal", err)
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
	ctx := groupCtx
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

// TestGroupIsolation shows that a second group sees none of the Alelo cards and purchases of the first (RF-82, RN-27).
func TestGroupIsolation(t *testing.T) {
	f := newFixture(t)
	c, err := f.s.Create(groupCtx, f.userID, f.card())
	if err != nil {
		t.Fatal(err)
	}
	nov := month(2026, time.November)
	p, err := f.s.AddPurchase(groupCtx, f.userID, Purchase{AleloID: c.ID, Date: date(2026, time.November, 11), Description: "Feira", AmountCents: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	carla := testutil.NewUser(t, f.conn, "carla", "Carla Ficticia")
	other := testutil.NewGroup(t, f.conn, "Outra", carla)

	tests := []struct {
		name    string
		run     func() error
		wantErr error
	}{
		{name: "get", run: func() error { _, err := f.s.Get(other, c.ID); return err }, wantErr: ErrNotFound},
		{name: "update", run: func() error {
			_, err := f.s.Update(other, c.ID, Changes{Name: "X", PersonID: carla.PersonID, MonthlyCreditCents: 100, CreditDay: 5, Style: "tudo", Active: true})
			return err
		}, wantErr: ErrNotFound},
		{name: "delete", run: func() error { return f.s.Delete(other, c.ID) }, wantErr: ErrNotFound},
		{name: "month", run: func() error { _, err := f.s.Month(other, c.ID, nov); return err }, wantErr: ErrNotFound},
		{name: "confirm credit", run: func() error {
			_, err := f.s.ConfirmCredit(other, carla.ID, Credit{AleloID: c.ID, Month: nov})
			return err
		}, wantErr: ErrNotFound},
		{name: "purchase on card of another group", run: func() error {
			_, err := f.s.AddPurchase(other, carla.ID, Purchase{AleloID: c.ID, Date: date(2026, time.November, 11), Description: "X", AmountCents: 100})
			return err
		}, wantErr: ErrNotFound},
		{name: "delete purchase", run: func() error { return f.s.DeletePurchase(other, p.ID) }, wantErr: ErrPurchaseNotFound},
		{name: "holder from another group", run: func() error { _, err := f.s.Create(other, carla.ID, f.card()); return err }, wantErr: ErrPersonNotFound},
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
		t.Fatalf("cards of the other group = %v, %v", list, err)
	}
	// The purchase the other group tried to delete is still there.
	if v, err := f.s.Month(groupCtx, c.ID, nov); err != nil || v.SpentCents != 1_000 {
		t.Fatalf("month of the first group = %+v, %v", v, err)
	}
}

// TestAleloHolderStaysWhileUsedElsewhere keeps the holder of an Alelo card with purchases in another group (RF-82).
func TestAleloHolderStaysWhileUsedElsewhere(t *testing.T) {
	f := newFixture(t)
	c, err := f.s.Create(groupCtx, f.userID, f.card())
	if err != nil {
		t.Fatal(err)
	}
	ana, err := account.NewService(f.conn).Get(groupCtx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	beach := testutil.NewGroup(t, f.conn, "Praia", ana)
	if _, err := f.s.AddPurchase(beach, f.userID, Purchase{AleloID: c.ID, Date: date(2026, time.November, 11), Description: "Feira", AmountCents: 1_000}); err != nil {
		t.Fatal(err)
	}
	ch := Changes{Name: "Alelo Ana", PersonID: f.persons[1], MonthlyCreditCents: 80_000, CreditDay: 5, Style: "tudo", Active: true}
	if _, err := f.s.Update(groupCtx, c.ID, ch); !errors.Is(err, ErrHolderUsedElsewhere) {
		t.Fatalf("holder change with purchases in the beach group: %v, want ErrHolderUsedElsewhere", err)
	}
	ch.PersonID = f.persons[0]
	ch.Name = "Alelo nova"
	if _, err := f.s.Update(groupCtx, c.ID, ch); err != nil {
		t.Fatalf("other changes still work: %v", err)
	}
}

// TestHolderView shows the Alelo like an account: a group reads only its purchases, without balances or credits,
// and the holder reads in her personal space every purchase with its space and the real balance (RF-83).
func TestHolderView(t *testing.T) {
	f := newFixture(t)
	c, err := f.s.Create(groupCtx, f.userID, f.card())
	if err != nil {
		t.Fatal(err)
	}
	nov := month(2026, time.November)
	if _, err := f.s.ConfirmCredit(anaPersonal, f.userID, Credit{AleloID: c.ID, Month: nov}); err != nil {
		t.Fatal(err)
	}
	ana, err := account.NewService(f.conn).Get(groupCtx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	// A second shared group of Ana, where she also uses the card.
	beach := testutil.NewGroup(t, f.conn, "Praia", ana)
	for _, p := range []struct {
		ctx  context.Context
		desc string
		cost int64
	}{{groupCtx, "Feira de Casa", 3_000}, {anaPersonal, "Lanche pessoal", 1_000}, {beach, "Peixe da praia", 500}} {
		if _, err := f.s.AddPurchase(p.ctx, f.userID, Purchase{AleloID: c.ID, Date: date(2026, time.November, 11), Description: p.desc, AmountCents: p.cost}); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name                string
		ctx                 context.Context
		full                bool
		purchases           []string
		spent, opening, end int64
		credit              int64
	}{
		{name: "group reads its purchases only", ctx: groupCtx, purchases: []string{"Feira de Casa"}, spent: 3_000},
		{name: "another group reads its purchases only", ctx: beach, purchases: []string{"Peixe da praia"}, spent: 500},
		{name: "holder reads every space in her personal space", ctx: anaPersonal, full: true, purchases: []string{"Peixe da praia", "Lanche pessoal", "Feira de Casa"}, spent: 4_500, opening: 12_000, credit: 80_000, end: 87_500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := f.s.Month(tt.ctx, c.ID, nov)
			if err != nil {
				t.Fatal(err)
			}
			if v.Full != tt.full || v.SpentCents != tt.spent || v.OpeningCents != tt.opening || v.ClosingCents != tt.end || v.Credit.AmountCents != tt.credit || v.HasCredit != tt.full {
				t.Fatalf("month = %+v", v)
			}
			if len(v.Purchases) != len(tt.purchases) {
				t.Fatalf("purchases = %+v, want %v", v.Purchases, tt.purchases)
			}
			for i, want := range tt.purchases {
				if v.Purchases[i].Description != want {
					t.Fatalf("purchase %d = %q, want %q", i, v.Purchases[i].Description, want)
				}
			}
			if tt.full && (v.Purchases[0].GroupName != "Praia" || v.Purchases[1].GroupName != "Pessoal" || v.Purchases[2].GroupName != "Casa") {
				t.Fatalf("spaces of the purchases = %+v", v.Purchases)
			}
		})
	}

	// The real balance counts every space and is owned only by the holder.
	asAna := auth.WithUserID(groupCtx, f.userID)
	bruno, err := account.NewService(f.conn).GetByLogin(context.Background(), "bruno")
	if err != nil {
		t.Fatal(err)
	}
	asBruno := auth.WithUserID(groupCtx, bruno.ID)
	for _, tt := range []struct {
		name  string
		ctx   context.Context
		owned bool
	}{{"holder", asAna, true}, {"other member", asBruno, false}} {
		got, err := f.s.Get(tt.ctx, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Owned != tt.owned || got.BalanceCents != 87_500 {
			t.Fatalf("%s reads %+v; want owned %v and balance 87500", tt.name, got, tt.owned)
		}
	}
}
