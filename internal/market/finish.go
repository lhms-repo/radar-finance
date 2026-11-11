package market

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/italogfx/radar-finance/internal/alelo"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/problem"
)

// PayMethod is how a finished purchase was paid (RF-77).
type PayMethod string

// Ways to pay a purchase: from an account, with a credit card or with an Alelo card.
const (
	PayAccount PayMethod = "account"
	PayCard    PayMethod = "card"
	PayAlelo   PayMethod = "alelo"
)

// ErrPayMethod is returned for an unknown way to pay.
var ErrPayMethod = problem.Invalid("list_pay_method", "Escolha como a compra foi paga: conta, cartão ou Alelo.")

// Finish is the real total of a purchase and how it was paid (RF-77).
type Finish struct {
	Method       PayMethod
	TotalCents   int64
	Date         civil.Date
	Description  string
	AccountID    int64
	CardID       int64
	Installments int
	AleloID      int64
}

// Payment is how a finished list was paid.
type Payment struct {
	Method      PayMethod
	TotalCents  int64
	Date        civil.Date
	Description string
}

// payments records the expense of a finished purchase in the right module.
type payments struct {
	ledger *ledger.Service
	cards  *card.Service
	alelo  *alelo.Service
}

// FinishList records the real total as an expense of Alimentação, Compras para casa, or as an Alelo purchase,
// and closes the list, all in one write (RF-77, RN-26).
func (s *Service) FinishList(ctx context.Context, userID, listID int64, in Finish) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if err := openList(ctx, q, listID); err != nil {
			return err
		}
		if err := s.pay(ctx, q, userID, in); err != nil {
			return err
		}
		return q.FinishShoppingList(ctx, store.FinishShoppingListParams{
			FinishedAt: sql.NullTime{Time: s.now().UTC(), Valid: true}, PaidCents: sql.NullInt64{Int64: in.TotalCents, Valid: true},
			PaidOn: sql.NullString{String: in.Date.String(), Valid: true}, PaidMethod: sql.NullString{String: string(in.Method), Valid: true},
			PaidWith: in.Description, ID: listID,
		})
	})
}

// pay writes the expense, the card purchase or the Alelo purchase inside the transaction.
func (s *Service) pay(ctx context.Context, q *store.Queries, userID int64, in Finish) error {
	switch in.Method {
	case PayAccount, PayCard:
		groceries, err := q.GetCategoryBySystemKey(ctx, sql.NullString{String: alelo.GroceriesKey, Valid: true})
		if err != nil {
			return fmt.Errorf("groceries category: %w", err)
		}
		if in.Method == PayAccount {
			_, err = s.payments.ledger.AddExpenseTx(ctx, q, userID, ledger.NewExpense{
				AccountID: in.AccountID, AmountCents: in.TotalCents, Date: in.Date, CategoryID: groceries, Description: in.Description,
			})
			return err
		}
		installments := in.Installments
		if installments < 1 {
			installments = 1
		}
		_, err = s.payments.cards.AddPurchaseTx(ctx, q, userID, card.PurchaseInput{
			CardID: in.CardID, Date: in.Date, Merchant: in.Description, CategoryID: groceries, AmountCents: in.TotalCents, Installments: installments,
		})
		return err
	case PayAlelo:
		_, err := s.payments.alelo.AddPurchaseTx(ctx, q, userID, alelo.Purchase{AleloID: in.AleloID, Date: in.Date, Description: in.Description, AmountCents: in.TotalCents})
		return err
	default:
		return ErrPayMethod
	}
}
