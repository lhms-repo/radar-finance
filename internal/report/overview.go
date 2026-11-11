package report

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/italogfx/radar-finance/internal/bill"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/financing"
	"github.com/italogfx/radar-finance/internal/income"
	"github.com/italogfx/radar-finance/internal/receivable"
	"github.com/italogfx/radar-finance/internal/savings"
)

// HousingKey is the system key of the Moradia category.
const HousingKey = "housing"

// housingReferenceBP is the reference share of the income for the house shown on the overview (section 5.3).
const housingReferenceBP = 3_000

// GroupKey names a group of the expected balance bar (section 5.3).
type GroupKey string

// Groups of the expected balance, in the order of the bar.
const (
	GroupHousing  GroupKey = "housing"
	GroupInvoices GroupKey = "invoices"
	GroupBills    GroupKey = "bills"
	GroupDayToDay GroupKey = "day_to_day"
	GroupSavings  GroupKey = "savings"
)

// Group is one slice of the expected balance with its share of the income.
type Group struct {
	Key     GroupKey
	Cents   int64
	ShareBP int
}

// Forecast is the expected balance of the month (RF-58).
type Forecast struct {
	Month civil.Month
	// IncomeCents is what already came in plus what is still expected (RN-13).
	IncomeCents   int64
	ReceivedCents int64
	ExpectedCents int64
	Groups        []Group
	OutCents      int64
	BalanceCents  int64
	BalanceBP     int
}

// Housing is the share of the income that goes to the house (section 5.3).
type Housing struct {
	HousingCents      int64
	CoupleIncomeCents int64
	PersonIncomeCents int64
	CoupleBP          int
	PersonBP          int
	ReferenceBP       int
}

// Commitment is what a future month already owes in installments and financings (RF-60).
type Commitment struct {
	Month          civil.Month
	CardCents      int64
	FinancingCents int64
	TotalCents     int64
	IncomeCents    int64
	BP             int
}

// Closing sums the month: what came in, what was spent, what was saved and what others owe (RF-61).
type Closing struct {
	Month                  civil.Month
	IncomeCents            int64
	SpendingCents          int64
	SavedCents             int64
	ReceivablePendingCents int64
	ReceivedCents          int64
}

// NetWorth is accounts and savings minus what is owed (RF-62).
type NetWorth struct {
	AccountsCents  int64
	SavingsCents   int64
	FinancingCents int64
	CardDebtCents  int64
	NetCents       int64
}

// Forecast returns the income of the month minus every expected way out, by group (RF-58).
// Housing holds the bills and installments filed under Moradia; invoices count by the couple
// share (RN-04); day to day is the one off spending from accounts; savings use the confirmed
// deposit, or the suggested one while none is confirmed.
func (s *Service) Forecast(ctx context.Context, m civil.Month) (Forecast, error) {
	inc, err := income.NewService(s.conn).Month(ctx, m)
	if err != nil {
		return Forecast{}, err
	}
	f := Forecast{Month: m, ReceivedCents: inc.ReceivedCents, ExpectedCents: inc.PendingCents, IncomeCents: inc.ReceivedCents + inc.PendingCents}
	sums := map[GroupKey]int64{}

	cards, err := s.cards.ListCards(ctx)
	if err != nil {
		return Forecast{}, err
	}
	for _, c := range cards {
		inv, err := s.cards.Invoice(ctx, c.ID, m)
		if err != nil {
			return Forecast{}, err
		}
		sums[GroupInvoices] += max(0, inv.NetCents)
	}

	housing, err := s.housingCategories(ctx)
	if err != nil {
		return Forecast{}, err
	}
	bills, err := bill.NewService(s.conn).Month(ctx, m)
	if err != nil {
		return Forecast{}, err
	}
	for _, b := range bills.Bills {
		sums[groupOf(housing, b.CategoryID)] += b.AmountCents
	}
	fins, err := s.financingCategories(ctx)
	if err != nil {
		return Forecast{}, err
	}
	installments, err := financing.NewService(s.conn).Month(ctx, m)
	if err != nil {
		return Forecast{}, err
	}
	for _, it := range installments {
		sums[groupOf(housing, fins[it.FinancingID])] += it.AmountCents
	}

	day, err := store.New(s.conn).DayToDaySpending(ctx, store.DayToDaySpendingParams{FirstDay: m.First().String(), LastDay: m.Last().String()})
	if err != nil {
		return Forecast{}, fmt.Errorf("day to day spending: %w", err)
	}
	sums[GroupDayToDay] = day

	boxes, err := savings.NewService(s.conn).Month(ctx, m)
	if err != nil {
		return Forecast{}, err
	}
	for _, b := range boxes.Boxes {
		if b.DepositedCents > 0 {
			sums[GroupSavings] += b.DepositedCents
		} else {
			sums[GroupSavings] += b.SuggestedCents
		}
	}

	for _, k := range []GroupKey{GroupHousing, GroupInvoices, GroupBills, GroupDayToDay, GroupSavings} {
		f.Groups = append(f.Groups, Group{Key: k, Cents: sums[k], ShareBP: share(sums[k], f.IncomeCents)})
		f.OutCents += sums[k]
	}
	f.BalanceCents = f.IncomeCents - f.OutCents
	f.BalanceBP = share(f.BalanceCents, f.IncomeCents)
	return f, nil
}

// Housing returns what the house costs in the month over the income of the couple and of one person (section 5.3).
// It is the Moradia spending of the month plus the Moradia bills and installments still unpaid.
func (s *Service) Housing(ctx context.Context, m civil.Month, personID int64) (Housing, error) {
	spend, err := s.Spending(ctx, m, m)
	if err != nil {
		return Housing{}, err
	}
	housing, err := s.housingCategories(ctx)
	if err != nil {
		return Housing{}, err
	}
	var h Housing
	for _, c := range spend[0].Categories {
		if housing[c.ID] {
			h.HousingCents += c.Cents
		}
	}
	bills, err := bill.NewService(s.conn).Month(ctx, m)
	if err != nil {
		return Housing{}, err
	}
	for _, b := range bills.Bills {
		if !b.Paid && groupOf(housing, b.CategoryID) == GroupHousing {
			h.HousingCents += b.AmountCents
		}
	}
	fins, err := s.financingCategories(ctx)
	if err != nil {
		return Housing{}, err
	}
	installments, err := financing.NewService(s.conn).Month(ctx, m)
	if err != nil {
		return Housing{}, err
	}
	for _, it := range installments {
		if !it.Paid && groupOf(housing, fins[it.FinancingID]) == GroupHousing {
			h.HousingCents += it.AmountCents
		}
	}

	inc, err := income.NewService(s.conn).Month(ctx, m)
	if err != nil {
		return Housing{}, err
	}
	h.CoupleIncomeCents = inc.ReceivedCents + inc.PendingCents
	for _, o := range inc.Occurrences {
		if o.PersonID != personID {
			continue
		}
		if o.Confirmed {
			h.PersonIncomeCents += o.ReceivedCents
		} else {
			h.PersonIncomeCents += o.ExpectedCents
		}
	}
	for _, o := range inc.OneOffs {
		if o.PersonID == personID {
			h.PersonIncomeCents += o.AmountCents
		}
	}
	h.CoupleBP, h.PersonBP, h.ReferenceBP = share(h.HousingCents, h.CoupleIncomeCents), share(h.HousingCents, h.PersonIncomeCents), housingReferenceBP
	return h, nil
}

// Commitments returns, for n months from first, the card installments (couple share) and financing
// installments already owed, against the expected income of each month (RF-60).
func (s *Service) Commitments(ctx context.Context, first civil.Month, n int) ([]Commitment, error) {
	last := first.Add(n - 1)
	rows, err := store.New(s.conn).CardInstallmentsByMonth(ctx, store.CardInstallmentsByMonthParams{FirstMonth: first.String(), LastMonth: last.String()})
	if err != nil {
		return nil, fmt.Errorf("card installments: %w", err)
	}
	card := map[string]int64{}
	for _, r := range rows {
		card[r.Month] = max(0, r.Cents)
	}
	incomes := income.NewService(s.conn)
	fins := financing.NewService(s.conn)
	out := make([]Commitment, 0, n)
	for m := first; monthIndex(m) <= monthIndex(last); m = m.Add(1) {
		c := Commitment{Month: m, CardCents: card[m.String()]}
		installments, err := fins.Month(ctx, m)
		if err != nil {
			return nil, err
		}
		for _, it := range installments {
			c.FinancingCents += it.AmountCents
		}
		inc, err := incomes.Month(ctx, m)
		if err != nil {
			return nil, err
		}
		c.IncomeCents = inc.ReceivedCents + inc.PendingCents
		c.TotalCents = c.CardCents + c.FinancingCents
		c.BP = share(c.TotalCents, c.IncomeCents)
		out = append(out, c)
	}
	return out, nil
}

// Closing sums the month (RF-61): income that came in by date, couple spending (RN-04), net
// savings and what other people owe and paid back.
func (s *Service) Closing(ctx context.Context, m civil.Month) (Closing, error) {
	q := store.New(s.conn)
	days := store.SavedInPeriodParams{FirstDay: m.First().String(), LastDay: m.Last().String()}
	c := Closing{Month: m}
	var err error
	if c.IncomeCents, err = q.IncomeInPeriod(ctx, store.IncomeInPeriodParams(days)); err != nil {
		return Closing{}, fmt.Errorf("income of the month: %w", err)
	}
	spend, err := s.Spending(ctx, m, m)
	if err != nil {
		return Closing{}, err
	}
	c.SpendingCents = spend[0].TotalCents
	if c.SavedCents, err = q.SavedInPeriod(ctx, days); err != nil {
		return Closing{}, err
	}
	if c.ReceivedCents, err = q.ReceivedInPeriod(ctx, store.ReceivedInPeriodParams(days)); err != nil {
		return Closing{}, err
	}
	people, err := receivable.NewService(s.conn).List(ctx)
	if err != nil {
		return Closing{}, err
	}
	for _, p := range people {
		c.ReceivablePendingCents += p.PendingCents
	}
	return c, nil
}

// NetWorth returns accounts and savings minus financing balances and closed unpaid invoices (RF-62).
func (s *Service) NetWorth(ctx context.Context) (NetWorth, error) {
	var n NetWorth
	var err error
	if n.AccountsCents, err = store.New(s.conn).AccountsTotal(ctx); err != nil {
		return NetWorth{}, fmt.Errorf("accounts total: %w", err)
	}
	boxes, err := savings.NewService(s.conn).Month(ctx, s.today().MonthOf())
	if err != nil {
		return NetWorth{}, err
	}
	for _, b := range boxes.Boxes {
		n.SavingsCents += b.BalanceCents
	}
	fins, err := financing.NewService(s.conn).List(ctx)
	if err != nil {
		return NetWorth{}, err
	}
	for _, f := range fins {
		n.FinancingCents += f.BalanceCents
	}
	if n.CardDebtCents, err = s.cards.ClosedDebt(ctx); err != nil {
		return NetWorth{}, err
	}
	n.NetCents = n.AccountsCents + n.SavingsCents - n.FinancingCents - n.CardDebtCents
	return n, nil
}

// housingCategories returns the Moradia category and its subcategories, by id.
func (s *Service) housingCategories(ctx context.Context) (map[int64]bool, error) {
	id, err := store.New(s.conn).GetCategoryBySystemKey(ctx, sql.NullString{String: HousingKey, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("housing category: %w", err)
	}
	tree, err := s.categories.List(ctx, true)
	if err != nil {
		return nil, err
	}
	out := map[int64]bool{id: true}
	for _, c := range tree {
		if c.ID == id {
			for _, sub := range c.Children {
				out[sub.ID] = true
			}
		}
	}
	return out, nil
}

// financingCategories maps each financing to its category.
func (s *Service) financingCategories(ctx context.Context) (map[int64]int64, error) {
	list, err := financing.NewService(s.conn).List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]int64, len(list))
	for _, f := range list {
		out[f.ID] = f.CategoryID
	}
	return out, nil
}

// groupOf puts a bill or installment in the housing group or in the bills group.
func groupOf(housing map[int64]bool, categoryID int64) GroupKey {
	if housing[categoryID] {
		return GroupHousing
	}
	return GroupBills
}

// share returns part over whole in basis points, rounded down; zero when whole is not positive.
func share(part, whole int64) int {
	if whole <= 0 {
		return 0
	}
	return int(part * fullBP / whole)
}
