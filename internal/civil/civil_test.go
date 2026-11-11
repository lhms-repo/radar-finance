package civil

import (
	"testing"
	"time"
)

// TestParseDate accepts ISO days and rejects impossible ones.
func TestParseDate(t *testing.T) {
	tests := []struct {
		in      string
		want    Date
		wantErr bool
	}{
		{in: "2026-11-11", want: Date{2026, time.November, 11}},
		{in: "2028-02-29", want: Date{2028, time.February, 29}},
		{in: "2026-02-29", wantErr: true},
		{in: "11/11/2026", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		got, err := ParseDate(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ParseDate(%q) = %v, %v", tt.in, got, err)
		}
		if err == nil && got.String() != tt.in {
			t.Errorf("String() = %q, want %q", got.String(), tt.in)
		}
	}
}

// TestMonthAdd crosses year boundaries in both directions.
func TestMonthAdd(t *testing.T) {
	tests := []struct {
		from Month
		n    int
		want string
	}{
		{Month{2026, time.November}, 1, "2026-12"},
		{Month{2026, time.December}, 1, "2027-01"},
		{Month{2026, time.January}, -1, "2025-12"},
		{Month{2026, time.March}, -14, "2025-01"},
		{Month{2026, time.March}, 22, "2028-01"},
	}
	for _, tt := range tests {
		if got := tt.from.Add(tt.n).String(); got != tt.want {
			t.Errorf("%v.Add(%d) = %s, want %s", tt.from, tt.n, got, tt.want)
		}
	}
}

// TestMonthDayClamps uses the last day when the month is shorter.
func TestMonthDayClamps(t *testing.T) {
	tests := []struct {
		m    Month
		day  int
		want string
	}{
		{Month{2026, time.February}, 31, "2026-02-28"},
		{Month{2028, time.February}, 30, "2028-02-29"},
		{Month{2026, time.April}, 31, "2026-04-30"},
		{Month{2026, time.May}, 10, "2026-05-10"},
	}
	for _, tt := range tests {
		if got := tt.m.Day(tt.day).String(); got != tt.want {
			t.Errorf("%v.Day(%d) = %s, want %s", tt.m, tt.day, got, tt.want)
		}
	}
	if (Month{2026, time.November}).Last().String() != "2026-11-30" {
		t.Error("Last of November")
	}
}

// TestToday reads the day in São Paulo, not in UTC.
func TestToday(t *testing.T) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Skip("no tzdata")
	}
	// 01:30 UTC on Nov 12 is still Nov 11 in São Paulo (UTC-3).
	utc := time.Date(2026, time.November, 12, 1, 30, 0, 0, time.UTC)
	if got := DateOf(utc.In(loc)).String(); got != "2026-11-11" {
		t.Fatalf("day in São Paulo = %s", got)
	}
}
