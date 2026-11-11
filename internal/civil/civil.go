// Package civil holds calendar dates and months without time of day, the unit
// of every business date in Radar, always read in America/Sao_Paulo.
package civil

import (
	"fmt"
	"time"
)

// Date is a calendar day.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// Month is a calendar month.
type Month struct {
	Year  int
	Month time.Month
}

// dateLayout is the ISO form used in the database and the API.
const dateLayout = "2006-01-02"

// ParseDate reads an ISO date such as "2026-11-11", rejecting impossible days.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return Date{}, fmt.Errorf("invalid date %q: %w", s, err)
	}
	return DateOf(t), nil
}

// DateOf returns the calendar day of t in its own location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{Year: y, Month: m, Day: d}
}

// Today returns the current day in loc.
func Today(loc *time.Location) Date {
	return DateOf(time.Now().In(loc))
}

// NewDate builds a date, clamping the day to the last day of the month.
func NewDate(year int, month time.Month, day int) Date {
	if last := DaysIn(year, month); day > last {
		day = last
	}
	if day < 1 {
		day = 1
	}
	return Date{Year: year, Month: month, Day: day}
}

// DaysIn returns how many days the month has.
func DaysIn(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// String returns the ISO form.
func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// Time returns midnight of the day in UTC, for arithmetic and API types.
func (d Date) Time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

// IsZero reports whether the date was never set.
func (d Date) IsZero() bool { return d == Date{} }

// Before reports whether d comes before o.
func (d Date) Before(o Date) bool { return d.Time().Before(o.Time()) }

// AddDays returns the date n days later.
func (d Date) AddDays(n int) Date { return DateOf(d.Time().AddDate(0, 0, n)) }

// MonthOf returns the month of the date.
func (d Date) MonthOf() Month { return Month{Year: d.Year, Month: d.Month} }

// ParseMonth reads a month such as "2026-11".
func ParseMonth(s string) (Month, error) {
	t, err := time.Parse("2006-01", s)
	if err != nil {
		return Month{}, fmt.Errorf("invalid month %q: %w", s, err)
	}
	return Month{Year: t.Year(), Month: t.Month()}, nil
}

// String returns the form "2026-11".
func (m Month) String() string { return fmt.Sprintf("%04d-%02d", m.Year, m.Month) }

// First returns the first day of the month.
func (m Month) First() Date { return Date{Year: m.Year, Month: m.Month, Day: 1} }

// Last returns the last day of the month.
func (m Month) Last() Date { return Date{Year: m.Year, Month: m.Month, Day: DaysIn(m.Year, m.Month)} }

// Add returns the month n months later, or earlier when n is negative.
func (m Month) Add(n int) Month {
	i := m.Year*12 + int(m.Month) - 1 + n
	return Month{Year: i / 12, Month: time.Month(i%12 + 1)}
}

// Day returns the given day of the month, clamped to its last day.
func (m Month) Day(day int) Date { return NewDate(m.Year, m.Month, day) }
