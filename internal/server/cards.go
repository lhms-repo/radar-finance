package server

import (
	"context"

	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/problem"
)

// errBadCount is returned for a forecast size outside 1 to 24.
var errBadCount = problem.Invalid("invalid_count", "Informe de 1 a 24 faturas.")

// ListCards lists the cards with limit use (RF-14, RF-20).
func (a *API) ListCards(ctx context.Context, _ gen.ListCardsRequestObject) (gen.ListCardsResponseObject, error) {
	list, err := a.d.Cards.ListCards(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListCards200JSONResponse, 0, len(list))
	for _, c := range list {
		out = append(out, toCard(c))
	}
	return out, nil
}

// GetCard returns one card (RF-20).
func (a *API) GetCard(ctx context.Context, req gen.GetCardRequestObject) (gen.GetCardResponseObject, error) {
	c, err := a.d.Cards.GetCard(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.GetCard200JSONResponse(toCard(c)), nil
}

// CreateCard registers a card (RF-14).
func (a *API) CreateCard(ctx context.Context, req gen.CreateCardRequestObject) (gen.CreateCardResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	b := req.Body
	c, err := a.d.Cards.CreateCard(ctx, auth.UserID(ctx), card.CardInput{
		Name: b.Name, PersonID: b.PersonId, LimitCents: b.LimitCents, ClosingDay: b.ClosingDay, DueDay: b.DueDay,
		LastDigits: deref(b.LastDigits), Color: string(b.Color), Finish: string(b.Finish),
	})
	if err != nil {
		return nil, err
	}
	return gen.CreateCard201JSONResponse(toCard(c)), nil
}

// UpdateCard replaces the fields of a card.
func (a *API) UpdateCard(ctx context.Context, req gen.UpdateCardRequestObject) (gen.UpdateCardResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	b := req.Body
	c, err := a.d.Cards.UpdateCard(ctx, req.Id, card.CardInput{
		Name: b.Name, PersonID: b.PersonId, LimitCents: b.LimitCents, ClosingDay: b.ClosingDay, DueDay: b.DueDay,
		LastDigits: deref(b.LastDigits), Color: string(b.Color), Finish: string(b.Finish),
	}, b.Active)
	if err != nil {
		return nil, err
	}
	return gen.UpdateCard200JSONResponse(toCard(c)), nil
}

// DeleteCard removes an unused card.
func (a *API) DeleteCard(ctx context.Context, req gen.DeleteCardRequestObject) (gen.DeleteCardResponseObject, error) {
	if err := a.d.Cards.DeleteCard(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteCard204Response{}, nil
}

// GetInvoice returns the invoice of the due month (RF-19, RN-01).
func (a *API) GetInvoice(ctx context.Context, req gen.GetInvoiceRequestObject) (gen.GetInvoiceResponseObject, error) {
	m, err := civil.ParseMonth(req.Month)
	if err != nil {
		return nil, errBadMonth
	}
	inv, err := a.d.Cards.Invoice(ctx, req.Id, m)
	if err != nil {
		return nil, err
	}
	return gen.GetInvoice200JSONResponse(toInvoice(inv)), nil
}

// PayInvoice pays the whole invoice from an account (RF-21, RN-07).
func (a *API) PayInvoice(ctx context.Context, req gen.PayInvoiceRequestObject) (gen.PayInvoiceResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	m, err := civil.ParseMonth(req.Month)
	if err != nil {
		return nil, errBadMonth
	}
	inv, err := a.d.Cards.Pay(ctx, auth.UserID(ctx), card.Payment{CardID: req.Id, Month: m, AccountID: req.Body.AccountId, Date: fromDate(req.Body.Date)})
	if err != nil {
		return nil, err
	}
	return gen.PayInvoice201JSONResponse(toInvoice(inv)), nil
}

// GetCardForecast returns the next invoice totals (RF-23).
func (a *API) GetCardForecast(ctx context.Context, req gen.GetCardForecastRequestObject) (gen.GetCardForecastResponseObject, error) {
	n := 12
	if req.Params.Count != nil {
		n = *req.Params.Count
	}
	if n < 1 || n > 24 {
		return nil, errBadCount
	}
	list, err := a.d.Cards.Forecast(ctx, req.Id, n)
	if err != nil {
		return nil, err
	}
	out := make(gen.GetCardForecast200JSONResponse, 0, len(list))
	for _, s := range list {
		out = append(out, gen.InvoiceSummary{Month: s.Month.String(), DueOn: apiDate(s.DueOn), Status: gen.InvoiceStatus(s.Status), TotalCents: s.TotalCents})
	}
	return out, nil
}

// AddPurchase registers a purchase (RF-15, RF-16).
func (a *API) AddPurchase(ctx context.Context, req gen.AddPurchaseRequestObject) (gen.AddPurchaseResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	p, err := a.d.Cards.AddPurchase(ctx, auth.UserID(ctx), fromPurchaseInput(req.Id, *req.Body))
	if err != nil {
		return nil, err
	}
	return gen.AddPurchase201JSONResponse(toPurchase(p)), nil
}

// UpdatePurchase replaces a purchase and its installments.
func (a *API) UpdatePurchase(ctx context.Context, req gen.UpdatePurchaseRequestObject) (gen.UpdatePurchaseResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	p, err := a.d.Cards.UpdatePurchase(ctx, req.Id, fromPurchaseInput(0, *req.Body))
	if err != nil {
		return nil, err
	}
	return gen.UpdatePurchase200JSONResponse(toPurchase(p)), nil
}

// DeletePurchase removes a purchase or credit.
func (a *API) DeletePurchase(ctx context.Context, req gen.DeletePurchaseRequestObject) (gen.DeletePurchaseResponseObject, error) {
	if err := a.d.Cards.DeletePurchase(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeletePurchase204Response{}, nil
}

// AddCredit registers a refund or credit (RF-22).
func (a *API) AddCredit(ctx context.Context, req gen.AddCreditRequestObject) (gen.AddCreditResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	b := req.Body
	p, err := a.d.Cards.AddCredit(ctx, auth.UserID(ctx), card.CreditInput{
		CardID: req.Id, Date: fromDate(b.Date), Merchant: b.Merchant, Description: deref(b.Description), CategoryID: b.CategoryId, AmountCents: b.AmountCents,
	})
	if err != nil {
		return nil, err
	}
	return gen.AddCredit201JSONResponse(toPurchase(p)), nil
}

// ListRecurrences lists the recurrences of a card (RF-17).
func (a *API) ListRecurrences(ctx context.Context, req gen.ListRecurrencesRequestObject) (gen.ListRecurrencesResponseObject, error) {
	list, err := a.d.Cards.Recurrences(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListRecurrences200JSONResponse, 0, len(list))
	for _, r := range list {
		out = append(out, toRecurrence(r))
	}
	return out, nil
}

// AddRecurrence registers a recurrence (RF-17).
func (a *API) AddRecurrence(ctx context.Context, req gen.AddRecurrenceRequestObject) (gen.AddRecurrenceResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	b := req.Body
	end, err := optionalMonth(b.EndMonth)
	if err != nil {
		return nil, err
	}
	r, err := a.d.Cards.AddRecurrence(ctx, auth.UserID(ctx), card.RecurrenceInput{
		CardID: req.Id, Description: b.Description, CategoryID: b.CategoryId, AmountCents: b.AmountCents, StartDate: fromDate(b.StartDate), EndMonth: end,
	})
	if err != nil {
		return nil, err
	}
	return gen.AddRecurrence201JSONResponse(toRecurrence(r)), nil
}

// UpdateRecurrence edits a recurrence; a new price applies from the current invoice.
func (a *API) UpdateRecurrence(ctx context.Context, req gen.UpdateRecurrenceRequestObject) (gen.UpdateRecurrenceResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	b := req.Body
	end, err := optionalMonth(b.EndMonth)
	if err != nil {
		return nil, err
	}
	r, err := a.d.Cards.UpdateRecurrence(ctx, req.Id, card.RecurrenceChanges{
		Description: b.Description, CategoryID: b.CategoryId, AmountCents: b.AmountCents, EndMonth: end,
	})
	if err != nil {
		return nil, err
	}
	return gen.UpdateRecurrence200JSONResponse(toRecurrence(r)), nil
}

// DeleteRecurrence removes a recurrence that is not on a paid invoice.
func (a *API) DeleteRecurrence(ctx context.Context, req gen.DeleteRecurrenceRequestObject) (gen.DeleteRecurrenceResponseObject, error) {
	if err := a.d.Cards.DeleteRecurrence(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteRecurrence204Response{}, nil
}

// fromPurchaseInput converts the API purchase input.
func fromPurchaseInput(cardID int64, b gen.PurchaseInput) card.PurchaseInput {
	return card.PurchaseInput{
		CardID: cardID, Date: fromDate(b.Date), Merchant: b.Merchant, Description: deref(b.Description), CategoryID: b.CategoryId,
		BuyerPersonID: b.BuyerPersonId, AmountCents: b.AmountCents, Installments: b.Installments,
		PerInstallment: b.PerInstallment != nil && *b.PerInstallment,
	}
}

// optionalMonth parses an optional YYYY-MM month.
func optionalMonth(s *string) (*civil.Month, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	m, err := civil.ParseMonth(*s)
	if err != nil {
		return nil, errBadMonth
	}
	return &m, nil
}

// toCard converts a card to the API model.
func toCard(c card.Card) gen.Card {
	return gen.Card{
		Id: c.ID, Name: c.Name, PersonId: c.PersonID, PersonName: c.PersonName, LimitCents: c.LimitCents,
		ClosingDay: c.ClosingDay, DueDay: c.DueDay, LastDigits: optional(c.LastDigits), Color: gen.CardColor(c.Color),
		Finish: gen.CardFinish(c.Finish), Active: c.Active, LimitUsedCents: c.LimitUsedCents, LimitFreeCents: c.LimitFreeCents,
		CurrentInvoice: c.CurrentInvoice.String(),
	}
}

// toInvoice converts an invoice to the API model.
func toInvoice(inv card.Invoice) gen.Invoice {
	out := gen.Invoice{
		CardId: inv.CardID, Month: inv.Month.String(), ClosingOn: apiDate(inv.ClosingOn), DueOn: apiDate(inv.DueOn),
		Status: gen.InvoiceStatus(inv.Status), TotalCents: inv.TotalCents, ThirdPartyCents: inv.ThirdPartyCents,
		ReceivedCents: inv.ReceivedCents, PendingCents: inv.PendingCents, NetCents: inv.NetCents,
		Items: make([]gen.InvoiceItem, 0, len(inv.Items)), PaymentOutdated: inv.PaymentOutdated,
	}
	for _, it := range inv.Items {
		item := gen.InvoiceItem{
			Kind: gen.InvoiceItemKind(it.Kind), PurchaseId: it.PurchaseID, RecurrenceId: it.RecurrenceID, Date: apiDatePtr(it.Date),
			Merchant: it.Merchant, Description: it.Description, CategoryId: it.CategoryID, CategoryName: optional(it.CategoryName),
			ParentCategoryName: optional(it.ParentCategoryName), CategoryIcon: optional(it.CategoryIcon), Number: it.Number,
			Count: it.Count, AmountCents: it.AmountCents, BuyerPersonId: it.BuyerPersonID, BuyerName: optional(it.BuyerName), CreatedByName: optional(it.CreatedByName),
		}
		if it.PurchaseTotalCents != 0 {
			total := it.PurchaseTotalCents
			item.PurchaseTotalCents = &total
		}
		out.Items = append(out.Items, item)
	}
	if p := inv.Payment; p != nil {
		out.Payment = &gen.InvoicePayment{EntryId: p.EntryID, AccountId: p.AccountID, AccountName: p.AccountName, PaidOn: apiDate(p.PaidOn), AmountCents: p.AmountCents}
	}
	return out
}

// toPurchase converts a purchase to the API model.
func toPurchase(p card.Purchase) gen.Purchase {
	return gen.Purchase{
		Id: p.ID, CardId: p.CardID, Kind: gen.PurchaseKind(p.Kind), Date: apiDate(p.Date), Merchant: p.Merchant, Description: p.Description,
		CategoryId: p.CategoryID, BuyerPersonId: p.BuyerPersonID, TotalCents: p.TotalCents, Installments: p.Installments,
		FirstInvoice: p.FirstInvoice.String(),
	}
}

// toRecurrence converts a recurrence to the API model.
func toRecurrence(r card.Recurrence) gen.Recurrence {
	out := gen.Recurrence{
		Id: r.ID, CardId: r.CardID, Description: r.Description, CategoryId: r.CategoryID, CategoryName: r.CategoryName,
		ParentCategoryName: optional(r.ParentCategoryName), CategoryIcon: optional(r.CategoryIcon), AmountCents: r.AmountCents,
		StartMonth: r.StartMonth.String(),
	}
	if r.EndMonth != nil {
		end := r.EndMonth.String()
		out.EndMonth = &end
	}
	return out
}
