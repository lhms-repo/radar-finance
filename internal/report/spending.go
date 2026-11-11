// Package report computes the indicators of the overview: spending by category,
// budgets, expected balance, future commitments, month closing and net worth
// (RF-51, RF-52, RF-58 to RF-62). Every figure is computed here; the frontend
// only shows it.
package report

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/category"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/config"
	"github.com/italogfx/radar-finance/internal/db/store"
)

// UncategorizedName labels credits on the card without a category.
const UncategorizedName = "Sem categoria"

// Sub is the spending of a subcategory.
type Sub struct {
	ID    int64
	Name  string
	Cents int64
}

// CategorySpend is the spending of a top level category with its subcategories, largest first.
type CategorySpend struct {
	ID    int64
	Name  string
	Icon  string
	Cents int64
	Subs  []Sub
}

// MonthSpend is the spending of a month by category, largest first (RN-04).
type MonthSpend struct {
	Month      civil.Month
	TotalCents int64
	Categories []CategorySpend
}

// Service computes the reports.
type Service struct {
	conn       *sql.DB
	cards      *card.Service
	categories *category.Service
	loc        *time.Location
	today      func() civil.Date
}

// NewService creates the report service; today is read in America/Sao_Paulo.
func NewService(conn *sql.DB, cards *card.Service) *Service {
	loc, err := time.LoadLocation(config.Location)
	if err != nil {
		loc = time.UTC
	}
	return &Service{conn: conn, cards: cards, categories: category.NewService(conn), loc: loc, today: func() civil.Date { return civil.Today(loc) }}
}

// Spending returns the spending of each month from first to last, couple share only (RN-04).
// Card lines count in the month of their invoice (RN-01); account expenses and Alelo purchases
// by date. Invoice payments, transfers, adjustments, savings and receipts are not spending (RN-07, RN-08).
func (s *Service) Spending(ctx context.Context, first, last civil.Month) ([]MonthSpend, error) {
	q := store.New(s.conn)
	type key struct {
		month civil.Month
		cat   int64
	}
	sums := map[key]int64{}
	add := func(month string, cat, cents int64) error {
		m, err := civil.ParseMonth(month)
		if err != nil {
			return err
		}
		sums[key{m, cat}] += cents
		return nil
	}

	cardRows, err := q.CardSpendingByMonth(ctx, store.CardSpendingByMonthParams{FirstMonth: first.String(), LastMonth: last.String()})
	if err != nil {
		return nil, fmt.Errorf("card spending: %w", err)
	}
	for _, r := range cardRows {
		if err := add(r.Month, r.CategoryID, r.Cents); err != nil {
			return nil, err
		}
	}
	charges, err := s.cards.RecurrenceCharges(ctx, first, last)
	if err != nil {
		return nil, err
	}
	for _, c := range charges {
		sums[key{c.Month, c.CategoryID}] += c.Cents
	}
	days := store.EntrySpendingByMonthParams{FirstDay: first.First().String(), LastDay: last.Last().String()}
	entryRows, err := q.EntrySpendingByMonth(ctx, days)
	if err != nil {
		return nil, fmt.Errorf("entry spending: %w", err)
	}
	for _, r := range entryRows {
		if err := add(r.Month, r.CategoryID, r.Cents); err != nil {
			return nil, err
		}
	}
	aleloRows, err := q.AleloSpendingByMonth(ctx, store.AleloSpendingByMonthParams{FirstDay: days.FirstDay, LastDay: days.LastDay})
	if err != nil {
		return nil, fmt.Errorf("alelo spending: %w", err)
	}
	for _, r := range aleloRows {
		if err := add(r.Month, r.CategoryID, r.Cents); err != nil {
			return nil, err
		}
	}

	tree, err := s.categories.List(ctx, true)
	if err != nil {
		return nil, err
	}
	parentOf := map[int64]category.Category{}
	nameOf := map[int64]string{}
	for _, c := range tree {
		for _, sub := range c.Children {
			parentOf[sub.ID] = c
			nameOf[sub.ID] = sub.Name
		}
	}

	var out []MonthSpend
	for m := first; monthIndex(m) <= monthIndex(last); m = m.Add(1) {
		ms := MonthSpend{Month: m, Categories: []CategorySpend{}}
		byTop := map[int64]*CategorySpend{}
		for k, cents := range sums {
			if k.month != m || cents == 0 {
				continue
			}
			ms.TotalCents += cents
			top, ok := parentOf[k.cat]
			id, name, icon := top.ID, top.Name, top.Icon
			if !ok {
				id, name, icon = 0, UncategorizedName, "ellipsis"
			}
			cs, ok := byTop[id]
			if !ok {
				cs = &CategorySpend{ID: id, Name: name, Icon: icon, Subs: []Sub{}}
				byTop[id] = cs
			}
			cs.Cents += cents
			if id != 0 {
				cs.Subs = append(cs.Subs, Sub{ID: k.cat, Name: nameOf[k.cat], Cents: cents})
			}
		}
		for _, cs := range byTop {
			sort.Slice(cs.Subs, func(i, j int) bool { return cs.Subs[i].Cents > cs.Subs[j].Cents })
			ms.Categories = append(ms.Categories, *cs)
		}
		sort.Slice(ms.Categories, func(i, j int) bool {
			if ms.Categories[i].Cents != ms.Categories[j].Cents {
				return ms.Categories[i].Cents > ms.Categories[j].Cents
			}
			return ms.Categories[i].Name < ms.Categories[j].Name
		})
		out = append(out, ms)
	}
	return out, nil
}

// monthIndex orders months as a single number.
func monthIndex(m civil.Month) int { return m.Year*12 + int(m.Month) }
