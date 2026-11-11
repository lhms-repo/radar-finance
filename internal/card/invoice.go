package card

import (
	"context"
	"fmt"

	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/ledger"
)

// KindInvoicePayment is the entry kind of an invoice payment, which is not spending (RN-07).
const KindInvoicePayment ledger.Kind = "invoice_payment"

// Status is the state of an invoice on a given day.
type Status string

// Invoice states: open until the closing day, closed until the due date, then overdue, or paid.
const (
	StatusOpen    Status = "open"
	StatusClosed  Status = "closed"
	StatusOverdue Status = "overdue"
	StatusPaid    Status = "paid"
)

// ItemKind tells where an invoice line comes from.
type ItemKind string

// Invoice line kinds.
const (
	ItemPurchase   ItemKind = "purchase"
	ItemCredit     ItemKind = "credit"
	ItemRecurrence ItemKind = "recurrence"
)

// Item is one line of an invoice; amounts are signed, credits negative.
type Item struct {
	Kind               ItemKind
	PurchaseID         *int64
	RecurrenceID       *int64
	Date               *civil.Date
	Merchant           string
	Description        string
	CategoryID         *int64
	CategoryName       string
	ParentCategoryName string
	CategoryIcon       string
	Number             int
	Count              int
	AmountCents        int64
	PurchaseTotalCents int64
	BuyerPersonID      *int64
	BuyerName          string
	CreatedByName      string
	// ThirdPartyCents is the part of this line owed by other people (RN-05).
	ThirdPartyCents int64
	Shares          []ItemShare
}

// PaymentInfo is the recorded payment of an invoice.
type PaymentInfo struct {
	EntryID     int64
	AccountID   int64
	AccountName string
	PaidOn      civil.Date
	AmountCents int64
}

// Invoice is the bill of one card in one month (RF-19).
type Invoice struct {
	CardID    int64
	Month     civil.Month
	ClosingOn civil.Date
	DueOn     civil.Date
	Status    Status
	// TotalCents is the real total of the invoice.
	TotalCents int64
	// ThirdPartyCents is owed by other people; the net part is what the couple pays (RN-05).
	ThirdPartyCents int64
	ReceivedCents   int64
	PendingCents    int64
	NetCents        int64
	Items           []Item
	Payment         *PaymentInfo
	// PaymentOutdated is true when the total changed after the payment was recorded.
	PaymentOutdated bool
}

// Summary is the total of an invoice for the forecast (RF-23).
type Summary struct {
	Month      civil.Month
	DueOn      civil.Date
	Status     Status
	TotalCents int64
}

// Payment pays an invoice in full from an account (RF-21).
type Payment struct {
	CardID    int64
	Month     civil.Month
	AccountID int64
	Date      civil.Date
}

// Invoice returns the invoice of the card in month m, with every line (RF-19).
func (s *Service) Invoice(ctx context.Context, cardID int64, m civil.Month) (Invoice, error) {
	return s.invoiceTx(ctx, store.New(s.conn), cardID, m)
}

// invoiceTx builds the invoice with the given queries, inside or outside a transaction.
func (s *Service) invoiceTx(ctx context.Context, q *store.Queries, cardID int64, m civil.Month) (Invoice, error) {
	c, err := getCard(ctx, q, cardID)
	if err != nil {
		return Invoice{}, err
	}
	inv := Invoice{CardID: c.ID, Month: m, ClosingOn: c.Cycle().Closing(m), DueOn: c.Cycle().Due(m), Items: []Item{}}
	rows, err := q.ListInvoiceInstallments(ctx, store.ListInvoiceInstallmentsParams{CardID: c.ID, InvoiceMonth: m.String()})
	if err != nil {
		return Invoice{}, fmt.Errorf("list installments: %w", err)
	}
	for _, r := range rows {
		on, err := civil.ParseDate(r.PurchasedOn)
		if err != nil {
			return Invoice{}, err
		}
		kind := ItemPurchase
		if r.Kind == kindCredit {
			kind = ItemCredit
		}
		pid := r.PurchaseID
		inv.Items = append(inv.Items, Item{
			Kind: kind, PurchaseID: &pid, Date: &on, Merchant: r.Merchant, Description: r.Description, CategoryID: idPtr(r.CategoryID),
			CategoryName: r.CategoryName, ParentCategoryName: r.ParentCategoryName, CategoryIcon: r.CategoryIcon,
			Number: int(r.Number), Count: int(r.InstallmentCount), AmountCents: r.AmountCents, PurchaseTotalCents: r.TotalCents,
			BuyerPersonID: idPtr(r.BuyerPersonID), BuyerName: r.BuyerName, CreatedByName: r.CreatedByName,
		})
		inv.TotalCents += r.AmountCents
	}
	if err := s.addShares(ctx, q, &inv); err != nil {
		return Invoice{}, err
	}
	recs, err := s.recurrences(ctx, q, c)
	if err != nil {
		return Invoice{}, err
	}
	for _, r := range recs {
		amount := r.priceIn(m)
		if amount == 0 {
			continue
		}
		rid, cid := r.ID, r.CategoryID
		inv.Items = append(inv.Items, Item{
			Kind: ItemRecurrence, RecurrenceID: &rid, Merchant: r.Description, CategoryID: &cid, CategoryName: r.CategoryName,
			ParentCategoryName: r.ParentCategoryName, CategoryIcon: r.CategoryIcon, AmountCents: amount,
		})
		inv.TotalCents += amount
	}
	payments, err := paymentsByMonth(ctx, q, c.ID)
	if err != nil {
		return Invoice{}, err
	}
	if p, ok := payments[m]; ok {
		inv.Payment = &p
		inv.PaymentOutdated = p.AmountCents != inv.TotalCents
	}
	inv.Status = s.status(c, m, inv.Payment != nil, inv.TotalCents)
	inv.NetCents = inv.TotalCents - inv.ThirdPartyCents
	return inv, nil
}

// addShares puts the shares of other people on the lines and in the totals: owed, received and pending (RN-05, RN-06).
func (s *Service) addShares(ctx context.Context, q *store.Queries, inv *Invoice) error {
	rows, err := q.ListInvoiceSplitInstallments(ctx, store.ListInvoiceSplitInstallmentsParams{CardID: inv.CardID, InvoiceMonth: inv.Month.String()})
	if err != nil {
		return fmt.Errorf("list shares: %w", err)
	}
	for _, r := range rows {
		for i := range inv.Items {
			it := &inv.Items[i]
			if it.PurchaseID == nil || *it.PurchaseID != r.PurchaseID {
				continue
			}
			it.Shares = append(it.Shares, ItemShare{PartyName: r.PartyName, AmountCents: r.AmountCents, Received: r.EntryID.Valid})
			it.ThirdPartyCents += r.AmountCents
		}
		inv.ThirdPartyCents += r.AmountCents
		if r.EntryID.Valid {
			inv.ReceivedCents += r.AmountCents
		}
	}
	inv.PendingCents = inv.ThirdPartyCents - inv.ReceivedCents
	return nil
}

// status tells the state of invoice m today; the invoice closes on its closing day (RN-02).
// An invoice with nothing to pay is never overdue.
func (s *Service) status(c Card, m civil.Month, paid bool, total int64) Status {
	today := s.today()
	switch {
	case paid:
		return StatusPaid
	case today.Before(c.Cycle().Closing(m)):
		return StatusOpen
	case total <= 0 || !c.Cycle().Due(m).Before(today):
		return StatusClosed
	default:
		return StatusOverdue
	}
}

// Pay records the full payment of a closed invoice as an entry in the account (RF-21, RN-07).
func (s *Service) Pay(ctx context.Context, userID int64, p Payment) (Invoice, error) {
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		inv, err := s.invoiceTx(ctx, q, p.CardID, p.Month)
		if err != nil {
			return err
		}
		switch {
		case inv.Payment != nil:
			return ErrAlreadyPaid
		case inv.Status == StatusOpen:
			return ErrInvoiceOpen
		case inv.TotalCents <= 0:
			return ErrNothingToPay
		}
		if p.Date.IsZero() {
			return ledger.ErrDateRequired
		}
		if err := ledger.CheckActiveAccount(ctx, q, p.AccountID); err != nil {
			return err
		}
		c, err := getCard(ctx, q, p.CardID)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		e, err := q.CreateEntry(ctx, store.CreateEntryParams{
			AccountID: p.AccountID, Kind: string(KindInvoicePayment), AmountCents: -inv.TotalCents, OccurredOn: p.Date.String(),
			Confirmed: true, Description: fmt.Sprintf("%s, fatura %02d/%d", c.Name, p.Month.Month, p.Month.Year),
			CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create payment entry: %w", err)
		}
		return q.CreateInvoicePayment(ctx, store.CreateInvoicePaymentParams{
			CardID: p.CardID, InvoiceMonth: p.Month.String(), EntryID: e.ID, AmountCents: inv.TotalCents, CreatedBy: userID, CreatedAt: now,
		})
	})
	if err != nil {
		return Invoice{}, err
	}
	return s.Invoice(ctx, p.CardID, p.Month)
}

// Forecast returns the totals of the next n invoices from the current one (RF-23).
func (s *Service) Forecast(ctx context.Context, cardID int64, n int) ([]Summary, error) {
	q := store.New(s.conn)
	c, err := getCard(ctx, q, cardID)
	if err != nil {
		return nil, err
	}
	totals, err := s.monthTotals(ctx, q, c)
	if err != nil {
		return nil, err
	}
	payments, err := paymentsByMonth(ctx, q, c.ID)
	if err != nil {
		return nil, err
	}
	start := c.Cycle().InvoiceFor(s.today())
	out := make([]Summary, 0, n)
	for i := range n {
		m := start.Add(i)
		_, paid := payments[m]
		out = append(out, Summary{Month: m, DueOn: c.Cycle().Due(m), Status: s.status(c, m, paid, totals[m]), TotalCents: totals[m]})
	}
	return out, nil
}

// limitUsed sums what is left to pay on every invoice, future installments included (RF-20, RN-17).
// Recurrences only count up to the current invoice, since later charges are not made yet.
func (s *Service) limitUsed(ctx context.Context, q *store.Queries, c Card) (int64, error) {
	totals, err := s.monthTotals(ctx, q, c)
	if err != nil {
		return 0, err
	}
	payments, err := paymentsByMonth(ctx, q, c.ID)
	if err != nil {
		return 0, err
	}
	current := c.Cycle().InvoiceFor(s.today())
	recs, err := s.recurrences(ctx, q, c)
	if err != nil {
		return 0, err
	}
	var used int64
	for m, total := range totals {
		// Future recurrence charges are in the totals for the forecast; take them out here.
		if monthIndex(m) > monthIndex(current) {
			for _, r := range recs {
				total -= r.priceIn(m)
			}
		}
		if left := total - payments[m].AmountCents; left > 0 {
			used += left
		}
	}
	return used, nil
}

// monthTotals returns the invoice total of every month with installments, plus recurrence charges
// from their start up to their end or one year after the current invoice.
func (s *Service) monthTotals(ctx context.Context, q *store.Queries, c Card) (map[civil.Month]int64, error) {
	rows, err := q.SumInstallmentsByMonth(ctx, c.ID)
	if err != nil {
		return nil, fmt.Errorf("sum installments: %w", err)
	}
	totals := map[civil.Month]int64{}
	for _, r := range rows {
		m, err := civil.ParseMonth(r.InvoiceMonth)
		if err != nil {
			return nil, err
		}
		totals[m] += r.TotalCents
	}
	recs, err := s.recurrences(ctx, q, c)
	if err != nil {
		return nil, err
	}
	horizon := c.Cycle().InvoiceFor(s.today()).Add(12)
	for _, r := range recs {
		last := horizon
		if r.EndMonth != nil && monthIndex(*r.EndMonth) < monthIndex(last) {
			last = *r.EndMonth
		}
		for m := r.StartMonth; monthIndex(m) <= monthIndex(last); m = m.Add(1) {
			totals[m] += r.priceIn(m)
		}
	}
	return totals, nil
}

// paymentsByMonth returns the recorded payments of the card keyed by invoice month.
func paymentsByMonth(ctx context.Context, q *store.Queries, cardID int64) (map[civil.Month]PaymentInfo, error) {
	rows, err := q.ListCardPayments(ctx, cardID)
	if err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	out := make(map[civil.Month]PaymentInfo, len(rows))
	for _, r := range rows {
		m, err := civil.ParseMonth(r.InvoiceMonth)
		if err != nil {
			return nil, err
		}
		on, err := civil.ParseDate(r.OccurredOn)
		if err != nil {
			return nil, err
		}
		out[m] = PaymentInfo{EntryID: r.EntryID, AccountID: r.AccountID, AccountName: r.AccountName, PaidOn: on, AmountCents: r.AmountCents}
	}
	return out, nil
}

// ClosedDebt sums what is left to pay on every closed or overdue invoice of every card, for the net worth (RF-62).
// Open invoices and future installments are not debt yet; they are commitments (RF-60).
func (s *Service) ClosedDebt(ctx context.Context) (int64, error) {
	q := store.New(s.conn)
	rows, err := q.ListCards(ctx)
	if err != nil {
		return 0, fmt.Errorf("list cards: %w", err)
	}
	var debt int64
	for _, r := range rows {
		c := Card{ID: r.ID, ClosingDay: int(r.ClosingDay), DueDay: int(r.DueDay)}
		totals, err := s.monthTotals(ctx, q, c)
		if err != nil {
			return 0, err
		}
		payments, err := paymentsByMonth(ctx, q, c.ID)
		if err != nil {
			return 0, err
		}
		for m, total := range totals {
			_, paid := payments[m]
			if st := s.status(c, m, paid, total); st != StatusClosed && st != StatusOverdue {
				continue
			}
			debt += max(0, total-payments[m].AmountCents)
		}
	}
	return debt, nil
}
