// Package savings manages the savings boxes: target, monthly percentage of the
// income of the month, suggested and confirmed deposits, withdrawals and the
// estimated completion (RF-43 to RF-49, RN-09 to RN-12).
package savings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/config"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/problem"
	"github.com/italogfx/radar-finance/internal/scope"
)

// Entry kinds of savings moves; neither is spending nor income (RN-08).
const (
	KindDepositEntry    ledger.Kind = "savings_deposit"
	KindWithdrawalEntry ledger.Kind = "savings_withdrawal"
)

// MoveKind tells a deposit from a withdrawal.
type MoveKind string

// Move kinds.
const (
	KindDeposit    MoveKind = "deposit"
	KindWithdrawal MoveKind = "withdrawal"
)

// fullBP is 100% in basis points.
const fullBP = 10_000

// estimateWindow is how many months before the current one feed the average deposit (RN-11).
const estimateWindow = 3

// Business errors returned by the service.
var (
	ErrNotFound       = problem.NotFound("savings_not_found", "Caixinha não encontrada.")
	ErrNameRequired   = problem.Invalid("savings_name_required", "Informe o nome da caixinha.")
	ErrNameTooLong    = problem.Invalid("savings_name_too_long", "Use no máximo 40 caracteres no nome.")
	ErrInvalidPercent = problem.Invalid("savings_percent_invalid", "Informe um percentual de 0 a 100.")
	ErrInvalidBalance = problem.Invalid("savings_balance_invalid", "Informe um saldo de zero para cima.")
	ErrReasonRequired = problem.Invalid("savings_reason_required", "Informe o motivo da retirada.")
	ErrInsufficient   = problem.Conflict("savings_insufficient", "A retirada passa do saldo da caixinha.")
	ErrInUse          = problem.Conflict("savings_in_use", "Esta caixinha já tem aportes ou retiradas e não pode ser excluída.")
)

// Box is a savings box as seen in one month (RF-48).
type Box struct {
	ID           int64
	Name         string
	TargetCents  int64
	PercentBP    int
	BalanceCents int64
	// ProgressBP is the balance over the target in basis points, capped at 100%.
	ProgressBP int
	Done       bool
	// SuggestedCents is the deposit suggested for the month, zero once deposited or done (RN-10, RN-12).
	SuggestedCents int64
	DepositedCents int64
	EstimatedMonth *civil.Month
	Moves          []MoveItem
	createdMonth   civil.Month
}

// MoveItem is one deposit or withdrawal of the history.
type MoveItem struct {
	ID            int64
	Kind          MoveKind
	Month         civil.Month
	Date          civil.Date
	AmountCents   int64
	Reason        string
	AccountName   string
	EntryID       int64
	CreatedByName string
}

// MonthView lists the boxes in a month with the base of the suggestions (RN-09).
type MonthView struct {
	Month     civil.Month
	BaseCents int64
	Boxes     []Box
}

// Input creates a box; the starting balance is the money already saved.
type Input struct {
	Name                string
	TargetCents         int64
	PercentBP           int
	InitialBalanceCents int64
}

// Changes edits target and percentage at any time (RF-44).
type Changes struct {
	Name        string
	TargetCents int64
	PercentBP   int
}

// Move is a deposit or a withdrawal between a box and an account.
type Move struct {
	BoxID       int64
	Month       civil.Month
	AmountCents int64
	AccountID   int64
	Date        civil.Date
	Reason      string
}

// Service reads and writes savings boxes.
type Service struct {
	conn   *sql.DB
	ledger *ledger.Service
	loc    *time.Location
	now    func() time.Time
	today  func() civil.Date
}

// NewService creates the savings service; today is read in America/Sao_Paulo.
func NewService(conn *sql.DB) *Service {
	loc, err := time.LoadLocation(config.Location)
	if err != nil {
		loc = time.UTC
	}
	return &Service{conn: conn, ledger: ledger.NewService(conn), loc: loc, now: time.Now, today: func() civil.Date { return civil.Today(loc) }}
}

// Create registers a box (RF-43).
func (s *Service) Create(ctx context.Context, userID int64, in Input) (Box, error) {
	name, err := validate(in.Name, in.TargetCents, in.PercentBP)
	if err != nil {
		return Box{}, err
	}
	if in.InitialBalanceCents < 0 || in.InitialBalanceCents > ledger.MaxAmountCents {
		return Box{}, ErrInvalidBalance
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return Box{}, err
	}
	now := s.now().UTC()
	row, err := store.New(s.conn).CreateSavingsBox(ctx, store.CreateSavingsBoxParams{
		Name: name, TargetCents: in.TargetCents, PercentBp: int64(in.PercentBP), InitialBalanceCents: in.InitialBalanceCents,
		CreatedBy: userID, CreatedAt: now, UpdatedAt: now, GroupID: groupID,
	})
	if err != nil {
		return Box{}, fmt.Errorf("create savings box: %w", err)
	}
	return s.get(ctx, row.ID)
}

// Update edits name, target and percentage (RF-44).
func (s *Service) Update(ctx context.Context, id int64, c Changes) (Box, error) {
	name, err := validate(c.Name, c.TargetCents, c.PercentBP)
	if err != nil {
		return Box{}, err
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return Box{}, err
	}
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := q.GetSavingsBox(ctx, store.GetSavingsBoxParams{ID: id, GroupID: groupID}); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		return q.UpdateSavingsBox(ctx, store.UpdateSavingsBoxParams{Name: name, TargetCents: c.TargetCents, PercentBp: int64(c.PercentBP), UpdatedAt: s.now().UTC(), ID: id, GroupID: groupID})
	})
	if err != nil {
		return Box{}, err
	}
	return s.get(ctx, id)
}

// Delete removes a box without moves.
func (s *Service) Delete(ctx context.Context, id int64) error {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := q.GetSavingsBox(ctx, store.GetSavingsBoxParams{ID: id, GroupID: groupID}); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if err := q.DeleteSavingsBox(ctx, store.DeleteSavingsBoxParams{ID: id, GroupID: groupID}); err != nil {
			if db.IsForeignKeyViolation(err) {
				return ErrInUse
			}
			return fmt.Errorf("delete savings box: %w", err)
		}
		return nil
	})
}

// Month lists every box as seen in the month, with the base of the suggestions (RF-45, RF-48, RN-09 to RN-12).
func (s *Service) Month(ctx context.Context, m civil.Month) (MonthView, error) {
	flow, err := s.ledger.MonthFlow(ctx, m)
	if err != nil {
		return MonthView{}, err
	}
	boxes, err := s.boxes(ctx, store.New(s.conn), m, flow.IncomeCents)
	if err != nil {
		return MonthView{}, err
	}
	return MonthView{Month: m, BaseCents: flow.IncomeCents, Boxes: boxes}, nil
}

// Deposit confirms a deposit with the amount really saved, taken from an account (RF-46, RN-08).
func (s *Service) Deposit(ctx context.Context, userID int64, mv Move) (ledger.Entry, error) {
	return s.move(ctx, userID, KindDeposit, mv)
}

// Withdraw takes money out of a box back into an account, with a reason (RF-47, RN-08).
func (s *Service) Withdraw(ctx context.Context, userID int64, mv Move) (ledger.Entry, error) {
	mv.Reason = strings.TrimSpace(mv.Reason)
	if mv.Reason == "" {
		return ledger.Entry{}, ErrReasonRequired
	}
	return s.move(ctx, userID, KindWithdrawal, mv)
}

// move writes the account entry and the move of a deposit or a withdrawal.
func (s *Service) move(ctx context.Context, userID int64, kind MoveKind, mv Move) (ledger.Entry, error) {
	if err := ledger.CheckPositive(mv.AmountCents); err != nil {
		return ledger.Entry{}, err
	}
	if mv.Date.IsZero() {
		return ledger.Entry{}, ledger.ErrDateRequired
	}
	reason, err := ledger.CleanDescription(mv.Reason)
	if err != nil {
		return ledger.Entry{}, err
	}
	if mv.Month == (civil.Month{}) {
		mv.Month = mv.Date.MonthOf()
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return ledger.Entry{}, err
	}
	var entryID int64
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		box, err := q.GetSavingsBox(ctx, store.GetSavingsBoxParams{ID: mv.BoxID, GroupID: groupID})
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := ledger.CheckActiveAccount(ctx, q, mv.AccountID); err != nil {
			return err
		}
		entryKind, amount, desc := KindDepositEntry, -mv.AmountCents, "Aporte em "+box.Name
		if kind == KindWithdrawal {
			boxes, err := s.boxes(ctx, q, mv.Month, 0)
			if err != nil {
				return err
			}
			for _, b := range boxes {
				if b.ID == box.ID && b.BalanceCents < mv.AmountCents {
					return ErrInsufficient
				}
			}
			entryKind, amount, desc = KindWithdrawalEntry, mv.AmountCents, "Retirada de "+box.Name
		}
		now := s.now().UTC()
		e, err := q.CreateEntry(ctx, store.CreateEntryParams{
			AccountID: mv.AccountID, Kind: string(entryKind), AmountCents: amount, OccurredOn: mv.Date.String(), Confirmed: true,
			Description: desc, CreatedBy: userID, CreatedAt: now, UpdatedAt: now, GroupID: groupID,
		})
		if err != nil {
			return fmt.Errorf("create savings entry: %w", err)
		}
		entryID = e.ID
		return q.CreateSavingsMove(ctx, store.CreateSavingsMoveParams{
			BoxID: box.ID, Kind: string(kind), Month: mv.Month.String(), AmountCents: mv.AmountCents, Reason: reason,
			EntryID: e.ID, CreatedBy: userID, CreatedAt: now,
		})
	})
	if err != nil {
		return ledger.Entry{}, err
	}
	return s.ledger.Entry(ctx, entryID)
}

// get returns one box as seen in the current month.
func (s *Service) get(ctx context.Context, id int64) (Box, error) {
	v, err := s.Month(ctx, s.today().MonthOf())
	if err != nil {
		return Box{}, err
	}
	for _, b := range v.Boxes {
		if b.ID == id {
			return b, nil
		}
	}
	return Box{}, ErrNotFound
}

// boxes builds every box as seen in month m, using base for the suggestion.
func (s *Service) boxes(ctx context.Context, q *store.Queries, m civil.Month, base int64) ([]Box, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListSavingsBoxes(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("list savings boxes: %w", err)
	}
	moves, err := q.ListSavingsMoves(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("list savings moves: %w", err)
	}
	out := make([]Box, 0, len(rows))
	for _, r := range rows {
		b := Box{
			ID: r.ID, Name: r.Name, TargetCents: r.TargetCents, PercentBP: int(r.PercentBp), BalanceCents: r.InitialBalanceCents,
			Moves: []MoveItem{}, createdMonth: civil.DateOf(r.CreatedAt.In(s.loc)).MonthOf(),
		}
		window := map[civil.Month]int64{}
		for _, mv := range moves {
			if mv.BoxID != b.ID {
				continue
			}
			item, err := toMove(mv)
			if err != nil {
				return nil, err
			}
			b.Moves = append(b.Moves, item)
			if item.Kind == KindDeposit {
				b.BalanceCents += item.AmountCents
				window[item.Month] += item.AmountCents
				if item.Month == m {
					b.DepositedCents += item.AmountCents
				}
			} else {
				b.BalanceCents -= item.AmountCents
			}
		}
		left := max(0, b.TargetCents-b.BalanceCents)
		b.Done = left == 0
		b.ProgressBP = int(min(int64(fullBP), b.BalanceCents*fullBP/b.TargetCents))
		if !b.Done && b.DepositedCents == 0 {
			b.SuggestedCents = min(base*int64(b.PercentBP)/fullBP, left)
		}
		if !b.Done {
			b.EstimatedMonth = estimate(m, b.createdMonth, window, left, b.DepositedCents > 0)
		}
		// Newest moves first for the history.
		for i, j := 0, len(b.Moves)-1; i < j; i, j = i+1, j-1 {
			b.Moves[i], b.Moves[j] = b.Moves[j], b.Moves[i]
		}
		out = append(out, b)
	}
	return out, nil
}

// estimate projects the month the box reaches the target from the average deposit of up to three months
// before m, counting only months the box existed; nil without deposits in that window (RN-11).
func estimate(m, created civil.Month, deposits map[civil.Month]int64, left int64, depositedThisMonth bool) *civil.Month {
	var sum int64
	var months int64
	for i := 1; i <= estimateWindow; i++ {
		w := m.Add(-i)
		if monthIndex(w) < monthIndex(created) {
			break
		}
		months++
		sum += deposits[w]
	}
	if sum <= 0 {
		return nil
	}
	// Deposits still needed, rounded up: left / (sum / months).
	needed := int((left*months + sum - 1) / sum)
	first := m
	if depositedThisMonth {
		first = m.Add(1)
	}
	end := first.Add(needed - 1)
	return &end
}

// toMove converts a stored move.
func toMove(r store.ListSavingsMovesRow) (MoveItem, error) {
	m, err := civil.ParseMonth(r.Month)
	if err != nil {
		return MoveItem{}, err
	}
	on, err := civil.ParseDate(r.OccurredOn)
	if err != nil {
		return MoveItem{}, err
	}
	return MoveItem{
		ID: r.ID, Kind: MoveKind(r.Kind), Month: m, Date: on, AmountCents: r.AmountCents, Reason: r.Reason, AccountName: r.AccountName,
		EntryID: r.EntryID, CreatedByName: r.CreatedByName,
	}, nil
}

// validate trims the name and checks name, target and percentage.
func validate(name string, target int64, percent int) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", ErrNameRequired
	case utf8.RuneCountInString(name) > 40:
		return "", ErrNameTooLong
	case percent < 0 || percent > fullBP:
		return "", ErrInvalidPercent
	}
	return name, ledger.CheckPositive(target)
}

// monthIndex orders months as a single number.
func monthIndex(m civil.Month) int { return m.Year*12 + int(m.Month) }
