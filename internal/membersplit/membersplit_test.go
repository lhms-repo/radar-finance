package membersplit

import (
	"slices"
	"testing"
)

// TestAllocate divides amounts by weight with the cents to the largest remainders and ties to whoever comes first,
// always adding up to the amount (RN-03, RN-28).
func TestAllocate(t *testing.T) {
	tests := []struct {
		name   string
		amount int64
		ws     []Weighted
		want   []int64
	}{
		{name: "equal thirds, cent to the first", amount: 10_000, ws: []Weighted{{1, 3334}, {2, 3333}, {3, 3333}}, want: []int64{3334, 3333, 3333}},
		{name: "equal weights, two cents left", amount: 100, ws: []Weighted{{1, 1}, {2, 1}, {3, 1}}, want: []int64{34, 33, 33}},
		{name: "largest remainder wins over order", amount: 10, ws: []Weighted{{1, 1}, {2, 2}}, want: []int64{3, 7}},
		{name: "sixty forty", amount: 12_345, ws: []Weighted{{1, 6000}, {2, 4000}}, want: []int64{7407, 4938}},
		{name: "credit gives negative parts", amount: -100, ws: []Weighted{{1, 1}, {2, 1}, {3, 1}}, want: []int64{-34, -33, -33}},
		{name: "zero weight takes nothing", amount: 101, ws: []Weighted{{1, 1}, {2, 0}, {3, 1}}, want: []int64{51, 0, 50}},
		{name: "custom values as proportion", amount: 1_000, ws: []Weighted{{1, 700}, {2, 300}}, want: []int64{700, 300}},
		{name: "one cent among three", amount: 1, ws: []Weighted{{1, 1}, {2, 1}, {3, 1}}, want: []int64{1, 0, 0}},
		{name: "nobody", amount: 50, ws: nil, want: []int64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Allocate(tt.amount, tt.ws)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("Allocate(%d) = %v, want %v", tt.amount, got, tt.want)
			}
			var sum int64
			for _, v := range got {
				sum += v
			}
			if len(tt.ws) > 0 && sum != tt.amount {
				t.Fatalf("parts add up to %d, want %d", sum, tt.amount)
			}
		})
	}
}

// TestRuleWeights turns each mode into proportions over the members of the group, in their order (RN-28).
func TestRuleWeights(t *testing.T) {
	group := []Weighted{{1, 5000}, {2, 3000}, {3, 2000}}
	tests := []struct {
		name string
		rule *Rule
		want []Weighted
	}{
		{name: "no rule follows the group", rule: nil, want: group},
		{name: "weights follows the group", rule: &Rule{Mode: ModeWeights}, want: group},
		{name: "equal between two, in group order", rule: &Rule{Mode: ModeEqual, Members: []Member{{PersonID: 3}, {PersonID: 1}}}, want: []Weighted{{1, 1}, {3, 1}}},
		{name: "custom values", rule: &Rule{Mode: ModeCustom, Members: []Member{{2, 700}, {1, 300}}}, want: []Weighted{{1, 300}, {2, 700}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rule.Weights(group); !slices.Equal(got, tt.want) {
				t.Fatalf("Weights = %v, want %v", got, tt.want)
			}
		})
	}
}
