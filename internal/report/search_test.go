package report

import (
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/civil"
)

// TestSearch filters every kind of entry by period, category, account, card, person and text (RF-63).
func TestSearch(t *testing.T) {
	w := newWorld(t)
	s := newReport(w)
	oct := civil.Month{Year: 2026, Month: time.October}
	tests := []struct {
		name string
		in   Filter
		want int
	}{
		{name: "whole November", in: Filter{From: nov.First(), To: nov.Last()}, want: 10},
		{name: "top level category takes its subcategories", in: Filter{From: nov.First(), To: nov.Last(), CategoryID: w.cat["Alimentação"]}, want: 2},
		{name: "subcategory", in: Filter{From: nov.First(), To: nov.Last(), CategoryID: w.cat["Energia"]}, want: 1},
		{name: "account", in: Filter{From: nov.First(), To: nov.Last(), AccountID: w.inter}, want: 1},
		{name: "card in October", in: Filter{From: oct.First(), To: oct.Last(), CardID: w.cardID}, want: 3},
		{name: "person who made it", in: Filter{From: nov.First(), To: nov.Last(), PersonID: w.persons[1]}, want: 0},
		{name: "text ignoring case", in: Filter{From: nov.First(), To: nov.Last(), Text: "ENER"}, want: 1},
		// % and _ are plain characters typed by the user, not wildcards.
		{name: "percent sign is literal", in: Filter{From: nov.First(), To: nov.Last(), Text: "%"}, want: 0},
		{name: "underscore is literal", in: Filter{From: nov.First(), To: nov.Last(), Text: "_"}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Search(groupCtx, tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.want {
				t.Fatalf("lines = %d, want %d: %+v", len(got), tt.want, got)
			}
		})
	}
	got, _ := s.Search(groupCtx, Filter{From: oct.First(), To: oct.Last(), CardID: w.cardID})
	if got[0].Source != SourceCard || got[0].CardName != "Nubank" || got[0].Description != "Crédito de anuidade" || got[0].AmountCents != 500 {
		t.Fatalf("first card line = %+v", got[0])
	}
}
