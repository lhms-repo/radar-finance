package server

import (
	"context"

	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/savings"
)

// GetSavingsMonth returns the boxes as seen in the month (RF-45, RF-48).
func (a *API) GetSavingsMonth(ctx context.Context, req gen.GetSavingsMonthRequestObject) (gen.GetSavingsMonthResponseObject, error) {
	m, err := civil.ParseMonth(req.Params.Month)
	if err != nil {
		return nil, errBadMonth
	}
	v, err := a.d.Savings.Month(ctx, m)
	if err != nil {
		return nil, err
	}
	out := gen.GetSavingsMonth200JSONResponse{Month: m.String(), BaseCents: v.BaseCents, Boxes: make([]gen.SavingsBox, 0, len(v.Boxes))}
	for _, b := range v.Boxes {
		out.Boxes = append(out.Boxes, toSavingsBox(b))
	}
	return out, nil
}

// CreateSavingsBox creates a box (RF-43).
func (a *API) CreateSavingsBox(ctx context.Context, req gen.CreateSavingsBoxRequestObject) (gen.CreateSavingsBoxResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	b := req.Body
	in := savings.Input{Name: b.Name, TargetCents: b.TargetCents, PercentBP: b.PercentBp}
	if b.InitialBalanceCents != nil {
		in.InitialBalanceCents = *b.InitialBalanceCents
	}
	box, err := a.d.Savings.Create(ctx, auth.UserID(ctx), in)
	if err != nil {
		return nil, err
	}
	return gen.CreateSavingsBox201JSONResponse(toSavingsBox(box)), nil
}

// UpdateSavingsBox edits a box (RF-44).
func (a *API) UpdateSavingsBox(ctx context.Context, req gen.UpdateSavingsBoxRequestObject) (gen.UpdateSavingsBoxResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	box, err := a.d.Savings.Update(ctx, req.Id, savings.Changes{Name: req.Body.Name, TargetCents: req.Body.TargetCents, PercentBP: req.Body.PercentBp})
	if err != nil {
		return nil, err
	}
	return gen.UpdateSavingsBox200JSONResponse(toSavingsBox(box)), nil
}

// DeleteSavingsBox removes a box without moves.
func (a *API) DeleteSavingsBox(ctx context.Context, req gen.DeleteSavingsBoxRequestObject) (gen.DeleteSavingsBoxResponseObject, error) {
	if err := a.d.Savings.Delete(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteSavingsBox204Response{}, nil
}

// DepositSavings confirms a deposit from an account (RF-46).
func (a *API) DepositSavings(ctx context.Context, req gen.DepositSavingsRequestObject) (gen.DepositSavingsResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	m, err := civil.ParseMonth(req.Body.Month)
	if err != nil {
		return nil, errBadMonth
	}
	e, err := a.d.Savings.Deposit(ctx, auth.UserID(ctx), savings.Move{
		BoxID: req.Id, Month: m, AmountCents: req.Body.AmountCents, AccountID: req.Body.AccountId, Date: fromDate(req.Body.Date),
	})
	if err != nil {
		return nil, err
	}
	return gen.DepositSavings201JSONResponse(toEntry(e)), nil
}

// WithdrawSavings takes money out of a box (RF-47).
func (a *API) WithdrawSavings(ctx context.Context, req gen.WithdrawSavingsRequestObject) (gen.WithdrawSavingsResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	e, err := a.d.Savings.Withdraw(ctx, auth.UserID(ctx), savings.Move{
		BoxID: req.Id, AmountCents: req.Body.AmountCents, AccountID: req.Body.AccountId, Date: fromDate(req.Body.Date), Reason: req.Body.Reason,
	})
	if err != nil {
		return nil, err
	}
	return gen.WithdrawSavings201JSONResponse(toEntry(e)), nil
}

// toSavingsBox converts a box to the API model.
func toSavingsBox(b savings.Box) gen.SavingsBox {
	out := gen.SavingsBox{
		Id: b.ID, Name: b.Name, TargetCents: b.TargetCents, PercentBp: b.PercentBP, BalanceCents: b.BalanceCents, ProgressBp: b.ProgressBP,
		Done: b.Done, SuggestedCents: b.SuggestedCents, DepositedCents: b.DepositedCents, Moves: make([]gen.SavingsMove, 0, len(b.Moves)),
	}
	if b.EstimatedMonth != nil {
		m := b.EstimatedMonth.String()
		out.EstimatedMonth = &m
	}
	for _, mv := range b.Moves {
		out.Moves = append(out.Moves, gen.SavingsMove{
			Id: mv.ID, Kind: gen.SavingsMoveKind(mv.Kind), Month: mv.Month.String(), Date: apiDate(mv.Date), AmountCents: mv.AmountCents,
			Reason: mv.Reason, AccountName: mv.AccountName, EntryId: mv.EntryID, CreatedByName: optional(mv.CreatedByName),
		})
	}
	return out
}
