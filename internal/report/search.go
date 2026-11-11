package report

import (
	"context"
	"fmt"
	"strings"

	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db/store"
)

// Source tells where a line of the search comes from.
type Source string

// Sources of the search.
const (
	SourceAccount Source = "account"
	SourceCard    Source = "card"
	SourceAlelo   Source = "alelo"
)

// Filter narrows the search; zero values are off (RF-63).
type Filter struct {
	From       civil.Date
	To         civil.Date
	CategoryID int64
	AccountID  int64
	CardID     int64
	PersonID   int64
	Text       string
}

// Line is one entry, card purchase or Alelo purchase found; amounts are signed, money out negative.
type Line struct {
	Source             Source
	ID                 int64
	Date               civil.Date
	Description        string
	AmountCents        int64
	Kind               string
	CategoryID         int64
	CategoryName       string
	ParentCategoryName string
	AccountName        string
	CardName           string
	PersonName         string
}

// Search finds entries, card purchases and Alelo purchases by period, category, account, card, person and text (RF-63).
func (s *Service) Search(ctx context.Context, f Filter) ([]Line, error) {
	rows, err := store.New(s.conn).SearchEntries(ctx, store.SearchEntriesParams{
		FirstDay: f.From.String(), LastDay: f.To.String(), CategoryID: f.CategoryID, AccountID: f.AccountID,
		CardID: f.CardID, PersonID: f.PersonID, Text: strings.TrimSpace(f.Text),
	})
	if err != nil {
		return nil, fmt.Errorf("search entries: %w", err)
	}
	out := make([]Line, 0, len(rows))
	for _, r := range rows {
		d, err := civil.ParseDate(r.OccurredOn)
		if err != nil {
			return nil, err
		}
		out = append(out, Line{
			Source: Source(r.Source), ID: r.ID, Date: d, Description: r.Description, AmountCents: r.AmountCents, Kind: r.Kind,
			CategoryID: r.CategoryID, CategoryName: r.CategoryName, ParentCategoryName: r.ParentCategoryName,
			AccountName: r.AccountName, CardName: r.CardName, PersonName: r.PersonName,
		})
	}
	return out, nil
}
