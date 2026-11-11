package server

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/income"
	"github.com/italogfx/radar-finance/internal/ledger"
)

// ListIncomes lists the registered incomes (RF-09, RF-11, RF-12).
func (a *API) ListIncomes(ctx context.Context, _ gen.ListIncomesRequestObject) (gen.ListIncomesResponseObject, error) {
	list, err := a.d.Incomes.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListIncomes200JSONResponse, 0, len(list))
	for _, i := range list {
		out = append(out, toIncome(i))
	}
	return out, nil
}

// CreateIncome registers an income (RF-09, RF-11).
func (a *API) CreateIncome(ctx context.Context, req gen.CreateIncomeRequestObject) (gen.CreateIncomeResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	in, err := fromIncomeInput(*req.Body)
	if err != nil {
		return nil, err
	}
	inc, err := a.d.Incomes.Create(ctx, auth.UserID(ctx), in)
	if err != nil {
		return nil, err
	}
	return gen.CreateIncome201JSONResponse(toIncome(inc)), nil
}

// UpdateIncome replaces the fields of an income (RF-12).
func (a *API) UpdateIncome(ctx context.Context, req gen.UpdateIncomeRequestObject) (gen.UpdateIncomeResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	in, err := fromIncomeInput(*req.Body)
	if err != nil {
		return nil, err
	}
	inc, err := a.d.Incomes.Update(ctx, req.Id, in)
	if err != nil {
		return nil, err
	}
	return gen.UpdateIncome200JSONResponse(toIncome(inc)), nil
}

// DeleteIncome removes an income without receipts.
func (a *API) DeleteIncome(ctx context.Context, req gen.DeleteIncomeRequestObject) (gen.DeleteIncomeResponseObject, error) {
	if err := a.d.Incomes.Delete(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteIncome204Response{}, nil
}

// GetIncomeMonth returns the expected and received incomes of the month (RF-10, RN-13).
func (a *API) GetIncomeMonth(ctx context.Context, req gen.GetIncomeMonthRequestObject) (gen.GetIncomeMonthResponseObject, error) {
	m, err := civil.ParseMonth(req.Params.Month)
	if err != nil {
		return nil, errBadMonth
	}
	v, err := a.d.Incomes.Month(ctx, m)
	if err != nil {
		return nil, err
	}
	out := gen.GetIncomeMonth200JSONResponse{
		Month: m.String(), PendingCents: v.PendingCents, ReceivedCents: v.ReceivedCents,
		Occurrences: make([]gen.IncomeOccurrence, 0, len(v.Occurrences)), OneOffs: make([]gen.OneOffIncome, 0, len(v.OneOffs)),
	}
	for _, o := range v.Occurrences {
		item := gen.IncomeOccurrence{
			IncomeId: o.IncomeID, Kind: gen.IncomeKind(o.Kind), Name: o.Name, PersonName: o.PersonName,
			AccountId: o.AccountID, AccountName: o.AccountName, ExpectedOn: apiDate(o.ExpectedOn),
			ExpectedCents: o.ExpectedCents, Confirmed: o.Confirmed, EntryId: o.EntryID,
		}
		if o.Confirmed {
			received := o.ReceivedCents
			item.ReceivedCents, item.ReceivedOn, item.ReceivedIn = &received, apiDatePtr(o.ReceivedOn), optional(o.ReceivedIn)
		}
		out.Occurrences = append(out.Occurrences, item)
	}
	for _, x := range v.OneOffs {
		out.OneOffs = append(out.OneOffs, gen.OneOffIncome{
			EntryId: x.EntryID, AmountCents: x.AmountCents, Date: apiDate(x.Date), Description: x.Description, AccountName: x.AccountName,
		})
	}
	return out, nil
}

// ConfirmIncome confirms the receipt of a month (RF-10, RN-13).
func (a *API) ConfirmIncome(ctx context.Context, req gen.ConfirmIncomeRequestObject) (gen.ConfirmIncomeResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	m, err := civil.ParseMonth(req.Body.Month)
	if err != nil {
		return nil, errBadMonth
	}
	c := income.Confirmation{IncomeID: req.Id, Month: m}
	if req.Body.AmountCents != nil {
		c.AmountCents = *req.Body.AmountCents
	}
	if req.Body.Date != nil {
		c.Date = fromDate(*req.Body.Date)
	}
	if req.Body.AccountId != nil {
		c.AccountID = *req.Body.AccountId
	}
	e, err := a.d.Incomes.Confirm(ctx, auth.UserID(ctx), c)
	if err != nil {
		return nil, err
	}
	return gen.ConfirmIncome201JSONResponse(toEntry(e)), nil
}

// AddAccountIncome records a one off income in the account (RF-13).
func (a *API) AddAccountIncome(ctx context.Context, req gen.AddAccountIncomeRequestObject) (gen.AddAccountIncomeResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	e, err := a.d.Ledger.AddIncome(ctx, auth.UserID(ctx), ledger.NewIncome{
		AccountID: req.Id, AmountCents: req.Body.AmountCents, Date: fromDate(req.Body.Date), Description: deref(req.Body.Description),
	})
	if err != nil {
		return nil, err
	}
	return gen.AddAccountIncome201JSONResponse(toEntry(e)), nil
}

// fromIncomeInput converts the API input, parsing the start month.
func fromIncomeInput(b gen.IncomeInput) (income.Input, error) {
	in := income.Input{
		Kind: income.Kind(b.Kind), Name: b.Name, PersonID: b.PersonId, AccountID: b.AccountId, AmountCents: b.AmountCents,
	}
	if b.Day != nil {
		in.Day = *b.Day
	}
	if b.StartMonth != nil {
		m, err := civil.ParseMonth(*b.StartMonth)
		if err != nil {
			return in, errBadMonth
		}
		in.StartMonth = m
	}
	if b.EndOn != nil {
		d := fromDate(*b.EndOn)
		in.EndOn = &d
	}
	if b.ExpectedOn != nil {
		in.ExpectedOn = fromDate(*b.ExpectedOn)
	}
	return in, nil
}

// toIncome converts an income to the API model, leaving out the fields of the other kind.
func toIncome(i income.Income) gen.Income {
	out := gen.Income{
		Id: i.ID, Kind: gen.IncomeKind(i.Kind), Name: i.Name, PersonId: i.PersonID, PersonName: i.PersonName,
		AccountId: i.AccountID, AccountName: i.AccountName, AmountCents: i.AmountCents,
		EndOn: apiDatePtr(i.EndOn), ExpectedOn: apiDatePtr(i.ExpectedOn), EndingSoon: i.EndingSoon, Ended: i.Ended,
	}
	if i.Kind == income.KindRecurring {
		day, start := i.Day, i.StartMonth.String()
		out.Day, out.StartMonth = &day, &start
	}
	return out
}

// apiDate converts a civil date to the API type.
func apiDate(d civil.Date) openapi_types.Date { return openapi_types.Date{Time: d.Time()} }

// apiDatePtr converts an optional civil date to the API type.
func apiDatePtr(d *civil.Date) *openapi_types.Date {
	if d == nil {
		return nil
	}
	v := apiDate(*d)
	return &v
}
