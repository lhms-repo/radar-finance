// Package membersplit keeps how an expense of a group is divided between its members and allocates the cents
// (RN-28). The settlement in internal/settle reads it; the services that record expenses write it.
package membersplit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/config"
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
// The products go through big integers, since custom values in cents times an amount in cents overflow int64.
func Allocate(amount int64, ws []Weighted) []int64 {
	out := make([]int64, len(ws))
	total := new(big.Int)
	for _, w := range ws {
		if w.Weight > 0 {
			total.Add(total, big.NewInt(w.Weight))
		}
	}
	if total.Sign() == 0 || amount == 0 {
		return out
	}
	sign := int64(1)
	if amount < 0 {
		sign, amount = -1, -amount
	}
	rems := make([]*big.Int, len(ws))
	var given int64
	for i, w := range ws {
		rems[i] = new(big.Int)
		if w.Weight <= 0 {
			continue
		}
		q, r := new(big.Int).QuoRem(new(big.Int).Mul(big.NewInt(amount), big.NewInt(w.Weight)), total, new(big.Int))
		out[i], rems[i] = q.Int64(), r
		given += out[i]
	}
	order := make([]int, len(ws))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return rems[order[a]].Cmp(rems[order[b]]) > 0 })
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

// exact tells the kinds whose values must add up to the expense itself; bills, financings and recurrences repeat
// with other amounts, so their values are proportions and only need to be above zero.
func exact(kind Kind) bool { return kind == KindEntry || kind == KindPurchase || kind == KindAlelo }

// Save records the division of an item of the group; nil keeps what is there (RN-28). Creating, it is the division
// of the new item from its start. Otherwise it is an edit, which applies from at on, so what was paid or closed
// keeps its division, and it records nothing when the division did not change. A personal space keeps none.
func Save(ctx context.Context, q *store.Queries, groupID int64, kind Kind, itemID int64, r *Rule, amount int64, at time.Time, creating bool) error {
	if r == nil {
		return nil
	}
	if r.Mode == "" {
		r.Mode = ModeWeights
	}
	g, err := q.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group of the split: %w", err)
	}
	if g.Kind == "personal" {
		return nil
	}
	if err := validate(ctx, q, groupID, kind, r, amount); err != nil {
		return err
	}
	if creating {
		if err := q.DeleteItemSplits(ctx, store.DeleteItemSplitsParams{ItemKind: string(kind), ItemID: itemID}); err != nil {
			return fmt.Errorf("clear member split: %w", err)
		}
		if r.Mode == ModeWeights {
			return nil
		}
		return insert(ctx, q, kind, itemID, "", "", r)
	}
	cur, err := Load(ctx, q, kind, itemID)
	if err != nil {
		return err
	}
	if cur.Equal(r) {
		return nil
	}
	loc, err := time.LoadLocation(config.Location)
	if err != nil {
		loc = time.UTC
	}
	return insert(ctx, q, kind, itemID, civil.DateOf(at.In(loc)).String(), at.UTC().Format(time.RFC3339Nano), r)
}

// validate checks the members and values of a division against the group and the part of the group in the expense.
func validate(ctx context.Context, q *store.Queries, groupID int64, kind Kind, r *Rule, amount int64) error {
	if r.Mode == ModeWeights {
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
	if r.Mode == ModeCustom && exact(kind) {
		if amount <= 0 {
			return ErrNothingToCut
		}
		if sum != amount {
			return ErrSumMismatch
		}
	}
	return nil
}

// insert writes one dated division with its members.
func insert(ctx context.Context, q *store.Queries, kind Kind, itemID int64, day, setAt string, r *Rule) error {
	id, err := q.InsertSplitRule(ctx, store.InsertSplitRuleParams{ItemKind: string(kind), ItemID: itemID, FromDay: day, SetAt: setAt, Mode: string(r.Mode)})
	if err != nil {
		return fmt.Errorf("save member split: %w", err)
	}
	if r.Mode == ModeWeights {
		return nil
	}
	for _, m := range r.Members {
		share := m.ShareCents
		if r.Mode == ModeEqual {
			share = 0
		} else if share == 0 {
			continue
		}
		if err := q.InsertSplitShare(ctx, store.InsertSplitShareParams{SplitID: id, PersonID: m.PersonID, ShareCents: share}); err != nil {
			return fmt.Errorf("save member split share: %w", err)
		}
	}
	return nil
}

// Load reads the division in force for an item, nil when it follows the weights of the group.
func Load(ctx context.Context, q *store.Queries, kind Kind, itemID int64) (*Rule, error) {
	row, err := q.LatestSplitRule(ctx, store.LatestSplitRuleParams{ItemKind: string(kind), ItemID: itemID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if Mode(row.Mode) == ModeWeights {
		return nil, nil
	}
	shares, err := q.ListSplitShares(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	r := &Rule{Mode: Mode(row.Mode)}
	for _, sh := range shares {
		r.Members = append(r.Members, Member{PersonID: sh.PersonID, ShareCents: sh.ShareCents})
	}
	return r, nil
}

// Equal tells whether two divisions are the same, nil standing for the weights.
func (r *Rule) Equal(o *Rule) bool {
	norm := func(x *Rule) (Mode, map[int64]int64) {
		if x == nil || x.Mode == "" || x.Mode == ModeWeights {
			return ModeWeights, nil
		}
		m := map[int64]int64{}
		for _, mm := range x.Members {
			if x.Mode == ModeEqual {
				m[mm.PersonID] = 0
			} else if mm.ShareCents > 0 {
				m[mm.PersonID] = mm.ShareCents
			}
		}
		return x.Mode, m
	}
	am, a := norm(r)
	bm, b := norm(o)
	if am != bm || len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// Applies tells whether something that starts on fromDay at setAt (empty for the start) is in force for an expense
// of day that was recorded at recorded: from the next days on, and on that day only for what was recorded after it,
// so a change never rewrites what came before (RN-28).
func Applies(fromDay, setAt string, day civil.Date, recorded time.Time) bool {
	if fromDay == "" {
		return true
	}
	from, err := civil.ParseDate(fromDay)
	if err != nil {
		return false
	}
	if from.Before(day) {
		return true
	}
	if from != day {
		return false
	}
	if setAt == "" {
		return true
	}
	at, err := time.Parse(time.RFC3339Nano, setAt)
	if err != nil {
		return true
	}
	return !recorded.Before(at)
}
