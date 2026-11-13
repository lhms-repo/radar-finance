package server

import (
	"context"

	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/membersplit"
	"github.com/italogfx/radar-finance/internal/settle"
)

// GetSettlement returns the balances of the members, who pays whom, the payments and the expenses of the month (RF-84).
func (a *API) GetSettlement(ctx context.Context, req gen.GetSettlementRequestObject) (gen.GetSettlementResponseObject, error) {
	m, err := civil.ParseMonth(req.Params.Month)
	if err != nil {
		return nil, errBadMonth
	}
	st, err := a.d.Settle.Settlement(ctx, m)
	if err != nil {
		return nil, err
	}
	out := gen.Settlement{
		Members: make([]gen.SettlementMember, 0, len(st.Members)), Suggestions: make([]gen.SettlementSuggestion, 0, len(st.Suggestions)),
		Payments: make([]gen.SettlementPayment, 0, len(st.Payments)), Lines: make([]gen.SettlementLine, 0, len(st.Lines)),
	}
	if !st.From.IsZero() {
		from := apiDate(st.From)
		out.From = &from
	}
	for _, m := range st.Members {
		out.Members = append(out.Members, gen.SettlementMember{PersonId: m.PersonID, Name: m.Name, PaidCents: m.PaidCents, ShareCents: m.ShareCents, SentCents: m.SentCents, ReceivedCents: m.ReceivedCents, BalanceCents: m.BalanceCents})
	}
	for _, s := range st.Suggestions {
		out.Suggestions = append(out.Suggestions, gen.SettlementSuggestion{FromPersonId: s.FromPersonID, FromName: s.FromName, ToPersonId: s.ToPersonID, ToName: s.ToName, AmountCents: s.AmountCents})
	}
	for _, p := range st.Payments {
		out.Payments = append(out.Payments, toSettlementPayment(p))
	}
	for _, l := range st.Lines {
		line := gen.SettlementLine{Date: apiDate(l.Date), Kind: gen.SettlementLineKind(l.Kind), Description: l.Description, Source: l.Source, PayerName: l.PayerName, AmountCents: l.AmountCents}
		for _, sh := range l.Shares {
			line.Shares = append(line.Shares, struct {
				Cents    gen.Cents `json:"cents"`
				Name     string    `json:"name"`
				PersonId int64     `json:"personId"`
			}{Cents: sh.Cents, Name: sh.Name, PersonId: sh.PersonID})
		}
		out.Lines = append(out.Lines, line)
	}
	return gen.GetSettlement200JSONResponse(out), nil
}

// CreateSettlementPayment records a payment between members as a transfer between their accounts (RN-30).
func (a *API) CreateSettlementPayment(ctx context.Context, req gen.CreateSettlementPaymentRequestObject) (gen.CreateSettlementPaymentResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	b := req.Body
	p, err := a.d.Settle.Pay(ctx, auth.UserID(ctx), settle.PaymentInput{FromAccountID: b.FromAccountId, ToAccountID: b.ToAccountId, AmountCents: b.AmountCents, Date: fromDate(b.Date)})
	if err != nil {
		return nil, err
	}
	return gen.CreateSettlementPayment201JSONResponse(toSettlementPayment(p)), nil
}

// DeleteSettlementPayment undoes a payment between members with its transfer.
func (a *API) DeleteSettlementPayment(ctx context.Context, req gen.DeleteSettlementPaymentRequestObject) (gen.DeleteSettlementPaymentResponseObject, error) {
	if err := a.d.Settle.DeletePayment(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteSettlementPayment204Response{}, nil
}

// GetMemberSplit returns the division between members of one expense of the active group (RN-28).
func (a *API) GetMemberSplit(ctx context.Context, req gen.GetMemberSplitRequestObject) (gen.GetMemberSplitResponseObject, error) {
	r, err := a.d.Settle.MemberSplit(ctx, membersplit.Kind(req.Kind), req.Id)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return gen.GetMemberSplit200JSONResponse(gen.MemberSplit{Mode: gen.MemberSplitModeWeights}), nil
	}
	members := make([]gen.MemberShare, 0, len(r.Members))
	for _, m := range r.Members {
		share := m.ShareCents
		members = append(members, gen.MemberShare{PersonId: m.PersonID, ShareCents: &share})
	}
	return gen.GetMemberSplit200JSONResponse(gen.MemberSplit{Mode: gen.MemberSplitMode(r.Mode), Members: &members}), nil
}

// toSettlementPayment converts a payment between members to the API model.
func toSettlementPayment(p settle.Payment) gen.SettlementPayment {
	return gen.SettlementPayment{Id: p.ID, FromPersonId: p.FromPersonID, FromName: p.FromName, ToPersonId: p.ToPersonID, ToName: p.ToName, AmountCents: p.AmountCents, Date: apiDate(p.Date)}
}

// fromMemberSplit converts the division chosen in a form, nil when it follows the weights (RN-28).
func fromMemberSplit(m *gen.MemberSplit) *membersplit.Rule {
	if m == nil || m.Mode == gen.MemberSplitModeWeights {
		return nil
	}
	r := &membersplit.Rule{Mode: membersplit.Mode(m.Mode)}
	if m.Members != nil {
		for _, x := range *m.Members {
			var share int64
			if x.ShareCents != nil {
				share = *x.ShareCents
			}
			r.Members = append(r.Members, membersplit.Member{PersonID: x.PersonId, ShareCents: share})
		}
	}
	return r
}
