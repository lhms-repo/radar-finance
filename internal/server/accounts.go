package server

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/problem"
)

// errBadMonth is returned for a month outside the YYYY-MM form.
var errBadMonth = problem.Invalid("invalid_month", "Mês inválido.")

// ListPersons lists the people of the couple.
func (a *API) ListPersons(ctx context.Context, _ gen.ListPersonsRequestObject) (gen.ListPersonsResponseObject, error) {
	persons, err := a.d.Accounts.ListPersons(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListPersons200JSONResponse, 0, len(persons))
	for _, p := range persons {
		out = append(out, gen.Person{Id: p.ID, Name: p.Name})
	}
	return out, nil
}

// ListAccounts lists the accounts with their balances (RF-04, RF-05).
func (a *API) ListAccounts(ctx context.Context, _ gen.ListAccountsRequestObject) (gen.ListAccountsResponseObject, error) {
	list, err := a.d.Ledger.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListAccounts200JSONResponse, 0, len(list))
	for _, acc := range list {
		out = append(out, toAccount(acc))
	}
	return out, nil
}

// GetAccount returns one account (RF-05).
func (a *API) GetAccount(ctx context.Context, req gen.GetAccountRequestObject) (gen.GetAccountResponseObject, error) {
	acc, err := a.d.Ledger.GetAccount(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.GetAccount200JSONResponse(toAccount(acc)), nil
}

// CreateAccount registers an account (RF-04).
func (a *API) CreateAccount(ctx context.Context, req gen.CreateAccountRequestObject) (gen.CreateAccountResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	acc, err := a.d.Ledger.CreateAccount(ctx, auth.UserID(ctx), ledger.NewAccount{
		Name: req.Body.Name, PersonID: req.Body.PersonId, InitialBalanceCents: req.Body.InitialBalanceCents,
	})
	if err != nil {
		return nil, err
	}
	return gen.CreateAccount201JSONResponse(toAccount(acc)), nil
}

// UpdateAccount edits an account.
func (a *API) UpdateAccount(ctx context.Context, req gen.UpdateAccountRequestObject) (gen.UpdateAccountResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	acc, err := a.d.Ledger.UpdateAccount(ctx, req.Id, ledger.AccountChanges{
		Name: req.Body.Name, PersonID: req.Body.PersonId, InitialBalanceCents: req.Body.InitialBalanceCents, Active: req.Body.Active,
	})
	if err != nil {
		return nil, err
	}
	return gen.UpdateAccount200JSONResponse(toAccount(acc)), nil
}

// DeleteAccount removes an unused account.
func (a *API) DeleteAccount(ctx context.Context, req gen.DeleteAccountRequestObject) (gen.DeleteAccountResponseObject, error) {
	if err := a.d.Ledger.DeleteAccount(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteAccount204Response{}, nil
}

// GetAccountStatement returns the entries of the month with balances (RF-05).
func (a *API) GetAccountStatement(ctx context.Context, req gen.GetAccountStatementRequestObject) (gen.GetAccountStatementResponseObject, error) {
	m, err := civil.ParseMonth(req.Params.Month)
	if err != nil {
		return nil, errBadMonth
	}
	st, err := a.d.Ledger.Statement(ctx, req.Id, m)
	if err != nil {
		return nil, err
	}
	entries := make([]gen.Entry, 0, len(st.Entries))
	for _, e := range st.Entries {
		entries = append(entries, toEntry(e))
	}
	return gen.GetAccountStatement200JSONResponse{
		Month: st.Month.String(), OpeningCents: st.OpeningCents, ClosingCents: st.ClosingCents, Entries: entries,
	}, nil
}

// AdjustAccountBalance records an adjustment to the informed balance (RF-06).
func (a *API) AdjustAccountBalance(ctx context.Context, req gen.AdjustAccountBalanceRequestObject) (gen.AdjustAccountBalanceResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	e, err := a.d.Ledger.AdjustBalance(ctx, auth.UserID(ctx), ledger.Adjustment{
		AccountID: req.Id, BalanceCents: req.Body.BalanceCents, Date: fromDate(req.Body.Date), Description: deref(req.Body.Description),
	})
	if err != nil {
		return nil, err
	}
	return gen.AdjustAccountBalance201JSONResponse(toEntry(e)), nil
}

// AddAccountExpense records a one off expense from the account.
func (a *API) AddAccountExpense(ctx context.Context, req gen.AddAccountExpenseRequestObject) (gen.AddAccountExpenseResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	e, err := a.d.Ledger.AddExpense(ctx, auth.UserID(ctx), ledger.NewExpense{
		AccountID: req.Id, AmountCents: req.Body.AmountCents, Date: fromDate(req.Body.Date),
		CategoryID: req.Body.CategoryId, Description: deref(req.Body.Description),
	})
	if err != nil {
		return nil, err
	}
	return gen.AddAccountExpense201JSONResponse(toEntry(e)), nil
}

// CreateTransfer moves money between accounts (RF-07, RN-08).
func (a *API) CreateTransfer(ctx context.Context, req gen.CreateTransferRequestObject) (gen.CreateTransferResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	t, err := a.d.Ledger.Transfer(ctx, auth.UserID(ctx), ledger.NewTransfer{
		FromAccountID: req.Body.FromAccountId, ToAccountID: req.Body.ToAccountId, AmountCents: req.Body.AmountCents,
		Date: fromDate(req.Body.Date), Description: deref(req.Body.Description),
	})
	if err != nil {
		return nil, err
	}
	return gen.CreateTransfer201JSONResponse{Id: t.ID, OutEntryId: t.OutEntryID, InEntryId: t.InEntryID}, nil
}

// DeleteEntry removes an entry or a whole transfer.
func (a *API) DeleteEntry(ctx context.Context, req gen.DeleteEntryRequestObject) (gen.DeleteEntryResponseObject, error) {
	if err := a.d.Ledger.DeleteEntry(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteEntry204Response{}, nil
}

// GetMonthFlow returns confirmed income and spending of the month (RN-08).
func (a *API) GetMonthFlow(ctx context.Context, req gen.GetMonthFlowRequestObject) (gen.GetMonthFlowResponseObject, error) {
	m, err := civil.ParseMonth(req.Params.Month)
	if err != nil {
		return nil, errBadMonth
	}
	f, err := a.d.Ledger.MonthFlow(ctx, m)
	if err != nil {
		return nil, err
	}
	return gen.GetMonthFlow200JSONResponse{Month: m.String(), IncomeCents: f.IncomeCents, ExpenseCents: f.ExpenseCents}, nil
}

// toAccount converts an account to the API model.
func toAccount(a ledger.Account) gen.Account {
	return gen.Account{
		Id: a.ID, Name: a.Name, PersonId: a.PersonID, PersonName: a.PersonName,
		InitialBalanceCents: a.InitialBalanceCents, BalanceCents: a.BalanceCents, Active: a.Active,
	}
}

// toEntry converts an entry to the API model, leaving empty names out.
func toEntry(e ledger.Entry) gen.Entry {
	return gen.Entry{
		Id: e.ID, AccountId: e.AccountID, Kind: gen.EntryKind(e.Kind), AmountCents: e.AmountCents,
		Date: openapi_types.Date{Time: e.Date.Time()}, Confirmed: e.Confirmed, CategoryId: e.CategoryID,
		CategoryName: optional(e.CategoryName), ParentCategoryName: optional(e.ParentCategoryName),
		CategoryIcon: optional(e.CategoryIcon), Description: e.Description, TransferId: e.TransferID,
		CounterpartAccountName: optional(e.CounterpartAccountName), CreatedByName: e.CreatedByName, CreatedAt: e.CreatedAt,
	}
}

// fromDate converts an API date to a civil date.
func fromDate(d openapi_types.Date) civil.Date {
	if d.IsZero() {
		return civil.Date{}
	}
	return civil.DateOf(d.Time)
}

// optional returns nil for an empty string.
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
