package server

import (
	"context"

	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/problem"
	"github.com/italogfx/radar-finance/internal/report"
)

// commitmentMonths is how many months ahead the overview shows commitments (RF-60).
const commitmentMonths = 6

// errBadRange is returned for a period that ends before it starts or is too long.
var errBadRange = problem.Invalid("invalid_range", "Período inválido. Use até 24 meses, com o fim depois do início.")

// GetSpending returns the spending of each month by category (RF-59, RN-04).
func (a *API) GetSpending(ctx context.Context, req gen.GetSpendingRequestObject) (gen.GetSpendingResponseObject, error) {
	from, err1 := civil.ParseMonth(req.Params.From)
	to, err2 := civil.ParseMonth(req.Params.To)
	if err1 != nil || err2 != nil {
		return nil, errBadMonth
	}
	if span := to.Year*12 + int(to.Month) - from.Year*12 - int(from.Month); span < 0 || span >= 24 {
		return nil, errBadRange
	}
	months, err := a.d.Reports.Spending(ctx, from, to)
	if err != nil {
		return nil, err
	}
	out := make(gen.GetSpending200JSONResponse, 0, len(months))
	for _, m := range months {
		ms := gen.MonthSpend{Month: m.Month.String(), TotalCents: m.TotalCents, Categories: make([]gen.CategorySpend, 0, len(m.Categories))}
		for _, c := range m.Categories {
			cs := gen.CategorySpend{Id: c.ID, Name: c.Name, Icon: c.Icon, Cents: c.Cents, Subs: make([]gen.SpendSub, 0, len(c.Subs))}
			for _, sub := range c.Subs {
				cs.Subs = append(cs.Subs, gen.SpendSub{Id: sub.ID, Name: sub.Name, Cents: sub.Cents})
			}
			ms.Categories = append(ms.Categories, cs)
		}
		out = append(out, ms)
	}
	return out, nil
}

// ListBudgets compares the budgets with the spending of the month (RF-51, RF-52).
func (a *API) ListBudgets(ctx context.Context, req gen.ListBudgetsRequestObject) (gen.ListBudgetsResponseObject, error) {
	m, err := civil.ParseMonth(req.Params.Month)
	if err != nil {
		return nil, errBadMonth
	}
	list, err := a.d.Reports.Budgets(ctx, m)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListBudgets200JSONResponse, 0, len(list))
	for _, b := range list {
		out = append(out, gen.BudgetStatus{
			CategoryId: b.CategoryID, Name: b.Name, Icon: b.Icon, BudgetCents: b.BudgetCents, SpentCents: b.SpentCents, UsedBp: b.UsedBP,
			Level: gen.BudgetStatusLevel(b.Level),
		})
	}
	return out, nil
}

// SetBudget sets or removes the budget of a top level category (RF-51).
func (a *API) SetBudget(ctx context.Context, req gen.SetBudgetRequestObject) (gen.SetBudgetResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	if err := a.d.Reports.SetBudget(ctx, auth.UserID(ctx), req.Id, req.Body.AmountCents); err != nil {
		return nil, err
	}
	return gen.SetBudget204Response{}, nil
}

// GetOverview returns the indicators of the month for the overview (RF-58, RF-60 to RF-62, section 5.3).
func (a *API) GetOverview(ctx context.Context, req gen.GetOverviewRequestObject) (gen.GetOverviewResponseObject, error) {
	m, err := civil.ParseMonth(req.Params.Month)
	if err != nil {
		return nil, errBadMonth
	}
	user, err := a.d.Accounts.Get(ctx, auth.UserID(ctx))
	if err != nil {
		return nil, err
	}
	r := a.d.Reports
	f, err := r.Forecast(ctx, m)
	if err != nil {
		return nil, err
	}
	h, err := r.Housing(ctx, m, user.PersonID)
	if err != nil {
		return nil, err
	}
	c, err := r.Closing(ctx, m)
	if err != nil {
		return nil, err
	}
	n, err := r.NetWorth(ctx)
	if err != nil {
		return nil, err
	}
	commitments, err := r.Commitments(ctx, m.Add(1), commitmentMonths)
	if err != nil {
		return nil, err
	}
	out := gen.GetOverview200JSONResponse{
		Month: m.String(),
		Forecast: gen.Forecast{
			IncomeCents: f.IncomeCents, ReceivedCents: f.ReceivedCents, ExpectedCents: f.ExpectedCents, OutCents: f.OutCents,
			BalanceCents: f.BalanceCents, BalanceBp: f.BalanceBP, Groups: make([]gen.ForecastGroup, 0, len(f.Groups)),
		},
		Housing: gen.Housing{
			HousingCents: h.HousingCents, CoupleIncomeCents: h.CoupleIncomeCents, PersonIncomeCents: h.PersonIncomeCents,
			CoupleBp: h.CoupleBP, PersonBp: h.PersonBP, ReferenceBp: h.ReferenceBP,
		},
		Closing: gen.Closing{
			IncomeCents: c.IncomeCents, SpendingCents: c.SpendingCents, SavedCents: c.SavedCents,
			ReceivablePendingCents: c.ReceivablePendingCents, ReceivedCents: c.ReceivedCents,
		},
		NetWorth: gen.NetWorth{
			AccountsCents: n.AccountsCents, SavingsCents: n.SavingsCents, FinancingCents: n.FinancingCents, CardDebtCents: n.CardDebtCents, NetCents: n.NetCents,
		},
		Commitments: make([]gen.Commitment, 0, len(commitments)),
	}
	for _, g := range f.Groups {
		out.Forecast.Groups = append(out.Forecast.Groups, gen.ForecastGroup{Key: gen.ForecastGroupKey(g.Key), Cents: g.Cents, ShareBp: g.ShareBP})
	}
	for _, cm := range commitments {
		out.Commitments = append(out.Commitments, gen.Commitment{
			Month: cm.Month.String(), CardCents: cm.CardCents, FinancingCents: cm.FinancingCents, TotalCents: cm.TotalCents, IncomeCents: cm.IncomeCents, Bp: cm.BP,
		})
	}
	return out, nil
}

// SearchEntries finds entries, card purchases and Alelo purchases (RF-63).
func (a *API) SearchEntries(ctx context.Context, req gen.SearchEntriesRequestObject) (gen.SearchEntriesResponseObject, error) {
	p := req.Params
	f := report.Filter{From: fromDate(p.From), To: fromDate(p.To), Text: deref(p.Text)}
	if f.To.Before(f.From) {
		return nil, errBadRange
	}
	for dst, src := range map[*int64]*int64{&f.CategoryID: p.CategoryId, &f.AccountID: p.AccountId, &f.CardID: p.CardId, &f.PersonID: p.PersonId} {
		if src != nil {
			*dst = *src
		}
	}
	lines, err := a.d.Reports.Search(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make(gen.SearchEntries200JSONResponse, 0, len(lines))
	for _, l := range lines {
		line := gen.SearchLine{
			Source: gen.SearchLineSource(l.Source), Id: l.ID, Date: apiDate(l.Date), Description: l.Description, AmountCents: l.AmountCents,
			Kind: l.Kind, CategoryName: l.CategoryName, ParentCategoryName: l.ParentCategoryName, AccountName: l.AccountName,
			CardName: l.CardName, PersonName: l.PersonName,
		}
		if l.CategoryID != 0 {
			id := l.CategoryID
			line.CategoryId = &id
		}
		out = append(out, line)
	}
	return out, nil
}

// ListNotifications returns the warnings of the user (RF-64).
func (a *API) ListNotifications(ctx context.Context, _ gen.ListNotificationsRequestObject) (gen.ListNotificationsResponseObject, error) {
	list, err := a.d.Notifications.List(ctx, auth.UserID(ctx))
	if err != nil {
		return nil, err
	}
	out := gen.ListNotifications200JSONResponse{Unread: list.Unread, Items: make([]gen.Notification, 0, len(list.Items))}
	for _, n := range list.Items {
		out.Items = append(out.Items, gen.Notification{
			Key: n.Key, Kind: gen.NotificationKind(n.Kind), Severity: gen.NotificationSeverity(n.Severity), Title: n.Title,
			Message: n.Message, Link: n.Link, Date: apiDate(n.Date), Read: n.Read,
		})
	}
	return out, nil
}

// MarkNotificationsRead marks the given notifications, or all, as read.
func (a *API) MarkNotificationsRead(ctx context.Context, req gen.MarkNotificationsReadRequestObject) (gen.MarkNotificationsReadResponseObject, error) {
	userID := auth.UserID(ctx)
	var err error
	if req.Body == nil || req.Body.Keys == nil {
		err = a.d.Notifications.MarkAllRead(ctx, userID)
	} else {
		err = a.d.Notifications.MarkRead(ctx, userID, *req.Body.Keys)
	}
	if err != nil {
		return nil, err
	}
	return gen.MarkNotificationsRead204Response{}, nil
}
