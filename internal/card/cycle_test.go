package card

import (
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/civil"
)

// d builds a civil date.
func d(y int, m time.Month, day int) civil.Date { return civil.Date{Year: y, Month: m, Day: day} }

// TestInvoiceFor assigns purchases to the invoice of the due month (RN-01, RN-02).
func TestInvoiceFor(t *testing.T) {
	tests := []struct {
		name     string
		cycle    Cycle
		purchase civil.Date
		want     string
	}{
		// Closes on the 3rd, due on the 10th of the same month.
		{name: "before closing, same month", cycle: Cycle{3, 10}, purchase: d(2026, time.November, 2), want: "2026-11"},
		{name: "on the closing day goes to the next", cycle: Cycle{3, 10}, purchase: d(2026, time.November, 3), want: "2026-12"},
		{name: "after closing goes to the next", cycle: Cycle{3, 10}, purchase: d(2026, time.November, 20), want: "2026-12"},
		{name: "December after closing goes to January", cycle: Cycle{3, 10}, purchase: d(2026, time.December, 15), want: "2027-01"},
		// Closes on the 28th, due on the 5th of the next month: the owner's example.
		{name: "day before closing, due next month", cycle: Cycle{28, 5}, purchase: d(2026, time.November, 27), want: "2026-12"},
		{name: "closing day, two months ahead", cycle: Cycle{28, 5}, purchase: d(2026, time.November, 28), want: "2027-01"},
		{name: "start of the month", cycle: Cycle{28, 5}, purchase: d(2026, time.November, 1), want: "2026-12"},
		// Closing day 31 clamps to the last day in short months.
		{name: "February closes on the 28th", cycle: Cycle{31, 8}, purchase: d(2027, time.February, 27), want: "2027-03"},
		{name: "February 28 is the closing day", cycle: Cycle{31, 8}, purchase: d(2027, time.February, 28), want: "2027-04"},
		{name: "April 30 is the closing day", cycle: Cycle{31, 8}, purchase: d(2026, time.April, 30), want: "2026-06"},
		{name: "March 30 before the 31st closing", cycle: Cycle{31, 8}, purchase: d(2026, time.March, 30), want: "2026-04"},
		// Same day for closing and due: the due date falls in the next month.
		{name: "closing equals due day", cycle: Cycle{10, 10}, purchase: d(2026, time.November, 9), want: "2026-12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cycle.InvoiceFor(tt.purchase).String(); got != tt.want {
				t.Fatalf("InvoiceFor(%v) = %s, want %s", tt.purchase, got, tt.want)
			}
		})
	}
}

// TestClosingAndDue returns the dates of an invoice, the closing always before the due date.
func TestClosingAndDue(t *testing.T) {
	tests := []struct {
		cycle        Cycle
		invoice      civil.Month
		closing, due string
	}{
		{cycle: Cycle{3, 10}, invoice: civil.Month{Year: 2026, Month: time.November}, closing: "2026-11-03", due: "2026-11-10"},
		{cycle: Cycle{28, 5}, invoice: civil.Month{Year: 2026, Month: time.December}, closing: "2026-11-28", due: "2026-12-05"},
		{cycle: Cycle{28, 5}, invoice: civil.Month{Year: 2027, Month: time.January}, closing: "2026-12-28", due: "2027-01-05"},
		{cycle: Cycle{31, 8}, invoice: civil.Month{Year: 2027, Month: time.March}, closing: "2027-02-28", due: "2027-03-08"},
		{cycle: Cycle{25, 31}, invoice: civil.Month{Year: 2027, Month: time.February}, closing: "2027-02-25", due: "2027-02-28"},
	}
	for _, tt := range tests {
		if got := tt.cycle.Closing(tt.invoice).String(); got != tt.closing {
			t.Errorf("Closing(%v) = %s, want %s", tt.invoice, got, tt.closing)
		}
		if got := tt.cycle.Due(tt.invoice).String(); got != tt.due {
			t.Errorf("Due(%v) = %s, want %s", tt.invoice, got, tt.due)
		}
	}
}

// TestInvoiceForIsConsistent checks, for every day of two years, that the purchase falls before the closing of its invoice and on or after the previous closing.
func TestInvoiceForIsConsistent(t *testing.T) {
	for _, c := range []Cycle{{3, 10}, {28, 5}, {31, 8}, {1, 9}, {15, 15}, {30, 1}} {
		for day := d(2026, time.January, 1); day.Before(d(2028, time.January, 1)); day = day.AddDays(1) {
			inv := c.InvoiceFor(day)
			if !day.Before(c.Closing(inv)) {
				t.Fatalf("%v: %v is not before closing %v of %v", c, day, c.Closing(inv), inv)
			}
			if day.Before(c.Closing(inv.Add(-1))) {
				t.Fatalf("%v: %v belongs to an earlier invoice than %v", c, day, inv)
			}
			if !c.Closing(inv).Before(c.Due(inv)) {
				t.Fatalf("%v: closing %v not before due %v", c, c.Closing(inv), c.Due(inv))
			}
		}
	}
}

// TestSplit divides a total into installments with the leftover cents on the first one (RN-03).
func TestSplit(t *testing.T) {
	tests := []struct {
		total int64
		n     int
		want  []int64
	}{
		{total: 100_000, n: 1, want: []int64{100_000}},
		{total: 100_000, n: 3, want: []int64{33_334, 33_333, 33_333}},
		{total: 100, n: 3, want: []int64{34, 33, 33}},
		{total: 299_990, n: 10, want: []int64{29_999, 29_999, 29_999, 29_999, 29_999, 29_999, 29_999, 29_999, 29_999, 29_999}},
		{total: 100_001, n: 12, want: []int64{8_338, 8_333, 8_333, 8_333, 8_333, 8_333, 8_333, 8_333, 8_333, 8_333, 8_333, 8_333}},
		{total: 5, n: 5, want: []int64{1, 1, 1, 1, 1}},
	}
	for _, tt := range tests {
		got := Split(tt.total, tt.n)
		var sum int64
		for _, v := range got {
			sum += v
		}
		if len(got) != len(tt.want) || sum != tt.total {
			t.Fatalf("Split(%d, %d) = %v, sum %d", tt.total, tt.n, got, sum)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Fatalf("Split(%d, %d) = %v, want %v", tt.total, tt.n, got, tt.want)
			}
		}
	}
}
