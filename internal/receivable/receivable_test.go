package receivable

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
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/ledger"
)

// fixture holds a migrated database with a card, an account and two purchases split with João and Maria.
type fixture struct {
	s         *Service
	cards     *card.Service
	ledger    *ledger.Service
	conn      *sql.DB
	userID    int64
	accountID int64
	cardID    int64
}

// newFixture registers an air conditioner in 3 installments split in equal parts with two people
// and a dinner split with João only.
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
	acc, _ := l.CreateAccount(ctx, ana.ID, ledger.NewAccount{Name: "Nubank Ana", PersonID: persons[0].ID})
	cards := card.NewService(conn)
	c, err := cards.CreateCard(ctx, ana.ID, card.CardInput{Name: "Nubank", PersonID: persons[0].ID, LimitCents: 1_000_000, ClosingDay: 3, DueDay: 10, Color: "purple", Finish: "matte"})
	if err != nil {
		t.Fatal(err)
	}
	var delivery int64
	_ = conn.QueryRow(`SELECT id FROM categories WHERE name = 'Delivery'`).Scan(&delivery)
	for _, p := range []card.PurchaseInput{
		{CardID: c.ID, Date: civil.Date{Year: 2026, Month: time.November, Day: 20}, Merchant: "Ar-condicionado", CategoryID: delivery, AmountCents: 300_000, Installments: 3,
			Split: &card.SplitInput{Mode: card.SplitEqual, CoupleParts: 1, Parties: []card.PartyShare{{Name: "João Ficticio"}, {Name: "Maria Ficticia"}}}},
		{CardID: c.ID, Date: civil.Date{Year: 2026, Month: time.November, Day: 25}, Merchant: "Jantar", CategoryID: delivery, AmountCents: 18_000, Installments: 1,
			Split: &card.SplitInput{Mode: card.SplitCustom, Parties: []card.PartyShare{{Name: "joão ficticio", ShareCents: 9_000}}}},
	} {
		if _, err := cards.AddPurchase(ctx, ana.ID, p); err != nil {
			t.Fatal(err)
		}
	}
	return fixture{s: NewService(conn), cards: cards, ledger: l, conn: conn, userID: ana.ID, accountID: acc.ID, cardID: c.ID}
}

// TestList groups what other people owe by person, with the total and each installment (RF-28).
func TestList(t *testing.T) {
	f := newFixture(t)
	got, err := f.s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "João Ficticio" || got[1].Name != "Maria Ficticia" {
		t.Fatalf("people = %+v", got)
	}
	joao := got[0]
	if joao.PendingCents != 100_000+9_000 || len(joao.Items) != 4 {
		t.Fatalf("João = %+v", joao)
	}
	tests := []struct {
		merchant string
		number   int
		count    int
		amount   int64
		due      string
	}{
		{merchant: "Ar-condicionado", number: 1, count: 3, amount: 33_334, due: "2026-12-10"},
		{merchant: "Jantar", number: 1, count: 1, amount: 9_000, due: "2026-12-10"},
		{merchant: "Ar-condicionado", number: 2, count: 3, amount: 33_333, due: "2027-01-10"},
		{merchant: "Ar-condicionado", number: 3, count: 3, amount: 33_333, due: "2027-02-10"},
	}
	for i, tt := range tests {
		it := joao.Items[i]
		if it.Merchant != tt.merchant || it.Number != tt.number || it.Count != tt.count || it.AmountCents != tt.amount || it.DueOn.String() != tt.due || it.Received {
			t.Errorf("item %d = %+v, want %+v", i, it, tt)
		}
	}
}

// TestMarkReceived records one entry for the shares paid together, without counting it as income (RF-26, RF-30, RN-06).
func TestMarkReceived(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	people, _ := f.s.List(ctx)
	joao, maria := people[0], people[1]
	dec := civil.Date{Year: 2026, Month: time.December, Day: 12}

	tests := []struct {
		name    string
		in      Receipt
		wantErr error
	}{
		{name: "nothing chosen", in: Receipt{AccountID: f.accountID, Date: dec}, wantErr: ErrNothingChosen},
		{name: "two people at once", in: Receipt{ItemIDs: []int64{joao.Items[0].ID, maria.Items[0].ID}, AccountID: f.accountID, Date: dec}, wantErr: ErrMixedPeople},
		{name: "unknown share", in: Receipt{ItemIDs: []int64{9999}, AccountID: f.accountID, Date: dec}, wantErr: ErrItemNotFound},
		{name: "missing account", in: Receipt{ItemIDs: []int64{joao.Items[0].ID}, AccountID: 999, Date: dec}, wantErr: ledger.ErrAccountNotFound},
		{name: "missing date", in: Receipt{ItemIDs: []int64{joao.Items[0].ID}, AccountID: f.accountID}, wantErr: ledger.ErrDateRequired},
		{name: "same share twice", in: Receipt{ItemIDs: []int64{joao.Items[0].ID, joao.Items[0].ID}, AccountID: f.accountID, Date: dec}, wantErr: ErrRepeatedItem},
		{name: "first installment and the dinner together", in: Receipt{ItemIDs: []int64{joao.Items[0].ID, joao.Items[1].ID}, AccountID: f.accountID, Date: dec}},
		{name: "already received", in: Receipt{ItemIDs: []int64{joao.Items[0].ID}, AccountID: f.accountID, Date: dec}, wantErr: ErrAlreadyReceived},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.s.MarkReceived(ctx, f.userID, tt.in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}

	if a, _ := f.ledger.GetAccount(ctx, f.accountID); a.BalanceCents != 33_334+9_000 {
		t.Fatalf("balance = %d", a.BalanceCents)
	}
	if flow, _ := f.ledger.MonthFlow(ctx, civil.Month{Year: 2026, Month: time.December}); flow.IncomeCents != 0 {
		t.Fatalf("a share paid back is not income: %+v (RN-09)", flow)
	}
	inv, _ := f.cards.Invoice(ctx, f.cardID, civil.Month{Year: 2026, Month: time.December})
	if inv.ThirdPartyCents != 33_334*2+9_000 || inv.ReceivedCents != 33_334+9_000 || inv.PendingCents != 33_334 {
		t.Fatalf("invoice third %d, received %d, pending %d", inv.ThirdPartyCents, inv.ReceivedCents, inv.PendingCents)
	}
	people, _ = f.s.List(ctx)
	if people[0].PendingCents != 66_666 || !people[0].Items[0].Received || people[0].Items[0].ReceivedIn != "Nubank Ana" {
		t.Fatalf("João after = %+v", people[0])
	}

	// Deleting the entry in the account statement makes both shares pending again.
	if err := f.ledger.DeleteEntry(ctx, *people[0].Items[0].EntryID); err != nil {
		t.Fatal(err)
	}
	people, _ = f.s.List(ctx)
	if people[0].PendingCents != 109_000 || people[0].Items[1].Received {
		t.Fatalf("João after undo = %+v", people[0])
	}
}

// TestMessage writes a ready to send charge with each pending installment and the total (RF-29).
func TestMessage(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	people, _ := f.s.List(ctx)
	if _, err := f.s.MarkReceived(ctx, f.userID, Receipt{ItemIDs: []int64{people[0].Items[1].ID}, AccountID: f.accountID, Date: civil.Date{Year: 2026, Month: time.December, Day: 1}}); err != nil {
		t.Fatal(err)
	}
	msg, err := f.s.Message(ctx, people[0].PartyID)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Oi, João!",
		"Ar-condicionado, parcela 1 de 3, fatura de 12/2026: R$ 333,34",
		"Ar-condicionado, parcela 3 de 3, fatura de 02/2027: R$ 333,33",
		"Total: R$ 1.000,00",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message misses %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "Jantar") || strings.ContainsAny(msg, "\u2014\u2013") {
		t.Fatalf("message has a received share or a dash:\n%s", msg)
	}
	if _, err := f.s.Message(ctx, 999); !errors.Is(err, ErrPartyNotFound) {
		t.Fatalf("unknown person: %v", err)
	}
}

// TestFormatBRL writes cents as Brazilian reais.
func TestFormatBRL(t *testing.T) {
	tests := []struct {
		cents int64
		want  string
	}{
		{0, "R$ 0,00"},
		{5, "R$ 0,05"},
		{33_334, "R$ 333,34"},
		{100_000, "R$ 1.000,00"},
		{123_456_789, "R$ 1.234.567,89"},
		{-1_050, "-R$ 10,50"},
	}
	for _, tt := range tests {
		if got := FormatBRL(tt.cents); got != tt.want {
			t.Errorf("FormatBRL(%d) = %q, want %q", tt.cents, got, tt.want)
		}
	}
}
