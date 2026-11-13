// Package membersplit keeps how an expense of a group is divided between its members and allocates the cents
// (RN-28). The settlement in internal/settle reads it; the services that record expenses write it.
package membersplit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/problem"
)

// Mode tells how the members divide an expense.
type Mode string

// Division modes: by the weights of the group, in equal parts between who took part, or by value of each member.
const (
	ModeWeights Mode = "weights"
	ModeEqual   Mode = "equal"
	ModeCustom  Mode = "custom"
)

// Kind names the record a division belongs to.
type Kind string

// Records that can carry a division between members (RF-84).
const (
	KindEntry      Kind = "entry"
	KindPurchase   Kind = "purchase"
	KindRecurrence Kind = "recurrence"
	KindBill       Kind = "bill"
	KindFinancing  Kind = "financing"
	KindAlelo      Kind = "alelo"
)

// Errors of a division.
var (
	ErrMode         = problem.Invalid("member_split_mode", "Escolha dividir pelos pesos, em partes iguais ou por valor.")
	ErrNotMember    = problem.Invalid("member_split_member", "Só membros do grupo entram na divisão.")
	ErrDuplicate    = problem.Invalid("member_split_duplicate", "O mesmo membro aparece duas vezes na divisão.")
	ErrNobody       = problem.Invalid("member_split_nobody", "Escolha pelo menos um membro para dividir.")
	ErrNegative     = problem.Invalid("member_split_negative", "O valor de um membro não pode ser negativo.")
	ErrSumMismatch  = problem.Invalid("member_split_sum", "Os valores dos membros precisam somar o valor do gasto.")
	ErrNothingToCut = problem.Invalid("member_split_empty", "Não há valor do grupo para dividir entre os membros.")
)

// Member is one member of a division, with her value in custom mode.
type Member struct {
	PersonID   int64
	ShareCents int64
}

// Rule is the division of one expense; nil or ModeWeights means the weights of the group.
type Rule struct {
	Mode    Mode
	Members []Member
}

// Weighted is someone who takes a part of an amount in proportion to Weight; the order breaks ties of cents.
type Weighted struct {
	PersonID int64
	Weight   int64
}

// Allocate divides amount in proportion to the weights, the cents left going to the largest remainders and, on a
// tie, to whoever comes first; the parts always add up to amount, and a negative amount gives negative parts (RN-03, RN-28).
func Allocate(amount int64, ws []Weighted) []int64 {
	out := make([]int64, len(ws))
	var total int64
	for _, w := range ws {
		if w.Weight > 0 {
			total += w.Weight
		}
	}
	if total == 0 || amount == 0 {
		return out
	}
	sign := int64(1)
	if amount < 0 {
		sign, amount = -1, -amount
	}
	rems := make([]int64, len(ws))
	var given int64
	for i, w := range ws {
		if w.Weight <= 0 {
			continue
		}
		out[i] = amount * w.Weight / total
		rems[i] = amount * w.Weight % total
		given += out[i]
	}
	order := make([]int, len(ws))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return rems[order[a]] > rems[order[b]] })
	for k := 0; given < amount; k++ {
		i := order[k%len(order)]
		if ws[i].Weight <= 0 {
			continue
		}
		out[i]++
		given++
	}
	for i := range out {
		out[i] *= sign
	}
	return out
}

// Weights turns a rule into the proportions of a division, given the weights of the group on the day; the members
// of the group come in their order, so ties of cents follow who entered first (RN-28).
func (r *Rule) Weights(group []Weighted) []Weighted {
	if r == nil || r.Mode == ModeWeights || len(r.Members) == 0 {
		return group
	}
	by := map[int64]int64{}
	for _, m := range r.Members {
		if r.Mode == ModeEqual {
			by[m.PersonID] = 1
		} else {
			by[m.PersonID] = m.ShareCents
		}
	}
	out := make([]Weighted, 0, len(r.Members))
	seen := map[int64]bool{}
	for _, g := range group {
		if w, ok := by[g.PersonID]; ok {
			out = append(out, Weighted{PersonID: g.PersonID, Weight: w})
			seen[g.PersonID] = true
		}
	}
	// A member of the rule who left the weights of the day still takes her part.
	for _, m := range r.Members {
		if !seen[m.PersonID] {
			out = append(out, Weighted{PersonID: m.PersonID, Weight: by[m.PersonID]})
		}
	}
	return out
}

// Save replaces the division of an item of the group; nil or weights removes it. In custom mode the values must
// add up to amount, the part of the group in the expense (RN-28). A personal space keeps no division.
func Save(ctx context.Context, q *store.Queries, groupID int64, kind Kind, itemID int64, r *Rule, amount int64) error {
	if err := q.DeleteMemberSplit(ctx, store.DeleteMemberSplitParams{ItemKind: string(kind), ItemID: itemID}); err != nil {
		return fmt.Errorf("clear member split: %w", err)
	}
	if r == nil || r.Mode == "" || r.Mode == ModeWeights {
		return nil
	}
	g, err := q.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group of the split: %w", err)
	}
	if g.Kind == "personal" {
		return nil
	}
	if r.Mode != ModeEqual && r.Mode != ModeCustom {
		return ErrMode
	}
	members, err := q.ListGroupMembers(ctx, groupID)
	if err != nil {
		return err
	}
	inGroup := map[int64]bool{}
	for _, m := range members {
		inGroup[m.PersonID] = true
	}
	seen := map[int64]bool{}
	var sum int64
	chosen := 0
	for _, m := range r.Members {
		switch {
		case !inGroup[m.PersonID]:
			return ErrNotMember
		case seen[m.PersonID]:
			return ErrDuplicate
		case m.ShareCents < 0:
			return ErrNegative
		}
		seen[m.PersonID] = true
		sum += m.ShareCents
		if r.Mode == ModeEqual || m.ShareCents > 0 {
			chosen++
		}
	}
	if chosen == 0 {
		return ErrNobody
	}
	if r.Mode == ModeCustom {
		if amount <= 0 {
			return ErrNothingToCut
		}
		if sum != amount {
			return ErrSumMismatch
		}
	}
	for _, m := range r.Members {
		share := m.ShareCents
		if r.Mode == ModeEqual {
			share = 0
		} else if share == 0 {
			continue
		}
		if err := q.InsertMemberSplit(ctx, store.InsertMemberSplitParams{ItemKind: string(kind), ItemID: itemID, Mode: string(r.Mode), PersonID: m.PersonID, ShareCents: share}); err != nil {
			return fmt.Errorf("save member split: %w", err)
		}
	}
	return nil
}

// Load reads the division of an item, nil when it follows the weights of the group.
func Load(ctx context.Context, q *store.Queries, kind Kind, itemID int64) (*Rule, error) {
	rows, err := q.ListMemberSplit(ctx, store.ListMemberSplitParams{ItemKind: string(kind), ItemID: itemID})
	if errors.Is(err, sql.ErrNoRows) || len(rows) == 0 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r := &Rule{Mode: Mode(rows[0].Mode)}
	for _, row := range rows {
		r.Members = append(r.Members, Member{PersonID: row.PersonID, ShareCents: row.ShareCents})
	}
	return r, nil
}
