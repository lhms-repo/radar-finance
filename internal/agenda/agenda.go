// Package agenda joins everything due in a month, fixed bills, financing
// installments and card invoices, for the due date calendar (RF-38, RF-40).
package agenda

import (
	"context"
	"sort"
	"strings"

	"github.com/italogfx/radar-finance/internal/bill"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/financing"
)

// Kind tells where an agenda item comes from.
type Kind string

// Agenda item kinds.
const (
	KindBill      Kind = "bill"
	KindFinancing Kind = "financing"
	KindInvoice   Kind = "invoice"
)

// Item is one thing to pay in the month.
type Item struct {
	Kind        Kind
	RefID       int64
	Name        string
	Icon        string
	AccountID   int64
	AccountName string
	DueOn       civil.Date
	AmountCents int64
	Paid        bool
	EntryID     *int64
	PaidOn      *civil.Date
	// Bill only: expected and real amounts (RN-14).
	ExpectedCents int64
	ActualCents   *int64
	// Financing only: installment number, total and the balance suggested when paying (RN-16).
	Number                int
	Total                 int
	SuggestedBalanceCents int64
	// Invoice only: its state.
	InvoiceStatus card.Status
}

// Day counts the items due on one day of the month, for the calendar chips.
type Day struct {
	Day       int
	Count     int
	PaidCount int
}

// Month is the agenda of a month with its totals.
type Month struct {
	Month      civil.Month
	Items      []Item
	Days       []Day
	TotalCents int64
	PaidCents  int64
	Count      int
	PaidCount  int
}

// Service builds the agenda from the bill, financing and card services.
type Service struct {
	bills      *bill.Service
	financings *financing.Service
	cards      *card.Service
}

// NewService creates the agenda service.
func NewService(bills *bill.Service, financings *financing.Service, cards *card.Service) *Service {
	return &Service{bills: bills, financings: financings, cards: cards}
}

// Month returns everything due in the month ordered by due date (RF-38).
func (s *Service) Month(ctx context.Context, m civil.Month) (Month, error) {
	var items []Item
	bills, err := s.bills.Month(ctx, m)
	if err != nil {
		return Month{}, err
	}
	for _, b := range bills.Bills {
		items = append(items, Item{
			Kind: KindBill, RefID: b.BillID, Name: b.Name, Icon: b.CategoryIcon, AccountID: b.AccountID, AccountName: b.AccountName,
			DueOn: b.DueOn, AmountCents: b.AmountCents, Paid: b.Paid, EntryID: b.EntryID, PaidOn: b.PaidOn,
			ExpectedCents: b.ExpectedCents, ActualCents: b.ActualCents,
		})
	}
	installments, err := s.financings.Month(ctx, m)
	if err != nil {
		return Month{}, err
	}
	for _, f := range installments {
		items = append(items, Item{
			Kind: KindFinancing, RefID: f.FinancingID, Name: f.Name, Icon: f.CategoryIcon, AccountID: f.AccountID, AccountName: f.AccountName,
			DueOn: f.DueOn, AmountCents: f.AmountCents, Paid: f.Paid, EntryID: f.EntryID, PaidOn: f.PaidOn,
			Number: f.Number, Total: f.Total, SuggestedBalanceCents: f.SuggestedBalanceCents,
		})
	}
	cards, err := s.cards.ListCards(ctx)
	if err != nil {
		return Month{}, err
	}
	for _, c := range cards {
		inv, err := s.cards.Invoice(ctx, c.ID, m)
		if err != nil {
			return Month{}, err
		}
		if inv.TotalCents <= 0 && inv.Payment == nil {
			continue
		}
		it := Item{
			Kind: KindInvoice, RefID: c.ID, Name: c.Name, Icon: "credit-card", DueOn: inv.DueOn, AmountCents: inv.TotalCents,
			Paid: inv.Payment != nil, InvoiceStatus: inv.Status,
		}
		if p := inv.Payment; p != nil {
			on, entry := p.PaidOn, p.EntryID
			it.PaidOn, it.EntryID, it.AccountID, it.AccountName = &on, &entry, p.AccountID, p.AccountName
		}
		items = append(items, it)
	}
	// Pending items first and paid ones at the end, as the paid item fades to the end of the list (section 8.6).
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Paid != b.Paid {
			return !a.Paid
		}
		if a.DueOn != b.DueOn {
			return a.DueOn.Before(b.DueOn)
		}
		return fold(a.Name) < fold(b.Name)
	})

	out := Month{Month: m, Items: items, Days: []Day{}}
	if out.Items == nil {
		out.Items = []Item{}
	}
	days := map[int]*Day{}
	for _, it := range items {
		out.TotalCents += it.AmountCents
		out.Count++
		d, ok := days[it.DueOn.Day]
		if !ok {
			d = &Day{Day: it.DueOn.Day}
			days[it.DueOn.Day] = d
		}
		d.Count++
		if it.Paid {
			out.PaidCents += it.AmountCents
			out.PaidCount++
			d.PaidCount++
		}
	}
	for _, d := range days {
		out.Days = append(out.Days, *d)
	}
	sort.Slice(out.Days, func(i, j int) bool { return out.Days[i].Day < out.Days[j].Day })
	return out, nil
}

// accents maps accented letters of Portuguese to their plain form, for sorting names.
var accents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "õ", "o", "ú", "u", "ç", "c",
)

// fold returns the name in lower case without accents, so "Água" sorts with the a's.
func fold(name string) string { return accents.Replace(strings.ToLower(name)) }
