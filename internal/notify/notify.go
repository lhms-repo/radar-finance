// Package notify derives the in app warnings from the current state: due soon,
// overdue, budgets at 80% and 100%, deposits waiting for confirmation and
// temporary incomes ending (RF-52, RF-64). Only the reads are stored, per user.
package notify

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/italogfx/radar-finance/internal/agenda"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/config"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/income"
	"github.com/italogfx/radar-finance/internal/money"
	"github.com/italogfx/radar-finance/internal/report"
	"github.com/italogfx/radar-finance/internal/savings"
	"github.com/italogfx/radar-finance/internal/scope"
)

// dueSoonDays is how many days ahead a payment counts as due soon.
const dueSoonDays = 3

// overdueMonths is how many past months are checked for unpaid items; months before an item was
// registered in the Radar are never checked (RF-64).
const overdueMonths = 12

// Kind tells what a notification is about.
type Kind string

// Notification kinds (RF-64).
const (
	KindOverdue        Kind = "overdue"
	KindBudgetOver     Kind = "budget_over"
	KindDueSoon        Kind = "due_soon"
	KindBudgetWarning  Kind = "budget_warning"
	KindSavingsPending Kind = "savings_pending"
	KindIncomeEnding   Kind = "income_ending"
)

// Severity orders notifications from the most urgent.
type Severity string

// Severities.
const (
	SeverityDanger  Severity = "danger"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Notification is one warning shown under the bell.
type Notification struct {
	Key      string
	Kind     Kind
	Severity Severity
	Title    string
	Message  string
	// Link is the screen that solves it.
	Link string
	Date civil.Date
	Read bool
}

// List is every current notification with the unread count.
type List struct {
	Items  []Notification
	Unread int
}

// Service derives notifications and stores reads.
type Service struct {
	conn    *sql.DB
	agenda  *agenda.Service
	reports *report.Service
	savings *savings.Service
	incomes *income.Service
	now     func() time.Time
	today   func() civil.Date
	loc     *time.Location
}

// NewService creates the notification service; today is read in America/Sao_Paulo.
func NewService(conn *sql.DB, ag *agenda.Service, reports *report.Service, sav *savings.Service, incomes *income.Service) *Service {
	loc, err := time.LoadLocation(config.Location)
	if err != nil {
		loc = time.UTC
	}
	return &Service{conn: conn, agenda: ag, reports: reports, savings: sav, incomes: incomes, now: time.Now, today: func() civil.Date { return civil.Today(loc) }, loc: loc}
}

// List returns the current notifications of the user, most urgent first (RF-64).
func (s *Service) List(ctx context.Context, userID int64) (List, error) {
	today := s.today()
	m := today.MonthOf()
	var items []Notification

	registered, first, err := s.registrations(ctx)
	if err != nil {
		return List{}, err
	}
	start := m.Add(-overdueMonths)
	if monthIndex(first) > monthIndex(start) {
		start = first
	}
	for month := start; monthIndex(month) <= monthIndex(m.Add(1)); month = month.Add(1) {
		ag, err := s.agenda.Month(ctx, month)
		if err != nil {
			return List{}, err
		}
		for _, it := range ag.Items {
			if it.Paid || it.AmountCents <= 0 {
				continue
			}
			if reg, ok := registered[string(it.Kind)+":"+strconv.FormatInt(it.RefID, 10)]; ok && monthIndex(it.DueOn.MonthOf()) < monthIndex(reg) {
				continue
			}
			ref := fmt.Sprintf("%s:%d:%s", it.Kind, it.RefID, it.DueOn.MonthOf())
			switch {
			case it.DueOn.Before(today):
				items = append(items, Notification{
					Key: "overdue:" + ref, Kind: KindOverdue, Severity: SeverityDanger, Title: overdueTitle(it), Link: link(it.Kind), Date: it.DueOn,
					Message: fmt.Sprintf("Venceu em %s e ainda não foi paga. Valor: %s.", dayMonth(it.DueOn), money.Format(it.AmountCents)),
				})
			case !today.AddDays(dueSoonDays).Before(it.DueOn):
				items = append(items, Notification{
					Key: "due_soon:" + ref, Kind: KindDueSoon, Severity: SeverityWarning, Title: dueSoonTitle(it), Link: link(it.Kind), Date: it.DueOn,
					Message: fmt.Sprintf("Vence em %s. Valor: %s.", dayMonth(it.DueOn), money.Format(it.AmountCents)),
				})
			}
		}
	}

	budgets, err := s.reports.Budgets(ctx, m)
	if err != nil {
		return List{}, err
	}
	for _, b := range budgets {
		msg := fmt.Sprintf("Gasto de %s para um orçamento de %s.", money.Format(b.SpentCents), money.Format(b.BudgetCents))
		key := fmt.Sprintf("%d:%s", b.CategoryID, m)
		switch b.Level {
		case report.LevelOver:
			items = append(items, Notification{Key: "budget_over:" + key, Kind: KindBudgetOver, Severity: SeverityDanger, Title: "Orçamento de " + b.Name + " estourado", Message: msg, Link: "/settings/budgets", Date: today})
		case report.LevelWarning:
			items = append(items, Notification{Key: "budget_warning:" + key, Kind: KindBudgetWarning, Severity: SeverityWarning, Title: fmt.Sprintf("Orçamento de %s em %d%%", b.Name, b.UsedBP/100), Message: msg, Link: "/settings/budgets", Date: today})
		}
	}

	boxes, err := s.savings.Month(ctx, m)
	if err != nil {
		return List{}, err
	}
	for _, b := range boxes.Boxes {
		if b.SuggestedCents > 0 {
			items = append(items, Notification{
				Key: fmt.Sprintf("savings_pending:%d:%s", b.ID, m), Kind: KindSavingsPending, Severity: SeverityInfo, Title: "Aporte pendente: " + b.Name,
				Message: fmt.Sprintf("Sugestão de %s para este mês. Confirme quando guardar.", money.Format(b.SuggestedCents)), Link: "/savings", Date: today,
			})
		}
	}

	incomes, err := s.incomes.List(ctx)
	if err != nil {
		return List{}, err
	}
	for _, inc := range incomes {
		if inc.EndingSoon && inc.EndOn != nil {
			items = append(items, Notification{
				Key: fmt.Sprintf("income_ending:%d:%s", inc.ID, inc.EndOn), Kind: KindIncomeEnding, Severity: SeverityInfo,
				Title:   fmt.Sprintf("%s termina em %02d/%02d/%d", inc.Name, inc.EndOn.Day, inc.EndOn.Month, inc.EndOn.Year),
				Message: "Confira se ela será renovada e ajuste a data de término.", Link: "/settings/incomes", Date: *inc.EndOn,
			})
		}
	}

	reads, err := store.New(s.conn).ListNotificationReads(ctx, userID)
	if err != nil {
		return List{}, fmt.Errorf("list reads: %w", err)
	}
	read := map[string]bool{}
	for _, k := range reads {
		read[k] = true
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return List{}, err
	}
	out := List{Items: []Notification{}}
	for _, n := range items {
		// The same card or category can warn in two groups, so reading it in one keeps it unread in the other.
		n.Key = fmt.Sprintf("g%d:%s", groupID, n.Key)
		n.Read = read[n.Key]
		if !n.Read {
			out.Unread++
		}
		out.Items = append(out.Items, n)
	}
	sort.SliceStable(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if rank[a.Kind] != rank[b.Kind] {
			return rank[a.Kind] < rank[b.Kind]
		}
		if a.Date != b.Date {
			return a.Date.Before(b.Date)
		}
		return a.Key < b.Key
	})
	return out, nil
}

// rank orders the kinds: danger first, then warning, then info.
var rank = map[Kind]int{KindOverdue: 0, KindBudgetOver: 1, KindDueSoon: 2, KindBudgetWarning: 3, KindSavingsPending: 4, KindIncomeEnding: 5}

// MarkRead marks notifications as read for the user.
func (s *Service) MarkRead(ctx context.Context, userID int64, keys []string) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		now := s.now().UTC()
		for _, k := range keys {
			if err := q.MarkNotificationRead(ctx, store.MarkNotificationReadParams{UserID: userID, Key: k, ReadAt: now}); err != nil {
				return err
			}
		}
		return nil
	})
}

// MarkAllRead marks every current notification as read for the user.
func (s *Service) MarkAllRead(ctx context.Context, userID int64) error {
	list, err := s.List(ctx, userID)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(list.Items))
	for _, n := range list.Items {
		keys = append(keys, n.Key)
	}
	return s.MarkRead(ctx, userID, keys)
}

// overdueTitle names an overdue item by kind.
func overdueTitle(it agenda.Item) string {
	switch it.Kind {
	case agenda.KindInvoice:
		return "Fatura atrasada: " + it.Name
	case agenda.KindFinancing:
		return "Parcela atrasada: " + it.Name
	default:
		return "Conta atrasada: " + it.Name
	}
}

// dueSoonTitle names an item due soon by kind.
func dueSoonTitle(it agenda.Item) string {
	switch it.Kind {
	case agenda.KindInvoice:
		return "Fatura vence em breve: " + it.Name
	case agenda.KindFinancing:
		return "Parcela vence em breve: " + it.Name
	default:
		return "Vence em breve: " + it.Name
	}
}

// link returns the screen where an agenda item is paid.
func link(k agenda.Kind) string {
	if k == agenda.KindInvoice {
		return "/cards"
	}
	return "/bills"
}

// dayMonth writes a date as day and month, like 05/11.
func dayMonth(d civil.Date) string { return fmt.Sprintf("%02d/%02d", d.Day, d.Month) }

// registrations returns the month each bill, financing and card was registered, keyed by agenda kind and
// id, and the earliest of them, so no month before the Radar knew about an item is checked (RF-64).
func (s *Service) registrations(ctx context.Context) (map[string]civil.Month, civil.Month, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return nil, civil.Month{}, err
	}
	rows, err := store.New(s.conn).ListRegistrations(ctx, groupID)
	if err != nil {
		return nil, civil.Month{}, fmt.Errorf("list registrations: %w", err)
	}
	out := make(map[string]civil.Month, len(rows))
	first := s.today().MonthOf()
	for _, r := range rows {
		m := civil.DateOf(r.CreatedAt.In(s.loc)).MonthOf()
		out[r.Kind+":"+strconv.FormatInt(r.ID, 10)] = m
		if monthIndex(m) < monthIndex(first) {
			first = m
		}
	}
	return out, first, nil
}

// monthIndex orders months as a single number.
func monthIndex(m civil.Month) int { return m.Year*12 + int(m.Month) }
