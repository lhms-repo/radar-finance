package card

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
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/scope"
	"github.com/italogfx/radar-finance/internal/testutil"
)

// fixture holds a migrated database where today is 2026-11-11, with users, an account and the services.
type fixture struct {
	s         *Service
	ledger    *ledger.Service
	conn      *sql.DB
	userID    int64
	persons   [2]int64
	accountID int64
	delivery  int64
	streaming int64
	today     *civil.Date
}

// groupCtx acts on the first group of a fresh database, created by every fixture.
var groupCtx = scope.WithGroup(context.Background(), 1)

// newFixture opens a fresh database for one test, with the group "Casa" of two fictional users.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := groupCtx
	conn := testutil.OpenDB(t)
	ana := testutil.NewUser(t, conn, "ana", "Ana Ficticia")
	bruno := testutil.NewUser(t, conn, "bruno", "Bruno Ficticio")
	testutil.FirstGroup(t, conn, ana, bruno)
	persons := []account.Person{{ID: ana.PersonID}, {ID: bruno.PersonID}}
	l := ledger.NewService(conn)
	acc, err := l.CreateAccount(ctx, ana.ID, ledger.NewAccount{Name: "Nubank Ana", PersonID: persons[0].ID, InitialBalanceCents: 1_000_000})
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{ledger: l, conn: conn, userID: ana.ID, persons: [2]int64{persons[0].ID, persons[1].ID}, accountID: acc.ID}
	today := d(2026, time.November, 11)
	f.today = &today
	f.s = NewService(conn)
	f.s.today = func() civil.Date { return *f.today }
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Delivery'`).Scan(&f.delivery)
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Streaming'`).Scan(&f.streaming)
	return f
}

// nubank returns a valid card input: closes on the 3rd, due on the 10th, limit of R$ 5.000,00.
func (f *fixture) nubank() CardInput {
	return CardInput{Name: "Nubank", PersonID: f.persons[0], LimitCents: 500_000, ClosingDay: 3, DueDay: 10, LastDigits: "1234", Color: "purple", Finish: "matte"}
}

// newCard creates the default card.
func (f *fixture) newCard(t *testing.T) Card {
	t.Helper()
	c, err := f.s.CreateCard(groupCtx, f.userID, f.nubank())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// buy registers a purchase of total cents in n installments on the date.
func (f *fixture) buy(t *testing.T, cardID int64, on civil.Date, total int64, n int) Purchase {
	t.Helper()
	p, err := f.s.AddPurchase(groupCtx, f.userID, PurchaseInput{CardID: cardID, Date: on, Merchant: "Loja Ficticia", CategoryID: f.delivery, AmountCents: total, Installments: n})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// invoice reads the invoice of the month.
func (f *fixture) invoice(t *testing.T, cardID int64, y int, m time.Month) Invoice {
	t.Helper()
	inv, err := f.s.Invoice(groupCtx, cardID, civil.Month{Year: y, Month: m})
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// TestCreateCard validates the card registration (RF-14).
func TestCreateCard(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	tests := []struct {
		name    string
		mod     func(*CardInput)
		wantErr error
	}{
		{name: "valid", mod: func(*CardInput) {}},
		{name: "without last digits", mod: func(c *CardInput) { c.Name, c.LastDigits = "Inter", "" }},
		{name: "empty name", mod: func(c *CardInput) { c.Name = " " }, wantErr: ErrNameRequired},
		{name: "duplicate name", mod: func(c *CardInput) { c.Name = "NUBANK" }, wantErr: ErrDuplicateName},
		{name: "negative limit", mod: func(c *CardInput) { c.Name, c.LimitCents = "C1", -1 }, wantErr: ErrInvalidLimit},
		{name: "closing day 0", mod: func(c *CardInput) { c.Name, c.ClosingDay = "C2", 0 }, wantErr: ErrInvalidDay},
		{name: "due day 32", mod: func(c *CardInput) { c.Name, c.DueDay = "C3", 32 }, wantErr: ErrInvalidDay},
		{name: "last digits with a letter", mod: func(c *CardInput) { c.Name, c.LastDigits = "C4", "12a4" }, wantErr: ErrInvalidDigits},
		{name: "unknown color", mod: func(c *CardInput) { c.Name, c.Color = "C5", "pink-neon" }, wantErr: ErrInvalidLook},
		{name: "unknown finish", mod: func(c *CardInput) { c.Name, c.Finish = "C6", "glitter" }, wantErr: ErrInvalidLook},
		{name: "unknown holder", mod: func(c *CardInput) { c.Name, c.PersonID = "C7", 999 }, wantErr: ErrPersonNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := f.nubank()
			tt.mod(&in)
			c, err := f.s.CreateCard(ctx, f.userID, in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (c.LimitFreeCents != in.LimitCents || c.PersonName == "" || c.CurrentInvoice.String() != "2026-12" || c.CurrentInvoiceDueOn.String() != "2026-12-10") {
				t.Fatalf("card = %+v", c)
			}
		})
	}
}

// TestAddPurchase spreads installments over consecutive invoices (RF-15, RF-16, RF-18, RN-02, RN-03).
func TestAddPurchase(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	c := f.newCard(t)
	closed, _ := f.s.CreateCard(ctx, f.userID, CardInput{Name: "Antigo", PersonID: f.persons[0], LimitCents: 1, ClosingDay: 1, DueDay: 8, Color: "graphite", Finish: "metal"})
	if _, err := f.s.UpdateCard(ctx, closed.ID, CardInput{Name: "Antigo", PersonID: f.persons[0], LimitCents: 1, ClosingDay: 1, DueDay: 8, Color: "graphite", Finish: "metal"}, false); err != nil {
		t.Fatal(err)
	}
	var topLevel int64
	_ = f.conn.QueryRow(`SELECT id FROM categories WHERE name = 'Alimentação'`).Scan(&topLevel)

	type inst struct {
		month  string
		amount int64
	}
	tests := []struct {
		name    string
		in      PurchaseInput
		want    []inst
		wantErr error
	}{
		{
			name: "three installments from the total", in: PurchaseInput{Date: d(2026, time.November, 20), AmountCents: 100_000, Installments: 3},
			want: []inst{{"2026-12", 33_334}, {"2027-01", 33_333}, {"2027-02", 33_333}},
		},
		{
			name: "installment amount times four", in: PurchaseInput{Date: d(2026, time.November, 2), AmountCents: 15_000, Installments: 4, PerInstallment: true},
			want: []inst{{"2026-11", 15_000}, {"2026-12", 15_000}, {"2027-01", 15_000}, {"2027-02", 15_000}},
		},
		{name: "single purchase on the closing day", in: PurchaseInput{Date: d(2026, time.November, 3), AmountCents: 4_590, Installments: 1}, want: []inst{{"2026-12", 4_590}}},
		{name: "zero amount", in: PurchaseInput{Date: d(2026, time.November, 3), Installments: 1}, wantErr: ledger.ErrAmountRequired},
		{name: "zero installments", in: PurchaseInput{Date: d(2026, time.November, 3), AmountCents: 100}, wantErr: ErrInvalidInstallments},
		{name: "49 installments", in: PurchaseInput{Date: d(2026, time.November, 3), AmountCents: 10_000, Installments: 49}, wantErr: ErrInvalidInstallments},
		{name: "fewer cents than installments", in: PurchaseInput{Date: d(2026, time.November, 3), AmountCents: 2, Installments: 3}, wantErr: ErrInvalidInstallments},
		{name: "missing merchant", in: PurchaseInput{Date: d(2026, time.November, 3), AmountCents: 100, Installments: 1}, wantErr: ErrMerchantRequired},
		{name: "top level category", in: PurchaseInput{Date: d(2026, time.November, 3), AmountCents: 100, Installments: 1, CategoryID: topLevel}, wantErr: category.ErrNotSelectable},
		{name: "unknown buyer", in: PurchaseInput{Date: d(2026, time.November, 3), AmountCents: 100, Installments: 1, BuyerPersonID: ptr(int64(999))}, wantErr: ErrPersonNotFound},
		{name: "inactive card", in: PurchaseInput{CardID: closed.ID, Date: d(2026, time.November, 3), AmountCents: 100, Installments: 1}, wantErr: ErrCardInactive},
		{name: "missing date", in: PurchaseInput{AmountCents: 100, Installments: 1}, wantErr: ledger.ErrDateRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			if in.CardID == 0 {
				in.CardID = c.ID
			}
			if in.Merchant == "" && !errors.Is(tt.wantErr, ErrMerchantRequired) {
				in.Merchant = "Loja Ficticia"
			}
			if in.CategoryID == 0 {
				in.CategoryID = f.delivery
			}
			p, err := f.s.AddPurchase(ctx, f.userID, in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			rows, _ := f.conn.Query(`SELECT invoice_month, amount_cents FROM installments WHERE purchase_id = ? ORDER BY number`, p.ID)
			defer rows.Close()
			var got []inst
			for rows.Next() {
				var i inst
				_ = rows.Scan(&i.month, &i.amount)
				got = append(got, i)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("installments = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("installments = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// TestInvoiceView shows items, totals, dates and status (RF-19, RN-01).
func TestInvoiceView(t *testing.T) {
	f := newFixture(t)
	c := f.newCard(t)
	buyer := f.persons[1]
	if _, err := f.s.AddPurchase(groupCtx, f.userID, PurchaseInput{CardID: c.ID, Date: d(2026, time.October, 25), Merchant: "Mercado Ficticio", CategoryID: f.delivery, AmountCents: 30_000, Installments: 3, BuyerPersonID: &buyer}); err != nil {
		t.Fatal(err)
	}
	f.buy(t, c.ID, d(2026, time.November, 1), 5_000, 1)

	inv := f.invoice(t, c.ID, 2026, time.November)
	if inv.ClosingOn.String() != "2026-11-03" || inv.DueOn.String() != "2026-11-10" {
		t.Fatalf("dates = %v, %v", inv.ClosingOn, inv.DueOn)
	}
	if inv.TotalCents != 15_000 || inv.NetCents != 15_000 || inv.ThirdPartyCents != 0 || len(inv.Items) != 2 {
		t.Fatalf("invoice = %+v", inv)
	}
	first := inv.Items[1]
	if first.Merchant != "Mercado Ficticio" || first.Number != 1 || first.Count != 3 || first.AmountCents != 10_000 || first.BuyerName != "Bruno Ficticio" || first.CategoryName != "Delivery" {
		t.Fatalf("item = %+v", first)
	}

	tests := []struct {
		name  string
		today civil.Date
		month time.Month
		want  Status
	}{
		{name: "before closing", today: d(2026, time.November, 2), month: time.November, want: StatusOpen},
		{name: "closing day", today: d(2026, time.November, 3), month: time.November, want: StatusClosed},
		{name: "due day", today: d(2026, time.November, 10), month: time.November, want: StatusClosed},
		{name: "after due date", today: d(2026, time.November, 11), month: time.November, want: StatusOverdue},
		{name: "next invoice still open", today: d(2026, time.November, 11), month: time.December, want: StatusOpen},
		{name: "empty invoice past due is not overdue", today: d(2026, time.November, 11), month: time.October, want: StatusClosed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*f.today = tt.today
			if got := f.invoice(t, c.ID, 2026, tt.month).Status; got != tt.want {
				t.Fatalf("status = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestCredits lowers the invoice with refunds and other credits (RF-22).
func TestCredits(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	c := f.newCard(t)
	f.buy(t, c.ID, d(2026, time.November, 5), 20_000, 1)
	tests := []struct {
		name    string
		in      CreditInput
		wantErr error
	}{
		{name: "refund with category", in: CreditInput{CardID: c.ID, Date: d(2026, time.November, 7), Merchant: "Estorno Loja Ficticia", AmountCents: 5_000, CategoryID: &f.delivery}},
		{name: "credit without category", in: CreditInput{CardID: c.ID, Date: d(2026, time.November, 8), Merchant: "Crédito de anuidade", AmountCents: 1_000}},
		{name: "zero amount", in: CreditInput{CardID: c.ID, Date: d(2026, time.November, 8), Merchant: "x"}, wantErr: ledger.ErrAmountRequired},
		{name: "missing merchant", in: CreditInput{CardID: c.ID, Date: d(2026, time.November, 8), AmountCents: 1}, wantErr: ErrMerchantRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.s.AddCredit(ctx, f.userID, tt.in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
	inv := f.invoice(t, c.ID, 2026, time.December)
	if inv.TotalCents != 14_000 || len(inv.Items) != 3 {
		t.Fatalf("invoice = %d with %d items, want 14000 with 3", inv.TotalCents, len(inv.Items))
	}
	for _, it := range inv.Items {
		if it.Kind == ItemCredit && it.AmountCents >= 0 {
			t.Fatalf("credit must be negative: %+v", it)
		}
	}
}

// TestRecurrence charges every invoice from the start and keeps closed invoices when the price changes (RF-17).
func TestRecurrence(t *testing.T) {
	f := newFixture(t) // today 2026-11-11, current invoice 2026-12
	ctx := groupCtx
	c := f.newCard(t)
	r, err := f.s.AddRecurrence(ctx, f.userID, RecurrenceInput{CardID: c.ID, Description: "Streaming de música", CategoryID: f.streaming, AmountCents: 2_190, StartDate: d(2026, time.October, 15)})
	if err != nil {
		t.Fatal(err)
	}
	if r.StartMonth.String() != "2026-11" || r.AmountCents != 2_190 {
		t.Fatalf("recurrence = %+v", r)
	}

	// Price goes up: applies from the open invoice (December) on.
	if _, err := f.s.UpdateRecurrence(ctx, r.ID, RecurrenceChanges{Description: "Streaming de música", CategoryID: f.streaming, AmountCents: 2_390}); err != nil {
		t.Fatal(err)
	}
	// Ends in February.
	if _, err := f.s.UpdateRecurrence(ctx, r.ID, RecurrenceChanges{Description: "Streaming de música", CategoryID: f.streaming, AmountCents: 2_390, EndMonth: &civil.Month{Year: 2027, Month: time.February}}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		month civil.Month
		want  int64
	}{
		{month: civil.Month{Year: 2026, Month: time.October}, want: 0},
		{month: civil.Month{Year: 2026, Month: time.November}, want: 2_190},
		{month: civil.Month{Year: 2026, Month: time.December}, want: 2_390},
		{month: civil.Month{Year: 2027, Month: time.February}, want: 2_390},
		{month: civil.Month{Year: 2027, Month: time.March}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.month.String(), func(t *testing.T) {
			inv, err := f.s.Invoice(ctx, c.ID, tt.month)
			if err != nil {
				t.Fatal(err)
			}
			if inv.TotalCents != tt.want {
				t.Fatalf("total = %d, want %d", inv.TotalCents, tt.want)
			}
			if tt.want > 0 && (len(inv.Items) != 1 || inv.Items[0].Kind != ItemRecurrence || inv.Items[0].CategoryName != "Streaming") {
				t.Fatalf("items = %+v", inv.Items)
			}
		})
	}

	errs := []struct {
		name    string
		in      RecurrenceInput
		wantErr error
	}{
		{name: "missing description", in: RecurrenceInput{CardID: c.ID, CategoryID: f.streaming, AmountCents: 1, StartDate: d(2026, time.November, 1)}, wantErr: ErrDescriptionRequired},
		{name: "zero amount", in: RecurrenceInput{CardID: c.ID, Description: "x", CategoryID: f.streaming, StartDate: d(2026, time.November, 1)}, wantErr: ledger.ErrAmountRequired},
		{name: "end before start", in: RecurrenceInput{CardID: c.ID, Description: "x", CategoryID: f.streaming, AmountCents: 1, StartDate: d(2026, time.November, 20), EndMonth: &civil.Month{Year: 2026, Month: time.November}}, wantErr: ErrEndBeforeStart},
	}
	for _, tt := range errs {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.s.AddRecurrence(ctx, f.userID, tt.in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestDeleteRecurrence refuses to rewrite paid invoices.
func TestDeleteRecurrence(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	c := f.newCard(t)
	r, _ := f.s.AddRecurrence(ctx, f.userID, RecurrenceInput{CardID: c.ID, Description: "Barbearia", CategoryID: f.streaming, AmountCents: 5_000, StartDate: d(2026, time.October, 15)})
	if _, err := f.s.Pay(ctx, f.userID, Payment{CardID: c.ID, Month: civil.Month{Year: 2026, Month: time.November}, AccountID: f.accountID, Date: d(2026, time.November, 10)}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DeleteRecurrence(ctx, r.ID); !errors.Is(err, ErrRecurrenceInUse) {
		t.Fatalf("err = %v, want ErrRecurrenceInUse", err)
	}
	fresh, _ := f.s.AddRecurrence(ctx, f.userID, RecurrenceInput{CardID: c.ID, Description: "Internet", CategoryID: f.streaming, AmountCents: 9_990, StartDate: d(2026, time.November, 20)})
	if err := f.s.DeleteRecurrence(ctx, fresh.ID); err != nil {
		t.Fatalf("delete unpaid recurrence: %v", err)
	}
}

// TestPay pays the whole invoice from an account without counting it as spending (RF-21, RN-07).
func TestPay(t *testing.T) {
	f := newFixture(t) // today 2026-11-11
	ctx := groupCtx
	c := f.newCard(t)
	f.buy(t, c.ID, d(2026, time.October, 20), 12_345, 1) // November invoice, closed on Nov 3
	f.buy(t, c.ID, d(2026, time.November, 5), 1_000, 1)  // December invoice, open
	nov := civil.Month{Year: 2026, Month: time.November}

	tests := []struct {
		name    string
		in      Payment
		wantErr error
	}{
		{name: "open invoice", in: Payment{CardID: c.ID, Month: civil.Month{Year: 2026, Month: time.December}, AccountID: f.accountID, Date: d(2026, time.November, 11)}, wantErr: ErrInvoiceOpen},
		{name: "empty invoice", in: Payment{CardID: c.ID, Month: civil.Month{Year: 2026, Month: time.October}, AccountID: f.accountID, Date: d(2026, time.November, 11)}, wantErr: ErrNothingToPay},
		{name: "missing account", in: Payment{CardID: c.ID, Month: nov, AccountID: 999, Date: d(2026, time.November, 11)}, wantErr: ledger.ErrAccountNotFound},
		{name: "closed invoice", in: Payment{CardID: c.ID, Month: nov, AccountID: f.accountID, Date: d(2026, time.November, 10)}},
		{name: "twice", in: Payment{CardID: c.ID, Month: nov, AccountID: f.accountID, Date: d(2026, time.November, 10)}, wantErr: ErrAlreadyPaid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.s.Pay(ctx, f.userID, tt.in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}

	inv := f.invoice(t, c.ID, 2026, time.November)
	if inv.Status != StatusPaid || inv.Payment == nil || inv.Payment.AmountCents != 12_345 || inv.Payment.AccountName != "Nubank Ana" || inv.PaymentOutdated {
		t.Fatalf("invoice = %+v", inv)
	}
	if a, _ := f.ledger.GetAccount(ctx, f.accountID); a.BalanceCents != 1_000_000-12_345 {
		t.Fatalf("balance = %d", a.BalanceCents)
	}
	if flow, _ := f.ledger.MonthFlow(ctx, nov); flow.ExpenseCents != 0 {
		t.Fatalf("invoice payment counted as spending: %+v (RN-07)", flow)
	}

	// A purchase registered late in a paid invoice flags the payment as outdated.
	f.buy(t, c.ID, d(2026, time.October, 30), 500, 1)
	if inv := f.invoice(t, c.ID, 2026, time.November); !inv.PaymentOutdated || inv.TotalCents != 12_845 {
		t.Fatalf("after late purchase = %+v", inv)
	}

	// Deleting the payment entry from the account undoes the payment.
	if err := f.ledger.DeleteEntry(ctx, inv.Payment.EntryID); err != nil {
		t.Fatal(err)
	}
	if inv := f.invoice(t, c.ID, 2026, time.November); inv.Status != StatusOverdue || inv.Payment != nil {
		t.Fatalf("after undo = %+v", inv)
	}
}

// TestLimit counts every unpaid installment, including future ones (RF-20, RN-17).
func TestLimit(t *testing.T) {
	f := newFixture(t) // today 2026-11-11, current invoice 2026-12
	ctx := groupCtx
	c := f.newCard(t) // limit 5.000,00
	steps := []struct {
		name string
		do   func(t *testing.T)
		want int64
	}{
		{name: "empty card", do: func(*testing.T) {}, want: 0},
		{name: "purchase in 12 installments uses the whole amount", do: func(t *testing.T) { f.buy(t, c.ID, d(2026, time.October, 10), 120_000, 12) }, want: 120_000},
		{name: "paying the November invoice frees one installment", do: func(t *testing.T) {
			if _, err := f.s.Pay(ctx, f.userID, Payment{CardID: c.ID, Month: civil.Month{Year: 2026, Month: time.November}, AccountID: f.accountID, Date: d(2026, time.November, 10)}); err != nil {
				t.Fatal(err)
			}
		}, want: 110_000},
		{name: "recurrence counts up to the current invoice only", do: func(t *testing.T) {
			if _, err := f.s.AddRecurrence(ctx, f.userID, RecurrenceInput{CardID: c.ID, Description: "Internet", CategoryID: f.streaming, AmountCents: 10_000, StartDate: d(2026, time.November, 20)}); err != nil {
				t.Fatal(err)
			}
		}, want: 120_000},
		{name: "credit lowers the use", do: func(t *testing.T) {
			if _, err := f.s.AddCredit(ctx, f.userID, CreditInput{CardID: c.ID, Date: d(2026, time.November, 12), Merchant: "Estorno", AmountCents: 2_000}); err != nil {
				t.Fatal(err)
			}
		}, want: 118_000},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			st.do(t)
			got, err := f.s.GetCard(ctx, c.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.LimitUsedCents != st.want || got.LimitFreeCents != 500_000-st.want {
				t.Fatalf("used, free = %d, %d; want %d, %d", got.LimitUsedCents, got.LimitFreeCents, st.want, 500_000-st.want)
			}
		})
	}
}

// TestForecast sums installments and recurrences of the next invoices (RF-23).
func TestForecast(t *testing.T) {
	f := newFixture(t) // current invoice 2026-12
	ctx := groupCtx
	c := f.newCard(t)
	f.buy(t, c.ID, d(2026, time.November, 20), 30_000, 3) // Dec, Jan, Feb
	if _, err := f.s.AddRecurrence(ctx, f.userID, RecurrenceInput{CardID: c.ID, Description: "Streaming", CategoryID: f.streaming, AmountCents: 2_000, StartDate: d(2026, time.December, 20), EndMonth: &civil.Month{Year: 2027, Month: time.February}}); err != nil {
		t.Fatal(err)
	}
	got, err := f.s.Forecast(ctx, c.ID, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		month string
		total int64
	}{{"2026-12", 10_000}, {"2027-01", 12_000}, {"2027-02", 12_000}, {"2027-03", 0}}
	if len(got) != len(want) {
		t.Fatalf("forecast = %+v", got)
	}
	for i, w := range want {
		if got[i].Month.String() != w.month || got[i].TotalCents != w.total {
			t.Errorf("forecast[%d] = %s %d, want %s %d", i, got[i].Month, got[i].TotalCents, w.month, w.total)
		}
	}
}

// TestUpdateAndDeletePurchase regenerates the installments and removes purchases.
func TestUpdateAndDeletePurchase(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	c := f.newCard(t)
	p := f.buy(t, c.ID, d(2026, time.November, 20), 30_000, 3)
	in := PurchaseInput{CardID: c.ID, Date: d(2026, time.November, 1), Merchant: "Loja Nova", CategoryID: f.delivery, AmountCents: 10_001, Installments: 2}
	got, err := f.s.UpdatePurchase(ctx, p.ID, in)
	if err != nil || got.FirstInvoice.String() != "2026-11" || got.TotalCents != 10_001 {
		t.Fatalf("update = %+v, %v", got, err)
	}
	if inv := f.invoice(t, c.ID, 2026, time.November); inv.TotalCents != 5_001 || inv.Items[0].Merchant != "Loja Nova" {
		t.Fatalf("november = %+v", inv)
	}
	if inv := f.invoice(t, c.ID, 2027, time.February); inv.TotalCents != 0 {
		t.Fatalf("old installment left in february: %+v", inv)
	}
	if err := f.s.DeletePurchase(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DeletePurchase(ctx, p.ID); !errors.Is(err, ErrPurchaseNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

// TestUpdatePartialImport keeps the schedule of a purchase imported from a middle installment (RF-57).
func TestUpdatePartialImport(t *testing.T) {
	ctx := groupCtx
	carla := &SplitInput{Mode: SplitCustom, Parties: []PartyShare{{Name: "Carla Ficticia", ShareCents: 40_000}}}
	cases := []struct {
		name    string
		split   bool
		edit    func(in *PurchaseInput)
		wantErr error
	}{
		{name: "only texts and category", edit: func(in *PurchaseInput) { in.Merchant, in.CategoryID = "Loja Renomeada", 0 }},
		{name: "same split kept", split: true, edit: func(in *PurchaseInput) { in.Split = carla }},
		{name: "amount changed", edit: func(in *PurchaseInput) { in.AmountCents = 90_000 }, wantErr: ErrPartialImport},
		{name: "installments changed", edit: func(in *PurchaseInput) { in.Installments = 9 }, wantErr: ErrPartialImport},
		{name: "date changed", edit: func(in *PurchaseInput) { in.Date = d(2026, time.November, 20) }, wantErr: ErrPartialImport},
		{name: "per installment equal total", edit: func(in *PurchaseInput) { in.AmountCents, in.PerInstallment = 10_000, true }},
		{name: "split added", edit: func(in *PurchaseInput) { in.Split = carla }, wantErr: ErrPartialImport},
		{name: "split removed", split: true, edit: func(in *PurchaseInput) {}, wantErr: ErrPartialImport},
		{name: "split share changed", split: true, edit: func(in *PurchaseInput) {
			in.Split = &SplitInput{Mode: SplitCustom, Parties: []PartyShare{{Name: "Carla Ficticia", ShareCents: 30_000}}}
		}, wantErr: ErrPartialImport},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			c := f.newCard(t)
			line := ImportedLine{CardID: c.ID, Invoice: civil.Month{Year: 2026, Month: time.December}, Date: d(2026, time.November, 4),
				Merchant: "Loja Ficticia", CategoryID: &f.delivery, AmountCents: 10_000, Installment: 3, Installments: 10}
			if tc.split {
				line.Split = &SplitInput{Mode: SplitEqual, CoupleParts: 1, Parties: []PartyShare{{Name: "Carla Ficticia"}}}
			}
			var id int64
			if err := db.WithTx(ctx, f.conn, func(q *store.Queries) error {
				var err error
				id, err = f.s.ImportLine(ctx, q, f.userID, line)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			in := PurchaseInput{CardID: c.ID, Date: d(2026, time.November, 4), Merchant: "Loja Ficticia", CategoryID: f.delivery, AmountCents: 100_000, Installments: 10}
			tc.edit(&in)
			if in.CategoryID == 0 {
				in.CategoryID = f.streaming
			}
			got, err := f.s.UpdatePurchase(ctx, id, in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err == nil && (got.Merchant != in.Merchant || *got.CategoryID != in.CategoryID) {
				t.Fatalf("texts not updated: %+v", got)
			}
			for _, m := range []struct {
				y    int
				mo   time.Month
				want int64
			}{{2026, time.October, 0}, {2026, time.November, 0}, {2026, time.December, 10_000}, {2027, time.July, 10_000}, {2027, time.August, 0}, {2027, time.September, 0}} {
				if inv := f.invoice(t, c.ID, m.y, m.mo); inv.TotalCents != m.want {
					t.Fatalf("%d-%02d total = %d, want %d", m.y, m.mo, inv.TotalCents, m.want)
				}
			}
			p, _ := f.s.GetPurchase(ctx, id)
			var share int64
			for _, sp := range p.Splits {
				share += sp.ShareCents
			}
			if tc.split && share != 40_000 {
				t.Fatalf("share = %d, want 40000", share)
			}
		})
	}
}

// TestDeleteCard keeps cards with history.
func TestDeleteCard(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	used := f.newCard(t)
	f.buy(t, used.ID, d(2026, time.November, 1), 100, 1)
	in := f.nubank()
	in.Name = "Vazio"
	unused, _ := f.s.CreateCard(ctx, f.userID, in)
	if err := f.s.DeleteCard(ctx, used.ID); !errors.Is(err, ErrCardInUse) {
		t.Fatalf("err = %v, want ErrCardInUse", err)
	}
	if err := f.s.DeleteCard(ctx, unused.ID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ErrCardInUse.Message, "Desative") {
		t.Fatal("message should suggest deactivating")
	}
}

// TestGroupIsolation shows that a second group sees nothing of the cards, purchases and shares of the first (RF-82, RN-27).
func TestGroupIsolation(t *testing.T) {
	f := newFixture(t)
	c := f.newCard(t)
	p, err := f.s.AddPurchase(groupCtx, f.userID, PurchaseInput{CardID: c.ID, Date: d(2026, time.November, 1), Merchant: "Loja Ficticia",
		CategoryID: f.delivery, AmountCents: 10_000, Installments: 2,
		Split: &SplitInput{Mode: SplitEqual, CoupleParts: 1, Parties: []PartyShare{{Name: "Dora Ficticia"}}}})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := f.s.AddRecurrence(groupCtx, f.userID, RecurrenceInput{CardID: c.ID, Description: "Streaming", CategoryID: f.streaming, AmountCents: 2_190, StartDate: d(2026, time.October, 15)})
	if err != nil {
		t.Fatal(err)
	}
	carla := testutil.NewUser(t, f.conn, "carla", "Carla Ficticia")
	other := testutil.NewGroup(t, f.conn, "Outra", carla)
	nov := civil.Month{Year: 2026, Month: time.November}

	tests := []struct {
		name    string
		run     func() error
		wantErr error
	}{
		{name: "get card", run: func() error { _, err := f.s.GetCard(other, c.ID); return err }, wantErr: ErrCardNotFound},
		{name: "update card", run: func() error { _, err := f.s.UpdateCard(other, c.ID, f.nubank(), true); return err }, wantErr: ErrCardNotFound},
		{name: "delete card", run: func() error { return f.s.DeleteCard(other, c.ID) }, wantErr: ErrCardNotFound},
		{name: "invoice", run: func() error { _, err := f.s.Invoice(other, c.ID, nov); return err }, wantErr: ErrCardNotFound},
		{name: "forecast", run: func() error { _, err := f.s.Forecast(other, c.ID, 3); return err }, wantErr: ErrCardNotFound},
		{name: "pay invoice", run: func() error {
			_, err := f.s.Pay(other, carla.ID, Payment{CardID: c.ID, Month: nov, AccountID: f.accountID, Date: d(2026, time.November, 20)})
			return err
		}, wantErr: ErrCardNotFound},
		{name: "purchase on card of another group", run: func() error {
			_, err := f.s.AddPurchase(other, carla.ID, PurchaseInput{CardID: c.ID, Date: d(2026, time.November, 1), Merchant: "X", CategoryID: f.delivery, AmountCents: 100, Installments: 1})
			return err
		}, wantErr: ErrCardNotFound},
		{name: "holder from another group", run: func() error {
			in := f.nubank()
			in.Name = "Intruso"
			_, err := f.s.CreateCard(other, carla.ID, in)
			return err
		}, wantErr: ErrPersonNotFound},
		{name: "get purchase", run: func() error { _, err := f.s.GetPurchase(other, p.ID); return err }, wantErr: ErrPurchaseNotFound},
		{name: "update purchase", run: func() error {
			_, err := f.s.UpdatePurchase(other, p.ID, PurchaseInput{CardID: c.ID, Date: d(2026, time.November, 1), Merchant: "X", CategoryID: f.delivery, AmountCents: 100, Installments: 1})
			return err
		}, wantErr: ErrPurchaseNotFound},
		{name: "delete purchase", run: func() error { return f.s.DeletePurchase(other, p.ID) }, wantErr: ErrPurchaseNotFound},
		{name: "recurrences", run: func() error { _, err := f.s.Recurrences(other, c.ID); return err }, wantErr: ErrCardNotFound},
		{name: "update recurrence", run: func() error {
			_, err := f.s.UpdateRecurrence(other, rec.ID, RecurrenceChanges{Description: "X", CategoryID: f.streaming, AmountCents: 1})
			return err
		}, wantErr: ErrRecurrenceNotFound},
		{name: "delete recurrence", run: func() error { return f.s.DeleteRecurrence(other, rec.ID) }, wantErr: ErrRecurrenceNotFound},
		{name: "no active group", run: func() error { _, err := f.s.ListCards(context.Background()); return err }, wantErr: scope.ErrNoGroup},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}

	if cards, err := f.s.ListCards(other); err != nil || len(cards) != 0 {
		t.Fatalf("cards of the other group = %v, %v", cards, err)
	}
	if parties, err := f.s.Parties(other); err != nil || len(parties) != 0 {
		t.Fatalf("parties of the other group = %v, %v", parties, err)
	}
	if debt, err := f.s.ClosedDebt(other); err != nil || debt != 0 {
		t.Fatalf("debt of the other group = %d, %v", debt, err)
	}
	// The same name of a party is free in the other group, which has its own list.
	own, err := f.s.CreateCard(other, carla.ID, CardInput{Name: "Nubank", PersonID: carla.PersonID, LimitCents: 100_000, ClosingDay: 3, DueDay: 10, Color: "purple", Finish: "matte"})
	if err != nil {
		t.Fatal(err)
	}
	cat := f.delivery
	if _, err := f.s.AddPurchase(other, carla.ID, PurchaseInput{CardID: own.ID, Date: d(2026, time.November, 1), Merchant: "Loja", CategoryID: cat, AmountCents: 5_000, Installments: 1,
		Split: &SplitInput{Mode: SplitEqual, CoupleParts: 1, Parties: []PartyShare{{Name: "dora ficticia"}}}}); err != nil {
		t.Fatal(err)
	}
	if inv := f.invoice(t, c.ID, 2026, time.November); inv.TotalCents != 5_000+2_190 {
		t.Fatalf("invoice of the first group = %d, want 7190", inv.TotalCents)
	}
	if cards, _ := f.s.ListCards(groupCtx); len(cards) != 1 {
		t.Fatalf("cards of the first group = %d, want 1", len(cards))
	}
}
