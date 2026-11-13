// Package settle works out the settlement between the members of a group: what each one paid of the group's
// expenses, her share of them by the weights or the division chosen, the payments between members, and who pays
// whom to close it (RF-84, RN-28 to RN-31).
package settle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/config"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/membersplit"
	"github.com/italogfx/radar-finance/internal/problem"
	"github.com/italogfx/radar-finance/internal/scope"
)

// Errors of the settlement.
var (
	ErrPersonal        = problem.Conflict("settlement_personal", "O acerto é entre os membros de um grupo; no espaço Pessoal não há acerto.")
	ErrSamePerson      = problem.Invalid("settlement_same_person", "O pagamento de acerto vai de uma pessoa para outra: escolha contas de membros diferentes.")
	ErrPaymentNotFound = problem.NotFound("settlement_not_found", "Pagamento de acerto não encontrado.")
	ErrNotParty        = problem.Forbidden("settlement_not_party", "Só quem paga, quem recebe ou um administrador do grupo registra ou desfaz este pagamento.")
	ErrPaymentDate     = problem.Invalid("settlement_date", "A data do pagamento precisa estar entre o início do acerto e hoje.")
)

// Line kinds: an account expense, a card invoice line or an Alelo purchase.
const (
	LineAccount = "account"
	LineCard    = "card"
	LineAlelo   = "alelo"
)

// Share is the part of a member in one expense.
type Share struct {
	PersonID int64
	Name     string
	Cents    int64
}

// Line is one expense of the group in the settlement, with who paid it and the share of each member.
type Line struct {
	Date        civil.Date
	Kind        string
	Description string
	Source      string
	PayerID     int64
	PayerName   string
	AmountCents int64
	Shares      []Share
}

// MemberBalance is where a member stands: positive is owed to her, negative is what she owes (RN-29).
type MemberBalance struct {
	PersonID      int64
	Name          string
	PaidCents     int64
	ShareCents    int64
	SentCents     int64
	ReceivedCents int64
	BalanceCents  int64
}

// Suggestion is one payment that closes part of the settlement.
type Suggestion struct {
	FromPersonID int64
	FromName     string
	ToPersonID   int64
	ToName       string
	AmountCents  int64
}

// Payment is a recorded payment between members (RN-30).
type Payment struct {
	ID           int64
	TransferID   int64
	FromPersonID int64
	FromName     string
	ToPersonID   int64
	ToName       string
	AmountCents  int64
	Date         civil.Date
}

// Settlement is the state of the group: balances that add up to zero, the payments that close them, the payments
// already made and the expenses of the month asked for.
type Settlement struct {
	From        civil.Date
	Members     []MemberBalance
	Suggestions []Suggestion
	Payments    []Payment
	Lines       []Line
}

// PaymentInput records a payment from an account of one member to an account of another.
type PaymentInput struct {
	FromAccountID int64
	ToAccountID   int64
	AmountCents   int64
	Date          civil.Date
}

// Service computes and records the settlement.
type Service struct {
	conn   *sql.DB
	cards  *card.Service
	ledger *ledger.Service
	now    func() time.Time
	today  func() civil.Date
}

// NewService returns the settlement service over the database.
func NewService(conn *sql.DB, cards *card.Service) *Service {
	loc, err := time.LoadLocation(config.Location)
	if err != nil {
		loc = time.UTC
	}
	return &Service{conn: conn, cards: cards, ledger: ledger.NewService(conn), now: time.Now, today: func() civil.Date { return civil.Today(loc) }}
}

// WithToday fixes the business day, for tests.
func (s *Service) WithToday(today func() civil.Date) *Service {
	s.today = today
	return s
}

// weightRow is the weight of a member from a day and moment on.
type weightRow struct {
	from   string
	setAt  string
	weight int64
}

// datedRule is a division of an item from a day and moment on; an empty day is the division since the start.
type datedRule struct {
	from  string
	setAt string
	rule  *membersplit.Rule
}

// group holds what the settlement of the active group needs: its members in order, weights over time and divisions.
type group struct {
	id      int64
	from    civil.Date
	order   []int64
	names   map[int64]string
	weights map[int64][]weightRow
	rules   map[string][]datedRule
}

// load reads the members, their weights over time and the divisions of the active group.
func (s *Service) load(ctx context.Context, q *store.Queries) (group, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return group{}, err
	}
	g, err := q.GetGroup(ctx, groupID)
	if err != nil {
		return group{}, fmt.Errorf("active group: %w", err)
	}
	if g.Kind == "personal" {
		return group{}, ErrPersonal
	}
	out := group{id: groupID, names: map[int64]string{}, weights: map[int64][]weightRow{}, rules: map[string][]datedRule{}}
	from, err := q.GroupSettlesFrom(ctx, groupID)
	if err != nil {
		return group{}, err
	}
	if from != "" {
		if out.from, err = civil.ParseDate(from); err != nil {
			return group{}, err
		}
	}
	members, err := q.ListGroupMembers(ctx, groupID)
	if err != nil {
		return group{}, err
	}
	for _, m := range members {
		out.order = append(out.order, m.PersonID)
		out.names[m.PersonID] = m.PersonName
	}
	rows, err := q.ListGroupWeights(ctx, groupID)
	if err != nil {
		return group{}, err
	}
	for _, r := range rows {
		out.weights[r.PersonID] = append(out.weights[r.PersonID], weightRow{from: r.FromDay, setAt: r.SetAt, weight: r.WeightBp})
	}
	splits, err := q.ListGroupMemberSplits(ctx, groupID)
	if err != nil {
		return group{}, err
	}
	byID := map[int64]*membersplit.Rule{}
	for _, r := range splits {
		rule := byID[r.ID]
		if rule == nil {
			rule = &membersplit.Rule{Mode: membersplit.Mode(r.Mode)}
			byID[r.ID] = rule
			key := ruleKey(membersplit.Kind(r.ItemKind), r.ItemID)
			out.rules[key] = append(out.rules[key], datedRule{from: r.FromDay, setAt: r.SetAt, rule: rule})
		}
		if r.PersonID != 0 {
			rule.Members = append(rule.Members, membersplit.Member{PersonID: r.PersonID, ShareCents: r.ShareCents})
		}
	}
	return out, nil
}

// ruleKey names the division of an item in the map of a group.
func ruleKey(kind membersplit.Kind, id int64) string { return fmt.Sprintf("%s:%d", kind, id) }

// weightsOn returns the weights of the members for an expense of day d recorded at recorded, in the order they
// joined; who had no weight yet takes no part (RN-28).
func (g group) weightsOn(d civil.Date, recorded time.Time) []membersplit.Weighted {
	var out []membersplit.Weighted
	for _, p := range g.order {
		var w int64
		for _, r := range g.weights[p] {
			if membersplit.Applies(r.from, r.setAt, d, recorded) {
				w = r.weight
			}
		}
		if w > 0 {
			out = append(out, membersplit.Weighted{PersonID: p, Weight: w})
		}
	}
	return out
}

// split divides one expense of the group between the members with the weights and the division in force when it
// happened and was recorded (RN-28).
func (g group) split(l *Line, kind membersplit.Kind, id int64, recorded time.Time) {
	var rule *membersplit.Rule
	for _, r := range g.rules[ruleKey(kind, id)] {
		if membersplit.Applies(r.from, r.setAt, l.Date, recorded) {
			rule = r.rule
		}
	}
	ws := rule.Weights(g.weightsOn(l.Date, recorded))
	parts := membersplit.Allocate(l.AmountCents, ws)
	for i, w := range ws {
		l.Shares = append(l.Shares, Share{PersonID: w.PersonID, Name: g.names[w.PersonID], Cents: parts[i]})
	}
}

// lines gathers every expense of the group from the start of the settlement up to today (RF-84).
func (s *Service) lines(ctx context.Context, q *store.Queries, g group) ([]Line, error) {
	if g.from.IsZero() {
		return nil, nil
	}
	today := s.today()
	if today.Before(g.from) {
		return nil, nil
	}
	var out []Line
	entries, err := q.SettlementExpenses(ctx, store.SettlementExpensesParams{GroupID: g.id, FromDay: g.from.String(), ToDay: today.String()})
	if err != nil {
		return nil, fmt.Errorf("settlement expenses: %w", err)
	}
	for _, e := range entries {
		d, err := civil.ParseDate(e.OccurredOn)
		if err != nil {
			return nil, err
		}
		desc := e.Description
		if desc == "" {
			desc = e.CategoryName
		}
		l := Line{Date: d, Kind: LineAccount, Description: desc, Source: e.AccountName, PayerID: e.PayerID, PayerName: g.names[e.PayerID], AmountCents: -e.AmountCents}
		// Paying a bill or an installment follows the division of the bill or the financing.
		switch {
		case e.BillID > 0:
			g.split(&l, membersplit.KindBill, e.BillID, e.CreatedAt)
		case e.FinancingID > 0:
			g.split(&l, membersplit.KindFinancing, e.FinancingID, e.CreatedAt)
		default:
			g.split(&l, membersplit.KindEntry, e.ID, e.CreatedAt)
		}
		out = append(out, l)
	}
	alelo, err := q.SettlementAleloPurchases(ctx, store.SettlementAleloPurchasesParams{GroupID: g.id, FromDay: g.from.String(), ToDay: today.String()})
	if err != nil {
		return nil, fmt.Errorf("settlement alelo: %w", err)
	}
	for _, a := range alelo {
		d, err := civil.ParseDate(a.PurchasedOn)
		if err != nil {
			return nil, err
		}
		l := Line{Date: d, Kind: LineAlelo, Description: a.Description, Source: a.CardName, PayerID: a.PayerID, PayerName: g.names[a.PayerID], AmountCents: a.AmountCents}
		g.split(&l, membersplit.KindAlelo, a.ID, a.CreatedAt)
		out = append(out, l)
	}
	charges, err := s.cards.GroupCharges(ctx, q, g.from, today)
	if err != nil {
		return nil, err
	}
	for _, c := range charges {
		l := Line{Date: c.Date, Kind: LineCard, Description: c.Description, Source: c.CardName, PayerID: c.PayerID, PayerName: g.names[c.PayerID], AmountCents: c.AmountCents}
		kind := membersplit.KindPurchase
		if c.Kind == "recurrence" {
			kind = membersplit.KindRecurrence
		}
		g.split(&l, kind, c.RefID, c.Recorded)
		out = append(out, l)
	}
	return out, nil
}

// Settlement returns the balances of the members, the payments that close them, the payments made and the
// expenses of month (RF-84, RN-29).
func (s *Service) Settlement(ctx context.Context, month civil.Month) (Settlement, error) {
	q := store.New(s.conn)
	g, err := s.load(ctx, q)
	if err != nil {
		return Settlement{}, err
	}
	lines, err := s.lines(ctx, q, g)
	if err != nil {
		return Settlement{}, err
	}
	payments, err := q.ListSettlements(ctx, g.id)
	if err != nil {
		return Settlement{}, fmt.Errorf("list settlements: %w", err)
	}
	bal := map[int64]*MemberBalance{}
	at := func(p int64) *MemberBalance {
		if bal[p] == nil {
			bal[p] = &MemberBalance{PersonID: p, Name: g.names[p]}
		}
		return bal[p]
	}
	for _, p := range g.order {
		at(p)
	}
	out := Settlement{From: g.from, Payments: []Payment{}, Lines: []Line{}, Suggestions: []Suggestion{}}
	for _, l := range lines {
		if len(l.Shares) == 0 {
			continue
		}
		at(l.PayerID).PaidCents += l.AmountCents
		for _, sh := range l.Shares {
			at(sh.PersonID).ShareCents += sh.Cents
		}
		if l.Date.MonthOf() == month {
			out.Lines = append(out.Lines, l)
		}
	}
	for _, p := range payments {
		d, err := civil.ParseDate(p.PaidOn)
		if err != nil {
			return Settlement{}, err
		}
		at(p.FromPersonID).SentCents += p.AmountCents
		at(p.ToPersonID).ReceivedCents += p.AmountCents
		out.Payments = append(out.Payments, Payment{ID: p.ID, TransferID: p.TransferID, FromPersonID: p.FromPersonID, FromName: p.FromName, ToPersonID: p.ToPersonID, ToName: p.ToName, AmountCents: p.AmountCents, Date: d})
	}
	order := append([]int64{}, g.order...)
	for p := range bal {
		if g.names[p] == "" {
			order = append(order, p)
		}
	}
	for _, p := range order {
		b := bal[p]
		b.BalanceCents = b.PaidCents - b.ShareCents + b.SentCents - b.ReceivedCents
		out.Members = append(out.Members, *b)
	}
	out.Suggestions = suggest(out.Members)
	sort.SliceStable(out.Lines, func(i, j int) bool { return out.Lines[j].Date.Before(out.Lines[i].Date) })
	return out, nil
}

// suggest pairs who owes the most with who is owed the most until every balance is zero, which takes at most one
// payment less than the members with a balance.
func suggest(members []MemberBalance) []Suggestion {
	type side struct {
		id    int64
		name  string
		cents int64
	}
	var owe, owed []side
	for _, m := range members {
		switch {
		case m.BalanceCents < 0:
			owe = append(owe, side{m.PersonID, m.Name, -m.BalanceCents})
		case m.BalanceCents > 0:
			owed = append(owed, side{m.PersonID, m.Name, m.BalanceCents})
		}
	}
	byAmount := func(xs []side) {
		sort.SliceStable(xs, func(i, j int) bool { return xs[i].cents > xs[j].cents })
	}
	byAmount(owe)
	byAmount(owed)
	out := []Suggestion{}
	i, j := 0, 0
	for i < len(owe) && j < len(owed) {
		pay := min(owe[i].cents, owed[j].cents)
		out = append(out, Suggestion{FromPersonID: owe[i].id, FromName: owe[i].name, ToPersonID: owed[j].id, ToName: owed[j].name, AmountCents: pay})
		owe[i].cents -= pay
		owed[j].cents -= pay
		if owe[i].cents == 0 {
			i++
		}
		if owed[j].cents == 0 {
			j++
		}
	}
	return out
}

// Pay records a payment between members as a transfer between their accounts, which is neither spending nor
// income, and counts it in the settlement (RN-30).
func (s *Service) Pay(ctx context.Context, userID int64, in PaymentInput) (Payment, error) {
	if in.Date.IsZero() {
		return Payment{}, ledger.ErrDateRequired
	}
	if err := ledger.CheckPositive(in.AmountCents); err != nil {
		return Payment{}, err
	}
	var id int64
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		g, err := s.load(ctx, q)
		if err != nil {
			return err
		}
		holders := make([]int64, 0, 2)
		for _, acc := range []int64{in.FromAccountID, in.ToAccountID} {
			a, err := q.GetAccount(ctx, store.GetAccountParams{ID: acc, GroupID: g.id})
			if errors.Is(err, sql.ErrNoRows) {
				return ledger.ErrAccountNotFound
			}
			if err != nil {
				return err
			}
			holders = append(holders, a.PersonID)
		}
		if holders[0] == holders[1] {
			return ErrSamePerson
		}
		if err := checkParty(ctx, q, g.id, userID, holders[0], holders[1]); err != nil {
			return err
		}
		if in.Date.Before(g.from) || s.today().Before(in.Date) {
			return ErrPaymentDate
		}
		t, err := s.ledger.TransferTx(ctx, q, userID, ledger.NewTransfer{
			FromAccountID: in.FromAccountID, ToAccountID: in.ToAccountID, AmountCents: in.AmountCents, Date: in.Date,
			Description: fmt.Sprintf("Acerto de %s para %s", g.names[holders[0]], g.names[holders[1]]),
		})
		if err != nil {
			return err
		}
		id, err = q.CreateSettlement(ctx, store.CreateSettlementParams{
			GroupID: g.id, TransferID: t.ID, FromPersonID: holders[0], ToPersonID: holders[1], AmountCents: in.AmountCents,
			PaidOn: in.Date.String(), CreatedBy: userID, CreatedAt: s.now().UTC(),
		})
		return err
	})
	if err != nil {
		return Payment{}, err
	}
	st, err := s.Settlement(ctx, in.Date.MonthOf())
	if err != nil {
		return Payment{}, err
	}
	for _, p := range st.Payments {
		if p.ID == id {
			return p, nil
		}
	}
	return Payment{}, ErrPaymentNotFound
}

// checkParty lets only who pays, who receives or an administrator of the group record or undo a payment, so no
// member moves money out of another's account (RN-30).
func checkParty(ctx context.Context, q *store.Queries, groupID, userID, from, to int64) error {
	u, err := q.GetUserByID(ctx, userID)
	if err != nil {
		return ErrNotParty
	}
	if u.PersonID == from || u.PersonID == to {
		return nil
	}
	m, err := q.GetGroupMember(ctx, store.GetGroupMemberParams{GroupID: groupID, PersonID: u.PersonID})
	if err != nil || m.Role != "admin" {
		return ErrNotParty
	}
	return nil
}

// DeletePayment undoes a payment between members with its transfer, by who paid, who received or an administrator.
func (s *Service) DeletePayment(ctx context.Context, userID, id int64) error {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		p, err := q.GetSettlement(ctx, store.GetSettlementParams{ID: id, GroupID: groupID})
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPaymentNotFound
		}
		if err != nil {
			return err
		}
		if err := checkParty(ctx, q, groupID, userID, p.FromPersonID, p.ToPersonID); err != nil {
			return err
		}
		return q.DeleteTransfer(ctx, store.DeleteTransferParams{ID: p.TransferID, GroupID: groupID})
	})
}

// ErrItemNotFound tells the item is not an expense of the active group.
var ErrItemNotFound = problem.NotFound("member_split_item", "Gasto não encontrado neste grupo.")

// MemberSplit returns the division of one expense of the active group, nil when it follows the weights (RN-28).
func (s *Service) MemberSplit(ctx context.Context, kind membersplit.Kind, id int64) (*membersplit.Rule, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return nil, err
	}
	q := store.New(s.conn)
	found, err := q.SplitItemInGroup(ctx, store.SplitItemInGroupParams{Kind: string(kind), ItemID: id, GroupID: groupID})
	if err != nil {
		return nil, err
	}
	if found == 0 {
		return nil, ErrItemNotFound
	}
	return membersplit.Load(ctx, q, kind, id)
}
