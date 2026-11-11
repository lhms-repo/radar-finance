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
		p, err := s.preparePurchase(ctx, q, in)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		row, err := q.CreatePurchase(ctx, store.CreatePurchaseParams{
			CardID: p.CardID, Kind: kindPurchase, PurchasedOn: p.Date.String(), Merchant: p.Merchant, Description: p.Description,
			CategoryID: nullID(p.CategoryID), BuyerPersonID: nullID(p.BuyerPersonID), TotalCents: p.TotalCents,
			InstallmentCount: int64(p.Installments), FirstInvoice: p.FirstInvoice.String(), CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create purchase: %w", err)
		}
		id = row.ID
		return writeInstallments(ctx, q, id, p, 1)
	})
	if err != nil {
		return Purchase{}, err
	}
	return s.getPurchase(ctx, store.New(s.conn), id)
}

// UpdatePurchase replaces a purchase and regenerates its installments; the card stays the same.
func (s *Service) UpdatePurchase(ctx context.Context, id int64, in PurchaseInput) (Purchase, error) {
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		cur, err := s.getPurchase(ctx, q, id)
		if err != nil {
			return err
		}
		if cur.Kind != kindPurchase {
			return ErrPurchaseNotFound
		}
		in.CardID = cur.CardID
		p, err := s.preparePurchase(ctx, q, in)
		if err != nil {
			return err
		}
		if err := q.UpdatePurchase(ctx, store.UpdatePurchaseParams{
			PurchasedOn: p.Date.String(), Merchant: p.Merchant, Description: p.Description, CategoryID: nullID(p.CategoryID),
			BuyerPersonID: nullID(p.BuyerPersonID), TotalCents: p.TotalCents, InstallmentCount: int64(p.Installments),
			FirstInvoice: p.FirstInvoice.String(), UpdatedAt: s.now().UTC(), ID: id,
		}); err != nil {
			return fmt.Errorf("update purchase: %w", err)
		}
		if err := q.DeleteInstallments(ctx, id); err != nil {
			return err
		}
		return writeInstallments(ctx, q, id, p, 1)
	})
	if err != nil {
		return Purchase{}, err
	}
	return s.getPurchase(ctx, store.New(s.conn), id)
}

// AddCredit registers a refund or credit as a negative amount on the invoice of its date (RF-22).
func (s *Service) AddCredit(ctx context.Context, userID int64, in CreditInput) (Purchase, error) {
	var id int64
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
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
			FirstInvoice: p.FirstInvoice.String(), CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
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

// DeletePurchase removes a purchase or credit with its installments.
func (s *Service) DeletePurchase(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := s.getPurchase(ctx, q, id); err != nil {
			return err
		}
		return q.DeletePurchase(ctx, id)
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
	categoryID := in.CategoryID
	return Purchase{
		CardID: c.ID, Kind: kindPurchase, Date: in.Date, Merchant: merchant, Description: desc, CategoryID: &categoryID,
		BuyerPersonID: in.BuyerPersonID, TotalCents: total, Installments: in.Installments, FirstInvoice: c.Cycle().InvoiceFor(in.Date),
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
	r, err := q.GetPurchase(ctx, id)
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
	return Purchase{
		ID: r.ID, CardID: r.CardID, Kind: r.Kind, Date: on, Merchant: r.Merchant, Description: r.Description,
		CategoryID: idPtr(r.CategoryID), BuyerPersonID: idPtr(r.BuyerPersonID), TotalCents: r.TotalCents,
		Installments: int(r.InstallmentCount), FirstInvoice: first,
	}, nil
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
