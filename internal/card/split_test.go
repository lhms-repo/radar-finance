package card

import (
	"context"
	"errors"
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
