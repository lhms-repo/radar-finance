package card

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

// writeSplits stores the shares and spreads each one over the installments of the purchase (RN-03).
func (s *Service) writeSplits(ctx context.Context, q *store.Queries, userID, purchaseID int64, p Purchase, shares []PartyShare) error {
	for _, sh := range shares {
		partyID, err := s.partyID(ctx, q, userID, sh.Name)
		if err != nil {
			return err
		}
		splitID, err := q.CreateSplit(ctx, store.CreateSplitParams{PurchaseID: purchaseID, PartyID: partyID, ShareCents: sh.ShareCents})
		if err != nil {
			return fmt.Errorf("create split: %w", err)
		}
		for i, amount := range Split(sh.ShareCents, p.Installments) {
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
