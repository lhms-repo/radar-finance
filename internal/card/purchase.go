package card

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/italogfx/radar-finance/internal/category"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/membersplit"
	"github.com/italogfx/radar-finance/internal/scope"
)

// Purchase kinds.
const (
	kindPurchase = "purchase"
	kindCredit   = "credit"
)

// Purchase is a card purchase or credit with the invoice of its first installment.
type Purchase struct {
	ID            int64
	CardID        int64
	Kind          string
	Date          civil.Date
	Merchant      string
	Description   string
	CategoryID    *int64
	BuyerPersonID *int64
	TotalCents    int64
	Installments  int
	FirstInvoice  civil.Month
	// Splits are the shares of other people; the couple keeps the rest (RF-24, RN-04).
	Splits           []SplitShare
	CoupleShareCents int64
	shares           []PartyShare
	createdBy        int64
}

// PurchaseInput registers a purchase (RF-15, RF-16); with PerInstallment the amount is of each installment.
type PurchaseInput struct {
	CardID         int64
	Date           civil.Date
	Merchant       string
	Description    string
	CategoryID     int64
	BuyerPersonID  *int64
	AmountCents    int64
	Installments   int
	PerInstallment bool
	// Split divides the purchase with other people; nil keeps it for the couple (RF-24).
	Split *SplitInput
	// Members divides the expense between the members of the group; nil keeps the weights (RN-28).
	Members *membersplit.Rule
}

// CreditInput registers a refund or another credit on the invoice of its date (RF-22).
type CreditInput struct {
	CardID      int64
	Date        civil.Date
	Merchant    string
	Description string
	CategoryID  *int64
	AmountCents int64
}

// AddPurchase registers a purchase and its installments on consecutive invoices (RF-15, RF-16, RF-18, RN-03).
func (s *Service) AddPurchase(ctx context.Context, userID int64, in PurchaseInput) (Purchase, error) {
	var id int64
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		var err error
		id, err = s.AddPurchaseTx(ctx, q, userID, in)
		return err
	})
	if err != nil {
		return Purchase{}, err
	}
	return s.getPurchase(ctx, store.New(s.conn), id)
}

// AddPurchaseTx registers the purchase inside a transaction of the caller and returns its id.
func (s *Service) AddPurchaseTx(ctx context.Context, q *store.Queries, userID int64, in PurchaseInput) (int64, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return 0, err
	}
	p, err := s.preparePurchase(ctx, q, in)
	if err != nil {
		return 0, err
	}
	now := s.now().UTC()
	row, err := q.CreatePurchase(ctx, store.CreatePurchaseParams{
		CardID: p.CardID, Kind: kindPurchase, PurchasedOn: p.Date.String(), Merchant: p.Merchant, Description: p.Description,
		CategoryID: nullID(p.CategoryID), BuyerPersonID: nullID(p.BuyerPersonID), TotalCents: p.TotalCents,
		InstallmentCount: int64(p.Installments), FirstInvoice: p.FirstInvoice.String(), CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
		GroupID: groupID,
	})
	if err != nil {
		return 0, fmt.Errorf("create purchase: %w", err)
	}
	if err := writeInstallments(ctx, q, row.ID, p, 1); err != nil {
		return 0, err
	}
	if err := s.writeSplits(ctx, q, userID, row.ID, p, p.shares); err != nil {
		return 0, err
	}
	part, err := q.PurchaseGroupPart(ctx, row.ID)
	if err != nil {
		return 0, err
	}
	return row.ID, membersplit.Save(ctx, q, groupID, membersplit.KindPurchase, row.ID, in.Members, part, s.now(), true)
}

// saveMembers records an edited division of a purchase, checked against its part as stored: the installments in
// the Radar without the shares of other people, even when it was imported from a middle installment (RF-84).
func (s *Service) saveMembers(ctx context.Context, q *store.Queries, groupID, id int64, r *membersplit.Rule) error {
	part, err := q.PurchaseGroupPart(ctx, id)
	if err != nil {
		return err
	}
	return membersplit.Save(ctx, q, groupID, membersplit.KindPurchase, id, r, part, s.now(), false)
}

// UpdatePurchase replaces a purchase and regenerates its installments and shares; the card stays the same.
func (s *Service) UpdatePurchase(ctx context.Context, id int64, in PurchaseInput) (Purchase, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return Purchase{}, err
	}
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		cur, err := s.getPurchase(ctx, q, id)
		if err != nil {
			return err
		}
		if cur.Kind != kindPurchase {
			return ErrPurchaseNotFound
		}
		if err := checkNoReceived(ctx, q, id); err != nil {
			return err
		}
		in.CardID = cur.CardID
		p, err := s.preparePurchase(ctx, q, in)
		if err != nil {
			return err
		}
		first, err := q.FirstInstallmentNumber(ctx, id)
		if err != nil {
			return err
		}
		if first > 1 {
			if err := updatePartialImport(ctx, q, groupID, cur, p, in.Split, s.now().UTC()); err != nil {
				return err
			}
			return s.saveMembers(ctx, q, groupID, id, in.Members)
		}
		if err := q.UpdatePurchase(ctx, store.UpdatePurchaseParams{
			PurchasedOn: p.Date.String(), Merchant: p.Merchant, Description: p.Description, CategoryID: nullID(p.CategoryID),
			BuyerPersonID: nullID(p.BuyerPersonID), TotalCents: p.TotalCents, InstallmentCount: int64(p.Installments),
			FirstInvoice: p.FirstInvoice.String(), UpdatedAt: s.now().UTC(), ID: id, GroupID: groupID,
		}); err != nil {
			return fmt.Errorf("update purchase: %w", err)
		}
		if err := q.DeleteInstallments(ctx, id); err != nil {
			return err
		}
		if err := q.DeleteSplits(ctx, id); err != nil {
			return err
		}
		if err := writeInstallments(ctx, q, id, p, 1); err != nil {
			return err
		}
		if err := s.writeSplits(ctx, q, cur.createdBy, id, p, p.shares); err != nil {
			return err
		}
		return s.saveMembers(ctx, q, groupID, id, in.Members)
	})
	if err != nil {
		return Purchase{}, err
	}
	return s.getPurchase(ctx, store.New(s.conn), id)
}

// updatePartialImport changes only the texts, category and buyer of a purchase imported from a middle installment,
// keeping its installments and shares, since the earlier installments are not in the Radar (RF-57).
func updatePartialImport(ctx context.Context, q *store.Queries, groupID int64, cur, p Purchase, split *SplitInput, now time.Time) error {
	if p.TotalCents != cur.TotalCents || p.Installments != cur.Installments || p.Date != cur.Date || !sameShares(cur.Splits, split) {
		return ErrPartialImport
	}
	if err := q.UpdatePurchase(ctx, store.UpdatePurchaseParams{
		PurchasedOn: cur.Date.String(), Merchant: p.Merchant, Description: p.Description, CategoryID: nullID(p.CategoryID),
		BuyerPersonID: nullID(p.BuyerPersonID), TotalCents: cur.TotalCents, InstallmentCount: int64(cur.Installments),
		FirstInvoice: cur.FirstInvoice.String(), UpdatedAt: now, ID: cur.ID, GroupID: groupID,
	}); err != nil {
		return fmt.Errorf("update purchase: %w", err)
	}
	return nil
}

// sameShares tells whether a split input repeats the stored shares, as the edit form sends them back.
func sameShares(cur []SplitShare, in *SplitInput) bool {
	if in == nil {
		return len(cur) == 0
	}
	if in.Mode != SplitCustom || len(in.Parties) != len(cur) {
		return false
	}
	for _, p := range in.Parties {
		found := false
		for _, c := range cur {
			if strings.EqualFold(strings.TrimSpace(p.Name), c.PartyName) && p.ShareCents == c.ShareCents {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// AddCredit registers a refund or credit as a negative amount on the invoice of its date (RF-22).
func (s *Service) AddCredit(ctx context.Context, userID int64, in CreditInput) (Purchase, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return Purchase{}, err
	}
	var id int64
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		c, err := activeCard(ctx, q, in.CardID)
		if err != nil {
			return err
		}
		merchant, desc, err := cleanTexts(in.Merchant, in.Description)
		if err != nil {
			return err
		}
		if err := ledger.CheckPositive(in.AmountCents); err != nil {
			return err
		}
		if in.Date.IsZero() {
			return ledger.ErrDateRequired
		}
		if in.CategoryID != nil {
			if err := category.CheckSelectable(ctx, q, *in.CategoryID); err != nil {
				return err
			}
		}
		p := Purchase{CardID: c.ID, Kind: kindCredit, Date: in.Date, Merchant: merchant, Description: desc, CategoryID: in.CategoryID,
			TotalCents: in.AmountCents, Installments: 1, FirstInvoice: c.Cycle().InvoiceFor(in.Date)}
		now := s.now().UTC()
		row, err := q.CreatePurchase(ctx, store.CreatePurchaseParams{
			CardID: c.ID, Kind: kindCredit, PurchasedOn: in.Date.String(), Merchant: merchant, Description: desc,
			CategoryID: nullID(in.CategoryID), TotalCents: in.AmountCents, InstallmentCount: 1,
			FirstInvoice: p.FirstInvoice.String(), CreatedBy: userID, CreatedAt: now, UpdatedAt: now, GroupID: groupID,
		})
		if err != nil {
			return fmt.Errorf("create credit: %w", err)
		}
		id = row.ID
		return writeInstallments(ctx, q, id, p, -1)
	})
	if err != nil {
		return Purchase{}, err
	}
	return s.getPurchase(ctx, store.New(s.conn), id)
}

// GetPurchase returns a purchase with the shares of other people.
func (s *Service) GetPurchase(ctx context.Context, id int64) (Purchase, error) {
	return s.getPurchase(ctx, store.New(s.conn), id)
}

// DeletePurchase removes a purchase or credit with its installments.
func (s *Service) DeletePurchase(ctx context.Context, id int64) error {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := s.getPurchase(ctx, q, id); err != nil {
			return err
		}
		if err := checkNoReceived(ctx, q, id); err != nil {
			return err
		}
		return q.DeletePurchase(ctx, store.DeletePurchaseParams{ID: id, GroupID: groupID})
	})
}

// preparePurchase validates the input and resolves the total and the first invoice.
func (s *Service) preparePurchase(ctx context.Context, q *store.Queries, in PurchaseInput) (Purchase, error) {
	c, err := activeCard(ctx, q, in.CardID)
	if err != nil {
		return Purchase{}, err
	}
	merchant, desc, err := cleanTexts(in.Merchant, in.Description)
	if err != nil {
		return Purchase{}, err
	}
	if err := ledger.CheckPositive(in.AmountCents); err != nil {
		return Purchase{}, err
	}
	if in.Installments < 1 || in.Installments > maxInstallments {
		return Purchase{}, ErrInvalidInstallments
	}
	total := in.AmountCents
	if in.PerInstallment {
		total = in.AmountCents * int64(in.Installments)
		if err := ledger.CheckPositive(total); err != nil {
			return Purchase{}, err
		}
	}
	if total < int64(in.Installments) {
		return Purchase{}, ErrInvalidInstallments
	}
	if in.Date.IsZero() {
		return Purchase{}, ledger.ErrDateRequired
	}
	if err := category.CheckSelectable(ctx, q, in.CategoryID); err != nil {
		return Purchase{}, err
	}
	if in.BuyerPersonID != nil {
		if err := checkPerson(ctx, q, *in.BuyerPersonID); err != nil {
			return Purchase{}, err
		}
	}
	shares, err := resolveShares(total, in.Installments, in.Split)
	if err != nil {
		return Purchase{}, err
	}
	categoryID := in.CategoryID
	return Purchase{
		CardID: c.ID, Kind: kindPurchase, Date: in.Date, Merchant: merchant, Description: desc, CategoryID: &categoryID,
		BuyerPersonID: in.BuyerPersonID, TotalCents: total, Installments: in.Installments, FirstInvoice: c.Cycle().InvoiceFor(in.Date),
		shares: shares,
	}, nil
}

// writeInstallments stores one installment per invoice from the first one; sign is -1 for credits (RN-03).
func writeInstallments(ctx context.Context, q *store.Queries, purchaseID int64, p Purchase, sign int64) error {
	for i, amount := range Split(p.TotalCents, p.Installments) {
		if err := q.CreateInstallment(ctx, store.CreateInstallmentParams{
			PurchaseID: purchaseID, CardID: p.CardID, InvoiceMonth: p.FirstInvoice.Add(i).String(),
			Number: int64(i + 1), AmountCents: sign * amount,
		}); err != nil {
			return fmt.Errorf("create installment: %w", err)
		}
	}
	return nil
}

// getPurchase reads a purchase or returns ErrPurchaseNotFound.
func (s *Service) getPurchase(ctx context.Context, q *store.Queries, id int64) (Purchase, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return Purchase{}, err
	}
	r, err := q.GetPurchase(ctx, store.GetPurchaseParams{ID: id, GroupID: groupID})
	if errors.Is(err, sql.ErrNoRows) {
		return Purchase{}, ErrPurchaseNotFound
	}
	if err != nil {
		return Purchase{}, err
	}
	on, err := civil.ParseDate(r.PurchasedOn)
	if err != nil {
		return Purchase{}, err
	}
	first, err := civil.ParseMonth(r.FirstInvoice)
	if err != nil {
		return Purchase{}, err
	}
	splits, err := q.ListPurchaseSplits(ctx, id)
	if err != nil {
		return Purchase{}, err
	}
	p := Purchase{
		ID: r.ID, CardID: r.CardID, Kind: r.Kind, Date: on, Merchant: r.Merchant, Description: r.Description,
		CategoryID: idPtr(r.CategoryID), BuyerPersonID: idPtr(r.BuyerPersonID), TotalCents: r.TotalCents,
		Installments: int(r.InstallmentCount), FirstInvoice: first, Splits: []SplitShare{}, CoupleShareCents: r.TotalCents,
		createdBy: r.CreatedBy,
	}
	for _, sp := range splits {
		p.Splits = append(p.Splits, SplitShare{PartyID: sp.PartyID, PartyName: sp.PartyName, ShareCents: sp.ShareCents})
		p.CoupleShareCents -= sp.ShareCents
	}
	return p, nil
}

// cleanTexts trims and validates the merchant and the description.
func cleanTexts(merchant, description string) (string, string, error) {
	merchant = strings.TrimSpace(merchant)
	if merchant == "" {
		return "", "", ErrMerchantRequired
	}
	if utf8.RuneCountInString(merchant) > maxMerchant {
		return "", "", ErrMerchantTooLong
	}
	desc, err := ledger.CleanDescription(description)
	return merchant, desc, err
}

// ImportedLine is a purchase or credit read from the invoice CSV, placed in the invoice chosen by the user.
type ImportedLine struct {
	CardID      int64
	Invoice     civil.Month
	Date        civil.Date
	Merchant    string
	CategoryID  *int64
	AmountCents int64
	Credit      bool
	// Installment and Installments come from "Parcela N/M"; zero means a single charge.
	Installment  int
	Installments int
	// Split divides the amount of the line, the value of each installment, with other people (RF-24).
	Split *SplitInput
}

// ImportLine records a CSV line inside the caller's transaction and returns the purchase id.
// A "Parcela N/M" line creates only installments N to M, from the chosen invoice on (RF-57).
func (s *Service) ImportLine(ctx context.Context, q *store.Queries, userID int64, in ImportedLine) (int64, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return 0, err
	}
	c, err := activeCard(ctx, q, in.CardID)
	if err != nil {
		return 0, err
	}
	merchant, _, err := cleanTexts(in.Merchant, "")
	if err != nil {
		return 0, err
	}
	if err := ledger.CheckPositive(in.AmountCents); err != nil {
		return 0, err
	}
	if in.Date.IsZero() {
		return 0, ledger.ErrDateRequired
	}
	n, m := in.Installment, in.Installments
	if n == 0 || m == 0 || in.Credit {
		n, m = 1, 1
	}
	if n > m || m > maxInstallments {
		return 0, ErrInvalidInstallments
	}
	kind, sign := kindPurchase, int64(1)
	if in.Credit {
		kind, sign = kindCredit, -1
	} else if in.CategoryID == nil {
		return 0, category.ErrNotSelectable
	}
	if in.CategoryID != nil {
		if err := category.CheckSelectable(ctx, q, *in.CategoryID); err != nil {
			return 0, err
		}
	}
	if in.Credit && in.Split != nil {
		return 0, ErrSplitCredit
	}
	// Shares are worked out on the line amount, the value of one installment.
	shares, err := resolveShares(in.AmountCents, 1, in.Split)
	if err != nil {
		return 0, err
	}
	first := in.Invoice.Add(-(n - 1))
	now := s.now().UTC()
	row, err := q.CreatePurchase(ctx, store.CreatePurchaseParams{
		CardID: c.ID, Kind: kind, PurchasedOn: in.Date.String(), Merchant: merchant, CategoryID: nullID(in.CategoryID),
		TotalCents: in.AmountCents * int64(m), InstallmentCount: int64(m), FirstInvoice: first.String(),
		CreatedBy: userID, CreatedAt: now, UpdatedAt: now, GroupID: groupID,
	})
	if err != nil {
		return 0, fmt.Errorf("create imported purchase: %w", err)
	}
	for k := n; k <= m; k++ {
		if err := q.CreateInstallment(ctx, store.CreateInstallmentParams{
			PurchaseID: row.ID, CardID: c.ID, InvoiceMonth: first.Add(k - 1).String(), Number: int64(k), AmountCents: sign * in.AmountCents,
		}); err != nil {
			return 0, fmt.Errorf("create imported installment: %w", err)
		}
	}
	if err := s.writeImportedSplits(ctx, q, userID, row.ID, c.ID, first, n, m, shares); err != nil {
		return 0, err
	}
	return row.ID, nil
}

// writeImportedSplits gives each person the same share on every installment created from the CSV, n to m only.
func (s *Service) writeImportedSplits(ctx context.Context, q *store.Queries, userID, purchaseID, cardID int64, first civil.Month, n, m int, shares []PartyShare) error {
	for _, sh := range shares {
		partyID, err := s.partyID(ctx, q, userID, sh.Name)
		if err != nil {
			return err
		}
		splitID, err := q.CreateSplit(ctx, store.CreateSplitParams{PurchaseID: purchaseID, PartyID: partyID, ShareCents: sh.ShareCents * int64(m-n+1)})
		if err != nil {
			return fmt.Errorf("create imported split: %w", err)
		}
		for k := n; k <= m; k++ {
			if err := q.CreateSplitInstallment(ctx, store.CreateSplitInstallmentParams{
				SplitID: splitID, Number: int64(k), CardID: cardID, InvoiceMonth: first.Add(k - 1).String(), AmountCents: sh.ShareCents,
			}); err != nil {
				return fmt.Errorf("create imported split installment: %w", err)
			}
		}
	}
	return nil
}

// RecurrenceChargesIn lists what the recurrences of a card bill on one invoice, in every space of the card, for
// duplicate checks (RN-20, RF-83).
func (s *Service) RecurrenceChargesIn(ctx context.Context, q *store.Queries, cardID int64, m civil.Month) ([]Recurrence, error) {
	c, err := getCard(ctx, q, cardID)
	if err != nil {
		return nil, err
	}
	all, err := s.recurrencesIn(ctx, q, c, view{full: true})
	if err != nil {
		return nil, err
	}
	var out []Recurrence
	for _, r := range all {
		if amount := r.priceIn(m); amount > 0 {
			r.AmountCents = amount
			out = append(out, r)
		}
	}
	return out, nil
}

// CheckNoReceived refuses changes to a purchase whose shares were already received.
func CheckNoReceived(ctx context.Context, q *store.Queries, purchaseID int64) error {
	return checkNoReceived(ctx, q, purchaseID)
}
