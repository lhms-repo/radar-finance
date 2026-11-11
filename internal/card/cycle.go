package card

import "github.com/italogfx/radar-finance/internal/civil"

// Cycle holds the closing and due days of a card, each clamped to short months.
type Cycle struct {
	ClosingDay int
	DueDay     int
}

// closingOffset is 1 when the invoice closes in the month before its due month.
func (c Cycle) closingOffset() int {
	if c.DueDay > c.ClosingDay {
		return 0
	}
	return 1
}

// Due returns the due date of the invoice; the invoice month is the due month (RN-01).
func (c Cycle) Due(invoice civil.Month) civil.Date {
	return invoice.Day(c.DueDay)
}

// Closing returns the closing date of the invoice, the last one before its due date.
func (c Cycle) Closing(invoice civil.Month) civil.Date {
	return invoice.Add(-c.closingOffset()).Day(c.ClosingDay)
}

// InvoiceFor returns the invoice a purchase belongs to: the first one closing after the purchase day,
// so a purchase on the closing day goes to the next invoice (RN-01, RN-02).
func (c Cycle) InvoiceFor(purchase civil.Date) civil.Month {
	inv := purchase.MonthOf().Add(c.closingOffset())
	if purchase.Before(c.Closing(inv)) {
		return inv
	}
	return inv.Add(1)
}

// Split divides a positive total into n installments; the leftover cents go to the first one (RN-03).
func Split(total int64, n int) []int64 {
	base := total / int64(n)
	out := make([]int64, n)
	for i := range out {
		out[i] = base
	}
	out[0] += total - base*int64(n)
	return out
}
