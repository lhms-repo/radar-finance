package market

import (
	"context"
	"sort"

	"github.com/italogfx/radar-finance/internal/db/store"
)

// RankedMarket is the total of the list in one market, with what is missing and stale (RF-72, RF-75, RN-23).
type RankedMarket struct {
	MarketID     int64
	Name         string
	Neighborhood string
	TotalCents   int64
	PricedCount  int
	MissingCount int
	StaleCount   int
	Missing      []string
}

// SplitItem is a list item bought in one market of the split, with its cost.
type SplitItem struct {
	ItemID    int64
	Label     string
	CostCents int64
}

// SplitMarket is what to buy in one market of the split.
type SplitMarket struct {
	MarketID   int64
	Name       string
	TotalCents int64
	Items      []SplitItem
}

// Split is the best division of the purchase among markets (RF-73, RN-24).
type Split struct {
	Markets    []SplitMarket
	TotalCents int64
	// SavingsCents is the saving over the best single market with every covered item; nil when no single market has them all.
	SavingsCents *int64
}

// Comparison is the ranking and the best split of a list (RF-72 to RF-75).
type Comparison struct {
	Ranking   []RankedMarket
	Split     *Split
	Uncovered []ListItem
	Settings  Settings
}

// itemCost is the cost of a quantity in thousandths at a price, truncated in cents (RN-23).
func itemCost(priceCents, qtyMilli int64) int64 {
	return priceCents * qtyMilli / 1_000
}

// Compare ranks the markets for a list and finds the best split, with the latest price of each product in each
// active market (RF-72 to RF-75, RN-22 to RN-25).
func (s *Service) Compare(ctx context.Context, listID int64) (Comparison, error) {
	detail, err := s.ListDetail(ctx, listID)
	if err != nil {
		return Comparison{}, err
	}
	q := store.New(s.conn)
	cfg, err := settings(ctx, q)
	if err != nil {
		return Comparison{}, err
	}
	prices, err := latest(ctx, q)
	if err != nil {
		return Comparison{}, err
	}
	all, err := s.Markets(ctx)
	if err != nil {
		return Comparison{}, err
	}
	staleLimit := s.today().AddDays(-cfg.StaleDays)
	out := Comparison{Settings: cfg, Ranking: []RankedMarket{}, Uncovered: []ListItem{}}

	var markets []Market
	for _, m := range all {
		if !m.Active {
			continue
		}
		r := RankedMarket{MarketID: m.ID, Name: m.Name, Neighborhood: m.Neighborhood, Missing: []string{}}
		for _, it := range detail.Items {
			p, ok := prices[Key{ProductID: it.Product.ID, MarketID: m.ID}]
			if !ok {
				r.MissingCount++
				r.Missing = append(r.Missing, it.Product.Label())
				continue
			}
			r.PricedCount++
			r.TotalCents += itemCost(p.PriceCents, it.QuantityMilli)
			if p.Date.Before(staleLimit) {
				r.StaleCount++
			}
		}
		if r.PricedCount > 0 {
			out.Ranking = append(out.Ranking, r)
			markets = append(markets, m)
		}
	}
	// Complete markets first by total, then the incomplete ones by missing items and total (RN-23).
	sort.SliceStable(out.Ranking, func(i, j int) bool {
		a, b := out.Ranking[i], out.Ranking[j]
		if (a.MissingCount == 0) != (b.MissingCount == 0) {
			return a.MissingCount == 0
		}
		if a.MissingCount != b.MissingCount {
			return a.MissingCount < b.MissingCount
		}
		return a.TotalCents < b.TotalCents
	})

	var covered []ListItem
	for _, it := range detail.Items {
		priced := false
		for _, m := range markets {
			if _, ok := prices[Key{ProductID: it.Product.ID, MarketID: m.ID}]; ok {
				priced = true
				break
			}
		}
		if priced {
			covered = append(covered, it)
		} else {
			out.Uncovered = append(out.Uncovered, it)
		}
	}
	if len(covered) > 0 {
		out.Split = bestSplit(covered, markets, prices, cfg)
	}
	return out, nil
}

// bestSplit applies RN-24 step by step: the best combination with one more market wins only when it saves the
// minimum over the best with one less; the search stops at the first step that does not.
func bestSplit(items []ListItem, markets []Market, prices map[Key]Price, cfg Settings) *Split {
	var current, single *Split
	for k := 1; k <= cfg.MaxMarkets && k <= len(markets); k++ {
		best := bestOfSize(items, markets, prices, k)
		if best == nil {
			continue
		}
		if k == 1 {
			single = best
		}
		if current == nil {
			current = best
			continue
		}
		if current.TotalCents-best.TotalCents < cfg.MinSavingsCents {
			break
		}
		current = best
	}
	if current == nil {
		return nil
	}
	if single != nil {
		saving := single.TotalCents - current.TotalCents
		current.SavingsCents = &saving
	}
	return current
}

// bestOfSize returns the cheapest combination of exactly k markets that prices every item and uses every market,
// each item going to the cheapest market of the combination.
func bestOfSize(items []ListItem, markets []Market, prices map[Key]Price, k int) *Split {
	var best *Split
	idx := make([]int, k)
	for i := range idx {
		idx[i] = i
	}
	for {
		if s := assign(items, markets, prices, idx); s != nil && (best == nil || s.TotalCents < best.TotalCents) {
			best = s
		}
		// Next combination in lexicographic order.
		i := k - 1
		for i >= 0 && idx[i] == len(markets)-k+i {
			i--
		}
		if i < 0 {
			return best
		}
		idx[i]++
		for j := i + 1; j < k; j++ {
			idx[j] = idx[j-1] + 1
		}
	}
}

// assign sends each item to the cheapest market of the combination; nil when an item has no price or a market is left idle.
func assign(items []ListItem, markets []Market, prices map[Key]Price, idx []int) *Split {
	out := &Split{Markets: make([]SplitMarket, len(idx))}
	for i, mi := range idx {
		out.Markets[i] = SplitMarket{MarketID: markets[mi].ID, Name: markets[mi].Name, Items: []SplitItem{}}
	}
	for _, it := range items {
		chosen, cost := -1, int64(0)
		for i, mi := range idx {
			p, ok := prices[Key{ProductID: it.Product.ID, MarketID: markets[mi].ID}]
			if !ok {
				continue
			}
			if c := itemCost(p.PriceCents, it.QuantityMilli); chosen < 0 || c < cost {
				chosen, cost = i, c
			}
		}
		if chosen < 0 {
			return nil
		}
		m := &out.Markets[chosen]
		m.Items = append(m.Items, SplitItem{ItemID: it.ID, Label: it.Product.Label(), CostCents: cost})
		m.TotalCents += cost
		out.TotalCents += cost
	}
	for _, m := range out.Markets {
		if len(m.Items) == 0 {
			return nil
		}
	}
	return out
}
