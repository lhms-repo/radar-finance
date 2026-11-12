package card

import (
	"context"
	"errors"
	"math/big"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/civil"
)

// TestSplitShares works out the share of each person, in equal parts or custom values (RF-24, RF-25).
func TestSplitShares(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.newCard(t)
	tests := []struct {
		name       string
		total      int64
		n          int
		split      SplitInput
		wantShares map[string]int64
		wantCouple int64
		wantErr    error
	}{
		{
			name: "equal, couple as one part", total: 300_000, n: 3,
			split:      SplitInput{Mode: SplitEqual, CoupleParts: 1, Parties: []PartyShare{{Name: "João Ficticio"}, {Name: "Maria Ficticia"}}},
			wantShares: map[string]int64{"João Ficticio": 100_000, "Maria Ficticia": 100_000}, wantCouple: 100_000,
		},
		{
			name: "equal, couple as two parts", total: 300_000, n: 1,
			split:      SplitInput{Mode: SplitEqual, CoupleParts: 2, Parties: []PartyShare{{Name: "João Ficticio"}, {Name: "Maria Ficticia"}}},
			wantShares: map[string]int64{"João Ficticio": 75_000, "Maria Ficticia": 75_000}, wantCouple: 150_000,
		},
		{
			name: "equal, leftover cents stay with the couple", total: 100_000, n: 1,
			split:      SplitInput{Mode: SplitEqual, CoupleParts: 1, Parties: []PartyShare{{Name: "João Ficticio"}, {Name: "Maria Ficticia"}}},
			wantShares: map[string]int64{"João Ficticio": 33_333, "Maria Ficticia": 33_333}, wantCouple: 33_334,
		},
		{
			name: "custom values", total: 100_000, n: 2,
			split:      SplitInput{Mode: SplitCustom, Parties: []PartyShare{{Name: "João Ficticio", ShareCents: 50_000}, {Name: "Maria Ficticia", ShareCents: 25_000}}},
			wantShares: map[string]int64{"João Ficticio": 50_000, "Maria Ficticia": 25_000}, wantCouple: 25_000,
		},
		{
			name: "custom, all for others", total: 10_000, n: 1,
			split:      SplitInput{Mode: SplitCustom, Parties: []PartyShare{{Name: "João Ficticio", ShareCents: 10_000}}},
			wantShares: map[string]int64{"João Ficticio": 10_000}, wantCouple: 0,
		},
		{name: "custom above the total", total: 10_000, n: 1, split: SplitInput{Mode: SplitCustom, Parties: []PartyShare{{Name: "João Ficticio", ShareCents: 10_001}}}, wantErr: ErrSplitTooLarge},
		{name: "custom zero share", total: 10_000, n: 1, split: SplitInput{Mode: SplitCustom, Parties: []PartyShare{{Name: "João Ficticio"}}}, wantErr: ErrShareRequired},
		{name: "same person twice ignoring case", total: 10_000, n: 1, split: SplitInput{Mode: SplitEqual, CoupleParts: 1, Parties: []PartyShare{{Name: "João"}, {Name: " joão "}}}, wantErr: ErrDuplicateParty},
		{name: "empty name", total: 10_000, n: 1, split: SplitInput{Mode: SplitEqual, CoupleParts: 1, Parties: []PartyShare{{Name: " "}}}, wantErr: ErrPartyName},
		{name: "nobody to split with", total: 10_000, n: 1, split: SplitInput{Mode: SplitEqual, CoupleParts: 1}, wantErr: ErrPartyName},
		{name: "negative couple parts", total: 10_000, n: 1, split: SplitInput{Mode: SplitEqual, CoupleParts: -1, Parties: []PartyShare{{Name: "João"}}}, wantErr: ErrInvalidParts},
		{name: "share smaller than the installments", total: 10_000, n: 3, split: SplitInput{Mode: SplitCustom, Parties: []PartyShare{{Name: "João", ShareCents: 2}}}, wantErr: ErrShareRequired},
		{name: "unknown mode", total: 10_000, n: 1, split: SplitInput{Mode: "thirds", Parties: []PartyShare{{Name: "João"}}}, wantErr: ErrInvalidSplit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			split := tt.split
			p, err := f.s.AddPurchase(ctx, f.userID, PurchaseInput{
				CardID: c.ID, Date: d(2026, time.November, 20), Merchant: "Loja de Climatização", CategoryID: f.delivery,
				AmountCents: tt.total, Installments: tt.n, Split: &split,
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if len(p.Splits) != len(tt.wantShares) || p.CoupleShareCents != tt.wantCouple {
				t.Fatalf("splits = %+v, couple %d", p.Splits, p.CoupleShareCents)
			}
			for _, s := range p.Splits {
				if tt.wantShares[s.PartyName] != s.ShareCents {
					t.Fatalf("share of %s = %d, want %d", s.PartyName, s.ShareCents, tt.wantShares[s.PartyName])
				}
			}
		})
	}

	// Names are reused across purchases, ignoring case (RF-27).
	parties, err := f.s.Parties(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(parties) != 2 || parties[0].Name != "João Ficticio" || parties[1].Name != "Maria Ficticia" {
		t.Fatalf("parties = %+v", parties)
	}
}

// TestInvoiceThirdParty spreads each share over the installments and nets it out of the invoice (RN-03, RN-05).
func TestInvoiceThirdParty(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.newCard(t)
	if _, err := f.s.AddPurchase(ctx, f.userID, PurchaseInput{
		CardID: c.ID, Date: d(2026, time.November, 20), Merchant: "Ar-condicionado", CategoryID: f.delivery, AmountCents: 300_000, Installments: 3,
		Split: &SplitInput{Mode: SplitEqual, CoupleParts: 1, Parties: []PartyShare{{Name: "João Ficticio"}, {Name: "Maria Ficticia"}}},
	}); err != nil {
		t.Fatal(err)
	}
	f.buy(t, c.ID, d(2026, time.November, 21), 5_000, 1)

	tests := []struct {
		month                   time.Month
		year                    int
		total, third, net, item int64
	}{
		{year: 2026, month: time.December, total: 105_000, third: 66_668, net: 38_332, item: 66_668},
		{year: 2027, month: time.January, total: 100_000, third: 66_666, net: 33_334, item: 66_666},
		{year: 2027, month: time.February, total: 100_000, third: 66_666, net: 33_334, item: 66_666},
	}
	var third, net int64
	for _, tt := range tests {
		t.Run(civil.Month{Year: tt.year, Month: tt.month}.String(), func(t *testing.T) {
			inv := f.invoice(t, c.ID, tt.year, tt.month)
			if inv.TotalCents != tt.total || inv.ThirdPartyCents != tt.third || inv.NetCents != tt.net || inv.PendingCents != tt.third || inv.ReceivedCents != 0 {
				t.Fatalf("invoice = total %d, third %d, net %d, pending %d", inv.TotalCents, inv.ThirdPartyCents, inv.NetCents, inv.PendingCents)
			}
			var ac *Item
			for i := range inv.Items {
				if inv.Items[i].Merchant == "Ar-condicionado" {
					ac = &inv.Items[i]
				}
			}
			if ac == nil || ac.ThirdPartyCents != tt.item || len(ac.Shares) != 2 || ac.Shares[0].PartyName != "João Ficticio" {
				t.Fatalf("item = %+v", ac)
			}
			third += inv.ThirdPartyCents
			net += inv.NetCents - map[time.Month]int64{time.December: 5_000}[tt.month]
		})
	}
	if third != 200_000 || net != 100_000 {
		t.Fatalf("over the installments: third %d, couple %d; want 200000 and 100000", third, net)
	}
}

// TestInvoiceCouplePartNeverNegative keeps the couple part of every invoice at zero or more when the whole
// purchase belongs to other people and the cents do not divide evenly (RN-03, RN-05).
func TestInvoiceCouplePartNeverNegative(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.newCard(t)
	if _, err := f.s.AddPurchase(ctx, f.userID, PurchaseInput{
		CardID: c.ID, Date: d(2026, time.November, 20), Merchant: "Loja", CategoryID: f.delivery, AmountCents: 10_000, Installments: 3,
		Split: &SplitInput{Mode: SplitEqual, CoupleParts: 0, Parties: []PartyShare{{Name: "João Ficticio"}, {Name: "Maria Ficticia"}}},
	}); err != nil {
		t.Fatal(err)
	}
	for _, m := range []struct {
		y  int
		mo time.Month
	}{{2026, time.December}, {2027, time.January}, {2027, time.February}} {
		inv := f.invoice(t, c.ID, m.y, m.mo)
		if inv.NetCents != 0 || inv.ThirdPartyCents != inv.TotalCents {
			t.Fatalf("%d-%02d: total %d, third %d, couple %d; the couple keeps nothing of this purchase", m.y, m.mo, inv.TotalCents, inv.ThirdPartyCents, inv.NetCents)
		}
	}
}

// TestSplitGuards keeps purchases with a received share from changing.
func TestSplitGuards(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.newCard(t)
	split := SplitInput{Mode: SplitEqual, CoupleParts: 1, Parties: []PartyShare{{Name: "João Ficticio"}}}
	p, err := f.s.AddPurchase(ctx, f.userID, PurchaseInput{CardID: c.ID, Date: d(2026, time.November, 20), Merchant: "Loja", CategoryID: f.delivery, AmountCents: 20_000, Installments: 2, Split: &split})
	if err != nil {
		t.Fatal(err)
	}

	// Editing a purchase without receipts regenerates the shares.
	in := PurchaseInput{CardID: c.ID, Date: d(2026, time.November, 20), Merchant: "Loja", CategoryID: f.delivery, AmountCents: 30_000, Installments: 3, Split: &split}
	got, err := f.s.UpdatePurchase(ctx, p.ID, in)
	if err != nil || got.Splits[0].ShareCents != 15_000 {
		t.Fatalf("update = %+v, %v", got, err)
	}
	var rows int
	_ = f.conn.QueryRow(`SELECT count(*) FROM split_installments`).Scan(&rows)
	if rows != 3 {
		t.Fatalf("split installments = %d, want 3", rows)
	}
	// Removing the split keeps the purchase for the couple only.
	in.Split = nil
	if got, err := f.s.UpdatePurchase(ctx, p.ID, in); err != nil || len(got.Splits) != 0 || got.CoupleShareCents != 30_000 {
		t.Fatalf("without split = %+v, %v", got, err)
	}

	in.Split = &split
	if _, err := f.s.UpdatePurchase(ctx, p.ID, in); err != nil {
		t.Fatal(err)
	}
	// Stand-in for a received share, which phase 4.2 records.
	if _, err := f.conn.Exec(`INSERT INTO entries (account_id, kind, amount_cents, occurred_on, created_by, created_at, updated_at) VALUES (?, 'receivable', 5000, '2026-11-25', ?, ?, ?)`, f.accountID, f.userID, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.conn.Exec(`UPDATE split_installments SET entry_id = (SELECT max(id) FROM entries) WHERE number = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.UpdatePurchase(ctx, p.ID, in); !errors.Is(err, ErrSplitReceived) {
		t.Fatalf("update with a received share: %v", err)
	}
	if err := f.s.DeletePurchase(ctx, p.ID); !errors.Is(err, ErrSplitReceived) {
		t.Fatalf("delete with a received share: %v", err)
	}
}

// TestSpreadShares spreads each share over the installments following the installment amounts, so the shares
// never add up to more than an installment and the couple part of a month is never negative (RN-03, RN-04, RN-05).
func TestSpreadShares(t *testing.T) {
	// Equal split among three people, which used to pile cents on some installments: 14,33 and 14,35 on equal ones.
	t.Run("three people stay within a cent", func(t *testing.T) {
		inst := Split(68_823, 16)
		shares := []int64{22_941, 22_941, 22_941}
		checkProportional(t, inst, shares, spreadShares(inst, shares))
	})
	tests := []struct {
		name   string
		total  int64
		n      int
		shares []int64
		want   [][]int64
	}{
		{
			// R$ 100,00 in 3x, all of it for two other people: 33,34 / 33,33 / 33,33.
			name: "whole purchase for two people", total: 10_000, n: 3, shares: []int64{5_000, 5_000},
			want: [][]int64{{1_667, 1_667, 1_666}, {1_667, 1_666, 1_667}},
		},
		{name: "one installment", total: 10_000, n: 1, shares: []int64{3_000}, want: [][]int64{{3_000}}},
		{name: "even split", total: 9_000, n: 3, shares: []int64{3_000, 1_500}, want: [][]int64{{1_000, 1_000, 1_000}, {500, 500, 500}}},
		{name: "one cent per installment each", total: 6, n: 3, shares: []int64{3, 3}, want: [][]int64{{1, 1, 1}, {1, 1, 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := spreadShares(Split(tt.total, tt.n), tt.shares)
			for p := range tt.want {
				for i := range tt.want[p] {
					if got[p][i] != tt.want[p][i] {
						t.Fatalf("got %v, want %v", got, tt.want)
					}
				}
			}
		})
	}
}

// checkProportional fails when a cell is not the floor or the ceiling of its proportional value: one cent per
// installment plus the rest of the share in proportion to the room each installment has after those cents.
func checkProportional(t *testing.T, inst, shares []int64, got [][]int64) {
	t.Helper()
	var room int64
	for _, a := range inst {
		room += a - int64(len(shares))
	}
	for p, s := range shares {
		for i, a := range inst {
			// The product reaches 1e22 at the amount cap, past int64.
			num := new(big.Int).Mul(big.NewInt(s-int64(len(inst))), big.NewInt(a-int64(len(shares))))
			low, high := int64(1), int64(1)
			if room > 0 {
				q, r := new(big.Int).QuoRem(num, big.NewInt(room), new(big.Int))
				low += q.Int64()
				high = low
				if r.Sign() != 0 {
					high++
				}
			}
			if got[p][i] < low || got[p][i] > high {
				t.Fatalf("installments %v shares %v: person %d has %d in installment %d, want %d or %d", inst, shares, p, got[p][i], i, low, high)
			}
		}
	}
}

// TestSpreadSharesInvariants checks many purchases: each share adds up exactly, no installment carries more
// in shares than its amount and every person has at least one cent per installment.
func TestSpreadSharesInvariants(t *testing.T) {
	for _, total := range []int64{7, 100, 1_001, 10_000, 99_999, 100_000_000_000} {
		for n := 1; n <= 12; n++ {
			if total < int64(n) {
				continue
			}
			for _, people := range [][]int64{{1}, {1, 1}, {1, 1, 1}, {1, 2, 3}, {5, 5}, {1, 1, 1, 1}, {1, 1, 1, 1, 1, 1, 1, 1, 1, 1}} {
				// Turn the weights into shares that use the whole purchase, the hardest case.
				var weight int64
				for _, w := range people {
					weight += w
				}
				shares := make([]int64, len(people))
				var used int64
				for p, w := range people {
					shares[p] = total * w / weight
					used += shares[p]
				}
				shares[0] += total - used
				ok := true
				for _, s := range shares {
					if s < int64(n) {
						ok = false
					}
				}
				if !ok {
					continue
				}
				inst := Split(total, n)
				got := spreadShares(inst, shares)
				for p, s := range shares {
					var sum int64
					for i, a := range got[p] {
						if a < 1 {
							t.Fatalf("total %d n %d shares %v: person %d has %d in installment %d", total, n, shares, p, a, i)
						}
						sum += a
					}
					if sum != s {
						t.Fatalf("total %d n %d shares %v: person %d adds %d, want %d", total, n, shares, p, sum, s)
					}
				}
				for i, amount := range inst {
					var col int64
					for p := range shares {
						col += got[p][i]
					}
					if col > amount {
						t.Fatalf("total %d n %d shares %v: installment %d carries %d of %d", total, n, shares, i, col, amount)
					}
				}
				checkProportional(t, inst, shares, got)
			}
		}
	}
}

// TestSpreadSharesRandom checks the invariants on random purchases with a fixed seed: up to 48 installments,
// up to 10 people and any valid shares.
func TestSpreadSharesRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 11))
	for range 20_000 {
		n := 1 + r.IntN(48)
		people := 1 + r.IntN(10)
		minTotal := int64(n * people)
		total := minTotal + r.Int64N(1_000_000)
		// Each person gets n cents, then a random slice of what is left, possibly leaving some for the couple.
		shares := make([]int64, people)
		spare := total - minTotal
		for p := range shares {
			give := int64(0)
			if spare > 0 {
				give = r.Int64N(spare + 1)
			}
			shares[p] = int64(n) + give
			spare -= give
		}
		inst := Split(total, n)
		got := spreadShares(inst, shares)
		for p, s := range shares {
			var sum int64
			for _, a := range got[p] {
				sum += a
			}
			if sum != s {
				t.Fatalf("installments %v shares %v: person %d adds %d, want %d", inst, shares, p, sum, s)
			}
		}
		for i, amount := range inst {
			var col int64
			for p := range shares {
				col += got[p][i]
			}
			if col > amount {
				t.Fatalf("installments %v shares %v: installment %d carries %d", inst, shares, i, col)
			}
		}
		checkProportional(t, inst, shares, got)
	}
}
