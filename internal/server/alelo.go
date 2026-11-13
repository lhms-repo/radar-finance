package server

import (
	"context"

	"github.com/italogfx/radar-finance/internal/alelo"
	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/civil"
)

// ListAlelo lists the Alelo cards (RF-31, RF-34).
func (a *API) ListAlelo(ctx context.Context, _ gen.ListAleloRequestObject) (gen.ListAleloResponseObject, error) {
	list, err := a.d.Alelo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListAlelo200JSONResponse, 0, len(list))
	for _, c := range list {
		out = append(out, toAleloCard(c))
	}
	return out, nil
}

// CreateAlelo registers an Alelo card (RF-31).
func (a *API) CreateAlelo(ctx context.Context, req gen.CreateAleloRequestObject) (gen.CreateAleloResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	b := req.Body
	c, err := a.d.Alelo.Create(ctx, auth.UserID(ctx), alelo.Input{
		Name: b.Name, PersonID: b.PersonId, MonthlyCreditCents: b.MonthlyCreditCents, CreditDay: b.CreditDay, InitialBalanceCents: b.InitialBalanceCents, Style: string(b.Style),
	})
	if err != nil {
		return nil, err
	}
	return gen.CreateAlelo201JSONResponse(toAleloCard(c)), nil
}

// UpdateAlelo edits an Alelo card.
func (a *API) UpdateAlelo(ctx context.Context, req gen.UpdateAleloRequestObject) (gen.UpdateAleloResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	b := req.Body
	c, err := a.d.Alelo.Update(ctx, req.Id, alelo.Changes{
		Name: b.Name, PersonID: b.PersonId, MonthlyCreditCents: b.MonthlyCreditCents, CreditDay: b.CreditDay, Style: string(b.Style), Active: b.Active,
	})
	if err != nil {
		return nil, err
	}
	return gen.UpdateAlelo200JSONResponse(toAleloCard(c)), nil
}

// DeleteAlelo removes an unused Alelo card.
func (a *API) DeleteAlelo(ctx context.Context, req gen.DeleteAleloRequestObject) (gen.DeleteAleloResponseObject, error) {
	if err := a.d.Alelo.Delete(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteAlelo204Response{}, nil
}

// GetAleloMonth returns the month of a card (RN-15).
func (a *API) GetAleloMonth(ctx context.Context, req gen.GetAleloMonthRequestObject) (gen.GetAleloMonthResponseObject, error) {
	m, err := civil.ParseMonth(req.Month)
	if err != nil {
		return nil, errBadMonth
	}
	v, err := a.d.Alelo.Month(ctx, req.Id, m)
	if err != nil {
		return nil, err
	}
	return gen.GetAleloMonth200JSONResponse(toAleloMonth(v)), nil
}

// ConfirmAleloCredit confirms the credit of a month (RF-32).
func (a *API) ConfirmAleloCredit(ctx context.Context, req gen.ConfirmAleloCreditRequestObject) (gen.ConfirmAleloCreditResponseObject, error) {
	m, err := civil.ParseMonth(req.Month)
	if err != nil {
		return nil, errBadMonth
	}
	cr := alelo.Credit{AleloID: req.Id, Month: m}
	if b := req.Body; b != nil {
		if b.AmountCents != nil {
			cr.AmountCents = *b.AmountCents
		}
		if b.Date != nil {
			cr.Date = fromDate(*b.Date)
		}
	}
	v, err := a.d.Alelo.ConfirmCredit(ctx, auth.UserID(ctx), cr)
	if err != nil {
		return nil, err
	}
	return gen.ConfirmAleloCredit201JSONResponse(toAleloMonth(v)), nil
}

// UndoAleloCredit undoes the confirmation of a month.
func (a *API) UndoAleloCredit(ctx context.Context, req gen.UndoAleloCreditRequestObject) (gen.UndoAleloCreditResponseObject, error) {
	m, err := civil.ParseMonth(req.Month)
	if err != nil {
		return nil, errBadMonth
	}
	if err := a.d.Alelo.UndoCredit(ctx, req.Id, m); err != nil {
		return nil, err
	}
	return gen.UndoAleloCredit204Response{}, nil
}

// AddAleloPurchase records a purchase filed as groceries (RF-33, RN-15).
func (a *API) AddAleloPurchase(ctx context.Context, req gen.AddAleloPurchaseRequestObject) (gen.AddAleloPurchaseResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	p, err := a.d.Alelo.AddPurchase(ctx, auth.UserID(ctx), alelo.Purchase{
		AleloID: req.Id, Date: fromDate(req.Body.Date), Description: req.Body.Description, AmountCents: req.Body.AmountCents,
		Members: fromMemberSplit(req.Body.MemberSplit),
	})
	if err != nil {
		return nil, err
	}
	return gen.AddAleloPurchase201JSONResponse(toAleloPurchase(p)), nil
}

// DeleteAleloPurchase removes a purchase.
func (a *API) DeleteAleloPurchase(ctx context.Context, req gen.DeleteAleloPurchaseRequestObject) (gen.DeleteAleloPurchaseResponseObject, error) {
	if err := a.d.Alelo.DeletePurchase(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteAleloPurchase204Response{}, nil
}

// toAleloCard converts a card to the API model.
func toAleloCard(c alelo.Card) gen.AleloCard {
	out := gen.AleloCard{
		Id: c.ID, Name: c.Name, PersonId: c.PersonID, PersonName: c.PersonName, MonthlyCreditCents: c.MonthlyCreditCents,
		CreditDay: c.CreditDay, StartMonth: c.StartMonth.String(), Style: gen.AleloStyle(c.Style), Active: c.Active, Owned: c.Owned,
	}
	// The real balance tells the purchases of every space, so only the holder reads it (RF-83).
	if c.Owned {
		balance := c.BalanceCents
		out.BalanceCents = &balance
	}
	return out
}

// toAleloMonth converts the month of a card to the API model.
func toAleloMonth(v alelo.MonthView) gen.AleloMonth {
	out := gen.AleloMonth{
		Month: v.Month.String(), HasCredit: v.HasCredit, Full: v.Full, CreditConfirmed: v.Credit.Confirmed, CreditCents: v.Credit.AmountCents,
		CreditedOn: apiDatePtr(v.Credit.CreditedOn), SpentCents: v.SpentCents, Purchases: make([]gen.AleloPurchase, 0, len(v.Purchases)),
	}
	// Balances and the expected credit belong to the holder, in her personal space (RF-83).
	if v.Full {
		opening, closing, expected, on := v.OpeningCents, v.ClosingCents, v.Credit.ExpectedCents, apiDate(v.Credit.ExpectedOn)
		out.OpeningCents, out.ClosingCents, out.ExpectedCreditCents, out.ExpectedCreditOn = &opening, &closing, &expected, &on
	}
	for _, p := range v.Purchases {
		out.Purchases = append(out.Purchases, toAleloPurchase(p))
	}
	return out
}

// toAleloPurchase converts a purchase to the API model.
func toAleloPurchase(p alelo.PurchaseItem) gen.AleloPurchase {
	return gen.AleloPurchase{
		Id: p.ID, Date: apiDate(p.Date), Description: p.Description, AmountCents: p.AmountCents, CreatedByName: optional(p.CreatedByName),
		GroupName: optional(p.GroupName), GroupId: optionalID(p.GroupID),
	}
}
