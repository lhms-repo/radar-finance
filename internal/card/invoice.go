package card

import (
	"context"
	"fmt"

	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/scope"
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
	// GroupName and GroupID are the space the line belongs to, on the full invoice of the owner (RF-83).
	GroupName string
	GroupID   int64
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
	// Full is the whole invoice, every space included, which only the owner reads in her personal space (RF-83).
	Full bool
	// CanPay tells the owner may pay it now: full, closed, unpaid and with something to pay.
	CanPay bool
}

// view is how a space reads a card: only its own lines, or every line for the owner in her personal space (RF-83).
type view struct {
	groupID int64
	full    bool
}

// fullFlag turns the full view into the integer the queries compare.
func (v view) fullFlag() int64 {
	if v.full {
		return 1
	}
	return 0
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

// Invoice returns the invoice of the card in month m as the active space reads it: its own lines, or the whole
// invoice for the owner in her personal space (RF-19, RF-83).
func (s *Service) Invoice(ctx context.Context, cardID int64, m civil.Month) (Invoice, error) {
	return s.invoiceTx(ctx, store.New(s.conn), cardID, m, false)
}

// InvoicePart returns only the lines of the active space, even for the owner, for the forecast of the month (RF-58, RF-83).
func (s *Service) InvoicePart(ctx context.Context, cardID int64, m civil.Month) (Invoice, error) {
	return s.invoiceTx(ctx, store.New(s.conn), cardID, m, true)
}

// viewOf tells how the active space reads card c: in full only in the personal space of its owner (RF-83).
func (s *Service) viewOf(ctx context.Context, q *store.Queries, c Card) (view, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return view{}, err
	}
	g, err := q.GetGroup(ctx, groupID)
	if err != nil {
		return view{}, fmt.Errorf("active group: %w", err)
	}
	full := g.Kind == "personal" && g.OwnerPersonID.Valid && g.OwnerPersonID.Int64 == c.PersonID
	return view{groupID: groupID, full: full}, nil
}

// invoiceTx builds the invoice with the given queries, inside or outside a transaction; part keeps it to the
// lines of the active space.
func (s *Service) invoiceTx(ctx context.Context, q *store.Queries, cardID int64, m civil.Month, part bool) (Invoice, error) {
	c, err := getCard(ctx, q, cardID)
	if err != nil {
		return Invoice{}, err
	}
	v, err := s.viewOf(ctx, q, c)
	if err != nil {
		return Invoice{}, err
	}
	if part {
		v.full = false
	}
	inv := Invoice{CardID: c.ID, Month: m, ClosingOn: c.Cycle().Closing(m), DueOn: c.Cycle().Due(m), Items: []Item{}, Full: v.full}
	rows, err := q.ListInvoiceInstallments(ctx, store.ListInvoiceInstallmentsParams{Full: v.fullFlag(), GroupID: v.groupID, CardID: c.ID, InvoiceMonth: m.String()})
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
			BuyerPersonID: idPtr(r.BuyerPersonID), BuyerName: r.BuyerName, CreatedByName: r.CreatedByName, GroupName: r.GroupName, GroupID: r.GroupID,
		})
		inv.TotalCents += r.AmountCents
	}
	if err := s.addShares(ctx, q, &inv, v); err != nil {
		return Invoice{}, err
	}
	recs, err := s.recurrencesIn(ctx, q, c, v)
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
			ParentCategoryName: r.ParentCategoryName, CategoryIcon: r.CategoryIcon, AmountCents: amount, GroupName: r.GroupName, GroupID: r.SpaceID,
		})
		inv.TotalCents += amount
	}
	payments, err := paymentsByMonth(ctx, q, c.ID)
	if err != nil {
		return Invoice{}, err
	}
	p, paid := payments[m]
	// A space reads only that the owner paid; the account and the amount stay on her full invoice.
	if paid && v.full {
		inv.Payment = &p
		inv.PaymentOutdated = p.AmountCents != inv.TotalCents
	}
	inv.Status = s.status(c, m, paid, inv.TotalCents)
	inv.CanPay = v.full && !paid && inv.TotalCents > 0 && (inv.Status == StatusClosed || inv.Status == StatusOverdue)
	inv.NetCents = inv.TotalCents - inv.ThirdPartyCents
	return inv, nil
}

// addShares puts the shares of other people on the lines and in the totals: owed, received and pending (RN-05, RN-06).
func (s *Service) addShares(ctx context.Context, q *store.Queries, inv *Invoice, v view) error {
	rows, err := q.ListInvoiceSplitInstallments(ctx, store.ListInvoiceSplitInstallmentsParams{Full: v.fullFlag(), GroupID: v.groupID, CardID: inv.CardID, InvoiceMonth: inv.Month.String()})
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

// Pay records the payment of the whole closed invoice as an entry in an account of the owner, made in her
// personal space (RF-21, RF-83, RN-07).
func (s *Service) Pay(ctx context.Context, userID int64, p Payment) (Invoice, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return Invoice{}, err
	}
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		inv, err := s.invoiceTx(ctx, q, p.CardID, p.Month, false)
		if err != nil {
			return err
		}
		switch {
		case !inv.Full:
			return ErrPayInPersonal
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
			CreatedBy: userID, CreatedAt: now, UpdatedAt: now, GroupID: groupID,
		})
		if err != nil {
			return fmt.Errorf("create payment entry: %w", err)
		}
		return q.CreateInvoicePayment(ctx, store.CreateInvoicePaymentParams{
			GroupID: groupID, CardID: p.CardID, InvoiceMonth: p.Month.String(), EntryID: e.ID, AmountCents: inv.TotalCents, CreatedBy: userID, CreatedAt: now,
		})
	})
	if err != nil {
		return Invoice{}, err
	}
	return s.Invoice(ctx, p.CardID, p.Month)
}

// Forecast returns the totals of the next n invoices from the current one, as the active space reads them (RF-23).
func (s *Service) Forecast(ctx context.Context, cardID int64, n int) ([]Summary, error) {
	q := store.New(s.conn)
	c, err := getCard(ctx, q, cardID)
	if err != nil {
		return nil, err
	}
	v, err := s.viewOf(ctx, q, c)
	if err != nil {
		return nil, err
	}
	totals, err := s.monthTotals(ctx, q, c, v)
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
// Recurrences only count up to the current invoice, since later charges are not made yet. The limit is of the
// whole card, so every space counts (RF-83).
func (s *Service) limitUsed(ctx context.Context, q *store.Queries, c Card) (int64, error) {
	all := view{full: true}
	totals, err := s.monthTotals(ctx, q, c, all)
	if err != nil {
		return 0, err
	}
	payments, err := paymentsByMonth(ctx, q, c.ID)
	if err != nil {
		return 0, err
	}
	current := c.Cycle().InvoiceFor(s.today())
	recs, err := s.recurrencesIn(ctx, q, c, all)
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
// from their start up to their end or one year after the current invoice, in the lines view v reads.
func (s *Service) monthTotals(ctx context.Context, q *store.Queries, c Card, v view) (map[civil.Month]int64, error) {
	rows, err := q.SumInstallmentsByMonth(ctx, store.SumInstallmentsByMonthParams{Full: v.fullFlag(), GroupID: v.groupID, CardID: c.ID})
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
	recs, err := s.recurrencesIn(ctx, q, c, v)
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

// paymentsByMonth returns the recorded payments of the card keyed by invoice month; the owner pays the whole
// invoice, so they are the same in every space (RF-83).
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
// Open invoices and future installments are not debt yet; they are commitments (RF-60). Each space counts the
// lines it reads, so the owner sees in her personal space the whole debt of her cards (RF-83).
func (s *Service) ClosedDebt(ctx context.Context) (int64, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return 0, err
	}
	q := store.New(s.conn)
	rows, err := q.ListCards(ctx, groupID)
	if err != nil {
		return 0, fmt.Errorf("list cards: %w", err)
	}
	var debt int64
	for _, r := range rows {
		c := Card{ID: r.ID, PersonID: r.PersonID, ClosingDay: int(r.ClosingDay), DueDay: int(r.DueDay)}
		v, err := s.viewOf(ctx, q, c)
		if err != nil {
			return 0, err
		}
		totals, err := s.monthTotals(ctx, q, c, v)
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
			debt += max(0, total)
		}
	}
	return debt, nil
}
