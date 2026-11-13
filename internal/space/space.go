// Package space moves items between the personal space of a person and her groups (RF-83).
package space

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/problem"
	"github.com/italogfx/radar-finance/internal/scope"
)

// Kind names what is moved.
type Kind string

// Kinds of items that are personal or of a group (RF-83).
const (
	KindEntry     Kind = "entry"
	KindPurchase  Kind = "purchase"
	KindBill      Kind = "bill"
	KindFinancing Kind = "financing"
	KindIncome    Kind = "income"
	KindSavings   Kind = "savings"
	// KindAlelo is a purchase on an Alelo card. Card recurrences never move: that would rewrite closed invoices.
	KindAlelo Kind = "alelo"
)

// Errors of a move.
var (
	ErrUnknownKind  = problem.Invalid("move_kind_invalid", "Escolha o que mover.")
	ErrItemNotFound = problem.NotFound("move_item_not_found", "Item não encontrado neste espaço.")
	ErrNotMember    = problem.Forbidden("move_target_forbidden", "Você não faz parte desse espaço.")
	ErrNotCreator   = problem.Forbidden("move_not_creator", "Só quem lançou este item pode movê-lo para outro espaço.")
	ErrCannotMove   = problem.Conflict("move_blocked", "Não dá para mover este item para esse espaço: a conta, o cartão ou a pessoa dele não fazem parte dele, ou ele está ligado a outro registro, como uma compra dividida ou o pagamento de uma conta fixa.")
)

// Service moves items between spaces.
type Service struct {
	conn     *sql.DB
	accounts *account.Service
}

// NewService returns the service over the database.
func NewService(conn *sql.DB) *Service {
	return &Service{conn: conn, accounts: account.NewService(conn)}
}

// Move takes an item of the active space, with the entries that belong to it, to target, a space the user is in.
func (s *Service) Move(ctx context.Context, kind Kind, id, target int64) error {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return err
	}
	move, ok := moves[kind]
	if !ok {
		return ErrUnknownKind
	}
	if _, member, err := s.accounts.Membership(ctx, target, auth.UserID(ctx)); err != nil {
		return err
	} else if !member {
		return ErrNotMember
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		found, err := q.ItemInGroup(ctx, store.ItemInGroupParams{Kind: string(kind), ItemID: id, GroupID: groupID})
		if err != nil {
			return fmt.Errorf("find item: %w", err)
		}
		if found == 0 {
			return ErrItemNotFound
		}
		// Another member would take the item away from whoever recorded it, or show it in a group she chose not to.
		creator, err := q.ItemCreator(ctx, store.ItemCreatorParams{Kind: string(kind), ItemID: id})
		if err != nil {
			return fmt.Errorf("item creator: %w", err)
		}
		if creator != auth.UserID(ctx) {
			return ErrNotCreator
		}
		if target == groupID {
			return nil
		}
		return move(ctx, q, id, groupID, target)
	})
}

// moveFunc moves one item and its entries inside a transaction.
type moveFunc func(ctx context.Context, q *store.Queries, id, from, to int64) error

// moves holds how each kind moves; each one refuses when the item cannot be seen or stay whole in the target.
var moves = map[Kind]moveFunc{
	KindEntry: func(ctx context.Context, q *store.Queries, id, from, to int64) error {
		n, err := q.MoveEntry(ctx, store.MoveEntryParams{Target: to, ItemID: id, GroupID: from})
		return moved(n, err)
	},
	KindPurchase: func(ctx context.Context, q *store.Queries, id, from, to int64) error {
		n, err := q.MovePurchase(ctx, store.MovePurchaseParams{Target: to, ItemID: id, GroupID: from})
		return moved(n, err)
	},
	KindBill: func(ctx context.Context, q *store.Queries, id, from, to int64) error {
		n, err := q.MoveBill(ctx, store.MoveBillParams{Target: to, ItemID: id, GroupID: from})
		if err := moved(n, err); err != nil {
			return err
		}
		return q.MoveBillEntries(ctx, store.MoveBillEntriesParams{Target: to, ItemID: sql.NullInt64{Int64: id, Valid: true}})
	},
	KindFinancing: func(ctx context.Context, q *store.Queries, id, from, to int64) error {
		n, err := q.MoveFinancing(ctx, store.MoveFinancingParams{Target: to, ItemID: id, GroupID: from})
		if err := moved(n, err); err != nil {
			return err
		}
		return q.MoveFinancingEntries(ctx, store.MoveFinancingEntriesParams{Target: to, ItemID: id})
	},
	KindIncome: func(ctx context.Context, q *store.Queries, id, from, to int64) error {
		n, err := q.MoveIncome(ctx, store.MoveIncomeParams{Target: to, ItemID: id, GroupID: from})
		if err := moved(n, err); err != nil {
			return err
		}
		return q.MoveIncomeEntries(ctx, store.MoveIncomeEntriesParams{Target: to, ItemID: sql.NullInt64{Int64: id, Valid: true}})
	},
	KindSavings: func(ctx context.Context, q *store.Queries, id, from, to int64) error {
		n, err := q.MoveSavingsBox(ctx, store.MoveSavingsBoxParams{Target: to, ItemID: id, GroupID: from})
		if err := moved(n, err); err != nil {
			return err
		}
		return q.MoveSavingsEntries(ctx, store.MoveSavingsEntriesParams{Target: to, ItemID: id})
	},
	KindAlelo: func(ctx context.Context, q *store.Queries, id, from, to int64) error {
		n, err := q.MoveAleloPurchase(ctx, store.MoveAleloPurchaseParams{Target: to, ItemID: id, GroupID: from})
		return moved(n, err)
	},
}

// moved turns the count of a move into ErrCannotMove when the item stayed where it was.
func moved(n int64, err error) error {
	if err != nil {
		return fmt.Errorf("move: %w", err)
	}
	if n == 0 {
		return ErrCannotMove
	}
	return nil
}
