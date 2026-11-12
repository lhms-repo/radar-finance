package card

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/italogfx/radar-finance/internal/category"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/scope"
)

// Recurrence is a charge repeated on every invoice, like a subscription (RF-17).
type Recurrence struct {
	ID                 int64
	CardID             int64
	Description        string
	CategoryID         int64
	CategoryName       string
	ParentCategoryName string
	CategoryIcon       string
	// AmountCents is the price on the current invoice, or on the first one when it has not started yet.
	AmountCents int64
	StartMonth  civil.Month
	EndMonth    *civil.Month
	prices      []price
}

// price is the amount of a recurrence from an invoice on.
type price struct {
	from   civil.Month
	amount int64
}

// RecurrenceInput registers a recurrence; the first invoice is the one of the start date.
type RecurrenceInput struct {
	CardID      int64
	Description string
	CategoryID  int64
	AmountCents int64
	StartDate   civil.Date
	EndMonth    *civil.Month
}

// RecurrenceChanges replaces the editable fields; a new amount applies from the current invoice on.
type RecurrenceChanges struct {
	Description string
	CategoryID  int64
	AmountCents int64
	EndMonth    *civil.Month
}

// priceIn returns the amount charged on invoice m, or zero when the recurrence is not active then.
func (r Recurrence) priceIn(m civil.Month) int64 {
	if monthIndex(m) < monthIndex(r.StartMonth) || (r.EndMonth != nil && monthIndex(m) > monthIndex(*r.EndMonth)) {
		return 0
	}
	var amount int64
	for _, p := range r.prices {
		if monthIndex(p.from) <= monthIndex(m) {
			amount = p.amount
		}
	}
	return amount
}

// Recurrences lists the recurrences of a card with their current price.
func (s *Service) Recurrences(ctx context.Context, cardID int64) ([]Recurrence, error) {
	q := store.New(s.conn)
	c, err := getCard(ctx, q, cardID)
	if err != nil {
		return nil, err
	}
	return s.recurrences(ctx, q, c)
}

// AddRecurrence registers a recurrence on a card (RF-17).
func (s *Service) AddRecurrence(ctx context.Context, userID int64, in RecurrenceInput) (Recurrence, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return Recurrence{}, err
	}
	var id int64
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		c, err := activeCard(ctx, q, in.CardID)
		if err != nil {
			return err
		}
		desc, err := cleanRecurrence(ctx, q, in.Description, in.CategoryID, in.AmountCents)
		if err != nil {
			return err
		}
		if in.StartDate.IsZero() {
			return ledger.ErrDateRequired
		}
		start := c.Cycle().InvoiceFor(in.StartDate)
		if in.EndMonth != nil && monthIndex(*in.EndMonth) < monthIndex(start) {
			return ErrEndBeforeStart
		}
		now := s.now().UTC()
		r, err := q.CreateRecurrence(ctx, store.CreateRecurrenceParams{
			CardID: c.ID, Description: desc, CategoryID: in.CategoryID, StartMonth: start.String(), EndMonth: nullMonth(in.EndMonth),
			CreatedBy: userID, CreatedAt: now, UpdatedAt: now, GroupID: groupID,
		})
		if err != nil {
			return fmt.Errorf("create recurrence: %w", err)
		}
		id = r.ID
		return q.UpsertRecurrencePrice(ctx, store.UpsertRecurrencePriceParams{RecurrenceID: id, FromMonth: start.String(), AmountCents: in.AmountCents})
	})
	if err != nil {
		return Recurrence{}, err
	}
	return s.getRecurrence(ctx, id)
}

// UpdateRecurrence edits a recurrence; a new price starts on the current invoice, so closed invoices keep theirs.
func (s *Service) UpdateRecurrence(ctx context.Context, id int64, ch RecurrenceChanges) (Recurrence, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return Recurrence{}, err
	}
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		cur, err := s.recurrenceTx(ctx, q, id)
		if err != nil {
			return err
		}
		c, err := getCard(ctx, q, cur.CardID)
		if err != nil {
			return err
		}
		desc, err := cleanRecurrence(ctx, q, ch.Description, ch.CategoryID, ch.AmountCents)
		if err != nil {
			return err
		}
		if ch.EndMonth != nil && monthIndex(*ch.EndMonth) < monthIndex(cur.StartMonth) {
			return ErrEndBeforeStart
		}
		if err := q.UpdateRecurrence(ctx, store.UpdateRecurrenceParams{
			Description: desc, CategoryID: ch.CategoryID, EndMonth: nullMonth(ch.EndMonth), UpdatedAt: s.now().UTC(), ID: id, GroupID: groupID,
		}); err != nil {
			return fmt.Errorf("update recurrence: %w", err)
		}
		from := c.Cycle().InvoiceFor(s.today())
		if monthIndex(from) < monthIndex(cur.StartMonth) {
			from = cur.StartMonth
		}
		if cur.priceIn(from) == ch.AmountCents {
			return nil
		}
		// Prices planned for later invoices give way to the new one.
		if err := q.DeleteRecurrencePricesAfter(ctx, store.DeleteRecurrencePricesAfterParams{RecurrenceID: id, FromMonth: from.String()}); err != nil {
			return err
		}
		return q.UpsertRecurrencePrice(ctx, store.UpsertRecurrencePriceParams{RecurrenceID: id, FromMonth: from.String(), AmountCents: ch.AmountCents})
	})
	if err != nil {
		return Recurrence{}, err
	}
	return s.getRecurrence(ctx, id)
}

// DeleteRecurrence removes a recurrence that is not on any paid invoice.
func (s *Service) DeleteRecurrence(ctx context.Context, id int64) error {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		r, err := s.recurrenceTx(ctx, q, id)
		if err != nil {
			return err
		}
		payments, err := q.ListCardPayments(ctx, store.ListCardPaymentsParams{GroupID: groupID, CardID: r.CardID})
		if err != nil {
			return err
		}
		for _, p := range payments {
			m, err := civil.ParseMonth(p.InvoiceMonth)
			if err != nil {
				return err
			}
			if r.priceIn(m) > 0 {
				return ErrRecurrenceInUse
			}
		}
		return q.DeleteRecurrence(ctx, store.DeleteRecurrenceParams{ID: id, GroupID: groupID})
	})
}

// getRecurrence reads one recurrence outside a transaction.
func (s *Service) getRecurrence(ctx context.Context, id int64) (Recurrence, error) {
	return s.recurrenceTx(ctx, store.New(s.conn), id)
}

// recurrenceTx reads one recurrence with its prices.
func (s *Service) recurrenceTx(ctx context.Context, q *store.Queries, id int64) (Recurrence, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return Recurrence{}, err
	}
	r, err := q.GetRecurrence(ctx, store.GetRecurrenceParams{ID: id, GroupID: groupID})
	if errors.Is(err, sql.ErrNoRows) {
		return Recurrence{}, ErrRecurrenceNotFound
	}
	if err != nil {
		return Recurrence{}, err
	}
	c, err := getCard(ctx, q, r.CardID)
	if err != nil {
		return Recurrence{}, err
	}
	all, err := s.recurrences(ctx, q, c)
	if err != nil {
		return Recurrence{}, err
	}
	for _, x := range all {
		if x.ID == id {
			return x, nil
		}
	}
	return Recurrence{}, ErrRecurrenceNotFound
}

// recurrences reads every recurrence of the card with prices, resolving the current amount.
func (s *Service) recurrences(ctx context.Context, q *store.Queries, c Card) ([]Recurrence, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCardRecurrences(ctx, store.ListCardRecurrencesParams{GroupID: groupID, CardID: c.ID})
	if err != nil {
		return nil, fmt.Errorf("list recurrences: %w", err)
	}
	prices, err := q.ListRecurrencePrices(ctx, store.ListRecurrencePricesParams{GroupID: groupID, CardID: c.ID})
	if err != nil {
		return nil, fmt.Errorf("list recurrence prices: %w", err)
	}
	byID := map[int64][]price{}
	for _, p := range prices {
		m, err := civil.ParseMonth(p.FromMonth)
		if err != nil {
			return nil, err
		}
		byID[p.RecurrenceID] = append(byID[p.RecurrenceID], price{from: m, amount: p.AmountCents})
	}
	current := c.Cycle().InvoiceFor(s.today())
	out := make([]Recurrence, 0, len(rows))
	for _, r := range rows {
		start, err := civil.ParseMonth(r.StartMonth)
		if err != nil {
			return nil, err
		}
		rec := Recurrence{
			ID: r.ID, CardID: r.CardID, Description: r.Description, CategoryID: r.CategoryID, CategoryName: r.CategoryName,
			ParentCategoryName: r.ParentCategoryName, CategoryIcon: r.CategoryIcon, StartMonth: start, prices: byID[r.ID],
		}
		if r.EndMonth.Valid {
			end, err := civil.ParseMonth(r.EndMonth.String)
			if err != nil {
				return nil, err
			}
			rec.EndMonth = &end
		}
		at := current
		if monthIndex(at) < monthIndex(start) {
			at = start
		}
		if rec.EndMonth != nil && monthIndex(at) > monthIndex(*rec.EndMonth) {
			at = *rec.EndMonth
		}
		rec.AmountCents = rec.priceIn(at)
		out = append(out, rec)
	}
	return out, nil
}

// cleanRecurrence validates the description, category and amount of a recurrence.
func cleanRecurrence(ctx context.Context, q *store.Queries, description string, categoryID, amount int64) (string, error) {
	desc := strings.TrimSpace(description)
	if desc == "" {
		return "", ErrDescriptionRequired
	}
	if utf8.RuneCountInString(desc) > maxMerchant {
		return "", ErrMerchantTooLong
	}
	if err := ledger.CheckPositive(amount); err != nil {
		return "", err
	}
	if err := category.CheckSelectable(ctx, q, categoryID); err != nil {
		return "", err
	}
	return desc, nil
}

// nullMonth stores an optional month as text.
func nullMonth(m *civil.Month) sql.NullString {
	if m == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: m.String(), Valid: true}
}

// monthIndex orders months as a single number.
func monthIndex(m civil.Month) int { return m.Year*12 + int(m.Month) }

// Charge is what a recurrence bills on one invoice.
type Charge struct {
	Month      civil.Month
	CardID     int64
	CategoryID int64
	Cents      int64
}

// RecurrenceCharges lists what every recurrence bills on the invoices from first to last, for the reports (RF-59).
func (s *Service) RecurrenceCharges(ctx context.Context, first, last civil.Month) ([]Charge, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return nil, err
	}
	q := store.New(s.conn)
	rows, err := q.ListCards(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("list cards: %w", err)
	}
	var out []Charge
	for _, r := range rows {
		c := Card{ID: r.ID, ClosingDay: int(r.ClosingDay), DueDay: int(r.DueDay)}
		recs, err := s.recurrences(ctx, q, c)
		if err != nil {
			return nil, err
		}
		for _, rec := range recs {
			for m := first; monthIndex(m) <= monthIndex(last); m = m.Add(1) {
				if cents := rec.priceIn(m); cents > 0 {
					out = append(out, Charge{Month: m, CardID: c.ID, CategoryID: rec.CategoryID, Cents: cents})
				}
			}
		}
	}
	return out, nil
}
