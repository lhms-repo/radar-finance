package report

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/problem"
	"github.com/italogfx/radar-finance/internal/scope"
)

// Thresholds of the budget alerts in basis points (RF-52).
const (
	warningBP = 8_000
	overBP    = 10_000
)

// ErrBudgetTopLevel is returned for a budget on a subcategory or on no category (RF-51).
var ErrBudgetTopLevel = problem.Invalid("budget_top_level", "O orçamento é definido nas categorias principais.")

// Level tells how close the spending is to the budget.
type Level string

// Budget levels: below 80%, from 80% and from 100% (RF-52).
const (
	LevelOK      Level = "ok"
	LevelWarning Level = "warning"
	LevelOver    Level = "over"
)

// BudgetStatus is a budget compared with the spending of the month.
type BudgetStatus struct {
	CategoryID  int64
	Name        string
	Icon        string
	BudgetCents int64
	SpentCents  int64
	// UsedBP is the spending over the budget in basis points, not capped.
	UsedBP int
	Level  Level
}

// SetBudget sets the monthly budget of a top level category of the active group; zero removes it (RF-51).
func (s *Service) SetBudget(ctx context.Context, userID, categoryID, cents int64) error {
	if cents < 0 {
		return ledger.ErrAmountRequired
	}
	if cents > ledger.MaxAmountCents {
		return ledger.ErrAmountTooLarge
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return err
	}
	q := store.New(s.conn)
	c, err := q.GetCategory(ctx, categoryID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && c.ParentID.Valid) {
		return ErrBudgetTopLevel
	}
	if err != nil {
		return err
	}
	if cents == 0 {
		return q.DeleteBudget(ctx, store.DeleteBudgetParams{GroupID: groupID, CategoryID: categoryID})
	}
	return q.UpsertBudget(ctx, store.UpsertBudgetParams{GroupID: groupID, CategoryID: categoryID, AmountCents: cents, CreatedBy: userID, UpdatedAt: s.now().UTC()})
}

// Budgets compares each budget with the spending of the month, the most critical first (RF-52).
func (s *Service) Budgets(ctx context.Context, m civil.Month) ([]BudgetStatus, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := store.New(s.conn).ListBudgets(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("list budgets: %w", err)
	}
	spend, err := s.Spending(ctx, m, m)
	if err != nil {
		return nil, err
	}
	spent := map[int64]int64{}
	for _, c := range spend[0].Categories {
		spent[c.ID] = c.Cents
	}
	tree, err := s.categories.List(ctx, true)
	if err != nil {
		return nil, err
	}
	out := make([]BudgetStatus, 0, len(rows))
	for _, r := range rows {
		b := BudgetStatus{CategoryID: r.CategoryID, BudgetCents: r.AmountCents, SpentCents: spent[r.CategoryID]}
		for _, c := range tree {
			if c.ID == r.CategoryID {
				b.Name, b.Icon = c.Name, c.Icon
			}
		}
		b.UsedBP = int(max(0, b.SpentCents) * fullBP / b.BudgetCents)
		switch {
		case b.UsedBP >= overBP:
			b.Level = LevelOver
		case b.UsedBP >= warningBP:
			b.Level = LevelWarning
		default:
			b.Level = LevelOK
		}
		out = append(out, b)
	}
	rank := map[Level]int{LevelOver: 0, LevelWarning: 1, LevelOK: 2}
	sort.Slice(out, func(i, j int) bool {
		if rank[out[i].Level] != rank[out[j].Level] {
			return rank[out[i].Level] < rank[out[j].Level]
		}
		if out[i].UsedBP != out[j].UsedBP {
			return out[i].UsedBP > out[j].UsedBP
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}
