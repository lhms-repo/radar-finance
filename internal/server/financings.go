package server

import (
	"context"

	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/financing"
)

// ListFinancings lists the financings (RF-39, RF-41).
func (a *API) ListFinancings(ctx context.Context, _ gen.ListFinancingsRequestObject) (gen.ListFinancingsResponseObject, error) {
	list, err := a.d.Financings.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListFinancings200JSONResponse, 0, len(list))
	for _, f := range list {
		out = append(out, toFinancing(f))
	}
	return out, nil
}

// CreateFinancing registers a financing (RF-39).
func (a *API) CreateFinancing(ctx context.Context, req gen.CreateFinancingRequestObject) (gen.CreateFinancingResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	b := req.Body
	next, err := civil.ParseMonth(b.NextMonth)
	if err != nil {
		return nil, errBadMonth
	}
	f, err := a.d.Financings.Create(ctx, auth.UserID(ctx), financing.Input{
		Name: b.Name, CategoryID: b.CategoryId, AccountID: b.AccountId, DueDay: b.DueDay, InstallmentCents: b.InstallmentCents,
		TotalInstallments: b.TotalInstallments, PaidInstallments: b.PaidInstallments, BalanceCents: b.BalanceCents, NextMonth: next,
	})
	if err != nil {
		return nil, err
	}
	return gen.CreateFinancing201JSONResponse(toFinancing(f)), nil
}

// UpdateFinancing edits the descriptive fields of a financing.
func (a *API) UpdateFinancing(ctx context.Context, req gen.UpdateFinancingRequestObject) (gen.UpdateFinancingResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	b := req.Body
	f, err := a.d.Financings.Update(ctx, req.Id, financing.Changes{Name: b.Name, CategoryID: b.CategoryId, AccountID: b.AccountId, DueDay: b.DueDay})
	if err != nil {
		return nil, err
	}
	return gen.UpdateFinancing200JSONResponse(toFinancing(f)), nil
}

// DeleteFinancing removes a financing without payments.
func (a *API) DeleteFinancing(ctx context.Context, req gen.DeleteFinancingRequestObject) (gen.DeleteFinancingResponseObject, error) {
	if err := a.d.Financings.Delete(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteFinancing204Response{}, nil
}

// PayFinancing pays the installment of a month (RF-41, RN-16).
func (a *API) PayFinancing(ctx context.Context, req gen.PayFinancingRequestObject) (gen.PayFinancingResponseObject, error) {
	m, err := civil.ParseMonth(req.Month)
	if err != nil {
		return nil, errBadMonth
	}
	p := financing.Payment{FinancingID: req.Id, Month: m}
	if b := req.Body; b != nil {
		if b.AmountCents != nil {
			p.AmountCents = *b.AmountCents
		}
		if b.AccountId != nil {
			p.AccountID = *b.AccountId
		}
		if b.Date != nil {
			p.Date = fromDate(*b.Date)
		}
		p.BalanceAfterCents = b.BalanceAfterCents
	}
	e, err := a.d.Financings.Pay(ctx, auth.UserID(ctx), p)
	if err != nil {
		return nil, err
	}
	return gen.PayFinancing201JSONResponse(toEntry(e)), nil
}

// CorrectFinancing sets the balance and the installment by hand (RF-42).
func (a *API) CorrectFinancing(ctx context.Context, req gen.CorrectFinancingRequestObject) (gen.CorrectFinancingResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	f, err := a.d.Financings.Correct(ctx, auth.UserID(ctx), financing.Correction{
		FinancingID: req.Id, BalanceCents: req.Body.BalanceCents, InstallmentCents: req.Body.InstallmentCents,
	})
	if err != nil {
		return nil, err
	}
	return gen.CorrectFinancing200JSONResponse(toFinancing(f)), nil
}

// GetAgenda returns everything due in the month (RF-38, RF-40).
func (a *API) GetAgenda(ctx context.Context, req gen.GetAgendaRequestObject) (gen.GetAgendaResponseObject, error) {
	m, err := civil.ParseMonth(req.Params.Month)
	if err != nil {
		return nil, errBadMonth
	}
	v, err := a.d.Agenda.Month(ctx, m)
	if err != nil {
		return nil, err
	}
	out := gen.GetAgenda200JSONResponse{
		Month: m.String(), TotalCents: v.TotalCents, PaidCents: v.PaidCents, Count: v.Count, PaidCount: v.PaidCount,
		Items: make([]gen.AgendaItem, 0, len(v.Items)), Days: make([]gen.AgendaDay, 0, len(v.Days)),
	}
	for _, it := range v.Items {
		item := gen.AgendaItem{
			Kind: gen.AgendaKind(it.Kind), RefId: it.RefID, Name: it.Name, Icon: optional(it.Icon), AccountName: optional(it.AccountName),
			DueOn: apiDate(it.DueOn), AmountCents: it.AmountCents, Paid: it.Paid, EntryId: it.EntryID, PaidOn: apiDatePtr(it.PaidOn),
			ActualCents: it.ActualCents,
		}
		if it.AccountID != 0 {
			id := it.AccountID
			item.AccountId = &id
		}
		if it.ExpectedCents != 0 {
			expected := it.ExpectedCents
			item.ExpectedCents = &expected
		}
		if it.Number != 0 {
			number, total, suggested := it.Number, it.Total, it.SuggestedBalanceCents
			item.Number, item.Total, item.SuggestedBalanceCents = &number, &total, &suggested
		}
		if it.InvoiceStatus != "" {
			status, payable := gen.InvoiceStatus(it.InvoiceStatus), it.Payable
			item.InvoiceStatus, item.Payable = &status, &payable
		}
		out.Items = append(out.Items, item)
	}
	for _, d := range v.Days {
		out.Days = append(out.Days, gen.AgendaDay{Day: d.Day, Count: d.Count, PaidCount: d.PaidCount})
	}
	return out, nil
}

// toFinancing converts a financing to the API model.
func toFinancing(f financing.Financing) gen.Financing {
	out := gen.Financing{
		Id: f.ID, Name: f.Name, CategoryId: f.CategoryID, CategoryName: f.CategoryName, CategoryIcon: optional(f.CategoryIcon),
		AccountId: f.AccountID, AccountName: f.AccountName, DueDay: f.DueDay, InstallmentCents: f.InstallmentCents,
		TotalInstallments: f.TotalInstallments, PaidInstallments: f.PaidInstallments, RemainingInstallments: f.RemainingInstallments,
		BalanceCents: f.BalanceCents, StartMonth: f.StartMonth.String(), Done: f.Done,
	}
	if f.NextMonth != nil {
		next := f.NextMonth.String()
		out.NextMonth = &next
	}
	return out
}
