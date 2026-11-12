package market

import (
	"errors"
	"testing"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/alelo"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/ledger"
)

// TestFinish turns the real total of a list into an expense of Compras para casa by account or card,
// or an Alelo purchase, and closes the list in the same write (RF-77, RN-26).
func TestFinish(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	persons, _ := account.NewService(f.conn).ListPersons(ctx)
	acc, err := ledger.NewService(f.conn).CreateAccount(ctx, f.userID, ledger.NewAccount{Name: "Conta Ficticia", PersonID: persons[0].ID, InitialBalanceCents: 100_000})
	if err != nil {
		t.Fatal(err)
	}
	cc, err := card.NewService(f.conn).CreateCard(ctx, f.userID, card.CardInput{Name: "Cartão Ficticio", PersonID: persons[0].ID, LimitCents: 500_000, ClosingDay: 3, DueDay: 10, Color: "purple", Finish: "matte"})
	if err != nil {
		t.Fatal(err)
	}
	al, err := alelo.NewService(f.conn).Create(ctx, f.userID, alelo.Input{Name: "Alelo Ficticio", PersonID: persons[0].ID, MonthlyCreditCents: 80_000, CreditDay: 5, Style: "tudo"})
	if err != nil {
		t.Fatal(err)
	}
	var groceries int64
	_ = f.conn.QueryRow(`SELECT id FROM categories WHERE system_key = 'groceries'`).Scan(&groceries)

	tests := []struct {
		name    string
		in      Finish
		wantErr error
		check   string
	}{
		{name: "paid from an account", in: Finish{Method: PayAccount, AccountID: acc.ID, TotalCents: 23_456, Date: day(10), Description: "Compra no Mercado"},
			check: `SELECT count(*) FROM entries WHERE kind = 'expense' AND amount_cents = -23456 AND category_id = ? AND occurred_on = '2026-11-10'`},
		{name: "paid by card in two installments", in: Finish{Method: PayCard, CardID: cc.ID, Installments: 2, TotalCents: 30_000, Date: day(10), Description: "Compra no Mercado"},
			check: `SELECT count(*) FROM purchases WHERE total_cents = 30000 AND installment_count = 2 AND category_id = ?`},
		{name: "paid with Alelo", in: Finish{Method: PayAlelo, AleloID: al.ID, TotalCents: 12_000, Date: day(10), Description: "Compra no Mercado"},
			check: `SELECT count(*) FROM alelo_purchases WHERE amount_cents = 12000 AND category_id = ?`},
		{name: "unknown way", in: Finish{Method: "pix", TotalCents: 100, Date: day(10), Description: "X"}, wantErr: ErrPayMethod},
		{name: "account missing", in: Finish{Method: PayAccount, AccountID: 999, TotalCents: 100, Date: day(10), Description: "X"}, wantErr: ledger.ErrAccountNotFound},
		{name: "zero total", in: Finish{Method: PayAccount, AccountID: acc.ID, Date: day(10), Description: "X"}, wantErr: ledger.ErrAmountRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, _ := f.s.CreateList(ctx, f.userID, "Compra")
			err := f.s.FinishList(ctx, f.userID, l.ID, tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			got, _ := f.s.ListDetail(ctx, l.ID)
			if tt.wantErr != nil {
				// Nothing is half written: the list stays open.
				if got.Done {
					t.Fatal("list closed after an error")
				}
				return
			}
			var n int
			if err := f.conn.QueryRow(tt.check, groceries).Scan(&n); err != nil || n != 1 {
				t.Fatalf("records = %d, %v", n, err)
			}
			if !got.Done || got.Paid == nil || got.Paid.TotalCents != tt.in.TotalCents || got.Paid.Method != tt.in.Method {
				t.Fatalf("list = %+v, paid %+v", got.ShoppingList, got.Paid)
			}
			if err := f.s.FinishList(ctx, f.userID, l.ID, tt.in); !errors.Is(err, ErrListDone) {
				t.Fatalf("second finish: %v", err)
			}
		})
	}
}
