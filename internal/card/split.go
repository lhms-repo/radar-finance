package card

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode/utf8"

	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/problem"
)

// Limits of a split.
const (
	maxParties   = 10
	maxPartyName = 40
)

// SplitMode tells how the shares are worked out (RF-25).
type SplitMode string

// Split modes: equal parts, with the couple counting as CoupleParts, or custom values.
const (
	SplitEqual  SplitMode = "equal"
	SplitCustom SplitMode = "custom"
)

// Business errors of splits.
var (
	ErrInvalidSplit   = problem.Invalid("split_invalid", "Escolha dividir em partes iguais ou em valores.")
	ErrPartyName      = problem.Invalid("split_party_name", "Informe o nome de cada pessoa da divisão.")
	ErrDuplicateParty = problem.Invalid("split_duplicate_party", "A mesma pessoa aparece duas vezes na divisão.")
	ErrTooManyParties = problem.Invalid("split_too_many", "Divida com no máximo %d pessoas.", maxParties)
	ErrInvalidParts   = problem.Invalid("split_parts_invalid", "Informe quantas partes o casal conta, de zero para cima.")
	ErrShareRequired  = problem.Invalid("split_share_required", "Cada pessoa precisa de pelo menos um centavo por parcela.")
	ErrSplitTooLarge  = problem.Invalid("split_too_large", "As cotas passam do valor da compra.")
	ErrSplitCredit    = problem.Invalid("split_credit", "Estorno ou crédito não pode ser dividido.")
	ErrSplitReceived  = problem.Conflict("split_received", "Esta compra tem cotas já recebidas. Desfaça os recebimentos em A receber antes de mudá-la.")
)

// PartyShare is a person of the split by name, with the share in custom mode.
type PartyShare struct {
	Name       string
	ShareCents int64
}

// SplitInput divides a purchase between the couple and other people (RF-24, RF-25).
type SplitInput struct {
	Mode SplitMode
	// CoupleParts is how many equal parts the couple keeps; zero leaves everything to the others.
	CoupleParts int
	Parties     []PartyShare
}

// SplitShare is the whole share of a person in a purchase.
type SplitShare struct {
	PartyID    int64
	PartyName  string
	ShareCents int64
}

// ItemShare is the share of a person in one installment of an invoice.
type ItemShare struct {
	PartyName   string
	AmountCents int64
	Received    bool
}

// Party is a person outside the couple, known by name (RF-27).
type Party struct {
	ID   int64
	Name string
}

// Parties lists the names already used, for suggestions (RF-27).
func (s *Service) Parties(ctx context.Context) ([]Party, error) {
	rows, err := store.New(s.conn).ListParties(ctx)
	if err != nil {
		return nil, fmt.Errorf("list parties: %w", err)
	}
	out := make([]Party, 0, len(rows))
	for _, r := range rows {
		out = append(out, Party{ID: r.ID, Name: r.Name})
	}
	return out, nil
}

// resolveShares works out the share of each person; equal parts round down and the couple keeps the leftover cents.
func resolveShares(total int64, installments int, in *SplitInput) ([]PartyShare, error) {
	if in == nil {
		return nil, nil
	}
	if len(in.Parties) == 0 {
		return nil, ErrPartyName
	}
	if len(in.Parties) > maxParties {
		return nil, ErrTooManyParties
	}
	seen := map[string]bool{}
	out := make([]PartyShare, 0, len(in.Parties))
	for _, p := range in.Parties {
		name := strings.TrimSpace(p.Name)
		if name == "" || utf8.RuneCountInString(name) > maxPartyName {
			return nil, ErrPartyName
		}
		key := strings.ToLower(name)
		if seen[key] {
			return nil, ErrDuplicateParty
		}
		seen[key] = true
		out = append(out, PartyShare{Name: name, ShareCents: p.ShareCents})
	}
	switch in.Mode {
	case SplitEqual:
		if in.CoupleParts < 0 {
			return nil, ErrInvalidParts
		}
		each := total / int64(in.CoupleParts+len(out))
		for i := range out {
			out[i].ShareCents = each
		}
	case SplitCustom:
		var sum int64
		for _, p := range out {
			if p.ShareCents <= 0 {
				return nil, ErrShareRequired
			}
			sum += p.ShareCents
		}
		if sum > total {
			return nil, ErrSplitTooLarge
		}
	default:
		return nil, ErrInvalidSplit
	}
	for _, p := range out {
		if p.ShareCents < int64(installments) {
			return nil, ErrShareRequired
		}
	}
	return out, nil
}

// writeSplits stores the shares and spreads each one over the installments of the purchase, following the
// installment amounts (RN-03).
func (s *Service) writeSplits(ctx context.Context, q *store.Queries, userID, purchaseID int64, p Purchase, shares []PartyShare) error {
	amounts := make([]int64, len(shares))
	for k, sh := range shares {
		amounts[k] = sh.ShareCents
	}
	spread := spreadShares(Split(p.TotalCents, p.Installments), amounts)
	for k, sh := range shares {
		partyID, err := s.partyID(ctx, q, userID, sh.Name)
		if err != nil {
			return err
		}
		splitID, err := q.CreateSplit(ctx, store.CreateSplitParams{PurchaseID: purchaseID, PartyID: partyID, ShareCents: sh.ShareCents})
		if err != nil {
			return fmt.Errorf("create split: %w", err)
		}
		for i, amount := range spread[k] {
			if err := q.CreateSplitInstallment(ctx, store.CreateSplitInstallmentParams{
				SplitID: splitID, Number: int64(i + 1), CardID: p.CardID, InvoiceMonth: p.FirstInvoice.Add(i).String(), AmountCents: amount,
			}); err != nil {
				return fmt.Errorf("create split installment: %w", err)
			}
		}
	}
	return nil
}

// partyID returns the id of the person with this name, ignoring case, creating it when new (RF-27).
func (s *Service) partyID(ctx context.Context, q *store.Queries, userID int64, name string) (int64, error) {
	p, err := q.GetPartyByName(ctx, name)
	if err == nil {
		return p.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("find party: %w", err)
	}
	created, err := q.CreateParty(ctx, store.CreatePartyParams{Name: name, CreatedBy: userID, CreatedAt: s.now().UTC()})
	if err != nil {
		return 0, fmt.Errorf("create party: %w", err)
	}
	return created.ID, nil
}

// checkNoReceived refuses changes to a purchase whose shares were already received.
func checkNoReceived(ctx context.Context, q *store.Queries, purchaseID int64) error {
	n, err := q.CountPaidSplitInstallments(ctx, purchaseID)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrSplitReceived
	}
	return nil
}

// spreadShares spreads each share over the installments so that the shares follow the installment amounts:
// every person gets at least one cent per installment, the shares of an installment never add up to more
// than it, so the couple part of a month is never negative, and the cents left over go, one at a time, to the
// installments with the largest remainder that still have room (RN-03, RN-04, RN-05). It needs, as
// resolveShares ensures, each share of at least one cent per installment and all shares within the total.
func spreadShares(installments, shares []int64) [][]int64 {
	n, people := len(installments), int64(len(shares))
	// Room in each installment after one cent per person, and their sum.
	room := make([]int64, n)
	var totalRoom int64
	for i, amount := range installments {
		room[i] = amount - people
		totalRoom += room[i]
	}
	// First every whole part, rest * room / totalRoom, exact since both factors can reach the 1e11 cents cap.
	out := make([][]int64, len(shares))
	remainders := make([][]*big.Int, len(shares))
	left := make([]int64, len(shares))
	free := append([]int64(nil), installments...)
	for p, share := range shares {
		out[p] = make([]int64, n)
		remainders[p] = make([]*big.Int, n)
		rest := share - int64(n)
		left[p] = rest
		for i := range installments {
			whole, r := new(big.Int), new(big.Int)
			if totalRoom > 0 {
				whole.QuoRem(new(big.Int).Mul(big.NewInt(rest), big.NewInt(room[i])), big.NewInt(totalRoom), r)
			}
			out[p][i] = 1 + whole.Int64()
			remainders[p][i] = r
			left[p] -= whole.Int64()
			free[i] -= out[p][i]
		}
	}
	// Then the cents left over: the whole parts never fill an installment past its amount, and together they
	// leave at least as much room as the cents still to place, so a free installment always exists.
	for p := range shares {
		for ; left[p] > 0; left[p]-- {
			best := -1
			for i := range installments {
				if free[i] > 0 && (best < 0 || remainders[p][i].Cmp(remainders[p][best]) > 0) {
					best = i
				}
			}
			out[p][best]++
			free[best]--
			remainders[p][best] = big.NewInt(-1)
		}
	}
	return out
}
