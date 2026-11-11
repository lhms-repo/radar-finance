package server

import (
	"context"

	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/bill"
	"github.com/italogfx/radar-finance/internal/civil"
)

// ListBills lists the fixed bills (RF-35).
func (a *API) ListBills(ctx context.Context, _ gen.ListBillsRequestObject) (gen.ListBillsResponseObject, error) {
	list, err := a.d.Bills.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListBills200JSONResponse, 0, len(list))
	for _, b := range list {
		out = append(out, toBill(b))
	}
	return out, nil
}

// CreateBill registers a fixed bill (RF-35).
func (a *API) CreateBill(ctx context.Context, req gen.CreateBillRequestObject) (gen.CreateBillResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	in, err := fromBillInput(*req.Body)
	if err != nil {
		return nil, err
	}
	b, err := a.d.Bills.Create(ctx, auth.UserID(ctx), in)
	if err != nil {
		return nil, err
	}
	return gen.CreateBill201JSONResponse(toBill(b)), nil
}

// UpdateBill replaces the fields of a bill.
func (a *API) UpdateBill(ctx context.Context, req gen.UpdateBillRequestObject) (gen.UpdateBillResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	in, err := fromBillInput(*req.Body)
	if err != nil {
		return nil, err
	}
	b, err := a.d.Bills.Update(ctx, req.Id, in)
	if err != nil {
		return nil, err
	}
	return gen.UpdateBill200JSONResponse(toBill(b)), nil
}

// DeleteBill removes a bill without payments.
func (a *API) DeleteBill(ctx context.Context, req gen.DeleteBillRequestObject) (gen.DeleteBillResponseObject, error) {
	if err := a.d.Bills.Delete(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteBill204Response{}, nil
}

// GetBillMonth returns the bills of the month (RF-36, RF-37, RN-14).
func (a *API) GetBillMonth(ctx context.Context, req gen.GetBillMonthRequestObject) (gen.GetBillMonthResponseObject, error) {
	m, err := civil.ParseMonth(req.Params.Month)
	if err != nil {
		return nil, errBadMonth
	}
	v, err := a.d.Bills.Month(ctx, m)
	if err != nil {
		return nil, err
	}
	out := gen.GetBillMonth200JSONResponse{
		Month: m.String(), TotalCents: v.TotalCents, PaidCents: v.PaidCents, PaidCount: v.PaidCount,
		Bills: make([]gen.BillOccurrence, 0, len(v.Bills)),
	}
	for _, o := range v.Bills {
		item := gen.BillOccurrence{
			BillId: o.BillID, Name: o.Name, CategoryId: o.CategoryID, CategoryName: o.CategoryName,
			ParentCategoryName: optional(o.ParentCategoryName), CategoryIcon: optional(o.CategoryIcon), AccountId: o.AccountID,
			AccountName: o.AccountName, DueOn: apiDate(o.DueOn), ExpectedCents: o.ExpectedCents, ActualCents: o.ActualCents,
			AmountCents: o.AmountCents, Paid: o.Paid, EntryId: o.EntryID, PaidOn: apiDatePtr(o.PaidOn), PaidFrom: optional(o.PaidFrom),
		}
		if o.Paid {
			paid := o.PaidCents
			item.PaidCents = &paid
		}
		out.Bills = append(out.Bills, item)
	}
	return out, nil
}

// SetBillActual informs the real amount of a month (RF-36).
func (a *API) SetBillActual(ctx context.Context, req gen.SetBillActualRequestObject) (gen.SetBillActualResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	m, err := civil.ParseMonth(req.Month)
	if err != nil {
		return nil, errBadMonth
	}
	if err := a.d.Bills.SetActual(ctx, req.Id, m, req.Body.ActualCents); err != nil {
		return nil, err
	}
	return gen.SetBillActual204Response{}, nil
}

// PayBill pays the bill of a month (RF-37).
func (a *API) PayBill(ctx context.Context, req gen.PayBillRequestObject) (gen.PayBillResponseObject, error) {
	m, err := civil.ParseMonth(req.Month)
	if err != nil {
		return nil, errBadMonth
	}
	p := bill.Payment{BillID: req.Id, Month: m}
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
	}
	e, err := a.d.Bills.Pay(ctx, auth.UserID(ctx), p)
	if err != nil {
		return nil, err
	}
	return gen.PayBill201JSONResponse(toEntry(e)), nil
}

// fromBillInput converts the API bill input, parsing the months.
func fromBillInput(b gen.BillInput) (bill.Input, error) {
	start, err := civil.ParseMonth(b.StartMonth)
	if err != nil {
		return bill.Input{}, errBadMonth
	}
	end, err := optionalMonth(b.EndMonth)
	if err != nil {
		return bill.Input{}, err
	}
	return bill.Input{
		Name: b.Name, CategoryID: b.CategoryId, AccountID: b.AccountId, DueDay: b.DueDay, ExpectedCents: b.ExpectedCents,
		StartMonth: start, EndMonth: end,
	}, nil
}

// toBill converts a bill to the API model.
func toBill(b bill.Bill) gen.Bill {
	out := gen.Bill{
		Id: b.ID, Name: b.Name, CategoryId: b.CategoryID, CategoryName: b.CategoryName, ParentCategoryName: optional(b.ParentCategoryName),
		CategoryIcon: optional(b.CategoryIcon), AccountId: b.AccountID, AccountName: b.AccountName, DueDay: b.DueDay,
		ExpectedCents: b.ExpectedCents, StartMonth: b.StartMonth.String(),
	}
	if b.EndMonth != nil {
		end := b.EndMonth.String()
		out.EndMonth = &end
	}
	return out
}
