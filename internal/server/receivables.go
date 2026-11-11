package server

import (
	"context"

	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/receivable"
)

// ListReceivables returns what other people owe, by person (RF-28).
func (a *API) ListReceivables(ctx context.Context, _ gen.ListReceivablesRequestObject) (gen.ListReceivablesResponseObject, error) {
	people, err := a.d.Receivables.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListReceivables200JSONResponse, 0, len(people))
	for _, p := range people {
		person := gen.ReceivablePerson{PartyId: p.PartyID, Name: p.Name, PendingCents: p.PendingCents, Items: make([]gen.ReceivableItem, 0, len(p.Items))}
		for _, it := range p.Items {
			person.Items = append(person.Items, gen.ReceivableItem{
				Id: it.ID, PurchaseId: it.PurchaseID, Merchant: it.Merchant, PurchaseDate: apiDate(it.PurchaseDate), CardName: it.CardName,
				Number: it.Number, Count: it.Count, InvoiceMonth: it.InvoiceMonth.String(), DueOn: apiDate(it.DueOn), AmountCents: it.AmountCents,
				Received: it.Received, EntryId: it.EntryID, ReceivedOn: apiDatePtr(it.ReceivedOn), ReceivedIn: optional(it.ReceivedIn),
			})
		}
		out = append(out, person)
	}
	return out, nil
}

// MarkReceived records shares paid back into an account (RF-26, RF-30, RN-06).
func (a *API) MarkReceived(ctx context.Context, req gen.MarkReceivedRequestObject) (gen.MarkReceivedResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	e, err := a.d.Receivables.MarkReceived(ctx, auth.UserID(ctx), receivable.Receipt{
		ItemIDs: req.Body.ItemIds, AccountID: req.Body.AccountId, Date: fromDate(req.Body.Date),
	})
	if err != nil {
		return nil, err
	}
	return gen.MarkReceived201JSONResponse(toEntry(e)), nil
}

// GetChargeMessage returns the charge message for a person (RF-29).
func (a *API) GetChargeMessage(ctx context.Context, req gen.GetChargeMessageRequestObject) (gen.GetChargeMessageResponseObject, error) {
	text, err := a.d.Receivables.Message(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.GetChargeMessage200JSONResponse{Text: text}, nil
}
